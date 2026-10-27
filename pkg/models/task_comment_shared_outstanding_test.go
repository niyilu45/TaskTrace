// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceSharedOutstandingCurrentListPermission(t *testing.T) {
	actor, _, manifest, path := taskTraceSharedParentForSubtask(t, true)
	s := db.NewSession()
	defer s.Close()
	originalAuthor := &user.User{ID: 2, Username: "user2"}
	body := taskTraceTeamOutstandingHTML(map[string]string{"old": "Original item"}, []string{"old"})
	older := &TaskComment{TaskID: 1, Comment: body}
	require.NoError(t, older.Create(s, originalAuthor))
	current := &TaskComment{TaskID: 1, Comment: taskTraceTeamOutstandingHTML(map[string]string{"old": "Current item"}, []string{"old"})}
	require.NoError(t, current.Create(s, originalAuthor))
	ordinary := &TaskComment{TaskID: 1, Comment: "A personal comment"}
	require.NoError(t, ordinary.Create(s, originalAuthor))
	require.NoError(t, s.Commit())
	updatedBody := taskTraceTeamOutstandingHTML(map[string]string{"old": "Current item", "new": "Added by receiving member"}, []string{"old", "new"})
	for _, candidate := range []*TaskComment{
		{ID: older.ID, TaskID: 1, Comment: updatedBody},
		{ID: ordinary.ID, TaskID: 1, Comment: updatedBody},
		{ID: current.ID, TaskID: 1, Comment: "Replace shared list with a personal comment"},
	} {
		allowed, err := candidate.CanUpdate(s, actor)
		require.NoError(t, err)
		require.False(t, allowed, "only the current canonical list may be edited by another writer")
	}
	update := &TaskComment{ID: current.ID, TaskID: 1, Comment: updatedBody}
	allowed, err := update.CanDelete(s, actor)
	require.NoError(t, err)
	require.False(t, allowed, "shared list editing does not grant comment deletion")
	allowed, err = update.CanUpdate(s, actor)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, update.Update(s, actor))
	require.NoError(t, s.Commit())
	list, err := taskTraceReadOutstanding(s, 1)
	require.NoError(t, err)
	require.Len(t, list.items, 2)
	require.Contains(t, list.original, "Added by receiving member")
	manifest.Permissions["root"][1].Write = false
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	allowed, err = update.CanUpdate(s, actor)
	require.NoError(t, err)
	require.False(t, allowed, "read-only task recipients cannot mutate the list")
}
