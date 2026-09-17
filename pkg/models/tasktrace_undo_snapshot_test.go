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
	"bytes"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/builder"
	"xorm.io/xorm"
)

type taskTraceUndoCRUD interface {
	web.CRUDable
	web.Permissions
}

func taskTraceUndoSnapshotMutation(t *testing.T, method string, object taskTraceUndoCRUD) (string, bool) {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)
	actor := &user.User{ID: 1}
	var can bool
	var err error
	switch method {
	case "create":
		can, err = object.CanCreate(s, actor)
	case "update":
		can, err = object.CanUpdate(s, actor)
	case "delete":
		can, err = object.CanDelete(s, actor)
	}
	require.NoError(t, err)
	require.True(t, can)
	before, supported, err := captureTaskTraceUndo(s, actor, method, object)
	require.NoError(t, err)
	require.True(t, supported)
	switch method {
	case "create":
		err = object.Create(s, actor)
	case "update":
		err = object.Update(s, actor)
	case "delete":
		err = object.Delete(s, actor)
	}
	require.NoError(t, err)
	payload, _, _, changed, err := finishTaskTraceUndo(s, actor, method, object, before)
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return payload, changed
}
func taskTraceUndoSnapshotApply(payload string, a web.Auth) error {
	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)
	_, err := restoreTaskTraceUndo(s, a, payload)
	if err != nil {
		_ = s.Rollback()
		return err
	}
	return s.Commit()
}
func taskTraceUndoSnapshotTask(t *testing.T, id int64) *Task {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	task := &Task{ID: id}
	require.NoError(t, task.ReadOne(s, &user.User{ID: 1}))
	return task
}
func taskTraceUndoSnapshotCreate(t *testing.T, title string) (*Task, string) {
	task := &Task{ProjectID: 1, Title: title}
	payload, changed := taskTraceUndoSnapshotMutation(t, "create", task)
	require.True(t, changed)
	return task, payload
}
func taskTraceUndoSnapshotDirect(t *testing.T, fn func(*xorm.Session)) {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)
	fn(s)
	require.NoError(t, s.Commit())
}

func TestTaskTraceUndoSnapshotTask(t *testing.T) {
	actor := &user.User{ID: 1}
	t.Run("only changed scalars restore and consecutive undo ignores updated time", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task := taskTraceUndoSnapshotTask(t, 1)
		originalTitle := task.Title
		task.Title = "new title"
		first, changed := taskTraceUndoSnapshotMutation(t, "update", task)
		require.True(t, changed)
		task = taskTraceUndoSnapshotTask(t, 1)
		task.Priority = 4
		second, changed := taskTraceUndoSnapshotMutation(t, "update", task)
		require.True(t, changed)
		taskTraceUndoSnapshotDirect(t, func(s *xorm.Session) {
			_, err := s.ID(1).Cols("description").Update(&Task{Description: "unrelated new description"})
			require.NoError(t, err)
		})
		require.NoError(t, taskTraceUndoSnapshotApply(second, actor))
		require.NoError(t, taskTraceUndoSnapshotApply(first, actor))
		restored := taskTraceUndoSnapshotTask(t, 1)
		assert.Equal(t, originalTitle, restored.Title)
		assert.Equal(t, "unrelated new description", restored.Description)
	})
	t.Run("conflicting field refuses without losing newer edit", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task := taskTraceUndoSnapshotTask(t, 1)
		task.Title = "new title"
		payload, _ := taskTraceUndoSnapshotMutation(t, "update", task)
		taskTraceUndoSnapshotDirect(t, func(s *xorm.Session) {
			_, err := s.ID(1).Cols("title").Update(&Task{Title: "someone else"})
			require.NoError(t, err)
		})
		require.ErrorAs(t, taskTraceUndoSnapshotApply(payload, actor), new(ErrTaskTraceUndoConflict))
		assert.Equal(t, "someone else", taskTraceUndoSnapshotTask(t, 1).Title)
	})
	t.Run("done and done timestamp restore", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task := taskTraceUndoSnapshotTask(t, 1)
		beforeDoneAt := task.DoneAt
		task.Done = true
		payload, _ := taskTraceUndoSnapshotMutation(t, "update", task)
		assert.True(t, taskTraceUndoSnapshotTask(t, 1).Done)
		require.NoError(t, taskTraceUndoSnapshotApply(payload, actor))
		restored := taskTraceUndoSnapshotTask(t, 1)
		assert.False(t, restored.Done)
		assert.True(t, beforeDoneAt.Equal(restored.DoneAt))
	})
	t.Run("create soft deletes and delete restores with positions", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task, created := taskTraceUndoSnapshotCreate(t, "new task")
		deleted, _ := taskTraceUndoSnapshotMutation(t, "delete", &Task{ID: task.ID})
		require.NoError(t, taskTraceUndoSnapshotApply(deleted, actor))
		assert.Equal(t, "new task", taskTraceUndoSnapshotTask(t, task.ID).Title)
		require.NoError(t, taskTraceUndoSnapshotApply(created, actor))
		s := db.NewSession()
		defer s.Close()
		var raw Task
		has, err := s.Unscoped().ID(task.ID).Get(&raw)
		require.NoError(t, err)
		assert.True(t, has)
		assert.False(t, raw.DeletedAt.IsZero())
	})
	t.Run("create refuses later dependent content", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task, payload := taskTraceUndoSnapshotCreate(t, "new task")
		taskTraceUndoSnapshotDirect(t, func(s *xorm.Session) {
			comment := &TaskComment{TaskID: task.ID, Comment: "added later"}
			require.NoError(t, comment.Create(s, actor))
		})
		require.ErrorAs(t, taskTraceUndoSnapshotApply(payload, actor), new(ErrTaskTraceUndoConflict))
		assert.Equal(t, "new task", taskTraceUndoSnapshotTask(t, task.ID).Title)
	})
	t.Run("no-op does not create undo record", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task := taskTraceUndoSnapshotTask(t, 1)
		_, changed := taskTraceUndoSnapshotMutation(t, "update", task)
		assert.False(t, changed)
	})
	t.Run("repeating completion restores reminders and dates", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task, _ := taskTraceUndoSnapshotCreate(t, "repeat")
		task = taskTraceUndoSnapshotTask(t, task.ID)
		task.RepeatAfter = 86400
		task.DueDate = time.Date(2026, 9, 18, 0, 0, 0, 0, time.UTC)
		task.Reminders = []*TaskReminder{{Reminder: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)}}
		taskTraceUndoSnapshotMutation(t, "update", task)
		task = taskTraceUndoSnapshotTask(t, task.ID)
		originalDate := task.DueDate
		originalReminder := task.Reminders[0].Reminder
		task.Done = true
		payload, _ := taskTraceUndoSnapshotMutation(t, "update", task)
		require.NoError(t, taskTraceUndoSnapshotApply(payload, actor))
		restored := taskTraceUndoSnapshotTask(t, task.ID)
		assert.True(t, originalDate.Equal(restored.DueDate))
		require.Len(t, restored.Reminders, 1)
		assert.True(t, originalReminder.Equal(restored.Reminders[0].Reminder))
	})
}

func TestTaskTraceUndoSnapshotComments(t *testing.T) {
	actor := &user.User{ID: 1}
	t.Run("create update delete restores exact history", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		comment := &TaskComment{TaskID: 1, Comment: "<h3>每日进展 · 2026-09-17</h3><p>first</p>"}
		created, _ := taskTraceUndoSnapshotMutation(t, "create", comment)
		original := comment.Comment
		savedSession := db.NewSession()
		var persisted TaskComment
		_, readErr := savedSession.ID(comment.ID).Get(&persisted)
		require.NoError(t, readErr)
		savedSession.Close()
		createdTime := persisted.Created
		comment.Comment = "changed"
		updated, _ := taskTraceUndoSnapshotMutation(t, "update", comment)
		deleted, _ := taskTraceUndoSnapshotMutation(t, "delete", comment)
		require.NoError(t, taskTraceUndoSnapshotApply(deleted, actor))
		require.NoError(t, taskTraceUndoSnapshotApply(updated, actor))
		s := db.NewSession()
		var restored TaskComment
		exists, err := s.ID(comment.ID).Get(&restored)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, original, restored.Comment)
		assert.True(t, createdTime.Equal(restored.Created))
		s.Close()
		require.NoError(t, taskTraceUndoSnapshotApply(created, actor))
		db.AssertMissing(t, "task_comments", map[string]interface{}{"id": comment.ID})
	})
	t.Run("different actor and revoked task permission deny", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		comment := &TaskComment{TaskID: 1, Comment: "own comment"}
		payload, _ := taskTraceUndoSnapshotMutation(t, "create", comment)
		require.ErrorAs(t, taskTraceUndoSnapshotApply(payload, &user.User{ID: 2}), new(ErrGenericForbidden))
		taskTraceUndoSnapshotDirect(t, func(s *xorm.Session) {
			_, err := s.ID(1).Cols("owner_id").Update(&Project{OwnerID: 2})
			require.NoError(t, err)
		})
		require.Error(t, taskTraceUndoSnapshotApply(payload, actor))
		db.AssertExists(t, "task_comments", map[string]interface{}{"id": comment.ID}, false)
	})
}

func TestTaskTraceUndoSnapshotRelations(t *testing.T) {
	actor := &user.User{ID: 1}
	t.Run("grouped create relation can both undo", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		task, created := taskTraceUndoSnapshotCreate(t, "child")
		relation := &TaskRelation{TaskID: 1, OtherTaskID: task.ID, RelationKind: RelationKindSubtask}
		linked, _ := taskTraceUndoSnapshotMutation(t, "create", relation)
		require.NoError(t, taskTraceUndoSnapshotApply(linked, actor))
		require.NoError(t, taskTraceUndoSnapshotApply(created, actor))
	})
	t.Run("relation deletion undo preserves original IDs", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		relation := &TaskRelation{TaskID: 1, OtherTaskID: 29, RelationKind: RelationKindSubtask}
		payload, _ := taskTraceUndoSnapshotMutation(t, "delete", relation)
		require.NoError(t, taskTraceUndoSnapshotApply(payload, actor))
		db.AssertExists(t, "task_relations", map[string]interface{}{"id": relation.ID, "task_id": 1, "other_task_id": 29}, false)
	})
	t.Run("move restores every parent and view position", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		var views []*ProjectView
		require.NoError(t, s.Where("project_id = ?", 1).Asc("id").Find(&views))
		require.NotEmpty(t, views)
		beforePositions, err := taskTraceUndoReadRows(s, &TaskPosition{}, builder.Eq{"project_view_id": views[0].ID})
		require.NoError(t, err)
		s.Close()
		moved, _ := taskTraceUndoSnapshotMutation(t, "create", &TaskTraceMove{TaskID: 29, ParentID: 2, ProjectViewID: views[0].ID})
		require.NoError(t, taskTraceUndoSnapshotApply(moved, actor))
		db.AssertExists(t, "task_relations", map[string]interface{}{"task_id": 29, "other_task_id": 1, "relation_kind": RelationKindParenttask}, false)
		s = db.NewSession()
		afterPositions, err := taskTraceUndoReadRows(s, &TaskPosition{}, builder.Eq{"project_view_id": views[0].ID})
		require.NoError(t, err)
		s.Close()
		assert.True(t, taskTraceUndoSame("positions", beforePositions, afterPositions))
	})
}

func TestTaskTraceUndoSnapshotOutstandingImages(t *testing.T) {
	actor := &user.User{ID: 1}
	t.Run("restore shared lists keeps copied attachment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		files.InitTestFileFixtures(t)
		var attachment *TaskAttachment
		taskTraceUndoSnapshotDirect(t, func(s *xorm.Session) {
			attachment = &TaskAttachment{TaskID: 1}
			require.NoError(t, attachment.NewAttachment(s, bytes.NewReader([]byte("image")), "image.png", 5, actor))
		})
		taskTraceSeedList(t, 1, taskTraceOutstandingItem{"a", fmt.Sprintf(`<img src="/api/v1/tasks/1/attachments/%d">`, attachment.ID), ""})
		taskTraceSeedList(t, 2, taskTraceOutstandingItem{"b", "B", ""})
		moved, _ := taskTraceUndoSnapshotMutation(t, "create", &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 2, ItemID: "a"})
		require.NoError(t, taskTraceUndoSnapshotApply(moved, actor))
		assert.Equal(t, []string{"a"}, taskTraceIDs(taskTraceReadList(t, 1)))
		assert.Equal(t, []string{"b"}, taskTraceIDs(taskTraceReadList(t, 2)))
		db.AssertExists(t, "task_attachments", map[string]interface{}{"task_id": 2}, false)
	})
	t.Run("missing original attachment refuses whole undo", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		files.InitTestFileFixtures(t)
		taskTraceSeedList(t, 1, taskTraceOutstandingItem{"a", `<img src="/api/v1/tasks/1/attachments/1">`, ""})
		taskTraceSeedList(t, 2, taskTraceOutstandingItem{"b", "B", ""})
		moved, _ := taskTraceUndoSnapshotMutation(t, "create", &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 2, ItemID: "a"})
		taskTraceUndoSnapshotDirect(t, func(s *xorm.Session) { require.NoError(t, (&TaskAttachment{ID: 1, TaskID: 1}).Delete(s, actor)) })
		require.ErrorAs(t, taskTraceUndoSnapshotApply(moved, actor), new(ErrTaskTraceUndoConflict))
		assert.Empty(t, taskTraceReadList(t, 1).items)
		assert.Equal(t, []string{"b", "a"}, taskTraceIDs(taskTraceReadList(t, 2)))
	})
}

func TestTaskTraceUndoSnapshotScope(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	task := taskTraceUndoSnapshotTask(t, 1)
	task.Priority = 2
	payload, changed := taskTraceUndoSnapshotMutation(t, "update", task)
	require.True(t, changed)
	var saved taskTraceUndoCapture
	require.NoError(t, json.Unmarshal([]byte(payload), &saved))
	assert.Equal(t, []string{"Priority"}, saved.Fields)
	assert.Empty(t, saved.Parts)
	assert.Empty(t, saved.Before.Parts)
	assert.NotContains(t, payload, "Description")
}

func TestTaskTraceUndoSnapshotHierarchySafety(t *testing.T) {
	actor := &user.User{ID: 1}
	for _, cycle := range []bool{false, true} {
		t.Run(fmt.Sprintf("cycle=%v", cycle), func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			parent, _ := taskTraceUndoSnapshotCreate(t, "parent")
			child, _ := taskTraceUndoSnapshotCreate(t, "child")
			relation := &TaskRelation{TaskID: parent.ID, OtherTaskID: child.ID, RelationKind: RelationKindSubtask}
			taskTraceUndoSnapshotMutation(t, "create", relation)
			deleted, _ := taskTraceUndoSnapshotMutation(t, "delete", relation)
			if cycle {
				taskTraceUndoSnapshotMutation(t, "create", &TaskRelation{TaskID: child.ID, OtherTaskID: parent.ID, RelationKind: RelationKindSubtask})
			} else {
				current := parent.ID
				for i := 0; i < 4; i++ {
					ancestor, _ := taskTraceUndoSnapshotCreate(t, fmt.Sprint("ancestor ", i))
					taskTraceUndoSnapshotMutation(t, "create", &TaskRelation{TaskID: ancestor.ID, OtherTaskID: current, RelationKind: RelationKindSubtask})
					current = ancestor.ID
				}
			}
			require.Error(t, taskTraceUndoSnapshotApply(deleted, actor))
			db.AssertMissing(t, "task_relations", map[string]interface{}{"task_id": parent.ID, "other_task_id": child.ID, "relation_kind": RelationKindSubtask})
		})
	}
}

func TestTaskTraceUndoSnapshotDeletedDependency(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	parent, _ := taskTraceUndoSnapshotCreate(t, "parent")
	child, _ := taskTraceUndoSnapshotCreate(t, "child")
	taskTraceUndoSnapshotMutation(t, "create", &TaskRelation{TaskID: parent.ID, OtherTaskID: child.ID, RelationKind: RelationKindSubtask})
	deleted, _ := taskTraceUndoSnapshotMutation(t, "delete", &Task{ID: child.ID})
	taskTraceUndoSnapshotMutation(t, "delete", &Task{ID: parent.ID})
	require.ErrorAs(t, taskTraceUndoSnapshotApply(deleted, &user.User{ID: 1}), new(ErrTaskTraceUndoConflict))
	s := db.NewSession()
	defer s.Close()
	var task Task
	has, err := s.ID(child.ID).Get(&task)
	require.NoError(t, err)
	assert.False(t, has)
}
