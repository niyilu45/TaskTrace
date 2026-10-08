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
	"path/filepath"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamReadOnlySnapshotsCannotDeleteOutstanding(t *testing.T) {
	old := TaskTraceTeamBase{Title: "Before", Status: TaskStatusTodo}
	accepted := TaskTraceTeamBase{Title: "After", Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"new": "Saved in browser"}, []string{"new"})}
	manifest := TaskTraceTeamManifest{Owner: "owner", Members: []string{"owner", "reader"}, Base: map[string]TaskTraceTeamBase{"node": accepted}}
	for _, tc := range []struct {
		name          string
		local, reader TaskTraceTeamBase
	}{
		{"upgraded computer has stale local base", old, accepted},
		{"read-only computer has stale base", accepted, old},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := TaskTraceTeamBinding{Base: map[string]TaskTraceTeamBase{"node": tc.local}}
			original := []TaskTraceTeamSnapshot{
				{Actor: "owner", Base: map[string]TaskTraceTeamBase{"node": accepted}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: accepted.Title, Outstanding: accepted.Outstanding}}},
				{Actor: "reader", Base: map[string]TaskTraceTeamBase{"node": tc.reader}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: tc.reader.Title, Outstanding: tc.reader.Outstanding}}},
			}
			filtered := taskTraceTeamFilterSnapshotsPermissions(original, &binding, &manifest)
			for _, field := range []string{"outstanding:new", "title"} {
				base := taskTraceTeamBaseValue(accepted, field)
				value, options, conflict := taskTraceTeamFindField(filtered, "node", field, base, "")
				require.False(t, conflict, "read-only snapshots must not introduce edits")
				require.Empty(t, options)
				require.Equal(t, base, value, "a permission filter must not turn a missing stale item into an intentional deletion")
			}
			require.Equal(t, tc.reader, original[1].Base["node"], "filtering must not mutate the original acknowledged base")
		})
	}
}

func TestTaskTraceTeamUpgradeSyncPreservesBrowserItems(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	defer s.Close()
	task := &Task{Title: "Shared upgrade", ProjectID: 1, Priority: 7}
	require.NoError(t, task.Create(s, actor))
	old := TaskTraceTeamBase{Title: task.Title, Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"old": "Keep"}, nil)}
	accepted := old
	accepted.Outstanding = taskTraceTeamOutstandingHTML(map[string]string{"old": "Keep", "new": `<p>Saved in browser</p><aside data-tasktrace-outstanding-note="true" hidden>Keep this note</aside>`}, nil)
	comment := &TaskComment{TaskID: task.ID, Comment: accepted.Outstanding}
	require.NoError(t, comment.Create(s, actor))
	require.NoError(t, s.Commit())
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "upgrade-test", Secret: "test-only-secret", Owner: actor.Username, Members: []string{"user1", "user2"}, RootTaskID: task.ID, ProjectID: 1, NodeTasks: map[string]int64{"node": task.ID}, Base: map[string]TaskTraceTeamBase{"node": old}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "node", Owner: actor.Username, Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: map[string]TaskTraceTeamBase{"node": accepted}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), &manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "owner-device", Bindings: []TaskTraceTeamBinding{binding}}))
	reader := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "reader-device", Updated: time.Now().UTC(), Base: map[string]TaskTraceTeamBase{"node": accepted}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: task.Title, Status: TaskStatusTodo, Outstanding: accepted.Outstanding}}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, reader))
	for cycle := 0; cycle < 3; cycle++ {
		// Every pass reopens both the DB session and persisted team state, as
		// happens after installing new binaries and restarting the application.
		func() {
			syncSession := db.NewSession()
			defer syncSession.Close()
			_, err := TaskTraceTeamSync(syncSession, actor)
			require.NoError(t, err)
			require.NoError(t, syncSession.Commit())
		}()
		func() {
			read := db.NewReadSession()
			defer read.Close()
			list, err := taskTraceReadOutstanding(read, task.ID)
			require.NoError(t, err)
			require.Len(t, list.items, 2)
			require.Contains(t, list.original, "Saved in browser")
			require.Contains(t, list.original, "Keep this note")
			state, err := taskTraceTeamLoadState()
			require.NoError(t, err)
			require.Empty(t, state.Bindings[0].LastError)
			require.Empty(t, state.Bindings[0].Conflicts)
			var rows []*TaskComment
			require.NoError(t, read.Where("task_id = ?", task.ID).Find(&rows))
			count := 0
			for _, row := range rows {
				if taskTraceTeamIsOutstanding(row.Comment) {
					count++
				}
			}
			require.Equal(t, 1, count, "repeated sync must not create duplicate lists")
		}()
	}
}

func TestTaskTraceTeamPermissionFilterPreservesIndependentOutstandingWrite(t *testing.T) {
	base := TaskTraceTeamBase{Title: "Trusted", Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"editable": "Before", "protected": "Keep"}, nil)}
	manifest := TaskTraceTeamManifest{Owner: "owner", Members: []string{"owner", "reader"}, Base: map[string]TaskTraceTeamBase{"node": base}, Permissions: map[string][]TaskTraceTeamMemberPermission{
		"node":                      {{Username: "owner", Read: true, Write: true}, {Username: "reader", Read: true}},
		"node/outstanding/editable": {{Username: "owner", Read: true, Write: true}, {Username: "reader", Read: true, Write: true}},
	}}
	snapshot := TaskTraceTeamSnapshot{Actor: "reader", Base: map[string]TaskTraceTeamBase{"node": base}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "Unauthorized", Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"editable": "After"}, nil)}}}
	filtered := taskTraceTeamFilterSnapshotsPermissions([]TaskTraceTeamSnapshot{snapshot}, &TaskTraceTeamBinding{}, &manifest)
	for field, expected := range map[string]string{"title": "Trusted", "outstanding:editable": "After", "outstanding:protected": "Keep"} {
		value, _, conflict := taskTraceTeamFindField(filtered, "node", field, taskTraceTeamBaseValue(base, field), "")
		require.False(t, conflict)
		require.Equal(t, expected, value)
	}
}

func TestTaskTraceTeamSnapshotKeepsCanonicalListAfterReopen(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	defer s.Close()
	task := &Task{Title: "Upgrade preservation", ProjectID: 1, Priority: 7}
	require.NoError(t, task.Create(s, actor))
	// Imported historical comments can have a later timestamp than the newest
	// canonical list. Web and floating clients both select by numeric comment ID.
	old := &TaskComment{TaskID: task.ID, AuthorID: 1, Comment: taskTraceTeamOutstandingHTML(map[string]string{"old": "Old"}, nil), Created: time.Now().Add(time.Hour), Updated: time.Now().Add(time.Hour)}
	_, err := s.NoAutoTime().Insert(old)
	require.NoError(t, err)
	current := &TaskComment{TaskID: task.ID, Comment: taskTraceTeamOutstandingHTML(map[string]string{"old": "Old", "new": "Saved in browser"}, nil)}
	require.NoError(t, current.Create(s, actor))
	require.Greater(t, current.ID, old.ID)
	require.NoError(t, s.Commit())
	reopened := db.NewSession()
	defer reopened.Close()
	list, err := taskTraceReadOutstanding(reopened, task.ID)
	require.NoError(t, err)
	require.Len(t, list.items, 2)
	binding := &TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "share", RootTaskID: task.ID, NodeTasks: map[string]int64{"node": task.ID}}
	snapshot, err := taskTraceTeamBuildSnapshot(reopened, binding, actor.Username, "device")
	require.NoError(t, err)
	items, _ := taskTraceTeamOutstandingItems(taskTraceTeamTaskMap(snapshot)["node"].Outstanding)
	require.Equal(t, map[string]string{"old": "Old", "new": "Saved in browser"}, items)
}
