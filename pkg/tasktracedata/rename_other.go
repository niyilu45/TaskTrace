//go:build !windows

// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

func temporaryRenameError(_ error) bool { return false }
