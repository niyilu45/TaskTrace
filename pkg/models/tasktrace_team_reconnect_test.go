// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func copyTeamRepositoryForTest(t *testing.T, source, target string) {
	t.Helper()
	require.NoError(t, filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		content, err := os.ReadFile(path) //nolint:gosec // Private t.TempDir fixture, no untrusted writers.
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o600) //nolint:gosec // Both roots are private t.TempDir fixtures.
	}))
}

func TestTaskTraceTeamReconnectCopiedRepository(t *testing.T) {
	t.Run("member-joins-restored-directory", func(t *testing.T) { testTaskTraceTeamReconnectCopiedRepository(t, false) })
	t.Run("owner-rejoins-original-directory", func(t *testing.T) { testTaskTraceTeamReconnectCopiedRepository(t, true) })
}

func testTaskTraceTeamReconnectCopiedRepository(t *testing.T, reconnectOwner bool) {
	db.LoadAndAssertFixtures(t)
	original := t.TempDir()
	restored := filepath.Join(t.TempDir(), "teamData", "imports", "restored")
	t.Setenv("TASKTRACE_TEAM_ROOT", original)
	owner := &taskTraceSyncPeer{data: t.TempDir(), actor: &user.User{ID: 1, Username: "user1"}}
	member := &taskTraceSyncPeer{data: t.TempDir(), actor: &user.User{ID: 2, Username: "user2"}}
	owner.activate(t)
	s := db.NewSession()
	defer s.Close()
	project := &Project{Title: "Owner project"}
	require.NoError(t, project.Create(s, owner.actor))
	task := &Task{Title: "Shared child", ProjectID: project.ID, Priority: 7}
	require.NoError(t, task.Create(s, owner.actor))
	owner.binding = TaskTraceTeamBinding{ShareID: "reconnect-test", Repository: original, Secret: "secret", Owner: "user1", Members: []string{"user1", "user2"},
		RootTaskID: task.ID, ProjectID: project.ID, NodeTasks: map[string]int64{"root": task.ID}, Base: map[string]TaskTraceTeamBase{}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: owner.binding.ShareID, RootNode: "root", Owner: "user1", Members: owner.binding.Members,
		TokenHash: taskTraceTeamTokenHash(owner.binding.Secret), Base: map[string]TaskTraceTeamBase{"root": {Title: task.Title, Status: TaskStatusTodo}},
		Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}}}
	owner.binding.Base = taskTraceTeamCopyBase(manifest.Base)
	require.NoError(t, taskTraceTeamWriteManifest(filepath.Join(taskTraceTeamShareDir(original, manifest.ShareID), "manifest.json"), &manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "owner-device", Bindings: []TaskTraceTeamBinding{owner.binding}}))
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	owner.sync(t)
	member.activate(t)
	s = db.NewSession()
	defer s.Close()
	memberProject := &Project{Title: "Member project"}
	require.NoError(t, memberProject.Create(s, member.actor))
	_, err := TaskTraceTeamImport(s, member.actor, TaskTraceTeamImportRequest{ProjectID: memberProject.ID, Link: taskTraceTeamEncodeLink(original, manifest.ShareID, "secret")})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	memberState, err := taskTraceTeamLoadState()
	require.NoError(t, err)
	member.binding = memberState.Bindings[0]
	memberTaskID := member.binding.RootTaskID
	// Each person retains a private arrangement of the shared task.
	var privateParentID int64
	member.edit(t, func(s *xorm.Session) {
		parent := &Task{Title: "Member private parent", ProjectID: memberProject.ID}
		require.NoError(t, parent.Create(s, member.actor))
		privateParentID = parent.ID
		require.NoError(t, (&TaskRelation{TaskID: parent.ID, OtherTaskID: memberTaskID, RelationKind: RelationKindSubtask}).Create(s, member.actor))
		require.NoError(t, (&TaskComment{TaskID: memberTaskID, Comment: taskTraceTeamAddMarker("Previously saved then deleted", "delete-me", member.actor.Username)}).Create(s, member.actor))
	})
	member.sync(t)
	owner.sync(t)
	copyTeamRepositoryForTest(t, original, restored)
	owner.activate(t)
	ownerState, err := taskTraceTeamLoadState()
	require.NoError(t, err)
	ownerState.Bindings[0].Repository = restored
	require.NoError(t, taskTraceTeamSaveState(ownerState))
	t.Setenv("TASKTRACE_TEAM_ROOT", restored)
	// Restoring/copying split the endpoint. Both sides can report success
	// while saving different edits to different repositories.
	owner.edit(t, func(s *xorm.Session) {
		_, err := s.ID(task.ID).Cols("description").Update(&Task{Description: "Owner restored description"})
		require.NoError(t, err)
		require.NoError(t, (&TaskComment{TaskID: task.ID, Comment: "Owner progress in restored repository"}).Create(s, owner.actor))
	})
	member.edit(t, func(s *xorm.Session) {
		_, err := s.ID(memberTaskID).Cols("title").Update(&Task{Title: "Member changed task title"})
		require.NoError(t, err)
		var comments []*TaskComment
		require.NoError(t, s.Where("task_id = ?", memberTaskID).Find(&comments))
		for _, comment := range comments {
			if strings.Contains(comment.Comment, "Previously saved then deleted") {
				require.NoError(t, comment.Delete(s, member.actor))
			}
		}
		image := &TaskAttachment{TaskID: memberTaskID}
		imageBytes := []byte("member's retained image")
		require.NoError(t, image.NewAttachment(s, bytes.NewReader(imageBytes), "progress.png", uint64(len(imageBytes)), member.actor))
		body := fmt.Sprintf(`<h3>每日进展 · 2026-10-14</h3><p>Member retained progress</p><img src="/api/v1/tasks/%d/attachments/%d">`, memberTaskID, image.ID) //nolint:gosmopolitan
		require.NoError(t, (&TaskComment{TaskID: memberTaskID, Comment: body}).Create(s, member.actor))
		member.list(t, s, "root", func(items map[string]string) {
			items["member-item"] = fmt.Sprintf(`<p>Member retained outstanding</p><img src="/api/v1/tasks/%d/attachments/%d">`, memberTaskID, image.ID)
		})
	})
	member.sync(t)
	owner.sync(t)
	// A target-side snapshot may contain a comment not observed by the old
	// installation. Reconnection must not turn its local absence into deletion.
	joiner, targetRepository, joinerProject := member, restored, memberProject
	if reconnectOwner {
		joiner, targetRepository, joinerProject = owner, original, project
	}
	joiner.activate(t)
	joiningState, err := taskTraceTeamLoadState()
	require.NoError(t, err)
	targetBinding := joiner.binding
	targetBinding.Repository = targetRepository
	var targetSnapshot TaskTraceTeamSnapshot
	targetPath := taskTraceTeamSnapshotPath(&targetBinding, joiner.actor.Username, joiningState.DeviceID)
	require.NoError(t, taskTraceTeamReadJSON(targetPath, &targetSnapshot))
	targetSnapshot.Tasks[0].Comments = append(targetSnapshot.Tasks[0].Comments, TaskTraceTeamComment{ID: "unseen-at-source", Body: "Only seen in target", Author: "user1", Created: time.Now().UTC(), Updated: time.Now().UTC()})
	require.NoError(t, taskTraceTeamWriteSnapshot(&targetBinding, targetSnapshot))
	s = db.NewSession()
	defer s.Close()
	var before []*TaskComment
	require.NoError(t, s.Where("task_id = ?", joiner.binding.RootTaskID).Find(&before))
	missingProgress := "Owner progress in restored"
	if reconnectOwner {
		missingProgress = "Member retained progress"
	}
	for _, comment := range before {
		require.NotContains(t, comment.Comment, missingProgress)
	}
	// Existing tasks keep their project even when the import dialog selects another.
	anotherProject := &Project{Title: "Import target is ignored for an existing share"}
	require.NoError(t, anotherProject.Create(s, joiner.actor))
	beforeReconnect, err := os.ReadFile(taskTraceTeamStatePath())
	require.NoError(t, err)
	link := taskTraceTeamEncodeLink(targetRepository, manifest.ShareID, "secret")
	result, err := TaskTraceTeamImport(s, joiner.actor, TaskTraceTeamImportRequest{ProjectID: anotherProject.ID, Link: link})
	require.NoError(t, err)
	require.Len(t, result.Bindings, 1)
	require.Equal(t, joiner.binding.RootTaskID, result.Bindings[0].RootTaskID)
	require.Equal(t, targetRepository, result.Bindings[0].Repository)
	// Simulate an interrupted database commit with the old connection state
	// still on disk. Retrying must reuse the original fork baseline, despite
	// the destination snapshot having already received the merged values.
	require.NoError(t, s.Rollback())
	require.NoError(t, s.Close())
	require.NoError(t, os.WriteFile(taskTraceTeamStatePath(), beforeReconnect, 0o600)) //nolint:gosec // TASKTRACE_DATA_ROOT is this peer's private t.TempDir.
	s = db.NewSession()
	defer s.Close()
	_, err = TaskTraceTeamImport(s, joiner.actor, TaskTraceTeamImportRequest{ProjectID: joinerProject.ID, Link: link})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	for range 2 {
		owner.sync(t)
		member.sync(t)
	}
	for _, peer := range []*taskTraceSyncPeer{owner, member} {
		peer.activate(t)
		check := db.NewReadSession()
		stored, err := GetTaskByIDSimple(check, peer.binding.RootTaskID)
		require.NoError(t, err)
		require.Equal(t, "Member changed task title", stored.Title)
		require.Equal(t, "Owner restored description", stored.Description)
		var comments []*TaskComment
		require.NoError(t, check.Where("task_id = ?", stored.ID).Find(&comments))
		for _, expected := range []string{"Member retained progress", "Owner progress in restored repository", "Only seen in target"} {
			count := 0
			for _, comment := range comments {
				require.NotContains(t, comment.Comment, "Previously saved then deleted")
				if strings.Contains(comment.Comment, expected) {
					count++
				}
			}
			require.Equal(t, 1, count, expected)
		}
		list, err := taskTraceReadOutstanding(check, stored.ID)
		require.NoError(t, err)
		require.Contains(t, list.original, "Member retained outstanding")
		require.Contains(t, list.original, fmt.Sprintf("/tasks/%d/attachments/", stored.ID))
		images, err := check.Where("task_id = ?", stored.ID).Count(&TaskAttachment{})
		require.NoError(t, err)
		require.EqualValues(t, 1, images)
		require.NoError(t, check.Close())
	}
	member.activate(t)
	check := db.NewSession()
	defer check.Close()
	relationExists, err := check.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", privateParentID, memberTaskID, RelationKindSubtask).Exist(&TaskRelation{})
	require.NoError(t, err)
	require.True(t, relationExists)
	stored, err := GetTaskByIDSimple(check, memberTaskID)
	require.NoError(t, err)
	require.Equal(t, memberProject.ID, stored.ProjectID)
	// Repeated import is idempotent, even without changing the endpoint.
	_, err = TaskTraceTeamImport(check, member.actor, TaskTraceTeamImportRequest{ProjectID: memberProject.ID, Link: link})
	require.NoError(t, err)
	require.NoError(t, check.Commit())
	count, err := check.Where("project_id = ?", memberProject.ID).Count(&Task{})
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

func TestTaskTraceTeamReconnectRejectsDifferentIdentity(t *testing.T) {
	for _, mismatch := range []string{"secret", "owner", "root", "local-access"} {
		t.Run(mismatch, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			data, repository := t.TempDir(), t.TempDir()
			t.Setenv("TASKTRACE_DATA_ROOT", data)
			t.Setenv("TASKTRACE_TEAM_ROOT", repository)
			actor := &user.User{ID: 1, Username: "user1"}
			s := db.NewSession()
			defer s.Close()
			task := &Task{Title: "Existing collaboration", ProjectID: 1}
			require.NoError(t, task.Create(s, actor))
			binding := TaskTraceTeamBinding{ShareID: "identity", Repository: t.TempDir(), Secret: "secret", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: task.ID, NodeTasks: map[string]int64{"root": task.ID}}
			require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "device", Bindings: []TaskTraceTeamBinding{binding}}))
			before, err := os.ReadFile(taskTraceTeamStatePath())
			require.NoError(t, err)
			secret := binding.Secret
			manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(secret)}
			if mismatch == "secret" {
				secret = "different-secret"
				manifest.TokenHash = taskTraceTeamTokenHash(secret)
			}
			if mismatch == "owner" {
				manifest.Owner = "user2"
			}
			if mismatch == "root" {
				manifest.RootNode = "different-root"
			}
			projectID := int64(1)
			if mismatch == "local-access" {
				actor = &user.User{ID: 2, Username: "user2"}
				project := &Project{Title: "Other actor's target"}
				require.NoError(t, project.Create(s, actor))
				projectID = project.ID
			}
			require.NoError(t, s.Commit())
			require.NoError(t, taskTraceTeamWriteManifest(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), &manifest))
			_, err = TaskTraceTeamImport(s, actor, TaskTraceTeamImportRequest{ProjectID: projectID, Link: taskTraceTeamEncodeLink(repository, binding.ShareID, secret)})
			require.Error(t, err)
			after, err := os.ReadFile(taskTraceTeamStatePath())
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.NoDirExists(t, filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "actors"))
		})
	}
}
