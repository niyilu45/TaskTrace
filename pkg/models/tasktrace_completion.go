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
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// PercentDone is always the manually entered value. Done is a display overlay.
func taskTraceCompletionValue(value *float64) float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return 0
	}
	return math.Max(0, math.Min(1, *value))
}
func taskTraceCompletionPointer(value float64) *float64 {
	normalized := taskTraceCompletionValue(&value)
	return &normalized
}
func taskTraceCompletionString(value float64) string {
	return strconv.FormatFloat(taskTraceCompletionValue(&value), 'f', -1, 64)
}

// Older clients do not know data-progress. Missing is not an explicit reset to zero.
func taskTraceRetainOutstandingCompletion(value, baseline string) string {
	if value == "" || strings.Contains(value, "data-progress=") {
		return value
	}
	_, state := taskTraceTeamOutstandingState(baseline)
	doc, _ := html.Parse(strings.NewReader("<span" + state + "></span>"))
	progress := ""
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "span" {
			progress = taskTraceAttribute(n, "data-progress")
		}
	})
	if progress == "" {
		return value
	}
	content, oldState := taskTraceTeamOutstandingState(value)
	return `<span data-tasktrace-outstanding-state="true" hidden` + oldState + ` data-progress="` + html.EscapeString(progress) + `"></span>` + content
}
func taskTraceRetainOutstandingListCompletion(value, baseline string) string {
	items, order := taskTraceTeamOutstandingItems(value)
	previous, _ := taskTraceTeamOutstandingItems(baseline)
	changed := false
	for id, content := range items {
		retained := taskTraceRetainOutstandingCompletion(content, previous[id])
		if retained != content {
			items[id] = retained
			changed = true
		}
	}
	if !changed {
		return value
	}
	return taskTraceTeamOutstandingHTML(items, order)
}

func taskTraceOutstandingCompletionValue(value string) int {
	parsed, _ := strconv.ParseFloat(value, 64)
	return int(math.Round(taskTraceCompletionValue(taskTraceCompletionPointer(parsed/100)) * 100))
}

// This small projection is intentionally separate from manifest.json: old peers
// rewrite that file through structs which discard newly introduced fields.
// Manifest callers already hold the share lock when writing. Reads never write.
type taskTraceCompletionState struct {
	Schema    int                                `json:"schema"`
	ShareID   string                             `json:"share_id"`
	TokenHash string                             `json:"token_hash"`
	Nodes     map[string]taskTraceCompletionNode `json:"nodes"`
}
type taskTraceCompletionNode struct {
	PercentDone *float64       `json:"percent_done,omitempty"`
	Outstanding map[string]int `json:"outstanding,omitempty"`
}

func taskTraceCompletionStatePath(manifestPath string) string {
	return filepath.Join(filepath.Dir(manifestPath), "completion-state.json")
}
func taskTraceReadCompletionState(path string, manifest *TaskTraceTeamManifest) (taskTraceCompletionState, error) {
	state := taskTraceCompletionState{Schema: 1, ShareID: manifest.ShareID, TokenHash: manifest.TokenHash, Nodes: map[string]taskTraceCompletionNode{}}
	content, err := os.ReadFile(taskTraceCompletionStatePath(path))
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state = taskTraceCompletionState{}
	if err = json.Unmarshal(content, &state); err != nil {
		return state, err
	}
	if state.Schema != 1 || state.ShareID != manifest.ShareID || state.TokenHash != manifest.TokenHash {
		return state, errors.New("completion state does not match the shared task")
	}
	if state.Nodes == nil {
		state.Nodes = map[string]taskTraceCompletionNode{}
	}
	return state, nil
}
func taskTraceOutstandingCompletionAttribute(value string) (int, bool) {
	_, state := taskTraceTeamOutstandingState(value)
	doc, _ := html.Parse(strings.NewReader("<span" + state + "></span>"))
	var result int
	found := false
	taskTraceWalk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || n.Data != "span" {
			return
		}
		for _, attr := range n.Attr {
			if attr.Key == "data-progress" {
				result = taskTraceOutstandingCompletionValue(attr.Val)
				found = true
			}
		}
	})
	return result, found
}
func taskTraceTeamReadManifest(path string, manifest *TaskTraceTeamManifest) error {
	if err := taskTraceTeamReadJSON(path, manifest); err != nil {
		return err
	}
	state, err := taskTraceReadCompletionState(path, manifest)
	if err != nil {
		return err
	}
	for node, base := range manifest.Base {
		stored, exists := state.Nodes[node]
		if !exists {
			continue
		}
		if !base.CompletionKnown && stored.PercentDone != nil {
			base.PercentDone = taskTraceCompletionValue(stored.PercentDone)
			base.CompletionKnown = true
		}
		items, order := taskTraceTeamOutstandingItems(base.Outstanding)
		changed := false
		for id, value := range items {
			if progress, ok := stored.Outstanding[id]; ok {
				retained := taskTraceRetainOutstandingCompletion(value, `<span data-tasktrace-outstanding-state="true" hidden data-progress="`+strconv.Itoa(progress)+`"></span>`)
				if retained != value {
					items[id] = retained
					changed = true
				}
			}
		}
		if changed {
			base.Outstanding = taskTraceTeamOutstandingHTML(items, order)
		}
		manifest.Base[node] = base
	}
	return nil
}
func taskTraceTeamWriteManifest(path string, manifest *TaskTraceTeamManifest) error {
	state, err := taskTraceReadCompletionState(path, manifest)
	if err != nil {
		return err
	}
	changed := false
	for node, base := range manifest.Base {
		stored := state.Nodes[node]
		if base.CompletionKnown && (stored.PercentDone == nil || *stored.PercentDone != base.PercentDone) {
			stored.PercentDone = taskTraceCompletionPointer(base.PercentDone)
			changed = true
		}
		items, _ := taskTraceTeamOutstandingItems(base.Outstanding)
		for id, value := range items {
			progress, found := taskTraceOutstandingCompletionAttribute(value)
			if !found {
				continue
			}
			if stored.Outstanding == nil {
				stored.Outstanding = map[string]int{}
			}
			if previous, exists := stored.Outstanding[id]; !exists || previous != progress {
				stored.Outstanding[id] = progress
				changed = true
			}
		}
		state.Nodes[node] = stored
	}
	// Keep tombstoned IDs only as metadata; read hydration never creates rows.
	// Persist the projection first, so an old peer cannot erase newly published values.
	if changed {
		if err := taskTraceTeamWriteJSON(taskTraceCompletionStatePath(path), &state); err != nil {
			return err
		}
	}
	var previous TaskTraceTeamManifest
	if err := taskTraceTeamReadJSON(path, &previous); err == nil && reflect.DeepEqual(previous, *manifest) {
		return nil
	}
	return taskTraceTeamWriteJSON(path, manifest)
}
