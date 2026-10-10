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

package websocket

import (
	"encoding/json"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"
	"github.com/ThreeDotsLabs/watermill/message"
)

// TaskTraceChangeListener invalidates the actor's other local views after commit.
// Only identifiers are sent, never task content or another user's notifications.
type TaskTraceChangeListener struct{}

func (*TaskTraceChangeListener) Name() string { return "websocket.tasktrace.changed" }

type taskTraceChange struct {
	TaskID        int64 `json:"task_id"`
	ProjectID     int64 `json:"project_id"`
	RelatedTaskID int64 `json:"related_task_id,omitempty"`
}

func (*TaskTraceChangeListener) Handle(msg *message.Message) error {
	var event struct {
		Task     *models.Task         `json:"task"`
		Doer     *user.User           `json:"doer"`
		Relation *models.TaskRelation `json:"relation"`
	}
	if err := json.Unmarshal(msg.Payload, &event); err != nil {
		return err
	}
	hub := GetHub()
	if hub == nil || event.Task == nil || event.Doer == nil || event.Doer.ID <= 0 {
		return nil
	}
	change := taskTraceChange{
		TaskID:    event.Task.ID,
		ProjectID: event.Task.ProjectID,
	}
	if event.Relation != nil {
		change.RelatedTaskID = event.Relation.OtherTaskID
	}
	hub.PublishForUser(event.Doer.ID, "tasktrace.task.changed", change)
	return nil
}
func registerTaskTraceChangeListeners() {
	for _, name := range []string{
		(&models.TaskTraceTaskChangedEvent{}).Name(),
		(&models.TaskCreatedEvent{}).Name(),
		(&models.TaskUpdatedEvent{}).Name(),
		(&models.TaskDeletedEvent{}).Name(),
		(&models.TaskCommentCreatedEvent{}).Name(),
		(&models.TaskCommentUpdatedEvent{}).Name(),
		(&models.TaskCommentDeletedEvent{}).Name(),
		(&models.TaskAttachmentCreatedEvent{}).Name(),
		(&models.TaskAttachmentDeletedEvent{}).Name(),
		(&models.TaskRelationCreatedEvent{}).Name(),
		(&models.TaskRelationDeletedEvent{}).Name(),
		(&models.TaskAssigneeCreatedEvent{}).Name(),
		(&models.TaskAssigneeDeletedEvent{}).Name(),
	} {
		events.RegisterListener(name, &TaskTraceChangeListener{})
	}
}
