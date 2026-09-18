// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

// TaskTrace shared outstanding-item moves use the task's update permission.
package models

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"golang.org/x/net/html"
	"xorm.io/xorm"
)

const taskTraceOutstandingHeading = "TaskTrace 遗留事项清单"

// ErrTaskTraceOutstandingMove means a move cannot be applied to the current lists.
type ErrTaskTraceOutstandingMove struct{ Reason string }

func (e ErrTaskTraceOutstandingMove) Error() string { return e.Reason }
func (e ErrTaskTraceOutstandingMove) HTTPError() web.HTTPError {
	return web.HTTPError{HTTPCode: http.StatusConflict, Code: 4092, Message: e.Reason}
}

// TaskTraceOutstandingMove atomically moves one item, keeping all other items intact.
type TaskTraceOutstandingMove struct {
	TaskID          int64  `json:"task_id" readOnly:"true" doc:"Source task, taken from the URL."`
	ItemID          string `json:"item_id" minLength:"1" maxLength:"200" doc:"Stable data-id of the item in the source list."`
	TargetTaskID    int64  `json:"target_task_id" minimum:"1" doc:"Destination task in the same project; may equal the source to reorder."`
	BeforeItemID    string `json:"before_item_id" maxLength:"200" doc:"Insert before this destination item; empty appends. A missing anchor rejects the move."`
	SourceCommentID int64  `json:"source_comment_id" readOnly:"true" doc:"Canonical source list comment after the move."`
	TargetCommentID int64  `json:"target_comment_id" readOnly:"true" doc:"Canonical destination list comment after the move."`
	web.CRUDable    `json:"-" xorm:"-"`
	web.Permissions `json:"-" xorm:"-"`
	source          Task
	target          Task
	createdFiles    []int64
}

func (m *TaskTraceOutstandingMove) CanCreate(s *xorm.Session, a web.Auth) (bool, error) {
	m.SourceCommentID, m.TargetCommentID = 0, 0
	m.source = Task{ID: m.TaskID}
	can, err := m.source.CanUpdate(s, a)
	if err != nil || !can {
		return can, err
	}
	m.target = Task{ID: m.TargetTaskID}
	can, err = m.target.CanUpdate(s, a)
	if err != nil || !can {
		return can, err
	}
	// CanUpdate caches the task rather than populating its receiver.
	m.source, err = GetTaskByIDSimple(s, m.TaskID)
	if err != nil {
		return false, err
	}
	m.target, err = GetTaskByIDSimple(s, m.TargetTaskID)
	if err != nil {
		return false, err
	}
	if m.source.ProjectID != m.target.ProjectID {
		return false, ErrTaskTraceOutstandingMove{"只能在同一个项目内移动遗留事项。"}
	}
	return true, nil
}

type taskTraceOutstandingItem struct{ id, content, metadata string }
type taskTraceOutstandingList struct {
	comment  *TaskComment
	original string
	items    []taskTraceOutstandingItem
}

func taskTraceWalk(n *html.Node, visit func(*html.Node)) {
	visit(n)
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		taskTraceWalk(child, visit)
	}
}
func taskTraceText(n *html.Node) string {
	var out strings.Builder
	taskTraceWalk(n, func(node *html.Node) {
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
		}
	})
	return out.String()
}
func taskTraceInnerHTML(n *html.Node) string {
	var out bytes.Buffer
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		_ = html.Render(&out, child)
	}
	return out.String()
}
func taskTraceAttribute(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}
func taskTraceOutstandingMetadata(n *html.Node) string {
	var out strings.Builder
	for _, attr := range n.Attr {
		if attr.Key == "data-done" || attr.Key == "data-priority" || attr.Key == "data-completed-at" {
			out.WriteString(` ` + attr.Key + `="` + html.EscapeString(attr.Val) + `"`)
		}
	}
	return out.String()
}
func taskTraceFirstHeading(doc *html.Node) *html.Node {
	var heading *html.Node
	taskTraceWalk(doc, func(n *html.Node) {
		if heading == nil && n.Type == html.ElementNode && n.Data == "h3" {
			heading = n
		}
	})
	return heading
}

var taskTraceDailyHeading = regexp.MustCompile(`^每日进展\s*[·:：]\s*(\d{4}-\d{2}-\d{2})$`)
var taskTraceLineBreak = regexp.MustCompile(`(?i)<br\s*/?>(?:\r?\n)?`)

func taskTraceReadOutstanding(s *xorm.Session, taskID int64) (*taskTraceOutstandingList, error) {
	comments := []*TaskComment{}
	if err := s.Where("task_id = ?", taskID).Desc("id").Find(&comments); err != nil {
		return nil, err
	}
	return taskTraceParseOutstanding(comments)
}
func taskTraceParseOutstanding(comments []*TaskComment) (*taskTraceOutstandingList, error) {
	result := &taskTraceOutstandingList{items: []taskTraceOutstandingItem{}}
	type note struct {
		comment      *TaskComment
		doc, heading *html.Node
		date         string
	}
	notes := []note{}
	absorbed := map[int64]bool{}
	// Canonical lists are selected by ID, not timestamps (as in the web helper).
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].ID > comments[j].ID })
	for _, comment := range comments {
		doc, err := html.Parse(strings.NewReader(comment.Comment))
		if err != nil {
			return nil, err
		}
		heading := taskTraceFirstHeading(doc)
		if heading == nil {
			continue
		}
		if taskTraceText(heading) == taskTraceOutstandingHeading {
			result.comment = comment
			result.original = comment.Comment
			taskTraceWalk(doc, func(n *html.Node) {
				if n.Type != html.ElementNode || n.Data != "li" || n.Parent == nil || n.Parent.Data != "ul" {
					return
				}
				id := taskTraceAttribute(n, "data-id")
				if id == "" {
					id = fmt.Sprintf("item-%d", len(result.items))
				}
				result.items = append(result.items, taskTraceOutstandingItem{id, taskTraceInnerHTML(n), taskTraceOutstandingMetadata(n)})
			})
			return result, nil
		}
		for _, idText := range strings.Split(taskTraceAttribute(heading, "data-tasktrace-merged"), ",") {
			id, _ := strconv.ParseInt(idText, 10, 64)
			if id > 0 && id != comment.ID {
				absorbed[id] = true
			}
		}
		match := taskTraceDailyHeading.FindStringSubmatch(strings.TrimSpace(taskTraceText(heading)))
		if len(match) == 0 {
			continue
		}
		date := match[1]
		if _, err := time.Parse("2006-01-02", date); err != nil {
			date = comment.Created.Format("2006-01-02")
		}
		notes = append(notes, note{comment, doc, heading, date})
	}
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].date != notes[j].date {
			return notes[i].date > notes[j].date
		}
		if !notes[i].comment.Created.Equal(notes[j].comment.Created) {
			return notes[i].comment.Created.After(notes[j].comment.Created)
		}
		return notes[i].comment.ID > notes[j].comment.ID
	})
	for _, note := range notes {
		if absorbed[note.comment.ID] {
			continue
		}
		outstanding := ""
		found := false
		taskTraceWalk(note.doc, func(n *html.Node) {
			if found || n.Type != html.ElementNode || n.Data != "p" || strings.TrimSpace(taskTraceText(n)) != "遗留问题 / 下一步" {
				return
			}
			strong := false
			taskTraceWalk(n, func(child *html.Node) {
				if child.Type == html.ElementNode && child.Data == "strong" {
					strong = true
				}
			})
			if !strong {
				return
			}
			found = true
			next := n.NextSibling
			for next != nil && next.Type != html.ElementNode {
				next = next.NextSibling
			}
			if next != nil && next.Data == "p" {
				outstanding = taskTraceInnerHTML(next)
			}
		})
		index := 0
		for _, content := range taskTraceLineBreak.Split(outstanding, -1) {
			if strings.TrimSpace(content) == "" {
				continue
			}
			result.items = append(result.items, taskTraceOutstandingItem{fmt.Sprintf("legacy-%d-%d", note.comment.ID, index), content, ""})
			index++
		}
		break // The latest daily note's empty list is authoritative too.
	}
	return result, nil
}

func taskTraceFindItem(items []taskTraceOutstandingItem, id string) int {
	for index, item := range items {
		if item.id == id {
			return index
		}
	}
	return -1
}
func taskTraceValidateItemIDs(items []taskTraceOutstandingItem) error {
	seen := map[string]bool{}
	for _, item := range items {
		if seen[item.id] {
			return ErrTaskTraceOutstandingMove{"遗留事项编号重复，请刷新清单后重试。"}
		}
		seen[item.id] = true
	}
	return nil
}

func (m *TaskTraceOutstandingMove) Create(s *xorm.Session, a web.Auth) error {
	// Row locks serialize moves on either list. SQLite serializes writers itself.
	ids := []int64{m.TaskID, m.TargetTaskID}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for index, id := range ids {
		if index > 0 && ids[index-1] == id {
			continue
		}
		var row Task
		exists, err := lockingSession(s).ID(id).Get(&row)
		if err != nil {
			return err
		}
		if !exists {
			return ErrTaskDoesNotExist{ID: id}
		}
		if row.ProjectID != m.source.ProjectID {
			return ErrTaskTraceOutstandingMove{"任务所属项目已改变，请刷新后重试。"}
		}
	}
	source, err := taskTraceReadOutstanding(s, m.TaskID)
	if err != nil {
		return err
	}
	if err := taskTraceValidateItemIDs(source.items); err != nil {
		return err
	}
	from := taskTraceFindItem(source.items, m.ItemID)
	if from < 0 {
		return ErrTaskTraceOutstandingMove{"遗留事项已改变或已被移动，请刷新后重试。"}
	}
	target := source
	if m.TaskID != m.TargetTaskID {
		target, err = taskTraceReadOutstanding(s, m.TargetTaskID)
		if err != nil {
			return err
		}
		if err := taskTraceValidateItemIDs(target.items); err != nil {
			return err
		}
		if taskTraceFindItem(target.items, m.ItemID) >= 0 {
			return ErrTaskTraceOutstandingMove{"目标任务已有相同编号的遗留事项，请刷新后重试。"}
		}
	}
	if m.BeforeItemID != "" && taskTraceFindItem(target.items, m.BeforeItemID) < 0 {
		return ErrTaskTraceOutstandingMove{"目标位置已改变，请刷新后重新拖动。"}
	}
	if m.TaskID == m.TargetTaskID && m.ItemID == m.BeforeItemID {
		if source.comment != nil {
			m.SourceCommentID = source.comment.ID
			m.TargetCommentID = source.comment.ID
		}
		return nil
	}
	item := source.items[from]
	source.items = append(source.items[:from:from], source.items[from+1:]...)
	if m.TaskID != m.TargetTaskID {
		item.content, err = m.copyImages(s, a, item.content)
		if err != nil {
			return err
		}
	}
	insert := len(target.items)
	if m.BeforeItemID != "" {
		insert = taskTraceFindItem(target.items, m.BeforeItemID)
	}
	target.items = append(target.items, taskTraceOutstandingItem{})
	copy(target.items[insert+1:], target.items[insert:])
	target.items[insert] = item
	doer, err := GetUserOrLinkShareUser(s, a)
	if err != nil {
		return err
	}
	m.SourceCommentID, err = taskTraceWriteOutstanding(s, a, doer, m.source, source)
	if err != nil {
		return err
	}
	if m.TaskID == m.TargetTaskID {
		m.TargetCommentID = m.SourceCommentID
		return nil
	}
	m.TargetCommentID, err = taskTraceWriteOutstanding(s, a, doer, m.target, target)
	return err
}
func taskTraceWriteOutstanding(s *xorm.Session, a web.Auth, doer *user.User, task Task, list *taskTraceOutstandingList) (int64, error) {
	var out strings.Builder
	out.WriteString("<h3>" + taskTraceOutstandingHeading + "</h3><ul>")
	for _, item := range list.items {
		out.WriteString(`<li data-id="` + html.EscapeString(item.id) + `"` + item.metadata + `>` + item.content + "</li>")
	}
	out.WriteString("</ul>")
	if list.comment == nil {
		comment := &TaskComment{TaskID: task.ID, Comment: out.String()}
		if err := comment.Create(s, a); err != nil {
			return 0, err
		}
		return comment.ID, nil
	}
	if list.original == out.String() {
		return list.comment.ID, nil
	}
	comment := *list.comment
	comment.Comment = out.String()
	count, err := s.Where("id = ? AND task_id = ? AND comment = ?", comment.ID, task.ID, list.original).Cols("comment").Update(&comment)
	if err != nil {
		return 0, err
	}
	if count != 1 {
		return 0, ErrTaskTraceOutstandingMove{"遗留事项清单已被其他操作修改，请刷新后重试。"}
	}
	events.DispatchOnCommit(s, &TaskCommentUpdatedEvent{Task: &task, Comment: &comment, Doer: doer})
	return comment.ID, nil
}

var taskTraceAttachmentPath = regexp.MustCompile(`^/api/v[12]/tasks/([0-9]+)/attachments/([0-9]+)$`)

func (m *TaskTraceOutstandingMove) copyImages(s *xorm.Session, a web.Auth, content string) (string, error) {
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return "", err
	}
	images := []*html.Node{}
	var body *html.Node
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			images = append(images, n)
		}
		if n.Type == html.ElementNode && n.Data == "body" {
			body = n
		}
	})
	copies := map[string]string{}
	for _, img := range images {
		for index, attr := range img.Attr {
			if attr.Key != "src" {
				continue
			}
			parsed, err := url.Parse(attr.Val)
			if err != nil {
				continue
			}
			// Only TaskTrace relative attachment references are handled. Never fetch remote URLs.
			if parsed.IsAbs() || parsed.Host != "" {
				continue
			}
			match := taskTraceAttachmentPath.FindStringSubmatch(parsed.Path)
			if len(match) == 0 {
				continue
			}
			ownerID, _ := strconv.ParseInt(match[1], 10, 64)
			attachmentID, _ := strconv.ParseInt(match[2], 10, 64)
			if ownerID == m.TargetTaskID {
				continue
			}
			replacement, exists := copies[parsed.Path]
			if !exists {
				replacement, err = m.copyAttachment(s, a, ownerID, attachmentID)
				if err != nil {
					return "", err
				}
				copies[parsed.Path] = replacement
			}
			img.Attr[index].Val = replacement
		}
	}
	if body == nil {
		return content, nil
	}
	return taskTraceInnerHTML(body), nil
}
func (m *TaskTraceOutstandingMove) copyAttachment(s *xorm.Session, a web.Auth, ownerID, attachmentID int64) (string, error) {
	attachment := &TaskAttachment{ID: attachmentID, TaskID: ownerID}
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
	temp, err := os.CreateTemp("", "tasktrace-move-image-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	if _, err = io.Copy(temp, attachment.File.File); err != nil {
		return "", err
	}
	if _, err = temp.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	newFile, err := files.CreateWithMimeAndSession(s, temp, attachment.File.Name, attachment.File.Size, a, attachment.File.Mime, true)
	if newFile != nil {
		m.createdFiles = append(m.createdFiles, newFile.ID)
	}
	if err != nil {
		return "", err
	}
	doer, err := GetUserOrLinkShareUser(s, a)
	if err != nil {
		return "", err
	}
	copied := &TaskAttachment{TaskID: m.TargetTaskID, FileID: newFile.ID, File: newFile, CreatedByID: doer.ID, CreatedBy: doer}
	if _, err = s.Insert(copied); err != nil {
		return "", err
	}
	events.DispatchOnCommit(s, &TaskAttachmentCreatedEvent{Task: &m.target, Attachment: copied, Doer: doer})
	return fmt.Sprintf("/api/v1/tasks/%d/attachments/%d", m.TargetTaskID, copied.ID), nil
}

// CleanupCreatedFiles must be called after a failed transaction, never after commit.
func (m *TaskTraceOutstandingMove) CleanupCreatedFiles() {
	for _, id := range m.createdFiles {
		if err := files.DeleteBlob(id); err != nil {
			log.Errorf("Could not clean up TaskTrace move image %d: %s", id, err)
		}
	}
	m.createdFiles = nil
}
