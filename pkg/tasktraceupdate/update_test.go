// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktraceupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/version"
	"github.com/stretchr/testify/require"
)

func TestUpdateStateAndSettingsSharePortableStorage(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TASKTRACE_DATA_ROOT", root)
	previousVersion := version.Version
	version.Version = "v1.2.3-beta.4"
	t.Cleanup(func() { version.Version = previousVersion })

	settings, err := ReadSettings()
	require.NoError(t, err)
	require.Equal(t, 60, settings.CheckIntervalMinutes)

	settings, err = WriteSettings(Settings{CheckIntervalMinutes: 90})
	require.NoError(t, err)
	require.Equal(t, 90, settings.CheckIntervalMinutes)

	state := State{LatestVersion: "v1.3.0", Available: true, Notify: true, Status: "idle"}
	content, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "update-state.json"), content, 0600))

	settings, state, err = Ignore("v1.3.0")
	require.NoError(t, err)
	require.Equal(t, "v1.3.0", settings.IgnoredVersion)
	require.False(t, state.Notify)
	require.Equal(t, "v1.2.3-beta.4", state.CurrentVersion)

	command, err := Queue("check", "")
	require.NoError(t, err)
	require.Equal(t, "check", command.Action)
	state, err = ReadState()
	require.NoError(t, err)
	require.Equal(t, "queued", state.Status)
	require.Empty(t, state.Error)

	command, err = Queue("install", "v1.3.0")
	require.NoError(t, err)
	require.Equal(t, "install", command.Action)
	require.Equal(t, "v1.3.0", command.Version)
	require.FileExists(t, filepath.Join(root, "update-command.json"))
}

func TestWriteSettingsRejectsUnsafeIntervals(t *testing.T) {
	t.Setenv("TASKTRACE_DATA_ROOT", t.TempDir())
	_, err := WriteSettings(Settings{CheckIntervalMinutes: 1})
	require.Error(t, err)
	_, err = WriteSettings(Settings{CheckIntervalMinutes: 10081})
	require.Error(t, err)
}
