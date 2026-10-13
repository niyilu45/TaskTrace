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
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamCommentSyncPreservesEventTime(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	t.Cleanup(func() { _ = s.Close() })
	root := &Task{Title: "Comment synchronization", ProjectID: 1}
	require.NoError(t, root.Create(s, actor))
	binding := &TaskTraceTeamBinding{ShareID: "comment-sync"}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	original := &TaskComment{TaskID: root.ID, Comment: taskTraceTeamAddMarker("<p>first</p>", "daily", "user2"), Created: start, Updated: start}
	require.NoError(t, original.CreateWithTimestamps(s, actor))
	for index, body := range []string{"<p>second</p>", "<p>third</p>"} {
		event := TaskTraceTeamComment{ID: "daily", Body: body, Author: "user2", Created: start, Updated: start.Add(time.Duration(index+1) * time.Second)}
		require.NoError(t, taskTraceTeamMergeComments(s, actor, binding, "root", "user1", root.ID, []TaskTraceTeamComment{event}))
		stored := &TaskComment{}
		found, err := s.ID(original.ID).Get(stored)
		require.NoError(t, err)
		require.True(t, found)
		require.Contains(t, stored.Comment, body)
		require.True(t, stored.Updated.Equal(event.Updated), "importing a remote update must not replace its event time with the receiver clock: %s != %s", stored.Updated, event.Updated)
	}
}

func TestTaskTraceTeamCommentSyncConvergesTimestampTies(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	t.Cleanup(func() { _ = s.Close() })
	root := &Task{Title: "Same second update", ProjectID: 1}
	require.NoError(t, root.Create(s, actor))
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	body := taskTraceTeamAddMarker("<p>a</p>", "daily", "user2")
	original := &TaskComment{TaskID: root.ID, Comment: body, Created: stamp, Updated: stamp}
	require.NoError(t, original.CreateWithTimestamps(s, actor))
	candidate := TaskTraceTeamComment{ID: "daily", Body: taskTraceTeamAddMarker("<p>z</p>", "daily", "user2"), Author: "user2", Created: stamp, Updated: stamp}
	current := TaskTraceTeamComment{ID: "daily", Body: body, Author: "user2", Created: stamp, Updated: stamp}
	require.True(t, taskTraceTeamCommentEventSupersedes(candidate, current))
	require.NoError(t, taskTraceTeamMergeComments(s, actor, &TaskTraceTeamBinding{}, "root", "user1", root.ID, []TaskTraceTeamComment{candidate}))
	stored := &TaskComment{}
	_, err := s.ID(original.ID).Get(stored)
	require.NoError(t, err)
	require.Equal(t, candidate.Body, stored.Comment)
}

func TestTaskTraceTeamCommentSyncSameBodyKeepsNewerEvent(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	t.Cleanup(func() { _ = s.Close() })
	root := &Task{Title: "Restored same body", ProjectID: 1}
	require.NoError(t, root.Create(s, actor))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	body := taskTraceTeamAddMarker("<p>same body</p>", "daily", "user2")
	original := &TaskComment{TaskID: root.ID, Comment: body, Created: start, Updated: start}
	require.NoError(t, original.CreateWithTimestamps(s, actor))
	latest := TaskTraceTeamComment{ID: "daily", Body: body, Author: "user2", Created: start, Updated: start.Add(2 * time.Second)}
	stale := TaskTraceTeamComment{ID: "daily", Body: "<p>intermediate edit</p>", Author: "user2", Created: start, Updated: start.Add(time.Second)}
	for _, event := range []TaskTraceTeamComment{latest, stale, latest} {
		require.NoError(t, taskTraceTeamMergeComments(s, actor, &TaskTraceTeamBinding{}, "root", "user1", root.ID, []TaskTraceTeamComment{event}))
		stored := &TaskComment{}
		_, err := s.ID(original.ID).Get(stored)
		require.NoError(t, err)
		require.Equal(t, body, stored.Comment)
		require.True(t, stored.Updated.Equal(latest.Updated))
	}
}
