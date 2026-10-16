// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceExportNotifications(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{
		ID:       1,
		Username: "user1",
	}
	other := &user.User{
		ID:       2,
		Username: "user2",
	}
	path := filepath.Join(t.TempDir(), "export.md")
	notice, err := TaskTraceRecordExport(s, actor, path)
	require.NoError(t, err)
	require.Equal(t, TaskTraceExportNotificationName, notice.Name)
	stored, err := TaskTraceExportLocation(s, actor, notice.ID)
	require.NoError(t, err)
	require.Equal(t, path, stored)
	_, err = TaskTraceExportLocation(s, other, notice.ID)
	require.Error(t, err)
	_, err = TaskTraceExportLocation(s, &LinkSharing{ID: 1}, notice.ID)
	require.Error(t, err)
	failed, err := TaskTraceRecordExportFailure(s, actor, "failed.md", "Access denied")
	require.NoError(t, err)
	require.Equal(t, TaskTraceExportFailedNotificationName, failed.Name)
	_, err = TaskTraceExportLocation(s, actor, failed.ID)
	require.Error(t, err)
	var payload TaskTraceExportNotification
	require.NoError(t, json.Unmarshal(failed.Notification.(json.RawMessage), &payload))
	require.Equal(t, TaskTraceExportFailedNotificationName, payload.Name())
	require.Empty(t, payload.Path)
	require.Contains(t, payload.ToTitle(""), "failed.md")
	require.NoError(t, (&DatabaseNotifications{DatabaseNotification: notifications.DatabaseNotification{ID: notice.ID}}).Delete(s, actor))
	stored, err = TaskTraceExportLocation(s, actor, notice.ID)
	require.NoError(t, err)
	require.Equal(t, path, stored, "cleared success notices still locate files from history")
	_, err = TaskTraceExportLocation(s, other, notice.ID)
	require.Error(t, err)
	_, err = TaskTraceRecordExport(s, &LinkSharing{ID: 1}, path)
	require.Error(t, err)
}
