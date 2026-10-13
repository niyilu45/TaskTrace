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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

type TaskTraceNotificationHistorySettings struct {
	MaxEntries int `json:"max_entries" minimum:"1" maximum:"10000" default:"100" doc:"Maximum cleared notifications retained on this installation. Oldest history is removed when this limit is exceeded; defaults to 100."`
}

type TaskTraceNotificationHistoryEntry struct {
	ID           string                              `json:"id" readOnly:"true" doc:"Stable history identity, scoped by notification source and collaboration share."`
	ArchivedAt   time.Time                           `json:"archived_at" readOnly:"true" doc:"When the notification was cleared into history."`
	Team         *TaskTraceTeamNotification          `json:"team,omitempty" readOnly:"true" doc:"Archived collaboration update, if this entry came from a shared task."`
	Notification *notifications.DatabaseNotification `json:"notification,omitempty" readOnly:"true" doc:"Archived ordinary notification, if this entry came from the personal notification list."`
}

type taskTraceNotificationHistoryRecord struct {
	TaskTraceNotificationHistoryEntry
	ProjectID int64 `json:"project_id,omitempty"`
}

type taskTraceNotificationHistoryState struct {
	Settings TaskTraceNotificationHistorySettings `json:"settings"`
	Entries  []taskTraceNotificationHistoryRecord `json:"entries"`
	// Keep lightweight receipts after history expires so an older peer replaying
	// the same shared file cannot put an already-cleared notice back in the inbox.
	Dismissed map[string]bool `json:"dismissed"`
}

var taskTraceNotificationHistoryMu sync.Mutex

func taskTraceNotificationHistoryPath(username string) string {
	if taskTraceTeamStatePath() == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(taskTraceTeamStatePath()), "notification-history", taskTraceTeamTokenHash(strings.ToLower(username))+".json")
}

func taskTraceNotificationHistoryLoad(username string) (taskTraceNotificationHistoryState, error) {
	state := taskTraceNotificationHistoryState{Settings: TaskTraceNotificationHistorySettings{MaxEntries: 100}, Entries: []taskTraceNotificationHistoryRecord{}, Dismissed: map[string]bool{}}
	path := taskTraceNotificationHistoryPath(username)
	if path == "" {
		return state, errors.New("TaskTrace notification history is unavailable")
	}
	err := taskTraceTeamReadJSON(path, &state)
	if os.IsNotExist(err) {
		err = nil
	}
	if state.Settings.MaxEntries < 1 || state.Settings.MaxEntries > 10000 {
		state.Settings.MaxEntries = 100
	}
	if state.Dismissed == nil {
		state.Dismissed = map[string]bool{}
	}
	return state, err
}

func taskTraceNotificationHistoryUser(a web.Auth) (*user.User, error) {
	if isLinkShare(a) {
		return nil, ErrGenericForbidden{}
	}
	return user.GetFromAuth(a)
}

func TaskTraceNotificationSettings(a web.Auth, next *TaskTraceNotificationHistorySettings) (*TaskTraceNotificationHistorySettings, error) {
	u, err := taskTraceNotificationHistoryUser(a)
	if err != nil {
		return nil, err
	}
	taskTraceNotificationHistoryMu.Lock()
	defer taskTraceNotificationHistoryMu.Unlock()
	state, err := taskTraceNotificationHistoryLoad(u.Username)
	if err != nil {
		return nil, err
	}
	if next != nil {
		if next.MaxEntries < 1 || next.MaxEntries > 10000 {
			return nil, errors.New("history limit must be between 1 and 10000")
		}
		state.Settings = *next
		taskTraceNotificationHistoryTrim(&state)
		if err := taskTraceTeamWriteJSON(taskTraceNotificationHistoryPath(u.Username), state); err != nil {
			return nil, err
		}
	}
	return &state.Settings, nil
}

func taskTraceNotificationHistoryTrim(state *taskTraceNotificationHistoryState) {
	sort.SliceStable(state.Entries, func(i, j int) bool {
		one, two := state.Entries[i], state.Entries[j]
		if !one.ArchivedAt.Equal(two.ArchivedAt) {
			return one.ArchivedAt.After(two.ArchivedAt)
		}
		created := func(entry taskTraceNotificationHistoryRecord) time.Time {
			if entry.Team != nil {
				return entry.Team.Created
			}
			if entry.Notification != nil {
				return entry.Notification.Created
			}
			return time.Time{}
		}
		if !created(one).Equal(created(two)) {
			return created(one).After(created(two))
		}
		return one.ID > two.ID
	})
	if len(state.Entries) > state.Settings.MaxEntries {
		state.Entries = state.Entries[:state.Settings.MaxEntries]
	}
}

func taskTraceArchiveNotifications(username string, entries []taskTraceNotificationHistoryRecord) error {
	if len(entries) == 0 {
		return nil
	}
	taskTraceNotificationHistoryMu.Lock()
	defer taskTraceNotificationHistoryMu.Unlock()
	state, err := taskTraceNotificationHistoryLoad(username)
	if err != nil {
		return err
	}
	known := map[string]bool{}
	for _, entry := range state.Entries {
		known[entry.ID] = true
	}
	now := time.Now().UTC()
	for _, entry := range entries {
		if entry.Team != nil && state.Dismissed[entry.ID] {
			continue
		}
		if known[entry.ID] {
			continue
		}
		entry.ArchivedAt = now
		if entry.Team != nil {
			entry.Team.Read = true
			state.Dismissed[entry.ID] = true
		}
		if entry.Notification != nil && entry.Notification.ReadAt.IsZero() {
			entry.Notification.ReadAt = now
		}
		state.Entries = append(state.Entries, entry)
		known[entry.ID] = true
	}
	taskTraceNotificationHistoryTrim(&state)
	return taskTraceTeamWriteJSON(taskTraceNotificationHistoryPath(username), state)
}

func taskTraceTeamNotificationHistoryKey(shareID, id string) string {
	return "team:" + shareID + ":" + id
}

func TaskTraceNotificationHistory(s *xorm.Session, a web.Auth) ([]TaskTraceNotificationHistoryEntry, error) {
	u, err := taskTraceNotificationHistoryUser(a)
	if err != nil {
		return nil, err
	}
	taskTraceNotificationHistoryMu.Lock()
	state, err := taskTraceNotificationHistoryLoad(u.Username)
	taskTraceNotificationHistoryMu.Unlock()
	if err != nil {
		return nil, err
	}
	// Never hold the history lock while acquiring the collaboration lock: clearing
	// a team notification already holds the collaboration lock before archiving.
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	team, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	result := make([]TaskTraceNotificationHistoryEntry, 0, len(state.Entries))
	for _, record := range state.Entries {
		entry := record.TaskTraceNotificationHistoryEntry
		if entry.Notification != nil {
			entry.Notification.ProjectID = record.ProjectID
			can, permissionErr := CanReadNotification(s, a, entry.Notification)
			if permissionErr != nil {
				return nil, permissionErr
			}
			if !can {
				continue
			}
		}
		if entry.Team != nil {
			if binding := taskTraceTeamFindBinding(&team, entry.Team.ShareID); binding != nil {
				taskTraceTeamResolveNotificationTarget(s, binding, entry.Team)
			}
			entry.Team.Read = true
		}
		result = append(result, entry)
	}
	return result, nil
}

func taskTraceArchiveDatabaseNotifications(s *xorm.Session, a web.Auth, id int64) error {
	if taskTraceTeamStatePath() == "" {
		return nil
	} // Non-portable deployments retain their existing delete behavior.
	u, err := taskTraceNotificationHistoryUser(a)
	if err != nil {
		return err
	}
	rows := []*notifications.DatabaseNotification{}
	query := s.Where("notifiable_id = ?", a.GetID())
	if id != 0 {
		query = query.And("id = ?", id)
	}
	if err := query.Find(&rows); err != nil {
		return err
	}
	records := make([]taskTraceNotificationHistoryRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, taskTraceNotificationHistoryRecord{TaskTraceNotificationHistoryEntry: TaskTraceNotificationHistoryEntry{ID: fmt.Sprintf("notification:%d", row.ID), Notification: row}, ProjectID: row.ProjectID})
	}
	return taskTraceArchiveNotifications(u.Username, records)
}
