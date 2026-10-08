// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

type recoveryTask struct {
	ID int64 `xorm:"pk autoincr"`
}

func (recoveryTask) TableName() string { return "tasks" }

type recoveryProject struct {
	ID int64 `xorm:"pk autoincr"`
}

func (recoveryProject) TableName() string { return "projects" }

type recoveryUser struct {
	ID int64 `xorm:"pk autoincr"`
}

func (recoveryUser) TableName() string { return "users" }

func createRecoveryDatabase(t *testing.T, directory string, tasks int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(directory, 0o700))
	engine, err := xorm.NewEngine("sqlite3", filepath.Join(directory, "tasktrace.db"))
	require.NoError(t, err)
	require.NoError(t, engine.Sync(new(recoveryTask), new(recoveryProject), new(recoveryUser)))
	for range tasks {
		_, err = engine.Insert(&recoveryTask{})
		require.NoError(t, err)
	}
	_, err = engine.Insert(&recoveryProject{})
	require.NoError(t, err)
	_, err = engine.Insert(&recoveryUser{})
	require.NoError(t, err)
	require.NoError(t, engine.Close())
}

func TestDetectAndImportOldData(t *testing.T) {
	packageDirectory := t.TempDir()
	currentData := filepath.Join(packageDirectory, "data")
	currentTeam := filepath.Join(packageDirectory, "teamData")
	createRecoveryDatabase(t, currentData, 2)
	require.NoError(t, os.MkdirAll(currentTeam, 0o700))
	t.Setenv("TASKTRACE_PACKAGE_ROOT", packageDirectory)
	t.Setenv("TASKTRACE_DATA_ROOT", currentData)
	t.Setenv("TASKTRACE_TEAM_ROOT", currentTeam)

	oldPackage := filepath.Join(t.TempDir(), "TaskTrace-local")
	oldData := filepath.Join(oldPackage, "data")
	oldTeam := filepath.Join(oldPackage, "teamData")
	createRecoveryDatabase(t, oldData, 3)
	require.NoError(t, os.MkdirAll(oldTeam, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(oldTeam, "shared.json"), []byte("team"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(packageDirectory, "tasktrace-settings.json"), []byte(`{"dataDirectory":"data/current","custom":true}`), 0o600))

	detection, err := Detect(oldPackage)
	require.NoError(t, err)
	var detected *TaskTraceDataCandidate
	for index := range detection.Candidates {
		require.False(t, samePath(detection.Candidates[index].DataDirectory, currentData))
		if samePath(detection.Candidates[index].DataDirectory, oldData) {
			detected = &detection.Candidates[index]
			break
		}
	}
	require.NotNil(t, detected)
	require.Equal(t, int64(3), detected.Tasks)
	require.Equal(t, oldTeam, detected.TeamDataDirectory)

	result, err := Import(TaskTraceDataImportRequest{
		DataDirectory:     oldData,
		TeamDataDirectory: oldTeam,
	})
	require.NoError(t, err)
	require.True(t, result.RestartRequired)
	require.FileExists(t, filepath.Join(result.DataDirectory, "tasktrace.db"))
	require.Equal(t, "team", string(requireFile(t, filepath.Join(result.TeamDataDirectory, "shared.json"))))
	require.FileExists(t, result.BackupSettings)

	settings := map[string]any{}
	require.NoError(t, json.Unmarshal(requireFile(t, filepath.Join(packageDirectory, "tasktrace-settings.json")), &settings))
	require.Equal(t, true, settings["custom"])
	require.Contains(t, settings["dataDirectory"], "data/imports/")
	require.Contains(t, settings["teamDataDirectory"], "teamData/imports/")

	copied, err := inspectCandidate(result.DataDirectory)
	require.NoError(t, err)
	require.Equal(t, int64(3), copied.Tasks)
}

func requireFile(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return content
}
