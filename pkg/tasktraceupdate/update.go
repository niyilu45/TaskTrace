// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktraceupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/version"
)

const (
	defaultIntervalMinutes = 60
	minIntervalMinutes     = 5
	maxIntervalMinutes     = 7 * 24 * 60
)

var fileMu sync.Mutex

type Settings struct {
	CheckIntervalMinutes int    `json:"check_interval_minutes" minimum:"5" maximum:"10080" doc:"Minutes between automatic GitHub release checks. Defaults to 60."`
	IgnoredVersion       string `json:"ignored_version" doc:"Release version the user declined. Automatic checks do not notify for this version."`
}

type State struct {
	CurrentVersion string    `json:"current_version" readOnly:"true" doc:"Version of the running TaskTrace package."`
	LatestVersion  string    `json:"latest_version" readOnly:"true" doc:"Newest compatible GitHub release version found by the desktop process."`
	Available      bool      `json:"available" readOnly:"true" doc:"Whether a release newer than the running version is available."`
	Notify         bool      `json:"notify" readOnly:"true" doc:"Whether the full interface should show the new-version notification."`
	PublishedAt    time.Time `json:"published_at,omitempty" readOnly:"true" doc:"GitHub publication time for the newest release."`
	ReleaseNotes   string    `json:"release_notes" readOnly:"true" doc:"Release notes supplied by the GitHub release."`
	ReleaseURL     string    `json:"release_url" readOnly:"true" doc:"GitHub page for the release."`
	AssetURL       string    `json:"asset_url" readOnly:"true" doc:"GitHub API download URL for the portable Windows archive."`
	ChecksumURL    string    `json:"checksum_url" readOnly:"true" doc:"GitHub API download URL for SHA256SUMS.txt when present."`
	CheckedAt      time.Time `json:"checked_at,omitempty" readOnly:"true" doc:"Time the desktop process completed the latest check."`
	Status         string    `json:"status" readOnly:"true" doc:"Current updater state: idle, checking, downloading, ready, or error."`
	Error          string    `json:"error" readOnly:"true" doc:"Last update error suitable for display to the local user."`
}

type Command struct {
	ID        string    `json:"id" readOnly:"true" doc:"Unique command identifier."`
	Action    string    `json:"action" enum:"check,install" doc:"Desktop update action to execute."`
	Version   string    `json:"version,omitempty" doc:"Release version to install."`
	CreatedAt time.Time `json:"created_at" readOnly:"true" doc:"Time the command was queued."`
}

func Enabled() bool { return dataRoot() != "" }

func CurrentVersion() string {
	v := strings.TrimSpace(version.Version)
	if v == "" || v == "dev" || v == "tasktrace-local" {
		return "v0.0.0"
	}
	if strings.HasPrefix(v, "tasktrace-local/") {
		v = strings.TrimPrefix(v, "tasktrace-local/")
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	return v
}

func ReadSettings() (Settings, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	return readSettingsLocked()
}

func WriteSettings(next Settings) (Settings, error) {
	if next.CheckIntervalMinutes < minIntervalMinutes || next.CheckIntervalMinutes > maxIntervalMinutes {
		return Settings{}, fmt.Errorf("check interval must be between %d and %d minutes", minIntervalMinutes, maxIntervalMinutes)
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	if err := writeJSONLocked(settingsPath(), next); err != nil {
		return Settings{}, err
	}
	return next, nil
}

func ReadState() (State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	state := State{CurrentVersion: CurrentVersion(), Status: "idle"}
	err := readJSONLocked(statePath(), &state)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return State{}, err
	}
	state.CurrentVersion = CurrentVersion()
	if state.Status == "" {
		state.Status = "idle"
	}
	settings, settingsErr := readSettingsLocked()
	if settingsErr != nil {
		return State{}, settingsErr
	}
	if state.LatestVersion != "" && state.LatestVersion == settings.IgnoredVersion {
		state.Notify = false
	}
	return state, nil
}

func Queue(action, releaseVersion string) (Command, error) {
	if action != "check" && action != "install" {
		return Command{}, fmt.Errorf("unsupported update action %q", action)
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	command := Command{ID: fmt.Sprintf("%d", time.Now().UnixNano()), Action: action, Version: strings.TrimSpace(releaseVersion), CreatedAt: time.Now().UTC()}
	if err := writeJSONLocked(commandPath(), command); err != nil {
		return Command{}, err
	}
	return command, nil
}

func Ignore(releaseVersion string) (Settings, State, error) {
	fileMu.Lock()
	defer fileMu.Unlock()
	settings, err := readSettingsLocked()
	if err != nil {
		return Settings{}, State{}, err
	}
	settings.IgnoredVersion = strings.TrimSpace(releaseVersion)
	if err = writeJSONLocked(settingsPath(), settings); err != nil {
		return Settings{}, State{}, err
	}
	state := State{CurrentVersion: CurrentVersion(), Status: "idle"}
	if stateErr := readJSONLocked(statePath(), &state); stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return Settings{}, State{}, stateErr
	}
	state.CurrentVersion = CurrentVersion()
	if state.LatestVersion == settings.IgnoredVersion {
		state.Notify = false
		if err = writeJSONLocked(statePath(), state); err != nil {
			return Settings{}, State{}, err
		}
	}
	return settings, state, nil
}

func readSettingsLocked() (Settings, error) {
	settings := Settings{CheckIntervalMinutes: defaultIntervalMinutes}
	err := readJSONLocked(settingsPath(), &settings)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Settings{}, err
	}
	if settings.CheckIntervalMinutes < minIntervalMinutes || settings.CheckIntervalMinutes > maxIntervalMinutes {
		settings.CheckIntervalMinutes = defaultIntervalMinutes
	}
	return settings, nil
}

func dataRoot() string     { return strings.TrimSpace(os.Getenv("TASKTRACE_DATA_ROOT")) }
func settingsPath() string { return filepath.Join(dataRoot(), "update-settings.json") }
func statePath() string    { return filepath.Join(dataRoot(), "update-state.json") }
func commandPath() string  { return filepath.Join(dataRoot(), "update-command.json") }

func readJSONLocked(path string, target any) error {
	if path == "" {
		return errors.New("TaskTrace update storage is unavailable")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, target)
}

func writeJSONLocked(path string, value any) error {
	if path == "" {
		return errors.New("TaskTrace update storage is unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err = os.WriteFile(temporary, content, 0600); err != nil {
		return err
	}
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporary, path)
}
