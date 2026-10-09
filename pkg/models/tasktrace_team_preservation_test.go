// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package models

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamRestartAfterUncommittedSyncPreservesAdditions(t *testing.T) {
	for _, committedBefore := range []bool{false, true} {
		name := "first sync interrupted"
		if committedBefore {
			name = "later sync interrupted"
		}
		t.Run(name, func(t *testing.T) { testTaskTraceTeamInterruptedSync(t, committedBefore) })
	}
}

func testTaskTraceTeamInterruptedSync(t *testing.T, committedBefore bool) {
	t.Helper()
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	root := &Task{Title: "Before upgrade", ProjectID: 1, Priority: 7}
	require.NoError(t, root.Create(s, actor))
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	base := TaskTraceTeamBase{Title: root.Title, Status: TaskStatusTodo}
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "restart", Secret: "test", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: root.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": root.ID}, Base: map[string]TaskTraceTeamBase{"root": base}}
	permissions := []TaskTraceTeamMemberPermission{{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: binding.Base, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions, "child": permissions}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), &manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
	if committedBefore {
		prime := db.NewSession()
		_, err := TaskTraceTeamSync(prime, actor)
		require.NoError(t, err)
		require.NoError(t, prime.Commit())
		require.NoError(t, prime.Close())
	}
	now := time.Now().UTC().Add(-time.Minute)
	remote := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "remote", Updated: now, Base: binding.Base, Tasks: []TaskTraceTeamTask{
		{NodeID: "root", Title: "Updated task", Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"new": "New outstanding item"}, nil), Comments: []TaskTraceTeamComment{{ID: "progress", Body: "<h3>每日进展 · 2026-10-09</h3><p>New progress</p>", Author: "user2", Created: now, Updated: now}}}, //nolint:gosmopolitan // Existing persisted daily progress header format.
		{NodeID: "child", ParentNode: "root", Title: "New child", Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"child-new": "New child outstanding"}, nil)},
	}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
	content := []byte("test attachment bytes for interrupted sync")
	sum := sha256.Sum256(content)
	attachmentID := hex.EncodeToString(sum[:])
	blob := taskTraceTeamAttachmentBlobPath(&binding, attachmentID)
	require.NoError(t, os.MkdirAll(filepath.Dir(blob), 0o700))
	require.NoError(t, os.WriteFile(blob, content, 0o600))
	remote.Tasks[0].Attachments = []TaskTraceTeamAttachment{{ID: attachmentID, Name: "image.png", Mime: "image/png", Size: uint64(len(content)), SourceTaskID: 900, SourceAttachmentID: 901}}
	remote.Tasks[0].Outstanding = taskTraceTeamOutstandingHTML(map[string]string{"new": `<p>New outstanding item</p><aside data-tasktrace-outstanding-note="true" hidden><img src="/api/v1/tasks/900/attachments/901"></aside>`}, nil)
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
	// Installation/restart can interrupt the database transaction after the
	// synchronization files have already been written.
	interrupted := db.NewSession()
	_, err := TaskTraceTeamSync(interrupted, actor)
	require.NoError(t, err)
	require.NoError(t, interrupted.Rollback())
	require.NoError(t, interrupted.Close())
	lastGeneration := ""
	for cycle := 0; cycle < 3; cycle++ {
		reopened := db.NewSession()
		_, err = TaskTraceTeamSync(reopened, actor)
		require.NoError(t, err)
		require.NoError(t, reopened.Commit())
		require.NoError(t, reopened.Close())
		state, err := taskTraceTeamLoadState()
		require.NoError(t, err)
		require.Empty(t, state.Bindings[0].LastError)
		require.Empty(t, state.Bindings[0].Conflicts)
		read := db.NewReadSession()
		checkpoint := &TaskTraceTeamSyncCheckpoint{}
		foundCheckpoint, err := read.Get(checkpoint)
		require.NoError(t, err)
		require.True(t, foundCheckpoint)
		if cycle == 2 {
			require.Equal(t, lastGeneration, checkpoint.Generation, "unchanged polling must reuse its committed checkpoint")
		}
		lastGeneration = checkpoint.Generation
		stored, err := GetTaskByIDSimple(read, root.ID)
		require.NoError(t, err)
		require.Equal(t, "Updated task", stored.Title)
		list, err := taskTraceReadOutstanding(read, root.ID)
		require.NoError(t, err)
		require.Contains(t, list.original, "New outstanding item")
		var comments []*TaskComment
		require.NoError(t, read.Where("task_id = ?", root.ID).Find(&comments))
		progressCount := 0
		for _, comment := range comments {
			if strings.Contains(comment.Comment, "New progress") {
				progressCount++
			}
		}
		require.Equal(t, 1, progressCount, "progress must survive restart without duplicates")
		localAttachment := state.Bindings[0].LocalAttachments[taskTraceTeamLocalAttachmentKey("root", attachmentID)]
		attachment := &TaskAttachment{}
		found, err := read.ID(localAttachment).Get(attachment)
		require.NoError(t, err)
		require.True(t, found, "recovery must not keep rolled-back attachment IDs")
		require.Contains(t, list.original, "<img")
		require.NotContains(t, list.original, "tasks/900/attachments/901")
		childList, err := taskTraceReadOutstanding(read, state.Bindings[0].NodeTasks["child"])
		require.NoError(t, err)
		require.Contains(t, childList.original, "New child outstanding")
		require.NoError(t, read.Close())
	}
	// A successful sync must still accept real local edits and explicit
	// deletions; recovery must not restore an old acknowledged snapshot.
	edit := db.NewSession()
	_, err = edit.ID(root.ID).Cols("title").Update(&Task{Title: "Edited after recovery"})
	require.NoError(t, err)
	var progress []*TaskComment
	require.NoError(t, edit.Where("task_id = ?", root.ID).Find(&progress))
	for _, comment := range progress {
		if strings.Contains(comment.Comment, "New progress") {
			require.NoError(t, comment.Delete(edit, actor))
		}
	}
	require.NoError(t, (&TaskComment{TaskID: root.ID, Comment: "Local progress after recovery"}).Create(edit, actor))
	require.NoError(t, edit.Commit())
	require.NoError(t, edit.Close())
	for cycle := 0; cycle < 2; cycle++ {
		if cycle == 1 {
			remote.Tasks[0].Comments = append(remote.Tasks[0].Comments, TaskTraceTeamComment{ID: "unrelated", Body: "Another remote progress", Author: "user2", Created: now, Updated: now})
			require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
		}
		sync := db.NewSession()
		_, err = TaskTraceTeamSync(sync, actor)
		require.NoError(t, err)
		require.NoError(t, sync.Commit())
		state, err := taskTraceTeamLoadState()
		require.NoError(t, err)
		require.Empty(t, state.Bindings[0].LastError)
		require.Empty(t, state.Bindings[0].Conflicts, "already accepted remote edits must not replay after a new local edit")
		stored, err := GetTaskByIDSimple(sync, root.ID)
		require.NoError(t, err)
		require.Equal(t, "Edited after recovery", stored.Title)
		progress = nil
		require.NoError(t, sync.Where("task_id = ?", root.ID).Find(&progress))
		localCount := 0
		for _, comment := range progress {
			require.NotContains(t, comment.Comment, "New progress", "explicitly deleted progress must not be revived")
			if strings.Contains(comment.Comment, "Local progress after recovery") {
				localCount++
			}
		}
		require.Equal(t, 1, localCount)
		require.NoError(t, sync.Close())
	}
}

func TestTaskTraceTeamAcceptedFieldsDoNotReplayOrHideNewEdits(t *testing.T) {
	before := TaskTraceTeamBase{Title: "Before"}
	accepted := TaskTraceTeamBase{Title: "Accepted"}
	latest := TaskTraceTeamBase{Title: "Latest"}
	snapshot := TaskTraceTeamSnapshot{Actor: "writer", DeviceID: "remote", Base: map[string]TaskTraceTeamBase{"root": before}, Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: accepted.Title}}}
	manifest := TaskTraceTeamManifest{Base: map[string]TaskTraceTeamBase{"root": accepted}}
	require.True(t, taskTraceTeamRememberAcceptedField(&manifest, []TaskTraceTeamSnapshot{snapshot}, "root", "title", accepted.Title))
	manifest.Base["root"] = latest
	filtered := taskTraceTeamAcknowledgeSnapshots([]TaskTraceTeamSnapshot{snapshot}, &manifest)
	value, options, conflict := taskTraceTeamFindField(filtered, "root", "title", latest.Title, "")
	require.False(t, conflict)
	require.Empty(t, options)
	require.Equal(t, latest.Title, value)
	require.Equal(t, before, snapshot.Base["root"], "acknowledging must not mutate the original snapshot")
	// An intentional revert after observing Latest is a new edit, even though
	// its value is the same as a previously accepted value.
	snapshot.Base["root"] = latest
	filtered = taskTraceTeamAcknowledgeSnapshots([]TaskTraceTeamSnapshot{snapshot}, &manifest)
	value, _, conflict = taskTraceTeamFindField(filtered, "root", "title", latest.Title, "")
	require.False(t, conflict)
	require.Equal(t, accepted.Title, value)
	// A separate edit based on an older baseline still requires conflict resolution.
	snapshot.Base["root"] = before
	snapshot.Tasks[0].Title = "Concurrent"
	filtered = taskTraceTeamAcknowledgeSnapshots([]TaskTraceTeamSnapshot{snapshot}, &manifest)
	_, options, conflict = taskTraceTeamFindField(filtered, "root", "title", latest.Title, "")
	require.True(t, conflict)
	require.Len(t, options, 2)
}

func TestTaskTraceTeamImportWriterPreservesOutstanding(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "writer-import", Secret: "test"}
	base := TaskTraceTeamBase{Title: "Shared task", Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"new": "Shared outstanding item"}, nil)}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user2", Members: []string{"user1", "user2"}, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: map[string]TaskTraceTeamBase{"root": base}, Permissions: map[string][]TaskTraceTeamMemberPermission{
		"root": {{Username: "user2", Owner: true, Read: true, Write: true}, {Username: "user1", Read: true, Write: true}},
	}}
	path := filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json")
	require.NoError(t, taskTraceTeamWriteJSON(path, &manifest))
	owner := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "owner", Base: manifest.Base, Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: base.Title, Status: TaskStatusTodo, Outstanding: base.Outstanding}}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, owner))
	s := db.NewSession()
	defer s.Close()
	_, err := TaskTraceTeamImport(s, &user.User{ID: 1, Username: "user1"}, TaskTraceTeamImportRequest{ProjectID: 1, Link: taskTraceTeamEncodeLink(repository, binding.ShareID, binding.Secret)})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	state, err := taskTraceTeamLoadState()
	require.NoError(t, err)
	list, err := taskTraceReadOutstanding(s, state.Bindings[0].RootTaskID)
	require.NoError(t, err)
	require.Contains(t, list.original, "Shared outstanding item")
	require.NoError(t, taskTraceTeamReadJSON(path, &manifest))
	require.Contains(t, manifest.Base["root"].Outstanding, "Shared outstanding item")
}

func TestTaskTraceTeamRestrictedReaderCannotEraseSharedOutstanding(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	reader := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	defer s.Close()
	root := &Task{Title: "Restricted collaboration", ProjectID: 1, Priority: 7}
	require.NoError(t, root.Create(s, reader))
	base := TaskTraceTeamBase{Title: root.Title, Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"visible": "Visible item", "private": "New owner item"}, nil)}
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "restricted", Secret: "test", Owner: "user2", Members: []string{"user1", "user2"}, RootTaskID: root.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": root.ID}, Base: map[string]TaskTraceTeamBase{"root": base}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user2", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: binding.Base, Permissions: map[string][]TaskTraceTeamMemberPermission{
		"root":                     {{Username: "user2", Owner: true, Read: true, Write: true}, {Username: "user1", Read: true}},
		"root/outstanding/private": {{Username: "user2", Owner: true, Read: true, Write: true}, {Username: "user1"}},
	}}
	manifestPath := filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json")
	require.NoError(t, taskTraceTeamWriteJSON(manifestPath, &manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "reader", Bindings: []TaskTraceTeamBinding{binding}}))
	owner := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "owner", Base: binding.Base, Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: root.Title, Status: TaskStatusTodo, Outstanding: base.Outstanding}}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, owner))
	_, err := TaskTraceTeamSync(s, reader)
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, taskTraceTeamReadJSON(manifestPath, &manifest))
	require.Contains(t, manifest.Base["root"].Outstanding, "New owner item", "hiding a private item locally must never delete it from the shared baseline")
	list, err := taskTraceReadOutstanding(s, root.ID)
	require.NoError(t, err)
	require.NotContains(t, list.original, "New owner item")
	require.Contains(t, list.original, "Visible item")
}

func TestTaskTraceTeamReadOnlySnapshotsCannotDeleteOutstanding(t *testing.T) {
	old := TaskTraceTeamBase{Title: "Before", Status: TaskStatusTodo}
	accepted := TaskTraceTeamBase{Title: "After", Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"new": "Saved in browser"}, []string{"new"})}
	manifest := TaskTraceTeamManifest{Owner: "owner", Members: []string{"owner", "reader"}, Base: map[string]TaskTraceTeamBase{"node": accepted}}
	for _, tc := range []struct {
		name          string
		local, reader TaskTraceTeamBase
	}{
		{"upgraded computer has stale local base", old, accepted},
		{"read-only computer has stale base", accepted, old},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binding := TaskTraceTeamBinding{Base: map[string]TaskTraceTeamBase{"node": tc.local}}
			original := []TaskTraceTeamSnapshot{
				{Actor: "owner", Base: map[string]TaskTraceTeamBase{"node": accepted}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: accepted.Title, Outstanding: accepted.Outstanding}}},
				{Actor: "reader", Base: map[string]TaskTraceTeamBase{"node": tc.reader}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: tc.reader.Title, Outstanding: tc.reader.Outstanding}}},
			}
			filtered := taskTraceTeamFilterSnapshotsPermissions(original, &binding, &manifest)
			for _, field := range []string{"outstanding:new", "title"} {
				base := taskTraceTeamBaseValue(accepted, field)
				value, options, conflict := taskTraceTeamFindField(filtered, "node", field, base, "")
				require.False(t, conflict, "read-only snapshots must not introduce edits")
				require.Empty(t, options)
				require.Equal(t, base, value, "a permission filter must not turn a missing stale item into an intentional deletion")
			}
			require.Equal(t, tc.reader, original[1].Base["node"], "filtering must not mutate the original acknowledged base")
		})
	}
}

func TestTaskTraceTeamUpgradeSyncPreservesBrowserItems(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	defer s.Close()
	task := &Task{Title: "Shared upgrade", ProjectID: 1, Priority: 7}
	require.NoError(t, task.Create(s, actor))
	old := TaskTraceTeamBase{Title: task.Title, Status: TaskStatusTodo, Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"old": "Keep"}, nil)}
	accepted := old
	accepted.Outstanding = taskTraceTeamOutstandingHTML(map[string]string{"old": "Keep", "new": `<p>Saved in browser</p><aside data-tasktrace-outstanding-note="true" hidden>Keep this note</aside>`}, nil)
	comment := &TaskComment{TaskID: task.ID, Comment: accepted.Outstanding}
	require.NoError(t, comment.Create(s, actor))
	require.NoError(t, s.Commit())
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "upgrade-test", Secret: "test-only-secret", Owner: actor.Username, Members: []string{"user1", "user2"}, RootTaskID: task.ID, ProjectID: 1, NodeTasks: map[string]int64{"node": task.ID}, Base: map[string]TaskTraceTeamBase{"node": old}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "node", Owner: actor.Username, Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: map[string]TaskTraceTeamBase{"node": accepted}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), &manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "owner-device", Bindings: []TaskTraceTeamBinding{binding}}))
	reader := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "reader-device", Updated: time.Now().UTC(), Base: map[string]TaskTraceTeamBase{"node": accepted}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: task.Title, Status: TaskStatusTodo, Outstanding: accepted.Outstanding}}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, reader))
	for cycle := 0; cycle < 3; cycle++ {
		// Every pass reopens both the DB session and persisted team state, as
		// happens after installing new binaries and restarting the application.
		func() {
			syncSession := db.NewSession()
			defer syncSession.Close()
			_, err := TaskTraceTeamSync(syncSession, actor)
			require.NoError(t, err)
			require.NoError(t, syncSession.Commit())
		}()
		func() {
			read := db.NewReadSession()
			defer read.Close()
			list, err := taskTraceReadOutstanding(read, task.ID)
			require.NoError(t, err)
			require.Len(t, list.items, 2)
			require.Contains(t, list.original, "Saved in browser")
			require.Contains(t, list.original, "Keep this note")
			state, err := taskTraceTeamLoadState()
			require.NoError(t, err)
			require.Empty(t, state.Bindings[0].LastError)
			require.Empty(t, state.Bindings[0].Conflicts)
			var rows []*TaskComment
			require.NoError(t, read.Where("task_id = ?", task.ID).Find(&rows))
			count := 0
			for _, row := range rows {
				if taskTraceTeamIsOutstanding(row.Comment) {
					count++
				}
			}
			require.Equal(t, 1, count, "repeated sync must not create duplicate lists")
		}()
	}
}

func TestTaskTraceTeamPermissionFilterPreservesIndependentOutstandingWrite(t *testing.T) {
	base := TaskTraceTeamBase{Title: "Trusted", Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"editable": "Before", "protected": "Keep"}, nil)}
	manifest := TaskTraceTeamManifest{Owner: "owner", Members: []string{"owner", "reader"}, Base: map[string]TaskTraceTeamBase{"node": base}, Permissions: map[string][]TaskTraceTeamMemberPermission{
		"node":                      {{Username: "owner", Read: true, Write: true}, {Username: "reader", Read: true}},
		"node/outstanding/editable": {{Username: "owner", Read: true, Write: true}, {Username: "reader", Read: true, Write: true}},
	}}
	snapshot := TaskTraceTeamSnapshot{Actor: "reader", Base: map[string]TaskTraceTeamBase{"node": base}, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "Unauthorized", Outstanding: taskTraceTeamOutstandingHTML(map[string]string{"editable": "After"}, nil)}}}
	filtered := taskTraceTeamFilterSnapshotsPermissions([]TaskTraceTeamSnapshot{snapshot}, &TaskTraceTeamBinding{}, &manifest)
	for field, expected := range map[string]string{"title": "Trusted", "outstanding:editable": "After", "outstanding:protected": "Keep"} {
		value, _, conflict := taskTraceTeamFindField(filtered, "node", field, taskTraceTeamBaseValue(base, field), "")
		require.False(t, conflict)
		require.Equal(t, expected, value)
	}
}

func TestTaskTraceTeamSnapshotKeepsCanonicalListAfterReopen(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	defer s.Close()
	task := &Task{Title: "Upgrade preservation", ProjectID: 1, Priority: 7}
	require.NoError(t, task.Create(s, actor))
	// Imported historical comments can have a later timestamp than the newest
	// canonical list. Web and floating clients both select by numeric comment ID.
	old := &TaskComment{TaskID: task.ID, AuthorID: 1, Comment: taskTraceTeamOutstandingHTML(map[string]string{"old": "Old"}, nil), Created: time.Now().Add(time.Hour), Updated: time.Now().Add(time.Hour)}
	_, err := s.NoAutoTime().Insert(old)
	require.NoError(t, err)
	current := &TaskComment{TaskID: task.ID, Comment: taskTraceTeamOutstandingHTML(map[string]string{"old": "Old", "new": "Saved in browser"}, nil)}
	require.NoError(t, current.Create(s, actor))
	require.Greater(t, current.ID, old.ID)
	require.NoError(t, s.Commit())
	reopened := db.NewSession()
	defer reopened.Close()
	list, err := taskTraceReadOutstanding(reopened, task.ID)
	require.NoError(t, err)
	require.Len(t, list.items, 2)
	binding := &TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "share", RootTaskID: task.ID, NodeTasks: map[string]int64{"node": task.ID}}
	snapshot, err := taskTraceTeamBuildSnapshot(reopened, binding, actor.Username, "device")
	require.NoError(t, err)
	items, _ := taskTraceTeamOutstandingItems(taskTraceTeamTaskMap(snapshot)["node"].Outstanding)
	require.Equal(t, map[string]string{"old": "Old", "new": "Saved in browser"}, items)
}
