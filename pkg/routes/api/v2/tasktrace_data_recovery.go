// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/tasktracedata"

	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTraceDataRecoveryRoutes) }

func RegisterTaskTraceDataRecoveryRoutes(api huma.API) {
	tags := []string{"tasktrace-data-recovery"}
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-recovery-detect",
		Summary:     "Detect old TaskTrace data",
		Description: "Scans known local portable-app locations and optionally validates a supplied directory. The current live database is excluded.",
		Method:      http.MethodGet,
		Path:        "/tasktrace/data-recovery",
		Tags:        tags,
	}, taskTraceDataRecoveryDetect)
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-recovery-import",
		Summary:     "Prepare old TaskTrace data for import",
		Description: "Copies the selected old personal and team datasets into protected local import directories, verifies the copied database, backs up settings, and activates the import on the next restart. Existing data is not overwritten.",
		Method:      http.MethodPost,
		Path:        "/tasktrace/data-recovery/import",
		Tags:        tags,
	}, taskTraceDataRecoveryImport)
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-backup-settings-read",
		Summary:     "Read TaskTrace backup settings",
		Description: "Returns the local automatic-backup schedule, destination, retention period, and minimum retained backup count.",
		Method:      http.MethodGet,
		Path:        "/tasktrace/data-recovery/backup/settings",
		Tags:        tags,
	}, taskTraceDataBackupSettingsRead)
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-backup-settings-write",
		Summary:     "Update TaskTrace backup settings",
		Description: "Stores the local automatic-backup schedule and retention policy without changing current personal or team data.",
		Method:      http.MethodPut,
		Path:        "/tasktrace/data-recovery/backup/settings",
		Tags:        tags,
	}, taskTraceDataBackupSettingsWrite)
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-backup-status",
		Summary:     "Read TaskTrace backup status",
		Description: "Returns the resolved destination, backup count, last result, and next scheduled check.",
		Method:      http.MethodGet,
		Path:        "/tasktrace/data-recovery/backup/status",
		Tags:        tags,
	}, taskTraceDataBackupStatus)
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-backup-run",
		Summary:     "Run a TaskTrace backup check",
		Description: "Creates a consistent local backup of personal and team data when content changed, then removes expired backups while keeping the configured minimum count.",
		Method:      http.MethodPost,
		Path:        "/tasktrace/data-recovery/backup/run",
		Tags:        tags,
	}, taskTraceDataBackupRun)
}

func taskTraceDataBackupSettingsRead(ctx context.Context, _ *struct{}) (*singleBody[tasktracedata.TaskTraceBackupSettings], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	settings, err := tasktracedata.ReadBackupSettings()
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("read backup settings", err)
	}
	return &singleBody[tasktracedata.TaskTraceBackupSettings]{Body: &settings}, nil
}

func taskTraceDataBackupSettingsWrite(ctx context.Context, in *struct {
	Body tasktracedata.TaskTraceBackupSettings
}) (*singleBody[tasktracedata.TaskTraceBackupSettings], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	settings, err := tasktracedata.WriteBackupSettings(in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("save backup settings", err)
	}
	return &singleBody[tasktracedata.TaskTraceBackupSettings]{Body: &settings}, nil
}

func taskTraceDataBackupStatus(ctx context.Context, _ *struct{}) (*singleBody[tasktracedata.TaskTraceBackupStatus], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	status, err := tasktracedata.BackupStatus()
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("read backup status", err)
	}
	return &singleBody[tasktracedata.TaskTraceBackupStatus]{Body: &status}, nil
}

func taskTraceDataBackupRun(ctx context.Context, _ *struct{}) (*singleBody[tasktracedata.TaskTraceBackupRunResult], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	result, err := tasktracedata.RunBackup()
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("run data backup", err)
	}
	return &singleBody[tasktracedata.TaskTraceBackupRunResult]{Body: &result}, nil
}

func requireTaskTraceDataRecovery(ctx context.Context) error {
	if _, err := authFromCtx(ctx); err != nil {
		return err
	}
	if !tasktracedata.Enabled() {
		return huma.Error404NotFound("TaskTrace local data recovery is unavailable")
	}
	return nil
}

func taskTraceDataRecoveryDetect(ctx context.Context, in *struct {
	Path string `query:"path" doc:"Optional old program or data directory to validate in addition to automatic detection."`
}) (*singleBody[tasktracedata.TaskTraceDataDetection], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	detection, err := tasktracedata.Detect(in.Path)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("detect old TaskTrace data", err)
	}
	return &singleBody[tasktracedata.TaskTraceDataDetection]{Body: &detection}, nil
}

func taskTraceDataRecoveryImport(ctx context.Context, in *struct {
	Body tasktracedata.TaskTraceDataImportRequest
}) (*singleBody[tasktracedata.TaskTraceDataImportResult], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	result, err := tasktracedata.Import(in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("import old TaskTrace data", err)
	}
	return &singleBody[tasktracedata.TaskTraceDataImportResult]{Body: &result}, nil
}
