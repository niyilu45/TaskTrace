// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"strings"
	"time"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/web"
	"github.com/google/uuid"
	"xorm.io/xorm"
)

// Task lifecycle events are explicit, ordered intents. An omitted task in an old
// snapshot, a failed import or an unavailable database is never a deletion.
// Versions are Lamport counters so clock skew cannot undo a deletion. Restoring
// a task requires observing its deletion and emitting a newer explicit intent.
type TaskTraceTeamTaskState struct {
	Version uint64    `json:"version"`
	Deleted bool      `json:"deleted"`
	Actor   string    `json:"actor"`
	EventID string    `json:"event_id"`
	Updated time.Time `json:"updated"`
}

func taskTraceTeamCopyTaskStates(source map[string]TaskTraceTeamTaskState) map[string]TaskTraceTeamTaskState {
	result := make(map[string]TaskTraceTeamTaskState, len(source))
	for node, state := range source {
		result[node] = state
	}
	return result
}

func taskTraceTeamTaskStateSupersedes(candidate, previous TaskTraceTeamTaskState) bool {
	if candidate.Version != previous.Version {
		return candidate.Version > previous.Version
	}
	if candidate.Deleted != previous.Deleted {
		return candidate.Deleted
	}
	return candidate.EventID > previous.EventID
}

func taskTraceTeamCaptureTaskStates(s *xorm.Session, binding *TaskTraceTeamBinding, actor string) error {
	if binding.TaskStates == nil {
		binding.TaskStates = map[string]TaskTraceTeamTaskState{}
	}
	for node, id := range binding.NodeTasks {
		var task Task
		found, err := s.Unscoped().ID(id).Get(&task)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		if id == binding.RootTaskID {
			binding.ProjectID = task.ProjectID
		}
		previous, known := binding.TaskStates[node]
		deleted := !task.DeletedAt.IsZero()
		if (!known && !deleted) || (known && previous.Deleted == deleted) {
			continue
		}
		binding.TaskStates[node] = TaskTraceTeamTaskState{Version: previous.Version + 1, Deleted: deleted, Actor: actor, EventID: uuid.NewString(), Updated: time.Now().UTC()}
	}
	return nil
}

func taskTraceTeamTaskDeleted(binding *TaskTraceTeamBinding, node string) bool {
	state, exists := binding.TaskStates[node]
	return exists && state.Deleted
}

func taskTraceTeamTaskStatesFromSnapshots(binding *TaskTraceTeamBinding, manifest *TaskTraceTeamManifest, snapshots []TaskTraceTeamSnapshot) map[string]TaskTraceTeamTaskState {
	states := taskTraceTeamCopyTaskStates(binding.TaskStates)
	for _, snapshot := range snapshots {
		for node, state := range snapshot.TaskStates {
			if strings.TrimSpace(node) == "" || state.Version == 0 || state.EventID == "" || state.Actor == "" || !taskTraceTeamMembersEqual(state.Actor, snapshot.Actor) || !taskTraceTeamCan(manifest, node, "", snapshot.Actor, true) {
				continue
			}
			if previous, ok := states[node]; !ok || taskTraceTeamTaskStateSupersedes(state, previous) {
				states[node] = state
			}
		}
	}
	return states
}

func taskTraceTeamApplyTaskStates(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, manifest *TaskTraceTeamManifest, snapshots []TaskTraceTeamSnapshot) ([]TaskTraceTeamSnapshot, error) {
	states := taskTraceTeamTaskStatesFromSnapshots(binding, manifest, snapshots)
	for node, state := range states {
		id := binding.NodeTasks[node]
		if id == 0 {
			continue
		}
		var task Task
		found, err := s.Unscoped().ID(id).Get(&task)
		if err != nil {
			return nil, err
		}
		if !found {
			if !state.Deleted {
				delete(binding.NodeTasks, node)
			}
			continue
		}
		if state.Deleted == !task.DeletedAt.IsZero() {
			continue
		}
		var deletedAt any
		if state.Deleted {
			deletedAt = state.Updated
			if state.Updated.IsZero() {
				deletedAt = time.Now().UTC()
			}
		}
		if _, err := s.Unscoped().Table("tasks").Where("id = ?", id).Update(map[string]any{"deleted_at": deletedAt}); err != nil {
			return nil, err
		}
		if state.Deleted {
			if _, err := s.Where("task_id = ?", id).Delete(&TaskPosition{}); err != nil {
				return nil, err
			}
			if _, err := s.Where("task_id = ?", id).Delete(&TaskBucket{}); err != nil {
				return nil, err
			}
		}
		events.DispatchOnCommit(s, &TaskTraceTaskChangedEvent{Task: &Task{ID: id, ProjectID: task.ProjectID}, Doer: doerFromAuth(s, a)})
	}
	binding.TaskStates = states
	filtered := make([]TaskTraceTeamSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		snapshot.Tasks = append([]TaskTraceTeamTask(nil), snapshot.Tasks...)
		tasks := snapshot.Tasks[:0]
		parents := map[string]string{}
		for _, task := range snapshot.Tasks {
			parents[task.NodeID] = task.ParentNode
		}
		for _, task := range snapshot.Tasks {
			hidden := false
			seen := map[string]bool{}
			for node := task.NodeID; node != "" && !seen[node]; node = parents[node] {
				seen[node] = true
				if taskTraceTeamTaskDeleted(binding, node) {
					hidden = true
					break
				}
			}
			if !hidden {
				tasks = append(tasks, task)
			}
		}
		snapshot.Tasks = tasks
		filtered = append(filtered, snapshot)
	}
	return filtered, nil
}
