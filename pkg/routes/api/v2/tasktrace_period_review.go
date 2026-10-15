// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"net/http"
	"time"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"
	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTracePeriodReviewRoutes) }
func RegisterTaskTracePeriodReviewRoutes(api huma.API) {
	Register(api, huma.Operation{OperationID: "tasktrace-period-review", Summary: "Review changes over a date range", Description: "Read-only Markdown summary of saved daily progress, newly added outstanding items and changed notes in selected readable projects. From is inclusive and until exclusive. Historical activity without field snapshots is explicitly labeled; images are embedded for offline export.", Method: http.MethodGet, Path: "/tasktrace/review", Tags: []string{"tasks"}}, taskTracePeriodReview)
}
func taskTracePeriodReview(ctx context.Context, in *struct {
	Projects []int64   `query:"projects" minItems:"1" maxItems:"5000" doc:"Comma-separated project IDs; access is checked for every project."`
	From     time.Time `query:"from" required:"true" doc:"Inclusive local day start with UTC offset."`
	Until    time.Time `query:"until" required:"true" doc:"Exclusive next-day start with UTC offset."`
}) (*singleBody[models.TaskTracePeriodReview], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	result := &models.TaskTracePeriodReview{ProjectIDs: in.Projects, From: in.From, Until: in.Until}
	if _, err = handler.DoReadOne(ctx, result, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTracePeriodReview]{Body: result}, nil
}
