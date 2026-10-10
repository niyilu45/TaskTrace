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
	"fmt"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamUnprovenRemoteRemovalPreservesSavedItem(t *testing.T) {
	for _, sharedAlreadyEmpty := range []bool{false, true} {
		for _, keep := range []bool{false, true} {
			t.Run(fmt.Sprintf("sharedAlreadyEmpty=%t/keep=%t", sharedAlreadyEmpty, keep), func(t *testing.T) {
				db.LoadAndAssertFixtures(t)
				t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
				repository := t.TempDir()
				t.Setenv("TASKTRACE_TEAM_ROOT", repository)
				actor := &user.User{ID: 1, Username: "user1"}
				s := db.NewSession()
				t.Cleanup(func() { _ = s.Close() })
				task := &Task{Title: "Saved in browser", ProjectID: 1, Priority: 7}
				require.NoError(t, task.Create(s, actor))
				content := `<p>New outstanding</p><aside data-tasktrace-outstanding-note="true" hidden><p>Saved note</p><img src="data:image/png;base64,aGVsbG8="></aside>`
				body := taskTraceTeamOutstandingHTML(map[string]string{"new": content}, nil)
				comment := &TaskComment{TaskID: task.ID, Comment: body}
				can, err := comment.CanCreate(s, actor)
				require.NoError(t, err)
				require.True(t, can)
				require.NoError(t, comment.Create(s, actor))
				require.NoError(t, s.Commit())
				require.NoError(t, s.Close())
				savedBase := TaskTraceTeamBase{Title: task.Title, Status: TaskStatusTodo, Outstanding: body}
				binding := TaskTraceTeamBinding{Repository: repository, ShareID: "remote-empty", Secret: "secret", Owner: actor.Username, Members: []string{"user1", "user2"}, RootTaskID: task.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": task.ID}, Base: map[string]TaskTraceTeamBase{"root": savedBase}}
				sharedBase := savedBase
				if sharedAlreadyEmpty {
					sharedBase.Outstanding = ""
				}
				permissions := []TaskTraceTeamMemberPermission{{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}
				manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: actor.Username, Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: map[string]TaskTraceTeamBase{"root": sharedBase}, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions}}
				require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), manifest))
				require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
				// A peer knows this item in its base but publishes an incomplete list.
				// It has no user deletion or move operation. Some older builds also
				// advanced the shared manifest before this device got the next poll.
				remote := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "other-pc", Base: binding.Base, Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: task.Title, Status: TaskStatusTodo}}}
				require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
				rewritePeer := func() {
					t.Helper()
					require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
					if sharedAlreadyEmpty {
						path := filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json")
						var legacyManifest TaskTraceTeamManifest
						require.NoError(t, taskTraceTeamReadJSON(path, &legacyManifest))
						empty := legacyManifest.Base["root"]
						empty.Outstanding = ""
						legacyManifest.Base["root"] = empty
						legacyManifest.AcceptedFields = nil // Older schemas discard unknown manifest fields.
						require.NoError(t, taskTraceTeamWriteJSON(path, legacyManifest))
					}
				}
				var lastConflict TaskTraceTeamConflict
				for cycle := 0; cycle < 3; cycle++ {
					rewritePeer()
					sync := db.NewSession()
					t.Cleanup(func() { _ = sync.Close() })
					_, err = TaskTraceTeamSync(sync, actor)
					require.NoError(t, err)
					require.NoError(t, sync.Commit())
					list, readErr := taskTraceReadOutstanding(sync, task.ID)
					require.NoError(t, readErr)
					require.Contains(t, list.original, "New outstanding", "a peer's missing list must not erase the item saved on this device")
					require.Contains(t, list.original, "Saved note")
					require.Contains(t, list.original, "data:image/png")
					state, stateErr := taskTraceTeamLoadState()
					require.NoError(t, stateErr)
					require.Empty(t, state.Bindings[0].LastError)
					require.Len(t, state.Bindings[0].Conflicts, 1)
					lastConflict = state.Bindings[0].Conflicts[0]
					count, countErr := sync.Where("task_id = ? AND comment LIKE ?", task.ID, "%item-activity%").Count(&TaskComment{})
					require.NoError(t, countErr)
					require.EqualValues(t, 1, count, "passive sync must not manufacture deletion or duplicate add activity")
					require.NoError(t, sync.Close())
				}
				value := ""
				if keep {
					value = taskTraceTeamCanonicalHTML(content, nil)
				}
				require.Equal(t, "outstanding:new", lastConflict.Field)
				resolve := db.NewSession()
				t.Cleanup(func() { _ = resolve.Close() })
				_, err = TaskTraceTeamResolve(resolve, actor, TaskTraceTeamResolveRequest{ShareID: binding.ShareID, Resolutions: []TaskTraceTeamResolution{{ConflictID: lastConflict.ID, Value: value}}})
				require.NoError(t, err)
				require.NoError(t, resolve.Commit())
				require.NoError(t, resolve.Close())
				for range 2 {
					rewritePeer()
					sync := db.NewSession()
					t.Cleanup(func() { _ = sync.Close() })
					_, err = TaskTraceTeamSync(sync, actor)
					require.NoError(t, err)
					require.NoError(t, sync.Commit())
					list, readErr := taskTraceReadOutstanding(sync, task.ID)
					require.NoError(t, readErr)
					if keep {
						require.Contains(t, list.original, "New outstanding")
					} else {
						require.Empty(t, list.original)
					}
					state, stateErr := taskTraceTeamLoadState()
					require.NoError(t, stateErr)
					require.Empty(t, state.Bindings[0].LastError)
					require.Empty(t, state.Bindings[0].Conflicts, "a confirmed resolution must not prompt again on each poll")
					require.NoError(t, sync.Close())
				}
			})
		}
	}
}

//nolint:gosmopolitan // Compatibility with persisted activity labels from older builds.
func TestTaskTraceTeamOutstandingRemovalEvidence(t *testing.T) {
	body := taskTraceTeamOutstandingHTML(map[string]string{"one": "Saved item"}, nil)
	deletion := `<p data-tasktrace-comment-type="item-activity" data-item-id="one"><strong>删除遗留事项</strong>：Saved item</p>`
	explicit := `<p data-tasktrace-comment-type="item-activity" data-item-id="one" data-tasktrace-item-action="delete">Deleted</p>`
	for _, tc := range []struct {
		name, body, actor, required, ack, source, destination string
		removed, taskMissing, denyDestination, expected       bool
	}{
		{name: "missing without operation", actor: "writer"},
		{name: "legacy deletion", actor: "writer", body: deletion, expected: true},
		{name: "explicit deletion", actor: "writer", body: explicit, expected: true},
		{name: "revoked writer", actor: "reader", body: deletion},
		{name: "different item", actor: "writer", body: `<p data-tasktrace-comment-type="item-activity" data-item-id="two"><strong>删除遗留事项</strong>：Saved item</p>`},
		{name: "ordinary comment", actor: "writer", body: `<p data-item-id="one"><strong>删除遗留事项</strong></p>`},
		{name: "mention inside add label", actor: "writer", body: `<p data-tasktrace-comment-type="item-activity" data-item-id="one"><strong>新增遗留事项</strong>：删除遗留事项</p>`},
		{name: "undone deletion activity", actor: "writer", body: deletion, removed: true},
		{name: "restored item", actor: "writer", body: deletion, source: body},
		{name: "old resolution", actor: "writer", body: deletion, required: "latest"},
		{name: "acknowledged resolution", actor: "writer", body: deletion, required: "latest", ack: "latest", expected: true},
		{name: "move", actor: "writer", destination: body, expected: true},
		{name: "copy is not move", actor: "writer", source: body, destination: body},
		{name: "missing task is not move", actor: "writer", taskMissing: true, destination: body},
		{name: "cannot write destination", actor: "writer", destination: body, denyDestination: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			permissions := []TaskTraceTeamMemberPermission{{Username: "writer", Read: true, Write: true}, {Username: "reader", Read: true}}
			manifest := &TaskTraceTeamManifest{Owner: "owner", Members: []string{"owner", "writer", "reader"}, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions, "destination": permissions}}
			if tc.denyDestination {
				manifest.Permissions["destination"] = []TaskTraceTeamMemberPermission{{Username: "writer", Read: true}}
			}
			tasks := []TaskTraceTeamTask{{NodeID: "destination", Outstanding: tc.destination}}
			if !tc.taskMissing {
				tasks = append(tasks, TaskTraceTeamTask{NodeID: "root", Outstanding: tc.source, Comments: []TaskTraceTeamComment{{Body: tc.body, Deleted: tc.removed}}})
			}
			snapshot := TaskTraceTeamSnapshot{Actor: tc.actor, Tasks: tasks, ResolutionAcks: map[string]string{"root:outstanding:one": tc.ack}}
			require.Equal(t, tc.expected, taskTraceTeamHasOutstandingRemoval([]TaskTraceTeamSnapshot{snapshot}, manifest, "root", "one", tc.required))
		})
	}
}

//nolint:gosmopolitan // Compatibility with older clients' explicit deletion records.
func TestTaskTraceTeamExplicitRemoteRemovalStillSyncs(t *testing.T) {
	for _, operation := range []string{"delete", "legacy-delete", "move"} {
		t.Run(operation, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
			repository := t.TempDir()
			t.Setenv("TASKTRACE_TEAM_ROOT", repository)
			actor := &user.User{ID: 1, Username: "user1"}
			s := db.NewSession()
			t.Cleanup(func() { _ = s.Close() })
			source := &Task{Title: "Source", ProjectID: 1, Priority: 7}
			destination := &Task{Title: "Destination", ProjectID: 1, Priority: 7}
			require.NoError(t, source.Create(s, actor))
			require.NoError(t, destination.Create(s, actor))
			body := taskTraceTeamOutstandingHTML(map[string]string{"one": "Saved item"}, nil)
			require.NoError(t, taskTraceTeamUpsertOutstanding(s, actor, source.ID, body))
			require.NoError(t, s.Commit())
			require.NoError(t, s.Close())
			base := map[string]TaskTraceTeamBase{"root": {Title: source.Title, Status: TaskStatusTodo, Outstanding: body}, "destination": {Title: destination.Title, Status: TaskStatusTodo}}
			binding := TaskTraceTeamBinding{Repository: repository, ShareID: "explicit-removal", Secret: "secret", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: source.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": source.ID, "destination": destination.ID}, Base: base}
			permissions := []TaskTraceTeamMemberPermission{{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}
			manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: base, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions, "destination": permissions}}
			require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), manifest))
			require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
			remote := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "other-pc", Base: base, Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: source.Title, Status: TaskStatusTodo}, {NodeID: "destination", Title: destination.Title, Status: TaskStatusTodo}}}
			switch operation {
			case "move":
				remote.Tasks[1].Outstanding = body
			case "delete":
				// Generate the proof through the same helper as an authorized API save.
				history := db.NewSession()
				t.Cleanup(func() { _ = history.Close() })
				require.NoError(t, taskTraceRecordOutstandingActivity(history, &user.User{ID: 2, Username: "user2"}, source.ID, taskTraceOutstandingActivityItems(body), ""))
				var rows []*TaskComment
				require.NoError(t, history.Where("task_id = ? AND comment LIKE ?", source.ID, "%item-activity%").Find(&rows))
				require.Len(t, rows, 1)
				require.Contains(t, rows[0].Comment, `data-tasktrace-item-action="delete"`)
				remote.Tasks[0].Comments = []TaskTraceTeamComment{{ID: "delete-event", Body: rows[0].Comment, Author: "user2"}}
				require.NoError(t, history.Rollback())
				require.NoError(t, history.Close())
			case "legacy-delete":
				remote.Tasks[0].Comments = []TaskTraceTeamComment{{ID: "delete-event", Body: `<p data-tasktrace-comment-type="item-activity" data-item-id="one"><strong>删除遗留事项</strong>：Saved item</p>`, Author: "user2"}}
			}
			for range 3 {
				require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
				sync := db.NewSession()
				t.Cleanup(func() { _ = sync.Close() })
				_, err := TaskTraceTeamSync(sync, actor)
				require.NoError(t, err)
				require.NoError(t, sync.Commit())
				list, err := taskTraceReadOutstanding(sync, source.ID)
				require.NoError(t, err)
				require.Empty(t, list.original)
				if operation == "move" {
					moved, readErr := taskTraceReadOutstanding(sync, destination.ID)
					require.NoError(t, readErr)
					require.Contains(t, moved.original, "Saved item")
				}
				state, err := taskTraceTeamLoadState()
				require.NoError(t, err)
				require.Empty(t, state.Bindings[0].LastError)
				require.Empty(t, state.Bindings[0].Conflicts)
				require.NoError(t, sync.Close())
			}
		})
	}
}
