// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"path/filepath"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func taskTraceSharedParentForSubtask(t *testing.T, write bool) (*user.User, TaskTraceTeamBinding, TaskTraceTeamManifest, string) {
	t.Helper()
	db.LoadAndAssertFixtures(t)
	repository, data := t.TempDir(), t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	t.Setenv("TASKTRACE_DATA_ROOT", data)
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "subtask", Owner: "user2", Members: []string{"user1", "user2"}, RootTaskID: 1, ProjectID: 1, NodeTasks: map[string]int64{"root": 1}}
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding}}))
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: binding.Owner, Members: binding.Members, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {
		{Username: "user2", Owner: true, Read: true, Write: true}, {Username: "user1", Read: true, Write: write},
	}}}
	path := filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json")
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	return &user.User{ID: 1, Username: "user1"}, binding, manifest, path
}

func TestTaskTraceAtomicSubtaskCreation(t *testing.T) {
	t.Run("writable receiving member creates both relations", func(t *testing.T) {
		actor, _, _, _ := taskTraceSharedParentForSubtask(t, true)
		s := db.NewSession()
		defer s.Close()
		child := &Task{Title: "Member child", ProjectID: 1, ParentTaskID: 1}
		allowed, err := child.CanCreate(s, actor)
		require.NoError(t, err)
		require.True(t, allowed)
		require.NoError(t, child.Create(s, actor))
		require.NoError(t, s.Commit())
		for _, relation := range []TaskRelation{{TaskID: 1, OtherTaskID: child.ID, RelationKind: RelationKindSubtask}, {TaskID: child.ID, OtherTaskID: 1, RelationKind: RelationKindParenttask}} {
			count, err := s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", relation.TaskID, relation.OtherTaskID, relation.RelationKind).Count(&TaskRelation{})
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
		}
	})
	t.Run("read-only receiving member creates no orphan", func(t *testing.T) {
		actor, _, _, _ := taskTraceSharedParentForSubtask(t, false)
		s := db.NewSession()
		defer s.Close()
		child := &Task{Title: "Forbidden child", ProjectID: 1, ParentTaskID: 1}
		allowed, err := child.CanCreate(s, actor)
		require.NoError(t, err)
		require.False(t, allowed)
		require.ErrorAs(t, child.Create(s, actor), &ErrGenericForbidden{})
		count, err := s.Where("title = ?", child.Title).Count(&Task{})
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("missing or foreign project parent creates no orphan", func(t *testing.T) {
		actor, _, _, _ := taskTraceSharedParentForSubtask(t, true)
		for _, parent := range []int64{999999, 14} {
			s := db.NewSession()
			child := &Task{Title: "Invalid parent", ProjectID: 1, ParentTaskID: parent}
			allowed, err := child.CanCreate(s, actor)
			require.Error(t, err)
			require.False(t, allowed)
			require.Error(t, child.Create(s, actor))
			count, err := s.Where("title = ?", child.Title).Count(&Task{})
			require.NoError(t, err)
			require.Zero(t, count)
			require.NoError(t, s.Close())
		}
	})
	t.Run("late relation failure rolls back task and relation rows", func(t *testing.T) {
		actor, _, _, _ := taskTraceSharedParentForSubtask(t, true)
		if !strings.Contains(db.GetDialect(), "sqlite") {
			t.Skip("fault injection uses a transaction-local SQLite trigger")
		}
		s := db.NewSession()
		defer s.Close()
		_, err := s.Exec("CREATE TEMP TRIGGER fail_subtask_relation BEFORE INSERT ON task_relations BEGIN SELECT RAISE(ABORT, 'injected relation failure'); END")
		require.NoError(t, err)
		child := &Task{Title: "Atomic rollback child", ProjectID: 1, ParentTaskID: 1}
		require.ErrorContains(t, child.Create(s, actor), "injected relation failure")
		require.NoError(t, s.Rollback())
		verify := db.NewSession()
		defer verify.Close()
		count, err := verify.Where("title = ?", child.Title).Count(&Task{})
		require.NoError(t, err)
		require.Zero(t, count)
	})
	t.Run("bulk task creation honors parents and rejects the whole invalid batch", func(t *testing.T) {
		actor, _, _, _ := taskTraceSharedParentForSubtask(t, true)
		s := db.NewSession()
		defer s.Close()
		batch := &BulkTaskCreation{ProjectID: 1, Tasks: []*Task{{Title: "Bulk child", ParentTaskID: 1}}}
		require.NoError(t, batch.Create(s, actor))
		require.NoError(t, s.Commit())
		count, err := s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", 1, batch.Tasks[0].ID, RelationKindSubtask).Count(&TaskRelation{})
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
		invalid := &BulkTaskCreation{ProjectID: 1, Tasks: []*Task{{Title: "Valid but rolled back", ParentTaskID: 1}, {Title: "Invalid batch child", ParentTaskID: 999999}}}
		require.Error(t, invalid.Create(s, actor))
		require.NoError(t, s.Rollback())
		verify := db.NewSession()
		defer verify.Close()
		count, err = verify.Where("title = ?", "Valid but rolled back").Count(&Task{})
		require.NoError(t, err)
		require.Zero(t, count)
	})
}

func TestTaskTraceUnpublishedSubtaskInheritsLivePermission(t *testing.T) {
	actor, binding, manifest, path := taskTraceSharedParentForSubtask(t, true)
	s := db.NewSession()
	defer s.Close()
	child := &Task{Title: "Unpublished child", ProjectID: 1, ParentTaskID: 1}
	require.NoError(t, child.Create(s, actor))
	require.NoError(t, s.Commit())
	binding.NodeTasks["new-child"] = child.ID
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding}}))
	allowed, err := child.CanUpdate(s, actor)
	require.NoError(t, err)
	require.True(t, allowed, "a newly mapped descendant inherits its actual shared parent's write permission")
	manifest.Permissions["new-child"] = []TaskTraceTeamMemberPermission{{Username: actor.Username, Read: true}}
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	allowed, err = child.CanUpdate(s, actor)
	require.NoError(t, err)
	require.False(t, allowed, "an explicit read-only child is never bypassed")
	manifest.Permissions["new-child"][0].Read = false
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	allowed, err = child.CanUpdate(s, actor)
	require.NoError(t, err)
	require.False(t, allowed, "an explicit no-access child is never bypassed")
	delete(manifest.Permissions, "new-child")
	manifest.Permissions["root"][1].Write = false
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	allowed, err = child.CanUpdate(s, actor)
	require.NoError(t, err)
	require.False(t, allowed, "revoked parent permissions apply to an unpublished child")
}
