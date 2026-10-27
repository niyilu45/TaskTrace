//go:build !windows

// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"context"
	"errors"
)

func taskTraceTeamSearchWindowsMembers(context.Context, string, bool) ([]TaskTraceTeamMemberCandidate, error) {
	return nil, errors.New("Windows account search is only available on Windows")
}

func taskTraceTeamListWindowsAccess(root string) ([]TaskTraceTeamRepositoryMember, error) {
	if members, ok := taskTraceWindowsAccessCache.cached(root); ok {
		return members, nil
	}
	return nil, errors.New("Windows folder permissions are only available on Windows")
}

func taskTraceTeamGrantWindowsAccess(string, string) (string, error) {
	return "", errors.New("Windows folder permissions are only available on Windows")
}

func taskTraceTeamRemoveWindowsAccess(string, string) error {
	return errors.New("Windows folder permissions are only available on Windows")
}

func taskTraceTeamRemoveWindowsAccessWithElevation(string, string, bool) error {
	return errors.New("Windows folder permissions are only available on Windows")
}

func taskTraceTeamSetWindowsAccessWithOptions(string, string, string, TaskTraceTeamAccessOptions) (string, error) {
	return "", errors.New("Windows folder permissions are only available on Windows")
}
