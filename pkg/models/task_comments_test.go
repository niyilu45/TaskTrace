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
	"fmt"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/builder"
)

func TestTaskComment_Create(t *testing.T) {
	u := &user.User{ID: 1}
	t.Run("normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{
			Comment: "test",
			TaskID:  1,
		}
		err := tc.Create(s, u)
		require.NoError(t, err)
		assert.Equal(t, "test", tc.Comment)
		assert.Equal(t, int64(1), tc.Author.ID)
		err = s.Commit()
		require.NoError(t, err)
		events.DispatchPending(context.Background(), s)
		events.AssertDispatched(t, &TaskCommentCreatedEvent{})

		db.AssertExists(t, "task_comments", map[string]interface{}{
			"id":        tc.ID,
			"author_id": u.ID,
			"comment":   "test",
			"task_id":   1,
		}, false)
	})
	t.Run("nonexisting task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{
			Comment: "test",
			TaskID:  99999,
		}
		err := tc.Create(s, u)
		require.Error(t, err)
		assert.True(t, IsErrTaskDoesNotExist(err))
	})
	t.Run("retry with the same team marker is idempotent", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		events.ClearDispatchedEvents()
		body := taskTraceTeamAddMarker("<h3>Progress 2026-09-24</h3><p>saved once</p>", "retry-token", "user1")

		firstSession := db.NewSession()
		first := &TaskComment{Comment: body, TaskID: 1}
		require.NoError(t, first.Create(firstSession, u))
		require.NoError(t, firstSession.Commit())
		events.DispatchPending(context.Background(), firstSession)
		firstSession.Close()

		secondSession := db.NewSession()
		second := &TaskComment{Comment: body, TaskID: 1}
		require.NoError(t, second.Create(secondSession, u))
		require.Equal(t, first.ID, second.ID)
		require.NoError(t, secondSession.Commit())
		events.DispatchPending(context.Background(), secondSession)
		secondSession.Close()

		db.AssertCount(t, "task_comments", builder.Eq{"task_id": 1, "comment": body}, 1)
		assert.Equal(t, 1, events.CountDispatchedEvents((&TaskCommentCreatedEvent{}).Name()))
	})
	t.Run("retry with the same outstanding list is idempotent", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		events.ClearDispatchedEvents()
		body := `<h3 data-tasktrace-comment-type="outstanding">Outstanding</h3><ul><li data-id="one">one</li></ul>`

		firstSession := db.NewSession()
		first := &TaskComment{Comment: body, TaskID: 1}
		require.NoError(t, first.Create(firstSession, u))
		require.NoError(t, firstSession.Commit())
		events.DispatchPending(context.Background(), firstSession)
		firstSession.Close()

		secondSession := db.NewSession()
		second := &TaskComment{Comment: body, TaskID: 1}
		require.NoError(t, second.Create(secondSession, u))
		require.Equal(t, first.ID, second.ID)
		require.NoError(t, secondSession.Commit())
		events.DispatchPending(context.Background(), secondSession)
		secondSession.Close()

		db.AssertCount(t, "task_comments", builder.Eq{"task_id": 1, "comment": body}, 1)
		assert.Equal(t, 1, events.CountDispatchedEvents((&TaskCommentCreatedEvent{}).Name()))
	})
	t.Run("should send notifications for comment mentions", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		task, err := GetTaskByIDSimple(s, 32)
		require.NoError(t, err)
		tc := &TaskComment{
			Comment: `<p>Lorem Ipsum <mention-user data-id="user2">@user2</mention-user></p>`,
			TaskID:  32, // user2 has access to the project that task belongs to
		}
		err = tc.Create(s, u)
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		ev := &TaskCommentCreatedEvent{
			Task:    &task,
			Doer:    u,
			Comment: tc,
		}

		events.TestListener(t, ev, &SendTaskCommentNotification{})
		db.AssertExists(t, "notifications", map[string]interface{}{
			"subject_id":    tc.ID,
			"notifiable_id": 2,
			"name":          (&TaskCommentNotification{}).Name(),
		}, false)
	})
	t.Run("should mark task unread for project members on comment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		task, err := GetTaskByIDSimple(s, 32)
		require.NoError(t, err)

		tc := &TaskComment{
			Comment: "test comment",
			TaskID:  32,
		}
		err = tc.Create(s, u)
		require.NoError(t, err)
		require.NoError(t, s.Commit())

		ev := &TaskCommentCreatedEvent{
			Task:    &task,
			Doer:    u,
			Comment: tc,
		}

		events.TestListener(t, ev, &MarkTaskUnreadOnComment{})

		db.AssertExists(t, "task_unread_statuses", map[string]interface{}{
			"task_id": task.ID,
			"user_id": 2,
		}, false)

		db.AssertMissing(t, "task_unread_statuses", map[string]interface{}{
			"task_id": task.ID,
			"user_id": u.ID,
		})
	})
}

func TestTaskComment_Delete(t *testing.T) {
	u := &user.User{ID: 1}

	t.Run("normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{
			ID:     1,
			TaskID: 1,
		}
		err := tc.Delete(s, u)
		require.NoError(t, err)
		err = s.Commit()
		require.NoError(t, err)

		db.AssertMissing(t, "task_comments", map[string]interface{}{
			"id": 1,
		})
	})
	t.Run("nonexisting comment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{ID: 9999}
		err := tc.Delete(s, u)
		require.Error(t, err)
		assert.True(t, IsErrTaskCommentDoesNotExist(err))
	})
	t.Run("not the own comment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{ID: 1, TaskID: 1}
		can, err := tc.CanDelete(s, &user.User{ID: 2})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("collaboration task owner can edit and delete every comment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		root := t.TempDir()
		dataRoot := t.TempDir()
		t.Setenv("TASKTRACE_TEAM_ROOT", root)
		t.Setenv("TASKTRACE_DATA_ROOT", dataRoot)
		require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(dataRoot, "team-sync.json"), &taskTraceTeamState{
			Schema: taskTraceTeamSchema,
			Bindings: []TaskTraceTeamBinding{{
				ShareID: "comment-owner", Repository: root, Owner: `CHINA\user1`, RootTaskID: 1,
				NodeTasks: map[string]int64{"root": 1},
			}},
		}))

		s := db.NewSession()
		defer s.Close()
		comment := &TaskComment{ID: 1001, TaskID: 1, AuthorID: 2, Comment: "another member"}
		_, err := s.Insert(comment)
		require.NoError(t, err)

		owner := &user.User{ID: 1, Username: "user1"}
		can, err := (&TaskComment{ID: comment.ID, TaskID: comment.TaskID}).CanUpdate(s, owner)
		require.NoError(t, err)
		require.True(t, can)
		can, err = (&TaskComment{ID: comment.ID, TaskID: comment.TaskID}).CanDelete(s, owner)
		require.NoError(t, err)
		require.True(t, can)
	})
}

func TestTaskComment_Update(t *testing.T) {
	u := &user.User{ID: 1}

	t.Run("normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{
			ID:      1,
			Comment: "testing",
		}
		err := tc.Update(s, u)
		require.NoError(t, err)
		err = s.Commit()
		require.NoError(t, err)

		db.AssertExists(t, "task_comments", map[string]interface{}{
			"id":      1,
			"comment": "testing",
		}, false)
	})
	t.Run("nonexisting comment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{
			ID: 9999,
		}
		err := tc.Update(s, u)
		require.Error(t, err)
		assert.True(t, IsErrTaskCommentDoesNotExist(err))
	})
	t.Run("retry with unchanged content does not emit another edit", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		events.ClearDispatchedEvents()

		firstSession := db.NewSession()
		first := &TaskComment{ID: 1, TaskID: 1, Comment: "updated once"}
		require.NoError(t, first.Update(firstSession, u))
		require.NoError(t, firstSession.Commit())
		events.DispatchPending(context.Background(), firstSession)
		firstSession.Close()

		secondSession := db.NewSession()
		retry := &TaskComment{ID: 1, TaskID: 1, Comment: "updated once"}
		require.NoError(t, retry.Update(secondSession, u))
		require.NoError(t, secondSession.Commit())
		events.DispatchPending(context.Background(), secondSession)
		secondSession.Close()

		assert.Equal(t, 1, events.CountDispatchedEvents((&TaskCommentUpdatedEvent{}).Name()))
	})
	t.Run("not the own comment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{ID: 1, TaskID: 1}
		can, err := tc.CanUpdate(s, &user.User{ID: 2})
		require.NoError(t, err)
		assert.False(t, can)
	})
}

func TestTaskComment_ReadOne(t *testing.T) {
	u := &user.User{ID: 1}

	t.Run("normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{ID: 1, TaskID: 1}
		err := tc.ReadOne(s, u)
		require.NoError(t, err)
		assert.Equal(t, "Lorem Ipsum Dolor Sit Amet", tc.Comment)
		assert.NotEmpty(t, tc.Author.ID)
		assert.Empty(t, tc.Author.Email)
	})
	t.Run("nonexisting", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{ID: 9999}
		err := tc.ReadOne(s, u)
		require.Error(t, err)
		assert.True(t, IsErrTaskCommentDoesNotExist(err))
	})
}

func TestTaskComment_ReadAll(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{TaskID: 1}
		u := &user.User{ID: 1}
		result, resultCount, total, err := tc.ReadAll(s, u, "", 0, -1)
		require.NoError(t, err)
		resultComment := result.([]*TaskComment)
		assert.Equal(t, 1, resultCount)
		assert.Equal(t, int64(1), total)
		assert.Equal(t, int64(1), resultComment[0].ID)
		assert.Equal(t, "Lorem Ipsum Dolor Sit Amet", resultComment[0].Comment)
		assert.NotEmpty(t, resultComment[0].Author.ID)
	})
	t.Run("no access to task", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{TaskID: 14}
		u := &user.User{ID: 1}
		_, _, _, err := tc.ReadAll(s, u, "", 0, -1)
		require.Error(t, err)
		assert.True(t, IsErrGenericForbidden(err))
	})
	t.Run("comment from link share", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{TaskID: 35}
		u := &user.User{ID: 1}
		result, _, _, err := tc.ReadAll(s, u, "", 0, -1)
		require.NoError(t, err)
		comments := result.([]*TaskComment)
		assert.Len(t, comments, 2)
		var foundComment bool
		for _, comment := range comments {
			if comment.AuthorID == -2 {
				foundComment = true
			}
			assert.NotNil(t, comment.Author)
		}
		assert.True(t, foundComment)
	})
	t.Run("normal", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()

		tc := &TaskComment{TaskID: 35}
		u := &user.User{ID: 1}
		result, _, _, err := tc.ReadAll(s, u, "COMMENT 15", 0, -1)
		require.NoError(t, err)
		resultComment := result.([]*TaskComment)
		assert.Equal(t, int64(15), resultComment[0].ID)
	})
}

func TestAddCommentsToTasksLimit(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()

	taskID := int64(1)

	// Add a bunch of comments to exceed the pagination limit
	for i := 0; i < 60; i++ {
		_, err := s.Insert(&TaskComment{Comment: fmt.Sprintf("bulk %d", i), TaskID: taskID, AuthorID: 1})
		require.NoError(t, err)
	}

	task := &Task{ID: taskID}
	taskMap := map[int64]*Task{taskID: task}

	err := addCommentsToTasks(s, []int64{taskID}, taskMap)
	require.NoError(t, err)
	assert.Len(t, task.Comments, 50)
}
