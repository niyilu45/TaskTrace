// SPDX-License-Identifier: AGPL-3.0-or-later
//
//nolint:gosmopolitan // These desktop-only messages follow the Chinese TaskTrace UI.
package apiv2

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/modules/humabridge"
	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/tasktraceexport"
	"code.vikunja.io/api/pkg/web"
	"github.com/danielgtaylor/huma/v2"
)

type taskTraceExportResult struct {
	Cancelled    bool                                `json:"cancelled" doc:"True when the native file picker was cancelled; no file or notification was created."`
	Path         string                              `json:"path,omitempty" doc:"Successfully saved local file path."`
	Notification *notifications.DatabaseNotification `json:"notification,omitempty" doc:"Persistent success or failure notification for the current user."`
	Error        string                              `json:"error,omitempty" doc:"Save failed; no success notification was created. The failure notification is returned when available."`
	Warning      string                              `json:"warning,omitempty" doc:"The file was saved but its notification could not be persisted."`
}

func init() { AddRouteRegistrar(RegisterTaskTraceExportRoutes) }
func RegisterTaskTraceExportRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID:  "tasktrace-export-markdown",
		Summary:      "Save Markdown with the native file picker",
		Description:  "Windows portable mode, authenticated local-loopback requests only. The native dialog selects and confirms the destination; arbitrary browser paths are not accepted. Creates an account-scoped success or failure notification after saving; cancellation creates no notification.",
		Method:       http.MethodPost,
		Path:         "/tasktrace/exports/markdown",
		MaxBodyBytes: 128 << 20,
		Tags:         []string{"tasktrace-exports"},
	}, taskTraceExportMarkdown)
	Register(api, huma.Operation{
		OperationID: "tasktrace-export-reveal",
		Summary:     "Locate a saved export in Explorer",
		Description: "Opens the saved file's folder and selects the file using an owned current or retained historical export notification. Local Windows portable requests only; the caller cannot supply a file path.",
		Method:      http.MethodPost,
		Path:        "/tasktrace/exports/{notification}/reveal",
		Tags:        []string{"tasktrace-exports"},
	}, taskTraceExportReveal)
}

//nolint:gosmopolitan // Native-only endpoint errors match the Chinese portable UI.
func requireTaskTraceDesktopExport(ctx context.Context) (web.Auth, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	if _, shared := a.(*models.LinkSharing); shared {
		return nil, huma.Error403Forbidden("导出定位只允许本机账户使用")
	}
	c := humabridge.EchoContextFrom(ctx)
	if c == nil {
		return nil, huma.Error403Forbidden("请在本机打开 TaskTrace 后导出")
	}
	host, _, err := net.SplitHostPort(c.Request().RemoteAddr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		return nil, huma.Error403Forbidden("请在本机打开 TaskTrace 后导出")
	}
	if !tasktraceexport.Supported || strings.TrimSpace(os.Getenv("TASKTRACE_DATA_ROOT")) == "" {
		return nil, huma.Error404NotFound("此安装不支持系统保存窗口")
	}
	return a, nil
}
func taskTraceExportMarkdown(ctx context.Context, in *struct {
	Body struct {
		Filename string `json:"filename" minLength:"1" maxLength:"250" doc:"Suggested Markdown file name; the destination is chosen in the native dialog."`
		Markdown string `json:"markdown" maxLength:"134217728" doc:"The already displayed Markdown snapshot, including embedded images."`
	}
}) (*singleBody[taskTraceExportResult], error) {
	a, err := requireTaskTraceDesktopExport(ctx)
	if err != nil {
		return nil, err
	}
	path, err := tasktraceexport.Save(ctx, in.Body.Filename, in.Body.Markdown)
	if errors.Is(err, tasktraceexport.ErrBusy) {
		return nil, huma.Error409Conflict(err.Error())
	}
	result := &taskTraceExportResult{
		Cancelled: path == "" && err == nil,
		Path:      path,
	}
	if result.Cancelled {
		return &singleBody[taskTraceExportResult]{Body: result}, nil
	}
	s := db.NewSession()
	defer s.Close()
	var notice *notifications.DatabaseNotification
	var recordErr error
	if err != nil {
		result.Error = err.Error()
		notice, recordErr = models.TaskTraceRecordExportFailure(s, a, in.Body.Filename, result.Error)
	} else {
		notice, recordErr = models.TaskTraceRecordExport(s, a, path)
	}
	if recordErr == nil {
		recordErr = s.Commit()
	}
	if recordErr != nil {
		if result.Error == "" {
			result.Warning = "文件已保存，但未能添加成功通知。保存位置：" + path
		}
	} else {
		result.Notification = notice
	}
	return &singleBody[taskTraceExportResult]{Body: result}, nil
}
func taskTraceExportReveal(ctx context.Context, in *struct {
	ID int64 `path:"notification" minimum:"1"`
}) (*emptyBody, error) {
	a, err := requireTaskTraceDesktopExport(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewReadSession()
	defer s.Close()
	path, err := models.TaskTraceExportLocation(s, a, in.ID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	if err = tasktraceexport.Reveal(path); err != nil {
		return nil, huma.Error409Conflict(err.Error())
	}
	return &emptyBody{}, nil
}
