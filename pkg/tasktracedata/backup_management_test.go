// SPDX-License-Identifier: AGPL-3.0-or-later
//
//nolint:gosmopolitan // User notes may contain Chinese text and newlines.
package tasktracedata

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackupManagementNotesAndDeletion(t *testing.T) {
	root := recoveryEnvironment(t)
	teamFile := filepath.Join(teamDataRoot(), "shared.json")
	require.NoError(t, os.WriteFile(teamFile, []byte("first"), 0o600))
	first, err := runBackup(time.Now().Add(-time.Hour))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(teamFile, []byte("second"), 0o600))
	second, err := RunBackup()
	require.NoError(t, err)
	listed, err := ListRecoverableBackups()
	require.NoError(t, err)
	require.Len(t, listed.Backups, 2)
	latest := listed.Backups[0]
	require.True(t, second.CreatedAt.Equal(latest.CreatedAt))
	require.Equal(t, int64(1), latest.Candidate.Tasks)
	require.Equal(t, filepath.Join(second.Directory, "teamData"), latest.Candidate.TeamDataDirectory)
	dataBefore := requireFile(t, filepath.Join(dataRoot(), "tasktrace.db"))
	manifestBefore := requireFile(t, filepath.Join(second.Directory, "backup-manifest.json"))
	backupBefore := requireFile(t, filepath.Join(second.Directory, "data", "tasktrace.db"))
	note := TaskTraceBackupNote{Note: "发布前备份\n包含团队任务 <b>原文</b>"}
	_, err = SaveBackupNote(latest.ID, note)
	require.NoError(t, err)
	listed, err = ListRecoverableBackups()
	require.NoError(t, err)
	require.Equal(t, note.Note, listed.Backups[0].Note)
	require.Empty(t, listed.Backups[1].Note)
	require.Equal(t, latest.ID, listed.Backups[0].ID)
	require.Equal(t, manifestBefore, requireFile(t, filepath.Join(second.Directory, "backup-manifest.json")))
	require.Equal(t, backupBefore, requireFile(t, filepath.Join(second.Directory, "data", "tasktrace.db")))
	unchanged, err := RunBackup()
	require.NoError(t, err)
	require.True(t, unchanged.Skipped, "notes must not affect change detection")
	_, err = SaveBackupNote(latest.ID, TaskTraceBackupNote{Note: strings.Repeat("字", 2001)})
	require.Error(t, err)
	stored, err := readBackupNote(second.Directory)
	require.NoError(t, err)
	require.Equal(t, note, stored, "failed validation must preserve the existing note")
	_, err = SaveBackupNote(latest.ID, TaskTraceBackupNote{})
	require.NoError(t, err)
	stored, err = readBackupNote(second.Directory)
	require.NoError(t, err)
	require.Empty(t, stored.Note)
	_, err = SaveBackupNote(latest.ID, note)
	require.NoError(t, err)
	// Explicit deletion is allowed even below the automatic minimum of three.
	require.NoError(t, DeleteBackup(latest.ID))
	require.NoDirExists(t, second.Directory)
	require.DirExists(t, first.Directory)
	require.Equal(t, dataBefore, requireFile(t, filepath.Join(dataRoot(), "tasktrace.db")))
	require.Equal(t, "second", string(requireFile(t, teamFile)))
	require.DirExists(t, root)
	status, err := BackupStatus()
	require.NoError(t, err)
	require.Equal(t, 1, status.BackupCount)
	require.True(t, first.CreatedAt.Equal(*status.LastBackupAt))
	require.Error(t, DeleteBackup(latest.ID))
}

func TestBackupManagementRejectsStaleAndUnmanagedTargets(t *testing.T) {
	recoveryEnvironment(t)
	created, err := RunBackup()
	require.NoError(t, err)
	listed, err := ListRecoverableBackups()
	require.NoError(t, err)
	require.Len(t, listed.Backups, 1)
	id := listed.Backups[0].ID
	for _, invalid := range []string{"", "../data", dataRoot(), created.Directory, strings.Repeat("a", 64)} {
		require.Error(t, DeleteBackup(invalid))
		_, err = SaveBackupNote(invalid, TaskTraceBackupNote{Note: "invalid"})
		require.Error(t, err)
	}
	require.DirExists(t, created.Directory)
	settings := DefaultBackupSettings()
	settings.Directory = filepath.Join(t.TempDir(), "another-backup-root")
	_, err = WriteBackupSettings(settings)
	require.NoError(t, err)
	require.Error(t, DeleteBackup(id))
	_, err = SaveBackupNote(id, TaskTraceBackupNote{Note: "stale"})
	require.Error(t, err)
	listed, err = ListRecoverableBackups()
	require.NoError(t, err)
	require.Empty(t, listed.Backups)
	require.DirExists(t, created.Directory)
	_, err = WriteBackupSettings(DefaultBackupSettings())
	require.NoError(t, err)
	// A folder without its backup manifest is never a managed deletion target.
	require.NoError(t, os.Rename(filepath.Join(created.Directory, "backup-manifest.json"), filepath.Join(created.Directory, "saved-manifest.json")))
	require.Error(t, DeleteBackup(id))
	require.DirExists(t, created.Directory)
}

func TestBackupManagementProtectsLiveAndLinkedData(t *testing.T) {
	recoveryEnvironment(t)
	created, err := RunBackup()
	require.NoError(t, err)
	listed, err := ListRecoverableBackups()
	require.NoError(t, err)
	id := listed.Backups[0].ID
	t.Run("active data in backup", func(t *testing.T) {
		t.Setenv("TASKTRACE_DATA_ROOT", filepath.Join(created.Directory, "data"))
		require.ErrorContains(t, DeleteBackup(id), "active")
		_, err = SaveBackupNote(id, TaskTraceBackupNote{Note: "unsafe"})
		require.Error(t, err)
		listed, err = ListRecoverableBackups()
		require.NoError(t, err)
		require.Empty(t, listed.Backups)
	})
	t.Run("linked descendant", func(t *testing.T) {
		link := filepath.Join(created.Directory, "linked-data")
		if err := os.Symlink(dataRoot(), link); err != nil {
			t.Skipf("symlink creation unavailable: %v", err)
		}
		require.Error(t, DeleteBackup(id))
		require.FileExists(t, filepath.Join(dataRoot(), "tasktrace.db"))
		require.FileExists(t, filepath.Join(created.Directory, "data", "tasktrace.db"))
		require.NoError(t, os.Remove(link))
	})
}
