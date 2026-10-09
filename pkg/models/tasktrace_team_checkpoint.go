// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"xorm.io/xorm"
)

// TaskTraceTeamSyncCheckpoint commits alongside imported tasks and comments.
// The recovery journal is written first, so a process exit cannot turn an
// uncommitted import into a local deletion on the next synchronization.
type TaskTraceTeamSyncCheckpoint struct {
	ID         string `xorm:"varchar(64) not null pk"`
	Generation string `xorm:"varchar(36) not null"`
}

func (TaskTraceTeamSyncCheckpoint) TableName() string { return "tasktrace_team_sync_checkpoints" }

type taskTraceTeamSyncJournal struct {
	ID         string                 `json:"id"`
	Generation string                 `json:"generation"`
	Before     TaskTraceTeamBinding   `json:"before"`
	Previous   *TaskTraceTeamSnapshot `json:"previous,omitempty"`
}

func taskTraceTeamBeginSync(s *xorm.Session, binding *TaskTraceTeamBinding, actor, device string) (*TaskTraceTeamSyncCheckpoint, *TaskTraceTeamSnapshot, error) {
	if !s.IsInTx() {
		return nil, nil, errors.New("team synchronization requires a database transaction")
	}
	digest := sha256.Sum256([]byte(binding.ShareID + "\x00" + device + "\x00" + strings.ToLower(actor)))
	id := hex.EncodeToString(digest[:])
	path := filepath.Join(filepath.Dir(taskTraceTeamStatePath()), "team-sync-recovery", id+".json")
	var previous *TaskTraceTeamSnapshot
	recovered := false
	var journal taskTraceTeamSyncJournal
	if err := taskTraceTeamReadJSON(path, &journal); err == nil {
		if journal.ID != id || journal.Generation == "" || journal.Before.ShareID != binding.ShareID || journal.Before.RootTaskID != binding.RootTaskID {
			return nil, nil, errors.New("team synchronization recovery journal does not match the local task")
		}
		var committed TaskTraceTeamSyncCheckpoint
		found, err := s.ID(id).Get(&committed)
		if err != nil {
			return nil, nil, err
		}
		if !found || committed.Generation != journal.Generation {
			// Keep personal preferences and a relocated repository path. Only the
			// synchronization metadata must return to the pre-transaction state.
			before := journal.Before
			before.Repository, before.Secret, before.Notify = binding.Repository, binding.Secret, binding.Notify
			before.OutstandingPriorities = binding.OutstandingPriorities
			*binding = before
			previous = journal.Previous
			recovered = true
		}
	} else if !os.IsNotExist(err) {
		return nil, nil, err
	}
	if !recovered && (binding.LastSnapshotHash != "" || !binding.LastSync.IsZero()) {
		var stored TaskTraceTeamSnapshot
		if err := taskTraceTeamReadJSON(taskTraceTeamSnapshotPath(binding, actor, device), &stored); err == nil {
			previous = &stored
		} else if !os.IsNotExist(err) {
			return nil, nil, err
		}
	}
	checkpoint := &TaskTraceTeamSyncCheckpoint{ID: id, Generation: uuid.NewString()}
	journal = taskTraceTeamSyncJournal{ID: id, Generation: checkpoint.Generation, Before: *binding, Previous: previous}
	if err := taskTraceTeamWriteJSON(path, &journal); err != nil {
		return nil, nil, err
	}
	return checkpoint, previous, nil
}

func taskTraceTeamFinishSync(s *xorm.Session, checkpoint *TaskTraceTeamSyncCheckpoint, binding *TaskTraceTeamBinding) error {
	stable := *binding
	stable.LastSync = time.Time{}
	content, err := json.Marshal(stable)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(content)
	checkpoint.Generation = hex.EncodeToString(digest[:16])
	path := filepath.Join(filepath.Dir(taskTraceTeamStatePath()), "team-sync-recovery", checkpoint.ID+".json")
	var journal taskTraceTeamSyncJournal
	if err := taskTraceTeamReadJSON(path, &journal); err != nil {
		return err
	}
	journal.Generation = checkpoint.Generation
	if err := taskTraceTeamWriteJSON(path, &journal); err != nil {
		return err
	}
	var committed TaskTraceTeamSyncCheckpoint
	found, err := s.ID(checkpoint.ID).Get(&committed)
	if err != nil {
		return err
	}
	if found && committed.Generation == checkpoint.Generation {
		// Idle polling must not write to SQLite or trigger database-change UI refreshes.
		return nil
	}
	// The enclosing transaction/savepoint makes replacement atomic even when
	// the process is killed before its final Commit.
	if _, err := s.ID(checkpoint.ID).Delete(&TaskTraceTeamSyncCheckpoint{}); err != nil {
		return err
	}
	_, err = s.Insert(checkpoint)
	return err
}
