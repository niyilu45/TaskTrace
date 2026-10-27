// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamMissingRepositoryAccessMergesFloorsWithoutDowngrading(t *testing.T) {
	existing := []TaskTraceTeamRepositoryMember{
		{AccountName: `DOMAIN\Alice`, Access: TaskTraceTeamAccessWrite},
		{AccountName: `DOMAIN\Bob`, Access: TaskTraceTeamAccessRead},
	}
	required := []TaskTraceTeamRepositoryMember{
		{AccountName: "Alice", Access: TaskTraceTeamAccessRead},
		{AccountName: "bob@example.test", Access: TaskTraceTeamAccessWrite},
		{AccountName: "Carol", Access: TaskTraceTeamAccessRead},
		{AccountName: `OTHER\Carol`, Access: TaskTraceTeamAccessWrite},
		{AccountName: "Administrators", Access: TaskTraceTeamAccessWrite},
	}
	missing := taskTraceTeamMissingRepositoryAccess(existing, required)
	require.Equal(t, []TaskTraceTeamRepositoryMember{
		{AccountName: `DOMAIN\Bob`, Access: TaskTraceTeamAccessWrite},
		{AccountName: `OTHER\Carol`, Access: TaskTraceTeamAccessWrite},
	}, missing)
}

func TestTaskTraceTeamManifestRequiredAccessRespectsTaskAndOutstandingChoices(t *testing.T) {
	manifest := &TaskTraceTeamManifest{
		Owner: "Owner", RootNode: "root", Members: []string{"Owner", "Reader", "Writer", "ItemWriter", "Blocked"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{
			"root": {
				{Username: "Owner", Read: true, Write: true, Owner: true},
				{Username: "Reader", Read: true},
				{Username: "Writer", Read: true, Write: true},
				{Username: "ItemWriter", Read: true},
				{Username: "Blocked"},
			},
			"root/outstanding/item": {
				{Username: "ItemWriter", Read: true, Write: true},
				{Username: "Blocked"},
			},
		},
	}
	got := taskTraceTeamManifestRequiredAccess(manifest, `DOMAIN\Owner`)
	require.ElementsMatch(t, []TaskTraceTeamRepositoryMember{
		{AccountName: "Reader", Access: TaskTraceTeamAccessRead},
		{AccountName: "Writer", Access: TaskTraceTeamAccessWrite},
		{AccountName: "ItemWriter", Access: TaskTraceTeamAccessWrite},
	}, got)
}

func TestTaskTraceTeamManifestRequiredAccessLegacyManifestUsesReadFloor(t *testing.T) {
	manifest := &TaskTraceTeamManifest{Owner: "RemoteOwner", Members: []string{"RemoteOwner", "Self", "Colleague", "Administrators"}}
	require.ElementsMatch(t, []TaskTraceTeamRepositoryMember{
		{AccountName: "RemoteOwner", Access: TaskTraceTeamAccessRead},
		{AccountName: "Colleague", Access: TaskTraceTeamAccessRead},
	}, taskTraceTeamManifestRequiredAccess(manifest, "self"))
}

func TestTaskTraceTeamPendingRepositoryAccessCombinesSeparateTeams(t *testing.T) {
	first := &TaskTraceTeamManifest{Owner: "Self", RootNode: "first", Members: []string{"Self", "Shared"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{"first": {{Username: "Shared", Read: true}}}}
	second := &TaskTraceTeamManifest{Owner: "Remote", RootNode: "second", Members: []string{"Remote", "Self", "Shared"},
		Permissions: map[string][]TaskTraceTeamMemberPermission{"second": {{Username: "Shared", Read: true, Write: true}}}}
	required := append(taskTraceTeamManifestRequiredAccess(first, "Self"), taskTraceTeamManifestRequiredAccess(second, "Self")...)
	missing := taskTraceTeamMissingRepositoryAccess([]TaskTraceTeamRepositoryMember{{AccountName: "Remote", Access: TaskTraceTeamAccessWrite}}, required)
	require.Equal(t, []TaskTraceTeamRepositoryMember{{AccountName: "Shared", Access: TaskTraceTeamAccessWrite}}, missing)
}

func TestTaskTraceTeamShareRequiredAccessPreservesExistingReaderWhenAddingMember(t *testing.T) {
	binding := &TaskTraceTeamBinding{Owner: "Owner", Members: []string{"Owner", "Reader", "Writer", "Blocked"}}
	manifest := &TaskTraceTeamManifest{Owner: "Owner", RootNode: "root", Members: binding.Members,
		Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {
			{Username: "Owner", Owner: true, Read: true, Write: true},
			{Username: "Reader", Read: true},
			{Username: "Writer", Read: true, Write: true},
			{Username: "Blocked"},
		}}}
	got, err := taskTraceTeamShareRequiredAccess(binding, manifest, append(binding.Members, "New"), "Owner")
	require.NoError(t, err)
	require.ElementsMatch(t, []TaskTraceTeamRepositoryMember{
		{AccountName: "Reader", Access: TaskTraceTeamAccessRead},
		{AccountName: "Writer", Access: TaskTraceTeamAccessWrite},
		{AccountName: "New", Access: TaskTraceTeamAccessWrite},
	}, got)
}

func TestTaskTraceTeamShareRequiredAccessRejectsAssigneeRemovalBeforeWindowsGrant(t *testing.T) {
	binding := &TaskTraceTeamBinding{Owner: "Owner", Members: []string{"Owner", "Assignee", "Reader"}}
	manifest := &TaskTraceTeamManifest{Owner: "Owner", Members: binding.Members,
		Permissions: map[string][]TaskTraceTeamMemberPermission{"root": {{Username: "Assignee", Assignee: true, Read: true, Write: true}}}}
	got, err := taskTraceTeamShareRequiredAccess(binding, manifest, []string{"Owner", "Reader", "New"}, "Owner")
	require.Error(t, err)
	require.Nil(t, got)
}
