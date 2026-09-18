// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.

package migration

import (
	"testing"

	"code.vikunja.io/api/pkg/db"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

type taskStatusBefore20260918110353 struct {
	ID        int64  `xorm:"bigint autoincr not null unique pk"`
	ProjectID int64  `xorm:"bigint not null index"`
	Title     string `xorm:"TEXT not null"`
	Done      bool   `xorm:"INDEX null"`
}

func (taskStatusBefore20260918110353) TableName() string { return "tasks" }

type taskStatusAfter20260918110353 struct {
	ID     int64  `xorm:"bigint autoincr not null unique pk"`
	Done   bool   `xorm:"INDEX null"`
	Status string `xorm:"varchar(20) not null default 'to-do'"`
}

func (taskStatusAfter20260918110353) TableName() string { return "tasks" }

func TestAddTaskStatus20260918110353(t *testing.T) {
	x, err := db.CreateTestEngine()
	require.NoError(t, err)
	table := taskStatusBefore20260918110353{}
	t.Cleanup(func() { require.NoError(t, x.DropTables(table)) })
	require.NoError(t, x.DropTables(table))
	require.NoError(t, x.Sync2(table))
	_, err = x.Insert(&taskStatusBefore20260918110353{ID: 1, ProjectID: 1, Title: "Open", Done: false}, &taskStatusBefore20260918110353{ID: 2, ProjectID: 1, Title: "Closed", Done: true})
	require.NoError(t, err)

	before := taskStatusTable20260918110353(t, x)
	require.NoError(t, addTaskStatus20260918110353(x))
	after := taskStatusTable20260918110353(t, x)
	require.NotNil(t, after.GetColumn("status"))
	for name, index := range before.Indexes {
		preserved, found := after.Indexes[name]
		require.Truef(t, found, "migration dropped index %s", name)
		require.Equal(t, index.Type, preserved.Type)
		require.Equal(t, index.Cols, preserved.Cols)
	}
	rows := []taskStatusAfter20260918110353{}
	require.NoError(t, x.Asc("id").Find(&rows))
	require.Equal(t, []string{"to-do", "done"}, []string{rows[0].Status, rows[1].Status})
}

func taskStatusTable20260918110353(t *testing.T, x *xorm.Engine) *schemas.Table {
	t.Helper()
	tables, err := x.DBMetas()
	require.NoError(t, err)
	for _, table := range tables {
		if table.Name == "tasks" {
			return table
		}
	}
	t.Fatal("tasks table not found")
	return nil
}
