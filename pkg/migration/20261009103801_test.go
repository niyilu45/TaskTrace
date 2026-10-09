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

type taskTraceSyncSentinel20261009103801 struct {
	ID    int64  `xorm:"autoincr pk"`
	Token string `xorm:"varchar(100) unique"`
}

func (taskTraceSyncSentinel20261009103801) TableName() string {
	return "tasktrace_sync_migration_sentinel"
}

func TestTaskTraceTeamSyncCheckpointMigration(t *testing.T) {
	engine, err := db.CreateTestEngine()
	require.NoError(t, err)
	tables := []any{
		TaskTraceTeamSyncCheckpoint20261009103801{},
		taskTraceSyncSentinel20261009103801{},
	}
	require.NoError(t, engine.DropTables(tables...))
	t.Cleanup(func() { require.NoError(t, engine.DropTables(tables...)) })
	require.NoError(t, engine.Sync2(taskTraceSyncSentinel20261009103801{}))
	_, err = engine.Insert(&taskTraceSyncSentinel20261009103801{Token: "preserve-me"})
	require.NoError(t, err)
	require.NoError(t, addTaskTraceTeamSyncCheckpoint20261009103801(engine))
	entry := &TaskTraceTeamSyncCheckpoint20261009103801{
		ID: "share-device-key", Generation: "committed-generation",
	}
	_, err = engine.Insert(entry)
	require.NoError(t, err)
	require.NoError(t, addTaskTraceTeamSyncCheckpoint20261009103801(engine))
	var stored TaskTraceTeamSyncCheckpoint20261009103801
	found, err := engine.ID(entry.ID).Get(&stored)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, entry.Generation, stored.Generation)
	s := engine.NewSession()
	defer s.Close()
	require.NoError(t, s.Begin())
	_, err = s.ID(entry.ID).Update(&TaskTraceTeamSyncCheckpoint20261009103801{Generation: "interrupted-generation"})
	require.NoError(t, err)
	require.NoError(t, s.Rollback())
	found, err = engine.ID(entry.ID).Get(&stored)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, entry.Generation, stored.Generation)
	sentinel := &taskTraceSyncSentinel20261009103801{}
	found, err = engine.Where("token = ?", "preserve-me").Get(sentinel)
	require.NoError(t, err)
	require.True(t, found)
	_, err = engine.Insert(&taskTraceSyncSentinel20261009103801{Token: "preserve-me"})
	require.Error(t, err, "existing unique index must survive")
}
