// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamPermissionsProtectSpecialRoles(t *testing.T) {
	permissions := taskTraceTeamNormalizePermissions(
		[]string{"owner", "assignee", "reader"},
		"owner",
		[]string{"assignee"},
		[]TaskTraceTeamMemberPermission{
			{Username: "owner"},
			{Username: "assignee"},
			{Username: "reader", Write: true},
		},
	)

	require.Len(t, permissions, 3)
	byUser := taskTraceTeamPermissionMap(permissions)
	require.True(t, byUser["owner"].Owner)
	require.True(t, byUser["owner"].Read)
	require.True(t, byUser["owner"].Write)
	require.True(t, byUser["assignee"].Assignee)
	require.True(t, byUser["assignee"].Read)
	require.True(t, byUser["assignee"].Write)
	require.True(t, byUser["reader"].Read)
	require.True(t, byUser["reader"].Write)
}

func TestTaskTraceTeamPermissionsDefaultMembersToReadOnly(t *testing.T) {
	permissions := taskTraceTeamNormalizePermissions([]string{"owner", "member"}, "owner", nil, nil)
	byUser := taskTraceTeamPermissionMap(permissions)

	require.True(t, byUser["owner"].Read)
	require.True(t, byUser["owner"].Write)
	require.True(t, byUser["member"].Read)
	require.False(t, byUser["member"].Write)
}

func TestTaskTraceTeamPermissionsChildAndOutstandingInheritIndependently(t *testing.T) {
	manifest := TaskTraceTeamManifest{
		Owner:   "owner",
		Members: []string{"owner", "member"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{
			"parent": {
				{Username: "owner", Read: true, Write: true, Owner: true},
				{Username: "member", Read: true, Write: true},
			},
		},
	}
	binding := TaskTraceTeamBinding{Members: manifest.Members}
	snapshot := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{
		{NodeID: "parent", Owner: "owner"},
		{NodeID: "child", ParentNode: "parent", Owner: "owner", Outstanding: `<ul><li data-id="pending">Follow up</li></ul>`},
	}}

	changed, err := taskTraceTeamReconcileManifestPermissions(nil, &binding, &manifest, snapshot, "member")
	require.NoError(t, err)
	require.True(t, changed)
	require.True(t, taskTraceTeamCan(&manifest, "child", "", "member", true))
	require.True(t, taskTraceTeamCan(&manifest, "child", "pending", "member", true))

	child := manifest.Permissions["child"]
	child[1].Write = false
	manifest.Permissions["child"] = child
	require.False(t, taskTraceTeamCan(&manifest, "child", "", "member", true))
	require.True(t, taskTraceTeamCan(&manifest, "child", "pending", "member", true), "outstanding item keeps its inherited copy")
}

func TestTaskTraceTeamPermissionsPreventReadOnlySnapshotWrites(t *testing.T) {
	manifest := TaskTraceTeamManifest{
		Members: []string{"owner", "reader"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{
			"node": {
				{Username: "owner", Read: true, Write: true, Owner: true},
				{Username: "reader", Read: true, Write: false},
			},
		},
	}
	previous := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{
		NodeID: "node", Title: "Original", Updated: time.Now().Add(-time.Hour),
	}}}
	current := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{
		NodeID: "node", Title: "Unauthorized edit", Updated: time.Now(),
	}}}

	taskTraceTeamProtectSnapshotPermissions(&current, &previous, &manifest, "reader")
	require.Len(t, current.Tasks, 1)
	require.Equal(t, "Original", current.Tasks[0].Title)
}

func TestTaskTraceTeamPermissionsReadOnlyMemberCannotBecomeAssigneeOrAddChild(t *testing.T) {
	manifest := TaskTraceTeamManifest{
		Owner:   "owner",
		Members: []string{"owner", "reader"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{
			"root": {
				{Username: "owner", Read: true, Write: true, Owner: true},
				{Username: "reader", Read: true},
			},
		},
	}
	binding := TaskTraceTeamBinding{Members: manifest.Members}
	snapshot := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{
		{NodeID: "root", Owner: "owner", Assignees: []string{"reader"}},
		{NodeID: "unauthorized-child", ParentNode: "root", Owner: "reader"},
	}}

	_, err := taskTraceTeamReconcileManifestPermissions(nil, &binding, &manifest, snapshot, "reader")
	require.NoError(t, err)
	require.False(t, taskTraceTeamCan(&manifest, "root", "", "reader", true))
	require.Empty(t, manifest.Permissions["unauthorized-child"])
	root := taskTraceTeamPermissionMap(manifest.Permissions["root"])
	require.False(t, root["reader"].Assignee)
}

func TestTaskTraceTeamPermissionsIgnoreReadOnlyRemoteTaskFields(t *testing.T) {
	manifest := TaskTraceTeamManifest{
		Owner:   "owner",
		Members: []string{"owner", "reader"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{
			"node": {
				{Username: "owner", Read: true, Write: true, Owner: true},
				{Username: "reader", Read: true},
			},
		},
	}
	binding := TaskTraceTeamBinding{Base: map[string]TaskTraceTeamBase{
		"node": {Title: "Trusted", Description: "Trusted description", Status: TaskStatusTodo},
	}}
	snapshots := []TaskTraceTeamSnapshot{{Actor: "reader", Tasks: []TaskTraceTeamTask{{
		NodeID: "node", Title: "Unauthorized", Description: "Unauthorized description", Status: TaskStatusDone,
	}}}}

	filtered := taskTraceTeamFilterSnapshotsPermissions(snapshots, &binding, &manifest)
	require.Len(t, filtered, 1)
	require.Len(t, filtered[0].Tasks, 1)
	require.Equal(t, "Trusted", filtered[0].Tasks[0].Title)
	require.Equal(t, "Trusted description", filtered[0].Tasks[0].Description)
	require.Equal(t, TaskStatusTodo, filtered[0].Tasks[0].Status)
}

func TestTaskTraceTeamPermissionsAllowIndependentOutstandingWrite(t *testing.T) {
	manifest := TaskTraceTeamManifest{
		Members: []string{"owner", "reader"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{
			"node": {
				{Username: "owner", Read: true, Write: true, Owner: true},
				{Username: "reader", Read: true},
			},
			"node/outstanding/pending": {
				{Username: "owner", Read: true, Write: true, Owner: true},
				{Username: "reader", Read: true, Write: true},
			},
		},
	}
	previous := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{
		NodeID: "node", Title: "Trusted", Outstanding: `<ul><li data-id="pending">Old item</li></ul>`,
	}}}
	current := TaskTraceTeamSnapshot{Tasks: []TaskTraceTeamTask{{
		NodeID: "node", Title: "Unauthorized title", Outstanding: `<ul><li data-id="pending">Updated item</li></ul>`,
	}}}

	taskTraceTeamProtectSnapshotPermissions(&current, &previous, &manifest, "reader")
	require.Equal(t, "Trusted", current.Tasks[0].Title)
	items, _ := taskTraceTeamOutstandingItems(current.Tasks[0].Outstanding)
	require.Contains(t, items["pending"], "Updated item")
}
