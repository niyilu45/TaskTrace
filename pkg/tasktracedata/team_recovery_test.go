// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createRecoveryTeam(t *testing.T, data, team string) {
	t.Helper()
	digest := sha256.Sum256([]byte("test-secret"))
	files := map[string]any{
		filepath.Join(data, "team-sync.json"): map[string]any{
			"schema":       1,
			"device_id":    "preserved-device",
			"future_field": "preserve",
			"bindings": []map[string]any{
				{
					"share_id":   "shared-task",
					"repository": "D:/old-install/teamData",
					"secret":     "test-secret",
					"node_tasks": map[string]int{"root": 42},
					"base": map[string]any{
						"root": map[string]string{"outstanding": "preserve"},
					},
					"outstanding_priorities": map[string]int{"root:item": 7},
				},
				{
					"share_id":   "remote-only",
					"repository": `\\server\teamData`,
					"secret":     "remote-secret",
				},
			},
		},
		filepath.Join(team, "tasks", "shared-task", "manifest.json"): map[string]any{
			"schema":     1,
			"share_id":   "shared-task",
			"token_hash": hex.EncodeToString(digest[:]),
		},
		filepath.Join(team, "tasks", "shared-task", "actors", "colleague", "device.json"): map[string]any{
			"schema":   1,
			"share_id": "shared-task",
			"tasks": []any{map[string]any{
				"node_id":     "root",
				"outstanding": "shared outstanding",
				"comments":    []any{map[string]string{"body": "daily progress"}},
			}},
		},
	}
	for path, value := range files {
		content, err := json.Marshal(value)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, content, 0o600))
	}
	attachment := filepath.Join(team, "tasks", "shared-task", "attachments", "image")
	require.NoError(t, os.MkdirAll(filepath.Dir(attachment), 0o700))
	require.NoError(t, os.WriteFile(attachment, []byte("image-bytes"), 0o600))
}

func TestDataRecoveryDetectsSeparateTeamAndRestoresBindings(t *testing.T) {
	for _, layout := range []string{"imports/selected", "storage/original", "storage/current", "imports/current", "."} {
		t.Run(filepath.ToSlash(layout), func(t *testing.T) {
			recoveryEnvironment(t)
			dataContainer := filepath.Join(t.TempDir(), "personal-copy")
			data := filepath.Join(dataContainer, layout)
			teamContainer := filepath.Join(t.TempDir(), "team-copy")
			team := filepath.Join(teamContainer, layout)
			createRecoveryDatabase(t, data, 3)
			createRecoveryTeam(t, data, team)
			originalState := requireFile(t, filepath.Join(data, "team-sync.json"))
			currentDB := requireFile(t, filepath.Join(dataRoot(), "tasktrace.db"))
			detection, err := Detect(dataContainer, teamContainer)
			require.NoError(t, err)
			require.Len(t, detection.Candidates, 1)
			candidate := detection.Candidates[0]
			require.Equal(t, team, candidate.TeamDataDirectory)
			require.Equal(t, 1, candidate.TeamShares)
			result, err := Import(TaskTraceDataImportRequest{
				DataDirectory:     candidate.DataDirectory,
				TeamDataDirectory: candidate.TeamDataDirectory,
			})
			require.NoError(t, err)
			require.Equal(t, currentDB, requireFile(t, filepath.Join(dataRoot(), "tasktrace.db")))
			require.Equal(t, originalState, requireFile(t, filepath.Join(data, "team-sync.json")))
			before, err := readRecoveryJSON(filepath.Join(data, "team-sync.json"))
			require.NoError(t, err)
			after, err := readRecoveryJSON(filepath.Join(result.DataDirectory, "team-sync.json"))
			require.NoError(t, err)
			var beforeBindings, afterBindings []map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(before["bindings"], &beforeBindings))
			require.NoError(t, json.Unmarshal(after["bindings"], &afterBindings))
			require.Equal(t, result.TeamDataDirectory, rawString(afterBindings[0]["repository"]))
			afterBindings[0]["repository"] = beforeBindings[0]["repository"]
			expected, err := json.Marshal(beforeBindings)
			require.NoError(t, err)
			actual, err := json.Marshal(afterBindings)
			require.NoError(t, err)
			require.JSONEq(t, string(expected), string(actual))
			for _, field := range []string{"schema", "device_id", "future_field"} {
				require.JSONEq(t, string(before[field]), string(after[field]))
			}
			for _, relative := range []string{"manifest.json", "actors/colleague/device.json", "attachments/image"} {
				require.Equal(t, requireFile(t, filepath.Join(team, "tasks/shared-task", relative)), requireFile(t, filepath.Join(result.TeamDataDirectory, "tasks/shared-task", relative)))
			}
		})
	}
}

func TestDataRecoveryRejectsWrongSeparateTeamWithoutChangingSettings(t *testing.T) {
	root := recoveryEnvironment(t)
	data, team := filepath.Join(t.TempDir(), "data"), filepath.Join(t.TempDir(), "teamData")
	createRecoveryDatabase(t, data, 2)
	createRecoveryTeam(t, data, team)
	_, err := Detect(data, filepath.Join(team, "missing"))
	require.Error(t, err)
	settingsPath := filepath.Join(root, "tasktrace-settings.json")
	require.NoError(t, os.WriteFile(settingsPath, []byte(`{"keep":"unchanged"}`), 0o600))
	manifest := filepath.Join(team, "tasks", "shared-task", "manifest.json")
	require.NoError(t, os.WriteFile(manifest, []byte(`{"share_id":"shared-task","token_hash":"wrong-secret"}`), 0o600))
	_, err = Import(TaskTraceDataImportRequest{
		DataDirectory:     data,
		TeamDataDirectory: team,
	})
	require.ErrorContains(t, err, "does not match")
	require.JSONEq(t, `{"keep":"unchanged"}`, string(requireFile(t, settingsPath)))
}

func TestDataRecoveryExplicitTeamOverridesUnrelatedSibling(t *testing.T) {
	recoveryEnvironment(t)
	data := filepath.Join(t.TempDir(), "data")
	team := filepath.Join(t.TempDir(), "selected-team")
	createRecoveryDatabase(t, data, 2)
	createRecoveryTeam(t, data, team)
	unrelated := filepath.Join(filepath.Dir(data), "teamData", "tasks", "broken", "manifest.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(unrelated), 0o700))
	require.NoError(t, os.WriteFile(unrelated, []byte("broken unrelated manifest"), 0o600))
	detected, err := Detect(data, team)
	require.NoError(t, err)
	require.Equal(t, team, detected.Candidates[0].TeamDataDirectory)
	require.Equal(t, 1, detected.Candidates[0].TeamShares)
	_, err = Import(TaskTraceDataImportRequest{
		DataDirectory:     data,
		TeamDataDirectory: team,
	})
	require.NoError(t, err)
}

func TestBackupRestoresTeamRepositoryAndBindingsTogether(t *testing.T) {
	recoveryEnvironment(t)
	createRecoveryTeam(t, dataRoot(), teamDataRoot())
	backup, err := runBackup(time.Now())
	require.NoError(t, err)
	detection, err := Detect(backup.Directory)
	require.NoError(t, err)
	require.Len(t, detection.Candidates, 1)
	require.Equal(t, 1, detection.Candidates[0].TeamShares)
	result, err := Import(TaskTraceDataImportRequest{DataDirectory: detection.Candidates[0].DataDirectory})
	require.NoError(t, err)
	state, err := readRecoveryJSON(filepath.Join(result.DataDirectory, "team-sync.json"))
	require.NoError(t, err)
	var bindings []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(state["bindings"], &bindings))
	require.Equal(t, result.TeamDataDirectory, rawString(bindings[0]["repository"]))
	require.FileExists(t, filepath.Join(result.TeamDataDirectory, "tasks/shared-task/attachments/image"))
}
