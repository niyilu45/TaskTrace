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

func TestTaskTraceTeamSyncAccountAliases(t *testing.T) {
	for _, receiver := range []string{"user1", "user2"} {
		t.Run(receiver, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
			repository := t.TempDir()
			t.Setenv("TASKTRACE_TEAM_ROOT", repository)
			actor := &user.User{ID: 1, Username: receiver}
			sender := "user2"
			if receiver == "user2" {
				actor.ID = 2
				sender = "user1"
			}
			s := db.NewSession()
			project := &Project{Title: "Receiver"}
			require.NoError(t, project.Create(s, actor))
			task := &Task{Title: "Shared task", ProjectID: project.ID}
			require.NoError(t, task.Create(s, actor))
			require.NoError(t, s.Commit())
			require.NoError(t, s.Close())
			binding := TaskTraceTeamBinding{Repository: repository, ShareID: "aliases", Secret: "test", Owner: "user1", RootTaskID: task.ID, ProjectID: project.ID,
				NodeTasks: map[string]int64{"root": task.ID}, Base: map[string]TaskTraceTeamBase{"root": {Title: task.Title, Status: TaskStatusTodo, CompletionKnown: true}}}
			manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: []string{`CHINA\user1`, "user2@example.test"}, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: binding.Base,
				Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {{Username: "user1", Read: true, Write: true, Owner: true}, {Username: "user2", Read: true, Write: true}}}}
			require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), manifest))
			require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: receiver, Bindings: []TaskTraceTeamBinding{binding}}))
			now := time.Now().UTC().Truncate(time.Second)
			remote := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: sender, DeviceID: sender, Updated: now, Base: binding.Base,
				Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: task.Title, Status: TaskStatusTodo,
					Comments: []TaskTraceTeamComment{{ID: "new-progress", Author: sender, Body: "<h3>Daily progress</h3><p>New member update</p>", Created: now, Updated: now}}}}}
			require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
			for range 2 {
				sync := db.NewSession()
				status, err := TaskTraceTeamSync(sync, actor)
				require.NoError(t, err)
				require.Empty(t, status.Bindings[0].LastError)
				require.NoError(t, sync.Commit())
				require.NoError(t, sync.Close())
			}
			read := db.NewReadSession()
			defer read.Close()
			var comments []*TaskComment
			require.NoError(t, read.Where("task_id = ?", task.ID).Find(&comments))
			require.Len(t, comments, 1)
			require.Contains(t, comments[0].Comment, "New member update")
		})
	}
}

func TestTaskTraceTeamValidateBindingRejectsOtherAccounts(t *testing.T) {
	binding := TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "membership", Secret: "test"}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, TokenHash: taskTraceTeamTokenHash(binding.Secret), Members: []string{`CHINA\user1`, "user2@example.test"}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json"), manifest))
	require.NoError(t, taskTraceTeamValidateBinding(&binding, "USER1"))
	require.NoError(t, taskTraceTeamValidateBinding(&binding, "user2"))
	require.Error(t, taskTraceTeamValidateBinding(&binding, "user3"))
	binding.Secret = "incorrect"
	require.Error(t, taskTraceTeamValidateBinding(&binding, "user1"))
}
