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
	"path/filepath"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamNewOutstandingSurvivesRefresh(t *testing.T) {
	for _, history := range []struct {
		name   string
		count  int
		hidden bool
	}{
		{name: "one visible activity"},
		{name: "long history", count: 55},
		{name: "one visible activity with legacy hidden lists", count: 55, hidden: true},
	} {
		for _, owner := range []string{"user1", "user2"} {
			t.Run(fmt.Sprintf("%s/owner=%s", history.name, owner), func(t *testing.T) {
				db.LoadAndAssertFixtures(t)
				t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
				repository := t.TempDir()
				t.Setenv("TASKTRACE_TEAM_ROOT", repository)
				originalLimit := config.ServiceMaxItemsPerPage.GetInt()
				config.ServiceMaxItemsPerPage.Set(50)
				t.Cleanup(func() { config.ServiceMaxItemsPerPage.Set(originalLimit) })
				actor := &user.User{ID: 1, Username: "user1"}
				seed := db.NewSession()
				t.Cleanup(func() { _ = seed.Close() })
				root := &Task{Title: "New shared outstanding", ProjectID: 1, Priority: 7}
				require.NoError(t, root.Create(seed, actor))
				for index := 0; index < history.count; index++ {
					body := fmt.Sprintf("Earlier comment %d", index)
					if history.hidden {
						// Older builds could leave multiple hidden canonical rows. The
						// visible comment count does not include these empty lists.
						body = `<h3 data-tasktrace-comment-type="outstanding">Legacy list</h3><ul></ul>`
					}
					_, err := seed.Insert(&TaskComment{TaskID: root.ID, AuthorID: actor.ID, Comment: body})
					require.NoError(t, err)
				}
				require.NoError(t, seed.Commit())
				require.NoError(t, seed.Close())
				base := map[string]TaskTraceTeamBase{"root": {Title: root.Title, Status: TaskStatusTodo}}
				binding := TaskTraceTeamBinding{Repository: repository, ShareID: "refresh", Secret: "secret", Owner: owner,
					Members: []string{"user1", "user2"}, RootTaskID: root.ID, ProjectID: 1,
					NodeTasks: map[string]int64{"root": root.ID}, Base: base}
				permissions := []TaskTraceTeamMemberPermission{
					{Username: "user1", Owner: owner == "user1", Read: true, Write: true},
					{Username: "user2", Owner: owner == "user2", Read: true, Write: true},
				}
				manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: binding.ShareID, RootNode: "root", Owner: owner,
					Members: binding.Members, TokenHash: taskTraceTeamTokenHash(binding.Secret), Base: base,
					Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions}}
				require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, binding.ShareID), "manifest.json"), manifest))
				require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "local", Bindings: []TaskTraceTeamBinding{binding}}))
				syncTask := func() {
					t.Helper()
					session := db.NewSession()
					t.Cleanup(func() { _ = session.Close() })
					_, err := TaskTraceTeamSync(session, actor)
					require.NoError(t, err)
					require.NoError(t, session.Commit())
					require.NoError(t, session.Close())
					state, err := taskTraceTeamLoadState()
					require.NoError(t, err)
					require.Empty(t, state.Bindings[0].LastError)
					require.Empty(t, state.Bindings[0].Conflicts)
				}
				if !history.hidden {
					syncTask()
				}
				edit := db.NewSession()
				t.Cleanup(func() { _ = edit.Close() })
				image := &TaskAttachment{TaskID: root.ID}
				imageData := []byte("new outstanding image")
				require.NoError(t, image.NewAttachment(edit, bytes.NewReader(imageData), "outstanding.png", uint64(len(imageData)), actor))
				imageURL := fmt.Sprintf("/api/v1/tasks/%d/attachments/%d", root.ID, image.ID)
				content := `<p>Just saved</p><aside data-tasktrace-outstanding-note="true" hidden><p>Note</p><img src="` + imageURL + `"></aside>`
				body := taskTraceTeamOutstandingHTML(map[string]string{"new-item": content}, nil)
				body = taskTraceTeamApplyOutstandingPriorities(body, map[string]string{"new-item": "3"})
				comment := &TaskComment{TaskID: root.ID, Comment: body}
				can, err := comment.CanCreate(edit, actor)
				require.NoError(t, err)
				require.True(t, can)
				require.NoError(t, comment.Create(edit, actor))
				require.NoError(t, edit.Commit())
				require.NoError(t, edit.Close())
				for cycle := 0; cycle < 3; cycle++ {
					syncTask()
					read := db.NewReadSession()
					t.Cleanup(func() { _ = read.Close() })
					saved, err := taskTraceReadOutstanding(read, root.ID)
					require.NoError(t, err)
					var activities []*TaskComment
					require.NoError(t, read.Where("task_id = ? AND comment LIKE ?", root.ID, "%item-activity%").Find(&activities))
					require.Len(t, activities, 1, "the original add activity must remain without duplicate sync records")
					require.Contains(t, saved.original, "Just saved", "refresh must not erase the list while leaving its activity")
					require.Contains(t, saved.original, "Note")
					require.Contains(t, saved.original, imageURL)
					comments, _, _, err := getAllCommentsForTasksWithoutPermissionCheck(read, []int64{root.ID}, "", 1, 100, "desc")
					require.NoError(t, err)
					listed, err := taskTraceParseOutstanding(comments)
					require.NoError(t, err)
					require.Equal(t, saved.original, listed.original, "web history must return the same saved list")
					if history.hidden || history.count == 0 {
						visible := 0
						for _, row := range comments {
							if !taskTraceTeamIsOutstanding(row.Comment) {
								visible++
							}
						}
						require.Equal(t, 1, visible, "only the add activity should appear in the comment section")
					}
					attachmentCount, err := read.Where("task_id = ?", root.ID).Count(&TaskAttachment{})
					require.NoError(t, err)
					require.EqualValues(t, 1, attachmentCount, "refresh must not duplicate the uploaded image")
					require.Equal(t, "3", taskTraceTeamOutstandingPriorities(saved.original)["new-item"])
					require.NoError(t, read.Close())
				}
			})
		}
	}
}

func TestTaskTraceTeamSnapshotIncludesEveryComment(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	actor := &user.User{ID: 1, Username: "user1"}
	s := db.NewSession()
	defer s.Close()
	task := &Task{Title: "Long comment history", ProjectID: 1}
	require.NoError(t, task.Create(s, actor))
	limit := config.ServiceMaxItemsPerPage.GetInt()
	config.ServiceMaxItemsPerPage.Set(2)
	t.Cleanup(func() { config.ServiceMaxItemsPerPage.Set(limit) })
	for index := 0; index < 5; index++ {
		require.NoError(t, (&TaskComment{TaskID: task.ID, Comment: fmt.Sprintf("Progress %d", index)}).Create(s, actor))
	}
	list := &TaskComment{TaskID: task.ID, Comment: taskTraceTeamOutstandingHTML(map[string]string{"last": "Saved after comments"}, nil)}
	require.NoError(t, list.Create(s, actor))
	// Imported timestamps can put the newest canonical list anywhere in the history.
	_, err := s.ID(list.ID).Cols("created").Update(&TaskComment{Created: time.Now().Add(time.Hour)})
	require.NoError(t, err)
	binding := TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "all-comments", RootTaskID: task.ID, NodeTasks: map[string]int64{"root": task.ID}}
	snapshot, err := taskTraceTeamBuildSnapshot(s, &binding, actor.Username, "local")
	require.NoError(t, err)
	require.Len(t, snapshot.Tasks, 1)
	require.Len(t, snapshot.Tasks[0].Comments, 5, "sync must never use a UI page as the complete comment state")
	require.Contains(t, snapshot.Tasks[0].Outstanding, "Saved after comments")
}
