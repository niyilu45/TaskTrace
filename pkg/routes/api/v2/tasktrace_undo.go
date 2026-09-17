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

package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTraceUndoRoutes) }

func RegisterTaskTraceUndoRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "tasktrace-undo-read",
		Summary:     "Read the current undo step",
		Description: "Returns only the authenticated user's newest undo step and number of retained action groups. History survives application restarts. Link shares cannot use undo.",
		Method:      http.MethodGet,
		Path:        "/tasktrace/undo",
		Tags:        []string{"tasks"},
	}, taskTraceUndoRead)
	Register(api, huma.Operation{
		OperationID: "tasktrace-undo-create",
		Summary:     "Undo the latest action group",
		Description: "Atomically restores the latest consecutive action group in reverse order. The supplied ID must still be the newest journal entry. Changed data, missing dependencies or revoked permissions reject the whole undo without overwriting newer edits. The undo itself is not recorded.",
		Method:      http.MethodPost,
		Path:        "/tasktrace/undo",
		Tags:        []string{"tasks"},
	}, taskTraceUndoCreate)
}

func taskTraceUndoRead(ctx context.Context, _ *struct{}) (*singleBody[models.TaskTraceUndo], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	state := &models.TaskTraceUndo{}
	if _, err := handler.DoReadOne(ctx, state, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTraceUndo]{Body: state}, nil
}

func taskTraceUndoCreate(ctx context.Context, in *struct {
	Body struct {
		ID int64 `json:"id" minimum:"1" doc:"Latest journal entry ID read from undo status."`
	}
}) (*singleBody[models.TaskTraceUndo], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	state := &models.TaskTraceUndo{ID: in.Body.ID}
	if err := handler.DoCreate(ctx, state, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTraceUndo]{Body: state}, nil
}
