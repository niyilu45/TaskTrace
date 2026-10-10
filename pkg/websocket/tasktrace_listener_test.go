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
	"testing"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceChangeListener(t *testing.T) {
	task := &models.Task{
		ID:        41,
		ProjectID: 9,
		Title:     "Private content",
	}
	actor := &user.User{ID: 1}
	for _, event := range []events.Event{
		&models.TaskUpdatedEvent{
			Task: task,
			Doer: actor,
		},
		&models.TaskCommentCreatedEvent{
			Task: task,
			Doer: actor,
		},
		&models.TaskCommentUpdatedEvent{
			Task: task,
			Doer: actor,
		},
		&models.TaskCommentDeletedEvent{
			Task: task,
			Doer: actor,
		},
	} {
		t.Run(event.Name(), func(t *testing.T) {
			InitHub()
			mine := &Connection{
				userID:        1,
				subscriptions: map[string]bool{"tasktrace.task.changed": true},
				send:          make(chan OutgoingMessage, 4),
			}
			other := &Connection{
				userID:        2,
				subscriptions: map[string]bool{"tasktrace.task.changed": true},
				send:          make(chan OutgoingMessage, 4),
			}
			GetHub().Register(mine)
			GetHub().Register(other)
			events.TestListener(t, event, &TaskTraceChangeListener{})
			require.Len(t, mine.send, 1)
			require.Empty(t, other.send)
			msg := <-mine.send
			require.Equal(t, "tasktrace.task.changed", msg.Event)
			require.Equal(t, taskTraceChange{
				TaskID:    41,
				ProjectID: 9,
			}, msg.Data)
		})
	}
	require.True(t, isValidEvent("tasktrace.task.changed"))
	events.TestListener(t, &models.TaskUpdatedEvent{}, &TaskTraceChangeListener{})
}
