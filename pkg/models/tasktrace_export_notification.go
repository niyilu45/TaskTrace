// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"encoding/json"
	"errors"
	"path/filepath"

	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

const TaskTraceExportNotificationName = "tasktrace.export.completed"
const TaskTraceExportFailedNotificationName = "tasktrace.export.failed"

type TaskTraceExportNotification struct {
	Filename string `json:"filename" doc:"Selected or suggested Markdown file name."`
	Error    string `json:"error,omitempty" doc:"Failure reason; absent on successful exports."`
	Path     string `json:"path,omitempty" doc:"Native file-picker destination, available only to the exporting account."`
}

func init() {
	notifications.Register(func() notifications.PersistedNotification { return &TaskTraceExportNotification{} })
	notifications.Register(func() notifications.PersistedNotification { return &TaskTraceExportNotification{Error: "failed"} })
}
func (n *TaskTraceExportNotification) Name() string {
	if n.Error != "" {
		return TaskTraceExportFailedNotificationName
	}
	return TaskTraceExportNotificationName
}
func (n *TaskTraceExportNotification) ProjectID() int64                  { return 0 }
func (n *TaskTraceExportNotification) ToDB() interface{}                 { return n }
func (n *TaskTraceExportNotification) ToMail(string) *notifications.Mail { return nil }

//nolint:gosmopolitan // Local export notices follow the TaskTrace UI language.
func (n *TaskTraceExportNotification) ToTitle(string) string {
	if n.Error != "" {
		return "导出失败：" + n.Filename
	}
	return "导出成功：" + n.Filename
}

func TaskTraceRecordExport(s *xorm.Session, a web.Auth, path string) (*notifications.DatabaseNotification, error) {
	return taskTraceRecordExportNotification(s, a, &TaskTraceExportNotification{
		Filename: filepath.Base(path),
		Path:     path,
	})
}
func TaskTraceRecordExportFailure(s *xorm.Session, a web.Auth, filename, reason string) (*notifications.DatabaseNotification, error) {
	return taskTraceRecordExportNotification(s, a, &TaskTraceExportNotification{
		Filename: filepath.Base(filename),
		Error:    reason,
	})
}
func taskTraceRecordExportNotification(s *xorm.Session, a web.Auth, payload *TaskTraceExportNotification) (*notifications.DatabaseNotification, error) {
	actor, err := taskTraceNotificationHistoryUser(a)
	if err != nil {
		return nil, err
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	notice := &notifications.DatabaseNotification{
		Name:         payload.Name(),
		NotifiableID: actor.ID,
		Notification: json.RawMessage(content),
	}
	_, err = s.Insert(notice)
	return notice, err
}

//nolint:gosmopolitan // Local Windows export messages follow the TaskTrace UI language.
func TaskTraceExportLocation(s *xorm.Session, a web.Auth, id int64) (string, error) {
	actor, err := taskTraceNotificationHistoryUser(a)
	if err != nil {
		return "", err
	}
	notice, err := notifications.GetNotificationByID(s, id)
	if err != nil {
		return "", err
	}
	owned := notice != nil && notice.NotifiableID == actor.ID
	if notice == nil {
		history, historyErr := TaskTraceNotificationHistory(s, a)
		if historyErr != nil {
			return "", historyErr
		}
		for _, entry := range history {
			if entry.Notification != nil && entry.Notification.ID == id {
				notice = entry.Notification
				owned = true // History was loaded for this authenticated account.
				break
			}
		}
	}
	if notice == nil || !owned || notice.Name != TaskTraceExportNotificationName {
		return "", ErrGenericForbidden{}
	}
	data, err := json.Marshal(notice.Notification)
	if err != nil {
		return "", err
	}
	var payload TaskTraceExportNotification
	if err = json.Unmarshal(data, &payload); err != nil {
		return "", err
	}
	if payload.Path == "" {
		return "", errors.New("导出通知没有保存位置信息")
	}
	return payload.Path, nil
}
