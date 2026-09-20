// SPDX-License-Identifier: AGPL-3.0-or-later

package webtests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHumaTaskTraceTeamMembersRequireAuthentication(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/api/v2/tasktrace/team/members/search?q=user", ""},
		{http.MethodPost, "/api/v2/tasktrace/team/members/access", `{"account_name":"user"}`},
		{http.MethodDelete, "/api/v2/tasktrace/team/members/access/user", ""},
		{http.MethodPost, "/api/v2/tasktrace/team/members/import", `{"link":"invalid"}`},
	}

	for _, tt := range tests {
		rec := humaRequest(t, e, tt.method, tt.path, tt.body, "", "")
		require.Equal(t, http.StatusUnauthorized, rec.Code, "%s %s body: %s", tt.method, tt.path, rec.Body.String())
	}
}
