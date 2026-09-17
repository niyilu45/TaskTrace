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

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"code.vikunja.io/api/pkg/web"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
)

func TestTaskTraceUndoHeaders(t *testing.T) {
	for _, test := range []struct {
		name, optIn, group string
		status             int
		enabled            bool
	}{
		{
			"absent",
			"",
			"",
			204,
			false,
		},
		{
			"disabled",
			"0",
			"invalid",
			204,
			false,
		},
		{
			"enabled",
			"1",
			"",
			204,
			true,
		},
		{
			"grouped",
			"1",
			uuid.NewString(),
			204,
			true,
		},
		{
			"bad group",
			"1",
			"invalid",
			400,
			false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()
			e.Use(TaskTraceUndo())
			e.POST("/test", func(c *echo.Context) error {
				options := web.TaskTraceUndoFromContext(c.Request().Context())
				assert.Equal(t, test.enabled, options.Enabled)
				if test.enabled {
					assert.Equal(t, test.group, options.GroupID)
				} else {
					assert.Empty(t, options.GroupID)
				}
				return c.NoContent(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set("X-TaskTrace-Undo", test.optIn)
			req.Header.Set("X-TaskTrace-Undo-Group", test.group)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			assert.Equal(t, test.status, rec.Code)
		})
	}
}
