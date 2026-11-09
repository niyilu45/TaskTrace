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
	"net/http"
	"sort"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/builder"
	"xorm.io/xorm"
)

// TaskTraceMove changes a task's parent and manual position in one transaction.
type TaskTraceMove struct {
	TaskID          int64   `json:"task_id" readOnly:"true" doc:"Task being moved, taken from the URL."`
	ProjectID       int64   `json:"project_id,omitempty" minimum:"0" doc:"Destination project. Zero keeps the current project. All descendants move with the task."`
	ParentID        int64   `json:"parent_id" minimum:"0" doc:"New parent in the destination project; zero makes the task a root task."`
	BeforeTaskID    int64   `json:"before_task_id" minimum:"0" doc:"Insert before this sibling; zero appends to the new parent's children."`
	ProjectViewID   int64   `json:"project_view_id" minimum:"1" doc:"Existing view of the destination project in which to save manual order. Its filters are ignored."`
	Position        float64 `json:"position" readOnly:"true" doc:"The persisted position after the move."`
	task            Task
	parents         []*TaskRelation
	subtree         []*Task
	localPlacement  *TaskTraceTeamBinding
	web.CRUDable    `json:"-" xorm:"-"`
	web.Permissions `json:"-" xorm:"-"`
}

// ErrTaskTraceMoveInvalid indicates an invalid destination or sibling anchor.
type ErrTaskTraceMoveInvalid struct{ Reason string }

func (e ErrTaskTraceMoveInvalid) Error() string { return e.Reason }

func (e ErrTaskTraceMoveInvalid) HTTPError() web.HTTPError {
	return web.HTTPError{
		HTTPCode: http.StatusConflict,
		Code:     4091,
		Message:  e.Reason,
	}
}

func (m *TaskTraceMove) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	m.subtree = nil
	m.parents = nil
	m.localPlacement = nil
	var err error
	m.task, err = GetTaskByIDSimple(s, m.TaskID)
	if err != nil {
		return false, err
	}
	if m.ProjectID < 0 || m.ParentID < 0 || m.BeforeTaskID < 0 {
		return false, ErrTaskTraceMoveInvalid{"Task and project identifiers cannot be negative."}
	}
	if m.ProjectID == 0 {
		m.ProjectID = m.task.ProjectID
	}
	if m.task.TaskTraceHiddenBy != 0 {
		return false, ErrTaskDoesNotExist{ID: m.TaskID}
	}
	can, err := m.canMoveSource(s, a)
	if err != nil || !can {
		return can, err
	}
	if m.localPlacement != nil {
		can, err = (&Project{ID: m.ProjectID}).CanWrite(s, a)
		if err != nil || !can {
			return can, err
		}
	}
	projects := []int64{m.task.ProjectID, m.ProjectID}
	if m.ProjectID != m.task.ProjectID {
		// Lock task rows before views, matching ordinary task updates. Authorize
		// every descendant, including completed and UI-filtered tasks.
		ids, _, err := taskTraceTeamSubtree(s, m.TaskID)
		if err != nil {
			return false, err
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		for _, id := range ids {
			task, err := GetTaskSimple(lockingSession(s), &Task{ID: id})
			if err != nil {
				return false, err
			}
			if id == m.TaskID && task.ProjectID != m.task.ProjectID {
				return false, ErrTaskTraceMoveInvalid{"The task was moved elsewhere. Refresh and try again."}
			}
			can, err := m.canMoveTask(s, a, task)
			if err != nil || !can {
				return can, err
			}
			m.subtree = append(m.subtree, &task)
			projects = append(projects, task.ProjectID)
		}
	}
	// Serialize source and destination views in a stable order.
	if _, err = lockProjectViewsForPositionUpdate(s, projects...); err != nil {
		return false, err
	}
	view, err := GetProjectViewByIDAndProject(s, m.ProjectViewID, m.ProjectID)
	if err != nil {
		return false, err
	}
	can, _, err = view.CanRead(s, a)
	if err != nil || !can {
		return can, err
	}
	if m.ParentID != 0 {
		parentTask, err := GetTaskByIDSimple(s, m.ParentID)
		if err != nil {
			return false, err
		}
		if parentTask.TaskTraceHiddenBy != 0 {
			return false, ErrTaskDoesNotExist{ID: m.ParentID}
		}
		can, err = m.canMoveTask(s, a, parentTask)
		if err != nil || !can {
			return can, err
		}
		if parentTask.ProjectID != m.ProjectID {
			return false, ErrTaskTraceMoveInvalid{"The new parent must belong to the destination project."}
		}
	}
	err = s.Where("task_id = ? AND relation_kind = ?", m.TaskID, RelationKindParenttask).Find(&m.parents)
	if err != nil {
		return false, err
	}
	for _, rel := range m.parents {
		parent, err := GetTaskByIDSimple(s, rel.OtherTaskID)
		if err != nil {
			return false, err
		}
		can, err = m.canMoveTask(s, a, parent)
		if err != nil || !can {
			return can, err
		}
	}
	return true, nil
}

func (m *TaskTraceMove) canMoveSource(s *xorm.Session, a web.Auth) (bool, error) {
	if !taskTraceTeamEnabled() {
		return m.canMoveTask(s, a, m.task)
	}
	actor, err := user.GetFromAuth(a)
	if err != nil {
		return false, err
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return false, err
	}
	binding, err := taskTraceTeamLocalBinding(s, &state, m.TaskID)
	if err != nil {
		return false, err
	}
	if binding == nil || taskTraceTeamMembersEqual(binding.Owner, actor.Username) {
		return m.canMoveTask(s, a, m.task)
	}
	if taskTraceTeamNodeForTask(binding, m.TaskID) == "" {
		return false, ErrTaskTraceMoveInvalid{"Synchronize this new shared task before moving it."}
	}
	if m.ParentID != 0 {
		target, err := taskTraceTeamLocalBinding(s, &state, m.ParentID)
		if err != nil {
			return false, err
		}
		if target != nil && target.ShareID != binding.ShareID {
			return false, ErrTaskTraceMoveInvalid{"Move the shared task to a project or a personal parent task."}
		}
	}
	m.localPlacement = binding
	return m.canMoveTask(s, a, m.task)
}

func (m *TaskTraceMove) canMoveTask(s *xorm.Session, a web.Auth, task Task) (bool, error) {
	if m.localPlacement != nil {
		return (&Project{ID: task.ProjectID}).CanWrite(s, a)
	}
	return (&Task{ID: task.ID, ProjectID: m.ProjectID}).CanUpdate(s, a)
}

func (m *TaskTraceMove) sameParent() bool {
	return (m.ParentID == 0 && len(m.parents) == 0) || (len(m.parents) == 1 && m.parents[0].OtherTaskID == m.ParentID)
}

// orderedTasks validates the destination before any mutation and prepares the
// global view order. Renumbering avoids floating-point collisions after many drags.
func (m *TaskTraceMove) orderedTasks(s *xorm.Session) ([]*Task, error) {
	var tasks []*Task
	if err := s.Where("project_id = ?", m.ProjectID).Find(&tasks); err != nil {
		return nil, err
	}
	for _, task := range m.subtree {
		if task.ProjectID != m.ProjectID {
			tasks = append(tasks, task)
		}
	}
	var positions []*TaskPosition
	if err := s.Where("project_view_id = ?", m.ProjectViewID).Find(&positions); err != nil {
		return nil, err
	}
	pos := make(map[int64]float64, len(positions))
	for _, p := range positions {
		pos[p.TaskID] = p.Position
	}
	ids := make(map[int64]bool, len(tasks))
	taskIDs := make([]int64, 0, len(tasks))
	for _, t := range tasks {
		ids[t.ID] = true
		taskIDs = append(taskIDs, t.ID)
		if _, ok := pos[t.ID]; !ok {
			pos[t.ID] = calculateDefaultPosition(t.Index, 0)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		if pos[tasks[i].ID] == pos[tasks[j].ID] {
			return tasks[i].ID < tasks[j].ID
		}
		return pos[tasks[i].ID] < pos[tasks[j].ID]
	})
	// Match the tree's deterministic parent choice for legacy multi-parent tasks.
	var relations []*TaskRelation
	if err := s.Where("relation_kind = ?", RelationKindParenttask).In("task_id", taskIDs).Find(&relations); err != nil {
		return nil, err
	}
	parents := make(map[int64]int64)
	for _, r := range relations {
		if !ids[r.TaskID] || !ids[r.OtherTaskID] {
			continue
		}
		if parents[r.TaskID] == 0 || r.OtherTaskID < parents[r.TaskID] {
			parents[r.TaskID] = r.OtherTaskID
		}
	}
	if m.BeforeTaskID != 0 && (m.BeforeTaskID == m.TaskID || !ids[m.BeforeTaskID] || parents[m.BeforeTaskID] != m.ParentID) {
		return nil, ErrTaskTraceMoveInvalid{"The insertion target must be another task under the new parent."}
	}
	ordered := make([]*Task, 0, len(tasks))
	insertAt := -1
	for _, t := range tasks {
		if t.ID == m.TaskID {
			continue
		}
		if t.ID == m.BeforeTaskID {
			insertAt = len(ordered)
		}
		ordered = append(ordered, t)
		if m.BeforeTaskID == 0 && parents[t.ID] == m.ParentID {
			insertAt = len(ordered)
		}
	}
	if insertAt < 0 {
		insertAt = len(ordered)
	}
	ordered = append(ordered, nil)
	copy(ordered[insertAt+1:], ordered[insertAt:])
	ordered[insertAt] = &m.task
	return ordered, nil
}

func (m *TaskTraceMove) Create(s *xorm.Session, a web.Auth) error {
	// The public CRUD pipeline populates cached task/parents in CanCreate.
	if m.task.ID == 0 {
		return ErrTaskTraceMoveInvalid{"The move has not been authorized."}
	}
	if err := lockPositionsForViewUpdate(s, m.ProjectViewID); err != nil {
		return err
	}
	// Reject a concurrently deleted view rather than creating orphan positions.
	if _, err := GetProjectViewByIDAndProject(s, m.ProjectViewID, m.ProjectID); err != nil {
		return err
	}
	if m.ParentID == m.TaskID {
		return ErrRelationTasksCannotBeTheSame{
			TaskID:      m.TaskID,
			OtherTaskID: m.ParentID,
		}
	}
	relation := &TaskRelation{
		TaskID:       m.TaskID,
		OtherTaskID:  m.ParentID,
		RelationKind: RelationKindParenttask,
	}
	if !m.sameParent() && m.ParentID != 0 {
		if err := checkTaskRelationCycle(s, relation, m.ParentID, nil, nil); err != nil {
			return err
		}
		if err := checkTaskHierarchyDepth(s, relation); err != nil {
			return err
		}
	}
	ordered, err := m.orderedTasks(s)
	if err != nil {
		return err
	}
	if len(m.subtree) > 0 {
		if err := m.moveSubtreeToProject(s, a); err != nil {
			return err
		}
	}
	if !m.sameParent() {
		if err := m.rememberSharedParent(s); err != nil {
			return err
		}
		for _, r := range m.parents {
			if err := r.Delete(s, a); err != nil {
				return err
			}
		}
		if m.ParentID != 0 {
			if err := relation.Create(s, a); err != nil {
				return err
			}
		}
	}
	positions := make([]*TaskPosition, 0, len(ordered))
	for i, t := range ordered {
		position := float64(i+1) * 65536
		positions = append(positions, &TaskPosition{
			TaskID:        t.ID,
			ProjectViewID: m.ProjectViewID,
			Position:      position,
		})
		if t.ID == m.TaskID {
			m.Position = position
		}
	}
	if err := bulkInsertTaskPositions(s, positions, true); err != nil {
		return err
	}
	events.DispatchOnCommit(s, &TaskPositionsRecalculatedEvent{NewTaskPositions: positions})
	return nil
}

// Keep the shared parent in snapshots even after this copy is detached or
// moved under another local parent. Project IDs never travel in snapshots.
func (m *TaskTraceMove) rememberSharedParent(s *xorm.Session) error {
	if m.localPlacement == nil || m.task.TaskTraceSharedParentID != nil {
		return nil
	}
	parent := int64(0)
	for _, relation := range m.parents {
		id := relation.OtherTaskID
		if taskTraceTeamNodeForTask(m.localPlacement, id) != "" && (parent == 0 || id < parent) {
			parent = id
		}
	}
	_, err := s.ID(m.TaskID).NoAutoTime().Cols("task_trace_shared_parent_id").
		Update(&Task{TaskTraceSharedParentID: &parent})
	return err
}

// moveSubtreeToProject only updates placement columns. A normal Task.Update
// also rewrites reminders and assignees, which a structural move must preserve.
func (m *TaskTraceMove) moveSubtreeToProject(s *xorm.Session, a web.Auth) error {
	ids, _, err := taskTraceTeamSubtree(s, m.TaskID)
	if err != nil {
		return err
	}
	known := map[int64]bool{}
	for _, task := range m.subtree {
		known[task.ID] = true
	}
	if len(ids) != len(known) {
		return ErrTaskTraceMoveInvalid{"The subtree changed during the move. Refresh and try again."}
	}
	for _, id := range ids {
		if !known[id] {
			return ErrTaskTraceMoveInvalid{"The subtree changed during the move. Refresh and try again."}
		}
	}
	moving := []*Task{}
	projects := map[int64]bool{m.ProjectID: true}
	for _, task := range m.subtree {
		if task.ProjectID == m.ProjectID {
			continue
		}
		projects[task.ProjectID] = true
		if err := retainTaskIndexAlias(s, task); err != nil {
			return err
		}
		task.ProjectID = m.ProjectID
		task.Index = 0
		moving = append(moving, task)
	}
	if err := setNewTaskIndexes(s, m.ProjectID, moving); err != nil {
		return err
	}
	for _, task := range moving {
		update := s.ID(task.ID).Cols("project_id", "index")
		if m.localPlacement != nil {
			update = update.NoAutoTime()
		}
		if _, err := update.Update(task); err != nil {
			return err
		}
		if _, err := s.Where("task_id = ?", task.ID).Delete(&TaskBucket{}); err != nil {
			return err
		}
		if _, err := s.Where("task_id = ?", task.ID).Delete(&TaskPosition{}); err != nil {
			return err
		}
		if task.ID == m.TaskID {
			m.task = *task
		}
		events.DispatchOnCommit(s, &TaskUpdatedEvent{Task: task, Doer: doerFromAuth(s, a)})
	}
	views, err := lockProjectViewsForPositionUpdate(s, m.ProjectID)
	if err != nil {
		return err
	}
	positions, buckets, err := setTasksInBucketInViews(s, views, moving, a, true, nil)
	if err != nil {
		return err
	}
	if err := bulkInsertTaskPositions(s, positions, false); err != nil {
		return err
	}
	for _, bucket := range buckets {
		if _, err := s.Insert(bucket); err != nil {
			return err
		}
	}
	for projectID := range projects {
		if err := updateProjectLastUpdated(s, &Project{ID: projectID}); err != nil {
			return err
		}
	}
	return nil
}

// TaskTracePositions reads the stored manual ordering without applying view filters.
type TaskTracePositions struct {
	ProjectID       int64 `json:"-"`
	ProjectViewID   int64 `json:"-"`
	web.CRUDable    `json:"-" xorm:"-"`
	web.Permissions `json:"-" xorm:"-"`
}

func (p *TaskTracePositions) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	view, err := GetProjectViewByIDAndProject(s, p.ProjectViewID, p.ProjectID)
	if err != nil {
		return false, 0, err
	}
	return view.CanRead(s, a)
}

func (p *TaskTracePositions) ReadAll(s *xorm.Session, a web.Auth, _ string, page, perPage int) (any, int, int64, error) {
	can, _, err := p.CanRead(s, a)
	if err != nil {
		return nil, 0, 0, err
	}
	if !can {
		return nil, 0, 0, ErrGenericForbidden{}
	}
	items := []*TaskPosition{}
	limit, start := getLimitFromPageIndex(page, perPage)
	total, err := s.Where("project_view_id = ?", p.ProjectViewID).In("task_id", builder.Select("id").From("tasks").Where(builder.And(builder.Eq{"project_id": p.ProjectID}, taskVisibleCond("tasks")))).OrderBy("position ASC, task_id ASC").Limit(limit, start).FindAndCount(&items)
	return items, len(items), total, err
}
