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
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"golang.org/x/net/html"
	"xorm.io/builder"
	"xorm.io/xorm"
)

func restoreTaskTraceUndo(s *xorm.Session, a web.Auth, payload string) ([]int64, error) {
	snap := &taskTraceUndoCapture{}
	if err := json.Unmarshal([]byte(payload), snap); err != nil {
		return nil, err
	}
	if snap.Version != 1 || snap.TaskID < 1 {
		return nil, ErrTaskTraceUndoConflict{"无法识别这条撤销记录。"}
	}
	if snap.ActorID != a.GetID() {
		return nil, ErrGenericForbidden{}
	}
	if snap.Barrier != "" {
		return nil, ErrTaskTraceUndoConflict{snap.Barrier}
	}
	if err := snap.checkPermissions(s, a); err != nil {
		return nil, err
	}
	if err := snap.lockScope(s); err != nil {
		return nil, err
	}
	current, err := snap.readState(s)
	if err != nil {
		return nil, err
	}
	if snap.Kind == "task" {
		if snap.Method == "update" {
			if current.Task == nil {
				return nil, taskTraceUndoConflict()
			}
			for _, field := range snap.Fields {
				if !bytes.Equal(current.Task[field], snap.After.Task[field]) {
					return nil, taskTraceUndoConflict()
				}
			}
		} else if !taskTraceUndoSame("task", []taskTraceUndoRow{current.Task}, []taskTraceUndoRow{snap.After.Task}) {
			return nil, taskTraceUndoConflict()
		}
		current.Parts["positions"] = taskTraceUndoFilterViews(current.Parts["positions"], snap.ViewIDs)
	}
	for _, name := range append(append([]string{}, snap.Parts...), snap.Guards...) {
		if !taskTraceUndoSame(name, current.Parts[name], snap.After.Parts[name]) {
			return nil, taskTraceUndoConflict()
		}
	}
	if err := snap.validateRestoreReferences(s, a); err != nil {
		return nil, err
	}
	if snap.Kind == "task" {
		switch snap.Method {
		case "create":
			// Delete only the live task. Attachments, comments and other dependencies
			// remain in the existing trash retention period, never hard-deleted here.
			task := &Task{ID: snap.TaskID}
			if err := task.Delete(s, a); err != nil {
				return nil, err
			}
		case "delete":
			if err := taskTraceUndoUpdateTask(s, snap.TaskID, taskTraceUndoRow{"DeletedAt": snap.Before.Task["DeletedAt"]}); err != nil {
				return nil, err
			}
		case "update":
			if err := taskTraceUndoUpdateTask(s, snap.TaskID, snap.Before.Task); err != nil {
				return nil, err
			}
		}
	}
	// Restore all recorded parts within this transaction. Hierarchy validation
	// runs after the complete graph is back in place; a failure rolls it all back.
	for _, name := range snap.Parts {
		if err := taskTraceUndoReplaceRows(s, name, current.Parts[name], snap.Before.Parts[name]); err != nil {
			return nil, err
		}
	}
	if snap.Kind == "move" || snap.Kind == "relation" || (snap.Kind == "task" && snap.Method == "delete") {
		ids := []int64{snap.TaskID}
		if snap.OtherTaskID > 0 {
			ids = append(ids, snap.OtherTaskID)
		}
		for _, id := range ids {
			if err := taskTraceUndoValidateHierarchy(s, id); err != nil {
				return nil, err
			}
		}
	}
	ids := []int64{snap.TaskID}
	if snap.OtherTaskID > 0 && snap.OtherTaskID != snap.TaskID {
		ids = append(ids, snap.OtherTaskID)
	}
	for _, id := range ids {
		task := &Task{}
		exists, err := s.ID(id).Get(task)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		events.DispatchOnCommit(s, &TaskUpdatedEvent{Task: task, Doer: doerFromAuth(s, a)})
		if err = updateProjectLastUpdated(s, &Project{ID: task.ProjectID}); err != nil {
			return nil, err
		}
	}
	for _, name := range snap.Parts {
		if name != "positions" {
			continue
		}
		positions := []*TaskPosition{}
		for _, row := range snap.Before.Parts[name] {
			position := &TaskPosition{}
			if err = taskTraceUndoSetBean(row, position); err != nil {
				return nil, err
			}
			positions = append(positions, position)
		}
		events.DispatchOnCommit(s, &TaskPositionsRecalculatedEvent{NewTaskPositions: positions})
	}
	return ids, nil
}
func taskTraceUndoRequire(can bool, err error) error {
	if err != nil {
		return err
	}
	if !can {
		return ErrGenericForbidden{}
	}
	return nil
}
func taskTraceUndoTaskWrite(s *xorm.Session, a web.Auth, id int64) error {
	task := &Task{ID: id}
	can, err := task.CanUpdate(s, a)
	return taskTraceUndoRequire(can, err)
}
func (snap *taskTraceUndoCapture) checkPermissions(s *xorm.Session, a web.Auth) error {
	switch snap.Kind {
	case "task":
		if snap.Method == "delete" {
			row, err := taskTraceUndoReadTask(s, snap.TaskID)
			if err != nil {
				return err
			}
			if row == nil {
				return taskTraceUndoConflict()
			}
			project := &Project{ID: taskTraceUndoInt(row, "ProjectID")}
			can, err := project.CanWrite(s, a)
			if err = taskTraceUndoRequire(can, err); err != nil {
				return err
			}
		} else if err := taskTraceUndoTaskWrite(s, a, snap.TaskID); err != nil {
			return err
		}
	case "comment":
		if err := taskTraceUndoTaskWrite(s, a, snap.TaskID); err != nil {
			return err
		}
		if snap.Method == "delete" {
			if len(snap.Before.Parts["comments"]) != 1 || taskTraceUndoInt(snap.Before.Parts["comments"][0], "AuthorID") != a.GetID() {
				return ErrGenericForbidden{}
			}
		} else {
			comment := &TaskComment{ID: snap.CommentID, TaskID: snap.TaskID}
			can, err := comment.CanUpdate(s, a)
			if err = taskTraceUndoRequire(can, err); err != nil {
				return err
			}
		}
	case "outstanding":
		if err := taskTraceUndoTaskWrite(s, a, snap.TaskID); err != nil {
			return err
		}
		if err := taskTraceUndoTaskWrite(s, a, snap.OtherTaskID); err != nil {
			return err
		}
	case "move":
		if err := taskTraceUndoTaskWrite(s, a, snap.TaskID); err != nil {
			return err
		}
		ids := map[int64]bool{}
		for _, rows := range [][]taskTraceUndoRow{snap.Before.Parts["relations"], snap.After.Parts["relations"]} {
			for _, row := range rows {
				ids[taskTraceUndoInt(row, "TaskID")] = true
				ids[taskTraceUndoInt(row, "OtherTaskID")] = true
			}
		}
		for id := range ids {
			if err := taskTraceUndoTaskWrite(s, a, id); err != nil {
				return err
			}
		}
	case "relation":
		relation := &TaskRelation{TaskID: snap.TaskID, OtherTaskID: snap.OtherTaskID, RelationKind: snap.RelationKind}
		var can bool
		var err error
		if snap.Method == "delete" {
			can, err = relation.CanCreate(s, a)
		} else {
			can, err = relation.CanDelete(s, a)
		}
		if err = taskTraceUndoRequire(can, err); err != nil {
			return err
		}
	case "position":
		pos := &TaskPosition{TaskID: snap.TaskID, ProjectViewID: snap.ViewID}
		can, err := pos.CanUpdate(s, a)
		if err = taskTraceUndoRequire(can, err); err != nil {
			return err
		}
	default:
		return ErrTaskTraceUndoConflict{"这类操作暂不支持撤销。"}
	}
	// Check views again even if the original action had permission. A deleted or
	// newly private view must never be resurrected just by replaying old positions.
	views := map[int64]bool{}
	for _, rows := range [][]taskTraceUndoRow{snap.Before.Parts["positions"], snap.Before.Parts["buckets"]} {
		for _, row := range rows {
			views[taskTraceUndoInt(row, "ProjectViewID")] = true
		}
	}
	for id := range views {
		view, err := GetProjectViewByID(s, id)
		if err != nil {
			return err
		}
		can, _, err := view.CanRead(s, a)
		if err = taskTraceUndoRequire(can, err); err != nil {
			return err
		}
	}
	return nil
}

func taskTraceUndoColumns(s *xorm.Session, bean any, row taskTraceUndoRow) (map[string]any, error) {
	if err := taskTraceUndoSetBean(row, bean); err != nil {
		return nil, err
	}
	table, err := s.Engine().TableInfo(bean)
	if err != nil {
		return nil, err
	}
	value := reflect.Indirect(reflect.ValueOf(bean))
	columns := map[string]any{}
	for _, column := range table.Columns() {
		if _, exists := row[column.FieldName]; !exists {
			continue
		}
		field := value.FieldByIndex(column.FieldIndex).Interface()
		if column.IsDeleted {
			if date, ok := field.(time.Time); ok && date.IsZero() {
				columns[column.Name] = nil
				continue
			}
		}
		columns[column.Name] = field
	}
	return columns, nil
}
func taskTraceUndoUpdateTask(s *xorm.Session, id int64, row taskTraceUndoRow) error {
	columns, err := taskTraceUndoColumns(s, &Task{}, row)
	if err != nil {
		return err
	}
	// Keys come exclusively from the internal task snapshot, never a client body.
	delete(columns, "id")
	columns["updated"] = time.Now()
	changed, err := s.Table(&Task{}).Unscoped().Where("id = ?", id).Update(columns)
	if err != nil {
		return err
	}
	if changed != 1 {
		return taskTraceUndoConflict()
	}
	return nil
}
func taskTraceUndoRowCondition(kind string, row taskTraceUndoRow) builder.Cond {
	switch kind {
	case "positions", "buckets":
		return builder.Eq{"task_id": taskTraceUndoInt(row, "TaskID"), "project_view_id": taskTraceUndoInt(row, "ProjectViewID")}
	case "favorites":
		return builder.Eq{"entity_id": taskTraceUndoInt(row, "EntityID"), "user_id": taskTraceUndoInt(row, "UserID"), "kind": taskTraceUndoInt(row, "Kind")}
	default:
		return builder.Eq{"id": taskTraceUndoInt(row, "ID")}
	}
}
func taskTraceUndoReplaceRows(s *xorm.Session, kind string, current, before []taskTraceUndoRow) error {
	if kind == "legacy" {
		return nil
	}
	bean := taskTraceUndoBean(kind)
	if bean == nil {
		return fmt.Errorf("unknown undo part %q", kind)
	}
	oldRows := map[string]taskTraceUndoRow{}
	desired := map[string]taskTraceUndoRow{}
	for _, row := range current {
		oldRows[taskTraceUndoRowKey(kind, row)] = row
	}
	for _, row := range before {
		desired[taskTraceUndoRowKey(kind, row)] = row
	}
	keys := []string{}
	for key := range oldRows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		row := oldRows[key]
		if next, exists := desired[key]; exists {
			if taskTraceUndoSame(kind, []taskTraceUndoRow{row}, []taskTraceUndoRow{next}) {
				continue
			}
			columns, err := taskTraceUndoColumns(s, taskTraceUndoBean(kind), next)
			if err != nil {
				return err
			}
			if kind == "comments" {
				columns["updated"] = time.Now()
			}
			if _, err = s.Table(bean).Where(taskTraceUndoRowCondition(kind, row)).Update(columns); err != nil {
				return err
			}
		} else {
			if _, err := s.Where(taskTraceUndoRowCondition(kind, row)).Delete(bean); err != nil {
				return err
			}
		}
	}
	for _, row := range before {
		if _, exists := oldRows[taskTraceUndoRowKey(kind, row)]; exists {
			continue
		}
		next := taskTraceUndoBean(kind)
		if err := taskTraceUndoSetBean(row, next); err != nil {
			return err
		}
		if comment, ok := next.(*TaskComment); ok {
			comment.Updated = time.Now()
		}
		if _, err := s.NoAutoTime().Insert(next); err != nil {
			return err
		}
	}
	return nil
}

func (snap *taskTraceUndoCapture) validateRestoreReferences(s *xorm.Session, a web.Auth) error {
	restoringTask := int64(0)
	if snap.Kind == "task" && snap.Method == "delete" {
		restoringTask = snap.TaskID
	}
	if description := taskTraceUndoString(snap.Before.Task, "Description"); description != "" {
		if err := taskTraceUndoValidateImages(s, a, description, restoringTask); err != nil {
			return err
		}
	}
	if cover := taskTraceUndoInt(snap.Before.Task, "CoverImageAttachmentID"); cover > 0 {
		if err := taskTraceUndoValidateAttachment(s, a, snap.TaskID, cover, restoringTask); err != nil {
			return err
		}
	}
	for _, row := range snap.Before.Parts["comments"] {
		if err := taskTraceUndoValidateImages(s, a, taskTraceUndoString(row, "Comment"), restoringTask); err != nil {
			return err
		}
	}
	// Undoing a legacy-list migration exposes the older progress notes again.
	for _, row := range snap.Before.Parts["legacy"] {
		if err := taskTraceUndoValidateImages(s, a, taskTraceUndoString(row, "Comment"), restoringTask); err != nil {
			return err
		}
	}
	if restoringTask > 0 {
		related := map[int64]bool{}
		for _, row := range snap.Before.Parts["relations"] {
			related[taskTraceUndoInt(row, "TaskID")] = true
			related[taskTraceUndoInt(row, "OtherTaskID")] = true
		}
		for id := range related {
			if id == restoringTask {
				continue
			}
			task := &Task{ID: id}
			can, _, err := task.CanRead(s, a)
			if IsErrTaskDoesNotExist(err) {
				return ErrTaskTraceUndoConflict{"关联任务已被删除，无法恢复原有层级。"}
			}
			if err = taskTraceUndoRequire(can, err); err != nil {
				return err
			}
		}
		for _, row := range snap.Before.Parts["attachments"] {
			if err := taskTraceUndoValidateAttachment(s, a, snap.TaskID, taskTraceUndoInt(row, "ID"), restoringTask); err != nil {
				return err
			}
		}
	}
	for _, row := range snap.Before.Parts["buckets"] {
		bucket := &Bucket{}
		exists, err := s.ID(taskTraceUndoInt(row, "BucketID")).Get(bucket)
		if err != nil {
			return err
		}
		if !exists || bucket.ProjectViewID != taskTraceUndoInt(row, "ProjectViewID") {
			return ErrTaskTraceUndoConflict{"原任务分组已被删除或移动，无法安全撤销。"}
		}
	}
	for _, row := range snap.Before.Parts["assignees"] {
		assignee, err := user.GetUserByID(s, taskTraceUndoInt(row, "UserID"))
		if err != nil {
			return err
		}
		if restoringTask > 0 {
			project := &Project{ID: snap.ProjectID}
			can, _, err := project.CanRead(s, assignee)
			if err = taskTraceUndoRequire(can, err); err != nil {
				return err
			}
		} else {
			task := &Task{ID: snap.TaskID}
			can, _, err := task.CanRead(s, assignee)
			if err = taskTraceUndoRequire(can, err); err != nil {
				return err
			}
		}
	}
	return nil
}
func taskTraceUndoValidateImages(s *xorm.Session, a web.Auth, content string, restoringTask int64) error {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return err
	}
	refs := map[string]bool{}
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			refs[taskTraceAttribute(n, "src")] = true
		}
	})
	for src := range refs {
		parsed, err := url.Parse(src)
		if err != nil || parsed.IsAbs() || parsed.Host != "" {
			continue
		}
		match := taskTraceAttachmentPath.FindStringSubmatch(parsed.Path)
		if len(match) == 0 {
			continue
		}
		taskID, _ := strconv.ParseInt(match[1], 10, 64)
		attachmentID, _ := strconv.ParseInt(match[2], 10, 64)
		if err := taskTraceUndoValidateAttachment(s, a, taskID, attachmentID, restoringTask); err != nil {
			return err
		}
	}
	return nil
}
func taskTraceUndoValidateAttachment(s *xorm.Session, a web.Auth, taskID, attachmentID, restoringTask int64) error {
	attachment := &TaskAttachment{TaskID: taskID, ID: attachmentID}
	if taskID != restoringTask {
		can, _, err := attachment.CanRead(s, a)
		if err = taskTraceUndoRequire(can, err); err != nil {
			return err
		}
	}
	if err := attachment.ReadOne(s, a); err != nil {
		return ErrTaskTraceUndoConflict{"原图片附件已不存在，无法恢复包含该图片的内容。"}
	}
	if err := attachment.File.LoadFileByID(); err != nil {
		return ErrTaskTraceUndoConflict{"原图片文件已不存在，无法恢复包含该图片的内容。"}
	}
	return attachment.File.File.Close()
}

func taskTraceUndoValidateHierarchy(s *xorm.Session, id int64) error {
	relations := []*TaskRelation{}
	if err := s.Where("relation_kind = ?", RelationKindParenttask).Find(&relations); err != nil {
		return err
	}
	parents := map[int64][]int64{}
	children := map[int64][]int64{}
	for _, relation := range relations {
		parents[relation.TaskID] = append(parents[relation.TaskID], relation.OtherTaskID)
		children[relation.OtherTaskID] = append(children[relation.OtherTaskID], relation.TaskID)
	}
	var span func(int64, map[int64][]int64, map[int64]bool) (int, error)
	span = func(current int64, graph map[int64][]int64, path map[int64]bool) (int, error) {
		if path[current] {
			return 0, ErrTaskTraceUndoConflict{"撤销会造成任务循环归属，操作已取消。"}
		}
		path[current] = true
		defer delete(path, current)
		depth := 1
		for _, next := range graph[current] {
			childDepth, err := span(next, graph, path)
			if err != nil {
				return 0, err
			}
			if childDepth+1 > depth {
				depth = childDepth + 1
			}
			if depth > MaxTaskHierarchyDepth {
				return 0, ErrTaskHierarchyDepth{}
			}
		}
		return depth, nil
	}
	above, err := span(id, parents, map[int64]bool{})
	if err != nil {
		return err
	}
	below, err := span(id, children, map[int64]bool{})
	if err != nil {
		return err
	}
	if above+below-1 > MaxTaskHierarchyDepth {
		return ErrTaskHierarchyDepth{}
	}
	return nil
}

// Follow the task-update lock order (task rows, then project views). Task moves
// already hold view locks in CanCreate and never change task rows, so they only
// take the shared view locks here. All snapshot reads additionally lock their rows.
func (snap *taskTraceUndoCapture) lockScope(s *xorm.Session) error {
	projects := map[int64]bool{}
	if snap.ProjectID > 0 {
		projects[snap.ProjectID] = true
	}
	ids := []int64{snap.TaskID}
	if snap.OtherTaskID > 0 {
		ids = append(ids, snap.OtherTaskID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if snap.Kind != "move" {
		for index, id := range ids {
			if id < 1 || (index > 0 && ids[index-1] == id) {
				continue
			}
			var task Task
			exists, err := lockingSession(s).Unscoped().ID(id).Get(&task)
			if err != nil {
				return err
			}
			if exists {
				projects[task.ProjectID] = true
			}
		}
	}
	projectIDs := []int64{}
	for id := range projects {
		projectIDs = append(projectIDs, id)
	}
	if len(projectIDs) > 0 {
		if _, err := lockProjectViewsForPositionUpdate(s, projectIDs...); err != nil {
			return err
		}
	}
	if snap.ViewID > 0 {
		if err := lockPositionsForViewUpdate(s, snap.ViewID); err != nil {
			return err
		}
	}
	return nil
}
