// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

func taskTraceTeamMappedBinding(state *taskTraceTeamState, taskID int64) *TaskTraceTeamBinding {
	for i := range state.Bindings {
		binding := &state.Bindings[i]
		if binding.RootTaskID == taskID || taskTraceTeamNodeForTask(binding, taskID) != "" {
			return binding
		}
	}
	return nil
}

// Before its first sync, a newly created child belongs to its nearest shared
// ancestor. No network access is needed to recognize the local collaboration.
func taskTraceTeamLocalBinding(s *xorm.Session, state *taskTraceTeamState, taskID int64) (*TaskTraceTeamBinding, error) {
	if len(state.Bindings) == 0 {
		return nil, nil
	}
	for range MaxTaskHierarchyDepth {
		if binding := taskTraceTeamMappedBinding(state, taskID); binding != nil {
			return binding, nil
		}
		var parent TaskRelation
		found, err := s.Where("task_id = ? AND relation_kind = ?", taskID, RelationKindParenttask).
			Asc("other_task_id").Get(&parent)
		if err != nil || !found {
			return nil, err
		}
		taskID = parent.OtherTaskID
	}
	return nil, nil
}

// Keep the rows and their sync identities intact. A pending local comment or
// edit remains available to the normal sync, including after an offline hide.
func taskTraceSetLocalHidden(s *xorm.Session, a web.Auth, rootID int64, ids []int64) error {
	if _, err := s.In("id", ids).Where("task_trace_hidden_by = 0").NoAutoTime().
		Cols("task_trace_hidden_by").Update(&Task{TaskTraceHiddenBy: rootID}); err != nil {
		return err
	}
	return taskTraceLocalViewChanged(s, a, rootID)
}

func taskTraceShowLocalView(s *xorm.Session, a web.Auth, roots []int64) error {
	if len(roots) == 0 {
		return nil
	}
	changed, err := s.In("task_trace_hidden_by", roots).NoAutoTime().
		Cols("task_trace_hidden_by").Update(&Task{})
	if err != nil || changed == 0 {
		return err
	}
	return taskTraceLocalViewChanged(s, a, roots[0])
}

func (snap *taskTraceUndoCapture) restoreLocalView(s *xorm.Session, a web.Auth) ([]int64, error) {
	if err := snap.lockScope(s); err != nil {
		return nil, err
	}
	task, err := GetTaskByIDSimple(s, snap.TaskID)
	if err != nil {
		return nil, err
	}
	if task.TaskTraceHiddenBy != snap.TaskID {
		return nil, taskTraceUndoConflict()
	}
	can, err := (&Project{ID: task.ProjectID}).CanWrite(s, a)
	if err = taskTraceUndoRequire(can, err); err != nil {
		return nil, err
	}
	pruned, err := s.Where("task_trace_hidden_by = ? AND task_trace_view_pruned = ?", snap.TaskID, true).Exist(&Task{})
	if err != nil {
		return nil, err
	}
	if pruned {
		if err := taskTraceRestoreLocalViewFromRepository(s, a, snap.TaskID); err != nil {
			return nil, err
		}
	}
	if err := taskTraceShowLocalView(s, a, []int64{snap.TaskID}); err != nil {
		return nil, err
	}
	return snap.scopeTaskIDs(), nil
}

//nolint:gosmopolitan // Display the retry message directly in the TaskTrace UI.
func taskTraceRestoreLocalViewFromRepository(s *xorm.Session, a web.Auth, rootID int64) error {
	if !taskTraceTeamMu.TryLock() {
		return ErrTaskTraceUndoConflict{"团队同步正在进行，请稍后重试。"}
	}
	defer taskTraceTeamMu.Unlock()
	actor, err := user.GetFromAuth(a)
	if err != nil {
		return err
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return err
	}
	binding := taskTraceTeamMappedBinding(&state, rootID)
	if binding == nil {
		return taskTraceUndoConflict()
	}
	if err := taskTraceTeamValidateBinding(binding, actor.Username); err != nil {
		return err
	}
	if err := taskTraceHydrateLocalViews(s, a, binding, actor.Username, state.DeviceID, []int64{rootID}); err != nil {
		return err
	}
	return taskTraceTeamSaveState(state)
}

func taskTraceLocalViewChanged(s *xorm.Session, a web.Auth, rootID int64) error {
	task, err := GetTaskByIDSimple(s, rootID)
	if err != nil {
		if IsErrTaskDoesNotExist(err) {
			return nil
		}
		return err
	}
	events.DispatchOnCommit(s, &TaskTraceTaskChangedEvent{Task: &task, Doer: doerFromAuth(s, a)})
	return updateProjectLastUpdated(s, &Project{ID: task.ProjectID})
}
