// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"path/filepath"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamRestoredListSurvivesPendingResolution(t *testing.T) {
	for _, resolved := range []string{"Resolved old item", ""} {
		t.Run("resolution="+resolved, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
			repository := t.TempDir()
			t.Setenv("TASKTRACE_TEAM_ROOT", repository)
			actor := &user.User{ID: 1, Username: "user1"}
			s := db.NewSession()
			root := &Task{Title: "Restored shared task", ProjectID: 1, Priority: 7}
			require.NoError(t, root.Create(s, actor))
			base := TaskTraceTeamBase{Title: root.Title, Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"old": "Old item"}, nil)}
			added := `<p>Saved before backup</p><aside data-tasktrace-outstanding-note="true" hidden><p>Saved note</p><img src="data:image/png;base64,aGVsbG8="></aside>`
			body := taskTraceTeamOutstandingHTML(map[string]string{"old": "Old item", "new": added}, []string{"old", "new"})
			require.NoError(t, (&TaskComment{TaskID: root.ID, Comment: body}).Create(s, actor))
			require.NoError(t, s.Commit())
			require.NoError(t, s.Close())
			binding := TaskTraceTeamBinding{Repository: repository, ShareID: "restore-resolution", Secret: "secret", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: root.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": root.ID}, Base: map[string]TaskTraceTeamBase{"root": base}}
			manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: binding.Base, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}}}
			require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), manifest))
			require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "restored", Bindings: []TaskTraceTeamBinding{binding}}))
			// The restored database has a newly saved item, while its sync baseline
			// still predates it. Another member resolved only the older item.
			record := taskTraceTeamResolutionRecord{ID: "resolution", NodeID: "root", Field: "outstanding:old", Value: resolved, Updated: time.Now().UTC()}
			require.NoError(t, taskTraceTeamWriteResolutions(&binding, map[string]taskTraceTeamResolutionRecord{"root:outstanding:old": record}))
			for range 3 {
				sync := db.NewSession()
				_, err := TaskTraceTeamSync(sync, actor)
				require.NoError(t, err)
				require.NoError(t, sync.Commit())
				list, err := taskTraceReadOutstanding(sync, root.ID)
				require.NoError(t, err)
				items, _ := taskTraceTeamOutstandingItems(list.original)
				require.Contains(t, items, "new", "resolving one old item must not delete a separately saved item")
				require.Contains(t, items["new"], "Saved note")
				require.Contains(t, items["new"], "data:image/png")
				require.Equal(t, resolved, items["old"])
				require.NoError(t, sync.Close())
			}
		})
	}
}

func TestTaskTraceTeamLegacyCommentsCannotReplaceOutstandingList(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	defer s.Close()
	root := &Task{Title: "Shared task", ProjectID: 1}
	require.NoError(t, root.Create(s, actor))
	body := taskTraceTeamOutstandingHTML(map[string]string{"new": "Saved item and note"}, nil)
	comment := &TaskComment{TaskID: root.ID, Comment: taskTraceTeamAddMarker(body, "list", "user1")}
	require.NoError(t, comment.Create(s, actor))
	binding := TaskTraceTeamBinding{ShareID: "legacy", NodeTasks: map[string]int64{"root": root.ID}}
	for _, stale := range []TaskTraceTeamComment{
		{ID: "old-list", Body: taskTraceTeamOutstandingHTML(map[string]string{"old": "Stale old list"}, nil), Author: "user2", Updated: time.Now().Add(time.Hour)},
		{ID: "list", Deleted: true, Updated: time.Now().Add(time.Hour)},
	} {
		require.NoError(t, taskTraceTeamMergeComments(s, actor, &binding, "root", "user1", root.ID, []TaskTraceTeamComment{stale}))
		list, err := taskTraceReadOutstanding(s, root.ID)
		require.NoError(t, err)
		require.Contains(t, list.original, "Saved item and note")
		var comments []*TaskComment
		require.NoError(t, s.Where("task_id = ?", root.ID).Find(&comments))
		require.Len(t, comments, 1, "ordinary comment replay must not create another canonical list")
	}
}

func TestTaskTraceTeamConflictingNewOutstandingRemainsUntilResolved(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	root := &Task{Title: "Shared conflict", ProjectID: 1}
	require.NoError(t, root.Create(s, actor))
	localBody := taskTraceTeamOutstandingHTML(map[string]string{"new": "My saved outstanding"}, nil)
	require.NoError(t, (&TaskComment{TaskID: root.ID, Comment: localBody}).Create(s, actor))
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	base := TaskTraceTeamBase{Title: root.Title, Status: TaskStatusTodo}
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "conflict", Secret: "secret", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: root.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": root.ID}, Base: map[string]TaskTraceTeamBase{"root": base}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: binding.Base, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
	remoteBody := taskTraceTeamOutstandingHTML(map[string]string{"new": "Other saved outstanding"}, nil)
	remote := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "remote", Base: binding.Base, Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: root.Title, Status: TaskStatusTodo, Outstanding: remoteBody}}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
	for cycle := range 4 {
		if cycle == 3 {
			record := taskTraceTeamResolutionRecord{ID: "resolved", NodeID: "root", Field: "outstanding:new", Value: "Other saved outstanding", Updated: time.Now().UTC()}
			require.NoError(t, taskTraceTeamWriteResolutions(&binding, map[string]taskTraceTeamResolutionRecord{"root:outstanding:new": record}))
		}
		sync := db.NewSession()
		_, err := TaskTraceTeamSync(sync, actor)
		require.NoError(t, err)
		require.NoError(t, sync.Commit())
		list, err := taskTraceReadOutstanding(sync, root.ID)
		require.NoError(t, err)
		expected := "My saved outstanding"
		if cycle == 3 {
			expected = "Other saved outstanding"
		}
		require.Contains(t, list.original, expected, "unresolved conflict must not erase the current saved item")
		state, err := taskTraceTeamLoadState()
		require.NoError(t, err)
		require.Empty(t, state.Bindings[0].LastError)
		if cycle < 3 {
			require.Len(t, state.Bindings[0].Conflicts, 1, "polling must not silently resolve the conflict by dropping one version")
		} else {
			require.Empty(t, state.Bindings[0].Conflicts)
		}
		require.NoError(t, sync.Close())
	}
}
