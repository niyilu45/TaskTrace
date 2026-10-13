// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

type taskTraceSyncPeer struct {
	data    string
	actor   *user.User
	binding TaskTraceTeamBinding
}

func (peer *taskTraceSyncPeer) activate(t *testing.T) {
	t.Helper()
	t.Setenv("TASKTRACE_DATA_ROOT", peer.data)
}
func (peer *taskTraceSyncPeer) sync(t *testing.T) {
	t.Helper()
	peer.activate(t)
	s := db.NewSession()
	t.Cleanup(func() { _ = s.Close() })
	_, err := TaskTraceTeamSync(s, peer.actor)
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	state, err := taskTraceTeamLoadState()
	require.NoError(t, err)
	require.Len(t, state.Bindings, 1)
	peer.binding = state.Bindings[0]
	require.Empty(t, peer.binding.LastError)
	require.Empty(t, peer.binding.Conflicts)
}
func (peer *taskTraceSyncPeer) edit(t *testing.T, change func(*xorm.Session)) {
	t.Helper()
	peer.activate(t)
	s := db.NewSession()
	t.Cleanup(func() { _ = s.Close() })
	change(s)
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
}
func (peer *taskTraceSyncPeer) list(t *testing.T, s *xorm.Session, node string, change func(map[string]string)) {
	t.Helper()
	id := peer.binding.NodeTasks[node]
	current, err := taskTraceReadOutstanding(s, id)
	require.NoError(t, err)
	items, order := taskTraceTeamOutstandingItems(current.original)
	change(items)
	body := taskTraceTeamOutstandingHTML(items, order)
	if current.comment != nil {
		current.comment.Comment = body
		can, err := current.comment.CanUpdate(s, peer.actor)
		require.NoError(t, err)
		require.True(t, can)
		require.NoError(t, current.comment.Update(s, peer.actor))
		return
	}
	comment := &TaskComment{TaskID: id, Comment: body}
	can, err := comment.CanCreate(s, peer.actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, comment.Create(s, peer.actor))
}
func TestTaskTraceTeamBidirectionalSynchronization(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	repository := t.TempDir()
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	peers := []*taskTraceSyncPeer{
		{data: t.TempDir(), actor: &user.User{ID: 1, Username: "user1"}},
		{data: t.TempDir(), actor: &user.User{ID: 2, Username: "user2"}},
	}
	base := map[string]TaskTraceTeamBase{
		"root":  {Title: "Root", Status: TaskStatusTodo, CompletionKnown: true},
		"child": {Title: "Child", Status: TaskStatusTodo, CompletionKnown: true},
	}
	permissions := []TaskTraceTeamMemberPermission{
		{Username: "user1", Owner: true, Read: true, Write: true},
		{Username: "user2", Read: true, Write: true},
	}
	manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: "bidirectional", RootNode: "root", Owner: "user1", Members: []string{"user1", "user2"}, TokenHash: taskTraceTeamTokenHash("secret"), Base: base,
		Permissions: map[string][]TaskTraceTeamMemberPermission{"root": permissions, "child": permissions}}
	require.NoError(t, taskTraceTeamWriteJSON(filepath.Join(taskTraceTeamShareDir(repository, manifest.ShareID), "manifest.json"), manifest))
	for _, peer := range peers {
		peer.activate(t)
		s := db.NewSession()
		t.Cleanup(func() { _ = s.Close() })
		project := &Project{Title: "Private tree " + peer.actor.Username}
		require.NoError(t, project.Create(s, peer.actor))
		root := &Task{Title: "Root", ProjectID: project.ID, Priority: 7}
		child := &Task{Title: "Child", ProjectID: project.ID, Priority: 7}
		require.NoError(t, root.Create(s, peer.actor))
		require.NoError(t, child.Create(s, peer.actor))
		require.NoError(t, (&TaskRelation{TaskID: root.ID, OtherTaskID: child.ID, RelationKind: RelationKindSubtask}).Create(s, peer.actor))
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		peer.binding = TaskTraceTeamBinding{Repository: repository, ShareID: manifest.ShareID, Secret: "secret", Owner: "user1", Members: manifest.Members,
			RootTaskID: root.ID, ProjectID: project.ID, NodeTasks: map[string]int64{"root": root.ID, "child": child.ID}, Base: taskTraceTeamCopyBase(base)}
		require.NoError(t, taskTraceTeamSaveState(taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: peer.actor.Username, Bindings: []TaskTraceTeamBinding{peer.binding}}))
	}
	roundtrip := func() {
		t.Helper()
		for range 3 {
			for _, peer := range peers {
				peer.sync(t)
			}
		}
	}
	roundtrip()
	// Distinct task trees, local ids, identities and journals share only the repository.
	for index, sender := range peers {
		node := []string{"root", "child"}[index]
		sender.edit(t, func(s *xorm.Session) {
			taskID := sender.binding.NodeTasks[node]
			_, err := s.ID(taskID).Cols("title", "description", "status", "done", "percent_done").Update(&Task{
				Title: "Edited by " + sender.actor.Username, Description: "<p>Shared description</p>",
				Status: TaskStatusDone, Done: true, PercentDone: .4})
			require.NoError(t, err)
			image := &TaskAttachment{TaskID: taskID}
			data := []byte("shared outstanding image")
			require.NoError(t, image.NewAttachment(s, bytes.NewReader(data), "note.png", uint64(len(data)), sender.actor))
			note := fmt.Sprintf(`<p>Item %s</p><aside data-tasktrace-outstanding-note="true" hidden><p>Note</p><img src="/api/v1/tasks/%d/attachments/%d"></aside>`, sender.actor.Username, taskID, image.ID)
			sender.list(t, s, node, func(items map[string]string) { items["shared-item"] = note })
			body := "<h3>每日进展 · 2026-10-13</h3><p>Progress from " + sender.actor.Username + "</p>" //nolint:gosmopolitan // Persisted daily progress format.
			require.NoError(t, (&TaskComment{TaskID: taskID, Comment: taskTraceTeamAddMarker(body, "progress-"+sender.actor.Username, sender.actor.Username)}).Create(s, sender.actor))
		})
		roundtrip()
		for _, receiver := range peers {
			receiver.activate(t)
			s := db.NewReadSession()
			task, err := GetTaskByIDSimple(s, receiver.binding.NodeTasks[node])
			require.NoError(t, err)
			require.Equal(t, "Edited by "+sender.actor.Username, task.Title)
			require.Equal(t, "<p>Shared description</p>", task.Description)
			require.True(t, task.Done)
			require.Equal(t, TaskStatusDone, task.Status)
			require.InDelta(t, .4, task.PercentDone, .001)
			list, err := taskTraceReadOutstanding(s, task.ID)
			require.NoError(t, err)
			require.Len(t, list.items, 1)
			require.Contains(t, list.original, "Item "+sender.actor.Username)
			require.Contains(t, list.original, "<p>Note</p>")
			require.Contains(t, list.original, fmt.Sprintf("/tasks/%d/attachments/", task.ID))
			var comments []*TaskComment
			require.NoError(t, s.Where("task_id = ?", task.ID).Find(&comments))
			progress, activities := 0, 0
			for _, comment := range comments {
				if strings.Contains(comment.Comment, "Progress from "+sender.actor.Username) {
					progress++
				}
				if strings.Contains(comment.Comment, "item-activity") {
					activities++
				}
			}
			require.Equal(t, 1, progress)
			require.Equal(t, 1, activities, "polling must not duplicate the original edit activity")
			count, err := s.Where("task_id = ?", task.ID).Count(&TaskAttachment{})
			require.NoError(t, err)
			require.EqualValues(t, 1, count, "image import must be idempotent")
			require.NoError(t, s.Close())
		}
	}
	// Concurrent changes to independent fields and items must both survive.
	peers[0].edit(t, func(s *xorm.Session) {
		_, err := s.ID(peers[0].binding.RootTaskID).Cols("description").Update(&Task{Description: "Owner concurrent description"})
		require.NoError(t, err)
		peers[0].list(t, s, "root", func(items map[string]string) { items["owner-added"] = "Owner addition" })
	})
	peers[1].edit(t, func(s *xorm.Session) {
		_, err := s.ID(peers[1].binding.RootTaskID).Cols("title").Update(&Task{Title: "Member's concurrent title"})
		require.NoError(t, err)
		peers[1].list(t, s, "root", func(items map[string]string) { items["member-added"] = "Member addition" })
	})
	roundtrip()
	for _, peer := range peers {
		peer.activate(t)
		s := db.NewReadSession()
		task, err := GetTaskByIDSimple(s, peer.binding.RootTaskID)
		require.NoError(t, err)
		require.Equal(t, "Member's concurrent title", task.Title)
		require.Equal(t, "Owner concurrent description", task.Description)
		list, err := taskTraceReadOutstanding(s, task.ID)
		require.NoError(t, err)
		require.Len(t, list.items, 3)
		require.NoError(t, s.Close())
	}
	// Each peer edits then deletes the item originally added by the other peer.
	for index, sender := range peers {
		node := []string{"child", "root"}[index]
		sender.edit(t, func(s *xorm.Session) {
			sender.list(t, s, node, func(items map[string]string) {
				items["shared-item"] = `<span data-tasktrace-outstanding-state="true" hidden data-done="true" data-progress="60" data-reminder="2026-10-14T09:00:00Z"></span><p>Changed note</p>`
			})
		})
		roundtrip()
		for _, peer := range peers {
			peer.activate(t)
			s := db.NewReadSession()
			list, err := taskTraceReadOutstanding(s, peer.binding.NodeTasks[node])
			require.NoError(t, err)
			require.Contains(t, list.original, "Changed note")
			_, nodes := taskTraceTeamOutstandingNodes(list.original)
			for _, item := range nodes {
				if taskTraceAttribute(item, "data-id") != "shared-item" {
					continue
				}
				require.Equal(t, "true", taskTraceAttribute(item, "data-done"))
				require.Equal(t, "60", taskTraceAttribute(item, "data-progress"))
				require.Equal(t, "2026-10-14T09:00:00Z", taskTraceAttribute(item, "data-reminder"))
			}
			require.NoError(t, s.Close())
		}
		sender.edit(t, func(s *xorm.Session) {
			sender.list(t, s, node, func(items map[string]string) { delete(items, "shared-item") })
		})
		roundtrip()
		for _, peer := range peers {
			peer.activate(t)
			s := db.NewReadSession()
			list, err := taskTraceReadOutstanding(s, peer.binding.NodeTasks[node])
			require.NoError(t, err)
			items, _ := taskTraceTeamOutstandingItems(list.original)
			require.NotContains(t, items, "shared-item", "explicit deletion must not be revived")
			require.NoError(t, s.Close())
		}
	}
}
