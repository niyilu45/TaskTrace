// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"
	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTraceMarkdownRoutes) }
func RegisterTaskTraceMarkdownRoutes(api huma.API) {
	Register(api, huma.Operation{OperationID: "tasks-tasktrace-markdown", Summary: "Export a task as Markdown", Description: "Read-only GFM snapshot of this task and readable descendants, daily progress, and outstanding items. Images are embedded for offline use; unreadable descendants are omitted.", Method: http.MethodGet, Path: "/tasks/{task}/markdown", Tags: []string{"tasks"}}, tasksTaskTraceMarkdown)
}
func tasksTaskTraceMarkdown(ctx context.Context, in *struct {
	TaskID int64 `path:"task" doc:"Root task of the exported subtree."`
}) (*singleBody[models.TaskTraceMarkdownExport], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	m := &models.TaskTraceMarkdownExport{TaskID: in.TaskID}
	if _, err = handler.DoReadOne(ctx, m, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTraceMarkdownExport]{Body: m}, nil
}
