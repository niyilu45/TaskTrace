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
	"context"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamOutstandingStateRoundTrip(t *testing.T) {
	body := `<h3 data-tasktrace-comment-type="outstanding">list</h3><ul><li data-id="one" data-priority="2" data-done="true" data-completed-at="2026-10-10T08:00:00Z" data-reminder="2026-10-11T08:00:00Z"><p>Saved content</p><aside data-tasktrace-outstanding-note="true" hidden><p>Note</p><img src="/api/v2/tasks/1/attachments/4"></aside></li></ul>`
	for range 3 {
		items, order := taskTraceTeamOutstandingItems(body)
		rebuilt := taskTraceTeamOutstandingHTML(items, order)
		require.NotContains(t, rebuilt, "data-priority", "priority must remain personal")
		body = taskTraceTeamApplyOutstandingPriorities(rebuilt, map[string]string{"one": "2"})
		_, nodes := taskTraceTeamOutstandingNodes(body)
		require.Len(t, nodes, 1)
		require.Equal(t, "true", taskTraceAttribute(nodes[0], "data-done"))
		require.Equal(t, "2026-10-10T08:00:00Z", taskTraceAttribute(nodes[0], "data-completed-at"))
		require.Equal(t, "2026-10-11T08:00:00Z", taskTraceAttribute(nodes[0], "data-reminder"))
		require.Equal(t, "2", taskTraceAttribute(nodes[0], "data-priority"))
		require.Contains(t, body, "attachments/4")
		require.Contains(t, body, "Note")
	}
}

func TestTaskTraceTeamOutstandingEditsSurviveSync(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	actor := &user.User{
		ID:       1,
		Username: "user1",
	}
	session := db.NewSession()
	root := &Task{
		Title:     "Parent",
		ProjectID: 1,
		Priority:  7,
	}
	child := &Task{
		Title:     "Child",
		ProjectID: 1,
		Priority:  7,
	}
	require.NoError(t, root.Create(session, actor))
	require.NoError(t, child.Create(session, actor))
	require.NoError(t, (&TaskRelation{
		TaskID:       root.ID,
		OtherTaskID:  child.ID,
		RelationKind: RelationKindSubtask,
	}).Create(session, actor))
	body := `<h3 data-tasktrace-comment-type="outstanding">list</h3><ul><li data-id="one" data-priority="7" data-done="false">Original</li></ul>`
	comment := &TaskComment{
		TaskID:  child.ID,
		Comment: body,
	}
	require.NoError(t, comment.Create(session, actor))
	require.NoError(t, session.Commit())
	require.NoError(t, session.Close())
	base := map[string]TaskTraceTeamBase{
		"root": {
			Title:  root.Title,
			Status: TaskStatusTodo,
		},
		"child": {
			Title:       child.Title,
			Status:      TaskStatusTodo,
			Outstanding: body,
		},
	}
	binding := TaskTraceTeamBinding{
		Repository: repository,
		ShareID:    "states",
		Secret:     "test",
		Owner:      "user1",
		Members:    []string{"user1", "user2"},
		RootTaskID: root.ID,
		ProjectID:  1,
		NodeTasks: map[string]int64{
			"root":  root.ID,
			"child": child.ID,
		},
		Base: base,
	}
	permissions := []TaskTraceTeamMemberPermission{
		{
			Username: "user1",
			Owner:    true,
			Read:     true,
			Write:    true,
		},
		{
			Username: "user2",
			Read:     true,
			Write:    true,
		},
	}
	manifest := TaskTraceTeamManifest{
		Schema:    taskTraceTeamSchema,
		ShareID:   binding.ShareID,
		RootNode:  "root",
		Owner:     "user1",
		Members:   binding.Members,
		TokenHash: taskTraceTeamTokenHash(binding.Secret),
		Base:      base,
		Permissions: map[string][]TaskTraceTeamMemberPermission{
			"root":  permissions,
			"child": permissions,
		},
	}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), &manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{
		Schema:   taskTraceTeamSchema,
		DeviceID: "local",
		Bindings: []TaskTraceTeamBinding{binding},
	}))
	for _, priority := range []string{"2", "0", "8"} {
		edit := db.NewSession()
		list, err := taskTraceReadOutstanding(edit, child.ID)
		require.NoError(t, err)
		list.comment.Comment = taskTraceTeamApplyOutstandingPriorities(list.original, map[string]string{"one": priority})
		require.NoError(t, list.comment.Update(edit, actor))
		require.NoError(t, edit.Commit())
		require.NoError(t, edit.Close())
		for range 3 {
			sync := db.NewSession()
			_, err = TaskTraceTeamSync(sync, actor)
			require.NoError(t, err)
			require.NoError(t, sync.Commit())
			saved, err := taskTraceReadOutstanding(sync, child.ID)
			require.NoError(t, err)
			require.Equal(t, priority, taskTraceTeamOutstandingPriorities(saved.original)["one"])
			require.Contains(t, saved.original, "Original")
			require.NoError(t, sync.Close())
		}
	}
	// A remote delete must invalidate web views, without manufacturing edit records.
	session = db.NewSession()
	defer session.Close()
	events.ClearDispatchedEvents()
	require.NoError(t, taskTraceTeamUpsertOutstanding(session, actor, child.ID, ""))
	require.NoError(t, session.Commit())
	events.DispatchPending(context.Background(), session)
	require.Equal(t, 1, events.CountDispatchedEvents("tasktrace.task.changed"))
	require.NotContains(t, taskTraceTeamStripOutstandingPriorities(body), "data-priority")
}
