//go:build !windows

// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import "errors"

func taskTraceTeamSearchWindowsMembers(string) ([]TaskTraceTeamMemberCandidate, error) {
	return nil, errors.New("Windows account search is only available on Windows")
}

func taskTraceTeamListWindowsAccess(string) ([]string, error) {
	return nil, errors.New("Windows folder permissions are only available on Windows")
}

func taskTraceTeamGrantWindowsAccess(string, string) (string, error) {
	return "", errors.New("Windows folder permissions are only available on Windows")
}

func taskTraceTeamRemoveWindowsAccess(string, string) error {
	return errors.New("Windows folder permissions are only available on Windows")
}
