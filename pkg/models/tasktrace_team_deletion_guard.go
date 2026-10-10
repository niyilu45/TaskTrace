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

	"golang.org/x/net/html"
)

// Old clients can publish an incomplete list (and advance manifest.Base to it).
// Missing data is not a deletion command. Accept an explicit item operation or
// a move to another task, otherwise leave the saved local item for resolution.
// The wire schema remains compatible with clients that do not know this guard.
func taskTraceTeamHasOutstandingRemoval(snapshots []TaskTraceTeamSnapshot, manifest *TaskTraceTeamManifest, node, id, requiredResolution string) bool {
	field := "outstanding:" + id
	for _, snapshot := range snapshots {
		if requiredResolution != "" && snapshot.ResolutionAcks[node+":"+field] != requiredResolution {
			continue
		}
		if !taskTraceTeamCan(manifest, node, id, snapshot.Actor, true) {
			continue
		}
		task, exists := taskTraceTeamTaskMap(snapshot)[node]
		if !exists || taskTraceTeamFieldValue(task, field) != "" {
			continue
		}
		for _, comment := range task.Comments {
			if !comment.Deleted && taskTraceOutstandingDeletionActivity(comment.Body, id) {
				return true
			}
		}
		// Moves preserve the item ID. The source must be absent and the
		// destination writable; a different task disappearing proves nothing.
		for _, destination := range snapshot.Tasks {
			if destination.NodeID != node && taskTraceTeamFieldValue(destination, field) != "" &&
				taskTraceTeamCan(manifest, destination.NodeID, id, snapshot.Actor, true) {
				return true
			}
		}
	}
	return false
}

//nolint:gosmopolitan // Also recognize the exact activity label emitted by older clients.
func taskTraceOutstandingDeletionActivity(body, id string) bool {
	if !strings.Contains(body, taskTraceOutstandingActivityType) {
		return false
	}
	doc, _ := html.Parse(strings.NewReader(body))
	found := false
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || taskTraceAttribute(n, taskTraceOutstandingTypeAttribute) != taskTraceOutstandingActivityType || taskTraceAttribute(n, "data-item-id") != id {
			return
		}
		if action := taskTraceAttribute(n, "data-tasktrace-item-action"); action != "" {
			found = found || action == "delete"
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && child.Data == "strong" && strings.TrimSpace(taskTraceText(child)) == "删除遗留事项" {
				found = true
			}
		}
	})
	return found
}

//nolint:gosmopolitan // Conflict choices are displayed in the Chinese local UI.
func taskTraceTeamUnprovenRemovalOptions(snapshots []TaskTraceTeamSnapshot, manifest *TaskTraceTeamManifest, node, field, requiredResolution string, local TaskTraceTeamTask) []TaskTraceTeamConflictOption {
	if !strings.HasPrefix(field, "outstanding:") {
		return nil
	}
	localValue := taskTraceTeamCanonicalFieldValue(local, field, taskTraceTeamFieldValue(local, field))
	if localValue == "" || taskTraceTeamHasOutstandingRemoval(snapshots, manifest, node, strings.TrimPrefix(field, "outstanding:"), requiredResolution) {
		return nil
	}
	return []TaskTraceTeamConflictOption{
		{Author: "保留本机已保存内容", Value: localValue},
		{Author: "其他设备清单中缺失（未找到明确删除记录）", Value: ""},
	}
}
