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

func TestTaskTraceTeamCombinesComputersWithSameUsername(t *testing.T) {
	now := time.Now().UTC()
	older := TaskTraceTeamSnapshot{Actor: "alice", DeviceID: "one", Updated: now, Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "old", Updated: now, Comments: []TaskTraceTeamComment{{ID: "first", Body: "one", Created: now}}}}}
	newer := TaskTraceTeamSnapshot{Actor: "Alice", DeviceID: "two", Updated: now.Add(time.Minute), Tasks: []TaskTraceTeamTask{{NodeID: "node", Title: "new", Updated: now.Add(time.Minute), Comments: []TaskTraceTeamComment{{ID: "second", Body: "two", Created: now.Add(time.Minute)}}}}}
	combined := taskTraceTeamLatestActorSnapshots([]TaskTraceTeamSnapshot{older, newer})
	require.Len(t, combined, 1)
	require.Equal(t, "new", combined[0].Tasks[0].Title)
	require.Len(t, combined[0].Tasks[0].Comments, 2)
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
