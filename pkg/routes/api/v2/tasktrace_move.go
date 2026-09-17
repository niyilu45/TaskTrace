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
	"fmt"
	"net/http"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTraceMoveRoutes) }

func RegisterTaskTraceMoveRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "tasks-tasktrace-move",
		Summary:     "Move and reorder a task",
		Description: "Atomically changes a task's parent within its project and its manual order in an existing project view. Requires write access to the task and all old/new parents. Cycles, depths greater than five, and invalid sibling anchors are rejected without changing data. View filters do not affect manual ordering.",
		Method:      http.MethodPost,
		Path:        "/tasks/{task}/move",
		Tags:        []string{"tasks"},
	}, tasksTaskTraceMove)
	Register(api, huma.Operation{
		OperationID: "projects-views-tasktrace-positions",
		Summary:     "Read unfiltered task positions",
		Description: "Returns stored manual positions in an existing project view, including completed tasks and tasks hidden by view filters. Requires read access to the project. Missing position rows are not created.",
		Method:      http.MethodGet,
		Path:        "/projects/{project}/views/{view}/tasktrace-positions",
		Tags:        []string{"projects"},
	}, taskTracePositionsReadAll)
}

func tasksTaskTraceMove(ctx context.Context, in *struct {
	TaskID int64 `path:"task" doc:"Task to move."`
	Body   models.TaskTraceMove
}) (*singleBody[models.TaskTraceMove], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	m := &in.Body
	m.TaskID = in.TaskID
	if err := handler.DoCreate(ctx, m, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTraceMove]{Body: m}, nil
}

type taskTracePositionsListBody struct {
	Body Paginated[*models.TaskPosition]
}

func taskTracePositionsReadAll(ctx context.Context, in *struct {
	ProjectID int64 `path:"project" doc:"Project owning the view."`
	ViewID    int64 `path:"view" doc:"Existing project view whose positions to read."`
	ListParams
}) (*taskTracePositionsListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	p := &models.TaskTracePositions{
		ProjectID:     in.ProjectID,
		ProjectViewID: in.ViewID,
	}
	result, _, total, err := handler.DoReadAll(ctx, p, a, "", in.Page, in.PerPage)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, ok := result.([]*models.TaskPosition)
	if !ok {
		return nil, fmt.Errorf("TaskTracePositions.ReadAll returned unexpected type %T", result)
	}
	return &taskTracePositionsListBody{Body: NewPaginated(items, total, in.Page, in.PerPage)}, nil
}
