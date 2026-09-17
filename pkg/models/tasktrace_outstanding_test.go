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
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func taskTraceSeedList(t *testing.T, taskID int64, items ...taskTraceOutstandingItem) int64 {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	task, err := GetTaskByIDSimple(s, taskID)
	require.NoError(t, err)
	u := &user.User{ID: 1}
	id, err := taskTraceWriteOutstanding(s, u, u, task, &taskTraceOutstandingList{items: items})
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return id
}
func taskTraceReadList(t *testing.T, taskID int64) *taskTraceOutstandingList {
	t.Helper()
	s := db.NewSession()
	defer s.Close()
	list, err := taskTraceReadOutstanding(s, taskID)
	require.NoError(t, err)
	return list
}
func taskTraceRunMove(move *TaskTraceOutstandingMove) error {
	s := db.NewSession()
	defer s.Close()
	defer events.CleanupPending(s)
	actor := &user.User{ID: 1}
	can, err := move.CanCreate(s, actor)
	if err == nil && !can {
		err = ErrGenericForbidden{}
	}
	if err == nil {
		err = move.Create(s, actor)
	}
	if err != nil {
		_ = s.Rollback()
		return err
	}
	return s.Commit()
}
func taskTraceMove(t *testing.T, move *TaskTraceOutstandingMove) error {
	t.Helper()
	err := taskTraceRunMove(move)
	if err != nil {
		move.CleanupCreatedFiles()
	}
	return err
}
func taskTraceIDs(list *taskTraceOutstandingList) []string {
	ids := []string{}
	for _, item := range list.items {
		ids = append(ids, item.id)
	}
	return ids
}

func TestTaskTraceOutstandingMove(t *testing.T) {
	t.Run("reorder then move without losing neighboring items", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		sourceID := taskTraceSeedList(t, 1, taskTraceOutstandingItem{"a", "<b>A</b>", ` data-done="true" data-priority="2"`}, taskTraceOutstandingItem{"b", "B", ""}, taskTraceOutstandingItem{"c", "C", ""})
		targetID := taskTraceSeedList(t, 2, taskTraceOutstandingItem{"d", "D", ""})
		require.NoError(t, taskTraceMove(t, &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 1, ItemID: "c", BeforeItemID: "a"}))
		assert.Equal(t, []string{"c", "a", "b"}, taskTraceIDs(taskTraceReadList(t, 1)))
		move := &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 2, ItemID: "a", BeforeItemID: "d"}
		require.NoError(t, taskTraceMove(t, move))
		assert.Equal(t, sourceID, move.SourceCommentID)
		assert.Equal(t, targetID, move.TargetCommentID)
		assert.Equal(t, []string{"c", "b"}, taskTraceIDs(taskTraceReadList(t, 1)))
		target := taskTraceReadList(t, 2)
		assert.Equal(t, []string{"a", "d"}, taskTraceIDs(target))
		assert.Equal(t, "<b>A</b>", target.items[0].content)
		assert.Equal(t, ` data-done="true" data-priority="2"`, target.items[0].metadata)
		require.NoError(t, taskTraceMove(t, &TaskTraceOutstandingMove{TaskID: 2, TargetTaskID: 2, ItemID: "a"}))
		assert.Equal(t, []string{"d", "a"}, taskTraceIDs(taskTraceReadList(t, 2)))
	})
	t.Run("stale or duplicate item does not change either list", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		taskTraceSeedList(t, 1, taskTraceOutstandingItem{"a", "A", ""})
		taskTraceSeedList(t, 2, taskTraceOutstandingItem{"b", "B", ""})
		for _, move := range []*TaskTraceOutstandingMove{
			{TaskID: 1, TargetTaskID: 2, ItemID: "missing"},
			{TaskID: 1, TargetTaskID: 2, ItemID: "a", BeforeItemID: "missing"},
		} {
			require.ErrorAs(t, taskTraceMove(t, move), new(ErrTaskTraceOutstandingMove))
			assert.Equal(t, []string{"a"}, taskTraceIDs(taskTraceReadList(t, 1)))
			assert.Equal(t, []string{"b"}, taskTraceIDs(taskTraceReadList(t, 2)))
		}
		taskTraceSeedList(t, 2, taskTraceOutstandingItem{"a", "Other A", ""})
		require.ErrorAs(t, taskTraceMove(t, &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 2, ItemID: "a"}), new(ErrTaskTraceOutstandingMove))
		assert.Equal(t, "A", taskTraceReadList(t, 1).items[0].content)
		assert.Equal(t, "Other A", taskTraceReadList(t, 2).items[0].content)
	})
	t.Run("moving last legacy item creates authoritative empty source", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		note := &TaskComment{TaskID: 1, AuthorID: 1, Comment: "<h3>每日进展 · 2026-09-17</h3><p>done</p><p><strong>遗留问题 / 下一步</strong></p><p>legacy</p>"}
		_, err := s.Insert(note)
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		s.Close()
		require.NoError(t, taskTraceMove(t, &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 2, ItemID: fmt.Sprintf("legacy-%d-0", note.ID)}))
		list := taskTraceReadList(t, 1)
		assert.Empty(t, list.items)
		require.NotNil(t, list.comment)
		db.AssertExists(t, "task_comments", map[string]interface{}{"id": note.ID, "comment": note.Comment}, false)
		assert.Equal(t, "legacy", taskTraceReadList(t, 2).items[0].content)
	})
	t.Run("compare and swap refuses a changed comment", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		taskTraceSeedList(t, 1, taskTraceOutstandingItem{"a", "A", ""})
		s := db.NewSession()
		defer s.Close()
		stale, err := taskTraceReadOutstanding(s, 1)
		require.NoError(t, err)
		_, err = s.ID(stale.comment.ID).Cols("comment").Update(&TaskComment{Comment: "concurrent edit"})
		require.NoError(t, err)
		stale.items[0].content = "overwrite"
		_, err = taskTraceWriteOutstanding(s, &user.User{ID: 1}, &user.User{ID: 1}, Task{ID: 1}, stale)
		require.ErrorAs(t, err, new(ErrTaskTraceOutstandingMove))
		var saved TaskComment
		_, err = s.ID(stale.comment.ID).Get(&saved)
		require.NoError(t, err)
		assert.Equal(t, "concurrent edit", saved.Comment)
	})
}

func TestTaskTraceOutstandingPermissions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		task    int64
		auth    web.Auth
		allowed bool
	}{
		{"owner", 1, &user.User{ID: 1}, true},
		{"unrelated user", 1, &user.User{ID: 2}, false},
		{"team write", 16, &user.User{ID: 1}, true},
		{"team read only", 15, &user.User{ID: 1}, false},
		{"inherited project write", 13, &user.User{ID: 3}, true},
		{"inherited ancestor write", 25, &user.User{ID: 1}, true},
		{"read only link", 1, &LinkSharing{ID: 1, ProjectID: 1, Permission: PermissionRead}, false},
		{"write link", 13, &LinkSharing{ID: 2, ProjectID: 2, Permission: PermissionWrite}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			s := db.NewSession()
			defer s.Close()
			m := &TaskTraceOutstandingMove{TaskID: tc.task, TargetTaskID: tc.task, ItemID: "a"}
			can, err := m.CanCreate(s, tc.auth)
			require.NoError(t, err)
			assert.Equal(t, tc.allowed, can)
		})
	}
	t.Run("destination must be writable", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		can, err := (&TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 34}).CanCreate(s, &user.User{ID: 1})
		require.NoError(t, err)
		assert.False(t, can)
	})
	t.Run("same project required even when both writable", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		s := db.NewSession()
		defer s.Close()
		can, err := (&TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 16}).CanCreate(s, &user.User{ID: 1})
		require.ErrorAs(t, err, new(ErrTaskTraceOutstandingMove))
		assert.False(t, can)
	})
}

func TestTaskTraceOutstandingImages(t *testing.T) {
	t.Run("copied image survives source attachment deletion", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		files.InitTestFileFixtures(t)
		s := db.NewSession()
		original := &TaskAttachment{TaskID: 1}
		content := []byte("test image bytes")
		require.NoError(t, original.NewAttachment(s, bytes.NewReader(content), "image.png", uint64(len(content)), &user.User{ID: 1}))
		require.NoError(t, s.Commit())
		s.Close()
		image := fmt.Sprintf(`<img src="/api/v1/tasks/1/attachments/%d"/>`, original.ID)
		taskTraceSeedList(t, 1, taskTraceOutstandingItem{"move", "image " + image + image, ""}, taskTraceOutstandingItem{"keep", image, ""})
		require.NoError(t, taskTraceMove(t, &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 2, ItemID: "move"}))
		target := taskTraceReadList(t, 2)
		assert.Contains(t, target.items[0].content, "/api/v1/tasks/2/attachments/")
		assert.NotContains(t, target.items[0].content, "/api/v1/tasks/1/attachments/")
		assert.Contains(t, taskTraceReadList(t, 1).items[0].content, fmt.Sprintf("/api/v1/tasks/1/attachments/%d", original.ID))
		s = db.NewSession()
		var copies []*TaskAttachment
		require.NoError(t, s.Where("task_id = ?", 2).Find(&copies))
		require.Len(t, copies, 1)
		assert.NotEqual(t, original.FileID, copies[0].FileID)
		require.NoError(t, original.Delete(s, &user.User{ID: 1}))
		require.NoError(t, s.Commit())
		s.Close()
		copiedFile := &files.File{ID: copies[0].FileID}
		require.NoError(t, copiedFile.LoadFileByID())
		defer copiedFile.File.Close()
		actual, err := io.ReadAll(copiedFile.File)
		require.NoError(t, err)
		assert.Equal(t, content, actual)
	})
	t.Run("broken image aborts lists and cleans earlier copies", func(t *testing.T) {
		db.LoadAndAssertFixtures(t)
		files.InitTestFileFixtures(t)
		taskTraceSeedList(t, 1, taskTraceOutstandingItem{"a", `<img src="/api/v1/tasks/1/attachments/1"><img src="/api/v1/tasks/1/attachments/999999">`, ""})
		taskTraceSeedList(t, 2, taskTraceOutstandingItem{"b", "B", ""})
		move := &TaskTraceOutstandingMove{TaskID: 1, TargetTaskID: 2, ItemID: "a"}
		err := taskTraceRunMove(move)
		require.Error(t, err)
		require.Len(t, move.createdFiles, 1)
		copyID := move.createdFiles[0]
		move.CleanupCreatedFiles()
		missing := &files.File{ID: copyID}
		require.Error(t, missing.LoadFileByID())
		assert.Equal(t, []string{"a"}, taskTraceIDs(taskTraceReadList(t, 1)))
		assert.Equal(t, []string{"b"}, taskTraceIDs(taskTraceReadList(t, 2)))
		db.AssertMissing(t, "task_attachments", map[string]interface{}{"task_id": 2})
		db.AssertMissing(t, "files", map[string]interface{}{"id": copyID})
	})
}

func TestTaskTraceOutstandingLegacyParsing(t *testing.T) {
	created := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC)
	comments := []*TaskComment{
		{ID: 10, Created: created, Comment: `<h3>每日进展 · 2026-09-16</h3><p><strong>遗留问题 / 下一步</strong></p><p>old</p>`},
		{ID: 11, Created: created, Comment: `<h3 data-tasktrace-merged="12">每日进展 · 2026-09-17</h3><p><strong>遗留问题 / 下一步</strong></p><p>A<br/>B</p>`},
		{ID: 12, Created: created, Comment: `<h3>每日进展 · 2026-09-17</h3><p><strong>遗留问题 / 下一步</strong></p><p>absorbed</p>`},
	}
	parsed, err := taskTraceParseOutstanding(comments)
	require.NoError(t, err)
	assert.Equal(t, []string{"legacy-11-0", "legacy-11-1"}, taskTraceIDs(parsed))
	comments = append(comments, &TaskComment{ID: 13, Comment: "<h3>" + taskTraceOutstandingHeading + "</h3><ul></ul>"})
	parsed, err = taskTraceParseOutstanding(comments)
	require.NoError(t, err)
	assert.Empty(t, parsed.items)
	assert.True(t, strings.Contains(parsed.original, taskTraceOutstandingHeading))
}
