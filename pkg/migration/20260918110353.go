// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

package migration

import (
	"fmt"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

type taskStatus20260918110353 struct {
	Status string `xorm:"varchar(20) not null default 'to-do'"`
}

func (taskStatus20260918110353) TableName() string { return "tasks" }

func addTaskStatus20260918110353(tx *xorm.Engine) error {
	if err := partialSync(tx, taskStatus20260918110353{}); err != nil {
		return fmt.Errorf("could not add tasks.status: %w", err)
	}
	if _, err := tx.Exec("UPDATE tasks SET status = ? WHERE done = ?", "done", true); err != nil {
		return fmt.Errorf("could not backfill completed task statuses: %w", err)
	}
	return nil
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260918110353",
		Description: "Add TaskTrace workflow status to tasks",
		Migrate:     addTaskStatus20260918110353,
		Rollback:    func(_ *xorm.Engine) error { return nil },
	})
}
