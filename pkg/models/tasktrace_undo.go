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

package models

import (
	"context"
	"net/http"
	"sort"
	"time"

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/google/uuid"
	"xorm.io/xorm"
)

const taskTraceUndoMaxGroups = 50

type TaskTraceUndoEntry struct {
	ID      int64     `xorm:"bigint autoincr not null unique pk"`
	UserID  int64     `xorm:"bigint not null index"`
	GroupID string    `xorm:"varchar(36) not null"`
	Label   string    `xorm:"varchar(250) not null"`
	Payload string    `xorm:"longtext not null"`
	Created time.Time `xorm:"created not null"`
}

func (*TaskTraceUndoEntry) TableName() string { return "tasktrace_undo_entries" }

type ErrTaskTraceUndo struct{ Reason string }

func (e ErrTaskTraceUndo) Error() string { return e.Reason }

func (e ErrTaskTraceUndo) HTTPError() web.HTTPError {
	return web.HTTPError{
		HTTPCode: http.StatusConflict,
		Code:     4093,
		Message:  e.Reason,
	}
}

type TaskTraceUndo struct {
	ID              int64   `json:"id" minimum:"0" doc:"Latest journal entry ID. Undo requires the most recently read ID; zero means no history remains."`
	Label           string  `json:"label" readOnly:"true" doc:"Description of the newest undo step."`
	Count           int     `json:"count" readOnly:"true" doc:"Number of retained consecutive action groups, up to 50."`
	AffectedTaskIDs []int64 `json:"affected_task_ids,omitempty" readOnly:"true" doc:"Tasks restored by this undo request."`
	userID          int64
	group           []*TaskTraceUndoEntry
	web.CRUDable    `xorm:"-" json:"-"`
	web.Permissions `xorm:"-" json:"-"`
}

// This allowlist is intentionally limited to TaskTrace task editing operations.
// Projects, attachments, labels, sharing, accounts, settings and undo itself are inert.
func taskTraceUndoModelSupported(method string, model any) bool {
	switch model.(type) {
	case *Task, *TaskComment:
		return method == "create" || method == "update" || method == "delete"
	case *TaskTraceMove, *TaskTraceOutstandingMove:
		return method == "create"
	case *TaskRelation:
		return method == "create" || method == "delete"
	case *TaskPosition:
		return method == "update"
	default:
		return false
	}
}

func taskTraceUndoUser(a web.Auth) (*user.User, bool) {
	u, ok := a.(*user.User)
	return u, ok && u != nil && u.ID > 0
}

func lockTaskTraceUndoUser(s *xorm.Session, a web.Auth) error {
	u, ok := taskTraceUndoUser(a)
	if !ok {
		return nil
	}
	var current user.User
	exists, err := lockingSession(s).ID(u.ID).Get(&current)
	if err != nil {
		return err
	}
	if !exists {
		return ErrGenericForbidden{}
	}
	return nil
}

func init() {
	web.RegisterMutationUndoHooks(web.MutationUndoHooks{
		Supports: taskTraceUndoModelSupported,
		Lock:     func(_ context.Context, s *xorm.Session, a web.Auth) error { return lockTaskTraceUndoUser(s, a) },
		Capture:  beginTaskTraceUndo,
	})
}

func beginTaskTraceUndo(ctx context.Context, s *xorm.Session, a web.Auth, method string, model any) (func() error, error) {
	u, ok := taskTraceUndoUser(a)
	if !ok {
		return nil, nil
	}
	before, supported, err := captureTaskTraceUndo(s, a, method, model)
	if err != nil || !supported {
		return nil, err
	}
	group := web.TaskTraceUndoFromContext(ctx).GroupID
	if group == "" {
		group = uuid.NewString()
	} else {
		parsed, err := uuid.Parse(group)
		if err != nil {
			return nil, ErrTaskTraceUndo{"撤销分组标识无效，请重试。"}
		}
		group = parsed.String()
	}
	return func() error {
		payload, label, _, changed, err := finishTaskTraceUndo(s, a, method, model, before)
		if err != nil || !changed {
			return err
		}
		entry := &TaskTraceUndoEntry{
			UserID:  u.ID,
			GroupID: group,
			Label:   label,
			Payload: payload,
		}
		if _, err := s.Insert(entry); err != nil {
			return err
		}
		return trimTaskTraceUndo(s, u.ID)
	}, nil
}

func taskTraceUndoRows(s *xorm.Session, userID int64) ([]*TaskTraceUndoEntry, error) {
	rows := []*TaskTraceUndoEntry{}
	err := s.Where("user_id = ?", userID).Cols("id", "group_id", "label").Desc("id").Find(&rows)
	return rows, err
}

func trimTaskTraceUndo(s *xorm.Session, userID int64) error {
	rows, err := taskTraceUndoRows(s, userID)
	if err != nil {
		return err
	}
	groups := 0
	last := ""
	for _, row := range rows {
		if row.GroupID != last {
			groups++
			last = row.GroupID
		}
		if groups > taskTraceUndoMaxGroups {
			_, err = s.Where("user_id = ? AND id <= ?", userID, row.ID).Delete(&TaskTraceUndoEntry{})
			return err
		}
	}
	return nil
}

func (u *TaskTraceUndo) loadStatus(s *xorm.Session) error {
	rows, err := taskTraceUndoRows(s, u.userID)
	if err != nil {
		return err
	}
	u.ID = 0
	u.Label = ""
	u.Count = 0
	if len(rows) > 0 {
		u.ID = rows[0].ID
		u.Label = rows[0].Label
	}
	last := ""
	for _, row := range rows {
		if row.GroupID != last {
			u.Count++
			last = row.GroupID
		}
	}
	return nil
}

func (u *TaskTraceUndo) CanRead(_ *xorm.Session, a web.Auth) (bool, int, error) {
	actor, ok := taskTraceUndoUser(a)
	if !ok {
		return false, 0, nil
	}
	u.userID = actor.ID
	return true, int(PermissionRead), nil
}

func (u *TaskTraceUndo) ReadOne(s *xorm.Session, _ web.Auth) error { return u.loadStatus(s) }

func (u *TaskTraceUndo) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	actor, ok := taskTraceUndoUser(a)
	if !ok {
		return false, nil
	}
	u.userID = actor.ID
	if err := lockTaskTraceUndoUser(s, a); err != nil {
		return false, err
	}
	rows, err := taskTraceUndoRows(s, u.userID)
	if err != nil {
		return false, err
	}
	if len(rows) == 0 {
		return false, ErrTaskTraceUndo{"没有可撤销的操作。"}
	}
	if u.ID != rows[0].ID {
		return false, ErrTaskTraceUndo{"已有新的操作，请刷新后再撤销。"}
	}
	oldest := rows[0].ID
	for _, row := range rows {
		if row.GroupID != rows[0].GroupID {
			break
		}
		oldest = row.ID
	}
	u.group = nil
	err = s.Where("user_id = ? AND id >= ? AND id <= ?", u.userID, oldest, u.ID).Desc("id").Find(&u.group)
	return err == nil, err
}

func (u *TaskTraceUndo) Create(s *xorm.Session, a web.Auth) error {
	if u.userID == 0 || len(u.group) == 0 {
		return ErrTaskTraceUndo{"请先读取最新撤销记录。"}
	}
	affected := map[int64]bool{}
	for _, entry := range u.group {
		ids, err := restoreTaskTraceUndo(s, a, entry.Payload)
		if err != nil {
			return err
		}
		for _, id := range ids {
			affected[id] = true
		}
	}
	oldest := u.group[len(u.group)-1].ID
	newest := u.group[0].ID
	if _, err := s.Where("user_id = ? AND id >= ? AND id <= ?", u.userID, oldest, newest).Delete(&TaskTraceUndoEntry{}); err != nil {
		return err
	}
	u.AffectedTaskIDs = []int64{}
	for id := range affected {
		u.AffectedTaskIDs = append(u.AffectedTaskIDs, id)
	}
	sort.Slice(u.AffectedTaskIDs, func(i, j int) bool { return u.AffectedTaskIDs[i] < u.AffectedTaskIDs[j] })
	return u.loadStatus(s)
}
