// SPDX-License-Identifier: AGPL-3.0-or-later

package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/models"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestMCPLazySchemas(t *testing.T) {
	m, err := NewLazy(newTestAPI(t), nil)
	require.NoError(t, err)
	require.NotEmpty(t, m.order)
	_ = m.ConnectionInfo()
	require.Nil(t, m.order[0].spec)

	e := echo.New()
	newRequest := func() (*echo.Context, *httptest.ResponseRecorder) {
		body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`
		req := httptest.NewRequest(http.MethodPost, RoutePrefix, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rec := httptest.NewRecorder()
		return e.NewContext(req, rec), rec
	}
	ctx, _ := newRequest()
	err = m.handler(ctx)
	require.ErrorContains(t, err, "MCP requires an API token")
	require.Nil(t, m.order[0].spec, "unauthorized calls must not build schemas")

	ctx, rec := newRequest()
	ctx.Set("api_token", &models.APIToken{APIPermissions: models.APIPermissions{"mcp": {"access"}}})
	m.authorize = func(*models.APIToken, string, string) bool { return true }
	require.NoError(t, m.handler(ctx))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	spec := m.order[0].spec
	require.NotNil(t, spec)
	require.NoError(t, m.prepareTools())
	require.Same(t, spec, m.order[0].spec)
}
