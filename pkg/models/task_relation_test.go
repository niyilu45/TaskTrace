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
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskRelation_Create(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  2,
			RelationKind: RelationKindSubtask,
		}
		err := rel.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		err = s.Commit()
		require.NoError(t, err)
		db.AssertExists(t, "task_relations", map[string]interface{}{
			"task_id":       1,
			"other_task_id": 2,
			"relation_kind": RelationKindSubtask,
			"created_by_id": 1,
		}, false)
	})
	t.Run("Two Tasks In Different Projects", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  13,
			RelationKind: RelationKindSubtask,
		}
		err := rel.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		err = s.Commit()
		require.NoError(t, err)
		db.AssertExists(t, "task_relations", map[string]interface{}{
			"task_id":       1,
			"other_task_id": 13,
			"relation_kind": RelationKindSubtask,
			"created_by_id": 1,
		}, false)
	})
	t.Run("Already Existing", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  29,
			RelationKind: RelationKindSubtask,
		}
		err := rel.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrRelationAlreadyExists(err))
	})
	t.Run("Same Task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:      1,
			OtherTaskID: 1,
		}
		err := rel.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrRelationTasksCannotBeTheSame(err))
	})
	t.Run("cycle with one subtask", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       29,
			OtherTaskID:  1,
			RelationKind: RelationKindSubtask,
		}
		err := rel.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err))
	})
	t.Run("cycle with multiple subtasks", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel1 := TaskRelation{
			TaskID:       1,
			OtherTaskID:  2,
			RelationKind: RelationKindSubtask,
		}
		err := rel1.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel2 := TaskRelation{
			TaskID:       2,
			OtherTaskID:  3,
			RelationKind: RelationKindSubtask,
		}
		err = rel2.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel3 := TaskRelation{
			TaskID:       3,
			OtherTaskID:  4,
			RelationKind: RelationKindSubtask,
		}
		err = rel3.Create(s, &user.User{ID: 1})
		require.NoError(t, err)

		// Cycle happens here
		rel4 := TaskRelation{
			TaskID:       4,
			OtherTaskID:  2,
			RelationKind: RelationKindSubtask,
		}
		err = rel4.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err))
	})
	t.Run("cycle with multiple subtasks tasks and relation back to parent", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel1 := TaskRelation{
			TaskID:       1,
			OtherTaskID:  2,
			RelationKind: RelationKindSubtask,
		}
		err := rel1.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel2 := TaskRelation{
			TaskID:       2,
			OtherTaskID:  3,
			RelationKind: RelationKindSubtask,
		}
		err = rel2.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel3 := TaskRelation{
			TaskID:       3,
			OtherTaskID:  4,
			RelationKind: RelationKindSubtask,
		}
		err = rel3.Create(s, &user.User{ID: 1})
		require.NoError(t, err)

		// Cycle happens here
		rel4 := TaskRelation{
			TaskID:       4,
			OtherTaskID:  1,
			RelationKind: RelationKindSubtask,
		}
		err = rel4.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err))
	})
	t.Run("cycle with one parenttask", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  29,
			RelationKind: RelationKindParenttask,
		}
		err := rel.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err))
	})
	t.Run("cycle with multiple parenttasks", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel1 := TaskRelation{
			TaskID:       1,
			OtherTaskID:  2,
			RelationKind: RelationKindParenttask,
		}
		err := rel1.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel2 := TaskRelation{
			TaskID:       2,
			OtherTaskID:  3,
			RelationKind: RelationKindParenttask,
		}
		err = rel2.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel3 := TaskRelation{
			TaskID:       3,
			OtherTaskID:  4,
			RelationKind: RelationKindParenttask,
		}
		err = rel3.Create(s, &user.User{ID: 1})
		require.NoError(t, err)

		// Cycle happens here
		rel4 := TaskRelation{
			TaskID:       4,
			OtherTaskID:  2,
			RelationKind: RelationKindParenttask,
		}
		err = rel4.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err))
	})
	t.Run("cycle with multiple parenttasks and relation back to parent", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel1 := TaskRelation{
			TaskID:       1,
			OtherTaskID:  2,
			RelationKind: RelationKindParenttask,
		}
		err := rel1.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel2 := TaskRelation{
			TaskID:       2,
			OtherTaskID:  3,
			RelationKind: RelationKindParenttask,
		}
		err = rel2.Create(s, &user.User{ID: 1})
		require.NoError(t, err)
		rel3 := TaskRelation{
			TaskID:       3,
			OtherTaskID:  4,
			RelationKind: RelationKindParenttask,
		}
		err = rel3.Create(s, &user.User{ID: 1})
		require.NoError(t, err)

		// Cycle happens here
		rel4 := TaskRelation{
			TaskID:       4,
			OtherTaskID:  1,
			RelationKind: RelationKindParenttask,
		}
		err = rel4.Create(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskRelationCycle(err))
	})
}

func TestTaskRelation_Delete(t *testing.T) {
	u := &user.User{ID: 1}

	t.Run("Normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  29,
			RelationKind: RelationKindSubtask,
		}
		err := rel.Delete(s, u)
		require.NoError(t, err)
		err = s.Commit()
		require.NoError(t, err)
		db.AssertMissing(t, "task_relations", map[string]interface{}{
			"task_id":       1,
			"other_task_id": 29,
			"relation_kind": RelationKindSubtask,
		})
		db.AssertMissing(t, "task_relations", map[string]interface{}{
			"task_id":       29,
			"other_task_id": 1,
			"relation_kind": RelationKindParenttask,
		})
	})
	t.Run("Not existing", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       9999,
			OtherTaskID:  3,
			RelationKind: RelationKindSubtask,
		}
		err := rel.Delete(s, u)
		require.Error(t, err)
		assert.True(t, IsErrRelationDoesNotExist(err))
	})
}

func TestTaskRelation_CanDelete(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()

	u1 := &user.User{ID: 1}
	u2 := &user.User{ID: 2}

	sharedProject := &Project{Title: "CanDelete shared project"}
	require.NoError(t, sharedProject.Create(s, u1))
	_, err := s.Insert(&ProjectUser{UserID: u2.ID, ProjectID: sharedProject.ID, Permission: PermissionWrite})
	require.NoError(t, err)

	baseTask := &Task{Title: "CanDelete base", ProjectID: sharedProject.ID}
	require.NoError(t, baseTask.Create(s, u1))

	privateProject := &Project{Title: "CanDelete private project"}
	require.NoError(t, privateProject.Create(s, u1))
	otherTask := &Task{Title: "CanDelete other", ProjectID: privateProject.ID}
	require.NoError(t, otherTask.Create(s, u1))

	rel := &TaskRelation{
		TaskID:       baseTask.ID,
		OtherTaskID:  otherTask.ID,
		RelationKind: RelationKindSubtask,
	}
	require.NoError(t, rel.Create(s, u1))
	require.NoError(t, s.Commit())

	t.Run("writer without access to the other task", func(t *testing.T) {
		can, err := rel.CanDelete(s, u2)
		require.NoError(t, err)
		require.False(t, can, "deleting a relation to a task the caller cannot read must be denied")
	})

	t.Run("writer with access to both tasks", func(t *testing.T) {
		can, err := rel.CanDelete(s, u1)
		require.NoError(t, err)
		require.True(t, can)
	})
}

func TestTaskRelation_CanCreate(t *testing.T) {
	t.Run("Normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  2,
			RelationKind: RelationKindSubtask,
		}
		can, err := rel.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.True(t, can)
	})
	t.Run("Two tasks on different projects", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  32,
			RelationKind: RelationKindSubtask,
		}
		can, err := rel.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.True(t, can)
	})
	t.Run("No update permissions on base task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       14,
			OtherTaskID:  1,
			RelationKind: RelationKindSubtask,
		}
		can, err := rel.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("No update permissions on base task, but read permissions", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       15,
			OtherTaskID:  1,
			RelationKind: RelationKindSubtask,
		}
		can, err := rel.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("No read permissions on other task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  14,
			RelationKind: RelationKindSubtask,
		}
		can, err := rel.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("Nonexisting base task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       999999,
			OtherTaskID:  1,
			RelationKind: RelationKindSubtask,
		}
		can, err := rel.CanCreate(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskDoesNotExist(err))
		assert.False(t, can)
	})
	t.Run("Nonexisting other task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		rel := TaskRelation{
			TaskID:       1,
			OtherTaskID:  999999,
			RelationKind: RelationKindSubtask,
		}
		can, err := rel.CanCreate(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.True(t, IsErrTaskDoesNotExist(err))
		assert.False(t, can)
	})
}

func TestTaskRelationDepthLimit(t *testing.T) {
	for _, inverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("inverse=%v", inverse), func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			s := db.NewSession()
			defer s.Close()
			_, err := s.Where("id > 0").Delete(&TaskRelation{})
			require.NoError(t, err)
			link := func(parent, child int64) error {
				rel := &TaskRelation{TaskID: parent, OtherTaskID: child, RelationKind: RelationKindSubtask}
				if inverse {
					rel.TaskID, rel.OtherTaskID, rel.RelationKind = child, parent, RelationKindParenttask
				}
				return rel.Create(s, &user.User{ID: 1})
			}
			for id := int64(1); id < 5; id++ {
				require.NoError(t, link(id, id+1))
			}
			before, err := s.Count(&TaskRelation{})
			require.NoError(t, err)
			var depthErr ErrTaskHierarchyDepth
			require.ErrorAs(t, link(5, 6), &depthErr)
			// Adding a parent above an existing five-level subtree also fails.
			require.ErrorAs(t, link(7, 1), &depthErr)
			after, err := s.Count(&TaskRelation{})
			require.NoError(t, err)
			assert.Equal(t, before, after, "neither direction of a rejected relation is written")
			// A second branch can have its own fifth level.
			require.NoError(t, link(4, 8))
			// An existing subtree must fit as a whole, not just its first node.
			require.NoError(t, link(9, 10))
			require.ErrorAs(t, link(4, 9), &depthErr)
			// Non-hierarchical relations remain unrestricted.
			require.NoError(t, (&TaskRelation{TaskID: 5, OtherTaskID: 6, RelationKind: RelationKindRelated}).Create(s, &user.User{ID: 1}))
			// Existing over-deep data remains editable/removable.
			require.NoError(t, (&TaskRelation{TaskID: 4, OtherTaskID: 5, RelationKind: RelationKindSubtask}).Delete(s, &user.User{ID: 1}))
		})
	}
}
