// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

// Missing access is a read-only status calculation. The UI submits an explicit
// grant operation, preserving the separate confirmations for creating an SMB
// share and temporarily elevating Windows rights.
func taskTraceTeamMissingRepositoryAccess(existing, required []TaskTraceTeamRepositoryMember) []TaskTraceTeamRepositoryMember {
	available := map[string]TaskTraceTeamRepositoryMember{}
	for _, member := range existing {
		filtered := taskTraceTeamFilterRepositoryMembers([]TaskTraceTeamRepositoryMember{member})
		if len(filtered) == 0 {
			continue
		}
		member = filtered[0]
		key := strings.ToLower(taskTraceTeamMembershipName(member.AccountName))
		if previous, ok := available[key]; !ok || previous.Access != TaskTraceTeamAccessWrite {
			available[key] = member
		}
	}
	needed := map[string]TaskTraceTeamRepositoryMember{}
	for _, member := range required {
		filtered := taskTraceTeamFilterRepositoryMembers([]TaskTraceTeamRepositoryMember{member})
		if len(filtered) == 0 {
			continue
		}
		member = filtered[0]
		key := strings.ToLower(taskTraceTeamMembershipName(member.AccountName))
		if previous, ok := needed[key]; !ok || previous.Access != TaskTraceTeamAccessWrite {
			needed[key] = member
		}
	}
	result := []TaskTraceTeamRepositoryMember{}
	for key, member := range needed {
		if current, ok := available[key]; ok {
			if current.Access == TaskTraceTeamAccessWrite || member.Access == TaskTraceTeamAccessRead {
				continue
			}
			member.AccountName = current.AccountName
		}
		result = append(result, member)
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].AccountName) < strings.ToLower(result[j].AccountName)
	})
	return result
}

func taskTraceTeamManifestRequiredAccess(manifest *TaskTraceTeamManifest, current string) []TaskTraceTeamRepositoryMember {
	if manifest == nil {
		return nil
	}
	nodes := map[string]bool{}
	for node := range manifest.Base {
		nodes[node] = true
	}
	for key := range manifest.Permissions {
		nodes[strings.SplitN(key, taskTraceTeamOutstandingPermissionSeparator, 2)[0]] = true
	}
	if manifest.RootNode != "" {
		nodes[manifest.RootNode] = true
	}
	result := []TaskTraceTeamRepositoryMember{}
	for _, member := range taskTraceTeamNormalizeMembers(manifest.Members, manifest.Owner) {
		if taskTraceTeamMembersEqual(member, current) {
			continue
		}
		access := ""
		if len(manifest.Permissions) == 0 {
			// Legacy manifests without item-level permissions default their
			// ordinary members to read access, matching taskTraceTeamCan.
			access = TaskTraceTeamAccessRead
		}
		for node := range nodes {
			if taskTraceTeamCan(manifest, node, "", member, true) {
				access = TaskTraceTeamAccessWrite
				break
			}
			if taskTraceTeamCan(manifest, node, "", member, false) {
				access = TaskTraceTeamAccessRead
			}
		}
		// An individually writable outstanding item also raises the repository
		// floor even when its task itself is read-only for this member.
		for key, permissions := range manifest.Permissions {
			if !strings.Contains(key, taskTraceTeamOutstandingPermissionSeparator) {
				continue
			}
			for _, permission := range permissions {
				if taskTraceTeamMembersEqual(permission.Username, member) {
					if permission.Write {
						access = TaskTraceTeamAccessWrite
					} else if permission.Read && access == "" {
						access = TaskTraceTeamAccessRead
					}
				}
			}
		}
		if access != "" {
			result = append(result, TaskTraceTeamRepositoryMember{AccountName: member, Access: access})
		}
	}
	return result
}

func taskTraceTeamEnsureRepositoryAccess(root string, required []TaskTraceTeamRepositoryMember, options TaskTraceTeamAccessOptions) ([]string, error) {
	if len(required) == 0 {
		return []string{}, nil
	}
	taskTraceTeamMemberAccessMu.Lock()
	defer taskTraceTeamMemberAccessMu.Unlock()
	existing, _ := taskTraceTeamListWindowsAccess(root)
	missing := taskTraceTeamMissingRepositoryAccess(existing, required)
	resolved := map[string]string{}
	for _, member := range existing {
		resolved[strings.ToLower(taskTraceTeamMembershipName(member.AccountName))] = member.AccountName
	}
	for _, member := range missing {
		account, err := taskTraceTeamSetWindowsAccessWithOptions(root, member.AccountName, member.Access, options)
		if err != nil {
			return nil, fmt.Errorf("prepare teamData access for %s: %w", member.AccountName, err)
		}
		resolved[strings.ToLower(taskTraceTeamMembershipName(account))] = account
	}
	if len(missing) > 0 {
		// Populate the post-mutation cache while still outside the global state
		// lock, so status immediately includes all granted members and the UNC.
		if _, err := taskTraceTeamListWindowsAccess(root); err != nil {
			//nolint:gosmopolitan // This actionable error is displayed by the local Chinese UI.
			return nil, fmt.Errorf("权限已设置，但读取共享信息失败，请重试：%w", err)
		}
	}
	members := make([]string, 0, len(required))
	for _, member := range required {
		account := resolved[strings.ToLower(taskTraceTeamMembershipName(member.AccountName))]
		if account == "" {
			account = member.AccountName
		}
		members = append(members, account)
	}
	return members, nil
}

func taskTraceTeamPrepareShareMembers(s *xorm.Session, a web.Auth, request TaskTraceTeamShareRequest) ([]string, error) {
	if !taskTraceTeamEnabled() {
		return nil, errors.New("TaskTrace team sync is unavailable")
	}
	u, err := user.GetFromAuth(a)
	if err != nil {
		return nil, err
	}
	can, err := (&Task{ID: request.TaskID}).CanWrite(s, a)
	if err != nil {
		return nil, err
	}
	if !can {
		return nil, ErrGenericForbidden{}
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return nil, err
	}
	members := taskTraceTeamNormalizeMembers(request.Members, u.Username)
	if len(members) < 2 {
		return nil, errors.New("at least one other team member is required")
	}
	root := taskTraceTeamRoot()
	var currentBinding *TaskTraceTeamBinding
	var manifest TaskTraceTeamManifest
	for index := range state.Bindings {
		binding := &state.Bindings[index]
		if binding.RootTaskID != request.TaskID {
			continue
		}
		if !taskTraceTeamMembersEqual(binding.Owner, u.Username) {
			return nil, ErrGenericForbidden{}
		}
		root = binding.Repository
		currentBinding = binding
		if err := taskTraceTeamReadManifest(filepath.Join(taskTraceTeamShareDir(root, binding.ShareID), "manifest.json"), &manifest); err != nil {
			return nil, err
		}
		break
	}
	required, err := taskTraceTeamShareRequiredAccess(currentBinding, &manifest, members, u.Username)
	if err != nil {
		return nil, err
	}
	resolved, err := taskTraceTeamEnsureRepositoryAccess(root, required, TaskTraceTeamAccessOptions{CreateShare: request.CreateShare, Elevate: request.Elevate})
	if err != nil {
		return nil, err
	}
	// An existing member with no access to any target still belongs to the team.
	// Omitting their Windows grant must not remove them from the manifest.
	result := []string{}
	for _, member := range members {
		if taskTraceTeamMembersEqual(member, u.Username) {
			continue
		}
		account := member
		for _, canonical := range resolved {
			if taskTraceTeamMembersEqual(member, canonical) {
				account = canonical
				break
			}
		}
		result = append(result, account)
	}
	return result, nil
}

func taskTraceTeamShareRequiredAccess(binding *TaskTraceTeamBinding, manifest *TaskTraceTeamManifest, members []string, actor string) ([]TaskTraceTeamRepositoryMember, error) {
	floors := map[string]string{}
	if binding != nil {
		for _, permissions := range manifest.Permissions {
			for _, permission := range permissions {
				if permission.Assignee && !taskTraceTeamContainsMember(members, permission.Username) {
					return nil, fmt.Errorf("cannot remove assignee %s from collaboration members", permission.Username)
				}
			}
		}
		for _, member := range taskTraceTeamManifestRequiredAccess(manifest, actor) {
			floors[strings.ToLower(taskTraceTeamMembershipName(member.AccountName))] = member.Access
		}
	}
	required := []TaskTraceTeamRepositoryMember{}
	for _, member := range members {
		if taskTraceTeamMembersEqual(member, actor) {
			continue
		}
		access := TaskTraceTeamAccessWrite
		if binding != nil && taskTraceTeamContainsMember(binding.Members, member) {
			access = floors[strings.ToLower(taskTraceTeamMembershipName(member))]
			if access == "" {
				continue
			}
		}
		required = append(required, TaskTraceTeamRepositoryMember{AccountName: member, Access: access})
	}
	return required, nil
}

func taskTraceTeamPreparePermissionAccess(a web.Auth, request TaskTraceTeamPermissionsRequest) error {
	u, err := user.GetFromAuth(a)
	if err != nil {
		return err
	}
	state, err := taskTraceTeamLoadState()
	if err != nil {
		return err
	}
	binding := taskTraceTeamFindBinding(&state, request.ShareID)
	if binding == nil {
		return errors.New("team task does not exist")
	}
	nodeID := taskTraceTeamNodeForTask(binding, request.TaskID)
	if nodeID == "" {
		return errors.New("task does not belong to this collaboration")
	}
	var manifest TaskTraceTeamManifest
	if err := taskTraceTeamReadManifest(filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "manifest.json"), &manifest); err != nil {
		return err
	}
	existing := manifest.Permissions[taskTraceTeamPermissionKey(nodeID, request.OutstandingID)]
	if request.OutstandingID != "" && len(existing) == 0 {
		existing = manifest.Permissions[nodeID]
	}
	if !taskTraceTeamMembersEqual(binding.Owner, u.Username) && !taskTraceTeamMembersEqual(taskTraceTeamPermissionOwner(existing, manifest.Owner), u.Username) {
		return ErrGenericForbidden{}
	}
	if request.OutstandingID != "" {
		items, _ := taskTraceTeamOutstandingItems(binding.Base[nodeID].Outstanding)
		if _, exists := items[request.OutstandingID]; !exists {
			return errors.New("outstanding item does not exist")
		}
	}
	if len(request.Permissions) == 0 && request.Assignees == nil {
		return errors.New("permissions or assignees are required")
	}
	// The Windows share lives on its host. A member managing a child of an
	// imported remote share must not try to create an SMB share for a UNC path.
	if strings.HasPrefix(binding.Repository, `\\`) {
		return nil
	}
	required := []TaskTraceTeamRepositoryMember{}
	for _, permission := range request.Permissions {
		if !taskTraceTeamContainsMember(manifest.Members, permission.Username) {
			return errors.New("permission member does not belong to this collaboration")
		}
		if permission.Read || permission.Write {
			access := TaskTraceTeamAccessRead
			if permission.Write {
				access = TaskTraceTeamAccessWrite
			}
			required = append(required, TaskTraceTeamRepositoryMember{AccountName: permission.Username, Access: access})
		}
	}
	if request.Assignees != nil {
		for _, assignee := range *request.Assignees {
			if request.OutstandingID != "" || !taskTraceTeamContainsMember(manifest.Members, assignee) {
				return errors.New("assignee does not belong to this collaboration")
			}
			required = append(required, TaskTraceTeamRepositoryMember{AccountName: assignee, Access: TaskTraceTeamAccessWrite})
		}
	}
	required = taskTraceTeamMissingRepositoryAccess([]TaskTraceTeamRepositoryMember{{AccountName: u.Username, Access: TaskTraceTeamAccessWrite}}, required)
	_, err = taskTraceTeamEnsureRepositoryAccess(binding.Repository, required, TaskTraceTeamAccessOptions{CreateShare: request.CreateShare, Elevate: request.Elevate})
	return err
}
