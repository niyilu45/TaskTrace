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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceCompletionOverlayAndPermission(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	task := &Task{Title: "manual completion", ProjectID: 1, PercentDone: 0.4}
	require.NoError(t, task.Create(s, actor))
	allowed, err := task.CanUpdate(s, actor)
	require.NoError(t, err)
	require.True(t, allowed)
	denied, err := task.CanUpdate(s, &user.User{ID: 13, Username: "user13"})
	require.NoError(t, err)
	require.False(t, denied)
	for _, done := range []bool{true, false, true, false} {
		changed := &Task{ID: task.ID, Done: done}
		require.NoError(t, changed.updateSingleTask(s, actor, []string{"done"}))
		saved, err := GetTaskByIDSimple(s, task.ID)
		require.NoError(t, err)
		require.InDelta(t, 0.4, saved.PercentDone, 0.00001)
		require.Equal(t, done, saved.Done)
	}
}
func TestTaskTraceCompletionMergeAndLegacy(t *testing.T) {
	base := TaskTraceTeamBase{PercentDone: 0.4, CompletionKnown: true}
	makeSnapshot := func(actor string, value *float64) TaskTraceTeamSnapshot {
		return TaskTraceTeamSnapshot{Actor: actor, Base: map[string]TaskTraceTeamBase{"root": base}, Tasks: []TaskTraceTeamTask{{NodeID: "root", PercentDone: value}}}
	}
	local, remote, legacy := makeSnapshot("local", taskTraceCompletionPointer(0.4)), makeSnapshot("remote", taskTraceCompletionPointer(0.7)), makeSnapshot("old", nil)
	value, _, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{local, remote, legacy}, "root", "percent_done", "0.4", "")
	require.False(t, conflict)
	require.Equal(t, "0.7", value)
	local.Tasks[0].PercentDone = taskTraceCompletionPointer(0.2)
	_, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{local, remote}, "root", "percent_done", "0.4", "")
	require.True(t, conflict)
	require.Len(t, options, 2)
	remote.Tasks[0].PercentDone = taskTraceCompletionPointer(0)
	value, _, conflict = taskTraceTeamFindField([]TaskTraceTeamSnapshot{remote, legacy}, "root", "percent_done", "0.4", "")
	require.False(t, conflict)
	require.Equal(t, "0", value, "explicit zero is an edit, unlike a missing field")
	old := `<p>kept content</p>`
	completed := `<span data-tasktrace-outstanding-state="true" hidden data-progress="40" data-done="true"></span>` + old
	require.Contains(t, taskTraceRetainOutstandingCompletion(old, completed), `data-progress="40"`)
	explicit := `<span data-tasktrace-outstanding-state="true" hidden data-progress="0"></span>` + old
	require.Equal(t, explicit, taskTraceRetainOutstandingCompletion(explicit, completed))
	body := `<h3 data-tasktrace-comment-type="outstanding">Outstanding items</h3><ul><li data-id="one" data-progress="40" data-done="true" data-priority="7">` + old + `</li></ul>`
	for range 3 {
		items, order := taskTraceTeamOutstandingItems(body)
		body = taskTraceTeamOutstandingHTML(items, order)
		require.Contains(t, body, `data-progress="40"`)
		require.Contains(t, body, `data-done="true"`)
	}
}
func TestTaskTraceCompletionSyncRoundTrip(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	task := &Task{Title: "completion sync", ProjectID: 1, PercentDone: 0.4}
	require.NoError(t, task.Create(s, actor))
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	body := `<h3 data-tasktrace-comment-type="outstanding">Outstanding items</h3><ul><li data-id="one" data-progress="30" data-done="false">one</li></ul>`
	base := map[string]TaskTraceTeamBase{"root": {Title: task.Title, Status: TaskStatusTodo, PercentDone: 0.4, CompletionKnown: true, Outstanding: body}}
	binding := TaskTraceTeamBinding{Repository: repository, ShareID: "completion", Secret: "test", Owner: "user1", Members: []string{"user1", "user2"}, RootTaskID: task.ID, ProjectID: 1, NodeTasks: map[string]int64{"root": task.ID}, Base: base}
	permissions := []TaskTraceTeamMemberPermission{{Username: "user1", Owner: true, Read: true, Write: true}, {Username: "user2", Read: true, Write: true}}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: "user1", Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: base, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), &manifest))
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
	s = db.NewSession()
	require.NoError(t, (&TaskComment{TaskID: task.ID, Comment: body}).Create(s, actor))
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	remote := TaskTraceTeamSnapshot{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Actor: "user2", DeviceID: "remote", Base: base, Tasks: []TaskTraceTeamTask{{NodeID: "root", Title: task.Title, Status: TaskStatusDone, Done: true, PercentDone: taskTraceCompletionPointer(0.7), Outstanding: `<h3 data-tasktrace-comment-type="outstanding">Outstanding items</h3><ul><li data-id="one" data-progress="60" data-done="true">one</li></ul>`}}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, remote))
	for range 3 {
		sync := db.NewSession()
		_, err := TaskTraceTeamSync(sync, actor)
		require.NoError(t, err)
		require.NoError(t, sync.Commit())
		saved, err := GetTaskByIDSimple(sync, task.ID)
		require.NoError(t, err)
		require.True(t, saved.Done)
		require.InDelta(t, 0.7, saved.PercentDone, 0.00001)
		list, err := taskTraceReadOutstanding(sync, task.ID)
		require.NoError(t, err)
		require.Contains(t, list.original, `data-progress="60"`)
		require.Contains(t, list.original, `data-done="true"`)
		require.NoError(t, sync.Close())
	}
}

func TestTaskTraceCompletionReadOnlySnapshot(t *testing.T) {
	base := TaskTraceTeamBase{PercentDone: .4, CompletionKnown: true}
	manifest := TaskTraceTeamManifest{Owner: "owner", Members: []string{"owner", "reader", "writer"}, Base: map[string]TaskTraceTeamBase{"root": base}, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {{Username: "owner", Owner: true, Read: true, Write: true}, {Username: "reader", Read: true}, {Username: "writer", Read: true, Write: true}}}}
	binding := TaskTraceTeamBinding{Base: manifest.Base}
	makeSnapshot := func(actor string) TaskTraceTeamSnapshot {
		return TaskTraceTeamSnapshot{Actor: actor, Base: map[string]TaskTraceTeamBase{"root": base}, Tasks: []TaskTraceTeamTask{{NodeID: "root", PercentDone: taskTraceCompletionPointer(.8)}}}
	}
	filtered := taskTraceTeamFilterSnapshotsPermissions([]TaskTraceTeamSnapshot{makeSnapshot("reader"), makeSnapshot("writer")}, &binding, &manifest)
	require.InDelta(t, .4, *filtered[0].Tasks[0].PercentDone, .00001)
	require.InDelta(t, .8, *filtered[1].Tasks[0].PercentDone, .00001)
	require.True(t, filtered[0].Base["root"].CompletionKnown)
	value, _, conflict := taskTraceTeamFindField(filtered, "root", "percent_done", "0.4", "")
	require.False(t, conflict)
	require.Equal(t, "0.8", value)
	manifest.Base["root"] = TaskTraceTeamBase{}
	filtered = taskTraceTeamFilterSnapshotsPermissions([]TaskTraceTeamSnapshot{makeSnapshot("reader")}, &binding, &manifest)
	require.Nil(t, filtered[0].Tasks[0].PercentDone, "untrusted peer must not initialize completion missing in a legacy manifest")
}
func TestTaskTraceCompletionDefaultDoesNotLogOrConflict(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	body := `<h3 data-tasktrace-comment-type="outstanding">Outstanding items</h3><ul><li data-id="one">one</li></ul>`
	before := taskTraceOutstandingActivityItems(body)
	zero := strings.ReplaceAll(body, `data-id="one"`, `data-id="one" data-progress="0"`)
	require.NoError(t, taskTraceRecordOutstandingActivity(s, actor, 1, before, zero))
	require.Empty(t, outstandingActivityRows(t, s))
	updated := strings.ReplaceAll(zero, `data-progress="0"`, `data-progress="30"`)
	require.NoError(t, taskTraceRecordOutstandingActivity(s, actor, 1, before, updated))
	require.Len(t, outstandingActivityRows(t, s), 1)
	base, _ := taskTraceTeamOutstandingItems(body)
	peer := TaskTraceTeamSnapshot{Actor: "peer", Base: map[string]TaskTraceTeamBase{"root": {Outstanding: body}}, Tasks: []TaskTraceTeamTask{{NodeID: "root", Outstanding: zero}}}
	value, _, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{peer}, "root", "outstanding:one", base["one"], "")
	require.False(t, conflict)
	require.Equal(t, base["one"], value)
}

func TestTaskTraceCompletionLegacyManifestProtection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	body := `<h3 data-tasktrace-comment-type="outstanding">Outstanding items</h3><ul><li data-id="one" data-progress="80">one</li></ul>`
	manifest := TaskTraceTeamManifest{ShareID: "test", TokenHash: "token", Base: map[string]TaskTraceTeamBase{"root": {PercentDone: .7, CompletionKnown: true, Outstanding: body}}}
	require.NoError(t, taskTraceTeamWriteManifest(path, &manifest))
	statePath := taskTraceCompletionStatePath(path)
	before, err := os.Stat(statePath)
	require.NoError(t, err)
	require.NoError(t, taskTraceTeamWriteManifest(path, &manifest))
	after, err := os.Stat(statePath)
	require.NoError(t, err)
	require.Equal(t, before.ModTime(), after.ModTime(), "unchanged polling must not rewrite projection")
	legacy := manifest
	legacy.Base = map[string]TaskTraceTeamBase{"root": {Outstanding: strings.ReplaceAll(body, ` data-progress="80"`, "")}}
	require.NoError(t, taskTraceTeamWriteJSON(path, &legacy))
	var restored TaskTraceTeamManifest
	require.NoError(t, taskTraceTeamReadManifest(path, &restored))
	require.InDelta(t, .7, restored.Base["root"].PercentDone, .00001)
	require.Contains(t, restored.Base["root"].Outstanding, `data-progress="80"`)
	// New A acknowledged 70, B is unchanged at 40. After old C erases the
	// manifest, B must keep protected 70 instead of publishing stale 40.
	a := TaskTraceTeamSnapshot{Actor: "A", Base: map[string]TaskTraceTeamBase{"root": restored.Base["root"]}, Tasks: []TaskTraceTeamTask{{NodeID: "root", PercentDone: taskTraceCompletionPointer(.7), Outstanding: body}}}
	staleBody := strings.ReplaceAll(body, `data-progress="80"`, `data-progress="40"`)
	b := TaskTraceTeamSnapshot{Actor: "B", Base: map[string]TaskTraceTeamBase{"root": {PercentDone: .4, CompletionKnown: true, Outstanding: staleBody}}, Tasks: []TaskTraceTeamTask{{NodeID: "root", PercentDone: taskTraceCompletionPointer(.4), Outstanding: staleBody}}}
	for _, field := range []string{"percent_done", "outstanding:one"} {
		base := taskTraceTeamBaseValue(restored.Base["root"], field)
		value, _, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{a, b}, "root", field, base, "")
		require.False(t, conflict)
		require.Equal(t, base, value)
	}
	zero := restored.Base["root"]
	zero.PercentDone = 0
	zero.Outstanding = strings.ReplaceAll(body, `data-progress="80"`, `data-progress="0"`)
	restored.Base["root"] = zero
	require.NoError(t, taskTraceTeamWriteManifest(path, &restored))
	require.NoError(t, taskTraceTeamWriteJSON(path, &legacy))
	require.NoError(t, taskTraceTeamReadManifest(path, &restored))
	require.Zero(t, restored.Base["root"].PercentDone)
	require.Contains(t, restored.Base["root"].Outstanding, `data-progress="0"`)
	legacy.Base = map[string]TaskTraceTeamBase{}
	require.NoError(t, taskTraceTeamWriteJSON(path, &legacy))
	// Decode into a fresh object, as every production manifest caller does.
	restored = TaskTraceTeamManifest{}
	require.NoError(t, taskTraceTeamReadManifest(path, &restored))
	require.Empty(t, restored.Base)
	require.NoError(t, os.WriteFile(statePath, []byte("broken"), 0o600))
	require.Error(t, taskTraceTeamReadManifest(path, &restored))
	for _, invalid := range []string{`{}`, `{"schema":1,"token_hash":"token","nodes":{}}`, `{"schema":1,"share_id":"other","token_hash":"token","nodes":{}}`} {
		require.NoError(t, os.WriteFile(statePath, []byte(invalid), 0o600))
		require.Error(t, taskTraceTeamReadManifest(path, &restored))
	}
}
