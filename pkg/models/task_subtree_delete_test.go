// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func taskTraceDeletionTree(t *testing.T) (*user.User, []int64, TaskTraceTeamManifest, string) {
	t.Helper()
	actor, binding, manifest, path := taskTraceSharedParentForSubtask(t, true)
	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)
	ids := []int64{}
	for index, title := range []string{"Deletion parent", "Deletion child", "Deletion grandchild", "Unrelated sibling"} {
		task := &Task{Title: title, ProjectID: 1}
		if index == 1 || index == 2 {
			task.ParentTaskID = ids[index-1]
		}
		require.NoError(t, task.Create(s, actor))
		ids = append(ids, task.ID)
	}
	binding.RootTaskID = ids[0]
	binding.NodeTasks = map[string]int64{"root": ids[0], "child": ids[1], "grandchild": ids[2]}
	manifest.Permissions["child"] = append([]TaskTraceTeamMemberPermission{}, manifest.Permissions["root"]...)
	manifest.Permissions["grandchild"] = append([]TaskTraceTeamMemberPermission{}, manifest.Permissions["root"]...)
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding}}))
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	comment := &TaskComment{TaskID: ids[2], Comment: taskTraceTeamOutstandingHTML(map[string]string{"retained": "Retained outstanding item"}, []string{"retained"})}
	require.NoError(t, comment.Create(s, actor))
	require.NoError(t, s.Commit())
	return actor, ids, manifest, path
}

func taskTraceDeleteSnapshot(t *testing.T, actor *user.User, id int64) string {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)
	task := &Task{ID: id}
	allowed, err := task.CanDelete(s, actor)
	require.NoError(t, err)
	require.True(t, allowed)
	before, supported, err := captureTaskTraceUndo(s, actor, "delete", task)
	require.NoError(t, err)
	require.True(t, supported)
	require.NoError(t, task.Delete(s, actor))
	payload, _, _, changed, err := finishTaskTraceUndo(s, actor, "delete", task, before)
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, s.Commit())
	return payload
}

func TestTaskTraceDeleteSubtreeAndUndo(t *testing.T) {
	actor, ids, _, _ := taskTraceDeletionTree(t)
	payload := taskTraceDeleteSnapshot(t, actor, ids[1])
	s := db.NewSession()
	for index, id := range ids {
		_, err := GetTaskByIDSimple(s, id)
		if index == 1 || index == 2 {
			require.ErrorAs(t, err, &ErrTaskDoesNotExist{})
		} else {
			require.NoError(t, err)
		}
	}
	list, err := taskTraceReadOutstanding(s, ids[2])
	require.NoError(t, err)
	require.Len(t, list.items, 1)
	require.NoError(t, s.Close())
	restore := db.NewSession()
	defer restore.Close()
	defer events.CleanupPending(restore)
	affected, err := restoreTaskTraceUndo(restore, actor, payload)
	require.NoError(t, err)
	require.ElementsMatch(t, []int64{ids[1], ids[2]}, affected)
	require.NoError(t, restore.Commit())
	for _, id := range ids {
		_, err := GetTaskByIDSimple(restore, id)
		require.NoError(t, err)
	}
}

func TestTaskTraceDeleteSubtreeChecksEveryPermission(t *testing.T) {
	actor, ids, manifest, path := taskTraceDeletionTree(t)
	manifest.Permissions["grandchild"][1].Write = false
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	s := db.NewSession()
	defer s.Close()
	task := &Task{ID: ids[1]}
	allowed, err := task.CanDelete(s, actor)
	require.ErrorAs(t, err, &ErrGenericForbidden{})
	require.False(t, allowed)
	require.ErrorAs(t, task.Delete(s, actor), &ErrGenericForbidden{})
	for _, id := range ids {
		_, err := GetTaskByIDSimple(s, id)
		require.NoError(t, err)
	}
}

func TestTaskTraceDeleteSubtreeUndoRejectsRevokedPermission(t *testing.T) {
	actor, ids, manifest, path := taskTraceDeletionTree(t)
	payload := taskTraceDeleteSnapshot(t, actor, ids[1])
	manifest.Permissions["grandchild"][1].Write = false
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	s := db.NewSession()
	defer s.Close()
	_, err := restoreTaskTraceUndo(s, actor, payload)
	require.ErrorAs(t, err, &ErrGenericForbidden{})
	for _, id := range ids[1:3] {
		_, err := GetTaskByIDSimple(s, id)
		require.ErrorAs(t, err, &ErrTaskDoesNotExist{})
	}
}

func TestTaskTraceDeleteSubtreeUndoGuardsLaterRelations(t *testing.T) {
	actor, ids, _, _ := taskTraceDeletionTree(t)
	payload := taskTraceDeleteSnapshot(t, actor, ids[1])
	s := db.NewSession()
	_, err := s.Insert(&TaskRelation{TaskID: ids[3], OtherTaskID: ids[2], RelationKind: RelationKindRelated, CreatedByID: actor.ID})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	restore := db.NewSession()
	defer restore.Close()
	_, err = restoreTaskTraceUndo(restore, actor, payload)
	require.ErrorAs(t, err, &ErrTaskTraceUndoConflict{})
}

func TestTaskTraceDeleteSubtreeRejectsCrossProjectLinks(t *testing.T) {
	actor, ids, _, _ := taskTraceDeletionTree(t)
	s := db.NewSession()
	defer s.Close()
	require.NoError(t, (&TaskRelation{TaskID: ids[2], OtherTaskID: 14, RelationKind: RelationKindSubtask}).Create(s, actor))
	require.NoError(t, s.Commit())
	require.ErrorAs(t, (&Task{ID: ids[1]}).Delete(s, actor), &ErrGenericForbidden{})
	for _, id := range append(ids, 14) {
		_, err := GetTaskByIDSimple(s, id)
		require.NoError(t, err)
	}
}

func TestTaskTraceDeleteSubtreeLeavesUpstreamSemanticsUnchanged(t *testing.T) {
	actor, ids, _, _ := taskTraceDeletionTree(t)
	t.Setenv("TASKTRACE_TEAM_ROOT", "")
	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)
	require.NoError(t, (&Task{ID: ids[1]}).Delete(s, actor))
	_, err := GetTaskByIDSimple(s, ids[1])
	require.ErrorAs(t, err, &ErrTaskDoesNotExist{})
	_, err = GetTaskByIDSimple(s, ids[2])
	require.NoError(t, err)
}

func TestTaskTraceDeleteSubtreeRejectsScopeChangesAndOtherRoots(t *testing.T) {
	t.Run("new descendant after permission check", func(t *testing.T) {
		actor, ids, _, _ := taskTraceDeletionTree(t)
		s := db.NewSession()
		defer s.Close()
		defer events.CleanupPending(s)
		task := &Task{ID: ids[1]}
		allowed, err := task.CanDelete(s, actor)
		require.NoError(t, err)
		require.True(t, allowed)
		added := &Task{Title: "Added during deletion", ProjectID: 1, ParentTaskID: ids[2]}
		require.NoError(t, added.Create(s, actor))
		require.ErrorAs(t, task.Delete(s, actor), &ErrTaskTraceUndoConflict{})
		for _, id := range append(ids, added.ID) {
			_, err := GetTaskByIDSimple(s, id)
			require.NoError(t, err)
		}
	})
	t.Run("descendant also belongs to another root", func(t *testing.T) {
		actor, ids, _, _ := taskTraceDeletionTree(t)
		s := db.NewSession()
		defer s.Close()
		defer events.CleanupPending(s)
		require.NoError(t, (&TaskRelation{TaskID: ids[3], OtherTaskID: ids[2], RelationKind: RelationKindSubtask}).Create(s, actor))
		require.ErrorAs(t, (&Task{ID: ids[1]}).Delete(s, actor), &ErrGenericForbidden{})
		for _, id := range ids {
			_, err := GetTaskByIDSimple(s, id)
			require.NoError(t, err)
		}
	})
}

func TestTaskTraceDeleteSubtreeUndoSavedFilterPositions(t *testing.T) {
	actor, ids, _, _ := taskTraceDeletionTree(t)
	s := db.NewSession()
	filter := &SavedFilter{Title: "Subtree filter", OwnerID: actor.ID, Filters: &TaskCollection{}}
	_, err := s.Insert(filter)
	require.NoError(t, err)
	view := &ProjectView{Title: "Filtered tasks", ProjectID: getProjectIDFromSavedFilterID(filter.ID), ViewKind: ProjectViewKindList}
	_, err = s.Insert(view)
	require.NoError(t, err)
	position := &TaskPosition{TaskID: ids[2], ProjectViewID: view.ID, Position: 1234.5}
	_, err = s.Insert(position)
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	payload := taskTraceDeleteSnapshot(t, actor, ids[1])
	restore := db.NewSession()
	defer restore.Close()
	defer events.CleanupPending(restore)
	_, err = restoreTaskTraceUndo(restore, actor, payload)
	require.NoError(t, err)
	require.NoError(t, restore.Commit())
	var recovered TaskPosition
	exists, err := restore.Where("task_id = ? AND project_view_id = ?", ids[2], view.ID).Get(&recovered)
	require.NoError(t, err)
	require.True(t, exists)
	require.InDelta(t, position.Position, recovered.Position, 1e-9, "saved-filter placement must survive deleting and undoing the ancestor")
}
