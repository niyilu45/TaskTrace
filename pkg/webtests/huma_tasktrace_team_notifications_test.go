// SPDX-License-Identifier: AGPL-3.0-or-later
package webtests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/models"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestHumaTaskTraceTeamNotificationsReadAndDelete(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	data, repository := t.TempDir(), t.TempDir()
	t.Setenv("TASKTRACE_DATA_ROOT", data)
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	binding := models.TaskTraceTeamBinding{Repository: repository, ShareID: "notification-http", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: 1, NodeTasks: map[string]int64{"root": 1}}
	write := func(path string, value interface{}) {
		b, marshalErr := json.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, b, 0o600))
	}
	write(filepath.Join(data, "team-sync.json"), map[string]interface{}{"schema": 1, "device_id": "test", "bindings": []models.TaskTraceTeamBinding{binding}})
	id, otherID := uuid.NewString(), uuid.NewString()
	path := func(username, noticeID string) string {
		return filepath.Join(repository, "tasks", binding.ShareID, "notifications", username, noticeID+".json")
	}
	write(path("user1", id), models.TaskTraceTeamNotification{ID: id, Actor: "user2", TaskTitle: "Keep history"})
	write(path("user1", otherID), models.TaskTraceTeamNotification{ID: otherID, Actor: "user2"})
	write(path("user2", id), models.TaskTraceTeamNotification{ID: id, Actor: "user1"})
	const base = "/api/v2/tasktrace/team/notifications/"
	body := `{"ids":["` + id + `"]}`
	for _, action := range []string{"read", "delete"} {
		rec := humaRequest(t, e, http.MethodPost, base+action, body, "", "")
		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	token := humaTokenFor(t, &testuser1)
	rec := humaRequest(t, e, http.MethodPost, base+"read", body, token, "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var status models.TaskTraceTeamStatus
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	require.Len(t, status.Notifications, 2)
	for _, n := range status.Notifications {
		require.Equal(t, n.ID == id, n.Read)
	}
	require.FileExists(t, path("user1", id))
	for _, action := range []string{"read", "delete"} {
		rec = humaRequest(t, e, http.MethodPost, base+action, `{"ids":["invalid"]}`, token, "")
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
		require.FileExists(t, path("user1", id))
	}
	rec = humaRequest(t, e, http.MethodPost, base+"delete", body, token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	require.Len(t, status.Notifications, 1)
	require.Equal(t, otherID, status.Notifications[0].ID)
	_, err = os.Stat(path("user1", id))
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, path("user2", id))
	rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasktrace/notifications/history", "", token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var history []models.TaskTraceNotificationHistoryEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &history))
	require.Len(t, history, 1)
	require.Equal(t, id, history[0].Team.ID)
	require.Equal(t, "Keep history", history[0].Team.TaskTitle)
	require.True(t, history[0].Team.Read)
	rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasktrace/notifications/history", "", humaTokenFor(t, &testuser2), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.JSONEq(t, "[]", rec.Body.String())

}
