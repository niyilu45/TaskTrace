// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

// Reimporting the same authenticated share repairs its endpoint, rather than
// making duplicate local tasks or discarding local edits and personal structure.
func taskTraceTeamReconnectBinding(s *xorm.Session, a web.Auth, state *taskTraceTeamState, binding *TaskTraceTeamBinding, repository string, link taskTraceTeamLink, manifest TaskTraceTeamManifest, actor string) (*TaskTraceTeamStatus, error) {
	root := &Task{ID: binding.RootTaskID}
	canRead, _, err := root.CanRead(s, a)
	if err != nil {
		return nil, err
	}
	if !canRead {
		return nil, ErrGenericForbidden{}
	}
	canWriteProject, err := (&Project{ID: root.ProjectID}).CanWrite(s, a)
	if err != nil {
		return nil, err
	}
	if !canWriteProject {
		return nil, ErrGenericForbidden{}
	}
	if binding.Secret != link.Secret || !taskTraceTeamMembersEqual(binding.Owner, manifest.Owner) || binding.NodeTasks[manifest.RootNode] != binding.RootTaskID {
		return nil, errors.New("the new link does not match the existing collaboration identity or shared root")
	}
	var previous *TaskTraceTeamSnapshot
	var snapshot TaskTraceTeamSnapshot
	if err := taskTraceTeamReadJSON(taskTraceTeamSnapshotPath(binding, actor, state.DeviceID), &snapshot); err == nil {
		if snapshot.Schema != taskTraceTeamSchema || snapshot.ShareID != binding.ShareID || snapshot.DeviceID != state.DeviceID || !taskTraceTeamMembersEqual(snapshot.Actor, actor) {
			return nil, errors.New("the original collaboration snapshot does not match this installation")
		}
		previous = &snapshot
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read original collaboration history before reconnecting: %w", err)
	}
	originalRepository := binding.Repository
	binding.Repository = repository
	binding.Members = manifest.Members
	release, err := taskTraceTeamAcquireShareLock(binding)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := taskTraceTeamValidateBinding(binding, actor); err != nil {
		return nil, err
	}
	if !strings.EqualFold(filepath.Clean(originalRepository), filepath.Clean(repository)) {
		binding.Base, err = taskTraceTeamReconnectionBase(binding, originalRepository, actor, state.DeviceID)
		if err != nil {
			return nil, err
		}
	}
	if err := taskTraceTeamMergeBinding(s, a, state, binding, actor, previous); err != nil {
		return nil, err
	}
	if err := taskTraceTeamSaveState(*state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLockedFast(s, a, *state)
	return &status, err
}

func taskTraceTeamReconnectionBase(binding *TaskTraceTeamBinding, originalRepository, actor, device string) (map[string]TaskTraceTeamBase, error) {
	// Preserve the destination's pre-reconnection snapshot before the normal
	// writer replaces its file. The deterministic archive also preserves
	// the common baseline if a reconnect is interrupted and retried.
	archiveID := fmt.Sprintf("reconnect-%x", sha256.Sum256([]byte(originalRepository+"\x00"+actor+"\x00"+device)))
	archivePath := taskTraceTeamSnapshotPath(binding, actor, archiveID)
	var destinationSnapshot TaskTraceTeamSnapshot
	if err := taskTraceTeamReadJSON(archivePath, &destinationSnapshot); os.IsNotExist(err) {
		if err := taskTraceTeamReadJSON(taskTraceTeamSnapshotPath(binding, actor, device), &destinationSnapshot); err == nil {
			if destinationSnapshot.Schema != taskTraceTeamSchema || destinationSnapshot.ShareID != binding.ShareID || destinationSnapshot.DeviceID != device || !taskTraceTeamMembersEqual(destinationSnapshot.Actor, actor) {
				return nil, errors.New("the destination collaboration snapshot does not match this installation")
			}
		} else if os.IsNotExist(err) {
			destinationSnapshot = TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: actor}
		} else {
			return nil, fmt.Errorf("read destination collaboration history: %w", err)
		}
		destinationSnapshot.DeviceID = archiveID
		if err := taskTraceTeamWriteSnapshot(binding, destinationSnapshot); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if destinationSnapshot.Schema != taskTraceTeamSchema || destinationSnapshot.ShareID != binding.ShareID || destinationSnapshot.DeviceID != archiveID || !taskTraceTeamMembersEqual(destinationSnapshot.Actor, actor) {
		return nil, errors.New("the saved reconnection baseline does not match this installation")
	}
	// An edit acknowledged by the old endpoint was not acknowledged by the
	// destination. Use their last common values, not the old endpoint's
	// latest values, or previously saved local edits would look unchanged.
	destinationBase := map[string]TaskTraceTeamBase{}
	for node := range binding.NodeTasks {
		destinationBase[node] = TaskTraceTeamBase{}
	}
	for node, base := range taskTraceTeamBaseFromSnapshot(destinationSnapshot) {
		destinationBase[node] = base
	}
	return destinationBase, nil
}
