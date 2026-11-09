// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

// A TaskTrace task tree is one deletion scope. Cross-project legacy links must
// never turn deleting one branch into deleting somebody else's project.
func taskTraceTaskDeletionScope(s *xorm.Session, rootID int64) ([]*Task, error) {
	root, err := GetTaskByIDSimple(s, rootID)
	if err != nil {
		return nil, err
	}
	ids, _, err := taskTraceTeamSubtree(s, rootID)
	if err != nil {
		return nil, err
	}
	inside := make(map[int64]bool, len(ids))
	for _, id := range ids {
		inside[id] = true
	}
	if len(ids) > 1 {
		var parents []*TaskRelation
		if err := s.In("task_id", ids[1:]).Where("relation_kind = ?", RelationKindParenttask).Find(&parents); err != nil {
			return nil, err
		}
		for _, parent := range parents {
			if !inside[parent.OtherTaskID] {
				return nil, ErrGenericForbidden{}
			}
		}
	}
	result := make([]*Task, 0, len(ids))
	for _, id := range ids {
		task, err := GetTaskByIDSimple(s, id)
		if err != nil {
			return nil, err
		}
		if task.ProjectID != root.ProjectID {
			return nil, ErrGenericForbidden{}
		}
		result = append(result, &task)
	}
	return result, nil
}

//nolint:gosmopolitan // TaskTrace presents this action error directly in Chinese.
func taskTraceTaskDeletionAllowed(s *xorm.Session, a web.Auth, rootID int64) ([]*Task, bool, error) {
	tasks, err := taskTraceTaskDeletionScope(s, rootID)
	if err != nil {
		return nil, false, err
	}
	if tasks[0].TaskTraceHiddenBy != 0 {
		return nil, false, ErrTaskDoesNotExist{ID: rootID}
	}
	actor, err := user.GetFromAuth(a)
	if err != nil {
		return nil, false, err
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, false, err
	}
	binding, err := taskTraceTeamLocalBinding(s, &state, rootID)
	if err != nil {
		return nil, false, err
	}
	localOnly := binding != nil && !taskTraceTeamMembersEqual(binding.Owner, actor.Username)
	if localOnly {
		allowed, err := (&Project{ID: tasks[0].ProjectID}).CanWrite(s, a)
		if err != nil {
			return nil, false, err
		}
		if !allowed {
			return nil, false, ErrGenericForbidden{}
		}
		return tasks, true, nil
	}
	// An imported branch under a personal task must never become a shared
	// deletion just because the user deletes its local parent.
	for _, task := range tasks {
		if imported := taskTraceTeamMappedBinding(&state, task.ID); imported != nil && !taskTraceTeamMembersEqual(imported.Owner, actor.Username) {
			return nil, false, ErrTaskTraceMoveInvalid{"请先将其他人共享的子任务移出此任务，再删除。"}
		}
		allowed, err := task.CanUpdate(s, a)
		if err != nil {
			return nil, false, err
		}
		if !allowed {
			return nil, false, ErrGenericForbidden{}
		}
	}
	return tasks, false, nil
}

func taskTraceSameDeletionScope(expected []int64, tasks []*Task) bool {
	if len(expected) != len(tasks) {
		return false
	}
	ids := make(map[int64]bool, len(expected))
	for _, id := range expected {
		ids[id] = true
	}
	for _, task := range tasks {
		if !ids[task.ID] {
			return false
		}
	}
	return true
}
