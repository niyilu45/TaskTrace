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

func TestTaskRelationImportedCollaborationSubtaskPermissions(t *testing.T) {
	for _, write := range []bool{false, true} {
		name := "read-only member"
		if write {
			name = "writable member"
		}
		t.Run(name, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			repository, data := t.TempDir(), t.TempDir()
			t.Setenv("TASKTRACE_TEAM_ROOT", repository)
			t.Setenv("TASKTRACE_DATA_ROOT", data)
			actor := &user.User{ID: 1, Username: "user1"}
			binding := TaskTraceTeamBinding{
				ShareID: "received", Repository: repository, Owner: "user2", Members: []string{"user1", "user2"},
				RootTaskID: 1, NodeTasks: map[string]int64{"root": 1},
			}
			require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding}}))
			manifest := TaskTraceTeamManifest{
				Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Owner: binding.Owner, Members: binding.Members,
				Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {
					{Username: "user2", Owner: true, Read: true, Write: true},
					{Username: "user1", Read: true, Write: write},
				}},
			}
			manifestPath := filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json")
			require.NoError(t, taskTraceTeamWriteJSON(manifestPath, &manifest))
			s := db.NewSession()
			defer s.Close()
			child := &Task{Title: "New local child", ProjectID: 1}
			canCreate, err := child.CanCreate(s, actor)
			require.NoError(t, err)
			require.True(t, canCreate, "the imported project is locally writable even when its shared task is not")
			require.NoError(t, child.Create(s, actor))
			relation := &TaskRelation{TaskID: 1, OtherTaskID: child.ID, RelationKind: RelationKindSubtask}
			allowed, err := relation.CanCreate(s, actor)
			require.NoError(t, err)
			require.Equal(t, write, allowed, "local project ownership must not bypass the shared parent's permission")
			if !write {
				return
			}
			require.NoError(t, relation.Create(s, actor))
			require.NoError(t, s.Commit())
			var count int64
			count, err = s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", 1, child.ID, RelationKindSubtask).Count(&TaskRelation{})
			require.NoError(t, err)
			require.EqualValues(t, 1, count)
			manifest.Permissions["root"][1].Write = false
			require.NoError(t, taskTraceTeamWriteJSON(manifestPath, &manifest))
			allowed, err = relation.CanCreate(s, actor)
			require.NoError(t, err)
			require.False(t, allowed, "revoked write access is effective without restarting")
			require.NoError(t, os.Remove(manifestPath))
			allowed, err = relation.CanCreate(s, actor)
			require.NoError(t, err)
			require.False(t, allowed, "an offline shared parent cannot silently fall back to local project ownership")
		})
	}
}
