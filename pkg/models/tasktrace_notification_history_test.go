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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceNotificationHistoryRetention(t *testing.T) {
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	actor := &user.User{ID: 1, Username: "user1"}
	settings, err := TaskTraceNotificationSettings(actor, nil)
	require.NoError(t, err)
	require.Equal(t, 100, settings.MaxEntries)
	for _, limit := range []int{0, -1, 10001} {
		_, err = TaskTraceNotificationSettings(actor, &TaskTraceNotificationHistorySettings{MaxEntries: limit})
		require.Error(t, err)
	}
	_, err = TaskTraceNotificationSettings(&LinkSharing{ID: 1}, nil)
	require.Error(t, err)
	records := make([]taskTraceNotificationHistoryRecord, 105)
	for i := range records {
		notice := &TaskTraceTeamNotification{ID: fmt.Sprint(i), Created: time.Unix(int64(i), 0), TaskTitle: fmt.Sprint(i)}
		records[i] = taskTraceNotificationHistoryRecord{TaskTraceNotificationHistoryEntry: TaskTraceNotificationHistoryEntry{ID: taskTraceTeamNotificationHistoryKey("share", notice.ID), Team: notice}}
	}
	require.NoError(t, taskTraceArchiveNotifications(actor.Username, records))
	state, err := taskTraceNotificationHistoryLoad(actor.Username)
	require.NoError(t, err)
	require.Len(t, state.Entries, 100)
	require.Equal(t, "104", state.Entries[0].Team.TaskTitle)
	require.True(t, state.Entries[0].Team.Read)
	// Replay, even after eviction, must not duplicate history or revive the inbox.
	require.NoError(t, taskTraceArchiveNotifications(actor.Username, records[:1]))
	state, err = taskTraceNotificationHistoryLoad(actor.Username)
	require.NoError(t, err)
	require.Len(t, state.Entries, 100)
	require.Equal(t, "104", state.Entries[0].Team.TaskTitle)
	_, err = TaskTraceNotificationSettings(actor, &TaskTraceNotificationHistorySettings{MaxEntries: 2})
	require.NoError(t, err)
	state, err = taskTraceNotificationHistoryLoad(actor.Username)
	require.NoError(t, err)
	require.Len(t, state.Entries, 2)
	require.Equal(t, 2, state.Settings.MaxEntries)
	require.True(t, state.Dismissed[records[0].ID])
	other, err := taskTraceNotificationHistoryLoad("user2")
	require.NoError(t, err)
	require.Empty(t, other.Entries)
	require.Equal(t, 100, other.Settings.MaxEntries)
}

func TestTaskTraceNotificationHistoryConcurrentArchives(t *testing.T) {
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	var wg sync.WaitGroup
	results := make(chan error, 20)
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- taskTraceArchiveNotifications("user1", []taskTraceNotificationHistoryRecord{{TaskTraceNotificationHistoryEntry: TaskTraceNotificationHistoryEntry{ID: fmt.Sprint(i), Team: &TaskTraceTeamNotification{ID: fmt.Sprint(i)}}}})
		}()
	}
	wg.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	state, err := taskTraceNotificationHistoryLoad("user1")
	require.NoError(t, err)
	require.Len(t, state.Entries, 20)
}

func TestTaskTraceNotificationHistoryDatabaseClear(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	row := &notifications.DatabaseNotification{NotifiableID: actor.ID, Name: "test.history", Notification: json.RawMessage(`{"message":"saved"}`)}
	_, err := s.Insert(row)
	require.NoError(t, err)
	selected := &DatabaseNotifications{}
	selected.ID = row.ID
	can, err := selected.CanDelete(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, selected.Delete(s, actor))
	require.NoError(t, s.Commit())
	history, err := TaskTraceNotificationHistory(s, actor)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.Equal(t, row.ID, history[0].Notification.ID)
	require.False(t, history[0].Notification.ReadAt.IsZero())
	other, err := TaskTraceNotificationHistory(s, &user.User{ID: 2, Username: "user2"})
	require.NoError(t, err)
	require.Empty(t, other)
	_, err = TaskTraceNotificationHistory(s, &LinkSharing{ID: 1})
	require.Error(t, err)
	// Failed archive writes must not remove the source notification.
	root := t.TempDir()
	t.Setenv("TASKTRACE_DATA_ROOT", root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "notification-history"), []byte("blocked"), 0600))
	row.ID = 0
	_, err = s.Insert(row)
	require.NoError(t, err)
	selected.ID = row.ID
	require.Error(t, selected.Delete(s, actor))
	exists, err := s.ID(row.ID).Exist(&notifications.DatabaseNotification{})
	require.NoError(t, err)
	require.True(t, exists)
}

func TestTaskTraceNotificationHistoryTeamReplayAndFailure(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	root, repository := t.TempDir(), t.TempDir()
	t.Setenv("TASKTRACE_DATA_ROOT", root)
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "archive", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: 1, NodeTasks: map[string]int64{"root": 1}}
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding}}))
	id := uuid.NewString()
	path := filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "notifications", "user1", id+".json")
	notice := TaskTraceTeamNotification{ID: id, NodeID: "root", Actor: "user2", TaskTitle: "Update to keep"}
	require.NoError(t, taskTraceTeamWriteJSON(path, notice))
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	require.NoError(t, os.WriteFile(filepath.Join(root, "notification-history"), []byte("blocked"), 0600))
	_, err := TaskTraceTeamNotificationsDelete(s, actor, TaskTraceTeamNotificationsDeleteRequest{IDs: []string{id}})
	require.Error(t, err)
	require.FileExists(t, path)
	require.NoError(t, os.Remove(filepath.Join(root, "notification-history")))
	status, err := TaskTraceTeamNotificationsDelete(s, actor, TaskTraceTeamNotificationsDeleteRequest{IDs: []string{id}})
	require.NoError(t, err)
	require.Empty(t, status.Notifications)
	history, err := TaskTraceNotificationHistory(s, actor)
	require.NoError(t, err)
	require.Len(t, history, 1)
	require.EqualValues(t, 1, history[0].Team.TaskID)
	require.Equal(t, "Update to keep", history[0].Team.TaskTitle)
	require.NoError(t, taskTraceTeamWriteJSON(path, notice))
	require.Empty(t, taskTraceTeamNotifications(s, &binding, "user1"), "old-client replay must not revive cleared notifications")
}
