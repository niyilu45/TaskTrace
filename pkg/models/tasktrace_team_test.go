// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamLinkRoundTrip(t *testing.T) {
	link := taskTraceTeamEncodeLinkPaths(`\\HOST\TaskTraceTeam`, []string{`\\10.143.58.8\TaskTraceTeam`}, "share-id", "secret")
	parsed, err := taskTraceTeamDecodeLink(link)
	require.NoError(t, err)
	require.Equal(t, `\\HOST\TaskTraceTeam`, parsed.Repository)
	require.Equal(t, []string{`\\10.143.58.8\TaskTraceTeam`}, parsed.Repositories)
	require.Equal(t, "share-id", parsed.ShareID)
	require.Equal(t, "secret", parsed.Secret)
	_, err = taskTraceTeamDecodeLink("https://example.test/task")
	require.Error(t, err)
}

func TestTaskTraceTeamRepositoryCandidatesPreferOverrideAndAcceptServerRoot(t *testing.T) {
	link := taskTraceTeamLink{Repository: `\\TASK-PC\teamData`, Repositories: []string{`\\10.0.0.8\teamData`, `\\TASK-PC\teamData`}}
	require.Equal(t, []string{
		`\\10.143.58.8\teamData`,
		`\\10.143.58.8`,
		`\\TASK-PC\teamData`,
		`\\10.0.0.8\teamData`,
	}, taskTraceTeamRepositoryCandidates(link, `\\10.143.58.8`))
}

func TestTaskTraceTeamMarkerKeepsRemoteAuthor(t *testing.T) {
	body := taskTraceTeamAddMarker("<p>progress</p>", "comment-id", "alice")
	marker, ok := taskTraceTeamReadMarker(body)
	require.True(t, ok)
	require.Equal(t, taskTraceTeamMarkerValue{ID: "comment-id", Author: "alice"}, marker)
	require.Equal(t, body, taskTraceTeamAddMarker(body, "other", "bob"))
}

func TestTaskTraceTeamFieldMergeAndConflict(t *testing.T) {
	base := TaskTraceTeamBase{Title: "before"}
	one := TaskTraceTeamSnapshot{Actor: "alice", Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "after"}}}
	two := TaskTraceTeamSnapshot{Actor: "bob", Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "before"}}}
	value, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{one, two}, "node", "title", taskTraceTeamBaseValue(base, "title"), "")
	require.False(t, conflict)
	require.Empty(t, options)
	require.Equal(t, "after", value)

	two.Tasks[0].Title = "different"
	value, options, conflict = taskTraceTeamFindField([]TaskTraceTeamSnapshot{one, two}, "node", "title", "before", "")
	require.True(t, conflict)
	require.Equal(t, "before", value)
	require.Len(t, options, 2)
}

func TestTaskTraceTeamFieldMergeTreatsEquivalentContentAsEqual(t *testing.T) {
	base := `<p class="note" data-kind="text">同一内容 <strong>加粗</strong><img src="/api/v1/tasks/10/attachments/20"></p>`
	alice := TaskTraceTeamSnapshot{Actor: "alice", Tasks: []TaskTraceTeamTask{{
		NodeID:      "node",
		Title:       " 相同标题 ",
		Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-priority="2"><p data-kind="text" class="note">同一内容   <strong>加粗</strong><img src="/api/v1/tasks/11/attachments/21"></p></li></ul>`,
		Attachments: []TaskTraceTeamAttachment{{ID: "same-image", SourceTaskID: 11, SourceAttachmentID: 21}},
	}}}
	bob := TaskTraceTeamSnapshot{Actor: "bob", Tasks: []TaskTraceTeamTask{{
		NodeID:      "node",
		Title:       "相同标题",
		Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-priority="8"><p class="note" data-kind="text">同一内容 <strong>加粗</strong><img src="/api/v2/tasks/12/attachments/22"></p></li></ul>`,
		Attachments: []TaskTraceTeamAttachment{{ID: "same-image", SourceTaskID: 12, SourceAttachmentID: 22}},
	}}}
	baseSnapshot := TaskTraceTeamSnapshot{Actor: "local", Tasks: []TaskTraceTeamTask{{
		NodeID:      "node",
		Title:       "相同标题",
		Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one">` + base + `</li></ul>`,
		Attachments: []TaskTraceTeamAttachment{{ID: "same-image", SourceTaskID: 10, SourceAttachmentID: 20}},
	}}}

	value, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{alice, bob, baseSnapshot}, "node", "title", "相同标题", "", baseSnapshot.Tasks[0])
	require.False(t, conflict)
	require.Empty(t, options)
	require.Equal(t, "相同标题", value)

	value, options, conflict = taskTraceTeamFindField([]TaskTraceTeamSnapshot{alice, bob, baseSnapshot}, "node", "outstanding:one", base, "", baseSnapshot.Tasks[0])
	require.False(t, conflict)
	require.Empty(t, options)
	require.Equal(t, base, value)
}

func TestTaskTraceTeamFieldMergeStillConflictsForDifferentImages(t *testing.T) {
	base := `<img src="/api/v1/tasks/10/attachments/20">`
	one := TaskTraceTeamSnapshot{Actor: "alice", Tasks: []TaskTraceTeamTask{{NodeID: "node", Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one"><img src="/api/v1/tasks/11/attachments/21"></li></ul>`, Attachments: []TaskTraceTeamAttachment{{ID: "image-a", SourceTaskID: 11, SourceAttachmentID: 21}}}}}
	two := TaskTraceTeamSnapshot{Actor: "bob", Tasks: []TaskTraceTeamTask{{NodeID: "node", Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one"><img src="/api/v1/tasks/12/attachments/22"></li></ul>`, Attachments: []TaskTraceTeamAttachment{{ID: "image-b", SourceTaskID: 12, SourceAttachmentID: 22}}}}}
	baseSnapshot := TaskTraceTeamSnapshot{Actor: "local", Tasks: []TaskTraceTeamTask{{NodeID: "node", Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one">` + base + `</li></ul>`, Attachments: []TaskTraceTeamAttachment{{ID: "base-image", SourceTaskID: 10, SourceAttachmentID: 20}}}}}

	_, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{one, two, baseSnapshot}, "node", "outstanding:one", base, "", baseSnapshot.Tasks[0])
	require.True(t, conflict)
	require.Len(t, options, 2)
}

func TestTaskTraceTeamDescriptionUsesThreeWayConflictDetection(t *testing.T) {
	base := TaskTraceTeamTask{NodeID: "node", Description: `<p>original<img src="/api/v1/tasks/10/attachments/20"></p>`, Attachments: []TaskTraceTeamAttachment{{ID: "same-image", SourceTaskID: 10, SourceAttachmentID: 20}}}
	alice := TaskTraceTeamSnapshot{Actor: "alice", Tasks: []TaskTraceTeamTask{{NodeID: "node", Description: `<p>Alice edit</p>`}}}
	bob := TaskTraceTeamSnapshot{Actor: "bob", Tasks: []TaskTraceTeamTask{{NodeID: "node", Description: `<p>Bob edit</p>`}}}

	_, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{alice, bob}, "node", "description", base.Description, "", base)
	require.True(t, conflict)
	require.Len(t, options, 2)

	alice.Tasks[0] = TaskTraceTeamTask{NodeID: "node", Description: `<p>same content<img src="/api/v1/tasks/11/attachments/21"></p>`, Attachments: []TaskTraceTeamAttachment{{ID: "same-image", SourceTaskID: 11, SourceAttachmentID: 21}}}
	bob.Tasks[0] = TaskTraceTeamTask{NodeID: "node", Description: `<p>same content<img src="/api/v2/tasks/12/attachments/22"></p>`, Attachments: []TaskTraceTeamAttachment{{ID: "same-image", SourceTaskID: 12, SourceAttachmentID: 22}}}
	value, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{alice, bob}, "node", "description", base.Description, "", base)
	require.False(t, conflict)
	require.Empty(t, options)
	require.Contains(t, value, "same content")
}

func TestTaskTraceTeamCombinesComputersWithSameUsername(t *testing.T) {
	now := time.Now().UTC()
	older := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "one", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "old", Updated: now, Comments: []TaskTraceTeamComment{{ID: "first", Body: "one", Created: now}}}}}
	newer := TaskTraceTeamSnapshot{Actor: "Alice", DeviceID: "two", Updated: now.Add(time.Minute), Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "new", Updated: now.Add(time.Minute), Comments: []TaskTraceTeamComment{{ID: "second", Body: "two", Created: now.Add(time.Minute)}}}}}
	combined := taskTraceTeamLatestActorSnapshots([]TaskTraceTeamSnapshot{older, newer})
	require.Len(t, combined, 1)
	require.Equal(t, "new", combined[0].Tasks[0].Title)
	require.Len(t, combined[0].Tasks[0].Comments, 2)
}

func TestTaskTraceTeamSameSecondMetadataTieIsDeterministic(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	one := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "a-device", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "first", Updated: now}}}
	two := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "b-device", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "second", Updated: now}}}

	for _, snapshots := range [][]TaskTraceTeamSnapshot{{one, two}, {two, one}} {
		combined := taskTraceTeamLatestActorSnapshots(snapshots)
		require.Len(t, combined, 1)
		require.Equal(t, "second", combined[0].Tasks[0].Title)
	}
}

func TestTaskTraceTeamCombinesSameAttachmentContentAcrossComputers(t *testing.T) {
	now := time.Now().UTC()
	one := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "one", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Updated: now, Attachments: []TaskTraceTeamAttachment{{ID: "same-content", SourceTaskID: 1, SourceAttachmentID: 10}}}}}
	two := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "two", Updated: now.Add(time.Second), Tasks: []TaskTraceTeamTask{{NodeID: "node", Updated: now.Add(time.Second), Attachments: []TaskTraceTeamAttachment{{ID: "same-content", SourceTaskID: 9, SourceAttachmentID: 99}}}}}
	combined := taskTraceTeamLatestActorSnapshots([]TaskTraceTeamSnapshot{one, two})
	require.Len(t, combined, 1)
	require.Len(t, combined[0].Tasks, 1)
	require.Len(t, combined[0].Tasks[0].Attachments, 2, "each computer's source id is needed to rewrite its HTML to the local attachment")
}

func TestTaskTraceTeamCommentTieIsDeterministic(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	one := TaskTraceTeamComment{ID: "progress", Body: "first", Author: "alice", Created: now, Updated: now}
	two := TaskTraceTeamComment{ID: "progress", Body: "second", Author: "alice", Created: now, Updated: now}
	snapshotOne := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "one", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Comments: []TaskTraceTeamComment{one}}}}
	snapshotTwo := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "two", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Comments: []TaskTraceTeamComment{two}}}}

	require.NotEqual(t, taskTraceTeamCommentEventSupersedes(one, two), taskTraceTeamCommentEventSupersedes(two, one))
	for _, snapshots := range [][]TaskTraceTeamSnapshot{{snapshotOne, snapshotTwo}, {snapshotTwo, snapshotOne}} {
		combined := taskTraceTeamLatestActorSnapshots(snapshots)
		require.Len(t, combined, 1)
		require.Len(t, combined[0].Tasks[0].Comments, 1)
		require.Equal(t, "second", combined[0].Tasks[0].Comments[0].Body)
	}
}

func TestTaskTraceTeamMissingTaskPlanUsesEverySnapshot(t *testing.T) {
	now := time.Now().UTC()
	binding := &TaskTraceTeamBinding{NodeTasks: map[string]int64{"root": 1}}
	snapshots := []TaskTraceTeamSnapshot{
		{Actor: "alice", Tasks: []TaskTraceTeamTask{{NodeID: "root"}, {NodeID: "old-one", ParentNode: "root"}, {NodeID: "old-two", ParentNode: "root"}}},
		{Actor: "bob", Tasks: []TaskTraceTeamTask{{NodeID: "root"}, {NodeID: "new-child", ParentNode: "root", Updated: now}}},
	}
	plan, err := taskTraceTeamMissingTaskPlan(binding, snapshots)
	require.NoError(t, err)
	require.Len(t, plan, 3)
	nodes := []string{plan[0].NodeID, plan[1].NodeID, plan[2].NodeID}
	require.ElementsMatch(t, []string{"old-one", "old-two", "new-child"}, nodes)
}

func TestTaskTraceTeamMissingTaskPlanRejectsInvalidHierarchyBeforeCreation(t *testing.T) {
	binding := &TaskTraceTeamBinding{NodeTasks: map[string]int64{}}
	_, err := taskTraceTeamMissingTaskPlan(binding, []TaskTraceTeamSnapshot{{Tasks: []TaskTraceTeamTask{{NodeID: "one", ParentNode: "two"}, {NodeID: "two", ParentNode: "one"}}}})
	require.ErrorContains(t, err, "cycle")

	_, err = taskTraceTeamMissingTaskPlan(binding, []TaskTraceTeamSnapshot{{Tasks: []TaskTraceTeamTask{{NodeID: "child", ParentNode: "missing"}}}})
	require.ErrorContains(t, err, "missing parent")

	tooDeep := TaskTraceTeamSnapshot{}
	parent := ""
	for i := 1; i <= MaxTaskHierarchyDepth+1; i++ {
		node := strconv.Itoa(i)
		tooDeep.Tasks = append(tooDeep.Tasks, TaskTraceTeamTask{NodeID: node, ParentNode: parent})
		parent = node
	}
	_, err = taskTraceTeamMissingTaskPlan(binding, []TaskTraceTeamSnapshot{tooDeep})
	require.ErrorContains(t, err, "exceeds")
}

func TestTaskTraceTeamPrunesMissingChildMappingWithoutDroppingBase(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	binding := &TaskTraceTeamBinding{
		RootTaskID:       1,
		NodeTasks:        map[string]int64{"root": 1, "missing": 999999},
		Base:             map[string]TaskTraceTeamBase{"missing": {Title: "recover me"}},
		LocalAttachments: map[string]int64{"missing:image": 123},
	}

	require.NoError(t, taskTraceTeamPruneMissingTaskMappings(s, binding))
	require.NotContains(t, binding.NodeTasks, "missing")
	require.NotContains(t, binding.LocalAttachments, "missing:image")
	require.Equal(t, "recover me", binding.Base["missing"].Title, "the merge base is needed to restore remote content without inventing a deletion")
}

func TestTaskTraceTeamSnapshotKeepsMappedTaskMovedOutsideSharedRoot(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	root := &Task{Title: "shared root", ProjectID: 1, Priority: 7}
	detached := &Task{Title: "detached shared task", ProjectID: 1, Priority: 7}
	require.NoError(t, root.Create(s, actor))
	require.NoError(t, detached.Create(s, actor))
	binding := &TaskTraceTeamBinding{
		Repository:     t.TempDir(),
		ShareID:        "share",
		RootTaskID:     root.ID,
		NodeTasks:      map[string]int64{"root": root.ID, "detached": detached.ID},
		ResolutionAcks: map[string]string{},
	}

	snapshot, err := taskTraceTeamBuildSnapshot(s, binding, actor.Username, "device")
	require.NoError(t, err)
	shared := taskTraceTeamTaskMap(snapshot)
	require.Contains(t, shared, "detached")
	require.Empty(t, shared["detached"].ParentNode, "a personal parent outside the shared subtree must remain local")
}

func TestTaskTraceTeamRejectsMissingCollaborativeRoot(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	binding := &TaskTraceTeamBinding{RootTaskID: 999999, NodeTasks: map[string]int64{"root": 999999}}

	require.ErrorContains(t, taskTraceTeamPruneMissingTaskMappings(s, binding), "root task was deleted")
}

func TestTaskTraceTeamRewritesEverySourceForSameAttachmentContent(t *testing.T) {
	rows := []struct {
		actor string
		task  TaskTraceTeamTask
	}{
		{actor: "alice", task: TaskTraceTeamTask{Attachments: []TaskTraceTeamAttachment{{ID: "same-content", SourceTaskID: 11, SourceAttachmentID: 21}}}},
		{actor: "bob", task: TaskTraceTeamTask{Attachments: []TaskTraceTeamAttachment{{ID: "same-content", SourceTaskID: 12, SourceAttachmentID: 22}}}},
	}
	attachments := taskTraceTeamAllAttachments(rows)
	require.Len(t, attachments, 2)
	binding := &TaskTraceTeamBinding{
		NodeTasks:        map[string]int64{"node": 42},
		LocalAttachments: map[string]int64{"node:same-content": 77},
	}
	body := `<p><img src="/api/v1/tasks/11/attachments/21"><img src="/api/v2/tasks/12/attachments/22"></p>`
	rewritten := taskTraceTeamRewriteAttachments(body, 42, attachments, binding)
	require.NotContains(t, rewritten, "tasks/11/attachments/21")
	require.NotContains(t, rewritten, "tasks/12/attachments/22")
	require.Equal(t, 2, strings.Count(rewritten, "tasks/42/attachments/77"))
}

func TestTaskTraceTeamConflictResolutionRewritesRemoteImageSource(t *testing.T) {
	binding := &TaskTraceTeamBinding{
		Repository:       t.TempDir(),
		ShareID:          "share",
		NodeTasks:        map[string]int64{"node": 42},
		LocalAttachments: map[string]int64{"node:same-content": 77},
	}
	snapshot := TaskTraceTeamSnapshot{
		Schema: taskTraceTeamSchema, ShareID: "share", Actor: "alice", DeviceID: "device",
		Tasks: []TaskTraceTeamTask{{NodeID: "node", Attachments: []TaskTraceTeamAttachment{{ID: "same-content", SourceTaskID: 11, SourceAttachmentID: 21}}}},
	}
	require.NoError(t, taskTraceTeamWriteSnapshot(binding, snapshot))

	value, err := taskTraceTeamLocalResolutionValue(binding, "node", `<p><img src="/api/v1/tasks/11/attachments/21"></p>`)
	require.NoError(t, err)
	require.Contains(t, value, "/api/v1/tasks/42/attachments/77")
}

func TestTaskTraceTeamExportKeepsDuplicateContentSourceIDs(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	task := &Task{Title: "attachment export test", ProjectID: 1}
	require.NoError(t, task.Create(s, actor))
	content := []byte("same outstanding image")
	first := &TaskAttachment{TaskID: task.ID}
	second := &TaskAttachment{TaskID: task.ID}
	require.NoError(t, first.NewAttachment(s, bytes.NewReader(content), "first.png", uint64(len(content)), actor))
	require.NoError(t, second.NewAttachment(s, bytes.NewReader(content), "second.png", uint64(len(content)), actor))
	binding := &TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "share", LocalAttachments: map[string]int64{}}
	exported, err := taskTraceTeamExportAttachments(s, binding, task.ID, "node")
	require.NoError(t, err)
	bySource := map[int64]TaskTraceTeamAttachment{}
	for _, attachment := range exported {
		bySource[attachment.SourceAttachmentID] = attachment
	}
	require.Contains(t, bySource, first.ID)
	require.Contains(t, bySource, second.ID)
	require.Equal(t, bySource[first.ID].ID, bySource[second.ID].ID)
}

func TestTaskTraceTeamSnapshotHashIgnoresSyncTimeAndAvatar(t *testing.T) {
	base := TaskTraceTeamSnapshot{
		Schema:   taskTraceTeamSchema,
		ShareID:  "share",
		Actor:    "alice",
		DeviceID: "device",
		Updated:  time.Now().UTC(),
		Avatar:   "data:image/png;base64,one",
		Tasks:    []TaskTraceTeamTask{{NodeID: "node", Title: "work"}},
	}
	changedPresentation := base
	changedPresentation.Updated = base.Updated.Add(time.Hour)
	changedPresentation.Avatar = "data:image/png;base64,two"
	require.Equal(t, taskTraceTeamSnapshotHash(base), taskTraceTeamSnapshotHash(changedPresentation))

	changedTask := base
	changedTask.Tasks = []TaskTraceTeamTask{{NodeID: "node", Title: "changed"}}
	require.NotEqual(t, taskTraceTeamSnapshotHash(base), taskTraceTeamSnapshotHash(changedTask))
}

func TestTaskTraceTeamProgressNotificationIsCreatedOncePerSavedContent(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	previous := TaskTraceTeamSnapshot{
		Actor: "alice",
		Tasks: []TaskTraceTeamTask{{
			NodeID: "node",
			Title:  "具体事项",
			Comments: []TaskTraceTeamComment{{
				ID:      "progress",
				Body:    taskTraceTeamAddMarker("<p>原进展</p>", "progress", "alice"),
				Author:  "alice",
				Created: now,
				Updated: now,
			}},
		}},
	}
	current := previous
	current.Tasks = append([]TaskTraceTeamTask(nil), previous.Tasks...)
	current.Tasks[0].Comments = append([]TaskTraceTeamComment(nil), previous.Tasks[0].Comments...)
	current.Tasks[0].Comments[0].Body = taskTraceTeamAddMarker("<p>更正后的进展</p>", "progress", "alice")
	current.Tasks[0].Comments[0].Updated = now.Add(time.Minute)

	change := taskTraceTeamLatestProgressChange(&previous, current, "Alice")
	require.NotNil(t, change)
	require.Equal(t, "具体事项", change.TaskTitle)
	require.Equal(t, "progress", change.Comment.ID)
	require.Equal(
		t,
		taskTraceTeamProgressNotificationID("share", "alice", change),
		taskTraceTeamProgressNotificationID("share", "Alice", change),
	)

	unchanged := current
	unchanged.Tasks = append([]TaskTraceTeamTask(nil), current.Tasks...)
	unchanged.Tasks[0].Comments = append([]TaskTraceTeamComment(nil), current.Tasks[0].Comments...)
	unchanged.Tasks[0].Comments[0].Body = taskTraceTeamAddMarker("<p>更正后的进展</p>", "progress", "alice")
	unchanged.Tasks[0].Comments[0].Updated = now.Add(2 * time.Minute)
	require.Nil(t, taskTraceTeamLatestProgressChange(&current, unchanged, "alice"), "同步标记或时间变化不能重复通知")
}

func TestTaskTraceTeamProgressNotificationIgnoresOtherAuthors(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	previous := TaskTraceTeamSnapshot{Actor: "alice", Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "事项"}}}
	current := previous
	current.Tasks = []TaskTraceTeamTask{{
		NodeID: "node",
		Title:  "事项",
		Comments: []TaskTraceTeamComment{{
			ID: "bob-progress", Body: "<p>Bob 的进展</p>", Author: "bob", Created: now, Updated: now,
		}},
	}}
	require.Nil(t, taskTraceTeamLatestProgressChange(&previous, current, "alice"), "同步到本机的他人进展不能再由本机重复通知")
}

func TestTaskTraceTeamSafeAvatarRejectsActiveContent(t *testing.T) {
	require.Equal(t, "data:image/png;base64,AAAA", taskTraceTeamSafeAvatar("data:image/png;base64,AAAA"))
	require.Empty(t, taskTraceTeamSafeAvatar("data:image/svg+xml;base64,AAAA"))
	require.Empty(t, taskTraceTeamSafeAvatar("javascript:alert(1)"))
}

func TestTaskTraceTeamUnmarkedCommentUsesExportedIdentity(t *testing.T) {
	created := time.Now().UTC().Truncate(time.Second)
	comment := &TaskComment{ID: 42, Comment: "<p>one progress</p>", Created: created}
	id := taskTraceTeamCommentID("share", "node", comment, "alice")
	shared := taskTraceTeamAddMarker(comment.Comment, id, "alice")
	marker, ok := taskTraceTeamReadMarker(shared)
	require.True(t, ok)
	require.Equal(t, id, marker.ID)
	require.Equal(t, id, taskTraceTeamCommentID("share", "node", comment, "alice"))
	indexed, duplicates := taskTraceTeamLocalCommentsByID(&TaskTraceTeamBinding{ShareID: "share"}, "node", "alice", []*TaskComment{comment})
	require.Same(t, comment, indexed[id])
	require.Empty(t, duplicates)

	markedCopy := &TaskComment{ID: 43, Comment: shared, Created: created, Updated: comment.Updated}
	indexed, duplicates = taskTraceTeamLocalCommentsByID(&TaskTraceTeamBinding{ShareID: "share"}, "node", "alice", []*TaskComment{comment, markedCopy})
	require.Same(t, markedCopy, indexed[id])
	require.Equal(t, []*TaskComment{comment}, duplicates)
}

func TestTaskTraceTeamCommentDeletionEventsSurviveRefreshAndAllowRestore(t *testing.T) {
	created := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	deletedAt := created.Add(time.Minute)
	previous := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{NodeID: "node", Comments: []TaskTraceTeamComment{{ID: "comment", Body: "<p>progress</p>", Author: "alice", Created: created, Updated: created}}}}}
	current := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{NodeID: "node"}}}

	taskTraceTeamReconcileLocalCommentEvents(&current, &previous, deletedAt)
	require.Len(t, current.Tasks[0].Comments, 1)
	tombstone := current.Tasks[0].Comments[0]
	require.True(t, tombstone.Deleted)
	require.Empty(t, tombstone.Body)
	require.Equal(t, deletedAt, tombstone.Updated)

	next := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{NodeID: "node"}}}
	taskTraceTeamReconcileLocalCommentEvents(&next, &current, deletedAt.Add(time.Minute))
	require.Equal(t, tombstone, next.Tasks[0].Comments[0], "refresh must retain the original tombstone instead of recreating the comment")

	restored := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{NodeID: "node", Comments: []TaskTraceTeamComment{{ID: "comment", Body: "<p>progress</p>", Author: "alice", Created: created, Updated: created}}}}}
	restoredAt := deletedAt.Add(2 * time.Minute)
	taskTraceTeamReconcileLocalCommentEvents(&restored, &next, restoredAt)
	require.False(t, restored.Tasks[0].Comments[0].Deleted)
	require.Equal(t, restoredAt, restored.Tasks[0].Comments[0].Updated, "an Undo restore must supersede the deletion event")

	latest := taskTraceTeamLatestCommentEventSnapshot([]TaskTraceTeamSnapshot{previous, current, restored})
	require.Len(t, latest.Tasks, 1)
	require.False(t, latest.Tasks[0].Comments[0].Deleted)
	require.Equal(t, restoredAt, latest.Tasks[0].Comments[0].Updated)
}

func TestTaskTraceTeamCommentDeletionWinsTimestampTie(t *testing.T) {
	updated := time.Date(2026, 9, 24, 8, 0, 0, 0, time.UTC)
	active := TaskTraceTeamSnapshot{Actor: "member", Tasks: []TaskTraceTeamTask{{NodeID: "node", Comments: []TaskTraceTeamComment{{ID: "progress", Body: "<p>member progress</p>", Author: "member", Updated: updated}}}}}
	deleted := TaskTraceTeamSnapshot{Actor: "owner", Tasks: []TaskTraceTeamTask{{NodeID: "node", Comments: []TaskTraceTeamComment{{ID: "progress", Author: "member", Updated: updated, Deleted: true}}}}}

	for _, snapshots := range [][]TaskTraceTeamSnapshot{{active, deleted}, {deleted, active}} {
		latest := taskTraceTeamLatestCommentEventSnapshot(snapshots)
		require.Len(t, latest.Tasks, 1)
		require.Len(t, latest.Tasks[0].Comments, 1)
		require.True(t, latest.Tasks[0].Comments[0].Deleted, "the owner's deletion must not depend on snapshot read order")
	}
}

func TestTaskTraceTeamMergeCommentDeletionDoesNotRecreateOnRefresh(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	created := time.Now().UTC().Add(-time.Minute)
	comment := &TaskComment{TaskID: 1, AuthorID: 2, Comment: taskTraceTeamAddMarker("<p>collaborator progress</p>", "shared-comment", "user2"), Created: created, Updated: created}
	_, err := s.NoAutoTime().Insert(comment)
	require.NoError(t, err)

	binding := &TaskTraceTeamBinding{ShareID: "share"}
	remote := []TaskTraceTeamComment{{ID: "shared-comment", Author: "user2", Created: created, Updated: created.Add(time.Minute), Deleted: true}}
	require.NoError(t, taskTraceTeamMergeComments(s, &user.User{ID: 1, Username: "user1"}, binding, "node", "user1", 1, remote))

	var stored TaskComment
	exists, err := s.ID(comment.ID).Get(&stored)
	require.NoError(t, err)
	require.False(t, exists)
}

func TestTaskTraceTeamOutstandingItemsMergeIndependently(t *testing.T) {
	baseHTML := `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one">原内容</li><li data-id="two">不变</li></ul>`
	base := TaskTraceTeamBase{Outstanding: baseHTML}
	one := TaskTraceTeamSnapshot{Actor: "alice", Tasks: []TaskTraceTeamTask{{NodeID: "node", Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one">Alice 修改</li><li data-id="two">不变</li></ul>`}}}
	two := TaskTraceTeamSnapshot{Actor: "bob", Tasks: []TaskTraceTeamTask{{NodeID: "node", Outstanding: `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one">Bob 修改</li><li data-id="two">不变</li><li data-id="three">Bob 新增</li></ul>`}}}

	_, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{one, two}, "node", "outstanding:one", taskTraceTeamBaseValue(base, "outstanding:one"), "")
	require.True(t, conflict)
	require.Len(t, options, 2)
	value, options, conflict := taskTraceTeamFindField([]TaskTraceTeamSnapshot{one, two}, "node", "outstanding:three", taskTraceTeamBaseValue(base, "outstanding:three"), "")
	require.False(t, conflict)
	require.Empty(t, options)
	require.Equal(t, "Bob 新增", value)
}

func TestTaskTraceTeamEmptyOutstandingHasNoSyntheticComment(t *testing.T) {
	require.Empty(t, taskTraceTeamOutstandingHTML(map[string]string{}, nil))
}

func TestTaskTraceTeamOutstandingUpsertCleansDuplicatesAndEmptyLists(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	body := `<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="one">内容</li></ul>`
	older := &TaskComment{TaskID: 1, AuthorID: 1, Comment: body, Created: time.Now().UTC().Add(-time.Minute), Updated: time.Now().UTC().Add(-time.Minute)}
	newer := &TaskComment{TaskID: 1, AuthorID: 1, Comment: body, Created: time.Now().UTC(), Updated: time.Now().UTC()}
	_, err := s.NoAutoTime().Insert(older, newer)
	require.NoError(t, err)

	require.NoError(t, taskTraceTeamUpsertOutstanding(s, &user.User{ID: 1, Username: "user1"}, 1, body))
	var comments []*TaskComment
	require.NoError(t, s.Where("task_id = ?", 1).Find(&comments))
	count := 0
	for _, comment := range comments {
		if taskTraceTeamIsOutstanding(comment.Comment) {
			count++
		}
	}
	require.Equal(t, 1, count)

	require.NoError(t, taskTraceTeamUpsertOutstanding(s, &user.User{ID: 1, Username: "user1"}, 1, ""))
	comments = nil
	require.NoError(t, s.Where("task_id = ?", 1).Find(&comments))
	for _, comment := range comments {
		require.False(t, taskTraceTeamIsOutstanding(comment.Comment))
	}
}

func TestTaskTraceTeamOutstandingResolutionKeepsOtherItems(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	body := `<h3 data-tasktrace-comment-type="outstanding">TaskTrace outstanding list</h3><ul><li data-id="one">original</li><li data-id="two">must remain</li></ul>`
	require.NoError(t, taskTraceTeamUpsertOutstanding(s, actor, 1, body))
	binding := &TaskTraceTeamBinding{NodeTasks: map[string]int64{"node": 1}, Base: map[string]TaskTraceTeamBase{"node": {Outstanding: body}}}

	err := taskTraceTeamApplyResolution(s, actor, binding, taskTraceTeamResolutionRecord{NodeID: "node", Field: "outstanding:one", Value: "updated"})
	require.NoError(t, err)

	var comments []*TaskComment
	require.NoError(t, s.Where("task_id = ?", 1).Find(&comments))
	for _, comment := range comments {
		if !taskTraceTeamIsOutstanding(comment.Comment) {
			continue
		}
		items, _ := taskTraceTeamOutstandingItems(comment.Comment)
		require.Equal(t, "updated", items["one"])
		require.Equal(t, "must remain", items["two"])
		return
	}
	t.Fatal("canonical outstanding comment was not saved")
}

func TestTaskTraceTeamOutstandingPriorityStaysLocal(t *testing.T) {
	local := `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-done="false" data-priority="2">本机内容</li><li data-id="two" data-priority="7">第二条</li></ul>`
	merged := `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one">协作者修改</li><li data-id="three">协作者新增</li></ul>`

	priorities := taskTraceTeamOutstandingPriorities(local)
	rebuilt := taskTraceTeamApplyOutstandingPriorities(merged, priorities)

	require.Contains(t, rebuilt, `<li data-id="one" data-priority="2">协作者修改</li>`)
	require.NotContains(t, rebuilt, `data-id="three" data-priority=`)
	require.Equal(t, map[string]string{"one": "2", "two": "7"}, priorities)

	binding := &TaskTraceTeamBinding{}
	taskTraceTeamRememberOutstandingPriorities(binding, TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{NodeID: "node", Outstanding: local}}})
	rebuilt = taskTraceTeamApplyOutstandingPriorities(merged, taskTraceTeamRememberedOutstandingPriorities(binding, "node"))
	require.Contains(t, rebuilt, `<li data-id="one" data-priority="2">协作者修改</li>`, "a later sync must not reset the local priority when rebuilt markup omits it")
}

func TestTaskTraceTeamOutstandingRichNoteSurvivesParsingAndPriorityRestore(t *testing.T) {
	local := `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-priority="3"><p>事项正文</p><aside data-tasktrace-outstanding-note="true" hidden><p>备注正文</p><ul><li>备注列表</li></ul><p><img src="/api/v1/tasks/42/attachments/8"></p></aside></li><li data-id="two">第二条</li></ul>`
	items, order := taskTraceTeamOutstandingItems(local)

	require.Equal(t, []string{"one", "two"}, order)
	require.Contains(t, items["one"], "备注正文")
	require.Contains(t, items["one"], "备注列表")
	require.Contains(t, items["one"], "attachments/8")
	require.Equal(t, "第二条", items["two"])

	rebuilt := taskTraceTeamOutstandingHTML(items, order)
	rebuilt = taskTraceTeamApplyOutstandingPriorities(rebuilt, taskTraceTeamOutstandingPriorities(local))
	require.Contains(t, rebuilt, `data-id="one" data-priority="3"`)
	require.Contains(t, rebuilt, `data-tasktrace-outstanding-note="true"`)
	require.Contains(t, rebuilt, "备注列表")
}
