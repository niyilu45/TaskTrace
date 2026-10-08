// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackupSkipsUnchangedDataAndPrunesWithMinimum(t *testing.T) {
	packageDirectory := t.TempDir()
	dataDirectory := filepath.Join(packageDirectory, "data")
	teamDirectory := filepath.Join(packageDirectory, "teamData")
	backupDirectory := filepath.Join(packageDirectory, "saved-backups")
	createRecoveryDatabase(t, dataDirectory, 2)
	require.NoError(t, os.MkdirAll(teamDirectory, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(teamDirectory, "shared.json"), []byte("team"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(packageDirectory, "tasktrace-settings.json"), []byte(`{"dataDirectory":"custom-data","teamDataDirectory":"custom-team"}`), 0o600))
	t.Setenv("TASKTRACE_PACKAGE_ROOT", packageDirectory)
	t.Setenv("TASKTRACE_DATA_ROOT", dataDirectory)
	t.Setenv("TASKTRACE_TEAM_ROOT", teamDirectory)

	settings, err := WriteBackupSettings(TaskTraceBackupSettings{
		Enabled:         true,
		Directory:       backupDirectory,
		IntervalMinutes: 15,
		RetentionDays:   1,
		MinimumBackups:  2,
	})
	require.NoError(t, err)
	require.Equal(t, backupDirectory, settings.Directory)

	firstTime := time.Now().Add(-72 * time.Hour).Round(time.Second)
	first, err := runBackup(firstTime)
	require.NoError(t, err)
	require.True(t, first.Created)
	require.FileExists(t, filepath.Join(first.Directory, "data", "tasktrace.db"))
	require.FileExists(t, filepath.Join(first.Directory, "teamData", "shared.json"))

	unchanged, err := runBackup(firstTime.Add(time.Hour))
	require.NoError(t, err)
	require.True(t, unchanged.Skipped)
	require.False(t, unchanged.Created)

	require.NoError(t, os.WriteFile(filepath.Join(teamDirectory, "shared.json"), []byte("changed"), 0o600))
	second, err := runBackup(firstTime.Add(2 * time.Hour))
	require.NoError(t, err)
	require.True(t, second.Created)

	require.NoError(t, os.WriteFile(filepath.Join(teamDirectory, "second.json"), []byte("new"), 0o600))
	third, err := runBackup(time.Now())
	require.NoError(t, err)
	require.True(t, third.Created)
	require.Equal(t, 1, third.Removed)

	entries, err := listBackups(backupDirectory)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, third.Digest, entries[0].manifest.Digest)

	stored := map[string]any{}
	require.NoError(t, json.Unmarshal(requireFile(t, filepath.Join(packageDirectory, "tasktrace-settings.json")), &stored))
	require.Contains(t, stored, "backup")
	require.Equal(t, "custom-data", stored["dataDirectory"])
	require.Equal(t, "custom-team", stored["teamDataDirectory"])
}

func TestBackupDirectoryCannotBeInsideLiveData(t *testing.T) {
	packageDirectory := t.TempDir()
	dataDirectory := filepath.Join(packageDirectory, "data")
	teamDirectory := filepath.Join(packageDirectory, "teamData")
	createRecoveryDatabase(t, dataDirectory, 1)
	require.NoError(t, os.MkdirAll(teamDirectory, 0o700))
	t.Setenv("TASKTRACE_PACKAGE_ROOT", packageDirectory)
	t.Setenv("TASKTRACE_DATA_ROOT", dataDirectory)
	t.Setenv("TASKTRACE_TEAM_ROOT", teamDirectory)

	_, err := WriteBackupSettings(TaskTraceBackupSettings{
		Directory:       filepath.Join(dataDirectory, "backups"),
		IntervalMinutes: 60,
		RetentionDays:   30,
		MinimumBackups:  3,
	})
	require.ErrorContains(t, err, "backup directory cannot be inside")
}
