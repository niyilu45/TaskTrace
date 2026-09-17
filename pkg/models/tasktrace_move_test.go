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
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func taskTraceMoveSession(t *testing.T) *xorm.Session {
	t.Helper()
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	t.Cleanup(func() { events.CleanupPending(s); _ = s.Close() })
	for id := int64(1001); id <= 1008; id++ {
		_, err := s.Insert(&Task{
			ID:          id,
			Title:       fmt.Sprint("Move ", id),
			ProjectID:   1,
			Index:       id,
			CreatedByID: 1,
		})
		require.NoError(t, err)
	}
	return s
}

func taskTraceLink(t *testing.T, s *xorm.Session, parent, child int64) {
	t.Helper()
	require.NoError(t, (&TaskRelation{
		TaskID:       parent,
		OtherTaskID:  child,
		RelationKind: RelationKindSubtask,
	}).Create(s, &user.User{ID: 1}))
}

func taskTracePerformMove(t *testing.T, s *xorm.Session, m *TaskTraceMove) {
	t.Helper()
	can, err := m.CanCreate(s, &user.User{ID: 1})
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, m.Create(s, &user.User{ID: 1}))
}

func taskTraceParentIDs(t *testing.T, s *xorm.Session, id int64) []int64 {
	t.Helper()
	var relations []*TaskRelation
	require.NoError(t, s.Where("task_id = ? AND relation_kind = ?", id, RelationKindParenttask).OrderBy("other_task_id ASC").Find(&relations))
	ids := []int64{}
	for _, r := range relations {
		ids = append(ids, r.OtherTaskID)
	}
	return ids
}

func taskTracePosition(t *testing.T, s *xorm.Session, id int64) float64 {
	t.Helper()
	p := &TaskPosition{}
	exists, err := s.Where("task_id = ? AND project_view_id = ?", id, 1).Get(p)
	require.NoError(t, err)
	require.True(t, exists)
	return p.Position
}

func TestTaskTraceMove(t *testing.T) {
	t.Run("reparent preserves subtree and inverse relations", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceLink(t, s, 1001, 1002)
		taskTraceLink(t, s, 1002, 1003)
		taskTraceLink(t, s, 1004, 1005)
		m := &TaskTraceMove{
			TaskID:        1002,
			ParentID:      1004,
			BeforeTaskID:  1005,
			ProjectViewID: 1,
		}
		taskTracePerformMove(t, s, m)
		assert.Equal(t, []int64{1004}, taskTraceParentIDs(t, s, 1002))
		assert.Equal(t, []int64{1002}, taskTraceParentIDs(t, s, 1003))
		old, err := s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", 1001, 1002, RelationKindSubtask).Exist(&TaskRelation{})
		require.NoError(t, err)
		assert.False(t, old)
		next, err := s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", 1004, 1002, RelationKindSubtask).Exist(&TaskRelation{})
		require.NoError(t, err)
		assert.True(t, next)
		assert.Less(t, m.Position, taskTracePosition(t, s, 1005))
		require.NoError(t, s.Commit())
		fresh := db.NewReadSession()
		defer fresh.Close()
		assert.Equal(t, m.Position, taskTracePosition(t, fresh, 1002))
		assert.Equal(t, []int64{1004}, taskTraceParentIDs(t, fresh, 1002))
	})
	t.Run("promote to root before anchor", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceLink(t, s, 1001, 1002)
		m := &TaskTraceMove{
			TaskID:        1002,
			ParentID:      0,
			BeforeTaskID:  1001,
			ProjectViewID: 1,
		}
		taskTracePerformMove(t, s, m)
		assert.Empty(t, taskTraceParentIDs(t, s, 1002))
		assert.Less(t, m.Position, taskTracePosition(t, s, 1001))
	})
	t.Run("reorder and append siblings", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceLink(t, s, 1001, 1002)
		taskTraceLink(t, s, 1001, 1003)
		taskTraceLink(t, s, 1001, 1004)
		taskTracePerformMove(t, s, &TaskTraceMove{
			TaskID:        1004,
			ParentID:      1001,
			BeforeTaskID:  1002,
			ProjectViewID: 1,
		})
		assert.Less(t, taskTracePosition(t, s, 1004), taskTracePosition(t, s, 1002))
		taskTracePerformMove(t, s, &TaskTraceMove{
			TaskID:        1004,
			ParentID:      1001,
			ProjectViewID: 1,
		})
		assert.Greater(t, taskTracePosition(t, s, 1004), taskTracePosition(t, s, 1003))
		assert.Equal(t, []int64{1001}, taskTraceParentIDs(t, s, 1004))
	})
	for _, tc := range []struct {
		name                 string
		task, parent, before int64
	}{
		{
			"self",
			1002,
			1002,
			0,
		},
		{
			"descendant",
			1001,
			1003,
			0,
		},
		{
			"sixth level",
			1007,
			1005,
			0,
		},
		{
			"subtree exceeds five",
			1007,
			1004,
			0,
		},
		{
			"wrong sibling anchor",
			1007,
			1001,
			1005,
		},
		{
			"self anchor",
			1007,
			0,
			1007,
		},
		{
			"missing anchor",
			1007,
			0,
			999999,
		},
	} {
		t.Run(tc.name+" leaves all data unchanged", func(t *testing.T) {
			s := taskTraceMoveSession(t)
			for id := int64(1001); id < 1005; id++ {
				taskTraceLink(t, s, id, id+1)
			}
			taskTraceLink(t, s, 1006, 1007)
			taskTraceLink(t, s, 1007, 1008)
			var beforeRelations []*TaskRelation
			var beforePositions []*TaskPosition
			require.NoError(t, s.OrderBy("id ASC").Find(&beforeRelations))
			require.NoError(t, s.OrderBy("project_view_id ASC, task_id ASC").Find(&beforePositions))
			m := &TaskTraceMove{
				TaskID:        tc.task,
				ParentID:      tc.parent,
				BeforeTaskID:  tc.before,
				ProjectViewID: 1,
			}
			can, err := m.CanCreate(s, &user.User{ID: 1})
			require.NoError(t, err)
			require.True(t, can)
			require.Error(t, m.Create(s, &user.User{ID: 1}))
			var afterRelations []*TaskRelation
			var afterPositions []*TaskPosition
			require.NoError(t, s.OrderBy("id ASC").Find(&afterRelations))
			require.NoError(t, s.OrderBy("project_view_id ASC, task_id ASC").Find(&afterPositions))
			assert.Equal(t, beforeRelations, afterRelations)
			assert.Equal(t, beforePositions, afterPositions)
			require.NoError(t, s.Rollback())
		})
	}
	t.Run("exactly fifth level allowed", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		for id := int64(1001); id < 1004; id++ {
			taskTraceLink(t, s, id, id+1)
		}
		taskTracePerformMove(t, s, &TaskTraceMove{
			TaskID:        1005,
			ParentID:      1004,
			ProjectViewID: 1,
		})
		assert.Equal(t, []int64{1004}, taskTraceParentIDs(t, s, 1005))
	})
	t.Run("missing or mismatched view rejected", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		for _, view := range []int64{
			999999,
			5,
		} {
			can, err := (&TaskTraceMove{
				TaskID:        1001,
				ProjectViewID: view,
			}).CanCreate(s, &user.User{ID: 1})
			require.Error(t, err)
			assert.False(t, can)
		}
	})
	t.Run("view deleted after permission check rejected", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceLink(t, s, 1001, 1002)
		m := &TaskTraceMove{
			TaskID:        1002,
			ParentID:      1003,
			ProjectViewID: 1,
		}
		can, err := m.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		require.True(t, can)
		_, err = s.ID(1).Delete(&ProjectView{})
		require.NoError(t, err)
		require.Error(t, m.Create(s, &user.User{ID: 1}))
		assert.Equal(t, []int64{1001}, taskTraceParentIDs(t, s, 1002))
	})
	t.Run("new parent must be same project", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		// User 1 can write project 10, but cannot move a task from project 1 under it.
		can, err := (&TaskTraceMove{
			TaskID:        1001,
			ParentID:      19,
			ProjectViewID: 1,
		}).CanCreate(s, &user.User{ID: 1})
		var invalid ErrTaskTraceMoveInvalid
		require.ErrorAs(t, err, &invalid)
		assert.False(t, can)
	})
	t.Run("old parent must be writable", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		taskTraceLink(t, s, 41, 1001)
		can, err := (&TaskTraceMove{
			TaskID:        1001,
			ProjectViewID: 1,
		}).CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
		assert.Equal(t, []int64{41}, taskTraceParentIDs(t, s, 1001))
	})
	t.Run("new parent must be writable", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		can, err := (&TaskTraceMove{
			TaskID:        1001,
			ParentID:      41,
			ProjectViewID: 1,
		}).CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
}

func TestTaskTraceMovePermissions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		project int64
		auth    web.Auth
		allowed bool
	}{
		{
			"owner",
			1,
			&user.User{ID: 1},
			true,
		},
		{
			"unrelated user",
			1,
			&user.User{ID: 4},
			false,
		},
		{
			"direct project share",
			10,
			&user.User{ID: 1},
			true,
		},
		{
			"read only project share",
			9,
			&user.User{ID: 1},
			false,
		},
		{
			"inherited project share",
			43,
			&user.User{ID: 1},
			true,
		},
		{
			"team write",
			7,
			&user.User{ID: 1},
			true,
		},
		{
			"team read",
			6,
			&user.User{ID: 1},
			false,
		},
		{
			"link write",
			2,
			&LinkSharing{
				ID:         2,
				ProjectID:  2,
				Permission: PermissionWrite,
				SharedByID: 1,
			},
			true,
		},
		{
			"link read",
			1,
			&LinkSharing{
				ID:         1,
				ProjectID:  1,
				Permission: PermissionRead,
				SharedByID: 1,
			},
			false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := taskTraceMoveSession(t)
			_, err := s.ID(1001).Cols("project_id").Update(&Task{ProjectID: tc.project})
			require.NoError(t, err)
			_, err = s.Insert(&ProjectView{
				ID:        1001,
				ProjectID: tc.project,
				Title:     "Move test",
				ViewKind:  ProjectViewKindList,
			})
			require.NoError(t, err)
			can, err := (&TaskTraceMove{
				TaskID:        1001,
				ProjectViewID: 1001,
			}).CanCreate(s, tc.auth)
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, can)
			positions := &TaskTracePositions{
				ProjectID:     tc.project,
				ProjectViewID: 1001,
			}
			canRead, _, err := positions.CanRead(s, tc.auth)
			require.NoError(t, err)
			assert.Equal(t, tc.name != "unrelated user", canRead)
		})
	}
}

func TestTaskTracePositions(t *testing.T) {
	t.Run("includes completed and filter-hidden tasks with pagination", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		_, err := s.ID(1).Cols("filter").Update(&ProjectView{Filter: &TaskCollection{Filter: "done = false"}})
		require.NoError(t, err)
		p := &TaskTracePositions{
			ProjectID:     1,
			ProjectViewID: 1,
		}
		can, _, err := p.CanRead(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.True(t, can)
		result, count, total, err := p.ReadAll(s, &user.User{ID: 1}, "", 1, 1)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.Equal(t, int64(2), total)
		assert.Equal(t, int64(1), result.([]*TaskPosition)[0].TaskID)
		result, count, total, err = p.ReadAll(s, &user.User{ID: 1}, "", 2, 1)
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.Equal(t, int64(2), total)
		assert.Equal(t, int64(2), result.([]*TaskPosition)[0].TaskID)
	})
	t.Run("read permission includes link but rejects unrelated user", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		p := &TaskTracePositions{
			ProjectID:     1,
			ProjectViewID: 1,
		}
		can, _, err := p.CanRead(s, &LinkSharing{
			ID:         1,
			ProjectID:  1,
			Permission: PermissionRead,
			SharedByID: 1,
		})
		require.NoError(t, err)
		assert.True(t, can)
		can, _, err = p.CanRead(s, &user.User{ID: 4})
		require.NoError(t, err)
		assert.False(t, can)
		_, _, _, err = p.ReadAll(s, &user.User{ID: 4}, "", 1, 100)
		assert.True(t, IsErrGenericForbidden(err))
	})
	t.Run("excludes stale foreign and soft deleted tasks", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		_, err := s.ID(1001).Delete(&Task{})
		require.NoError(t, err)
		_, err = s.Insert(&TaskPosition{
			TaskID:        13,
			ProjectViewID: 1,
			Position:      100,
		}, &TaskPosition{
			TaskID:        1001,
			ProjectViewID: 1,
			Position:      101,
		})
		require.NoError(t, err)
		p := &TaskTracePositions{
			ProjectID:     1,
			ProjectViewID: 1,
		}
		result, count, total, err := p.ReadAll(s, &user.User{ID: 1}, "", 1, 100)
		require.NoError(t, err)
		assert.Equal(t, 2, count)
		assert.Equal(t, int64(2), total)
		assert.Len(t, result.([]*TaskPosition), 2)
	})
	t.Run("missing and foreign views rejected", func(t *testing.T) {
		s := taskTraceMoveSession(t)
		for _, view := range []int64{
			5,
			999999,
		} {
			can, _, err := (&TaskTracePositions{
				ProjectID:     1,
				ProjectViewID: view,
			}).CanRead(s, &user.User{ID: 1})
			require.Error(t, err)
			assert.False(t, can)
		}
	})
}
