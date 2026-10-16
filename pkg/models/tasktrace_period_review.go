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
	ProjectIDs      []int64                  `json:"-"`
	From            time.Time                `json:"-"`
	Until           time.Time                `json:"-"`
	Filename        string                   `json:"filename" readOnly:"true"`
	Markdown        string                   `json:"markdown" readOnly:"true"`
	Warnings        []string                 `json:"warnings" readOnly:"true"`
	TaskCount       int                      `json:"task_count" readOnly:"true"`
	ChangeCount     int                      `json:"change_count" readOnly:"true"`
	Projects        []TaskTraceReviewProject `json:"projects" readOnly:"true"`
	projects        map[int64]*Project
	web.CRUDable    `json:"-" xorm:"-"`
	web.Permissions `json:"-" xorm:"-"`
}
type TaskTraceReviewProject struct {
	ID    int64                 `json:"id"`
	Title string                `json:"title"`
	Tasks []TaskTraceReviewTask `json:"tasks"`
}
type TaskTraceReviewTask struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description" doc:"Current task description as Markdown."`
	Progress    string `json:"progress" doc:"Daily progress changes as Markdown."`
	Outstanding string `json:"outstanding" doc:"Latest unfinished items and items completed during the period, with numbered period flags as Markdown."`
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
	when                   time.Time
	id                     int64
	label, body, author    string
	outstanding, completed bool
	itemOrder              int
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
func taskTraceReviewActivity(body string) (action, label, id string) {
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
		id = taskTraceAttribute(n, "data-item-id")
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
	// Read full histories so activity flags and unchanged items accompanying a
	// changed task are resolved against the same canonical list.
	withComments := builder.Select("task_id").From("task_comments")
	if err := s.In("project_id", r.ProjectIDs).Where("deleted_at IS NULL").And(builder.In("id", withComments)).Asc("project_id", "id").Find(&candidates); err != nil {
		return err
	}
	readable := make([]*Task, 0, len(candidates))
	taskIDs := make([]int64, 0, len(candidates))
	for _, task := range candidates {
		can, _, err := task.CanRead(s, a)
		if err != nil {
			return err
		}
		if can {
			readable = append(readable, task)
			taskIDs = append(taskIDs, task.ID)
		}
	}
	// Batch histories to avoid one author/reaction query per unchanged task.
	commentsByTask := map[int64][]*TaskComment{}
	for offset := 0; offset < len(taskIDs); offset += 256 {
		comments, _, _, err := getAllCommentsForTasksWithoutPermissionCheck(s, taskIDs[offset:min(offset+256, len(taskIDs))], "", 0, -1, "desc")
		if err != nil {
			return err
		}
		for _, comment := range comments {
			commentsByTask[comment.TaskID] = append(commentsByTask[comment.TaskID], comment)
		}
	}
	r.TaskCount = 0
	r.ChangeCount = 0
	r.Projects = []TaskTraceReviewProject{}
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
	output.WriteString("项目范围：" + strings.Join(names, "、") + "\n\n仅展示周期内每日进展或遗留事项有变化的任务。按实际保存或修改时间汇总；每日进展按日期及填写人保留区间内最后一版。遗留事项展示最新内容并编号：周期内新增、完成或修改分别标注；无变化的未完成事项不加标志，周期外完成的事项不展示。已删除任务和遗留事项不展示；任务描述为当前内容。\n\n")
	lastProject := int64(-1)
	for _, task := range readable {
		comments := commentsByTask[task.ID]
		entries, err := r.collectEntries(comments, convert)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			continue
		}
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].outstanding != entries[j].outstanding {
				return !entries[i].outstanding
			}
			if entries[i].outstanding {
				if entries[i].completed != entries[j].completed {
					return !entries[i].completed
				}
				return entries[i].itemOrder < entries[j].itemOrder
			}
			if entries[i].when.Equal(entries[j].when) {
				return entries[i].id > entries[j].id
			}
			return entries[i].when.After(entries[j].when)
		})
		if lastProject != task.ProjectID {
			fmt.Fprintf(&output, "\n## %s\n\n| 任务名 | 任务描述 | 遗留事项 | 进展 |\n| --- | --- | --- | --- |\n", taskTraceReviewText(r.projects[task.ProjectID].Title))
			lastProject = task.ProjectID
			r.Projects = append(r.Projects, TaskTraceReviewProject{ID: task.ProjectID, Title: r.projects[task.ProjectID].Title, Tasks: []TaskTraceReviewTask{}})
		}
		description, err := convert(task.Description)
		if err != nil {
			return err
		}
		row := TaskTraceReviewTask{ID: task.ID, Title: task.Title, Description: description}

		itemNumber := 0
		for _, e := range entries {
			if e.outstanding {
				itemNumber++
				prefix := fmt.Sprintf("%d. ", itemNumber)
				row.Outstanding += prefix + strings.ReplaceAll(strings.TrimSpace(e.body), "\n", "\n"+strings.Repeat(" ", len(prefix))) + "\n\n"
				continue
			}
			var entry strings.Builder
			fmt.Fprintf(&entry, "#### %s\n\n保存时间：%s", e.label, e.when.In(r.From.Location()).Format("2006-01-02 15:04"))
			if e.author != "" {
				entry.WriteString(" · " + taskTraceReviewText(e.author))
			}
			entry.WriteString("\n\n" + e.body + "\n\n")
			row.Progress += entry.String()
		}
		line, err := taskTraceReviewMarkdownRow(row)
		if err != nil {
			return err
		}
		output.WriteString(line)
		project := &r.Projects[len(r.Projects)-1]
		project.Tasks = append(project.Tasks, row)
		r.TaskCount++
		r.ChangeCount += len(entries)
	}
	if r.ChangeCount == 0 {
		output.WriteString("所选项目在此时间范围内没有可回顾的每日进展或遗留事项变化。\n")
	}
	r.Warnings = exporter.Warnings
	r.Filename = taskTraceMarkdownFilename("周期回顾_" + start + "_" + end)
	r.Markdown = output.String()
	return nil
}

//nolint:gosmopolitan // Saved daily progress labels are part of the local report.
func (r *TaskTracePeriodReview) collectEntries(comments []*TaskComment, convert func(string) (string, error)) ([]taskTraceReviewEntry, error) {
	// The current canonical list, including changes after Until, determines existence.
	// Operation logs remain for audit and must not resurrect deleted or moved items.
	current, err := taskTraceParseOutstanding(append([]*TaskComment(nil), comments...))
	if err != nil {
		return nil, err
	}
	// Legacy activity without an ID is only matched when its title is unambiguous.
	labels := map[string][]string{}
	for _, item := range current.items {
		label := taskTraceOutstandingActivityLabel(item)
		labels[label] = append(labels[label], item.id)
	}
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
			return nil, err
		}
		entries = append(entries, taskTraceReviewEntry{when: when, id: c.ID, label: "每日进展 · " + day.date, body: body, author: taskTraceReviewAuthor(c)})
	}
	type itemActivity struct {
		firstAdded, lastCompleted time.Time
		modified                  bool
	}
	activity := map[string]itemActivity{}
	inPeriod := func(when time.Time) bool { return !when.IsZero() && !when.Before(r.From) && when.Before(r.Until) }
	for _, c := range comments {
		action, label, id := taskTraceReviewActivity(c.Comment)
		if action == "" || action == "删除遗留事项" {
			continue
		}
		if id == "" && len(labels[label]) == 1 {
			id = labels[label][0]
		}
		if id == "" {
			continue
		}
		when := taskTraceReviewTime(c)
		state := activity[id]
		switch {
		case action == "新增遗留事项":
			if state.firstAdded.IsZero() || when.Before(state.firstAdded) {
				state.firstAdded = when
			}
		case action == "完成遗留事项":
			if when.After(state.lastCompleted) {
				state.lastCompleted = when
			}
		case strings.HasPrefix(action, "修改"), action == "重新打开遗留事项":
			state.modified = state.modified || inPeriod(when)
		}
		activity[id] = state
	}
	type reviewItem struct {
		item      taskTraceOutstandingItem
		index     int
		completed bool
		flags     []string
	}
	items := []reviewItem{}
	hasPeriodChanges := len(entries) > 0
	seenItems := map[string]bool{}
	for index, item := range current.items {
		if seenItems[item.id] {
			continue
		}
		seenItems[item.id] = true
		state := activity[item.id]
		// A known item created after this historical period did not yet exist.
		if !state.firstAdded.IsZero() && !state.firstAdded.Before(r.Until) {
			continue
		}
		completed := taskTraceOutstandingItemAttribute(item, "data-done") == "true"
		completedAt := state.lastCompleted
		if stored, err := time.Parse(time.RFC3339Nano, taskTraceOutstandingItemAttribute(item, "data-completed-at")); err == nil {
			completedAt = stored
		}
		if completed && !inPeriod(completedAt) {
			continue
		}
		flags := []string{}
		if inPeriod(state.firstAdded) {
			flags = append(flags, "**【新增】**")
		}
		if completed {
			flags = append(flags, "**【已完成】**")
		}
		if state.modified {
			flags = append(flags, "**【有改动】**")
		}
		hasPeriodChanges = hasPeriodChanges || len(flags) > 0
		items = append(items, reviewItem{
			item:      item,
			index:     index,
			completed: completed,
			flags:     flags,
		})
	}
	// Unchanged items accompany active tasks, but cannot include a task on their
	// own. Filter before exporting their images to avoid unnecessary reads/warnings.
	if !hasPeriodChanges {
		return nil, nil
	}
	for _, selected := range items {
		// Activity supplies flags only. Never render stale snapshots or repeat an item
		// for each edit; use the same current list as the editors, including images.
		title, note := taskTraceReviewItemParts(selected.item)
		body, err := convert(title)
		if err != nil {
			return nil, err
		}
		memo, err := convert(note)
		if err != nil {
			return nil, err
		}
		if memo != "" {
			body += "\n\n**备注**\n\n" + memo
		}
		if len(selected.flags) > 0 {
			body = strings.Join(selected.flags, " ") + " " + body
		}
		entries = append(entries, taskTraceReviewEntry{
			outstanding: true,
			completed:   selected.completed,
			itemOrder:   selected.index,
			body:        body,
		})
	}
	return entries, nil
}

// HTML inside GFM cells preserves paragraphs, lists and images without splitting
// one task across rows. Both the table and the web view use these same row fields.
//
//nolint:gosmopolitan // Export labels mirror the Chinese web table.
func taskTraceReviewMarkdownRow(row TaskTraceReviewTask) (string, error) {
	values := []string{taskTraceReviewText(row.Title), row.Description, row.Outstanding, row.Progress}
	fallbacks := []string{"", "暂无描述", "无符合条件的遗留事项", "期间无进展更新"}
	cells := make([]string, len(values))
	for i, value := range values {
		if value == "" {
			value = fallbacks[i]
		}
		body, err := richtext.MarkdownToHTML(value)
		if err != nil {
			return "", err
		}
		doc, err := html.Parse(strings.NewReader(body))
		if err != nil {
			return "", err
		}
		taskTraceWalk(doc, func(n *html.Node) {
			if n.Data == "li" && taskTraceAttribute(n, "data-type") == "taskItem" {
				mark := "☐ "
				if taskTraceAttribute(n, "data-checked") == "true" {
					mark = "☑ "
				}
				parent := n
				if n.FirstChild != nil && n.FirstChild.Data == "p" {
					parent = n.FirstChild
				}
				parent.InsertBefore(&html.Node{Type: html.TextNode, Data: mark}, parent.FirstChild)
			}
		})
		taskTraceWalk(doc, func(n *html.Node) {
			if n.Data == "body" {
				cells[i] = strings.NewReplacer("|", "&#124;", "\r", "&#13;", "\n", "&#10;").Replace(taskTraceInnerHTML(n))
			}
		})
	}
	return "| " + strings.Join(cells, " | ") + " |\n", nil
}
