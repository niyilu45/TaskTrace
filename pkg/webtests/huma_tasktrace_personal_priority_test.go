// SPDX-License-Identifier: AGPL-3.0-or-later

package webtests

import (
	"encoding/json"
	"net/http"
	"testing"

	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestHumaTaskTracePersonalPriority(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	t.Setenv("TASKTRACE_TEAM_ROOT", t.TempDir())
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	path := "/api/v2/tasktrace/tasks/1/priority"
	rec := humaRequest(t, e, http.MethodPut, path, `{"priority":2}`, "", "")
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	owner := humaTokenFor(t, &user.User{ID: 1, Username: "user1"})
	other := humaTokenFor(t, &user.User{ID: 2, Username: "user2"})
	rec = humaRequest(t, e, http.MethodPut, path, `{"priority":2}`, other, "")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	rec = humaRequest(t, e, http.MethodPut, path, `{"task_id":2,"priority":2}`, owner, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var saved struct {
		TaskID         int64 `json:"task_id"`
		Priority       int64 `json:"priority"`
		StoredPriority int64 `json:"stored_priority"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &saved))
	require.Equal(t, int64(1), saved.TaskID)
	require.Equal(t, int64(2), saved.Priority)
	require.Equal(t, int64(8), saved.StoredPriority)
	rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/1", "", owner, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var task struct {
		Title    string `json:"title"`
		Priority int64  `json:"priority"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &task))
	require.Equal(t, "task #1", task.Title)
	require.Equal(t, int64(8), task.Priority)
	rec = humaRequest(t, e, http.MethodPut, path, `{"priority":10}`, owner, "")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = humaRequest(t, e, http.MethodPut, path, `{"priority":1,"outstanding_id":"missing"}`, owner, "")
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
}
