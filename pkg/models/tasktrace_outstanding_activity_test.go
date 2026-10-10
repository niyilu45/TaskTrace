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
	"context"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func outstandingActivityRows(t *testing.T, s *xorm.Session) []*TaskComment {
	t.Helper()
	var rows []*TaskComment
	require.NoError(t, s.Where("task_id = ? AND comment LIKE ?", 1, "%item-activity%").Asc("id").Find(&rows))
	return rows
}

//nolint:gosmopolitan // Verify the persisted Chinese labels used by the local UI.
func TestTaskTraceOutstandingActivityLifecycle(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	events.ClearDispatchedEvents()
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	body := `<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-done="false" data-priority="7"><p>等待确认</p></li></ul>`
	comment := &TaskComment{TaskID: 1, Comment: body}
	can, err := comment.CanCreate(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, comment.Create(s, actor))
	require.Len(t, outstandingActivityRows(t, s), 1)
	retry := &TaskComment{TaskID: 1, Comment: body}
	can, err = retry.CanCreate(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, retry.Create(s, actor))
	require.Equal(t, comment.ID, retry.ID)
	require.Len(t, outstandingActivityRows(t, s), 1)
	update := func(body string) {
		t.Helper()
		edit := &TaskComment{ID: comment.ID, TaskID: 1, Comment: body}
		allowed, checkErr := edit.CanUpdate(s, actor)
		require.NoError(t, checkErr)
		require.True(t, allowed)
		require.NoError(t, edit.Update(s, actor))
	}
	body = strings.ReplaceAll(body, "等待确认", "等待复测")
	update(body)
	update(body)
	require.Len(t, outstandingActivityRows(t, s), 2)
	body = strings.ReplaceAll(body, `data-done="false"`, `data-done="true"`)
	update(body)
	body = strings.ReplaceAll(body, `data-done="true"`, `data-done="false"`)
	update(body)
	body = strings.ReplaceAll(body, `data-priority="7"`, `data-priority="2"`)
	update(body)
	remove := &TaskComment{TaskID: 1, ID: comment.ID}
	can, err = remove.CanDelete(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, remove.Delete(s, actor))
	rows := outstandingActivityRows(t, s)
	require.Len(t, rows, 6)
	for index, action := range []string{"新增", "修改", "完成", "重新打开", "修改优先级", "删除"} {
		require.Contains(t, rows[index].Comment, action)
		require.False(t, taskTraceTeamIsOutstanding(rows[index].Comment))
		marker, ok := taskTraceTeamReadMarker(rows[index].Comment)
		require.True(t, ok)
		require.Equal(t, "user1", marker.Author)
		require.Equal(t, int64(1), rows[index].AuthorID)
		require.False(t, rows[index].Created.IsZero())
	}
	require.True(t, taskTraceOutstandingLocalActivity(rows[4].Comment))
	require.False(t, taskTraceOutstandingLocalActivity(rows[1].Comment))

	exportRoot := &Task{Title: "activity export", ProjectID: 1}
	require.NoError(t, exportRoot.Create(s, actor))
	for _, row := range rows {
		cloned := *row
		cloned.ID, cloned.TaskID = 0, exportRoot.ID
		_, insertErr := s.Insert(&cloned)
		require.NoError(t, insertErr)
	}
	binding := &TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "activity-export", RootTaskID: exportRoot.ID, NodeTasks: map[string]int64{"root": exportRoot.ID}}
	snapshot, err := taskTraceTeamBuildSnapshot(s, binding, "user1", "device")
	require.NoError(t, err)
	exported := 0
	for _, task := range snapshot.Tasks {
		for _, comment := range task.Comments {
			require.False(t, taskTraceOutstandingLocalActivity(comment.Body))
			if strings.Contains(comment.Body, "item-activity") {
				exported++
			}
		}
	}
	require.Equal(t, 5, exported, "personal priority history stays local")
	require.NoError(t, s.Commit())
	events.DispatchPending(context.Background(), s)
	require.Equal(t, 1, events.CountDispatchedEvents((&TaskCommentCreatedEvent{}).Name()))
	require.Equal(t, 4, events.CountDispatchedEvents((&TaskCommentUpdatedEvent{}).Name()))
	require.Equal(t, 1, events.CountDispatchedEvents((&TaskCommentDeletedEvent{}).Name()))
}

//nolint:gosmopolitan // Verify the persisted Chinese labels used by the local UI.
func TestTaskTraceOutstandingActivityReplayAndRollback(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	body := `<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="one">协作遗留事项</li></ul>`
	require.NoError(t, taskTraceTeamUpsertOutstanding(s, actor, 1, body))
	require.NoError(t, taskTraceTeamUpsertOutstanding(s, actor, 1, strings.ReplaceAll(body, "协作", "更新协作")))
	require.Empty(t, outstandingActivityRows(t, s))
	logBody := `<p data-tasktrace-comment-type="item-activity" data-item-id="one"><strong>新增遗留事项</strong>：协作遗留事项</p>`
	binding := &TaskTraceTeamBinding{ShareID: "activity-test"}
	remote := []TaskTraceTeamComment{{ID: "bob-operation", Author: "bob", Body: logBody}}
	for range 3 {
		require.NoError(t, taskTraceTeamMergeComments(s, actor, binding, "node", "user1", 1, remote))
	}
	rows := outstandingActivityRows(t, s)
	require.Len(t, rows, 1)
	marker, ok := taskTraceTeamReadMarker(rows[0].Comment)
	require.True(t, ok)
	require.Equal(t, "bob", marker.Author)
	require.NoError(t, s.Rollback())
	read := db.NewSession()
	defer read.Close()
	require.Empty(t, outstandingActivityRows(t, read))
}

//nolint:gosmopolitan // Verify the persisted Chinese labels used by the local UI.
func TestTaskTraceOutstandingActivityContentAndPermission(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	body := `<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="one">名称 &lt;img&gt;<aside data-tasktrace-outstanding-note="true" hidden>备注<img src="/image"></aside></li></ul>`
	comment := &TaskComment{TaskID: 1, Comment: body}
	can, err := comment.CanCreate(s, &user.User{ID: 999})
	require.NoError(t, err)
	require.False(t, can)
	require.False(t, comment.recordOutstandingActivity)
	can, err = comment.CanCreate(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, comment.Create(s, actor))
	changed := strings.ReplaceAll(body, "备注", "新备注")
	edit := &TaskComment{TaskID: 1, ID: comment.ID, Comment: changed}
	can, err = edit.CanUpdate(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, edit.Update(s, actor))
	rows := outstandingActivityRows(t, s)
	require.Len(t, rows, 2)
	require.Contains(t, rows[1].Comment, "修改遗留事项")
	require.Contains(t, rows[1].Comment, "&lt;img&gt;")
	require.NotContains(t, rows[1].Comment, "<img")
	require.NotContains(t, rows[1].Comment, "备注")
	denied := &TaskComment{TaskID: 1, ID: comment.ID}
	can, err = denied.CanUpdate(s, &user.User{ID: 999})
	require.NoError(t, err)
	require.False(t, can)
	can, err = denied.CanDelete(s, &user.User{ID: 999})
	require.NoError(t, err)
	require.False(t, can)
	require.NoError(t, s.Rollback())
	read := db.NewSession()
	defer read.Close()
	require.Empty(t, outstandingActivityRows(t, read), "list and history roll back together")
}
