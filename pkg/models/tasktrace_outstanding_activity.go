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

package models

import (
	"strings"

	"code.vikunja.io/api/pkg/user"

	"github.com/google/uuid"
	"golang.org/x/net/html"
	"xorm.io/xorm"
)

const taskTraceOutstandingActivityType = "item-activity"

// Activity rows are independent from the mutable canonical list. Only an
// authorized API mutation records them; replaying team snapshots must not.
func taskTraceOutstandingActivityItems(body string) []taskTraceOutstandingItem {
	if !taskTraceTeamIsOutstanding(body) {
		return nil
	}
	list, _ := taskTraceParseOutstanding([]*TaskComment{{ID: 1, Comment: body}})
	if list == nil {
		return nil
	}
	return list.items
}

func taskTraceOutstandingItemAttribute(item taskTraceOutstandingItem, name string) string {
	doc, _ := html.Parse(strings.NewReader("<ul><li" + item.metadata + "></li></ul>"))
	value := ""
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" {
			value = taskTraceAttribute(n, name)
		}
	})
	if name == "data-priority" && (len(value) != 1 || value[0] < '0' || value[0] > '9') {
		return "7"
	}
	return value
}

//nolint:gosmopolitan // Persisted activity labels follow the Chinese TaskTrace local UI.
func taskTraceOutstandingActivityLabel(item taskTraceOutstandingItem) string {
	doc, _ := html.Parse(strings.NewReader(item.content))
	var hidden []*html.Node
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "aside" || n.Data == "script" || n.Data == "style") {
			hidden = append(hidden, n)
		}
	})
	for _, n := range hidden {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	label := strings.TrimSpace(taskTraceText(doc))
	if label == "" {
		return "（图片遗留事项）"
	}
	return label
}

//nolint:gosmopolitan // Persisted activity labels follow the Chinese TaskTrace local UI.
func taskTraceRecordOutstandingActivity(s *xorm.Session, actor *user.User, taskID int64, before []taskTraceOutstandingItem, body string) error {
	after := taskTraceOutstandingActivityItems(body)
	previous := map[string]taskTraceOutstandingItem{}
	current := map[string]bool{}
	for _, item := range before {
		previous[item.id] = item
	}
	write := func(item taskTraceOutstandingItem, action string, local bool) error {
		attrs := ` data-item-id="` + html.EscapeString(item.id) + `"`
		if action == "删除" {
			attrs += ` data-tasktrace-item-action="delete"`
		}
		if local {
			attrs += ` data-tasktrace-local="true"`
		}
		content := `<p data-tasktrace-comment-type="` + taskTraceOutstandingActivityType + `"` + attrs + `><strong>` + action + `遗留事项</strong>：` + html.EscapeString(taskTraceOutstandingActivityLabel(item)) + `</p>`
		record := &TaskComment{TaskID: taskID, AuthorID: actor.ID,
			Comment: taskTraceTeamAddMarker(content, uuid.NewString(), actor.Username)}
		// The original mutation already dispatches the task/comment event. A second
		// event would duplicate notifications and real-time refresh work.
		_, err := s.Insert(record)
		return err
	}
	for _, item := range after {
		current[item.id] = true
		old, exists := previous[item.id]
		action, local := "", false
		switch {
		case !exists:
			action = "新增"
		case (taskTraceOutstandingItemAttribute(old, "data-done") == "true") != (taskTraceOutstandingItemAttribute(item, "data-done") == "true"):
			action = "完成"
			if taskTraceOutstandingItemAttribute(item, "data-done") != "true" {
				action = "重新打开"
			}
		case taskTraceTeamCanonicalHTML(old.content, nil) != taskTraceTeamCanonicalHTML(item.content, nil):
			action = "修改"
		case taskTraceOutstandingItemAttribute(old, "data-priority") != taskTraceOutstandingItemAttribute(item, "data-priority"):
			action, local = "修改优先级 · ", true
		}
		if action != "" {
			if err := write(item, action, local); err != nil {
				return err
			}
		}
	}
	for _, item := range before {
		if !current[item.id] {
			if err := write(item, "删除", false); err != nil {
				return err
			}
		}
	}
	return nil
}

func taskTraceOutstandingLocalActivity(body string) bool {
	if !strings.Contains(body, taskTraceOutstandingActivityType) {
		return false
	}
	doc, _ := html.Parse(strings.NewReader(body))
	local := false
	taskTraceWalk(doc, func(n *html.Node) {
		if taskTraceAttribute(n, taskTraceOutstandingTypeAttribute) == taskTraceOutstandingActivityType && taskTraceAttribute(n, "data-tasktrace-local") == "true" {
			local = true
		}
	})
	return local
}
