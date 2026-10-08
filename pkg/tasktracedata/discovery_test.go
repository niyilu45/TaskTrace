// SPDX-License-Identifier: AGPL-3.0-or-later
//
//nolint:gosmopolitan // Tests intentionally cover Chinese copy paths and localized recovery errors.
package tasktracedata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func recoveryEnvironment(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "installed")
	current := filepath.Join(root, "data", "current")
	createRecoveryDatabase(t, current, 1)
	team := filepath.Join(root, "teamData", "current")
	require.NoError(t, os.MkdirAll(team, 0o700))
	t.Setenv("TASKTRACE_PACKAGE_ROOT", root)
	t.Setenv("TASKTRACE_DATA_ROOT", current)
	t.Setenv("TASKTRACE_TEAM_ROOT", team)
	return root
}

func TestDataRecoveryDetectsCopiedLayoutsAndPairsTeamData(t *testing.T) {
	recoveryEnvironment(t)
	copyRoot := filepath.Join(t.TempDir(), "复制数据 #1")
	legacy := filepath.Join(copyRoot, "data")
	first := filepath.Join(legacy, "imports", "20261007-120000")
	second := filepath.Join(legacy, "imports", "20261008-120000")
	createRecoveryDatabase(t, legacy, 2)
	createRecoveryDatabase(t, first, 3)
	createRecoveryDatabase(t, second, 4)
	team := filepath.Join(copyRoot, "teamData")
	require.NoError(t, os.MkdirAll(filepath.Join(team, "imports", "20261007-120000"), 0o700))
	require.NoError(t, os.MkdirAll(filepath.Join(team, "imports", "20261008-120000"), 0o700))
	for _, input := range []string{copyRoot, legacy, `"` + legacy + `"`, team} {
		t.Run(input, func(t *testing.T) {
			result, err := Detect(input)
			require.NoError(t, err)
			require.Len(t, result.Candidates, 3)
			found := map[string]TaskTraceDataCandidate{}
			for _, candidate := range result.Candidates {
				found[candidate.DataDirectory] = candidate
			}
			require.Equal(t, int64(2), found[legacy].Tasks)
			require.Equal(t, int64(3), found[first].Tasks)
			require.Equal(t, int64(4), found[second].Tasks)
			require.Equal(t, team, found[legacy].TeamDataDirectory)
			require.Equal(t, filepath.Join(team, "imports", "20261007-120000"), found[first].TeamDataDirectory)
			require.Equal(t, filepath.Join(team, "imports", "20261008-120000"), found[second].TeamDataDirectory)
		})
	}
	for _, input := range []string{first, filepath.Join(first, "tasktrace.db"), filepath.Join(team, "imports", "20261007-120000")} {
		result, err := Detect(input)
		require.NoError(t, err)
		require.Len(t, result.Candidates, 1)
		require.Equal(t, first, result.Candidates[0].DataDirectory)
	}
	_, err := resolveCandidateDirectory(filepath.Join(legacy, "imports"))
	require.ErrorContains(t, err, "多份数据")
}

func TestDataRecoveryDetectsNewOnlyRenamedAndAutomaticContainers(t *testing.T) {
	root := recoveryEnvironment(t)
	container := filepath.Join(t.TempDir(), "改名后的数据")
	imported := filepath.Join(container, "imports", "new")
	createRecoveryDatabase(t, imported, 3)
	result, err := Detect(container)
	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, imported, result.Candidates[0].DataDirectory)
	resolved, err := resolveCandidateDirectory(container)
	require.NoError(t, err)
	require.Equal(t, imported, resolved)
	// Nearby detection must also discover imported datasets instead of only data/tasktrace.db.
	other := filepath.Join(root, "data", "imports", "older")
	createRecoveryDatabase(t, other, 5)
	result, err = Detect("")
	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, other, result.Candidates[0].DataDirectory)
	// An explicit path must not be contaminated by unrelated automatic results.
	result, err = Detect(container)
	require.NoError(t, err)
	require.Len(t, result.Candidates, 1)
	require.Equal(t, imported, result.Candidates[0].DataDirectory)
}

func TestDataRecoveryImportsMatchingCopiedDataWithoutOverwriting(t *testing.T) {
	for _, separateTeam := range []bool{false, true} {
		t.Run(map[bool]string{false: "automatic-pair", true: "separate-team-copy"}[separateTeam], func(t *testing.T) {
			root := recoveryEnvironment(t)
			copyRoot := filepath.Join(t.TempDir(), "copied")
			sourceData := filepath.Join(copyRoot, "data", "imports", "selected")
			sourceTeam := filepath.Join(copyRoot, "teamData", "imports", "selected")
			teamInput := ""
			if separateTeam {
				teamInput = filepath.Join(t.TempDir(), "团队单独复制")
				sourceTeam = filepath.Join(teamInput, "imports", "selected")
			}
			createRecoveryDatabase(t, sourceData, 7)
			require.NoError(t, os.MkdirAll(filepath.Join(sourceData, "files"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(sourceData, "files", "42"), []byte("outstanding-note-image"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(sourceData, "team-sync.json"), []byte(`{"bindings":[{"share_id":"keep"}]}`), 0o600))
			require.NoError(t, os.MkdirAll(filepath.Join(sourceTeam, "tasks", "keep"), 0o700))
			require.NoError(t, os.WriteFile(filepath.Join(sourceTeam, "tasks", "keep", "snapshot.json"), []byte("shared progress and outstanding items"), 0o600))
			beforeSource := requireFile(t, filepath.Join(sourceData, "tasktrace.db"))
			beforeCurrent := requireFile(t, filepath.Join(dataRoot(), "tasktrace.db"))
			result, err := Import(TaskTraceDataImportRequest{DataDirectory: sourceData, TeamDataDirectory: teamInput})
			require.NoError(t, err)
			require.True(t, result.RestartRequired)
			require.Equal(t, beforeSource, requireFile(t, filepath.Join(result.DataDirectory, "tasktrace.db")))
			require.Equal(t, beforeSource, requireFile(t, filepath.Join(sourceData, "tasktrace.db")))
			require.Equal(t, beforeCurrent, requireFile(t, filepath.Join(dataRoot(), "tasktrace.db")))
			require.Equal(t, "outstanding-note-image", string(requireFile(t, filepath.Join(result.DataDirectory, "files", "42"))))
			require.Equal(t, requireFile(t, filepath.Join(sourceData, "team-sync.json")), requireFile(t, filepath.Join(result.DataDirectory, "team-sync.json")))
			require.Equal(t, "shared progress and outstanding items", string(requireFile(t, filepath.Join(result.TeamDataDirectory, "tasks", "keep", "snapshot.json"))))
			require.NoDirExists(t, filepath.Join(result.TeamDataDirectory, "imports"))
			require.FileExists(t, filepath.Join(root, "tasktrace-settings.json"))
		})
	}
}

func TestDataRecoveryDoesNotSilentlyMixUnmatchedTeamImports(t *testing.T) {
	root := recoveryEnvironment(t)
	copyRoot := t.TempDir()
	data := filepath.Join(copyRoot, "data", "imports", "selected")
	team := filepath.Join(copyRoot, "teamData")
	createRecoveryDatabase(t, data, 3)
	require.NoError(t, os.MkdirAll(filepath.Join(team, "imports", "different"), 0o700))
	result, err := Detect(filepath.Join(copyRoot, "data"))
	require.NoError(t, err)
	require.Empty(t, result.Candidates[0].TeamDataDirectory)
	_, err = Import(TaskTraceDataImportRequest{DataDirectory: data, TeamDataDirectory: team})
	require.ErrorContains(t, err, "没有找到与所选个人数据对应")
	require.NoFileExists(t, filepath.Join(root, "tasktrace-settings.json"))
	require.NoDirExists(t, filepath.Join(root, "data", "imports"))
}

func TestDataRecoveryReportsInvalidOrTeamOnlyData(t *testing.T) {
	recoveryEnvironment(t)
	teamOnly := filepath.Join(t.TempDir(), "teamData")
	require.NoError(t, os.MkdirAll(teamOnly, 0o700))
	_, err := Detect(teamOnly)
	require.ErrorContains(t, err, "配套 data")
	_, err = Detect(filepath.Join(t.TempDir(), "missing"))
	require.ErrorContains(t, err, "路径和访问权限")
	broken := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(broken, "tasktrace.db"), []byte("not a database"), 0o600))
	_, err = Detect(broken)
	require.ErrorContains(t, err, "找到数据库但无法读取")
	// Attachments must never be recursively mistaken for imported databases.
	empty := t.TempDir()
	createRecoveryDatabase(t, filepath.Join(empty, "files", "data"), 1)
	_, err = Detect(empty)
	require.ErrorContains(t, err, "未找到 TaskTrace 数据库")
}
