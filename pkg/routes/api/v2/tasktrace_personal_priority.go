// SPDX-License-Identifier: AGPL-3.0-or-later

package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"
	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTracePersonalPriorityRoutes) }
func RegisterTaskTracePersonalPriorityRoutes(api huma.API) {
	Register(api, huma.Operation{OperationID: "tasks-personal-priority-update", Summary: "Set a local personal task or outstanding priority", Description: "Changes only this local copy's priority. Shared task read-only permissions do not restrict this personal preference; local project write access is still required. No shared content, comments or notifications are modified.", Method: http.MethodPut, Path: "/tasktrace/tasks/{task}/priority", Tags: []string{"tasktrace-team"}}, taskTracePersonalPriorityUpdate)
}
func taskTracePersonalPriorityUpdate(ctx context.Context, in *struct {
	TaskID int64 `path:"task"`
	Body   models.TaskTracePersonalPriority
}) (*singleBody[models.TaskTracePersonalPriority], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	value := &in.Body
	value.TaskID = in.TaskID
	if err := handler.DoUpdate(ctx, value, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTracePersonalPriority]{Body: value}, nil
}
