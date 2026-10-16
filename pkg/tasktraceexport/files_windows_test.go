// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktraceexport

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestTaskTraceExportLockedFilePreservesOriginal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "locked.md")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	ptr, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)
	handle, err := windows.CreateFile(ptr, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	defer func() { require.NoError(t, windows.CloseHandle(handle)) }()
	_, err = save(context.Background(), "locked.md", "replacement", func(context.Context, string) (string, error) { return path, nil })
	require.Error(t, err)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "original", string(body))
	files, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, files, 1)
}
