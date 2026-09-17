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

package migration

import (
	"testing"

	"code.vikunja.io/api/pkg/db"

	"github.com/stretchr/testify/require"
)

type taskTraceUndoSentinel20260917130307 struct {
	ID    int64  `xorm:"autoincr pk"`
	Token string `xorm:"varchar(100) unique"`
}

func (taskTraceUndoSentinel20260917130307) TableName() string {
	return "tasktrace_undo_migration_sentinel"
}

func TestTaskTraceUndoMigration(t *testing.T) {
	engine, err := db.CreateTestEngine()
	require.NoError(t, err)
	tables := []any{
		TaskTraceUndoEntry20260917130307{},
		taskTraceUndoSentinel20260917130307{},
	}
	require.NoError(t, engine.DropTables(tables...))
	t.Cleanup(func() { require.NoError(t, engine.DropTables(tables...)) })
	require.NoError(t, engine.Sync2(taskTraceUndoSentinel20260917130307{}))
	_, err = engine.Insert(&taskTraceUndoSentinel20260917130307{Token: "preserve-me"})
	require.NoError(t, err)
	require.NoError(t, addTaskTraceUndo20260917130307(engine))
	entry := &TaskTraceUndoEntry20260917130307{
		UserID:  1,
		GroupID: "test",
		Label:   "撤销测试",
		Payload: "{}",
	}
	_, err = engine.Insert(entry)
	require.NoError(t, err)
	require.Positive(t, entry.ID)
	require.False(t, entry.Created.IsZero())
	require.NoError(t, addTaskTraceUndo20260917130307(engine))
	var stored TaskTraceUndoEntry20260917130307
	found, err := engine.ID(entry.ID).Get(&stored)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "撤销测试", stored.Label)
	sentinel := &taskTraceUndoSentinel20260917130307{}
	found, err = engine.Where("token = ?", "preserve-me").Get(sentinel)
	require.NoError(t, err)
	require.True(t, found)
	_, err = engine.Insert(&taskTraceUndoSentinel20260917130307{Token: "preserve-me"})
	require.Error(t, err, "existing unique index must survive")
}
