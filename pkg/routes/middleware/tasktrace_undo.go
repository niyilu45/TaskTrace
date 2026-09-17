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
	"strings"

	"code.vikunja.io/api/pkg/web"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"
)

func TaskTraceUndo() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			req := c.Request()
			if req.Header.Get("X-TaskTrace-Undo") != "1" {
				return next(c)
			}
			group := strings.TrimSpace(req.Header.Get("X-TaskTrace-Undo-Group"))
			if group != "" {
				parsed, err := uuid.Parse(group)
				if err != nil {
					return echo.NewHTTPError(http.StatusBadRequest, "撤销分组标识无效，请重试。")
				}
				group = parsed.String()
			}
			c.SetRequest(req.WithContext(web.WithTaskTraceUndo(req.Context(), group)))
			return next(c)
		}
	}
}
