// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"strings"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

//nolint:gosmopolitan // Persisted Chinese documents are the report input.
func TestTaskTracePeriodReview(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	task := &Task{ProjectID: 1, Title: "Review target"}
	require.NoError(t, task.Create(s, actor))
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.FixedZone("local", 8*3600))
	until := from.AddDate(0, 0, 7)
	add := func(body string, when time.Time) {
		t.Helper()
		_, err := s.NoAutoTime().Insert(&TaskComment{TaskID: task.ID, AuthorID: 1, Comment: body, Created: when, Updated: when})
		require.NoError(t, err)
	}
	add(taskTraceTeamAddMarker("<h3>每日进展 · 2026-10-01</h3><p>superseded draft</p>", "p1", "alice"), from.Add(time.Hour))
	add(taskTraceTeamAddMarker("<h3>每日进展 · 2026-10-01</h3><p>final progress</p>", "p2", "alice"), from.Add(2*time.Hour))
	add(taskTraceTeamAddMarker("<h3>每日进展 · 2026-10-01</h3><p>other member progress</p>", "p3", "bob"), from.Add(3*time.Hour))
	add("<h3>每日进展 · 2026-10-17</h3><p>exclusive end</p>", until)
	add("<h3>每日进展 · 2026-10-09</h3><p>before range</p>", from.Add(-time.Second))
	old := taskTraceOutstandingItem{id: "item", content: `<p>New issue</p><aside data-tasktrace-outstanding-note="true" hidden><p>old note</p></aside>`}
	updated := old
	updated.content = strings.ReplaceAll(old.content, "old note", "new note")
	add(`<p data-tasktrace-comment-type="item-activity" data-item-id="item"><strong>新增遗留事项</strong>：New issue</p>`+taskTraceReviewItemSnapshot(taskTraceOutstandingItem{}, false, old), from)
	add(`<p data-tasktrace-comment-type="item-activity" data-item-id="item"><strong>修改遗留事项</strong>：New issue</p>`+taskTraceReviewItemSnapshot(old, true, updated), until.Add(-time.Second))
	add(`<p data-tasktrace-comment-type="item-activity"><strong>完成遗留事项</strong>：irrelevant completion</p>`, from.Add(time.Hour))
	report := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: from, Until: until}
	can, _, err := report.CanRead(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, report.ReadOne(s, actor))
	require.Equal(t, 1, report.TaskCount)
	require.Equal(t, 4, report.ChangeCount)
	for _, text := range []string{"final progress", "other member progress", "New issue", "old note", "new note", "修改前", "修改后", "2026-10-16 23:59"} {
		require.Contains(t, report.Markdown, text)
	}
	for _, text := range []string{"superseded draft", "exclusive end", "before range", "irrelevant completion", "data-tasktrace"} {
		require.NotContains(t, report.Markdown, text)
	}
	require.Empty(t, report.Warnings)
	empty := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: until, Until: until.Add(time.Second)}
	require.NoError(t, empty.ReadOne(s, actor))
	if empty.ChangeCount != 1 {
		rows, queryErr := s.QueryString("SELECT comment,created,updated FROM task_comments WHERE task_id = ?", task.ID)
		t.Logf("range=%s to=%s rows=%+v err=%v report=%s", until, until.Add(time.Second), rows, queryErr, empty.Markdown)
	}
	require.Equal(t, 1, empty.ChangeCount, "exclusive range boundary is included in next report")
	for _, sample := range []struct {
		ids     []int64
		who     *user.User
		allowed bool
	}{{[]int64{1}, actor, true}, {[]int64{2}, actor, false}, {[]int64{1}, &user.User{ID: 999}, false}} {
		request := &TaskTracePeriodReview{ProjectIDs: sample.ids, From: from, Until: until}
		allowed, _, err := request.CanRead(s, sample.who)
		require.NoError(t, err)
		require.Equal(t, sample.allowed, allowed)
	}
	invalid := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: until, Until: from}
	_, _, err = invalid.CanRead(s, actor)
	require.Error(t, err)
	legacy := `<p data-tasktrace-comment-type="item-activity"><strong>修改遗留事项</strong>：legacy item</p>`
	add(legacy, from.Add(4*time.Hour))
	report = &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: from, Until: until}
	require.NoError(t, report.ReadOne(s, actor))
	require.NotEmpty(t, report.Warnings)
	require.Contains(t, report.Markdown, "旧记录")
}
