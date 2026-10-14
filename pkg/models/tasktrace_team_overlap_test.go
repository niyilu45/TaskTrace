// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

// A parent shared later is a separate collaboration. Members of the old
// child link must still exchange that child's edits with the owner, without
// gaining the private parent or sibling tasks.
func TestTaskTraceTeamOverlappingParentAndChildShares(t *testing.T) {
	for _, parentFirst := range []bool{false, true} {
		name := "child-first"
		if parentFirst {
			name = "parent-first"
		}
		t.Run(name, func(t *testing.T) {
			db.LoadAndAssertFixtures(t)
			repository := t.TempDir()
			t.Setenv("TASKTRACE_TEAM_ROOT", repository)
			owner := &user.User{ID: 1, Username: "user1"}
			member := &user.User{ID: 2, Username: "user2"}
			ownerData, memberData := t.TempDir(), t.TempDir()
			t.Setenv("TASKTRACE_DATA_ROOT", ownerData)
			s := db.NewSession()
			defer s.Close()
			project := &Project{Title: "Owner project"}
			require.NoError(t, project.Create(s, owner))
			parent := &Task{Title: "Private parent", ProjectID: project.ID, Priority: 7}
			child := &Task{Title: "Originally shared child", ProjectID: project.ID, Priority: 7}
			sibling := &Task{Title: "Private sibling", ProjectID: project.ID, Priority: 7}
			for _, task := range []*Task{parent, child, sibling} {
				require.NoError(t, task.Create(s, owner))
			}
			for _, task := range []*Task{child, sibling} {
				require.NoError(t, (&TaskRelation{TaskID: parent.ID, OtherTaskID: task.ID, RelationKind: RelationKindSubtask}).Create(s, owner))
			}
			memberProject := &Project{Title: "Member project"}
			require.NoError(t, memberProject.Create(s, member))
			require.NoError(t, s.Commit())
			require.NoError(t, s.Close())

			publish := func(root int64, shareID string) TaskTraceTeamBinding {
				t.Helper()
				session := db.NewSession()
				defer session.Close()
				binding := TaskTraceTeamBinding{ShareID: shareID, Repository: repository, Secret: "test", Owner: owner.Username,
					Members: []string{owner.Username, member.Username}, RootTaskID: root, ProjectID: project.ID,
					NodeTasks: map[string]int64{}, Base: map[string]TaskTraceTeamBase{}}
				snapshot, err := taskTraceTeamBuildSnapshot(session, &binding, owner.Username, "owner-device")
				require.NoError(t, err)
				require.NotEmpty(t, snapshot.Tasks)
				manifest := TaskTraceTeamManifest{Schema: taskTraceTeamSchema, ShareID: shareID, RootNode: snapshot.Tasks[0].NodeID,
					Owner: owner.Username, Members: binding.Members, Base: taskTraceTeamBaseFromSnapshot(snapshot),
					TokenHash: taskTraceTeamTokenHash(binding.Secret), Permissions: map[string][]TaskTraceTeamMemberPermission{}, Updated: time.Now().UTC()}
				for _, task := range snapshot.Tasks {
					manifest.Permissions[task.NodeID] = []TaskTraceTeamMemberPermission{
						{Username: owner.Username, Owner: true, Read: true, Write: true},
						{Username: member.Username, Read: true, Write: true},
					}
				}
				require.NoError(t, taskTraceTeamWriteManifest(filepath.Join(taskTraceTeamShareDir(repository, shareID), "manifest.json"), &manifest))
				require.NoError(t, taskTraceTeamWriteSnapshot(&binding, snapshot))
				binding.Base = taskTraceTeamCopyBase(manifest.Base)
				binding.LastSnapshotHash = taskTraceTeamSnapshotHash(snapshot)
				require.NoError(t, session.Commit())
				return binding
			}
			old := publish(child.ID, "old-child")
			ownerState := taskTraceTeamState{Schema: taskTraceTeamSchema, DeviceID: "owner-device", Bindings: []TaskTraceTeamBinding{old}}
			require.NoError(t, taskTraceTeamSaveState(ownerState))
			t.Setenv("TASKTRACE_DATA_ROOT", memberData)
			imported := db.NewSession()
			_, err := TaskTraceTeamImport(imported, member, TaskTraceTeamImportRequest{
				ProjectID: memberProject.ID, Link: taskTraceTeamEncodeLink(repository, old.ShareID, old.Secret)})
			require.NoError(t, err)
			require.NoError(t, imported.Commit())
			require.NoError(t, imported.Close())
			memberState, err := taskTraceTeamLoadState()
			require.NoError(t, err)
			require.Len(t, memberState.Bindings, 1)
			memberTaskID := memberState.Bindings[0].RootTaskID
			t.Setenv("TASKTRACE_DATA_ROOT", ownerData)
			expanded := publish(parent.ID, "new-parent")
			ownerState.Bindings = append(ownerState.Bindings, expanded)
			if parentFirst {
				ownerState.Bindings[0], ownerState.Bindings[1] = ownerState.Bindings[1], ownerState.Bindings[0]
			}
			require.NoError(t, taskTraceTeamSaveState(ownerState))

			sync := func(data string, actor *user.User) {
				t.Helper()
				t.Setenv("TASKTRACE_DATA_ROOT", data)
				session := db.NewSession()
				defer session.Close()
				result, syncErr := TaskTraceTeamSync(session, actor)
				require.NoError(t, syncErr)
				for _, binding := range result.Bindings {
					require.Empty(t, binding.LastError)
					require.Empty(t, binding.Conflicts)
				}
				require.NoError(t, session.Commit())
			}
			edit := func(data string, actor *user.User, taskID int64) {
				t.Helper()
				t.Setenv("TASKTRACE_DATA_ROOT", data)
				session := db.NewSession()
				defer session.Close()
				comment := &TaskComment{TaskID: taskID, Comment: "<h3>每日进展 · 2026-10-14</h3><p>Progress from " + actor.Username + "</p>"} //nolint:gosmopolitan
				allowed, err := comment.CanCreate(session, actor)
				require.NoError(t, err)
				require.True(t, allowed)
				require.NoError(t, comment.Create(session, actor))
				list, err := taskTraceReadOutstanding(session, taskID)
				require.NoError(t, err)
				items, order := taskTraceTeamOutstandingItems(list.original)
				items[actor.Username] = "Outstanding from " + actor.Username
				body := taskTraceTeamOutstandingHTML(items, order)
				if list.comment == nil {
					require.NoError(t, (&TaskComment{TaskID: taskID, Comment: body}).Create(session, actor))
				} else {
					list.comment.Comment = body
					require.NoError(t, list.comment.Update(session, actor))
				}
				require.NoError(t, session.Commit())
			}
			for range 2 {
				sync(ownerData, owner)
				sync(memberData, member)
			}
			edit(memberData, member, memberTaskID)
			for range 3 {
				sync(memberData, member)
				sync(ownerData, owner)
			}
			edit(ownerData, owner, child.ID)
			for range 3 {
				sync(ownerData, owner)
				sync(memberData, member)
			}
			check := db.NewReadSession()
			defer check.Close()
			for _, taskID := range []int64{child.ID, memberTaskID} {
				var comments []*TaskComment
				require.NoError(t, check.Where("task_id = ?", taskID).Find(&comments))
				for _, actor := range []*user.User{owner, member} {
					count := 0
					for _, comment := range comments {
						if strings.Contains(comment.Comment, "Progress from "+actor.Username) {
							count++
						}
					}
					require.Equal(t, 1, count, "daily progress must be present exactly once for %s on task %d", actor.Username, taskID)
					list, err := taskTraceReadOutstanding(check, taskID)
					require.NoError(t, err)
					require.Contains(t, list.original, "Outstanding from "+actor.Username)
				}
			}
			t.Setenv("TASKTRACE_DATA_ROOT", memberData)
			memberState, err = taskTraceTeamLoadState()
			require.NoError(t, err)
			require.Len(t, memberState.Bindings, 1)
			require.Len(t, memberState.Bindings[0].NodeTasks, 1, "the old child link must not reveal the new parent scope")
			var memberTasks []*Task
			require.NoError(t, check.Where("project_id = ?", memberProject.ID).Find(&memberTasks))
			require.Len(t, memberTasks, 1)
		})
	}
}
