// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"path/filepath"
	"testing"
	"time"

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

	ownerAllowed, err := taskTraceTeamCanWriteLocalTask(&user.User{ID: 1, Username: "owner"}, 1)
	require.NoError(t, err)
	require.True(t, ownerAllowed, "the collaboration owner must retain write access")

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

	// A team mutation already owns this mutex. Permission checks performed from
	// that mutation, and ordinary saves running beside a slow sync, must not try
	// to acquire it again and deadlock the server.
	taskTraceTeamMu.Lock()
	result := make(chan struct {
		allowed bool
		err     error
	}, 1)
	go func() {
		canWrite, checkErr := taskTraceTeamCanWriteLocalTask(actor, 1)
		result <- struct {
			allowed bool
			err     error
		}{canWrite, checkErr}
	}()
	select {
	case check := <-result:
		taskTraceTeamMu.Unlock()
		require.NoError(t, check.err)
		require.True(t, check.allowed)
	case <-time.After(2 * time.Second):
		taskTraceTeamMu.Unlock()
		t.Fatal("live collaboration permission check waited on the team mutation mutex")
	}
}

func TestTaskTraceTeamConfigurePermissionsPersistsMemberWrite(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	root := t.TempDir()
	dataRoot := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", root)
	t.Setenv("TASKTRACE_DATA_ROOT", dataRoot)
	binding := TaskTraceTeamBinding{
		ShareID: "permissions", Repository: root, Owner: "user1", Members: []string{"user1", "user2"},
		RootTaskID: 1, NodeTasks: map[string]int64{"root": 1}, Base: map[string]TaskTraceTeamBase{"root": {}},
	}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(dataRoot, "team-sync.json"), &taskTraceTeamState{
		Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding},
	}))
	manifestPath := filepath.Join(taskTraceTeamShareDir(root, binding.ShareID), "manifest.json")
	manifest := TaskTraceTeamManifest{
		Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Owner: binding.Owner, Members: binding.Members,
		Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {
			{Username: "user1", Read: true, Write: true, Owner: true},
			{Username: "user2", Read: true},
		}},
	}
	require.NoError(t, taskTraceTeamWriteJSON(manifestPath, &manifest))

	s := db.NewSession()
	defer s.Close()
	_, err := TaskTraceTeamConfigurePermissions(s, &user.User{ID: 1, Username: "user1"}, TaskTraceTeamPermissionsRequest{
		ShareID: binding.ShareID, TaskID: 1,
		Permissions: []TaskTraceTeamPermissionUpdate{{Username: "user2", Read: true, Write: true}},
	})
	require.NoError(t, err)
	require.NoError(t, taskTraceTeamReadJSON(manifestPath, &manifest))
	require.True(t, taskTraceTeamPermissionMap(manifest.Permissions["root"])["user2"].Write)
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
