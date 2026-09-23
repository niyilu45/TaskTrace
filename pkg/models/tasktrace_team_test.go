// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"testing"
	"time"

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

func TestTaskTraceTeamCombinesComputersWithSameUsername(t *testing.T) {
	now := time.Now().UTC()
	older := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "one", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "old", Updated: now, Comments: []TaskTraceTeamComment{{ID: "first", Body: "one", Created: now}}}}}
	newer := TaskTraceTeamSnapshot{Actor: "Alice", DeviceID: "two", Updated: now.Add(time.Minute), Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "new", Updated: now.Add(time.Minute), Comments: []TaskTraceTeamComment{{ID: "second", Body: "two", Created: now.Add(time.Minute)}}}}}
	combined := taskTraceTeamLatestActorSnapshots([]TaskTraceTeamSnapshot{older, newer})
	require.Len(t, combined, 1)
	require.Equal(t, "new", combined[0].Tasks[0].Title)
	require.Len(t, combined[0].Tasks[0].Comments, 2)
}

func TestTaskTraceTeamCombinesSameAttachmentContentAcrossComputers(t *testing.T) {
	now := time.Now().UTC()
	one := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "one", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Updated: now, Attachments: []TaskTraceTeamAttachment{{ID: "same-content", SourceTaskID: 1, SourceAttachmentID: 10}}}}}
	two := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "two", Updated: now.Add(time.Second), Tasks: []TaskTraceTeamTask{{NodeID: "node", Updated: now.Add(time.Second), Attachments: []TaskTraceTeamAttachment{{ID: "same-content", SourceTaskID: 9, SourceAttachmentID: 99}}}}}
	combined := taskTraceTeamLatestActorSnapshots([]TaskTraceTeamSnapshot{one, two})
	require.Len(t, combined, 1)
	require.Len(t, combined[0].Tasks, 1)
	require.Len(t, combined[0].Tasks[0].Attachments, 1)
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

func TestTaskTraceTeamOutstandingPriorityStaysLocal(t *testing.T) {
	local := `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-done="false" data-priority="2">本机内容</li><li data-id="two" data-priority="7">第二条</li></ul>`
	merged := `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one">协作者修改</li><li data-id="three">协作者新增</li></ul>`

	priorities := taskTraceTeamOutstandingPriorities(local)
	rebuilt := taskTraceTeamApplyOutstandingPriorities(merged, priorities)

	require.Contains(t, rebuilt, `<li data-id="one" data-priority="2">协作者修改</li>`)
	require.NotContains(t, rebuilt, `data-id="three" data-priority=`)
	require.Equal(t, map[string]string{"one": "2", "two": "7"}, priorities)
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
