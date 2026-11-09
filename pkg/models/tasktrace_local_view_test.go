// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/files"

	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestTaskTraceImportedLocalView(t *testing.T) {
	peers := taskTraceLifecyclePeers(t)
	owner, member := peers[0], peers[1]
	root, child := member.binding.RootTaskID, member.binding.NodeTasks["child"]
	manifestPath := filepath.Join(taskTraceTeamShareDir(owner.binding.Repository, owner.binding.ShareID), "manifest.json")
	var manifest TaskTraceTeamManifest
	require.NoError(t, taskTraceTeamReadManifest(manifestPath, &manifest))
	childPermissions := append([]TaskTraceTeamMemberPermission(nil), manifest.Permissions["child"]...)
	for i := range manifest.Permissions["child"] {
		if manifest.Permissions["child"][i].Username == member.actor.Username {
			manifest.Permissions["child"][i].Write = false
		}
	}
	require.NoError(t, taskTraceTeamWriteManifest(manifestPath, &manifest))

	var destination Project
	member.edit(t, func(s *xorm.Session) {
		destination.Title = "Empty destination"
		require.NoError(t, destination.Create(s, member.actor))
		require.NotEmpty(t, destination.Views)
		task := &Task{ID: child}
		can, err := task.CanUpdate(s, member.actor)
		require.NoError(t, err)
		require.False(t, can, "moving must not require shared-content write access")
		move := &TaskTraceMove{TaskID: child, ProjectID: destination.ID, ProjectViewID: destination.Views[0].ID}
		can, _ = move.CanCreate(s, &user.User{ID: 3, Username: "user3"})
		require.False(t, can)
		can, err = move.CanCreate(s, member.actor)
		require.NoError(t, err)
		require.True(t, can)
		require.NoError(t, move.Create(s, member.actor))
		require.Empty(t, taskTraceParentIDs(t, s, child))
	})
	member.sync(t)
	owner.sync(t)
	owner.edit(t, func(s *xorm.Session) {
		require.Equal(t, []int64{owner.binding.RootTaskID}, taskTraceParentIDs(t, s, owner.binding.NodeTasks["child"]))
		// New synced children follow the receiving copy's actual project.
		parent := owner.binding.NodeTasks["child"]
		descendant := &Task{Title: "New synced descendant", ProjectID: owner.binding.ProjectID, ParentTaskID: parent}
		require.NoError(t, descendant.Create(s, owner.actor))
	})
	owner.sync(t)
	member.sync(t)

	var descendant int64
	member.edit(t, func(s *xorm.Session) {
		for _, id := range member.binding.NodeTasks {
			if id == root || id == child {
				continue
			}
			descendant = id
			task, err := GetTaskByIDSimple(s, id)
			require.NoError(t, err)
			require.Equal(t, destination.ID, task.ProjectID)
			require.Equal(t, []int64{child}, taskTraceParentIDs(t, s, id))
		}
		require.NotZero(t, descendant)
	})

	// Give the member write access long enough to save a comment that has
	// not reached the owner at the time of the local removal.
	require.NoError(t, taskTraceTeamReadManifest(manifestPath, &manifest))
	manifest.Permissions["child"] = childPermissions
	require.NoError(t, taskTraceTeamWriteManifest(manifestPath, &manifest))
	const body = "<p>Member comment saved before local removal</p>"
	const payload = "pending attachment"
	t.Setenv("TASKTRACE_PACKAGE_ROOT", t.TempDir())
	var localFileID int64
	var undo string
	member.edit(t, func(s *xorm.Session) {
		note := &TaskComment{TaskID: child, Comment: body}
		can, err := note.CanCreate(s, member.actor)
		require.NoError(t, err)
		require.True(t, can)
		require.NoError(t, note.Create(s, member.actor))
		attachment := &TaskAttachment{TaskID: child}
		require.NoError(t, attachment.NewAttachment(s, bytes.NewReader([]byte(payload)), "pending.txt", uint64(len(payload)), member.actor))
		localFileID = attachment.FileID
		task := &Task{ID: child}
		can, _ = task.CanDelete(s, &user.User{ID: 3, Username: "user3"})
		require.False(t, can)
		can, err = task.CanDelete(s, member.actor)
		require.NoError(t, err)
		require.True(t, can)
		snapshot, supported, err := captureTaskTraceUndo(s, member.actor, "delete", task)
		require.NoError(t, err)
		require.True(t, supported)
		require.NoError(t, task.Delete(s, member.actor))
		var changed bool
		undo, _, _, changed, err = finishTaskTraceUndo(s, member.actor, "delete", task, snapshot)
		require.NoError(t, err)
		require.True(t, changed)
	})
	// A share can revoke write access between local save and the first sync.
	// Keep the pending comment and attachment until a later sync can send them.
	require.NoError(t, taskTraceTeamReadManifest(manifestPath, &manifest))
	for i := range manifest.Permissions["child"] {
		if manifest.Permissions["child"][i].Username == member.actor.Username {
			manifest.Permissions["child"][i].Write = false
		}
	}
	require.NoError(t, taskTraceTeamWriteManifest(manifestPath, &manifest))
	member.edit(t, func(s *xorm.Session) {
		status, err := TaskTraceTeamSync(s, member.actor)
		require.NoError(t, err)
		require.Len(t, status.Bindings, 1)
		require.NotEmpty(t, status.Bindings[0].LastError)
		raw, err := GetTaskByIDSimple(s, child)
		require.NoError(t, err)
		require.False(t, raw.TaskTraceViewPruned)
		var notes []TaskComment
		require.NoError(t, s.Where("task_id = ?", child).Find(&notes))
		require.Len(t, notes, 1)
		require.Contains(t, notes[0].Comment, body)
	})
	require.NoError(t, taskTraceTeamReadManifest(manifestPath, &manifest))
	manifest.Permissions["child"] = childPermissions
	require.NoError(t, taskTraceTeamWriteManifest(manifestPath, &manifest))
	member.sync(t)
	owner.sync(t)
	// Further background syncs must leave the actor's published contribution
	// intact without rebuilding its local copy.
	member.sync(t)
	owner.sync(t)
	member.edit(t, func(s *xorm.Session) {
		for _, id := range []int64{child, descendant} {
			task := &Task{ID: id}
			_, _, err := task.CanRead(s, member.actor)
			require.True(t, IsErrTaskDoesNotExist(err))
			raw, err := GetTaskByIDSimple(s, id)
			require.NoError(t, err)
			require.Equal(t, child, raw.TaskTraceHiddenBy)
			require.True(t, raw.DeletedAt.IsZero())
			require.Truef(t, raw.TaskTraceViewPruned, "task %d still has local title %q", id, raw.Title)
			require.Empty(t, raw.Title)
			notes, err := s.Where("task_id = ?", id).Count(&TaskComment{})
			require.NoError(t, err)
			require.Zero(t, notes)
		}
		require.Empty(t, member.binding.TaskStates)
		collection := &TaskCollection{ProjectID: destination.ID}
		tasks, _, _, err := collection.ReadAll(s, member.actor, "", 0, 0)
		require.NoError(t, err)
		require.Empty(t, tasks)
	})
	owner.edit(t, func(s *xorm.Session) {
		task := &Task{ID: owner.binding.NodeTasks["child"]}
		can, _, err := task.CanRead(s, owner.actor)
		require.NoError(t, err)
		require.True(t, can)
		require.Empty(t, owner.binding.TaskStates)
		var notes []TaskComment
		require.NoError(t, s.Where("task_id = ?", task.ID).Find(&notes))
		require.Len(t, notes, 1)
		require.Contains(t, notes[0].Comment, body)
	})
	member.activate(t)
	require.NoError(t, taskTraceCleanupImages(time.Now(), files.DeleteBlob))
	member.edit(t, func(s *xorm.Session) {
		cached, err := s.ID(localFileID).Exist(&files.File{})
		require.NoError(t, err)
		require.False(t, cached, "published attachment cache should be released")
		_, err = restoreTaskTraceUndo(s, member.actor, undo)
		require.NoError(t, err)
		for _, id := range []int64{child, descendant} {
			task, err := GetTaskByIDSimple(s, id)
			require.NoError(t, err)
			require.Zero(t, task.TaskTraceHiddenBy)
			require.False(t, task.TaskTraceViewPruned)
			require.NotEmpty(t, task.Title)
			require.Equal(t, destination.ID, task.ProjectID)
		}
		var notes []TaskComment
		require.NoError(t, s.Where("task_id = ?", child).Find(&notes))
		require.Len(t, notes, 1)
		require.Contains(t, notes[0].Comment, body)
		attachments, err := getTaskAttachmentsByTaskIDs(s, []int64{child})
		require.NoError(t, err)
		require.Len(t, attachments, 1)
		require.NotEqual(t, localFileID, attachments[0].FileID)
	})

	// A read-only member can hide the root even while the share is offline.
	member.edit(t, func(s *xorm.Session) {
		state, err := taskTraceTeamLoadState()
		require.NoError(t, err)
		state.Bindings[0].Repository = t.TempDir()
		require.NoError(t, taskTraceTeamSaveState(state))
		task := &Task{ID: root}
		can, err := task.CanUpdate(s, member.actor)
		require.NoError(t, err)
		require.False(t, can)
		can, err = task.CanDelete(s, member.actor)
		require.NoError(t, err)
		require.True(t, can)
		require.NoError(t, task.Delete(s, member.actor))
		state.Bindings[0].Repository = member.binding.Repository
		require.NoError(t, taskTraceTeamSaveState(state))
	})
	member.edit(t, func(s *xorm.Session) {
		link := taskTraceTeamEncodeLinkPaths(member.binding.Repository, nil, member.binding.ShareID, member.binding.Secret)
		_, err := TaskTraceTeamImport(s, member.actor, TaskTraceTeamImportRequest{Link: link, ProjectID: member.binding.ProjectID})
		require.NoError(t, err)
		task, err := GetTaskByIDSimple(s, root)
		require.NoError(t, err)
		require.Zero(t, task.TaskTraceHiddenBy)
	})
}
