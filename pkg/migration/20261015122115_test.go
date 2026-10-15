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
	"code.vikunja.io/api/pkg/db"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type weeklyReminderBefore struct {
	ID       int64     `xorm:"bigint autoincr not null unique pk"`
	TaskID   int64     `xorm:"bigint not null INDEX"`
	Reminder time.Time `xorm:"DATETIME not null INDEX 'reminder'"`
}

func (weeklyReminderBefore) TableName() string { return "task_reminders" }
func TestWeeklyReminderMigration(t *testing.T) {
	engine, err := db.CreateTestEngine()
	require.NoError(t, err)
	require.NoError(t, engine.DropTables(weeklyReminderBefore{}))
	t.Cleanup(func() { require.NoError(t, engine.DropTables(weeklyReminderBefore{})) })
	require.NoError(t, engine.Sync2(weeklyReminderBefore{}))
	before := weeklyReminderBefore{TaskID: 42, Reminder: time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)}
	_, err = engine.Insert(&before)
	require.NoError(t, err)
	tables, err := engine.DBMetas()
	require.NoError(t, err)
	indexes := map[string]bool{}
	for _, table := range tables {
		if table.Name == "task_reminders" {
			for name := range table.Indexes {
				indexes[name] = true
			}
		}
	}
	require.NoError(t, partialSync(engine, TaskReminder20261015122115{}))
	require.NoError(t, partialSync(engine, TaskReminder20261015122115{}))
	var after weeklyReminderBefore
	found, err := engine.ID(before.ID).Get(&after)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, before.TaskID, after.TaskID)
	require.True(t, before.Reminder.Equal(after.Reminder))
	tables, err = engine.DBMetas()
	require.NoError(t, err)
	for _, table := range tables {
		if table.Name == "task_reminders" {
			for name := range indexes {
				require.Contains(t, table.Indexes, name)
			}
			require.NotNil(t, table.GetColumn("repeat_rule"))
		}
	}
}
