//go:build windows

// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"xorm.io/xorm"
)

// Match Launch-TaskTrace.ps1's FileShare.None handles without touching a live app.
func holdBackupTestFile(t *testing.T, path string) {
	t.Helper()
	name, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, windows.CloseHandle(handle)) })
}

func TestBackupWithWindowsRuntimeFilesLocked(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	team := filepath.Join(root, "teamData")
	createRecoveryDatabase(t, data, 2)
	require.NoError(t, os.MkdirAll(team, 0o700))
	t.Setenv("TASKTRACE_PACKAGE_ROOT", root)
	t.Setenv("TASKTRACE_DATA_ROOT", data)
	t.Setenv("TASKTRACE_TEAM_ROOT", team)

	runtimeFiles := []string{"session.lock", "server.log", "server-error.log"}
	for _, name := range runtimeFiles {
		holdBackupTestFile(t, filepath.Join(data, name))
	}
	// Do not discard a user attachment just because it has a reserved filename.
	files := map[string]string{
		"files/image.png":    "image bytes",
		"files/session.lock": "user attachment",
		"secret.txt":         "persistent secret",
		"local-user-id.txt":  "1",
		"team-sync.json":     "collaboration bindings",
	}
	for relative, content := range files {
		path := filepath.Join(data, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(team, "shared.json"), []byte("team data"), 0o600))

	// Keep a live SQLite connection and an uncommitted writer open as the app does.
	engine, err := xorm.NewEngine("sqlite3", filepath.Join(data, "tasktrace.db")+"?_journal_mode=WAL")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, engine.Close()) })
	_, err = engine.Insert(&recoveryTask{})
	require.NoError(t, err)
	session := engine.NewSession()
	t.Cleanup(func() { _ = session.Rollback(); _ = session.Close() })
	require.NoError(t, session.Begin())
	_, err = session.Insert(&recoveryTask{})
	require.NoError(t, err)

	result, err := runBackup(time.Now())
	require.NoError(t, err)
	require.True(t, result.Created)
	for _, name := range runtimeFiles {
		require.NoFileExists(t, filepath.Join(result.Directory, "data", name))
		require.FileExists(t, filepath.Join(data, name))
	}
	for relative, content := range files {
		require.Equal(t, content, string(requireFile(t, filepath.Join(result.Directory, "data", relative))))
	}
	require.Equal(t, "team data", string(requireFile(t, filepath.Join(result.Directory, "teamData", "shared.json"))))
	snapshot, err := inspectCandidate(filepath.Join(result.Directory, "data"))
	require.NoError(t, err)
	require.Equal(t, int64(3), snapshot.Tasks)
	require.NoFileExists(t, filepath.Join(result.Directory, "data", "tasktrace.db-wal"))
	require.NoFileExists(t, filepath.Join(result.Directory, "data", "tasktrace.db-shm"))
}

func TestBackupDoesNotSkipLockedUserData(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	team := filepath.Join(root, "teamData")
	createRecoveryDatabase(t, data, 1)
	require.NoError(t, os.MkdirAll(team, 0o700))
	t.Setenv("TASKTRACE_PACKAGE_ROOT", root)
	t.Setenv("TASKTRACE_DATA_ROOT", data)
	t.Setenv("TASKTRACE_TEAM_ROOT", team)
	require.NoError(t, os.MkdirAll(filepath.Join(data, "files"), 0o700))
	attachment := filepath.Join(data, "files", "important.png")
	holdBackupTestFile(t, attachment)

	result, err := runBackup(time.Now())
	require.ErrorContains(t, err, attachment)
	require.False(t, result.Created)
	entries, err := listBackups(filepath.Join(root, "backups"))
	require.NoError(t, err)
	require.Empty(t, entries)
	require.FileExists(t, attachment)
}
