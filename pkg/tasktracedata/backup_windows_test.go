//go:build windows

// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"xorm.io/xorm"
)

func TestBackupRenameRetriesWindowsDirectoryLock(t *testing.T) {
	root := t.TempDir()
	staging, final := filepath.Join(root, "staging"), filepath.Join(root, "published")
	require.NoError(t, os.MkdirAll(staging, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(staging, "data.json"), []byte("backup"), 0o600))
	name, err := windows.UTF16PtrFromString(staging)
	require.NoError(t, err)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	require.NoError(t, err)
	// A real Windows handle without FILE_SHARE_DELETE reproduces access denied.
	err = os.Rename(staging, final)
	if !temporaryRenameError(err) {
		_ = windows.CloseHandle(handle)
		t.Fatalf("expected Windows rename lock, got %v", err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(125 * time.Millisecond)
		released <- windows.CloseHandle(handle)
	}()
	err = renameDataFile(staging, final)
	require.NoError(t, <-released)
	require.NoError(t, err)
	require.Equal(t, "backup", string(requireFile(t, filepath.Join(final, "data.json"))))
}

func TestBackupAtomicWritePreservesLockedDestination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	name, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	err = writeFileAtomic(path, []byte("replacement"), 0o600)
	require.NoError(t, windows.CloseHandle(handle))
	require.Error(t, err)
	require.Equal(t, "original", string(requireFile(t, path)))
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	// Once the handle is gone, ordinary replacement works without deleting first.
	require.NoError(t, writeFileAtomic(path, []byte("replacement"), 0o600))
	require.Equal(t, "replacement", string(requireFile(t, path)))
}

func TestBackupRenameDoesNotRetryUnrelatedErrors(t *testing.T) {
	calls := 0
	err := retryDataRename(func() error { calls++; return os.ErrNotExist })
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Equal(t, 1, calls)
	calls = 0
	err = retryDataRename(func() error {
		calls++
		if calls == 1 {
			return &os.LinkError{Op: "rename", Err: windows.ERROR_ACCESS_DENIED}
		}
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.False(t, temporaryRenameError(errors.New("permanent error")))
}

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
