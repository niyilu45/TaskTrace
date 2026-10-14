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
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/builder"
	"xorm.io/xorm"
)

func taskTraceMoveDestination(t *testing.T, s *xorm.Session, project int64) {
	t.Helper()
	_, err := s.Insert(&ProjectView{ID: 1001, ProjectID: project, Title: "Destination", ViewKind: ProjectViewKindList})
	require.NoError(t, err)
}

func TestTaskTraceMoveCrossProject(t *testing.T) {
	t.Run("moves complete subtree preserving task data and old parent", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceMoveDestination(t, s, 10)
		taskTraceLink(t, s, 1001, 1002)
		taskTraceLink(t, s, 1002, 1003)
		taskTraceLink(t, s, 1003, 1004)
		_, err := s.ID(1003).Cols("done", "status", "description", "percent_done", "priority").Update(&Task{Done: true, Status: TaskStatusDone, Description: "Keep description and inline images", PercentDone: .7, Priority: 3})
		require.NoError(t, err)
		_, err = s.Insert(&TaskComment{ID: 1001, TaskID: 1003, AuthorID: 1, Comment: "<p>Outstanding items and daily progress with inline images</p>"})
		require.NoError(t, err)
		_, err = s.Insert(&TaskAttachment{ID: 1001, TaskID: 1003, FileID: 1, CreatedByID: 1})
		require.NoError(t, err)
		_, err = s.Insert(&TaskReminder{ID: 1001, TaskID: 1003, Reminder: time.Now().Add(time.Hour)})
		require.NoError(t, err)
		require.NoError(t, addToFavorites(s, 1003, &user.User{ID: 1}, FavoriteKindTask))
		before := map[int64]taskTraceUndoRow{}
		for _, id := range []int64{1001, 1002, 1003, 1004} {
			before[id], err = taskTraceUndoReadTask(s, id)
			require.NoError(t, err)
		}
		parts := map[string][]taskTraceUndoRow{}
		for _, name := range []string{"comments", "attachments", "reminders", "favorites", "assignees"} {
			parts[name], err = taskTraceUndoReadRows(s, taskTraceUndoBean(name), builder.Eq{})
			require.NoError(t, err)
		}
		taskTracePerformMove(t, s, &TaskTraceMove{TaskID: 1002, ProjectID: 10, ProjectViewID: 1001})
		for _, id := range []int64{1001, 1002, 1003, 1004} {
			after, err := taskTraceUndoReadTask(s, id)
			require.NoError(t, err)
			expectedProject := int64(10)
			if id == 1001 {
				expectedProject = 1
			}
			assert.Equal(t, expectedProject, taskTraceUndoInt(after, "ProjectID"))
			if id != 1001 {
				delete(before[id], "ProjectID")
				delete(before[id], "Index")
				delete(after, "ProjectID")
				delete(after, "Index")
			}
			assert.Equal(t, before[id], after)
		}
		for name, rows := range parts {
			after, err := taskTraceUndoReadRows(s, taskTraceUndoBean(name), builder.Eq{})
			require.NoError(t, err)
			assert.Equal(t, rows, after, name)
		}
		assert.Empty(t, taskTraceParentIDs(t, s, 1002))
		assert.Equal(t, []int64{1002}, taskTraceParentIDs(t, s, 1003))
		assert.Equal(t, []int64{1003}, taskTraceParentIDs(t, s, 1004))
		var positions []*TaskPosition
		require.NoError(t, s.Where("project_view_id = ?", 1001).Find(&positions))
		found := map[int64]bool{}
		for _, position := range positions {
			found[position.TaskID] = true
		}
		assert.True(t, found[1002] && found[1003] && found[1004])
		for _, id := range []int64{1002, 1003, 1004} {
			alias := &TaskIndexAlias{ProjectID: 1, Index: id}
			has, err := s.Get(alias)
			require.NoError(t, err)
			require.True(t, has)
			assert.Equal(t, id, alias.TaskID)
		}
		// Moving back can also set a new parent in the destination project.
		taskTracePerformMove(t, s, &TaskTraceMove{TaskID: 1002, ProjectID: 1, ParentID: 1001, ProjectViewID: 1})
		assert.Equal(t, []int64{1001}, taskTraceParentIDs(t, s, 1002))
		child, err := GetTaskSimple(s, &Task{ID: 1004})
		require.NoError(t, err)
		assert.Equal(t, int64(1), child.ProjectID)
	})
	t.Run("rejects invalid sibling before changing project", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceMoveDestination(t, s, 10)
		taskTraceLink(t, s, 1001, 1002)
		m := &TaskTraceMove{TaskID: 1002, ProjectID: 10, ProjectViewID: 1001, BeforeTaskID: 1001}
		can, err := m.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		require.True(t, can)
		require.Error(t, m.Create(s, &user.User{ID: 1}))
		task, err := GetTaskSimple(s, &Task{ID: 1002})
		require.NoError(t, err)
		assert.Equal(t, int64(1), task.ProjectID)
		assert.Equal(t, []int64{1001}, taskTraceParentIDs(t, s, 1002))
	})
	t.Run("rejects subtree changed after authorization", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceMoveDestination(t, s, 10)
		m := &TaskTraceMove{TaskID: 1002, ProjectID: 10, ProjectViewID: 1001}
		can, err := m.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		require.True(t, can)
		taskTraceLink(t, s, 1002, 1003)
		require.Error(t, m.Create(s, &user.User{ID: 1}))
		task, err := GetTaskSimple(s, &Task{ID: 1002})
		require.NoError(t, err)
		assert.Equal(t, int64(1), task.ProjectID)
	})
	t.Run("rollback restores project indexes positions and relations together", func(t *testing.T) {
		setup := taskTraceMoveSession(t)
		taskTraceMoveDestination(t, setup, 10)
		taskTraceLink(t, setup, 1001, 1002)
		taskTraceLink(t, setup, 1002, 1003)
		require.NoError(t, setup.Commit())
		s := db.NewSession()
		defer s.Close()
		defer events.CleanupPending(s)
		taskTracePerformMove(t, s, &TaskTraceMove{TaskID: 1002, ProjectID: 10, ProjectViewID: 1001})
		require.NoError(t, s.Rollback())
		fresh := db.NewSession()
		defer fresh.Close()
		for _, id := range []int64{1002, 1003} {
			task, err := GetTaskSimple(fresh, &Task{ID: id})
			require.NoError(t, err)
			assert.Equal(t, int64(1), task.ProjectID)
			assert.Equal(t, id, task.Index)
		}
		assert.Equal(t, []int64{1001}, taskTraceParentIDs(t, fresh, 1002))
		has, err := fresh.Where("project_view_id = ?", 1001).Exist(&TaskPosition{})
		require.NoError(t, err)
		assert.False(t, has)
	})
	t.Run("cross project move does not create a partial structural undo", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceMoveDestination(t, s, 10)
		require.NoError(t, s.Commit())
		payload, changed := taskTraceUndoSnapshotMutation(t, "create", &TaskTraceMove{TaskID: 1002, ProjectID: 10, ProjectViewID: 1001})
		require.True(t, changed)
		var conflict ErrTaskTraceUndoConflict
		require.ErrorAs(t, taskTraceUndoSnapshotApply(payload, &user.User{ID: 1}), &conflict)
	})
}

func TestTaskTraceMoveCrossProjectPermissions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		project int64
		allowed bool
	}{
		{"direct write", 10, true}, {"direct read", 9, false},
		{"inherited write", 43, true}, {"team write", 7, true}, {"team read", 6, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := taskTraceMoveSession(t)
			taskTraceMoveDestination(t, s, tc.project)
			can, err := (&TaskTraceMove{TaskID: 1001, ProjectID: tc.project, ProjectViewID: 1001}).CanCreate(s, &user.User{ID: 1})
			if tc.allowed {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.allowed, can)
		})
	}
	t.Run("a read only descendant denies entire move", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceMoveDestination(t, s, 10)
		taskTraceLink(t, s, 1001, 1002)
		_, err := s.ID(1002).Cols("project_id").Update(&Task{ProjectID: 9})
		require.NoError(t, err)
		can, err := (&TaskTraceMove{TaskID: 1001, ProjectID: 10, ProjectViewID: 1001}).CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("link share does not grant destination access", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceMoveDestination(t, s, 10)
		_, err := s.ID(1001).Cols("project_id").Update(&Task{ProjectID: 2})
		require.NoError(t, err)
		var actor web.Auth = &LinkSharing{ID: 2, ProjectID: 2, Permission: PermissionWrite, SharedByID: 1}
		can, err := (&TaskTraceMove{TaskID: 1001, ProjectID: 10, ProjectViewID: 1001}).CanCreate(s, actor)
		require.Error(t, err)
		assert.False(t, can)
	})
}
