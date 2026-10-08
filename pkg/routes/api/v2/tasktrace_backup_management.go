// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/tasktracedata"

	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskTraceBackupManagementRoutes) }

func RegisterTaskTraceBackupManagementRoutes(api huma.API) {
	tags := []string{"tasktrace-data-recovery"}
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-backups-list",
		Summary:     "List recoverable TaskTrace backups",
		Description: "Returns validated backups in the configured local backup directory, newest first, including their saved notes. Unrelated folders and live data are excluded.",
		Method:      http.MethodGet,
		Path:        "/tasktrace/data-recovery/backups",
		Tags:        tags,
	}, taskTraceDataBackupsList)
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-backup-note-write",
		Summary:     "Set a TaskTrace backup note",
		Description: "Replaces the selected backup's note without changing data, creation time or retention. Only identifiers from the currently configured backup directory are accepted.",
		Method:      http.MethodPut,
		Path:        "/tasktrace/data-recovery/backups/{id}/note",
		Tags:        tags,
	}, taskTraceDataBackupNoteWrite)
	Register(api, huma.Operation{
		OperationID: "tasktrace-data-backup-delete",
		Summary:     "Delete a TaskTrace backup",
		Description: "Permanently removes only the selected managed backup, including its personal data, team data and note. Manual deletion is independent of automatic minimum retention. Live data and linked directories cannot be deleted.",
		Method:      http.MethodDelete,
		Path:        "/tasktrace/data-recovery/backups/{id}",
		Tags:        tags,
	}, taskTraceDataBackupDelete)
}

func taskTraceDataBackupsList(ctx context.Context, _ *struct{}) (*singleBody[tasktracedata.TaskTraceBackupList], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	result, err := tasktracedata.ListRecoverableBackups()
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("list recoverable backups", err)
	}
	return &singleBody[tasktracedata.TaskTraceBackupList]{Body: &result}, nil
}

func taskTraceDataBackupNoteWrite(ctx context.Context, in *struct {
	ID   string `path:"id" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$" doc:"Opaque backup identifier returned by the backup list."`
	Body tasktracedata.TaskTraceBackupNote
}) (*singleBody[tasktracedata.TaskTraceBackupNote], error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	note, err := tasktracedata.SaveBackupNote(in.ID, in.Body)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("save backup note", err)
	}
	return &singleBody[tasktracedata.TaskTraceBackupNote]{Body: &note}, nil
}

func taskTraceDataBackupDelete(ctx context.Context, in *struct {
	ID string `path:"id" minLength:"64" maxLength:"64" pattern:"^[a-f0-9]{64}$" doc:"Opaque backup identifier returned by the backup list."`
}) (*emptyBody, error) {
	if err := requireTaskTraceDataRecovery(ctx); err != nil {
		return nil, err
	}
	if err := tasktracedata.DeleteBackup(in.ID); err != nil {
		return nil, huma.Error422UnprocessableEntity("delete backup", err)
	}
	return &emptyBody{}, nil
}
