// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceMarkdownExport(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	root := &Task{ProjectID: 1, Title: "Export : task", Description: "<p>Description<br>second line</p>", PercentDone: 0.7}
	require.NoError(t, root.Create(s, actor))
	child := &Task{ProjectID: 1, Title: "Readable child"}
	require.NoError(t, child.Create(s, actor))
	require.NoError(t, (&TaskRelation{TaskID: root.ID, OtherTaskID: child.ID, RelationKind: RelationKindSubtask}).Create(s, actor))
	// A malformed legacy cross-project edge must not disclose a private descendant.
	foreign := &Task{ProjectID: 2, Title: "Private child secret"}
	require.NoError(t, foreign.Create(s, &user.User{ID: 2}))
	_, err := s.Insert(&TaskRelation{TaskID: root.ID, OtherTaskID: foreign.ID, RelationKind: RelationKindSubtask})
	require.NoError(t, err)
	_, err = s.Insert(&TaskRelation{TaskID: child.ID, OtherTaskID: root.ID, RelationKind: RelationKindSubtask})
	require.NoError(t, err)
	var picture bytes.Buffer
	bitmap := image.NewRGBA(image.Rect(0, 0, 3, 2))
	bitmap.Set(0, 0, color.RGBA{R: 200, A: 255})
	require.NoError(t, png.Encode(&picture, bitmap))
	attachment := &TaskAttachment{TaskID: root.ID}
	require.NoError(t, attachment.NewAttachment(s, bytes.NewReader(picture.Bytes()), "image.png", uint64(picture.Len()), actor))
	body := fmt.Sprintf(`<h3>每日进展 · 2026-10-14</h3><p>before</p><img src="/api/v2/tasks/%d/attachments/%d"><p>after</p>`, root.ID, attachment.ID)                                                                                                                                                                                                     //nolint:gosmopolitan
	for _, note := range []string{taskTraceTeamAddMarker("<h3>每日进展 · 2026-10-14</h3><p>old revision</p>", "old", "user2"), taskTraceTeamAddMarker(body, "new", "user2"), `<h3>TaskTrace 遗留事项清单</h3><ul><li data-id="one" data-done="true"><p>Outstanding title</p><aside data-tasktrace-outstanding-note="one"><p>Note body</p></aside></li></ul>`} { //nolint:gosmopolitan
		require.NoError(t, (&TaskComment{TaskID: root.ID, Comment: note}).Create(s, actor))
	}
	export := &TaskTraceMarkdownExport{TaskID: root.ID}
	can, _, err := export.CanRead(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, export.ReadOne(s, actor))
	require.Equal(t, "Export _ task.md", export.Filename)
	for _, text := range []string{"Description", "Readable child", "Outstanding title", "Note body", "- [x]", "user2", "before", "after", "data:image/png;base64,"} {
		require.Contains(t, export.Markdown, text)
	}
	require.NotContains(t, export.Markdown, "old revision")
	require.NotContains(t, export.Markdown, "Private child secret")
	require.NotContains(t, export.Markdown, "tasktrace-team:")
	require.Empty(t, export.Warnings)
	// Export performs no changes and never includes ancestors when exporting a leaf.
	_, err = s.Where("task_id = ? AND other_task_id = ?", child.ID, root.ID).Delete(&TaskRelation{})
	require.NoError(t, err)
	leaf := &TaskTraceMarkdownExport{TaskID: child.ID}
	require.NoError(t, leaf.ReadOne(s, actor))
	require.NotContains(t, leaf.Markdown, "Description")
	forbidden := &TaskTraceMarkdownExport{TaskID: foreign.ID}
	can, _, err = forbidden.CanRead(s, actor)
	require.NoError(t, err)
	require.False(t, can)
	missing := &TaskTraceMarkdownExport{TaskID: 999999}
	_, _, err = missing.CanRead(s, actor)
	require.Error(t, err)
	require.Equal(t, "_CON.md", taskTraceMarkdownFilename("CON"))
}
