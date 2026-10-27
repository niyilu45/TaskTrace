// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"path/filepath"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func taskTraceLifecyclePeers(t *testing.T) []*taskTraceSyncPeer {
	t.Helper()
	db.LoadAndAssertFixtures(t)
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	peers := []*taskTraceSyncPeer{{data: t.TempDir(), actor: &user.User{ID: 1, Username: "user1"}}, {data: t.TempDir(), actor: &user.User{ID: 2, Username: "user2"}}}
	base := map[string]TaskTraceTeamBase{"root": {Title: "Root", Status: TaskStatusTodo, CompletionKnown: true}, "child": {Title: "Child", Status: TaskStatusTodo, CompletionKnown: true}}
	permissions := []TaskTraceTeamMemberPermission{{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: "lifecycle", RootNode: "root", Owner: "user1", Members: []string{"user1", "user2"}, TokenHash: taskTraceTeamTokenHash("secret"), Base: base, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions, "child": permissions}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, manifest.ShareID), "manifest.json"), manifest))
	for _, peer := range peers {
		peer.activate(t)
		s := db.NewSession()
		project := &Project{Title: "Local task lifecycle " + peer.actor.Username}
		require.NoError(t, project.Create(s, peer.actor))
		root := &Task{Title: "Root", ProjectID: project.ID}
		child := &Task{Title: "Child", ProjectID: project.ID}
		require.NoError(t, root.Create(s, peer.actor))
		require.NoError(t, child.Create(s, peer.actor))
		require.NoError(t, (&TaskRelation{TaskID: root.ID, OtherTaskID: child.ID, RelationKind: RelationKindSubtask}).Create(s, peer.actor))
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		peer.binding = TaskTraceTeamBinding{Repository: repository, ShareID: manifest.ShareID, Secret: "secret", Owner: "user1", Members: manifest.Members, RootTaskID: root.ID, ProjectID: project.ID, NodeTasks: map[string]int64{"root": root.ID, "child": child.ID}, Base: taskTraceTeamCopyBase(base)}
		require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: peer.actor.Username, Bindings: []TaskTraceTeamBinding{peer.binding}}))
	}
	for _, peer := range peers {
		peer.sync(t)
	}
	return peers
}

func TestTaskTraceTeamTaskLifecycleDeleteOldSnapshotsAndUndo(t *testing.T) {
	peers := taskTraceLifecyclePeers(t)
	owner, member := peers[0], peers[1]
	var old TaskTraceTeamSnapshot
	require.NoError(t, taskTraceTeamReadJSON(taskTraceTeamSnapshotPath(&member.binding, member.actor.Username, member.actor.Username), &old))
	old.TaskStates = nil
	owner.edit(t, func(s *xorm.Session) {
		task := &Task{ID: owner.binding.NodeTasks["child"]}
		can, err := task.CanDelete(s, owner.actor)
		require.NoError(t, err)
		require.True(t, can)
		require.NoError(t, task.Delete(s, owner.actor))
	})
	owner.sync(t)
	require.True(t, owner.binding.TaskStates["child"].Deleted)
	require.NoError(t, taskTraceTeamWriteJSON(taskTraceTeamSnapshotPath(&member.binding, member.actor.Username, member.actor.Username), &old))
	member.sync(t)
	owner.sync(t)
	member.sync(t)
	for _, peer := range peers {
		s := db.NewSession()
		_, err := GetTaskByIDSimple(s, peer.binding.NodeTasks["child"])
		require.True(t, IsErrTaskDoesNotExist(err))
		require.NoError(t, s.Close())
		require.True(t, peer.binding.TaskStates["child"].Deleted)
	}
	// Simulate the existing undo operation: restore the same soft-deleted ID.
	owner.edit(t, func(s *xorm.Session) {
		_, err := s.Unscoped().Table("tasks").Where("id = ?", owner.binding.NodeTasks["child"]).Update(map[string]any{"deleted_at": nil})
		require.NoError(t, err)
	})
	owner.sync(t)
	require.False(t, owner.binding.TaskStates["child"].Deleted)
	require.Greater(t, owner.binding.TaskStates["child"].Version, uint64(1))
	member.sync(t)
	for _, peer := range peers {
		s := db.NewSession()
		task, err := GetTaskByIDSimple(s, peer.binding.NodeTasks["child"])
		require.NoError(t, err)
		require.Equal(t, "Child", task.Title)
		require.NoError(t, s.Close())
	}
}

func TestTaskTraceTeamTaskLifecycleRequiresExplicitWritableIntent(t *testing.T) {
	manifest := &TaskTraceTeamManifest{Owner: "owner", Members: []string{"owner", "reader"}, Permissions: map[string][]TaskTraceTeamMemberPermission{"child": {{Username: "owner", Read: true, Write: true}, {Username: "reader", Read: true}}}}
	binding := &TaskTraceTeamBinding{TaskStates: map[string]TaskTraceTeamTaskState{}}
	deleted := TaskTraceTeamTaskState{Version: 1, Deleted: true, Actor: "reader", EventID: "delete", Updated: time.Now().UTC()}
	states := taskTraceTeamTaskStatesFromSnapshots(binding, manifest, []TaskTraceTeamSnapshot{{Actor: "reader", TaskStates: map[string]TaskTraceTeamTaskState{"child": deleted}}})
	require.Empty(t, states, "read-only members cannot delete a shared task")
	forged := deleted
	forged.Actor = "owner"
	states = taskTraceTeamTaskStatesFromSnapshots(binding, manifest, []TaskTraceTeamSnapshot{{Actor: "reader", TaskStates: map[string]TaskTraceTeamTaskState{"child": forged}}})
	require.Empty(t, states, "a read-only snapshot cannot claim a writable author's deletion intent")
	states = taskTraceTeamTaskStatesFromSnapshots(binding, manifest, []TaskTraceTeamSnapshot{{Actor: "owner", Tasks: nil}})
	require.Empty(t, states, "an omitted task alone is not a delete intent")
	deleted.Actor = "owner"
	binding.TaskStates["child"] = deleted
	states = taskTraceTeamTaskStatesFromSnapshots(binding, manifest, []TaskTraceTeamSnapshot{{Actor: "reader", Tasks: []TaskTraceTeamTask{{NodeID: "child", Title: "Old client copy"}}}})
	require.True(t, states["child"].Deleted, "an old active snapshot must not clear a tombstone")
	restored := deleted
	restored.Version = 2
	restored.Deleted = false
	restored.EventID = "restore"
	states = taskTraceTeamTaskStatesFromSnapshots(binding, manifest, []TaskTraceTeamSnapshot{{Actor: "owner", TaskStates: map[string]TaskTraceTeamTaskState{"child": restored}}})
	require.False(t, states["child"].Deleted)
	require.True(t, taskTraceTeamTaskStateSupersedes(deleted, TaskTraceTeamTaskState{Version: 1, EventID: "z"}), "concurrent deletion wins over a restore that did not observe it")
}

func TestTaskTraceTeamTaskLifecycleWholeTreeDeleteAndRestore(t *testing.T) {
	peers := taskTraceLifecyclePeers(t)
	owner, member := peers[0], peers[1]
	owner.edit(t, func(s *xorm.Session) {
		require.NoError(t, (&Task{ID: owner.binding.RootTaskID}).Delete(s, owner.actor))
	})
	owner.sync(t)
	member.sync(t)
	for _, peer := range peers {
		for _, node := range []string{"root", "child"} {
			require.True(t, peer.binding.TaskStates[node].Deleted)
			s := db.NewSession()
			_, err := GetTaskByIDSimple(s, peer.binding.NodeTasks[node])
			require.True(t, IsErrTaskDoesNotExist(err))
			require.NoError(t, s.Close())
		}
	}
	owner.edit(t, func(s *xorm.Session) {
		for _, node := range []string{"root", "child"} {
			_, err := s.Unscoped().Table("tasks").Where("id = ?", owner.binding.NodeTasks[node]).Update(map[string]any{"deleted_at": nil})
			require.NoError(t, err)
		}
	})
	owner.sync(t)
	member.sync(t)
	for _, peer := range peers {
		s := db.NewSession()
		for _, node := range []string{"root", "child"} {
			_, err := GetTaskByIDSimple(s, peer.binding.NodeTasks[node])
			require.NoError(t, err)
			require.False(t, peer.binding.TaskStates[node].Deleted)
		}
		exists, err := s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", peer.binding.NodeTasks["root"], peer.binding.NodeTasks["child"], RelationKindSubtask).Exist(&TaskRelation{})
		require.NoError(t, err)
		require.True(t, exists)
		require.NoError(t, s.Close())
	}
}

func TestTaskTraceTeamTaskLifecycleUnknownMissingIsNotDeletion(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	binding := &TaskTraceTeamBinding{NodeTasks: map[string]int64{"unknown": 999999}}
	require.NoError(t, taskTraceTeamCaptureTaskStates(s, binding, "user1"))
	require.Empty(t, binding.TaskStates)
}
