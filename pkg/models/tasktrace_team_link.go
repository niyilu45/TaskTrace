// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"fmt"
	"path/filepath"
	"reflect"
)

type taskTraceTeamLinkAddress struct {
	Schema       int      `json:"schema"`
	ShareID      string   `json:"share_id"`
	Owner        string   `json:"owner"`
	TokenHash    string   `json:"token_hash"`
	Repository   string   `json:"repository"`
	Repositories []string `json:"repositories,omitempty"`
}

func taskTraceTeamBindingLink(binding *TaskTraceTeamBinding) string {
	var address taskTraceTeamLinkAddress
	path := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "share-link.json")
	if err := taskTraceTeamReadJSON(path, &address); err == nil &&
		address.Schema == taskTraceTeamSchema && address.ShareID == binding.ShareID &&
		taskTraceTeamMembersEqual(address.Owner, binding.Owner) &&
		address.TokenHash == taskTraceTeamTokenHash(binding.Secret) && address.Repository != "" {
		return taskTraceTeamEncodeLinkPaths(address.Repository, address.Repositories, binding.ShareID, binding.Secret)
	}
	info := taskTraceTeamRepositoryMetadata(binding.Repository)
	if info.Path != binding.Repository || len(info.Paths) > 1 {
		return taskTraceTeamEncodeLinkPaths(info.Path, info.Paths, binding.ShareID, binding.Secret)
	}
	if cached, err := taskTraceTeamDecodeLink(binding.ShareLink); err == nil &&
		cached.ShareID == binding.ShareID && cached.Secret == binding.Secret {
		return binding.ShareLink
	}
	return taskTraceTeamEncodeLinkPaths(info.Path, info.Paths, binding.ShareID, binding.Secret)
}

// The owner publishes addresses separately so legacy peers rewriting the task
// manifest cannot replace them with a member's local path or connection alias.
func taskTraceTeamPublishLink(binding *TaskTraceTeamBinding, actor string) error {
	if !taskTraceTeamMembersEqual(binding.Owner, actor) {
		return nil
	}
	info := taskTraceTeamRepositoryInfoCached(binding.Repository)
	link := taskTraceTeamEncodeLinkPaths(info.Path, info.Paths, binding.ShareID, binding.Secret)
	parsed, err := taskTraceTeamDecodeLink(link)
	if err != nil {
		return err
	}
	address := taskTraceTeamLinkAddress{
		Schema:       taskTraceTeamSchema,
		ShareID:      binding.ShareID,
		Owner:        binding.Owner,
		TokenHash:    taskTraceTeamTokenHash(binding.Secret),
		Repository:   parsed.Repository,
		Repositories: parsed.Repositories,
	}
	path := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "share-link.json")
	var previous taskTraceTeamLinkAddress
	if err := taskTraceTeamReadJSON(path, &previous); err != nil || !reflect.DeepEqual(previous, address) {
		if err := taskTraceTeamWriteJSON(path, address); err != nil {
			return fmt.Errorf("publish owner's task share link: %w", err)
		}
	}
	binding.ShareLink = link
	return nil
}
