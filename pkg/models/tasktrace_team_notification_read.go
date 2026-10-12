// SPDX-License-Identifier: AGPL-3.0-or-later
package models

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/web"
	"github.com/google/uuid"
	"xorm.io/xorm"
)

// Read receipts are personal, durable data. Never rewrite the shared notification:
// a read-only collaborator can acknowledge it, and an older peer replaying the
// same notification cannot reset its read state.
func taskTraceTeamNotificationReadPath(binding *TaskTraceTeamBinding, username string) string {
	statePath := taskTraceTeamStatePath()
	if statePath == "" {
		return ""
	}
	key := taskTraceTeamTokenHash(strings.ToLower(username) + "\x00" + binding.ShareID)
	return filepath.Join(filepath.Dir(statePath), "notification-read", key+".json")
}

func taskTraceTeamLoadNotificationReads(binding *TaskTraceTeamBinding, username string) (map[string]bool, error) {
	read := map[string]bool{}
	path := taskTraceTeamNotificationReadPath(binding, username)
	if path == "" {
		return read, errors.New("TaskTrace notification state is unavailable")
	}
	if err := taskTraceTeamReadJSON(path, &read); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if read == nil {
		read = map[string]bool{}
	}
	return read, nil
}

func TaskTraceTeamNotificationsRead(s *xorm.Session, a web.Auth, request TaskTraceTeamNotificationsReadRequest) (*TaskTraceTeamStatus, error) {
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
	selected := map[string]bool{}
	for _, id := range request.IDs {
		parsed, parseErr := uuid.Parse(id)
		if parseErr != nil {
			return nil, errors.New("invalid team notification ID")
		}
		selected[parsed.String()] = true
	}
	for i := range state.Bindings {
		binding := &state.Bindings[i]
		dir := filepath.Join(taskTraceTeamShareDir(binding.Repository, binding.ShareID), "notifications", strings.ToLower(u.Username))
		entries, readErr := os.ReadDir(dir)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return nil, readErr
		}
		read, readErr := taskTraceTeamLoadNotificationReads(binding, u.Username)
		if readErr != nil {
			return nil, readErr
		}
		changed := false
		for _, entry := range entries {
			id := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") || (len(selected) > 0 && !selected[id]) || read[id] {
				continue
			}
			read[id] = true
			changed = true
		}
		if changed {
			if err := taskTraceTeamWriteJSON(taskTraceTeamNotificationReadPath(binding, u.Username), read); err != nil {
				return nil, err
			}
		}
	}
	status, err := taskTraceTeamStatusLockedFast(s, a, state)
	return &status, err
}
