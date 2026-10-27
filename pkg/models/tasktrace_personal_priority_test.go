// SPDX-License-Identifier: AGPL-3.0-or-later

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

func seedPersonalPriorityReader(t *testing.T) {
	t.Helper()
	t.Setenv("TASKTRACE_TEAM_ROOT", t.TempDir())
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	binding := TaskTraceTeamBinding{ShareID: "priority", Repository: taskTraceTeamRoot(), Owner: "remote-owner", Members: []string{"remote-owner", "user1"}, RootTaskID: 1, NodeTasks: map[string]int64{"root": 1}}
	require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, Bindings: []TaskTraceTeamBinding{binding}}))
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, Owner: binding.Owner, RootNode: "root", Members: binding.Members, Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {{Username: "remote-owner", Read: true, Write: true, Owner: true}, {Username: "user1", Read: true}}}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json"), &manifest))
}

func TestTaskTracePersonalPriorityReadonlyAndUndo(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	seedPersonalPriorityReader(t)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	can, err := (&Task{ID: 1}).CanUpdate(s, actor)
	require.NoError(t, err)
	require.False(t, can)
	personal := &TaskTracePersonalPriority{TaskID: 1, Priority: 2}
	can, err = personal.CanUpdate(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	can, err = personal.CanUpdate(s, &user.User{ID: 999, Username: "unrelated"})
	require.NoError(t, err)
	require.False(t, can)
	before, err := taskTracePersonalPriorityValue(s, 1, "")
	require.NoError(t, err)
	s.Close()
	payload, changed := taskTraceUndoSnapshotMutation(t, "update", personal)
	require.True(t, changed)
	s = db.NewSession()
	after, err := taskTracePersonalPriorityValue(s, 1, "")
	s.Close()
	require.NoError(t, err)
	require.Equal(t, int64(2), after)
	require.NoError(t, taskTraceUndoSnapshotApply(payload, actor))
	s = db.NewSession()
	restored, err := taskTracePersonalPriorityValue(s, 1, "")
	s.Close()
	require.NoError(t, err)
	require.Equal(t, before, restored)
}

func TestTaskTracePersonalOutstandingPriorityPreservesContentAndUndo(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	seedPersonalPriorityReader(t)
	id := taskTraceSeedList(t, 1, taskTraceOutstandingItem{id: "item", content: `<p>Body <img src="/api/v1/tasks/1/attachments/1"></p><aside data-tasktrace-outstanding-note="true">Note</aside>`, metadata: ` data-priority="7" data-done="false"`})
	s := db.NewSession()
	old := &TaskComment{ID: id, TaskID: 1}
	require.NoError(t, getTaskCommentSimple(s, old))
	s.Close()
	payload, changed := taskTraceUndoSnapshotMutation(t, "update", &TaskTracePersonalPriority{TaskID: 1, OutstandingID: "item", Priority: 0})
	require.True(t, changed)
	s = db.NewSession()
	now := &TaskComment{ID: id, TaskID: 1}
	require.NoError(t, getTaskCommentSimple(s, now))
	s.Close()
	require.Equal(t, old.AuthorID, now.AuthorID)
	require.Equal(t, old.Updated, now.Updated)
	require.Equal(t, taskTraceTeamStripOutstandingPriorities(old.Comment), taskTraceTeamStripOutstandingPriorities(now.Comment))
	require.Contains(t, now.Comment, `data-priority="0"`)
	// An unrelated later edit must survive undo of the personal priority.
	s = db.NewSession()
	_, err := s.ID(id).NoAutoTime().Cols("comment").Update(&TaskComment{Comment: strings.ReplaceAll(now.Comment, "Note", "New note")})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	s.Close()
	require.NoError(t, taskTraceUndoSnapshotApply(payload, &user.User{ID: 1, Username: "user1"}))
	s = db.NewSession()
	now = &TaskComment{ID: id, TaskID: 1}
	require.NoError(t, getTaskCommentSimple(s, now))
	s.Close()
	require.Contains(t, now.Comment, `data-priority="7"`)
	require.Contains(t, now.Comment, "New note")
}

func TestTaskTracePersonalPriorityWireAndDefault(t *testing.T) {
	seedPersonalPriorityReader(t)
	//nolint:gosec // The environment directory was set to an isolated t.TempDir by this test.
	require.NoError(t, os.WriteFile(filepath.Join(os.Getenv("TASKTRACE_DATA_ROOT"), "autosave.json"), []byte(`{"default_priority":4}`), 0600))
	require.Equal(t, int64(4), taskTraceDefaultDisplayPriority())
	binding := TaskTraceTeamBinding{Repository: taskTraceTeamRoot(), ShareID: "priority"}
	snapshot := TaskTraceTeamSnapshot{Actor: "user1", DeviceID: "device", Tasks: []TaskTraceTeamTask{{NodeID: "root", Outstanding: `<h3 data-tasktrace-comment-type="outstanding">TaskTrace outstanding</h3><ul><li data-id="item" data-priority="2">Body</li></ul>`}}}
	require.NoError(t, taskTraceTeamWriteSnapshot(&binding, snapshot))
	raw, err := os.ReadFile(taskTraceTeamSnapshotPath(&binding, snapshot.Actor, snapshot.DeviceID))
	require.NoError(t, err)
	require.NotContains(t, string(raw), "data-priority")
	require.Contains(t, snapshot.Tasks[0].Outstanding, "data-priority", "serializing must not mutate the caller's local preference")
	other := snapshot
	other.Tasks = append([]TaskTraceTeamTask(nil), snapshot.Tasks...)
	other.Tasks[0].Outstanding = strings.ReplaceAll(other.Tasks[0].Outstanding, `data-priority="2"`, `data-priority="8"`)
	require.Equal(t, taskTraceTeamSnapshotHash(snapshot), taskTraceTeamSnapshotHash(other))
}

func TestTaskTracePersonalPriorityUndoRejectsLaterPriority(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	seedPersonalPriorityReader(t)
	payload, changed := taskTraceUndoSnapshotMutation(t, "update", &TaskTracePersonalPriority{TaskID: 1, Priority: 2})
	require.True(t, changed)
	s := db.NewSession()
	require.NoError(t, (&TaskTracePersonalPriority{TaskID: 1, Priority: 1}).Update(s, &user.User{ID: 1, Username: "user1"}))
	require.NoError(t, s.Commit())
	s.Close()
	require.Error(t, taskTraceUndoSnapshotApply(payload, &user.User{ID: 1, Username: "user1"}))
	s = db.NewSession()
	value, err := taskTracePersonalPriorityValue(s, 1, "")
	s.Close()
	require.NoError(t, err)
	require.Equal(t, int64(1), value)
}
