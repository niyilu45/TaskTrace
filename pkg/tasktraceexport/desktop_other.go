//go:build !windows

// SPDX-License-Identifier: AGPL-3.0-or-later
//nolint:gosmopolitan // These desktop-only messages follow the Chinese TaskTrace UI.
package tasktraceexport

import (
	"context"
	"errors"
)

const Supported = false

func choosePath(context.Context, string) (string, error) {
	return "", errors.New("系统保存窗口仅在 Windows 本机版提供")
}
func reveal(string) error { return errors.New("打开导出目录仅在 Windows 本机版提供") }
