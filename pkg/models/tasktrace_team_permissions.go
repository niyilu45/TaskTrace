// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"

	"xorm.io/xorm"
)

const taskTraceTeamOutstandingPermissionSeparator = "/outstanding/"

type TaskTraceTeamMemberPermission struct {
	Username string `json:"username" doc:"The Windows username of this collaboration member."`
	Read     bool   `json:"read" doc:"Whether this member may read the shared task or outstanding item."`
	Write    bool   `json:"write" doc:"Whether this member may modify the shared task or outstanding item. Write access always includes read access."`
	Owner    bool   `json:"owner" readOnly:"true" doc:"Whether this member owns this shared item. Owners always have read and write access."`
	Assignee bool   `json:"assignee" readOnly:"true" doc:"Whether this member is assigned to complete this task. Assignees always have read and write access."`
}

type TaskTraceTeamPermissionTarget struct {
	NodeID        string                          `json:"node_id" readOnly:"true" doc:"The stable shared node id for this task."`
	TaskID        int64                           `json:"task_id" readOnly:"true" doc:"The local task id represented by this permission target."`
	OutstandingID string                          `json:"outstanding_id,omitempty" readOnly:"true" doc:"The stable outstanding-item id when this target represents an outstanding item."`
	Kind          string                          `json:"kind" readOnly:"true" enum:"task,outstanding" doc:"Whether this permission target is a task or an outstanding item."`
	Title         string                          `json:"title" readOnly:"true" doc:"The local display title for this permission target."`
	CanManage     bool                            `json:"can_manage" readOnly:"true" doc:"Whether the current user may configure this target's member permissions."`
	Permissions   []TaskTraceTeamMemberPermission `json:"permissions" readOnly:"true" doc:"The effective member permissions and protected owner or assignee roles."`
}

type TaskTraceTeamPermissionUpdate struct {
	Username string `json:"username" minLength:"1" doc:"The Windows username to configure."`
	Read     bool   `json:"read" doc:"Whether this member may read the target."`
	Write    bool   `json:"write" doc:"Whether this member may modify the target. Write access also enables read access."`
}

type TaskTraceTeamPermissionsRequest struct {
	ShareID       string                          `json:"share_id" minLength:"1" doc:"The collaboration share to update."`
	TaskID        int64                           `json:"task_id" minimum:"1" doc:"The local task whose permissions are being updated."`
	OutstandingID string                          `json:"outstanding_id,omitempty" doc:"The outstanding-item id to update. Leave empty to update the task itself."`
	Permissions   []TaskTraceTeamPermissionUpdate `json:"permissions,omitempty" doc:"The complete set of editable member read and write choices."`
	Assignees     *[]string                       `json:"assignees,omitempty" doc:"When supplied for a task, replaces the collaboration members assigned to complete it."`
}

func taskTraceTeamPermissionKey(nodeID, outstandingID string) string {
	if outstandingID == "" {
		return nodeID
	}
	return nodeID + taskTraceTeamOutstandingPermissionSeparator + outstandingID
}

func taskTraceTeamPermissionOwner(permissions []TaskTraceTeamMemberPermission, fallback string) string {
	for _, permission := range permissions {
		if permission.Owner {
			return permission.Username
		}
	}
	return fallback
}

func taskTraceTeamPermissionAssignees(permissions []TaskTraceTeamMemberPermission) []string {
	result := []string{}
	for _, permission := range permissions {
		if permission.Assignee {
			result = append(result, permission.Username)
		}
	}
	return result
}

func taskTraceTeamPermissionMap(permissions []TaskTraceTeamMemberPermission) map[string]TaskTraceTeamMemberPermission {
	result := make(map[string]TaskTraceTeamMemberPermission, len(permissions))
	for _, permission := range permissions {
		if username := taskTraceTeamMembershipName(permission.Username); username != "" {
			permission.Username = username
			if permission.Write {
				permission.Read = true
			}
			result[strings.ToLower(username)] = permission
		}
	}
	return result
}

func taskTraceTeamContainsMember(members []string, username string) bool {
	for _, member := range members {
		if taskTraceTeamMembersEqual(member, username) {
			return true
		}
	}
	return false
}

func taskTraceTeamNormalizePermissions(members []string, owner string, assignees []string, inherited []TaskTraceTeamMemberPermission) []TaskTraceTeamMemberPermission {
	values := taskTraceTeamPermissionMap(inherited)
	owner = taskTraceTeamMembershipName(owner)
	assigneeSet := map[string]bool{}
	for _, assignee := range assignees {
		if normalized := taskTraceTeamMembershipName(assignee); normalized != "" {
			assigneeSet[strings.ToLower(normalized)] = true
		}
	}
	allMembers := taskTraceTeamNormalizeMembers(append(append([]string{}, members...), assignees...), owner)
	result := make([]TaskTraceTeamMemberPermission, 0, len(allMembers))
	for _, member := range allMembers {
		key := strings.ToLower(member)
		permission, exists := values[key]
		if !exists {
			permission = TaskTraceTeamMemberPermission{Username: member, Read: true}
		}
		permission.Username = member
		permission.Owner = owner != "" && strings.EqualFold(member, owner)
		permission.Assignee = assigneeSet[key]
		if permission.Owner || permission.Assignee {
			permission.Read = true
			permission.Write = true
		} else if permission.Write {
			permission.Read = true
		}
		result = append(result, permission)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Owner != result[j].Owner {
			return result[i].Owner
		}
		if result[i].Assignee != result[j].Assignee {
			return result[i].Assignee
		}
		return strings.ToLower(result[i].Username) < strings.ToLower(result[j].Username)
	})
	return result
}

func taskTraceTeamCan(manifest *TaskTraceTeamManifest, nodeID, outstandingID, username string, write bool) bool {
	if manifest == nil || nodeID == "" || username == "" {
		return false
	}
	key := taskTraceTeamPermissionKey(nodeID, outstandingID)
	permissions := manifest.Permissions[key]
	if len(permissions) == 0 && outstandingID != "" {
		permissions = manifest.Permissions[nodeID]
	}
	if len(permissions) == 0 {
		for _, member := range manifest.Members {
			if strings.EqualFold(member, username) {
				if strings.EqualFold(manifest.Owner, username) {
					return true
				}
				return !write
			}
		}
		return false
	}
	for _, permission := range permissions {
		if strings.EqualFold(permission.Username, username) {
			if write {
				return permission.Write
			}
			return permission.Read || permission.Write
		}
	}
	return false
}

func taskTraceTeamReconcileManifestPermissions(s *xorm.Session, binding *TaskTraceTeamBinding, manifest *TaskTraceTeamManifest, snapshot TaskTraceTeamSnapshot, actor string) (bool, error) {
	if manifest.Permissions == nil {
		manifest.Permissions = map[string][]TaskTraceTeamMemberPermission{}
	}
	changed := false
	knownBindingMembers := append([]string{}, binding.Members...)
	previousMembers := append([]string{}, manifest.Members...)
	members := append([]string{}, previousMembers...)
	for _, shared := range snapshot.Tasks {
		existing := manifest.Permissions[taskTraceTeamPermissionKey(shared.NodeID, "")]
		mayChangeRoles := len(existing) > 0 && taskTraceTeamCan(manifest, shared.NodeID, "", actor, true)
		if len(existing) == 0 {
			mayChangeRoles = (shared.ParentNode == "" && strings.EqualFold(actor, manifest.Owner)) ||
				(shared.ParentNode != "" && taskTraceTeamCan(manifest, shared.ParentNode, "", actor, true))
		}
		if !mayChangeRoles {
			continue
		}
		members = append(members, shared.Assignees...)
		if shared.Owner != "" {
			members = append(members, shared.Owner)
		}
	}
	normalizedMembers := taskTraceTeamNormalizeMembers(members, manifest.Owner)
	if !reflect.DeepEqual(normalizedMembers, manifest.Members) {
		manifest.Members = normalizedMembers
		changed = true
	}
	binding.Members = normalizedMembers
	if strings.EqualFold(actor, manifest.Owner) && strings.EqualFold(strings.TrimRight(binding.Repository, `\/`), strings.TrimRight(taskTraceTeamRoot(), `\/`)) {
		for _, member := range manifest.Members {
			if strings.EqualFold(member, actor) || taskTraceTeamContainsMember(knownBindingMembers, member) {
				continue
			}
			if _, err := taskTraceTeamGrantWindowsAccess(binding.Repository, member); err != nil {
				return changed, fmt.Errorf("无法为受理人或团队成员 %s 设置 teamData 读写权限: %w", member, err)
			}
		}
	}
	for _, shared := range snapshot.Tasks {
		key := taskTraceTeamPermissionKey(shared.NodeID, "")
		existing, exists := manifest.Permissions[key]
		if !exists && ((shared.ParentNode == "" && !strings.EqualFold(actor, manifest.Owner)) ||
			(shared.ParentNode != "" && !taskTraceTeamCan(manifest, shared.ParentNode, "", actor, true))) {
			continue
		}
		owner := shared.Owner
		assignees := shared.Assignees
		if exists {
			owner = taskTraceTeamPermissionOwner(existing, owner)
			if !taskTraceTeamCan(manifest, shared.NodeID, "", actor, true) {
				assignees = taskTraceTeamPermissionAssignees(existing)
			}
		} else if shared.ParentNode == "" {
			owner = manifest.Owner
		}
		inherited := existing
		if !exists && shared.ParentNode != "" {
			inherited = manifest.Permissions[taskTraceTeamPermissionKey(shared.ParentNode, "")]
		}
		normalized := taskTraceTeamNormalizePermissions(manifest.Members, owner, assignees, inherited)
		if !reflect.DeepEqual(normalized, existing) {
			manifest.Permissions[key] = normalized
			changed = true
		}
		items, _ := taskTraceTeamOutstandingItems(shared.Outstanding)
		for outstandingID := range items {
			outstandingKey := taskTraceTeamPermissionKey(shared.NodeID, outstandingID)
			outstanding, exists := manifest.Permissions[outstandingKey]
			if !exists && !taskTraceTeamCan(manifest, shared.NodeID, "", actor, true) {
				continue
			}
			if !exists {
				outstanding = manifest.Permissions[key]
			}
			normalizedOutstanding := taskTraceTeamNormalizePermissions(manifest.Members, owner, assignees, outstanding)
			if !reflect.DeepEqual(normalizedOutstanding, manifest.Permissions[outstandingKey]) {
				manifest.Permissions[outstandingKey] = normalizedOutstanding
				changed = true
			}
		}
	}
	if changed {
		manifest.Updated = time.Now().UTC()
	}
	return changed, nil
}

func taskTraceTeamFilterSnapshotsPermissions(snapshots []TaskTraceTeamSnapshot, binding *TaskTraceTeamBinding, manifest *TaskTraceTeamManifest) []TaskTraceTeamSnapshot {
	filtered := make([]TaskTraceTeamSnapshot, 0, len(snapshots))
	for _, snapshot := range snapshots {
		tasks := make([]TaskTraceTeamTask, 0, len(snapshot.Tasks))
		for _, task := range snapshot.Tasks {
			if !taskTraceTeamCan(manifest, task.NodeID, "", snapshot.Actor, false) {
				continue
			}
			base, hasBase := binding.Base[task.NodeID]
			if !taskTraceTeamCan(manifest, task.NodeID, "", snapshot.Actor, true) {
				if !hasBase {
					continue
				}
				task.Title = base.Title
				task.Description = base.Description
				task.Done = base.Done
				task.Status = base.Status
				task.Comments = nil
			}
			items, order := taskTraceTeamOutstandingItems(task.Outstanding)
			baseItems, baseOrder := taskTraceTeamOutstandingItems(base.Outstanding)
			for id := range items {
				if taskTraceTeamCan(manifest, task.NodeID, id, snapshot.Actor, true) {
					continue
				}
				if value, ok := baseItems[id]; ok {
					items[id] = value
				} else {
					delete(items, id)
				}
			}
			for _, id := range baseOrder {
				if !taskTraceTeamCan(manifest, task.NodeID, id, snapshot.Actor, true) {
					if _, ok := items[id]; !ok {
						items[id] = baseItems[id]
						order = append(order, id)
					}
				}
			}
			task.Outstanding = taskTraceTeamOutstandingHTML(items, order)
			tasks = append(tasks, task)
		}
		snapshot.Tasks = tasks
		filtered = append(filtered, snapshot)
	}
	return filtered
}

func taskTraceTeamProtectSnapshotPermissions(current *TaskTraceTeamSnapshot, previous *TaskTraceTeamSnapshot, manifest *TaskTraceTeamManifest, actor string) {
	if current == nil || previous == nil {
		return
	}
	previousTasks := taskTraceTeamTaskMap(*previous)
	filtered := make([]TaskTraceTeamTask, 0, len(current.Tasks))
	for _, task := range current.Tasks {
		if !taskTraceTeamCan(manifest, task.NodeID, "", actor, false) {
			continue
		}
		old, hasOld := previousTasks[task.NodeID]
		if !taskTraceTeamCan(manifest, task.NodeID, "", actor, true) && hasOld {
			currentOutstanding := task.Outstanding
			currentAttachments := task.Attachments
			task = old
			task.Outstanding = taskTraceTeamProtectOutstandingChanges(currentOutstanding, old.Outstanding, task.NodeID, actor, manifest)
			task.Attachments = taskTraceTeamMergeAttachmentMetadata(old.Attachments, currentAttachments)
		} else if hasOld {
			task.Outstanding = taskTraceTeamProtectOutstandingChanges(task.Outstanding, old.Outstanding, task.NodeID, actor, manifest)
		}
		filtered = append(filtered, task)
	}
	current.Tasks = filtered
}

func taskTraceTeamProtectOutstandingChanges(currentHTML, oldHTML, nodeID, actor string, manifest *TaskTraceTeamManifest) string {
	currentItems, currentOrder := taskTraceTeamOutstandingItems(currentHTML)
	oldItems, oldOrder := taskTraceTeamOutstandingItems(oldHTML)
	result := make(map[string]string, len(oldItems)+len(currentItems))
	for id, value := range oldItems {
		result[id] = value
	}
	order := append([]string{}, oldOrder...)
	seen := map[string]bool{}
	for _, id := range order {
		seen[id] = true
	}
	for _, id := range currentOrder {
		if !taskTraceTeamCan(manifest, nodeID, id, actor, true) {
			continue
		}
		result[id] = currentItems[id]
		if !seen[id] {
			order = append(order, id)
			seen[id] = true
		}
	}
	for id := range oldItems {
		if taskTraceTeamCan(manifest, nodeID, id, actor, true) {
			if _, exists := currentItems[id]; !exists {
				delete(result, id)
			}
		}
	}
	return taskTraceTeamOutstandingHTML(result, order)
}

func taskTraceTeamMergeAttachmentMetadata(groups ...[]TaskTraceTeamAttachment) []TaskTraceTeamAttachment {
	byID := map[string]TaskTraceTeamAttachment{}
	for _, attachments := range groups {
		for _, attachment := range attachments {
			byID[attachment.ID] = attachment
		}
	}
	result := make([]TaskTraceTeamAttachment, 0, len(byID))
	for _, attachment := range byID {
		result = append(result, attachment)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func taskTraceTeamPermissionTargets(s *xorm.Session, binding *TaskTraceTeamBinding, manifest *TaskTraceTeamManifest, username string) []TaskTraceTeamPermissionTarget {
	result := []TaskTraceTeamPermissionTarget{}
	nodes := make([]string, 0, len(binding.NodeTasks))
	for node := range binding.NodeTasks {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		taskID := binding.NodeTasks[node]
		if !taskTraceTeamCan(manifest, node, "", username, false) {
			continue
		}
		title := ""
		if task, err := GetTaskByIDSimple(s, taskID); err == nil {
			title = task.Title
		}
		permissions := manifest.Permissions[taskTraceTeamPermissionKey(node, "")]
		if len(permissions) == 0 {
			permissions = taskTraceTeamNormalizePermissions(manifest.Members, manifest.Owner, nil, nil)
		}
		canManage := strings.EqualFold(manifest.Owner, username) || strings.EqualFold(taskTraceTeamPermissionOwner(permissions, manifest.Owner), username)
		result = append(result, TaskTraceTeamPermissionTarget{NodeID: node, TaskID: taskID, Kind: "task", Title: title, CanManage: canManage, Permissions: permissions})
		base := binding.Base[node]
		items, order := taskTraceTeamOutstandingItems(base.Outstanding)
		for index, outstandingID := range order {
			if !taskTraceTeamCan(manifest, node, outstandingID, username, false) {
				continue
			}
			itemTitle := strings.TrimSpace(taskTraceTeamStripHTML(items[outstandingID]))
			if itemTitle == "" {
				itemTitle = fmt.Sprintf("遗留事项 %d", index+1)
			}
			outstandingPermissions := manifest.Permissions[taskTraceTeamPermissionKey(node, outstandingID)]
			if len(outstandingPermissions) == 0 {
				outstandingPermissions = taskTraceTeamNormalizePermissions(manifest.Members, taskTraceTeamPermissionOwner(permissions, manifest.Owner), taskTraceTeamPermissionAssignees(permissions), permissions)
			}
			result = append(result, TaskTraceTeamPermissionTarget{
				NodeID: node, TaskID: taskID, OutstandingID: outstandingID, Kind: "outstanding", Title: itemTitle,
				CanManage: canManage, Permissions: outstandingPermissions,
			})
		}
	}
	return result
}

var taskTraceTeamHTMLTags = regexp.MustCompile(`<[^>]*>`)

func taskTraceTeamStripHTML(value string) string {
	return strings.Join(strings.Fields(taskTraceTeamHTMLTags.ReplaceAllString(value, " ")), " ")
}

func taskTraceTeamApplyPermissionUpdates(existing []TaskTraceTeamMemberPermission, updates map[string]TaskTraceTeamPermissionUpdate) []TaskTraceTeamMemberPermission {
	result := append([]TaskTraceTeamMemberPermission{}, existing...)
	for index := range result {
		if result[index].Owner || result[index].Assignee {
			continue
		}
		if update, ok := updates[strings.ToLower(result[index].Username)]; ok {
			result[index].Read = update.Read || update.Write
			result[index].Write = update.Write
		}
	}
	return result
}

func taskTraceTeamPermissionDescendants(s *xorm.Session, binding *TaskTraceTeamBinding, rootNode string) map[string]bool {
	result := map[string]bool{rootNode: true}
	ids, parents, err := taskTraceTeamSubtree(s, binding.RootTaskID)
	if err != nil {
		return result
	}
	changed := true
	for changed {
		changed = false
		for _, taskID := range ids {
			node := taskTraceTeamNodeForTask(binding, taskID)
			parentNode := taskTraceTeamNodeForTask(binding, parents[taskID])
			if node != "" && !result[node] && result[parentNode] {
				result[node] = true
				changed = true
			}
		}
	}
	return result
}

func TaskTraceTeamConfigurePermissions(s *xorm.Session, a web.Auth, request TaskTraceTeamPermissionsRequest) (*TaskTraceTeamStatus, error) {
	taskTraceTeamMu.Lock()
	defer taskTraceTeamMu.Unlock()
	u, err := user.GetFromAuth(a)
	if err != nil {
		return nil, err
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	binding := taskTraceTeamFindBinding(&state, request.ShareID)
	if binding == nil {
		return nil, errors.New("team task does not exist")
	}
	nodeID := taskTraceTeamNodeForTask(binding, request.TaskID)
	if nodeID == "" {
		return nil, errors.New("task does not belong to this collaboration")
	}
	var manifest TaskTraceTeamManifest
	manifestPath := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json")
	if err := taskTraceTeamReadJSON(manifestPath, &manifest); err != nil {
		return nil, err
	}
	key := taskTraceTeamPermissionKey(nodeID, request.OutstandingID)
	existing := manifest.Permissions[key]
	if request.OutstandingID != "" {
		base := binding.Base[nodeID]
		items, _ := taskTraceTeamOutstandingItems(base.Outstanding)
		if _, ok := items[request.OutstandingID]; !ok {
			return nil, errors.New("outstanding item does not exist")
		}
		if len(existing) == 0 {
			existing = manifest.Permissions[nodeID]
		}
	}
	if len(existing) == 0 {
		existing = taskTraceTeamNormalizePermissions(manifest.Members, manifest.Owner, nil, nil)
	}
	if !strings.EqualFold(binding.Owner, u.Username) && !strings.EqualFold(taskTraceTeamPermissionOwner(existing, manifest.Owner), u.Username) {
		return nil, ErrGenericForbidden{}
	}
	updates := map[string]TaskTraceTeamPermissionUpdate{}
	for _, permission := range request.Permissions {
		username := taskTraceTeamMembershipName(permission.Username)
		if username != "" {
			updates[strings.ToLower(username)] = permission
		}
	}
	if len(request.Permissions) == 0 && request.Assignees == nil {
		return nil, errors.New("permissions or assignees are required")
	}
	if request.Assignees != nil {
		if request.OutstandingID != "" {
			return nil, errors.New("outstanding items cannot have separate assignees")
		}
		assignees := []string{}
		for _, candidate := range *request.Assignees {
			username := taskTraceTeamMembershipName(candidate)
			if username == "" || !taskTraceTeamContainsMember(manifest.Members, username) {
				return nil, fmt.Errorf("协作成员 %q 不在共享人员名单中", candidate)
			}
			if !taskTraceTeamContainsMember(assignees, username) {
				assignees = append(assignees, username)
			}
		}
		owner := taskTraceTeamPermissionOwner(existing, manifest.Owner)
		manifest.Permissions[key] = taskTraceTeamNormalizePermissions(manifest.Members, owner, assignees, existing)
	} else {
		if request.OutstandingID != "" {
			owner := taskTraceTeamPermissionOwner(existing, manifest.Owner)
			assignees := taskTraceTeamPermissionAssignees(existing)
			manifest.Permissions[key] = taskTraceTeamNormalizePermissions(manifest.Members, owner, assignees, taskTraceTeamApplyPermissionUpdates(existing, updates))
		} else {
			// A task-level permission change is inherited by every existing child task
			// and outstanding item. Each target keeps its own owner and assignee roles.
			for targetNode := range taskTraceTeamPermissionDescendants(s, binding, nodeID) {
				targetKey := taskTraceTeamPermissionKey(targetNode, "")
				target := manifest.Permissions[targetKey]
				if len(target) == 0 {
					target = existing
				}
				owner := taskTraceTeamPermissionOwner(target, manifest.Owner)
				assignees := taskTraceTeamPermissionAssignees(target)
				manifest.Permissions[targetKey] = taskTraceTeamNormalizePermissions(manifest.Members, owner, assignees, taskTraceTeamApplyPermissionUpdates(target, updates))
				items, _ := taskTraceTeamOutstandingItems(binding.Base[targetNode].Outstanding)
				for outstandingID := range items {
					outstandingKey := taskTraceTeamPermissionKey(targetNode, outstandingID)
					outstanding := manifest.Permissions[outstandingKey]
					if len(outstanding) == 0 {
						outstanding = manifest.Permissions[targetKey]
					}
					manifest.Permissions[outstandingKey] = taskTraceTeamNormalizePermissions(manifest.Members, owner, assignees, taskTraceTeamApplyPermissionUpdates(outstanding, updates))
				}
			}
		}
	}
	manifest.Updated = time.Now().UTC()
	if err := taskTraceTeamWriteJSON(manifestPath, &manifest); err != nil {
		return nil, err
	}
	binding.Members = manifest.Members
	if err := taskTraceTeamSaveState(state); err != nil {
		return nil, err
	}
	status, err := taskTraceTeamStatusLocked(s, a, state)
	return &status, err
}
