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

func init() { AddRouteRegistrar(RegisterTaskTraceOutstandingRoutes) }

// RegisterTaskTraceOutstandingRoutes registers atomic shared-list moves.
func RegisterTaskTraceOutstandingRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "tasks-outstanding-move", Summary: "Move or reorder a shared outstanding item",
		Description: "Requires update access to both tasks in the same project. Source and destination lists change in one transaction. Referenced local attachment images are copied when moving to another task, so deleting the source task does not break them. A missing item, missing insertion anchor or conflicting edit returns 409 without moving anything.",
		Method:      http.MethodPost, Path: "/tasks/{task}/outstanding/move", Tags: []string{"tasks"},
	}, taskTraceOutstandingMove)
}
func taskTraceOutstandingMove(ctx context.Context, in *struct {
	TaskID int64 `path:"task" doc:"Source task containing the outstanding item."`
	Body   models.TaskTraceOutstandingMove
}) (*singleBody[models.TaskTraceOutstandingMove], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	move := &in.Body
	move.TaskID = in.TaskID
	if err := handler.DoCreate(ctx, move, a); err != nil {
		move.CleanupCreatedFiles()
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTraceOutstandingMove]{Body: move}, nil
}
