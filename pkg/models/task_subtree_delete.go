// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
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

func taskTraceTaskDeletionAllowed(s *xorm.Session, a web.Auth, rootID int64) ([]*Task, error) {
	tasks, err := taskTraceTaskDeletionScope(s, rootID)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		allowed, err := task.CanUpdate(s, a)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, ErrGenericForbidden{}
		}
	}
	return tasks, nil
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
