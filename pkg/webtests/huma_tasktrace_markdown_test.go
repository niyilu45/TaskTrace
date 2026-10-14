// SPDX-License-Identifier: AGPL-3.0-or-later
package webtests

import (
	"encoding/json"
	"net/http"
	"testing"

	"code.vikunja.io/api/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestHumaTaskTraceMarkdown(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	token := humaTokenFor(t, &testuser1)
	for _, sample := range []struct {
		path, auth string
		status     int
	}{{"/api/v2/tasks/1/markdown", token, http.StatusOK}, {"/api/v2/tasks/1/markdown", "", http.StatusUnauthorized}, {"/api/v2/tasks/999999/markdown", token, http.StatusNotFound}, {"/api/v2/tasks/14/markdown", token, http.StatusForbidden}} {
		rec := humaRequest(t, e, http.MethodGet, sample.path, "", sample.auth, "")
		require.Equal(t, sample.status, rec.Code, rec.Body.String())
		if rec.Code == http.StatusOK {
			var result models.TaskTraceMarkdownExport
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.NotEmpty(t, result.Markdown)
			require.Contains(t, result.Filename, ".md")
		}
	}
}
