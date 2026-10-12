// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"os"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamNotificationDeleteSelected(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "notification-test", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: 1, NodeTasks: map[string]int64{"root": 1}}
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
	ids := []string{uuid.NewString(), uuid.NewString()}
	path := func(member, id string) string {
		return filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "notifications", member, id+".json")
	}
	for _, id := range ids {
		require.NoError(t, taskTraceTeamWriteJSON(path("user1", id), TaskTraceTeamNotification{ID: id, Actor: "user2"}))
	}
	require.NoError(t, taskTraceTeamWriteJSON(path("user2", ids[0]), TaskTraceTeamNotification{ID: ids[0], Actor: "user1"}))
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	_, err := TaskTraceTeamNotificationsDelete(s, actor, TaskTraceTeamNotificationsDeleteRequest{IDs: []string{"invalid"}})
	require.Error(t, err)
	for _, id := range ids {
		require.FileExists(t, path("user1", id))
	}
	_, err = TaskTraceTeamNotificationsDelete(s, actor, TaskTraceTeamNotificationsDeleteRequest{IDs: ids[:1]})
	require.NoError(t, err)
	_, err = os.Stat(path("user1", ids[0]))
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, path("user1", ids[1]))
	require.FileExists(t, path("user2", ids[0]))
}

func TestTaskTraceTeamResolveOnlySelectedConflict(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	task := &Task{Title: "Original", ProjectID: 1, Priority: 7}
	require.NoError(t, task.Create(s, actor))
	require.NoError(t, s.Commit())
	base := TaskTraceTeamBase{Title: "Original", Status: TaskStatusTodo}
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "partial-resolve", Secret: "test", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: task.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": task.ID}, Base: map[string]TaskTraceTeamBase{"root": base}, Conflicts: []TaskTraceTeamConflict{
		{ID: "title-conflict", ShareID: "partial-resolve", NodeID: "root", TaskID: task.ID, Field: "title", Base: "Original", Options: []TaskTraceTeamConflictOption{{Author: "user2", Value: "Chosen"}}},
		{ID: "done-conflict", ShareID: "partial-resolve", NodeID: "root", TaskID: task.ID, Field: "done", Base: "false", Options: []TaskTraceTeamConflictOption{{Author: "user2", Value: "true"}}},
	}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: binding.Base}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
	_, err := TaskTraceTeamResolve(s, actor, TaskTraceTeamResolveRequest{ShareID: binding.ShareID, Resolutions: []TaskTraceTeamResolution{{ConflictID: "title-conflict", Value: "Chosen"}}})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	stored, err := GetTaskByIDSimple(s, task.ID)
	require.NoError(t, err)
	require.Equal(t, "Chosen", stored.Title)
	require.False(t, stored.Done)
	state, err := taskTraceTeamLoadState()
	require.NoError(t, err)
	require.Len(t, state.Bindings[0].Conflicts, 1)
	require.Equal(t, "done-conflict", state.Bindings[0].Conflicts[0].ID)
}

func TestTaskTraceTeamConflictPreviewsKeepCanonicalValues(t *testing.T) {
	html := `<img src="tasktrace-team-attachment:abc123">`
	binding := TaskTraceTeamBinding{NodeTasks: map[string]int64{"root": 5}, LocalAttachments: map[string]int64{"root:abc123": 8}, Conflicts: []TaskTraceTeamConflict{{ID: "same", NodeID: "root", Base: html, Options: []TaskTraceTeamConflictOption{{Author: "user2", Value: html}}}}}
	result := taskTraceTeamConflictPreviews(&binding)
	require.Equal(t, "same", result[0].ID)
	require.Equal(t, html, result[0].Base)
	require.Equal(t, html, result[0].Options[0].Value)
	require.Contains(t, result[0].BasePreview, "/api/v1/tasks/5/attachments/8")
	require.Contains(t, result[0].Options[0].Preview, "/api/v1/tasks/5/attachments/8")
	require.Empty(t, binding.Conflicts[0].Options[0].Preview)
}

func TestTaskTraceTeamNotificationsReadRetainsHistory(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "read-test", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: 1, NodeTasks: map[string]int64{"root": 1}}
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	path := func(member, id string) string {
		return filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "notifications", member, id+".json")
	}
	for _, id := range ids[:2] {
		require.NoError(t, taskTraceTeamWriteJSON(path("user1", id), TaskTraceTeamNotification{ID: id, Actor: "user2", TaskTitle: "Saved task"}))
	}
	require.NoError(t, taskTraceTeamWriteJSON(path("user2", ids[0]), TaskTraceTeamNotification{ID: ids[0], Actor: "user1"}))
	before, err := os.ReadFile(path("user1", ids[0]))
	require.NoError(t, err)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	_, err = TaskTraceTeamNotificationsRead(s, &LinkSharing{ID: 1}, TaskTraceTeamNotificationsReadRequest{})
	require.Error(t, err)
	_, err = TaskTraceTeamNotificationsDelete(s, &LinkSharing{ID: 1}, TaskTraceTeamNotificationsDeleteRequest{})
	require.Error(t, err)
	_, err = TaskTraceTeamNotificationsRead(s, actor, TaskTraceTeamNotificationsReadRequest{IDs: []string{"invalid"}})
	require.Error(t, err)
	for _, n := range taskTraceTeamNotifications(s, &binding, "user1") {
		require.False(t, n.Read)
	}

	result, err := TaskTraceTeamNotificationsRead(s, actor, TaskTraceTeamNotificationsReadRequest{IDs: ids[:1]})
	require.NoError(t, err)
	require.Len(t, result.Notifications, 2)
	for _, n := range result.Notifications {
		require.Equal(t, n.ID == ids[0], n.Read)
	}
	// Loading again uses the file on disk, not an in-memory UI state.
	for _, n := range taskTraceTeamNotifications(s, &binding, "user1") {
		require.Equal(t, n.ID == ids[0], n.Read)
	}
	require.FileExists(t, taskTraceTeamNotificationReadPath(&binding, "user1"))
	after, err := os.ReadFile(path("user1", ids[0]))
	require.NoError(t, err)
	require.Equal(t, before, after, "reading must not modify or remove the shared notification")
	for _, n := range taskTraceTeamNotifications(s, &binding, "user2") {
		require.False(t, n.Read)
	}
	otherShare := binding
	otherShare.ShareID = "another-share"
	reads, err := taskTraceTeamLoadNotificationReads(&otherShare, "user1")
	require.NoError(t, err)
	require.Empty(t, reads)

	// Replayed shared files and repeated reads leave existing history intact.
	require.NoError(t, os.WriteFile(path("user1", ids[0]), before, 0o600)) // #nosec G703 -- Fixture path contains only t.TempDir and generated UUIDs.
	_, err = TaskTraceTeamNotificationsRead(s, actor, TaskTraceTeamNotificationsReadRequest{})
	require.NoError(t, err)
	for _, n := range taskTraceTeamNotifications(s, &binding, "user1") {
		require.True(t, n.Read)
	}
	require.NoError(t, taskTraceTeamWriteJSON(path("user1", ids[2]), TaskTraceTeamNotification{ID: ids[2], Actor: "user2"}))
	next := taskTraceTeamNotifications(s, &binding, "user1")
	require.Len(t, next, 3)
	for _, n := range next {
		require.Equal(t, n.ID != ids[2], n.Read)
	}

	// A failed receipt write must not erase history or silently acknowledge it.
	receipt := taskTraceTeamNotificationReadPath(&binding, "user1")
	require.NoError(t, os.Remove(receipt))
	require.NoError(t, os.Mkdir(receipt, 0o700))
	_, err = TaskTraceTeamNotificationsRead(s, actor, TaskTraceTeamNotificationsReadRequest{IDs: ids[2:]})
	require.Error(t, err)
	for _, id := range ids {
		require.FileExists(t, path("user1", id))
	}
}
