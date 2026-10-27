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
	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

// CanRead checks if a user can read a comment
func (tc *TaskComment) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	t := Task{ID: tc.TaskID}
	return t.CanRead(s, a)
}

func (tc *TaskComment) canUserModifyTaskComment(s *xorm.Session, a web.Auth, allowTeamOwner bool) (bool, error) {
	t := Task{ID: tc.TaskID}
	canWriteTask, err := t.CanWrite(s, a)
	if err != nil {
		return false, err
	}
	if !canWriteTask {
		return false, nil
	}

	savedComment := &TaskComment{
		ID:     tc.ID,
		TaskID: tc.TaskID,
	}
	err = getTaskCommentSimple(s, savedComment)
	if err != nil {
		return false, err
	}

	if a.GetID() == savedComment.AuthorID {
		return true, nil
	}
	if allowTeamOwner {
		return taskTraceTeamOwnsLocalTask(a, savedComment.TaskID)
	}
	return false, nil
}

// CanDelete checks if a user can delete a comment
func (tc *TaskComment) CanDelete(s *xorm.Session, a web.Auth) (bool, error) {
	can, err := tc.canUserModifyTaskComment(s, a, true)
	tc.recordOutstandingActivity = can && err == nil
	return can, err
}

// CanUpdate checks if a user can update a comment
func (tc *TaskComment) CanUpdate(s *xorm.Session, a web.Auth) (bool, error) {
	can, err := tc.canUserModifyTaskComment(s, a, true)
	if err == nil && !can {
		can, err = tc.canUpdateSharedOutstandingList(s, a)
	}
	tc.recordOutstandingActivity = can && err == nil
	return can, err
}

// The current outstanding list is shared task state, rather than its original
// author's personal comment. Never extend this exception to ordinary comments,
// historical list snapshots, deleting comments, or converting a list to prose.
func (tc *TaskComment) canUpdateSharedOutstandingList(s *xorm.Session, a web.Auth) (bool, error) {
	if !taskTraceTeamEnabled() || !taskTraceTeamIsOutstanding(tc.Comment) {
		return false, nil
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return false, err
	}
	shared := false
	for index := range state.Bindings {
		if taskTraceTeamNodeForTask(&state.Bindings[index], tc.TaskID) != "" {
			shared = true
			break
		}
	}
	if !shared {
		return false, nil
	}
	allowed, err := (&Task{ID: tc.TaskID}).CanWrite(s, a)
	if err != nil || !allowed {
		return allowed, err
	}
	current, err := taskTraceReadOutstanding(s, tc.TaskID)
	if err != nil {
		return false, err
	}
	return current.comment != nil && current.comment.ID == tc.ID && taskTraceTeamIsOutstanding(current.original), nil
}

// CanCreate checks if a user can create a new comment
func (tc *TaskComment) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	t := Task{ID: tc.TaskID}
	can, err := t.CanWrite(s, a)
	tc.recordOutstandingActivity = can && err == nil
	return can, err
}
