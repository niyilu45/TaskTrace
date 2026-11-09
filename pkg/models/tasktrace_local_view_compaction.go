// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

// Keep removed views as sync sources until the repository confirms their
// data was written. A pruned view keeps only local identity and placement.
type taskTraceRemovedViews map[string]*Task

func taskTraceTeamRemovedLocalViews(s *xorm.Session, binding *TaskTraceTeamBinding) (taskTraceRemovedViews, error) {
	result := taskTraceRemovedViews{}
	if len(binding.NodeTasks) == 0 {
		return result, nil
	}
	ids := make([]int64, 0, len(binding.NodeTasks))
	for _, id := range binding.NodeTasks {
		ids = append(ids, id)
	}
	rows := map[int64]*Task{}
	if err := s.In("id", ids).Where("task_trace_hidden_by <> 0").
		Cols("id", "task_trace_hidden_by", "task_trace_view_pruned").Find(&rows); err != nil {
		return nil, err
	}
	for node, id := range binding.NodeTasks {
		if task := rows[id]; task != nil {
			result[node] = task
		}
	}
	return result, nil
}

func (views taskTraceRemovedViews) prunedNodes() map[string]bool {
	result := map[string]bool{}
	for node, task := range views {
		if task.TaskTraceViewPruned {
			result[node] = true
		}
	}
	return result
}

func taskTraceTeamReadPublishedView(binding *TaskTraceTeamBinding, actor, device string) (*TaskTraceTeamSnapshot, error) {
	var snapshot TaskTraceTeamSnapshot
	path := taskTraceTeamSnapshotPath(binding, actor, device)
	if err := taskTraceTeamReadJSON(path, &snapshot); err != nil {
		return nil, fmt.Errorf("read published collaboration data: %w", err)
	}
	if snapshot.Schema != taskTraceTeamSchema || snapshot.ShareID != binding.ShareID ||
		snapshot.DeviceID != device || !taskTraceTeamMembersEqual(snapshot.Actor, actor) {
		return nil, errors.New("published collaboration data does not match this device")
	}
	return &snapshot, nil
}

func taskTraceKeepRemoteViewRows(nodes map[string]bool, target, published *TaskTraceTeamSnapshot) *TaskTraceTeamSnapshot {
	if target == nil {
		return published
	}
	next := *target
	next.Tasks = slices.DeleteFunc(slices.Clone(target.Tasks), func(task TaskTraceTeamTask) bool { return nodes[task.NodeID] })
	next.Base = taskTraceTeamCopyBase(target.Base)
	for node := range nodes {
		delete(next.Base, node)
	}
	if published == nil {
		return &next
	}
	for _, task := range published.Tasks {
		if nodes[task.NodeID] {
			next.Tasks = append(next.Tasks, task)
			if base, ok := published.Base[task.NodeID]; ok {
				next.Base[task.NodeID] = base
			}
		}
	}
	return &next
}

func (views taskTraceRemovedViews) unpublishedNodes(raw, protected TaskTraceTeamSnapshot, manifest *TaskTraceTeamManifest, previous *TaskTraceTeamSnapshot) map[string]bool {
	blocked := map[string]bool{}
	if len(views) == 0 {
		return blocked
	}
	published := taskTraceTeamTaskMap(protected)
	for _, task := range raw.Tasks {
		if view := views[task.NodeID]; view == nil || view.TaskTraceViewPruned {
			continue
		}
		next, ok := published[task.NodeID]
		// A lost write grant must not cause the local edit to be replaced
		// by the last published version before it can be sent.
		if !ok || !taskTraceTeamCan(manifest, task.NodeID, "", raw.Actor, false) ||
			(previous == nil && !taskTraceTeamCan(manifest, task.NodeID, "", raw.Actor, true)) ||
			!taskTraceSamePublishedTask(task, next) {
			blocked[task.NodeID] = true
		}
	}
	return blocked
}

func taskTraceSamePublishedTask(a, b TaskTraceTeamTask) bool {
	if a.Title != b.Title || a.Status != b.Status || a.Done != b.Done ||
		taskTraceCompletionValue(a.PercentDone) != taskTraceCompletionValue(b.PercentDone) ||
		taskTraceTeamCanonicalHTML(a.Description, a.Attachments) != taskTraceTeamCanonicalHTML(b.Description, b.Attachments) {
		return false
	}
	left, _ := taskTraceTeamOutstandingItems(taskTraceTeamCanonicalHTML(a.Outstanding, a.Attachments))
	right, _ := taskTraceTeamOutstandingItems(taskTraceTeamCanonicalHTML(b.Outstanding, b.Attachments))
	return maps.Equal(left, right) && maps.Equal(taskTracePublishedCommentBodies(a), taskTracePublishedCommentBodies(b))
}

func taskTracePublishedCommentBodies(task TaskTraceTeamTask) map[string]string {
	result := map[string]string{}
	for _, comment := range task.Comments {
		if !comment.Deleted {
			result[comment.ID] = taskTraceTeamCanonicalHTML(taskTraceTeamCommentContent(comment.Body), task.Attachments)
		}
	}
	return result
}

func (views taskTraceRemovedViews) prunePublished(s *xorm.Session, state *taskTraceTeamState, binding *TaskTraceTeamBinding, snapshot TaskTraceTeamSnapshot, pending map[string]bool) error {
	for _, conflict := range binding.Conflicts {
		pending[conflict.NodeID] = true
	}
	for _, shared := range snapshot.Tasks {
		node := shared.NodeID
		task := views[node]
		if task == nil || pending[node] || taskTraceTeamLocalViewOverlaps(state, binding, task.ID) {
			continue
		}
		if !task.TaskTraceViewPruned {
			if err := taskTracePruneLocalTask(s, task.ID); err != nil {
				return err
			}
		}
		delete(binding.Base, node)
	}
	return nil
}

func taskTraceTeamLocalViewOverlaps(state *taskTraceTeamState, current *TaskTraceTeamBinding, id int64) bool {
	for i := range state.Bindings {
		binding := &state.Bindings[i]
		if binding.ShareID != current.ShareID && taskTraceTeamNodeForTask(binding, id) != "" {
			return true
		}
	}
	return false
}

func taskTracePruneLocalTask(s *xorm.Session, id int64) error {
	var comments []int64
	if err := s.Table("task_comments").Where("task_id = ?", id).Cols("id").Find(&comments); err != nil {
		return err
	}
	if len(comments) > 0 {
		if _, err := s.In("entity_id", comments).Where("entity_kind = ?", ReactionKindComment).Delete(&Reaction{}); err != nil {
			return err
		}
		if _, err := s.Where("task_id = ?", id).Delete(&TaskComment{}); err != nil {
			return err
		}
	}
	_, err := s.ID(id).Where("task_trace_hidden_by <> 0").NoAutoTime().
		Cols("title", "description", "done", "status", "percent_done", "cover_image_attachment_id", "task_trace_view_pruned").
		Update(&Task{TaskTraceViewPruned: true})
	return err
}

func taskTraceHydrateLocalViews(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, actor, device string, roots []int64) error {
	views, err := taskTraceTeamRemovedLocalViews(s, binding)
	if err != nil {
		return err
	}
	selected := map[string]*Task{}
	for node, view := range views {
		if view.TaskTraceViewPruned && slices.Contains(roots, view.TaskTraceHiddenBy) {
			selected[node] = view
		}
	}
	if len(selected) == 0 {
		return nil
	}
	snapshot, err := taskTraceTeamReadPublishedView(binding, actor, device)
	if err != nil {
		return err
	}
	var manifest TaskTraceTeamManifest
	if err := taskTraceTeamReadManifest(filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json"), &manifest); err != nil {
		return err
	}
	byNode := taskTraceTeamTaskMap(*snapshot)
	for node, view := range selected {
		if !taskTraceTeamCan(&manifest, node, "", actor, false) {
			return ErrGenericForbidden{}
		}
		task, ok := byNode[node]
		if !ok {
			return errors.New("the shared repository no longer has the removed task")
		}
		if err := taskTraceRestorePublishedTask(s, a, binding, view.ID, actor, task); err != nil {
			return err
		}
		if base, ok := snapshot.Base[node]; ok {
			if binding.Base == nil {
				binding.Base = map[string]TaskTraceTeamBase{}
			}
			binding.Base[node] = base
		}
	}
	return nil
}

func taskTraceRestorePublishedTask(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, id int64, actor string, task TaskTraceTeamTask) error {
	for key, localID := range binding.LocalAttachments {
		if !strings.HasPrefix(key, task.NodeID+":") {
			continue
		}
		found, err := s.ID(localID).Where("task_id = ?", id).Exist(&TaskAttachment{})
		if err != nil {
			return err
		}
		if !found {
			delete(binding.LocalAttachments, key)
		}
	}
	if err := taskTraceTeamEnsureAttachments(s, a, binding, []TaskTraceTeamSnapshot{{Tasks: []TaskTraceTeamTask{task}}}); err != nil {
		return err
	}
	rewrite := func(body string) string { return taskTraceTeamRewriteAttachments(body, id, task.Attachments, binding) }
	if err := taskTraceTeamApplyTaskFields(s, a, id, task.Title, rewrite(task.Description), task.Done, task.Status, taskTraceCompletionValue(task.PercentDone)); err != nil {
		return err
	}
	outstanding := taskTraceTeamApplyOutstandingPriorities(rewrite(task.Outstanding), taskTraceTeamRememberedOutstandingPriorities(binding, task.NodeID))
	if err := taskTraceTeamUpsertOutstanding(s, a, id, outstanding); err != nil {
		return err
	}
	task.Comments = slices.Clone(task.Comments)
	for i := range task.Comments {
		task.Comments[i].Body = rewrite(task.Comments[i].Body)
	}
	if err := taskTraceTeamMergeComments(s, a, binding, task.NodeID, actor, id, task.Comments); err != nil {
		return err
	}
	_, err := s.ID(id).NoAutoTime().Cols("task_trace_view_pruned").Update(&Task{})
	return err
}

func taskTraceDropPrunedCheckpointRows(nodes map[string]bool, journal *taskTraceTeamSyncJournal) {
	if len(nodes) == 0 {
		return
	}
	journal.Before.Base = taskTraceTeamCopyBase(journal.Before.Base)
	for node := range nodes {
		delete(journal.Before.Base, node)
	}
	if journal.Previous != nil {
		previous := *journal.Previous
		previous.Tasks = slices.DeleteFunc(slices.Clone(previous.Tasks), func(task TaskTraceTeamTask) bool { return nodes[task.NodeID] })
		previous.Base = taskTraceTeamCopyBase(previous.Base)
		for node := range nodes {
			delete(previous.Base, node)
		}
		journal.Previous = &previous
	}
}
