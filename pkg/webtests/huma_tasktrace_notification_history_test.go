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
	"net/http"
	"testing"

	"code.vikunja.io/api/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestHumaTaskTraceNotificationHistory(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	const base = "/api/v2/tasktrace/notifications/"
	for _, path := range []string{"history", "settings"} {
		rec := humaRequest(t, e, http.MethodGet, base+path, "", "", "")
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
	token := humaTokenFor(t, &testuser1)
	rec := humaRequest(t, e, http.MethodGet, base+"settings", "", token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"max_entries":100`)
	rec = humaRequest(t, e, http.MethodPut, base+"settings", `{"max_entries":2}`, token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, body := range []string{`{"max_entries":0}`, `{"max_entries":10001}`} {
		rec = humaRequest(t, e, http.MethodPut, base+"settings", body, token, "")
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	}
	rec = humaRequest(t, e, http.MethodGet, base+"settings", "", humaTokenFor(t, &testuser2), "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), `"max_entries":100`)
	rec = humaRequest(t, e, http.MethodGet, base+"history", "", token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var entries []models.TaskTraceNotificationHistoryEntry
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &entries))
	require.Empty(t, entries)
}
