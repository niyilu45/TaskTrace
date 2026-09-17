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
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func undoEngineSession(t *testing.T) *xorm.Session {
	t.Helper()
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func undoEngineEntry(t *testing.T, s *xorm.Session, userID int64, group string) *TaskTraceUndoEntry {
	t.Helper()
	entry := &TaskTraceUndoEntry{
		UserID:  userID,
		GroupID: group,
		Label:   "测试修改",
		Payload: "{}",
	}
	_, err := s.Insert(entry)
	require.NoError(t, err)
	return entry
}

func TestTaskTraceUndoEngine(t *testing.T) {
	t.Run("only explicit model and operation allowlist", func(t *testing.T) {
		for _, model := range []any{
			&Project{},
			&TaskAttachment{},
			&Label{},
			&TaskTraceUndo{},
			&user.User{},
		} {
			assert.False(t, taskTraceUndoModelSupported("create", model))
			assert.NoError(t, web.LockMutationUndo(web.WithTaskTraceUndo(context.Background(), ""), nil, &user.User{ID: 1}, "create", model))
			finish, err := web.CaptureMutationUndo(web.WithTaskTraceUndo(context.Background(), ""), nil, &user.User{ID: 1}, "create", model)
			require.NoError(t, err)
			assert.Nil(t, finish)
		}
		assert.True(t, taskTraceUndoModelSupported("update", &Task{}))
		assert.False(t, taskTraceUndoModelSupported("read", &Task{}))
		assert.False(t, taskTraceUndoModelSupported("update", &TaskTraceMove{}))
	})
	t.Run("opt-in required and link shares unrecorded", func(t *testing.T) {
		assert.NoError(t, web.LockMutationUndo(context.Background(), nil, &user.User{ID: 1}, "update", &Task{}))
		finish, err := web.CaptureMutationUndo(context.Background(), nil, &user.User{ID: 1}, "update", &Task{})
		require.NoError(t, err)
		assert.Nil(t, finish)
		finish, err = web.CaptureMutationUndo(web.WithTaskTraceUndo(context.Background(), ""), nil, &LinkSharing{ID: 1}, "update", &Task{})
		require.NoError(t, err)
		assert.Nil(t, finish)
	})
	t.Run("history is private and persists across sessions", func(t *testing.T) {
		s := undoEngineSession(t)
		entry := undoEngineEntry(t, s, 1, uuid.NewString())
		require.NoError(t, s.Commit())
		fresh := db.NewReadSession()
		defer fresh.Close()
		owner := &TaskTraceUndo{}
		can, _, err := owner.CanRead(fresh, &user.User{ID: 1})
		require.NoError(t, err)
		require.True(t, can)
		require.NoError(t, owner.ReadOne(fresh, &user.User{ID: 1}))
		assert.Equal(t, entry.ID, owner.ID)
		assert.Equal(t, 1, owner.Count)
		other := &TaskTraceUndo{}
		can, _, err = other.CanRead(fresh, &user.User{ID: 2})
		require.NoError(t, err)
		require.True(t, can)
		require.NoError(t, other.ReadOne(fresh, &user.User{ID: 2}))
		assert.Zero(t, other.ID)
		assert.Zero(t, other.Count)
		can, _, err = other.CanRead(fresh, &LinkSharing{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("latest ID guard and ownership", func(t *testing.T) {
		s := undoEngineSession(t)
		group := uuid.NewString()
		old := undoEngineEntry(t, s, 1, group)
		latest := undoEngineEntry(t, s, 1, group)
		allowed := &TaskTraceUndo{ID: latest.ID}
		can, err := allowed.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.True(t, can)
		assert.Len(t, allowed.group, 2)
		can, err = (&TaskTraceUndo{ID: old.ID}).CanCreate(s, &user.User{ID: 1})
		require.Error(t, err)
		assert.False(t, can)
		can, err = (&TaskTraceUndo{ID: latest.ID}).CanCreate(s, &user.User{ID: 2})
		require.Error(t, err)
		assert.False(t, can)
		can, err = (&TaskTraceUndo{ID: latest.ID}).CanCreate(s, &LinkSharing{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("only contiguous repetitions share one step", func(t *testing.T) {
		s := undoEngineSession(t)
		group := uuid.NewString()
		undoEngineEntry(t, s, 1, group)
		undoEngineEntry(t, s, 1, uuid.NewString())
		undoEngineEntry(t, s, 1, group)
		latest := undoEngineEntry(t, s, 1, group)
		state := &TaskTraceUndo{userID: 1}
		require.NoError(t, state.loadStatus(s))
		assert.Equal(t, 3, state.Count)
		state.ID = latest.ID
		can, err := state.CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		require.True(t, can)
		assert.Len(t, state.group, 2)
	})
	t.Run("retains last fifty whole groups per user", func(t *testing.T) {
		s := undoEngineSession(t)
		other := undoEngineEntry(t, s, 2, uuid.NewString())
		for i := 0; i < 53; i++ {
			group := uuid.NewString()
			for j := 0; j < 3; j++ {
				undoEngineEntry(t, s, 1, group)
			}
		}
		require.NoError(t, trimTaskTraceUndo(s, 1))
		state := &TaskTraceUndo{userID: 1}
		require.NoError(t, state.loadStatus(s))
		assert.Equal(t, 50, state.Count)
		count, err := s.Where("user_id = ?", 1).Count(&TaskTraceUndoEntry{})
		require.NoError(t, err)
		assert.Equal(t, int64(150), count)
		exists, err := s.ID(other.ID).Exist(&TaskTraceUndoEntry{})
		require.NoError(t, err)
		assert.True(t, exists)
	})
}
