// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/richtext"
	"code.vikunja.io/api/pkg/web"
	"golang.org/x/net/html"
	"xorm.io/builder"
	"xorm.io/xorm"
)

// TaskTracePeriodReview builds a read-only report from persisted progress and activity.
type TaskTracePeriodReview struct {
	ProjectIDs      []int64   `json:"-"`
	From            time.Time `json:"-"`
	Until           time.Time `json:"-"`
	Filename        string    `json:"filename" readOnly:"true"`
	Markdown        string    `json:"markdown" readOnly:"true"`
	Warnings        []string  `json:"warnings" readOnly:"true"`
	TaskCount       int       `json:"task_count" readOnly:"true"`
	ChangeCount     int       `json:"change_count" readOnly:"true"`
	projects        map[int64]*Project
	web.CRUDable    `json:"-" xorm:"-"`
	web.Permissions `json:"-" xorm:"-"`
}
type errTaskTraceReview string

func (e errTaskTraceReview) Error() string { return string(e) }
func (e errTaskTraceReview) HTTPError() web.HTTPError {
	return web.HTTPError{HTTPCode: http.StatusBadRequest, Code: 1046, Message: string(e)}
}
func (r *TaskTracePeriodReview) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	if len(r.ProjectIDs) == 0 || len(r.ProjectIDs) > 5000 {
		return false, 0, errTaskTraceReview("Select at least one project (maximum 5000)")
	}
	if r.From.IsZero() || r.Until.IsZero() || !r.Until.After(r.From) {
		return false, 0, errTaskTraceReview("Select a valid date range")
	}
	r.projects = map[int64]*Project{}
	for _, id := range r.ProjectIDs {
		if _, ok := r.projects[id]; ok {
			continue
		}
		p := &Project{ID: id}
		can, _, err := p.CanRead(s, a)
		if err != nil || !can {
			return false, 0, err
		}
		r.projects[id] = p
	}
	return true, 0, nil
}

type taskTraceReviewEntry struct {
	when                time.Time
	id                  int64
	label, body, author string
}

func taskTraceReviewTime(c *TaskComment) time.Time {
	if c.Updated.After(c.Created) {
		return c.Updated
	}
	return c.Created
}
func taskTraceReviewAuthor(c *TaskComment) string {
	if marker, ok := taskTraceTeamReadMarker(c.Comment); ok && marker.Author != "" {
		return marker.Author
	}
	if c.Author != nil {
		if c.Author.Name != "" {
			return c.Author.Name
		}
		return c.Author.Username
	}
	return ""
}
func taskTraceReviewText(text string) string {
	value, _ := richtext.HTMLToMarkdown(html.EscapeString(text))
	return value
}
func taskTraceReviewItemParts(item taskTraceOutstandingItem) (title, note string) {
	doc, _ := html.Parse(strings.NewReader(item.content))
	var notes []*html.Node
	taskTraceWalk(doc, func(n *html.Node) {
		if taskTraceAttribute(n, "data-tasktrace-outstanding-note") == "true" {
			notes = append(notes, n)
		}
	})
	for _, n := range notes {
		note += taskTraceInnerHTML(n)
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	var content string
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Data == "body" {
			content = taskTraceInnerHTML(n)
		}
	})
	return content, note
}

// Store immutable field snapshots with the existing operation, not another log.
// Real image nodes allow the collaboration attachment mapper to keep URLs valid.
func taskTraceReviewItemSnapshot(old taskTraceOutstandingItem, exists bool, item taskTraceOutstandingItem) string {
	title, after := taskTraceReviewItemParts(item)
	_, before := taskTraceReviewItemParts(old)
	if exists && taskTraceTeamCanonicalHTML(before, nil) == taskTraceTeamCanonicalHTML(after, nil) {
		return `<section data-tasktrace-review-change="1" data-review-added="false" hidden></section>`
	}
	added := "false"
	if !exists {
		added = "true"
	}
	return `<section data-tasktrace-review-change="1" data-review-added="` + added + `" hidden><div data-review-field="title">` + title + `</div><div data-review-field="before">` + before + `</div><div data-review-field="after">` + after + `</div></section>`
}
func taskTraceReviewSnapshot(body string) (added bool, fields map[string]string, found bool) {
	fields = map[string]string{}
	doc, _ := html.Parse(strings.NewReader(body))
	taskTraceWalk(doc, func(n *html.Node) {
		if taskTraceAttribute(n, "data-tasktrace-review-change") != "1" {
			return
		}
		found = true
		added = taskTraceAttribute(n, "data-review-added") == "true"
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if key := taskTraceAttribute(c, "data-review-field"); key != "" {
				fields[key] = taskTraceInnerHTML(c)
			}
		}
	})
	return
}
func taskTraceReviewActivity(body string) (action, label string) {
	doc, _ := html.Parse(strings.NewReader(body))
	taskTraceWalk(doc, func(n *html.Node) {
		if taskTraceAttribute(n, taskTraceOutstandingTypeAttribute) != taskTraceOutstandingActivityType {
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Data == "strong" {
				action = taskTraceText(child)
				break
			}
		}
		label = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(strings.TrimPrefix(taskTraceText(n), action)), "："))
	})
	return
}

//nolint:gosmopolitan // TaskTrace's Chinese report labels match its saved activity documents.
func (r *TaskTracePeriodReview) ReadOne(s *xorm.Session, a web.Auth) error {
	if r.projects == nil {
		can, _, err := r.CanRead(s, a)
		if err != nil {
			return err
		}
		if !can {
			return ErrGenericForbidden{}
		}
	}
	exporter := &TaskTraceMarkdownExport{Warnings: []string{}}
	images := map[string]string{}
	convert := func(body string) (string, error) { return exporter.convert(s, a, body, images) }
	var candidates []*Task
	// Match the UTC database DATETIME format, including exact second boundaries.
	// Driver time values can carry an offset suffix which changes SQLite comparisons.
	// Only load histories for tasks with activity during the requested interval.
	changed := builder.Select("task_id").From("task_comments").Where(builder.Or(
		builder.And(builder.Gte{"created": r.From.UTC().Format(dbTimeFormat)}, builder.Lt{"created": r.Until.UTC().Format(dbTimeFormat)}),
		builder.And(builder.Gte{"updated": r.From.UTC().Format(dbTimeFormat)}, builder.Lt{"updated": r.Until.UTC().Format(dbTimeFormat)}),
	))
	if err := s.In("project_id", r.ProjectIDs).Where("deleted_at IS NULL").And(builder.In("id", changed)).Asc("project_id", "id").Find(&candidates); err != nil {
		return err
	}
	var output strings.Builder
	start, end := r.From.Format("2006-01-02"), r.Until.Add(-time.Second).Format("2006-01-02")
	fmt.Fprintf(&output, "# 周期回顾\n\n时间范围：%s 至 %s（含当天）\n\n", start, end)
	names := []string{}
	ids := make([]int64, 0, len(r.projects))
	for id := range r.projects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		p := r.projects[id]
		if p.Title == "" {
			_, err := s.ID(id).Get(p)
			if err != nil {
				return err
			}
		}
		names = append(names, taskTraceReviewText(p.Title))
	}
	output.WriteString("项目范围：" + strings.Join(names, "、") + "\n\n按实际保存或修改时间汇总；每日进展按日期及填写人保留区间内最后一版。\n\n")
	lastProject := int64(-1)
	legacy := false
	for _, task := range candidates {
		can, _, err := task.CanRead(s, a)
		if err != nil {
			return err
		}
		if !can {
			continue
		}
		comments, _, _, err := getAllCommentsForTasksWithoutPermissionCheck(s, []int64{task.ID}, "", 0, -1, "desc")
		if err != nil {
			return err
		}
		entries, old, err := r.collectEntries(comments, convert)
		if err != nil {
			return err
		}
		legacy = legacy || old
		if len(entries) == 0 {
			continue
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].when.Equal(entries[j].when) {
				return entries[i].id > entries[j].id
			}
			return entries[i].when.After(entries[j].when)
		})
		if lastProject != task.ProjectID {
			fmt.Fprintf(&output, "## %s\n\n", taskTraceReviewText(r.projects[task.ProjectID].Title))
			lastProject = task.ProjectID
		}
		fmt.Fprintf(&output, "### %s\n\n", taskTraceReviewText(task.Title))
		for _, e := range entries {
			fmt.Fprintf(&output, "#### %s\n\n保存时间：%s", e.label, e.when.In(r.From.Location()).Format("2006-01-02 15:04"))
			if e.author != "" {
				output.WriteString(" · " + taskTraceReviewText(e.author))
			}
			output.WriteString("\n\n" + e.body + "\n\n")
		}
		r.TaskCount++
		r.ChangeCount += len(entries)
	}
	if r.ChangeCount == 0 {
		output.WriteString("所选项目在此时间范围内没有可回顾的进展更新、遗留事项新增或备注修改。\n")
	}
	r.Warnings = exporter.Warnings
	if legacy {
		r.Warnings = append(r.Warnings, "部分旧遗留事项记录只有操作名称，未保存修改前后内容；已单独标注，未推测历史差异。")
	}
	r.Filename = taskTraceMarkdownFilename("周期回顾_" + start + "_" + end)
	r.Markdown = output.String()
	return nil
}

//nolint:gosmopolitan // Saved daily progress labels are part of the local report.
func (r *TaskTracePeriodReview) collectEntries(comments []*TaskComment, convert func(string) (string, error)) ([]taskTraceReviewEntry, bool, error) {
	legacy := false
	// A later edit must not be mislabeled as a past version.
	history := []*TaskComment{}
	for _, c := range comments {
		if taskTraceReviewTime(c).Before(r.Until) {
			history = append(history, c)
		}
	}
	sort.SliceStable(history, func(i, j int) bool {
		a, b := taskTraceReviewTime(history[i]), taskTraceReviewTime(history[j])
		if a.Equal(b) {
			return history[i].ID > history[j].ID
		}
		return a.After(b)
	})
	byID := map[int64]*TaskComment{}
	for _, c := range history {
		byID[c.ID] = c
	}
	entries := []taskTraceReviewEntry{}
	seen := map[string]bool{}
	for _, day := range taskTraceMarkdownDaily(history) {
		c := byID[day.id]
		key := day.date + "/" + taskTraceTeamMembershipName(taskTraceReviewAuthor(c))
		if taskTraceReviewAuthor(c) == "" {
			key += "/" + strconv.FormatInt(c.AuthorID, 10)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		when := taskTraceReviewTime(c)
		if when.Before(r.From) {
			continue
		}
		body, err := convert(day.body)
		if err != nil {
			return nil, false, err
		}
		entries = append(entries, taskTraceReviewEntry{when: when, id: c.ID, label: "每日进展 · " + day.date, body: body, author: taskTraceReviewAuthor(c)})
	}
	seenActivity := map[string]bool{}
	for _, c := range history {
		when := taskTraceReviewTime(c)
		if when.Before(r.From) {
			continue
		}
		action, label := taskTraceReviewActivity(c.Comment)
		if action == "" {
			continue
		}
		if marker, ok := taskTraceTeamReadMarker(c.Comment); ok && marker.ID != "" {
			if seenActivity[marker.ID] {
				continue
			}
			seenActivity[marker.ID] = true
		}
		kind, body, old, err := taskTraceReviewActivityContent(c.Comment, action, label, convert)
		if err != nil {
			return nil, false, err
		}
		legacy = legacy || old
		if kind == "" {
			continue
		}
		entries = append(entries, taskTraceReviewEntry{when: when, id: c.ID, label: kind, body: body, author: taskTraceReviewAuthor(c)})
	}
	return entries, legacy, nil
}

//nolint:gosmopolitan // Chinese report field labels match the local application.
func taskTraceReviewActivityContent(raw, action, label string, convert func(string) (string, error)) (kind, body string, legacy bool, err error) {
	added, fields, found := taskTraceReviewSnapshot(raw)
	switch {
	case found:
		if !added && taskTraceTeamCanonicalHTML(fields["before"], nil) == taskTraceTeamCanonicalHTML(fields["after"], nil) {
			return
		}
		title, e := convert(fields["title"])
		if e != nil {
			return "", "", false, e
		}
		after, e := convert(fields["after"])
		if e != nil {
			return "", "", false, e
		}
		if added {
			kind = "新增遗留事项"
			body = title
			if after != "" {
				body += "\n\n**备注**\n\n" + after
			}
		} else {
			before, e := convert(fields["before"])
			if e != nil {
				return "", "", false, e
			}
			if before == "" {
				before = "（空）"
			}
			if after == "" {
				after = "（已清空）"
			}
			kind = "遗留事项备注修改"
			body = title + "\n\n**修改前**\n\n" + before + "\n\n**修改后**\n\n" + after
		}
	case action == "新增遗留事项":
		kind = action
		body = taskTraceReviewText(label)
	case action == "修改遗留事项":
		legacy = true
		kind = "遗留事项修改（旧记录）"
		body = taskTraceReviewText(label) + "\n\n旧记录未保存字段快照，无法区分正文与备注的变化，也无法还原修改前后内容。"
	}
	return
}
