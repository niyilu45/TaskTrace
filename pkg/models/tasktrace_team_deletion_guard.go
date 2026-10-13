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
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// Old clients can publish an incomplete list (and advance manifest.Base to it).
// Missing data is not a deletion command. Accept an explicit item operation or
// a move to another task, otherwise retain the saved item without manufacturing a conflict.
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

// taskTraceTeamFindMergeField distinguishes missing peer data from an actual
// deletion. Old peers may omit an item, including from the shared baseline.
// Only an authorized delete/move operation can contribute an empty version.
func taskTraceTeamFindMergeField(snapshots []TaskTraceTeamSnapshot, manifest *TaskTraceTeamManifest, node, field, base, requiredResolution string, local TaskTraceTeamTask) (string, []TaskTraceTeamConflictOption, bool) {
	if !strings.HasPrefix(field, "outstanding:") {
		return taskTraceTeamFindField(snapshots, node, field, base, requiredResolution, local)
	}
	id := strings.TrimPrefix(field, "outstanding:")
	candidates := make([]TaskTraceTeamSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if requiredResolution != "" && snapshot.ResolutionAcks[node+":"+field] != requiredResolution {
			continue
		}
		task, exists := taskTraceTeamTaskMap(snapshot)[node]
		if !exists {
			continue
		}
		if taskTraceTeamFieldValue(task, field) == "" && !taskTraceTeamHasOutstandingRemoval([]TaskTraceTeamSnapshot{snapshot}, manifest, node, id, requiredResolution) {
			continue
		}
		candidates = append(candidates, snapshot)
	}
	value, options, conflict := taskTraceTeamFindField(candidates, node, field, base, requiredResolution, local)
	if taskTraceTeamHasOutstandingRemoval(candidates, manifest, node, id, requiredResolution) {
		return value, options, conflict
	}
	if conflict {
		nonempty := make([]TaskTraceTeamConflictOption, 0, len(options))
		for _, option := range options {
			if option.Value != "" {
				nonempty = append(nonempty, option)
			}
		}
		if len(nonempty) == 1 {
			return nonempty[0].Value, nil, false
		}
		if len(nonempty) > 1 {
			return value, nonempty, true
		}
	}
	if value != "" {
		return value, nil, false
	}
	// A legacy writer can erase manifest.Base after this client acknowledged
	// an addition. Recover the unchanged, saved values instead of treating the
	// missing baseline as a new deletion or asking the user to choose emptiness.
	byValue := map[string][]string{}
	for _, snapshot := range candidates {
		task := taskTraceTeamTaskMap(snapshot)[node]
		stored := taskTraceTeamCanonicalFieldValue(task, field, taskTraceTeamFieldValue(task, field))
		if stored != "" {
			byValue[stored] = append(byValue[stored], snapshot.Actor)
		}
	}
	options = nil
	for stored, authors := range byValue {
		if len(byValue) == 1 {
			return stored, nil, false
		}
		sort.Strings(authors)
		options = append(options, TaskTraceTeamConflictOption{Author: strings.Join(authors, "、"), Value: stored})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Author < options[j].Author })
	return value, options, len(options) > 1
}
