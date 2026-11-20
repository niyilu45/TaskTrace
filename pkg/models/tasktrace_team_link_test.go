// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"os"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamShareLinkOwnerAuthority(t *testing.T) {
	repository := t.TempDir()
	info := map[string]any{
		"path":  "\\\\OWNER\\teamData",
		"paths": []string{"\\\\10.0.0.8\\teamData", "\\\\OWNER\\teamData"},
	}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(repository, "repository-info.json"), info))
	owner := TaskTraceTeamBinding{
		Repository: repository,
		ShareID:    "same-task",
		Secret:     "private-test-secret",
		Owner:      "owner",
	}
	require.NoError(t, taskTraceTeamPublishLink(&owner, "OWNER"))
	expected := taskTraceTeamEncodeLinkPaths("\\\\OWNER\\teamData", []string{"\\\\10.0.0.8\\teamData"}, owner.ShareID, owner.Secret)
	require.Equal(t, expected, taskTraceTeamBindingLink(&owner))

	member := owner
	member.Repository = repository + string(os.PathSeparator) + "."
	member.ShareLink = taskTraceTeamEncodeLink("\\\\10.0.0.8\\teamData", owner.ShareID, owner.Secret)
	require.NotEqual(t, expected, member.ShareLink)
	require.Equal(t, expected, taskTraceTeamBindingLink(&member))

	path := filepath.Join(taskTraceTeamShareDir(repository, owner.ShareID), "share-link.json")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(before), owner.Secret)
	require.NoError(t, taskTraceTeamPublishLink(&member, "member"))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after, "a member cannot publish a replacement address")

	// An older peer knows only the manifest fields and may rewrite that file.
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, owner.ShareID), "manifest.json"), TaskTraceTeamManifest{
		Schema:  taskTraceTeamSchema,
		ShareID: owner.ShareID,
		Owner:   owner.Owner,
	}))
	require.Equal(t, expected, taskTraceTeamBindingLink(&member))
	stat, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, taskTraceTeamPublishLink(&owner, owner.Owner))
	statAfter, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, stat.ModTime(), statAfter.ModTime(), "unchanged links must not be rewritten")

	owner.Secret = "rotated-secret"
	fallback := taskTraceTeamEncodeLinkPaths("\\\\OWNER\\teamData", []string{"\\\\10.0.0.8\\teamData"}, owner.ShareID, owner.Secret)
	require.Equal(t, fallback, taskTraceTeamBindingLink(&owner), "a revoked cached link must not be reused")

	require.NoError(t, os.Rename(path, path+".old"))
	require.Equal(t, expected, taskTraceTeamBindingLink(&member), "legacy shares use the owner repository metadata")
}

func TestTaskTraceTeamShareLinkThroughSyncAndImport(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	repository := t.TempDir()
	ownerData, memberData := t.TempDir(), t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	t.Setenv("TASKTRACE_DATA_ROOT", ownerData)
	owner := &user.User{ID: 1, Username: "user1"}
	member := &user.User{ID: 2, Username: "user2"}
	binding := TaskTraceTeamBinding{
		ShareID:    "existing-share",
		Repository: repository,
		Secret:     "secret",
		Owner:      owner.Username,
		Members:    []string{owner.Username, member.Username},
		RootTaskID: 1,
		ProjectID:  1,
		NodeTasks:  map[string]int64{},
		Base:       map[string]TaskTraceTeamBase{},
	}
	s := db.NewSession()
	defer s.Close()
	task := &Task{Title: "Shared task for link test", ProjectID: 1}
	require.NoError(t, task.Create(s, owner))
	binding.RootTaskID = task.ID
	snapshot, err := taskTraceTeamBuildSnapshot(s, &binding, owner.Username, "owner-device")
	require.NoError(t, err)
	require.NotEmpty(t, snapshot.Tasks)
	manifest := TaskTraceTeamManifest{
		Schema:      taskTraceTeamSchema,
		ShareID:     binding.ShareID,
		RootNode:    snapshot.Tasks[0].NodeID,
		Owner:       owner.Username,
		Members:     binding.Members,
		TokenHash:   taskTraceTeamTokenHash(binding.Secret),
		Base:        taskTraceTeamBaseFromSnapshot(snapshot),
		Permissions: map[string][]TaskTraceTeamMemberPermission{},
	}
	for _, task := range snapshot.Tasks {
		manifest.Permissions[task.NodeID] = []TaskTraceTeamMemberPermission{
			{Username: owner.Username, Owner: true, Read: true, Write: true},
			{Username: member.Username, Read: true},
		}
	}
	require.NoError(t, taskTraceTeamWriteManifest(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), &manifest))
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, snapshot))
	binding.Base = taskTraceTeamCopyBase(manifest.Base)
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{
		Schema:   taskTraceTeamSchema,
		DeviceID: "owner-device",
		Bindings: []TaskTraceTeamBinding{binding},
	}))
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(repository, "repository-info.json"), map[string]any{
		"path":  repository,
		"paths": []string{repository, "\\\\OWNER\\teamData", "\\\\10.0.0.8\\teamData"},
	}))
	ownerStatus, err := TaskTraceTeamSync(s, owner)
	require.NoError(t, err)
	require.Len(t, ownerStatus.Bindings, 1)
	require.Empty(t, ownerStatus.Bindings[0].LastError)
	expected := ownerStatus.Bindings[0].Link
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())

	t.Setenv("TASKTRACE_DATA_ROOT", memberData)
	s = db.NewSession()
	defer s.Close()
	project := &Project{Title: "Member project"}
	require.NoError(t, project.Create(s, member))
	alias := repository + string(os.PathSeparator) + "."
	aliasLink := taskTraceTeamEncodeLink(alias, binding.ShareID, binding.Secret)
	require.NotEqual(t, expected, aliasLink)
	memberStatus, err := TaskTraceTeamImport(s, member, TaskTraceTeamImportRequest{
		ProjectID: project.ID,
		Link:      aliasLink,
	})
	require.NoError(t, err)
	require.Len(t, memberStatus.Bindings, 1)
	require.Equal(t, alias, memberStatus.Bindings[0].Repository)
	require.Equal(t, expected, memberStatus.Bindings[0].Link)
	require.NoError(t, s.Commit())

	state, err := taskTraceTeamLoadState()
	require.NoError(t, err)
	require.Len(t, state.Bindings, 1)
	require.Equal(t, expected, state.Bindings[0].ShareLink)
	require.Error(t, taskTraceTeamValidateBinding(&state.Bindings[0], "unrelated-user"))

	// Disconnecting the shared folder must preserve the last owner-issued link.
	tasks := filepath.Join(repository, "tasks")
	require.NoError(t, os.Rename(tasks, tasks+"-offline"))
	offline, err := taskTraceTeamStatusLockedFast(s, member, state)
	require.NoError(t, err)
	require.Len(t, offline.Bindings, 1)
	require.Equal(t, expected, offline.Bindings[0].Link)
}
