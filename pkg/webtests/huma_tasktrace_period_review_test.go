// SPDX-License-Identifier: AGPL-3.0-or-later
package webtests

import (
	"encoding/json"
	"net/http"
	"testing"

	"code.vikunja.io/api/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestHumaTaskTracePeriodReview(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	token := humaTokenFor(t, &testuser1)
	query := "?projects=1&from=2026-10-10T00:00:00%2B08:00&until=2026-10-17T00:00:00%2B08:00"
	for _, sample := range []struct {
		query, token string
		status       int
	}{{query, token, 200}, {query, "", 401}, {"?projects=2&from=2026-10-10T00:00:00Z&until=2026-10-17T00:00:00Z", token, 403}, {"?projects=1&from=invalid&until=invalid", token, 422}, {"?projects=1&from=2026-10-17T00:00:00Z&until=2026-10-10T00:00:00Z", token, 400}} {
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasktrace/review"+sample.query, "", sample.token, "")
		require.Equal(t, sample.status, rec.Code, rec.Body.String())
		if rec.Code == 200 {
			var result models.TaskTracePeriodReview
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &result))
			require.NotEmpty(t, result.Markdown)
		}
	}
}
