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
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/web"
	"xorm.io/builder"
	"xorm.io/xorm"
)

// ErrTaskTraceUndoConflict refuses an undo that would overwrite newer work.
type ErrTaskTraceUndoConflict struct{ Reason string }

func (e ErrTaskTraceUndoConflict) Error() string { return e.Reason }
func (e ErrTaskTraceUndoConflict) HTTPError() web.HTTPError {
	return web.HTTPError{HTTPCode: http.StatusConflict, Code: 4094, Message: e.Reason}
}
func taskTraceUndoConflict() error {
	return ErrTaskTraceUndoConflict{"内容已被其他操作修改，无法安全撤销。请刷新后检查。"}
}

type taskTraceUndoRow map[string]json.RawMessage
type taskTraceUndoState struct {
	Task  taskTraceUndoRow              `json:"task,omitempty"`
	Parts map[string][]taskTraceUndoRow `json:"parts"`
}
type taskTraceUndoCapture struct {
	Version      int                `json:"version"`
	Kind         string             `json:"kind"`
	Method       string             `json:"method"`
	TaskID       int64              `json:"task_id"`
	OtherTaskID  int64              `json:"other_task_id,omitempty"`
	ProjectID    int64              `json:"project_id,omitempty"`
	CommentID    int64              `json:"comment_id,omitempty"`
	ViewID       int64              `json:"view_id,omitempty"`
	ActorID      int64              `json:"actor_id"`
	RelationKind RelationKind       `json:"relation_kind,omitempty"`
	Before       taskTraceUndoState `json:"before"`
	After        taskTraceUndoState `json:"after"`
	Fields       []string           `json:"fields,omitempty"`
	Parts        []string           `json:"changed_parts,omitempty"`
	Guards       []string           `json:"guards,omitempty"`
	ViewIDs      []int64            `json:"view_ids,omitempty"`
	Barrier      string             `json:"barrier,omitempty"`
}

func taskTraceUndoBean(kind string) any {
	switch kind {
	case "positions":
		return &TaskPosition{}
	case "buckets":
		return &TaskBucket{}
	case "reminders":
		return &TaskReminder{}
	case "assignees":
		return &TaskAssginee{}
	case "favorites":
		return &Favorite{}
	case "relations":
		return &TaskRelation{}
	case "comments", "legacy":
		return &TaskComment{}
	case "attachments":
		return &TaskAttachment{}
	case "labels":
		return &LabelTask{}
	case "time_entries":
		return &TimeEntry{}
	case "reactions":
		return &Reaction{}
	}
	return nil
}
func taskTraceUndoRowOf(s *xorm.Session, bean any) (taskTraceUndoRow, error) {
	table, err := s.Engine().TableInfo(bean)
	if err != nil {
		return nil, err
	}
	value := reflect.Indirect(reflect.ValueOf(bean))
	row := taskTraceUndoRow{}
	for _, col := range table.Columns() {
		if col.IsUpdated {
			continue
		}
		field := value.FieldByIndex(col.FieldIndex)
		v := field.Interface()
		if tm, ok := v.(time.Time); ok {
			v = tm.UTC()
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		row[col.FieldName] = raw
	}
	return row, nil
}
func taskTraceUndoSetBean(row taskTraceUndoRow, bean any) error {
	value := reflect.Indirect(reflect.ValueOf(bean))
	for key, raw := range row {
		field := value.FieldByName(key)
		if !field.IsValid() || !field.CanAddr() {
			return fmt.Errorf("unknown undo field %q", key)
		}
		if err := json.Unmarshal(raw, field.Addr().Interface()); err != nil {
			return err
		}
	}
	return nil
}
func taskTraceUndoReadRows(s *xorm.Session, bean any, cond builder.Cond) ([]taskTraceUndoRow, error) {
	rows := []taskTraceUndoRow{}
	typ := reflect.TypeOf(bean).Elem()
	list := reflect.New(reflect.SliceOf(typ))
	if err := lockingSession(s).Where(cond).Find(list.Interface()); err != nil {
		return nil, err
	}
	value := list.Elem()
	for i := 0; i < value.Len(); i++ {
		row, err := taskTraceUndoRowOf(s, value.Index(i).Addr().Interface())
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	taskTraceUndoSortRows(rows)
	return rows, nil
}
func taskTraceUndoSortRows(rows []taskTraceUndoRow) {
	sort.Slice(rows, func(i, j int) bool {
		a, _ := json.Marshal(rows[i])
		b, _ := json.Marshal(rows[j])
		return bytes.Compare(a, b) < 0
	})
}
func taskTraceUndoInt(row taskTraceUndoRow, key string) int64 {
	var value int64
	_ = json.Unmarshal(row[key], &value)
	return value
}
func taskTraceUndoString(row taskTraceUndoRow, key string) string {
	var value string
	_ = json.Unmarshal(row[key], &value)
	return value
}
func taskTraceUndoSemantic(kind string, rows []taskTraceUndoRow) string {
	normalized := make([]taskTraceUndoRow, 0, len(rows))
	for _, row := range rows {
		copyRow := taskTraceUndoRow{}
		for key, value := range row {
			if key == "Updated" {
				continue
			}
			if (kind == "reminders" || kind == "assignees" || kind == "relations") && (key == "ID" || key == "Created") {
				continue
			}
			copyRow[key] = value
		}
		normalized = append(normalized, copyRow)
	}
	taskTraceUndoSortRows(normalized)
	raw, _ := json.Marshal(normalized)
	return string(raw)
}
func taskTraceUndoSame(kind string, a, b []taskTraceUndoRow) bool {
	return taskTraceUndoSemantic(kind, a) == taskTraceUndoSemantic(kind, b)
}
func taskTraceUndoPart(state *taskTraceUndoState, name string, rows []taskTraceUndoRow, err error) error {
	if err != nil {
		return err
	}
	state.Parts[name] = rows
	return nil
}
func taskTraceUndoReadTask(s *xorm.Session, id int64) (taskTraceUndoRow, error) {
	if id == 0 {
		return nil, nil
	}
	task := &Task{}
	has, err := lockingSession(s).Unscoped().ID(id).Get(task)
	if err != nil || !has {
		return nil, err
	}
	return taskTraceUndoRowOf(s, task)
}

func captureTaskTraceUndo(s *xorm.Session, a web.Auth, method string, model any) (any, bool, error) {
	snap := &taskTraceUndoCapture{Version: 1, Method: method, ActorID: a.GetID()}
	switch object := model.(type) {
	case *Task:
		if method != "create" && method != "update" && method != "delete" {
			return nil, false, nil
		}
		snap.Kind = "task"
		snap.TaskID = object.ID
		snap.ProjectID = object.ProjectID
		if method == "create" {
			snap.TaskID = 0
		} else {
			row, err := taskTraceUndoReadTask(s, snap.TaskID)
			if err != nil {
				return nil, true, err
			}
			snap.ProjectID = taskTraceUndoInt(row, "ProjectID")
		}
	case *TaskComment:
		if method != "create" && method != "update" && method != "delete" {
			return nil, false, nil
		}
		snap.Kind = "comment"
		snap.TaskID = object.TaskID
		snap.CommentID = object.ID
		if method == "create" {
			snap.CommentID = 0
		} else {
			old := &TaskComment{ID: object.ID}
			if err := getTaskCommentSimple(s, old); err != nil {
				return nil, true, err
			}
			snap.TaskID = old.TaskID
		}
	case *TaskTraceMove:
		if method != "create" {
			return nil, false, nil
		}
		snap.Kind = "move"
		snap.TaskID = object.TaskID
		snap.ViewID = object.ProjectViewID
		snap.ProjectID = object.task.ProjectID
	case *TaskTraceOutstandingMove:
		if method != "create" {
			return nil, false, nil
		}
		snap.Kind = "outstanding"
		snap.TaskID = object.TaskID
		snap.OtherTaskID = object.TargetTaskID
	case *TaskRelation:
		if method != "create" && method != "delete" {
			return nil, false, nil
		}
		snap.Kind = "relation"
		snap.TaskID = object.TaskID
		snap.OtherTaskID = object.OtherTaskID
		snap.RelationKind = object.RelationKind
	case *TaskPosition:
		if method != "update" {
			return nil, false, nil
		}
		snap.Kind = "position"
		snap.TaskID = object.TaskID
		snap.ViewID = object.ProjectViewID
	default:
		return nil, false, nil
	}
	if err := snap.lockScope(s); err != nil {
		return nil, true, err
	}
	before, err := snap.readState(s)
	if err != nil {
		return nil, true, err
	}
	snap.Before = before
	return snap, true, nil
}
func (snap *taskTraceUndoCapture) readState(s *xorm.Session) (taskTraceUndoState, error) {
	state := taskTraceUndoState{Parts: map[string][]taskTraceUndoRow{}}
	put := func(name string, cond builder.Cond) error {
		rows, err := taskTraceUndoReadRows(s, taskTraceUndoBean(name), cond)
		return taskTraceUndoPart(&state, name, rows, err)
	}
	switch snap.Kind {
	case "task":
		var err error
		state.Task, err = taskTraceUndoReadTask(s, snap.TaskID)
		if err != nil {
			return state, err
		}
		// Only this project can be renumbered by task writes. Existing positions of
		// this task in saved-filter views are included for delete/restore as well.
		if err = put("positions", builder.Or(builder.In("project_view_id", builder.Select("id").From("project_views").Where(builder.Eq{"project_id": snap.ProjectID})), builder.Eq{"task_id": snap.TaskID})); err != nil {
			return state, err
		}
		for _, name := range []string{"buckets", "reminders", "assignees"} {
			if err = put(name, builder.Eq{"task_id": snap.TaskID}); err != nil {
				return state, err
			}
		}
		if err = put("favorites", builder.Eq{"entity_id": snap.TaskID, "user_id": snap.ActorID, "kind": FavoriteKindTask}); err != nil {
			return state, err
		}
		if snap.Method != "update" {
			for _, name := range []string{"comments", "attachments", "labels", "time_entries"} {
				if err = put(name, builder.Eq{"task_id": snap.TaskID}); err != nil {
					return state, err
				}
			}
			if err = put("relations", builder.Or(builder.Eq{"task_id": snap.TaskID}, builder.Eq{"other_task_id": snap.TaskID})); err != nil {
				return state, err
			}
			if err = put("reactions", builder.Eq{"entity_id": snap.TaskID, "entity_kind": ReactionKindTask}); err != nil {
				return state, err
			}
		}
	case "comment":
		if err := put("comments", builder.Eq{"id": snap.CommentID}); err != nil {
			return state, err
		}
		if err := put("reactions", builder.Eq{"entity_id": snap.CommentID, "entity_kind": ReactionKindComment}); err != nil {
			return state, err
		}
	case "outstanding":
		comments := []taskTraceUndoRow{}
		legacy := []taskTraceUndoRow{}
		for index, id := range []int64{snap.TaskID, snap.OtherTaskID} {
			if index == 1 && snap.TaskID == id {
				continue
			}
			var history []*TaskComment
			if err := lockingSession(s).Where("task_id = ?", id).Find(&history); err != nil {
				return state, err
			}
			shared, err := taskTraceParseOutstanding(history)
			if err != nil {
				return state, err
			}
			if shared.comment != nil {
				row, err := taskTraceUndoRowOf(s, shared.comment)
				if err != nil {
					return state, err
				}
				comments = append(comments, row)
			}
			for _, note := range history {
				if strings.Contains(note.Comment, taskTraceOutstandingHeading) {
					continue
				}
				row, err := taskTraceUndoRowOf(s, note)
				if err != nil {
					return state, err
				}
				legacy = append(legacy, row)
			}
		}
		taskTraceUndoSortRows(comments)
		taskTraceUndoSortRows(legacy)
		state.Parts["comments"] = comments
		state.Parts["legacy"] = legacy
	case "move":
		if err := put("relations", builder.Or(builder.Eq{"task_id": snap.TaskID, "relation_kind": RelationKindParenttask}, builder.Eq{"other_task_id": snap.TaskID, "relation_kind": RelationKindSubtask})); err != nil {
			return state, err
		}
		if err := put("positions", builder.Eq{"project_view_id": snap.ViewID}); err != nil {
			return state, err
		}
	case "position":
		if err := put("positions", builder.Eq{"project_view_id": snap.ViewID}); err != nil {
			return state, err
		}
	case "relation":
		if err := put("relations", builder.Or(builder.Eq{"task_id": snap.TaskID, "other_task_id": snap.OtherTaskID, "relation_kind": snap.RelationKind}, builder.Eq{"task_id": snap.OtherTaskID, "other_task_id": snap.TaskID, "relation_kind": getInverseRelation(snap.RelationKind)})); err != nil {
			return state, err
		}
	}
	return state, nil
}

func finishTaskTraceUndo(s *xorm.Session, a web.Auth, method string, model any, before any) (string, string, []int64, bool, error) {
	snap, ok := before.(*taskTraceUndoCapture)
	if !ok || snap == nil {
		return "", "", nil, false, nil
	}
	switch object := model.(type) {
	case *Task:
		if method == "create" {
			snap.TaskID = object.ID
		}
	case *TaskComment:
		if method == "create" {
			snap.CommentID = object.ID
		}
	}
	after, err := snap.readState(s)
	if err != nil {
		return "", "", nil, false, err
	}
	snap.After = after
	if snap.Kind == "task" && snap.Method == "update" {
		for field, old := range snap.Before.Task {
			if !bytes.Equal(old, snap.After.Task[field]) {
				snap.Fields = append(snap.Fields, field)
			}
		}
		sort.Strings(snap.Fields)
		for _, field := range snap.Fields {
			if field == "ProjectID" || field == "Index" || field == "UID" || field == "CreatedByID" || field == "DeletedAt" {
				snap.Barrier = "跨项目移动等任务结构操作暂不支持撤销，请在当前操作中手动恢复。"
			}
		}
	}
	if snap.Kind == "task" && snap.Method != "update" {
		snap.Guards = []string{"comments", "attachments", "labels", "relations", "time_entries", "reactions", "reminders", "assignees", "favorites"}
	}
	if snap.Kind == "comment" && snap.Method != "update" {
		snap.Guards = []string{"reactions"}
	}
	if snap.Kind == "outstanding" && len(snap.Before.Parts["comments"]) < len(snap.After.Parts["comments"]) {
		snap.Guards = []string{"legacy"}
	}
	isGuard := func(name string) bool {
		for _, guard := range snap.Guards {
			if guard == name {
				return true
			}
		}
		return false
	}
	for name, rows := range snap.After.Parts {
		if name == "legacy" || isGuard(name) {
			continue
		}
		if !taskTraceUndoSame(name, snap.Before.Parts[name], rows) {
			snap.Parts = append(snap.Parts, name)
		}
	}
	sort.Strings(snap.Parts)
	if snap.Kind == "task" {
		changed := map[int64]bool{}
		oldPos := map[string]taskTraceUndoRow{}
		newPos := map[string]taskTraceUndoRow{}
		for _, row := range snap.Before.Parts["positions"] {
			oldPos[taskTraceUndoRowKey("positions", row)] = row
		}
		for _, row := range snap.After.Parts["positions"] {
			newPos[taskTraceUndoRowKey("positions", row)] = row
		}
		for key, row := range oldPos {
			if !taskTraceUndoSame("positions", []taskTraceUndoRow{row}, []taskTraceUndoRow{newPos[key]}) {
				changed[taskTraceUndoInt(row, "ProjectViewID")] = true
			}
		}
		for key, row := range newPos {
			if !taskTraceUndoSame("positions", []taskTraceUndoRow{row}, []taskTraceUndoRow{oldPos[key]}) {
				changed[taskTraceUndoInt(row, "ProjectViewID")] = true
			}
		}
		for id := range changed {
			snap.ViewIDs = append(snap.ViewIDs, id)
		}
		sort.Slice(snap.ViewIDs, func(i, j int) bool { return snap.ViewIDs[i] < snap.ViewIDs[j] })
		snap.Before.Parts["positions"] = taskTraceUndoFilterViews(snap.Before.Parts["positions"], snap.ViewIDs)
		snap.After.Parts["positions"] = taskTraceUndoFilterViews(snap.After.Parts["positions"], snap.ViewIDs)
	}
	changed := len(snap.Parts) > 0 || len(snap.Fields) > 0
	if snap.Kind == "task" && snap.Method != "update" {
		changed = !taskTraceUndoSame("task", []taskTraceUndoRow{snap.Before.Task}, []taskTraceUndoRow{snap.After.Task})
	}
	if !changed {
		return "", "", nil, false, nil
	}
	ids := []int64{snap.TaskID}
	if snap.OtherTaskID != 0 && snap.OtherTaskID != snap.TaskID {
		ids = append(ids, snap.OtherTaskID)
	}
	labels := map[string]string{"task": "修改任务", "comment": "修改进展或遗留事项", "move": "移动任务", "position": "调整任务顺序", "relation": "调整任务层级", "outstanding": "移动遗留事项"}
	label := labels[snap.Kind]
	if snap.Kind == "task" && method == "create" {
		label = "新增任务"
	}
	if snap.Kind == "task" && method == "delete" {
		label = "删除任务"
	}
	// Keep only relevant fields and parts on disk. Large attachment metadata and
	// ordinary comment bodies are retained only when guarding task life cycles.
	if snap.Kind == "task" && method == "update" {
		old, newRow := taskTraceUndoRow{}, taskTraceUndoRow{}
		for _, field := range snap.Fields {
			old[field] = snap.Before.Task[field]
			newRow[field] = snap.After.Task[field]
		}
		snap.Before.Task, snap.After.Task = old, newRow
	}
	keep := map[string]bool{}
	for _, name := range append(snap.Parts, snap.Guards...) {
		keep[name] = true
	}
	for name := range snap.Before.Parts {
		if !keep[name] {
			delete(snap.Before.Parts, name)
		}
	}
	for name := range snap.After.Parts {
		if !keep[name] {
			delete(snap.After.Parts, name)
		}
	}
	raw, err := json.Marshal(snap)
	return string(raw), label, ids, true, err
}
func taskTraceUndoFilterViews(rows []taskTraceUndoRow, ids []int64) []taskTraceUndoRow {
	keep := map[int64]bool{}
	for _, id := range ids {
		keep[id] = true
	}
	result := []taskTraceUndoRow{}
	for _, row := range rows {
		if keep[taskTraceUndoInt(row, "ProjectViewID")] {
			result = append(result, row)
		}
	}
	return result
}
func taskTraceUndoRowKey(kind string, row taskTraceUndoRow) string {
	switch kind {
	case "positions", "buckets":
		return fmt.Sprintf("%d/%d", taskTraceUndoInt(row, "TaskID"), taskTraceUndoInt(row, "ProjectViewID"))
	case "favorites":
		return fmt.Sprintf("%d/%d/%d", taskTraceUndoInt(row, "EntityID"), taskTraceUndoInt(row, "UserID"), taskTraceUndoInt(row, "Kind"))
	default:
		return fmt.Sprint(taskTraceUndoInt(row, "ID"))
	}
}
