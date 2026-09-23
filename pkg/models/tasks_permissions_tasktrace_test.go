// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/require"
)

func TestTaskCanUpdateUsesLiveTaskTraceTeamPermission(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	root := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", root)
	t.Setenv("TASKTRACE_DATA_ROOT", dataRoot)

	binding := TaskTraceTeamBinding{
		ShareID:    "shared",
		Repository: root,
		Owner:      "owner",
		Members:    []string{"owner", "user1"},
		RootTaskID: 1,
		NodeTasks:  map[string]int64{"root": 1},
	}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(dataRoot, "team-sync.json"), &taskTraceTeamState{
		Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding},
	}))
	manifestPath := filepath.Join(taskTraceTeamShareDir(root, binding.ShareID), "manifest.json")
	manifest := TaskTraceTeamManifest{
		Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Owner: "owner", Members: binding.Members,
		Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {
			{Username: "owner", Read: true, Write: true, Owner: true},
			{Username: "user1", Read: true},
		}},
	}
	require.NoError(t, taskTraceTeamWriteJSON(manifestPath, &manifest))

	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	allowed, err := (&Task{ID: 1}).CanUpdate(s, actor)
	require.NoError(t, err)
	require.False(t, allowed, "ordinary project ownership must not bypass collaboration write permissions")

	manifest.Permissions["root"][1].Write = true
	require.NoError(t, taskTraceTeamWriteJSON(manifestPath, &manifest))
	allowed, err = (&Task{ID: 1}).CanUpdate(s, actor)
	require.NoError(t, err)
	require.True(t, allowed, "a newly granted write permission must work in the already-running process")
}

func TestTaskCanUpdateKeepsNonCollaborativeProjectPermission(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_TEAM_ROOT", t.TempDir())
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())

	s := db.NewSession()
	defer s.Close()
	allowed, err := (&Task{ID: 1}).CanUpdate(s, &user.User{ID: 1, Username: "user1"})
	require.NoError(t, err)
	require.True(t, allowed)
}
