// SPDX-License-Identifier: AGPL-3.0-or-later
package webtests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHumaTaskTraceExports(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	token := humaTokenFor(t, &testuser1)
	body := `{"filename":"report.md","markdown":"# Review"}`
	rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasktrace/exports/markdown", body, "", "")
	require.Equal(t, 401, rec.Code)
	// Non-loopback browser requests cannot open dialogs on the server's desktop.
	rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasktrace/exports/markdown", body, token, "")
	require.Equal(t, 403, rec.Code)
	rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasktrace/exports/1/reveal", "", token, "")
	require.Equal(t, 403, rec.Code)
	t.Setenv("TASKTRACE_DATA_ROOT", "")
	req := httptest.NewRequest(http.MethodPost, "/api/v2/tasktrace/exports/markdown", strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:50000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	require.Equal(t, 404, rec.Code, "non-portable installs cannot run desktop commands")
}
