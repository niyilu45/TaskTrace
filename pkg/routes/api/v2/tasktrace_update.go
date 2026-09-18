// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"net/http"
	"strings"

	"code.vikunja.io/api/pkg/tasktraceupdate"

	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTraceUpdateRoutes) }

func RegisterTaskTraceUpdateRoutes(api huma.API) {
	tags := []string{"tasktrace-update"}
	Register(api, huma.Operation{OperationID: "tasktrace-update-status", Summary: "Read TaskTrace update status", Description: "Returns the release information last written by the Windows desktop process, including whether the user should be notified.", Method: http.MethodGet, Path: "/tasktrace/update", Tags: tags}, taskTraceUpdateStatus)
	Register(api, huma.Operation{OperationID: "tasktrace-update-settings-read", Summary: "Read TaskTrace update settings", Description: "Returns the shared portable-app update interval and ignored release version.", Method: http.MethodGet, Path: "/tasktrace/update/settings", Tags: tags}, taskTraceUpdateSettingsRead)
	Register(api, huma.Operation{OperationID: "tasktrace-update-settings-write", Summary: "Save TaskTrace update settings", Description: "Replaces the shared update settings. The desktop process observes the saved interval without restarting.", Method: http.MethodPut, Path: "/tasktrace/update/settings", Tags: tags}, taskTraceUpdateSettingsWrite)
	Register(api, huma.Operation{OperationID: "tasktrace-update-check", Summary: "Request a GitHub release check", Description: "Queues a check for the Windows desktop process, which uses the current user's Windows system proxy.", Method: http.MethodPost, Path: "/tasktrace/update/check", Tags: tags}, taskTraceUpdateCheck)
	Register(api, huma.Operation{OperationID: "tasktrace-update-ignore", Summary: "Ignore a TaskTrace release", Description: "Suppresses future automatic notifications for one release version while preserving manual checks.", Method: http.MethodPost, Path: "/tasktrace/update/ignore", Tags: tags}, taskTraceUpdateIgnore)
	Register(api, huma.Operation{OperationID: "tasktrace-update-install", Summary: "Request TaskTrace update installation", Description: "Queues download and installation for the desktop process after the user has agreed to close the running application.", Method: http.MethodPost, Path: "/tasktrace/update/install", Tags: tags}, taskTraceUpdateInstall)
}

func requireTaskTraceUpdate(ctx context.Context) error {
	if _, err := authFromCtx(ctx); err != nil {
		return err
	}
	if !tasktraceupdate.Enabled() {
		return huma.Error404NotFound("TaskTrace local updates are unavailable")
	}
	return nil
}

func taskTraceUpdateStatus(ctx context.Context, _ *struct{}) (*singleBody[tasktraceupdate.State], error) {
	if err := requireTaskTraceUpdate(ctx); err != nil {
		return nil, err
	}
	state, err := tasktraceupdate.ReadState()
	if err != nil {
		return nil, huma.Error500InternalServerError("read update status", err)
	}
	return &singleBody[tasktraceupdate.State]{Body: &state}, nil
}

func taskTraceUpdateSettingsRead(ctx context.Context, _ *struct{}) (*singleBody[tasktraceupdate.Settings], error) {
	if err := requireTaskTraceUpdate(ctx); err != nil {
		return nil, err
	}
	settings, err := tasktraceupdate.ReadSettings()
	if err != nil {
		return nil, huma.Error500InternalServerError("read update settings", err)
	}
	return &singleBody[tasktraceupdate.Settings]{Body: &settings}, nil
}

func taskTraceUpdateSettingsWrite(ctx context.Context, in *struct{ Body tasktraceupdate.Settings }) (*singleBody[tasktraceupdate.Settings], error) {
	if err := requireTaskTraceUpdate(ctx); err != nil {
		return nil, err
	}
	settings, err := tasktraceupdate.WriteSettings(in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("invalid update settings", err)
	}
	return &singleBody[tasktraceupdate.Settings]{Body: &settings}, nil
}

func taskTraceUpdateCheck(ctx context.Context, _ *struct{}) (*singleBody[tasktraceupdate.Command], error) {
	if err := requireTaskTraceUpdate(ctx); err != nil {
		return nil, err
	}
	command, err := tasktraceupdate.Queue("check", "")
	if err != nil {
		return nil, huma.Error500InternalServerError("queue update check", err)
	}
	return &singleBody[tasktraceupdate.Command]{Body: &command}, nil
}

func taskTraceUpdateIgnore(ctx context.Context, in *struct {
	Body struct {
		Version string `json:"version" minLength:"1" doc:"Release version the user declined."`
	}
}) (*singleBody[tasktraceupdate.State], error) {
	if err := requireTaskTraceUpdate(ctx); err != nil {
		return nil, err
	}
	_, state, err := tasktraceupdate.Ignore(in.Body.Version)
	if err != nil {
		return nil, huma.Error500InternalServerError("ignore update", err)
	}
	return &singleBody[tasktraceupdate.State]{Body: &state}, nil
}

func taskTraceUpdateInstall(ctx context.Context, in *struct {
	Body struct {
		Version string `json:"version" minLength:"1" doc:"Release version confirmed by the user."`
	}
}) (*singleBody[tasktraceupdate.Command], error) {
	if err := requireTaskTraceUpdate(ctx); err != nil {
		return nil, err
	}
	state, err := tasktraceupdate.ReadState()
	if err != nil {
		return nil, huma.Error500InternalServerError("read update status", err)
	}
	requested := strings.TrimSpace(in.Body.Version)
	if !state.Available || requested == "" || requested != state.LatestVersion || state.AssetURL == "" {
		return nil, huma.Error409Conflict("the selected release is no longer ready to install")
	}
	command, err := tasktraceupdate.Queue("install", requested)
	if err != nil {
		return nil, huma.Error500InternalServerError("queue update installation", err)
	}
	return &singleBody[tasktraceupdate.Command]{Body: &command}, nil
}
