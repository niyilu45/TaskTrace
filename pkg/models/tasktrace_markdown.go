// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"code.vikunja.io/api/pkg/richtext"
	"code.vikunja.io/api/pkg/web"
	xhtml "golang.org/x/net/html"
	"xorm.io/xorm"
)

// TaskTraceMarkdownExport is a read-only, self-contained snapshot of one subtree.
type TaskTraceMarkdownExport struct {
	TaskID          int64    `json:"-"`
	Filename        string   `json:"filename" readOnly:"true" doc:"Suggested Windows-safe Markdown filename."`
	Markdown        string   `json:"markdown" readOnly:"true" doc:"GFM document including readable descendants. Inline images are embedded as data URLs for offline use."`
	Warnings        []string `json:"warnings" readOnly:"true" doc:"Missing or inaccessible images, omitted without disclosing protected content."`
	web.CRUDable    `json:"-" xorm:"-"`
	web.Permissions `json:"-" xorm:"-"`
}

func (m *TaskTraceMarkdownExport) CanRead(s *xorm.Session, a web.Auth) (bool, int, error) {
	return (&Task{ID: m.TaskID}).CanRead(s, a)
}
func (m *TaskTraceMarkdownExport) ReadOne(s *xorm.Session, a web.Auth) error {
	m.Warnings = []string{}
	var output strings.Builder
	seen := map[int64]bool{}
	images := map[string]string{}
	var walk func(int64, int) error
	walk = func(id int64, depth int) error {
		if seen[id] {
			return nil
		}
		seen[id] = true
		task := &Task{ID: id}
		can, _, err := task.CanRead(s, a)
		if err != nil {
			return err
		}
		if !can {
			return nil
		}
		stored, err := GetTaskByIDSimple(s, id)
		if err != nil {
			return err
		}
		if depth == 1 {
			m.Filename = taskTraceMarkdownFilename(stored.Title)
		}
		title, err := richtext.HTMLToMarkdown(html.EscapeString(stored.Title))
		if err != nil {
			return err
		}
		fmt.Fprintf(&output, "%s %s\n\n", strings.Repeat("#", min(depth, 6)), title)
		completion := stored.PercentDone * 100
		if stored.Done {
			completion = 100
		}
		//nolint:gosmopolitan // TaskTrace exports use the same Chinese labels as its saved documents.
		fmt.Fprintf(&output, "状态：%s · 完成度：%.0f%% · 优先级：P%d\n\n", stored.Status, completion, stored.Priority)
		content, err := m.convert(s, a, stored.Description, images)
		if err != nil {
			return err
		}
		if content != "" {
			output.WriteString(content + "\n\n")
		}
		var comments []*TaskComment
		if err = s.Where("task_id = ?", id).Desc("created", "id").Find(&comments); err != nil {
			return err
		}
		for _, entry := range taskTraceMarkdownDaily(comments) {
			content, err = m.convert(s, a, entry.body, images)
			if err != nil {
				return err
			}
			fmt.Fprintf(&output, "**%s%s**\n\n%s\n\n", entry.date, entry.author, content)
		}
		outstanding, err := taskTraceParseOutstanding(comments)
		if err != nil {
			return err
		}
		if len(outstanding.items) > 0 {
			//nolint:gosmopolitan // User-facing TaskTrace document label.
			output.WriteString("**遗留事项**\n\n")
			for _, item := range outstanding.items {
				doc, parseErr := xhtml.Parse(strings.NewReader("<li " + item.metadata + ">" + item.content + "</li>"))
				if parseErr != nil {
					return parseErr
				}
				done := false
				taskTraceWalk(doc, func(n *xhtml.Node) {
					if n.Data == "li" && taskTraceAttribute(n, "data-done") == "true" {
						done = true
					}
				})
				mark := " "
				if done {
					mark = "x"
				}
				content, err = m.convert(s, a, item.content, images)
				if err != nil {
					return err
				}
				output.WriteString("- [" + mark + "] " + strings.ReplaceAll(content, "\n", "\n  ") + "\n")
			}
			output.WriteString("\n")
		}
		var relations []*TaskRelation
		if err = s.Where("task_id = ? AND relation_kind = ?", id, RelationKindSubtask).Asc("other_task_id").Find(&relations); err != nil {
			return err
		}
		for _, relation := range relations {
			if err = walk(relation.OtherTaskID, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(m.TaskID, 1); err != nil {
		return err
	}
	m.Markdown = output.String()
	return nil
}

var taskTraceMarkdownUnsafeName = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)

func taskTraceMarkdownFilename(title string) string {
	name := strings.Trim(taskTraceMarkdownUnsafeName.ReplaceAllString(title, "_"), " .")
	if name == "" {
		//nolint:gosmopolitan // User-facing default filename.
		name = "任务"
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	for _, reserved := range []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"} {
		if strings.EqualFold(strings.Split(name, ".")[0], reserved) {
			name = "_" + name
			break
		}
	}
	return name + ".md"
}

type taskTraceMarkdownDay struct {
	date, author, body, key string
	id                      int64
	merged                  []string
	team                    bool
}

func taskTraceMarkdownDaily(comments []*TaskComment) []taskTraceMarkdownDay {
	entries := []taskTraceMarkdownDay{}
	hidden := map[string]bool{}
	for _, c := range comments {
		doc, err := xhtml.Parse(strings.NewReader(c.Comment))
		if err != nil {
			continue
		}
		heading := taskTraceFirstHeading(doc)
		if heading == nil || taskTraceIsOutstandingDocument(doc) {
			continue
		}
		date := taskTraceDailyHeading.FindStringSubmatch(taskTraceText(heading))
		if len(date) < 2 {
			continue
		}
		marker, team := taskTraceTeamReadMarker(c.Comment)
		author := marker.Author
		if author == "" {
			author = strconv.FormatInt(c.AuthorID, 10)
		}
		key := strconv.FormatInt(c.ID, 10)
		attr := "data-tasktrace-merged"
		if team {
			key = marker.ID
			attr = "data-tasktrace-team-merged"
		}
		heading.Parent.RemoveChild(heading)
		body := taskTraceInnerHTML(doc)
		entries = append(entries, taskTraceMarkdownDay{date: date[1], author: author, body: body, key: key, id: c.ID, merged: strings.Split(taskTraceAttribute(heading, attr), ","), team: team})
	}
	for _, e := range entries {
		for _, id := range e.merged {
			for _, other := range entries {
				if other.key == id && other.id != e.id && other.date == e.date && other.author == e.author && other.team == e.team && (e.team || e.id > other.id) {
					hidden[other.key] = true
				}
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].date > entries[j].date })
	seen := map[string]bool{}
	result := []taskTraceMarkdownDay{}
	for _, e := range entries {
		lineage := e.date + "/" + taskTraceTeamMembershipName(e.author)
		if hidden[e.key] || (e.team && seen[lineage]) {
			continue
		}
		seen[lineage] = true
		if e.team {
			e.author = " · " + e.author
		} else {
			e.author = ""
		}
		result = append(result, e)
	}
	return result
}

var taskTraceMarkdownImage = regexp.MustCompile(`(?:^|/)(?:api/v[12]/)?tasks/(\d+)/attachments/(\d+)$`)

func (m *TaskTraceMarkdownExport) convert(s *xorm.Session, a web.Auth, input string, cache map[string]string) (string, error) {
	doc, err := xhtml.Parse(strings.NewReader(input))
	if err != nil {
		return "", err
	}
	var clean func(*xhtml.Node) error
	clean = func(n *xhtml.Node) error {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == xhtml.CommentNode || (child.Type == xhtml.ElementNode && (child.Data == "script" || child.Data == "style" || child.Data == "iframe" || taskTraceAttribute(child, "hidden") != "" || taskTraceAttribute(child, "data-tasktrace-outstanding-state") == "true")) {
				n.RemoveChild(child)
				child = next
				continue
			}
			if child.Type == xhtml.ElementNode && child.Data == "img" {
				src := taskTraceAttribute(child, "data-src")
				if src == "" {
					src = taskTraceAttribute(child, "src")
				}
				match := taskTraceMarkdownImage.FindStringSubmatch(src)
				if len(match) == 3 {
					value, ok := cache[src]
					if !ok {
						value, err = taskTraceMarkdownImageData(s, a, match[1], match[2])
						if err != nil {
							//nolint:gosmopolitan // User-facing TaskTrace export warning.
							m.Warnings = append(m.Warnings, "部分图片无法读取，已在正文中标注。")
							value = ""
						}
						cache[src] = value
					}
					if value == "" {
						child.Type = xhtml.TextNode
						//nolint:gosmopolitan // User-facing TaskTrace export warning.
						child.Data = "（图片无法导出）"
						child.Attr = nil
					} else {
						child.Attr = []xhtml.Attribute{{Key: "src", Val: value}, {Key: "alt", Val: taskTraceAttribute(child, "alt")}}
					}
				} else if !strings.HasPrefix(src, "data:image/") {
					child.Type = xhtml.TextNode
					//nolint:gosmopolitan // User-facing TaskTrace export warning.
					child.Data = "（外部图片未导出）"
					child.Attr = nil
				}
			}
			if err := clean(child); err != nil {
				return err
			}
			child = next
		}
		return nil
	}
	if err = clean(doc); err != nil {
		return "", err
	}
	return richtext.HTMLToMarkdown(taskTraceInnerHTML(doc))
}
func taskTraceMarkdownImageData(s *xorm.Session, a web.Auth, taskID, attachmentID string) (string, error) {
	task, _ := strconv.ParseInt(taskID, 10, 64)
	id, _ := strconv.ParseInt(attachmentID, 10, 64)
	attachment := &TaskAttachment{ID: id, TaskID: task}
	can, _, err := attachment.CanRead(s, a)
	if err != nil {
		return "", err
	}
	if !can {
		return "", ErrGenericForbidden{}
	}
	if err = attachment.ReadOne(s, a); err != nil {
		return "", err
	}
	if err = attachment.File.LoadFileByID(); err != nil {
		return "", err
	}
	defer attachment.File.File.Close()
	data, err := io.ReadAll(io.LimitReader(attachment.File.File, 32*1024*1024+1))
	if err != nil {
		return "", err
	}
	if len(data) > 32*1024*1024 {
		return "", fmt.Errorf("image exceeds export limit")
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return "", fmt.Errorf("attachment is not an image")
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}
