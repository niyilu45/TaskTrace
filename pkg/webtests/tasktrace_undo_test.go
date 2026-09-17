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

package webtests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/models"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func undoRequest(t *testing.T, e *echo.Echo, method, path, body, token, group string, record bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if method == http.MethodPatch {
		req.Header.Set("Content-Type", "application/merge-patch+json")
	} else {
		req.Header.Set("Content-Type", "application/json")
	}
	if record {
		req.Header.Set("X-TaskTrace-Undo", "1")
	}
	if group != "" {
		req.Header.Set("X-TaskTrace-Undo-Group", group)
	}
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func undoStatus(t *testing.T, e *echo.Echo, token string) models.TaskTraceUndo {
	t.Helper()
	rec := undoRequest(t, e, http.MethodGet, "/api/v2/tasktrace/undo", "", token, "", false)
	require.Equal(t, 200, rec.Code, rec.Body.String())
	var status models.TaskTraceUndo
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &status))
	return status
}

func TestTaskTraceUndoHTTPAtomicGroup(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	token := humaTokenFor(t, &testuser1)
	group := uuid.NewString()
	for _, body := range []string{`{"title":"Tracked first"}`, `{"description":"Tracked second"}`} {
		changed := undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", body, token, group, true)
		require.Equal(t, 200, changed.Code, changed.Body.String())
	}
	latest := undoStatus(t, e, token)
	require.Equal(t, 1, latest.Count)
	changed := undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"title":"Newer unrecorded title"}`, token, "", false)
	require.Equal(t, 200, changed.Code, changed.Body.String())
	// The newest entry can restore its description, but the older title conflicts.
	// The handler must roll back both that partial restore and any journal changes.
	undone := undoRequest(t, e, http.MethodPost, "/api/v2/tasktrace/undo", fmt.Sprintf(`{"id":%d}`, latest.ID), token, "", false)
	require.Equal(t, 409, undone.Code, undone.Body.String())
	assert.Equal(t, latest.ID, undoStatus(t, e, token).ID)
	after := undoRequest(t, e, http.MethodGet, "/api/v2/tasks/1", "", token, "", false)
	require.Equal(t, 200, after.Code, after.Body.String())
	var preserved models.Task
	require.NoError(t, json.Unmarshal(after.Body.Bytes(), &preserved))
	assert.Equal(t, "Newer unrecorded title", preserved.Title)
	assert.Equal(t, "Tracked second", preserved.Description)
}
func TestTaskTraceUndoHTTP(t *testing.T) {
	t.Run("opt-in patch grouping and reverse restore", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		group := uuid.NewString()
		before := undoRequest(t, e, http.MethodGet, "/api/v2/tasks/1", "", token, "", false)
		require.Equal(t, 200, before.Code)
		var original models.Task
		require.NoError(t, json.Unmarshal(before.Body.Bytes(), &original))
		changed := undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"title":"Undo title"}`, token, group, true)
		require.Equal(t, 200, changed.Code, changed.Body.String())
		first := undoStatus(t, e, token)
		require.Equal(t, 1, first.Count)
		require.Positive(t, first.ID)
		changed = undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"description":"Undo description"}`, token, group, true)
		require.Equal(t, 200, changed.Code, changed.Body.String())
		latest := undoStatus(t, e, token)
		require.Equal(t, 1, latest.Count)
		assert.Greater(t, latest.ID, first.ID)
		stale := undoRequest(t, e, http.MethodPost, "/api/v2/tasktrace/undo", fmt.Sprintf(`{"id":%d}`, first.ID), token, "", false)
		require.Equal(t, 409, stale.Code, stale.Body.String())
		undone := undoRequest(t, e, http.MethodPost, "/api/v2/tasktrace/undo", fmt.Sprintf(`{"id":%d}`, latest.ID), token, "", true)
		require.Equal(t, 201, undone.Code, undone.Body.String())
		assert.Zero(t, undoStatus(t, e, token).Count)
		after := undoRequest(t, e, http.MethodGet, "/api/v2/tasks/1", "", token, "", false)
		var restored models.Task
		require.NoError(t, json.Unmarshal(after.Body.Bytes(), &restored))
		assert.Equal(t, original.Title, restored.Title)
		assert.Equal(t, original.Description, restored.Description)
	})
	t.Run("disabled and unsupported writes do not record", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		changed := undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"title":"Unrecorded title"}`, token, "", false)
		require.Equal(t, 200, changed.Code, changed.Body.String())
		assert.Zero(t, undoStatus(t, e, token).Count)
		label := undoRequest(t, e, http.MethodPost, "/api/v2/labels", `{"title":"Not an undo model"}`, token, "", true)
		require.Equal(t, 201, label.Code, label.Body.String())
		assert.Zero(t, undoStatus(t, e, token).Count)
	})
	t.Run("conflicting edit preserves data and history", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		changed := undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"title":"Tracked title"}`, token, "", true)
		require.Equal(t, 200, changed.Code, changed.Body.String())
		latest := undoStatus(t, e, token)
		changed = undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"title":"Newer edit"}`, token, "", false)
		require.Equal(t, 200, changed.Code, changed.Body.String())
		rec := undoRequest(t, e, http.MethodPost, "/api/v2/tasktrace/undo", fmt.Sprintf(`{"id":%d}`, latest.ID), token, "", false)
		require.Equal(t, 409, rec.Code, rec.Body.String())
		assert.Equal(t, latest.ID, undoStatus(t, e, token).ID)
		after := undoRequest(t, e, http.MethodGet, "/api/v2/tasks/1", "", token, "", false)
		assert.Contains(t, after.Body.String(), "Newer edit")
	})
	t.Run("another user cannot read or undo entries", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		owner := humaTokenFor(t, &testuser1)
		other := humaTokenFor(t, &testuser2)
		rec := undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"priority":5}`, owner, "", true)
		require.Equal(t, 200, rec.Code, rec.Body.String())
		latest := undoStatus(t, e, owner)
		require.Positive(t, latest.ID)
		assert.Zero(t, undoStatus(t, e, other).ID)
		rec = undoRequest(t, e, http.MethodPost, "/api/v2/tasktrace/undo", fmt.Sprintf(`{"id":%d}`, latest.ID), other, "", false)
		require.Equal(t, 409, rec.Code, rec.Body.String())
		assert.Equal(t, latest.ID, undoStatus(t, e, owner).ID)
		rec = undoRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"priority":9}`, other, "", true)
		require.Equal(t, 403, rec.Code, rec.Body.String())
		assert.Zero(t, undoStatus(t, e, other).Count)
	})
}
