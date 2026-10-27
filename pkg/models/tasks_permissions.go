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

// CanDelete checks if the user can delete an task
func (t *Task) CanDelete(s *xorm.Session, a web.Auth) (bool, error) {
	allowed, err := t.canDoTask(s, a)
	if err != nil || !allowed || !taskTraceTeamEnabled() {
		return allowed, err
	}
	tasks, err := taskTraceTaskDeletionAllowed(s, a, t.ID)
	if err != nil {
		return false, err
	}
	t.deletionScopeIDs = make([]int64, 0, len(tasks))
	for _, task := range tasks {
		t.deletionScopeIDs = append(t.deletionScopeIDs, task.ID)
	}
	return true, nil
}

// CanUpdate determines if a user has the permission to update a project task
func (t *Task) CanUpdate(s *xorm.Session, a web.Auth) (bool, error) {
	return t.canDoTask(s, a)
}

// CanCreate determines if a user has the permission to create a project task
func (t *Task) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	// A user can do a task if he has write acces to its project
	l := &Project{ID: t.ProjectID}
	allowed, err := l.CanWrite(s, a)
	if err != nil || !allowed {
		return allowed, err
	}
	return t.canCreateUnderParent(s, a)
}

func (t *Task) canCreateUnderParent(s *xorm.Session, a web.Auth) (bool, error) {
	if t.ParentTaskID == 0 {
		return true, nil
	}
	parent, err := GetTaskByIDSimple(s, t.ParentTaskID)
	if err != nil {
		return false, err
	}
	if parent.ProjectID != t.ProjectID {
		return false, ErrGenericForbidden{}
	}
	allowed, err := parent.CanUpdate(s, a)
	if err != nil || !allowed {
		return allowed, err
	}
	depth, err := taskHierarchySpan(s, parent.ID, RelationKindParenttask, 1)
	if err != nil {
		return false, err
	}
	if depth >= MaxTaskHierarchyDepth {
		return false, ErrTaskHierarchyDepth{}
	}
	return true, nil
}

// CanRead determines if a user can read a task
func (t *Task) CanRead(s *xorm.Session, a web.Auth) (canRead bool, maxPermission int, err error) {
	expand := t.Expand
	if err = t.resolveIDFromProjectAndIndex(s); err != nil {
		return
	}
	*t, err = GetTaskByIDSimple(s, t.ID)
	if err != nil {
		return
	}

	t.Expand = expand

	// A user can read a task if it has access to the project
	l := &Project{ID: t.ProjectID}
	return l.CanRead(s, a)
}

// CanWrite checks if a user has write access to a task
func (t *Task) CanWrite(s *xorm.Session, a web.Auth) (canWrite bool, err error) {
	return t.canDoTask(s, a)
}

// Helper function to check if a user can do stuff on a project task
func (t *Task) canDoTask(s *xorm.Session, a web.Auth) (bool, error) {
	// Get the task
	ot, err := GetTaskByIDSimple(s, t.ID)
	if err != nil {
		return false, err
	}

	// Check if we're moving the task into a different project to check if the user has sufficient permissions for that on the new project
	if t.ProjectID != 0 && t.ProjectID != ot.ProjectID {
		newProject := &Project{ID: t.ProjectID}
		can, err := newProject.CanWrite(s, a)
		if err != nil {
			return false, err
		}
		if !can {
			return false, ErrGenericForbidden{}
		}
	}

	// A user can do a task if it has write access to its project and, for a
	// TaskTrace collaboration task, the live shared manifest grants write access.
	l := &Project{ID: ot.ProjectID}
	canWriteProject, err := l.CanWrite(s, a)
	if err != nil || !canWriteProject {
		return canWriteProject, err
	}
	return taskTraceTeamCanWriteLocalTask(a, ot.ID, s)
}
