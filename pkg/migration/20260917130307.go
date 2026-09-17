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
	"time"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

type TaskTraceUndoEntry20260917130307 struct {
	ID      int64     `xorm:"bigint autoincr not null unique pk"`
	UserID  int64     `xorm:"bigint not null index"`
	GroupID string    `xorm:"varchar(36) not null"`
	Label   string    `xorm:"varchar(250) not null"`
	Payload string    `xorm:"longtext not null"`
	Created time.Time `xorm:"created not null"`
}

func (TaskTraceUndoEntry20260917130307) TableName() string { return "tasktrace_undo_entries" }

func addTaskTraceUndo20260917130307(tx *xorm.Engine) error {
	return tx.Sync2(TaskTraceUndoEntry20260917130307{})
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260917130307",
		Description: "Add per-user undo history for opted-in TaskTrace task editing",
		Migrate:     addTaskTraceUndo20260917130307,
		Rollback:    func(tx *xorm.Engine) error { return tx.DropTables(TaskTraceUndoEntry20260917130307{}) },
	})
}
