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

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTraceNotificationHistoryRoutes) }
func RegisterTaskTraceNotificationHistoryRoutes(api huma.API) {
	tags := []string{"notifications"}
	Register(api, huma.Operation{OperationID: "tasktrace-notification-history-list", Summary: "Read cleared notification history", Description: "Returns the current user's retained ordinary and collaboration notifications, newest archive first. Clearing moves notices here and does not create unread badges.", Method: http.MethodGet, Path: "/tasktrace/notifications/history", Tags: tags}, taskTraceNotificationHistoryList)
	Register(api, huma.Operation{OperationID: "tasktrace-notification-settings-read", Summary: "Read notification history settings", Description: "The per-user local history limit defaults to 100 notifications.", Method: http.MethodGet, Path: "/tasktrace/notifications/settings", Tags: tags}, taskTraceNotificationSettingsRead)
	Register(api, huma.Operation{OperationID: "tasktrace-notification-settings-write", Summary: "Save notification history settings", Description: "Sets the per-user history limit and immediately removes the oldest archived notices beyond that limit. Active notifications are unaffected.", Method: http.MethodPut, Path: "/tasktrace/notifications/settings", Tags: tags}, taskTraceNotificationSettingsWrite)
}

type taskTraceNotificationHistoryBody struct {
	Body []models.TaskTraceNotificationHistoryEntry
}

func taskTraceNotificationHistoryList(ctx context.Context, _ *struct{}) (*taskTraceNotificationHistoryBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewReadSession()
	defer s.Close()
	entries, err := models.TaskTraceNotificationHistory(s, a)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &taskTraceNotificationHistoryBody{Body: entries}, nil
}
func taskTraceNotificationSettingsRead(ctx context.Context, _ *struct{}) (*singleBody[models.TaskTraceNotificationHistorySettings], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := models.TaskTraceNotificationSettings(a, nil)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTraceNotificationHistorySettings]{Body: settings}, nil
}
func taskTraceNotificationSettingsWrite(ctx context.Context, in *struct {
	Body models.TaskTraceNotificationHistorySettings
}) (*singleBody[models.TaskTraceNotificationHistorySettings], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := models.TaskTraceNotificationSettings(a, &in.Body)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskTraceNotificationHistorySettings]{Body: settings}, nil
}
