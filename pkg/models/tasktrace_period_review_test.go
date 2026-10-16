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
	add(`<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="item"><p>Latest issue title</p><aside data-tasktrace-outstanding-note="true" hidden><p>latest note after period</p></aside></li><li data-id="legacy">legacy item</li></ul>`, until.Add(time.Hour))
	report := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: from, Until: until}
	can, _, err := report.CanRead(s, actor)
	require.NoError(t, err)
	require.True(t, can)
	require.NoError(t, report.ReadOne(s, actor))
	require.Equal(t, 1, report.TaskCount)
	require.Equal(t, 4, report.ChangeCount)
	for _, text := range []string{"final progress", "other member progress", "1. **【新增】** **【有改动】** Latest issue title", "latest note after period"} {
		require.Contains(t, report.Markdown, text)
	}
	for _, text := range []string{"superseded draft", "exclusive end", "before range", "irrelevant completion", "data-tasktrace", "New issue", "old note", "new note", "修改前", "修改后"} {
		require.NotContains(t, report.Markdown, text)
	}
	require.Empty(t, report.Warnings)
	empty := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: until, Until: until.Add(time.Second)}
	require.NoError(t, empty.ReadOne(s, actor))
	require.Equal(t, 3, empty.ChangeCount, "one progress entry and two unfinished items are included")
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
	require.Empty(t, report.Warnings)
	require.Contains(t, report.Markdown, "2. **【有改动】** legacy item")
	require.NotContains(t, report.Markdown, "旧记录")
}

//nolint:gosmopolitan // Persisted Chinese activity fixtures cover current existence filtering.
func TestTaskTracePeriodReviewDeleted(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	until := from.AddDate(0, 0, 7)
	create := func(title string) *Task {
		task := &Task{ProjectID: 1, Title: title, Description: "<p>Current context</p>"}
		require.NoError(t, task.Create(s, actor))
		return task
	}
	add := func(id int64, body string, when time.Time) {
		_, err := s.NoAutoTime().Insert(&TaskComment{TaskID: id, AuthorID: 1, Comment: body, Created: when, Updated: when})
		require.NoError(t, err)
	}
	kept := create("Existing task")
	add(kept.ID, taskTraceTeamAddMarker(`<p data-tasktrace-comment-type="item-activity" data-item-id="gone"><strong>新增遗留事项</strong>：Same title</p>`, "add-gone", "member"), from)
	add(kept.ID, `<p data-tasktrace-comment-type="item-activity" data-item-id="kept"><strong>新增遗留事项</strong>：Same title</p>`, from)
	add(kept.ID, `<p data-tasktrace-comment-type="item-activity" data-item-id="gone"><strong>修改遗留事项</strong>：deleted note history</p>`, from.Add(time.Hour))
	// No deletion log is needed: canonical state after the review period is authoritative.
	add(kept.ID, `<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul><li data-id="kept" data-done="true" data-completed-at="2026-10-10T01:00:00Z">Same title</li></ul>`, until.Add(time.Hour))
	empty := create("All items removed")
	add(empty.ID, `<p data-tasktrace-comment-type="item-activity" data-item-id="gone"><strong>新增遗留事项</strong>：Removed item</p>`, from)
	add(empty.ID, `<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul></ul>`, until.Add(time.Hour))
	for _, hard := range []bool{false, true} {
		removed := create("Deleted task")
		add(removed.ID, "<h3>每日进展 · 2026-10-10</h3><p>Deleted task progress</p>", from)
		if hard {
			_, err := s.Unscoped().ID(removed.ID).Delete(&Task{})
			require.NoError(t, err)
		} else {
			_, err := s.ID(removed.ID).Delete(&Task{})
			require.NoError(t, err)
		}
	}
	report := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: from, Until: until}
	require.NoError(t, report.ReadOne(s, actor))
	require.Equal(t, 1, report.TaskCount)
	require.Equal(t, 1, report.ChangeCount)
	require.Len(t, report.Projects, 1)
	require.Len(t, report.Projects[0].Tasks, 1)
	require.Contains(t, report.Projects[0].Tasks[0].Outstanding, "Same title")
	require.Contains(t, report.Projects[0].Tasks[0].Description, "Current context")
	require.Contains(t, report.Markdown, "Same title")
	for _, absent := range []string{"Deleted task", "Removed item", "All items removed", "deleted note history"} {
		require.NotContains(t, report.Markdown, absent)
	}
	require.Empty(t, report.Warnings)
}

//nolint:gosmopolitan // Persisted Chinese activity and numbering match the UI.
func TestTaskTracePeriodReviewLatestOutstanding(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	task := &Task{ProjectID: 1, Title: "Latest outstanding"}
	require.NoError(t, task.Create(s, actor))
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	until := from.AddDate(0, 0, 7)
	add := func(body string, when time.Time) {
		_, err := s.NoAutoTime().Insert(&TaskComment{TaskID: task.ID, AuthorID: 1, Comment: body, Created: when, Updated: when})
		require.NoError(t, err)
	}
	for _, action := range []string{"新增", "修改", "修改"} {
		body := `<p data-tasktrace-comment-type="item-activity" data-item-id="edited"><strong>` + action + `遗留事项</strong>：Old title</p>`
		add(taskTraceTeamAddMarker(body, "shared-operation", "member"), from)
	}
	add(`<p data-tasktrace-comment-type="item-activity" data-item-id="completed"><strong>完成遗留事项</strong>：Finished title</p>`, until.Add(-time.Second))
	add(`<p data-tasktrace-comment-type="item-activity" data-item-id="outside"><strong>修改遗留事项</strong>：Outside period</p>`, until)
	add(`<p data-tasktrace-comment-type="item-activity" data-item-id="outside"><strong>新增遗留事项</strong>：Outside period</p>`, from.Add(-time.Second))
	image := "data:image/png;base64,iVBORw0KGgo="
	add(`<h3 data-tasktrace-comment-type="outstanding">TaskTrace 遗留事项清单</h3><ul>`+
		`<li data-id="completed" data-done="true">Finished title</li>`+
		`<li data-id="edited"><p>Latest title</p><aside data-tasktrace-outstanding-note="true" hidden><p>Latest memo</p><img alt="current picture" src="`+image+`"><p>After picture</p></aside></li>`+
		`<li data-id="untouched">Unchanged item</li><li data-id="outside">Outside period</li></ul>`, until.Add(time.Hour))
	report := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: from, Until: until}
	require.NoError(t, report.ReadOne(s, actor))
	require.Equal(t, 4, report.ChangeCount)
	body := report.Projects[0].Tasks[0].Outstanding
	require.Contains(t, body, "1. **【新增】** **【有改动】** Latest title")
	require.Contains(t, body, "4. **【已完成】** Finished title")
	require.Equal(t, 1, strings.Count(body, "Latest title"))
	require.Contains(t, body, "2. Unchanged item")
	require.Contains(t, body, "3. Outside period")
	require.Contains(t, body, "Latest memo")
	require.Contains(t, body, image)
	require.Contains(t, body, "After picture")
	for _, absent := range []string{"Old title", "保存时间", "新增遗留事项", "修改遗留事项", "修改前", "修改后"} {
		require.NotContains(t, body, absent)
	}
	require.Contains(t, report.Markdown, body)
}

//nolint:gosmopolitan // Period flags are persisted Chinese UI labels.
func TestTaskTracePeriodReviewFlagsAndUnchangedItems(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	from := time.Date(2026, 10, 10, 0, 0, 0, 0, time.FixedZone("local", 8*3600))
	until := from.AddDate(0, 0, 7)
	create := func(title string) *Task {
		task := &Task{Title: title, ProjectID: 1}
		require.NoError(t, task.Create(s, actor))
		return task
	}
	add := func(taskID int64, body string, when time.Time) {
		_, err := s.NoAutoTime().Insert(&TaskComment{TaskID: taskID, AuthorID: 1, Comment: body, Created: when, Updated: when})
		require.NoError(t, err)
	}
	unchanged := create("Task with no recent activity")
	add(unchanged.ID, `<h3 data-tasktrace-comment-type="outstanding">list</h3><ul><li data-id="quiet">Still open</li></ul>`, from.Add(-24*time.Hour))
	task := create("Flag matrix")
	activity := func(id, action string, when time.Time) {
		add(task.ID, `<p data-tasktrace-comment-type="item-activity" data-item-id="`+id+`"><strong>`+action+`遗留事项</strong>：title</p>`, when)
	}
	activity("all", "新增", from)
	activity("all", "修改", from.Add(time.Hour))
	activity("all", "完成", until.Add(-time.Second))
	activity("edited", "修改", from)
	activity("new", "新增", from)
	activity("new", "新增", from.Add(time.Hour)) // duplicate replay stays one item/flag
	activity("old-done", "完成", from.Add(-time.Second))
	activity("old-done", "修改", from.Add(time.Hour)) // modifying an old completed item does not reinclude it
	activity("after-done", "完成", until)
	activity("reopened", "完成", from.Add(time.Hour))
	activity("reopened", "重新打开", from.Add(2*time.Hour))
	activity("future", "新增", until)
	atStart := from.Format(time.RFC3339Nano)
	beforeStart := from.Add(-time.Second).Format(time.RFC3339Nano)
	add(task.ID, `<h3 data-tasktrace-comment-type="outstanding">list</h3><ul>`+
		`<li data-id="all" data-done="true">All flags</li><li data-id="edited">Edited title</li>`+
		`<li data-id="new">New title</li><li data-id="new">Duplicate canonical item</li>`+
		`<li data-id="old-done" data-done="true" data-completed-at="`+beforeStart+`">Old completion hidden</li>`+
		`<li data-id="after-done" data-done="true">Later completion hidden</li>`+
		`<li data-id="metadata-done" data-done="true" data-completed-at="`+atStart+`">Completion timestamp only</li>`+
		`<li data-id="unknown-done" data-done="true">Unknown old completion hidden</li>`+
		`<li data-id="reopened">Reopened title</li><li data-id="future">Future item hidden</li></ul>`, until.Add(time.Hour))
	report := &TaskTracePeriodReview{ProjectIDs: []int64{1}, From: from, Until: until}
	require.NoError(t, report.ReadOne(s, actor))
	require.Equal(t, 2, report.TaskCount)
	require.Equal(t, 6, report.ChangeCount)
	rows := report.Projects[0].Tasks
	require.Equal(t, "1. Still open\n\n", rows[0].Outstanding, "no activity task is included without a badge")
	body := rows[1].Outstanding
	for _, text := range []string{"1. **【有改动】** Edited title", "2. **【新增】** New title", "3. **【有改动】** Reopened title", "4. **【新增】** **【已完成】** **【有改动】** All flags", "5. **【已完成】** Completion timestamp only"} {
		require.Contains(t, body, text)
	}
	for _, absent := range []string{"Old completion hidden", "Later completion hidden", "Unknown old completion hidden", "Future item hidden", "Duplicate canonical item"} {
		require.NotContains(t, report.Markdown, absent)
	}
}
