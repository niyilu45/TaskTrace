// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"golang.org/x/net/html"
	"xorm.io/xorm"
)

// Personal priority belongs to the local copy, even when shared content is read-only.
// Use a dedicated mutation so a priority update cannot smuggle changes to shared fields.
type TaskTracePersonalPriority struct {
	TaskID          int64  `json:"task_id" readOnly:"true" doc:"The local task id, supplied by the URL."`
	OutstandingID   string `json:"outstanding_id,omitempty" maxLength:"200" doc:"An existing outstanding item id; empty changes the task priority."`
	Priority        int64  `json:"priority" minimum:"0" maximum:"9" doc:"Personal display priority: 0 is highest and 9 is lowest."`
	StoredPriority  int64  `json:"stored_priority" readOnly:"true" doc:"The local task-priority encoding used by normal task responses."`
	CommentID       int64  `json:"comment_id,omitempty" readOnly:"true" doc:"The current outstanding-list comment, when an item priority was changed."`
	web.CRUDable    `json:"-" xorm:"-"`
	web.Permissions `json:"-" xorm:"-"`
}

func (p *TaskTracePersonalPriority) CanUpdate(s *xorm.Session, a web.Auth) (bool, error) {
	if !taskTraceTeamEnabled() {
		return false, nil
	}
	task, err := GetTaskByIDSimple(s, p.TaskID)
	if err != nil {
		return false, err
	}
	// The imported local project is writable by its local user. This does not
	// grant write access to the remote task or to another user's private project.
	return (&Project{ID: task.ProjectID}).CanWrite(s, a)
}

func (p *TaskTracePersonalPriority) Update(s *xorm.Session, a web.Auth) error {
	if p.Priority < 0 || p.Priority > 9 {
		return ErrTaskTraceOutstandingMove{Reason: "Priority must be between 0 and 9"}
	}
	p.StoredPriority = 10 - p.Priority
	if p.OutstandingID == "" {
		changed, err := s.ID(p.TaskID).Where("priority IS NULL OR priority <> ?", p.StoredPriority).NoAutoTime().Cols("priority").Update(&Task{Priority: p.StoredPriority})
		if err == nil && changed > 0 {
			p.notifyLocalViews(s, a)
		}
		return err
	}
	list, err := taskTraceReadOutstanding(lockingSession(s), p.TaskID)
	if err != nil {
		return err
	}
	if list.comment == nil {
		return ErrTaskTraceOutstandingMove{Reason: "Outstanding item no longer exists; refresh the task"}
	}
	doc, nodes := taskTraceTeamOutstandingNodes(list.original)
	found := false
	for _, node := range nodes {
		if taskTraceAttribute(node, "data-id") != p.OutstandingID {
			continue
		}
		found = true
		changed := false
		for i := range node.Attr {
			if node.Attr[i].Key == "data-priority" {
				node.Attr[i].Val = strconv.FormatInt(p.Priority, 10)
				changed = true
			}
		}
		if !changed {
			node.Attr = append(node.Attr, html.Attribute{Key: "data-priority", Val: strconv.FormatInt(p.Priority, 10)})
		}
	}
	if !found {
		return ErrTaskTraceOutstandingMove{Reason: "Outstanding item no longer exists; refresh the task"}
	}
	p.CommentID = list.comment.ID
	body := taskTraceTeamRenderDocument(doc)
	// Personal preferences do not change comment authors, timestamps, activity,
	// notifications or attachment metadata. The normal sync preserves this attr.
	if body == list.original {
		return nil
	}
	changed, err := s.ID(p.CommentID).Where("comment = ?", list.original).NoAutoTime().Cols("comment").Update(&TaskComment{Comment: body})
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrTaskTraceOutstandingMove{Reason: "The outstanding list changed while saving priority; refresh and retry"}
	}
	p.notifyLocalViews(s, a)
	return nil
}

func taskTraceDefaultDisplayPriority() int64 {
	value := int64(7)
	var settings struct {
		DefaultPriority *int64 `json:"default_priority"`
	}
	dataRoot := os.Getenv("TASKTRACE_DATA_ROOT")
	if dataRoot == "" {
		return value
	}
	path := filepath.Join(dataRoot, "autosave.json")
	//nolint:gosec // Fixed settings filename under the trusted process-configured data directory, never a request path.
	if raw, err := os.ReadFile(path); err == nil && json.Unmarshal(raw, &settings) == nil && settings.DefaultPriority != nil && *settings.DefaultPriority >= 0 && *settings.DefaultPriority <= 9 {
		value = *settings.DefaultPriority
	}
	return value
}

func taskTracePersonalPriorityValue(s *xorm.Session, taskID int64, itemID string) (int64, error) {
	if itemID == "" {
		task := &Task{}
		found, err := lockingSession(s).ID(taskID).Get(task)
		if err != nil {
			return 0, err
		}
		if !found {
			return 0, ErrTaskDoesNotExist{ID: taskID}
		}
		if task.Priority >= 1 && task.Priority <= 10 {
			return 10 - task.Priority, nil
		}
		return 9, nil
	}
	list, err := taskTraceReadOutstanding(lockingSession(s), taskID)
	if err != nil {
		return 0, err
	}
	_, nodes := taskTraceTeamOutstandingNodes(list.original)
	for _, node := range nodes {
		if taskTraceAttribute(node, "data-id") == itemID {
			value, err := strconv.ParseInt(taskTraceAttribute(node, "data-priority"), 10, 64)
			if err == nil && value >= 0 && value <= 9 {
				return value, nil
			}
			return taskTraceDefaultDisplayPriority(), nil
		}
	}
	return 0, fmt.Errorf("outstanding item %s no longer exists", itemID)
}

func (p *TaskTracePersonalPriority) notifyLocalViews(s *xorm.Session, a web.Auth) {
	task, err := GetTaskByIDSimple(s, p.TaskID)
	if err != nil {
		return
	}
	doer, err := user.GetFromAuth(a)
	if err != nil {
		return
	}
	events.DispatchOnCommit(s, &TaskTraceTaskChangedEvent{Task: &task, Doer: doer})
}
