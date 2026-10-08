// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"xorm.io/xorm"
)

type TaskTraceDataCandidate struct {
	DataDirectory     string    `json:"data_directory" readOnly:"true" doc:"Absolute directory containing the detected TaskTrace database and local files."`
	TeamDataDirectory string    `json:"team_data_directory,omitempty" readOnly:"true" doc:"Detected teamData directory next to the old program, when present."`
	DatabaseSize      int64     `json:"database_size" readOnly:"true" doc:"Size of tasktrace.db in bytes."`
	ModifiedAt        time.Time `json:"modified_at" readOnly:"true" doc:"Last modification time of tasktrace.db."`
	Tasks             int64     `json:"tasks" readOnly:"true" doc:"Number of task rows found in the candidate database."`
	Projects          int64     `json:"projects" readOnly:"true" doc:"Number of project rows found in the candidate database."`
	Users             int64     `json:"users" readOnly:"true" doc:"Number of user rows found in the candidate database."`
}

type TaskTraceDataDetection struct {
	CurrentDataDirectory     string                   `json:"current_data_directory" readOnly:"true" doc:"Data directory used by the running TaskTrace process."`
	CurrentTeamDataDirectory string                   `json:"current_team_data_directory" readOnly:"true" doc:"teamData directory used by the running TaskTrace process."`
	Candidates               []TaskTraceDataCandidate `json:"candidates" readOnly:"true" doc:"Valid old TaskTrace datasets found outside the current data directory."`
}

type TaskTraceDataImportRequest struct {
	DataDirectory     string `json:"data_directory" minLength:"1" doc:"Detected old TaskTrace data directory to copy into protected local storage."`
	TeamDataDirectory string `json:"team_data_directory,omitempty" doc:"Optional old teamData directory to copy together with the personal data."`
}

type TaskTraceDataImportResult struct {
	DataDirectory     string `json:"data_directory" readOnly:"true" doc:"New protected data directory prepared from the old dataset."`
	TeamDataDirectory string `json:"team_data_directory" readOnly:"true" doc:"New protected teamData directory prepared from the old dataset."`
	BackupSettings    string `json:"backup_settings,omitempty" readOnly:"true" doc:"Backup of the previous tasktrace-settings.json, when one existed."`
	RestartRequired   bool   `json:"restart_required" readOnly:"true" doc:"Whether TaskTrace must be restarted before it uses the imported data."`
}

func Enabled() bool {
	return packageRoot() != "" && dataRoot() != "" && teamDataRoot() != ""
}

func Detect(extraPath string) (TaskTraceDataDetection, error) {
	if !Enabled() {
		return TaskTraceDataDetection{}, errors.New("TaskTrace local data recovery is unavailable")
	}

	currentData, err := canonicalDirectory(dataRoot())
	if err != nil {
		return TaskTraceDataDetection{}, fmt.Errorf("resolve current data directory: %w", err)
	}
	currentTeam, err := canonicalDirectory(teamDataRoot())
	if err != nil {
		return TaskTraceDataDetection{}, fmt.Errorf("resolve current team data directory: %w", err)
	}

	paths := knownCandidatePaths()
	if strings.TrimSpace(extraPath) != "" {
		manual, manualErr := resolveCandidateDirectory(extraPath)
		if manualErr != nil {
			return TaskTraceDataDetection{}, manualErr
		}
		paths = append(paths, manual)
	}

	seen := map[string]bool{}
	candidates := make([]TaskTraceDataCandidate, 0, len(paths))
	for _, path := range paths {
		candidatePath, candidateErr := canonicalDirectory(path)
		if candidateErr != nil || samePath(candidatePath, currentData) {
			continue
		}
		key := strings.ToLower(candidatePath)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidate, inspectErr := inspectCandidate(candidatePath)
		if inspectErr == nil {
			candidates = append(candidates, candidate)
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ModifiedAt.After(candidates[j].ModifiedAt)
	})

	return TaskTraceDataDetection{
		CurrentDataDirectory:     currentData,
		CurrentTeamDataDirectory: currentTeam,
		Candidates:               candidates,
	}, nil
}

func Import(request TaskTraceDataImportRequest) (TaskTraceDataImportResult, error) {
	if !Enabled() {
		return TaskTraceDataImportResult{}, errors.New("TaskTrace local data recovery is unavailable")
	}

	sourceData, err := resolveCandidateDirectory(request.DataDirectory)
	if err != nil {
		return TaskTraceDataImportResult{}, err
	}
	currentData, err := canonicalDirectory(dataRoot())
	if err != nil {
		return TaskTraceDataImportResult{}, fmt.Errorf("resolve current data directory: %w", err)
	}
	if samePath(sourceData, currentData) {
		return TaskTraceDataImportResult{}, errors.New("the selected directory is already in use")
	}
	sourceCandidate, err := inspectCandidate(sourceData)
	if err != nil {
		return TaskTraceDataImportResult{}, err
	}
	if sourceCandidate.Tasks == 0 && sourceCandidate.Projects == 0 && sourceCandidate.Users == 0 {
		return TaskTraceDataImportResult{}, errors.New("the selected database does not contain TaskTrace data")
	}

	stamp := time.Now().Format("20060102-150405.000000000")
	destinationData := filepath.Join(packageRoot(), "data", "imports", stamp)
	destinationTeam := filepath.Join(packageRoot(), "teamData", "imports", stamp)
	stagingRoot := filepath.Join(packageRoot(), ".cache", "data-import-staging", stamp)
	stagedData := filepath.Join(stagingRoot, "data")
	stagedTeam := filepath.Join(stagingRoot, "teamData")
	defer os.RemoveAll(stagingRoot)
	if err = copyDirectory(sourceData, stagedData); err != nil {
		return TaskTraceDataImportResult{}, fmt.Errorf("copy old data: %w", err)
	}

	copiedCandidate, err := inspectCandidate(stagedData)
	if err != nil || copiedCandidate.Tasks != sourceCandidate.Tasks || copiedCandidate.Projects != sourceCandidate.Projects || copiedCandidate.Users != sourceCandidate.Users {
		if err != nil {
			return TaskTraceDataImportResult{}, fmt.Errorf("verify copied data: %w", err)
		}
		return TaskTraceDataImportResult{}, errors.New("copied database verification did not match the source")
	}

	sourceTeam := strings.TrimSpace(request.TeamDataDirectory)
	if sourceTeam == "" {
		sourceTeam = sourceCandidate.TeamDataDirectory
	}
	if sourceTeam != "" {
		sourceTeam, err = canonicalDirectory(sourceTeam)
		if err != nil {
			return TaskTraceDataImportResult{}, fmt.Errorf("resolve old team data directory: %w", err)
		}
		if err = copyDirectory(sourceTeam, stagedTeam); err != nil {
			return TaskTraceDataImportResult{}, fmt.Errorf("copy old team data: %w", err)
		}
	} else if err = os.MkdirAll(stagedTeam, 0o700); err != nil {
		return TaskTraceDataImportResult{}, fmt.Errorf("prepare imported team data directory: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(destinationData), 0o700); err != nil {
		return TaskTraceDataImportResult{}, fmt.Errorf("prepare protected data directory: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(destinationTeam), 0o700); err != nil {
		return TaskTraceDataImportResult{}, fmt.Errorf("prepare protected team data directory: %w", err)
	}
	if err = os.Rename(stagedData, destinationData); err != nil {
		return TaskTraceDataImportResult{}, fmt.Errorf("activate protected data directory: %w", err)
	}
	if err = os.Rename(stagedTeam, destinationTeam); err != nil {
		_ = os.RemoveAll(destinationData)
		return TaskTraceDataImportResult{}, fmt.Errorf("activate protected team data directory: %w", err)
	}

	backupSettings, err := writeImportedSettings(destinationData, destinationTeam, stamp)
	if err != nil {
		_ = os.RemoveAll(destinationData)
		_ = os.RemoveAll(destinationTeam)
		return TaskTraceDataImportResult{}, err
	}

	return TaskTraceDataImportResult{
		DataDirectory:     destinationData,
		TeamDataDirectory: destinationTeam,
		BackupSettings:    backupSettings,
		RestartRequired:   true,
	}, nil
}

func inspectCandidate(dataDirectory string) (TaskTraceDataCandidate, error) {
	databasePath := filepath.Join(dataDirectory, "tasktrace.db")
	info, err := os.Stat(databasePath)
	if err != nil {
		return TaskTraceDataCandidate{}, fmt.Errorf("read candidate database %s: %w", databasePath, err)
	}
	if !info.Mode().IsRegular() {
		return TaskTraceDataCandidate{}, fmt.Errorf("candidate database %s is not a file", databasePath)
	}

	engine, err := xorm.NewEngine("sqlite3", "file:"+filepath.ToSlash(databasePath)+"?mode=ro&_busy_timeout=2000")
	if err != nil {
		return TaskTraceDataCandidate{}, fmt.Errorf("open candidate database: %w", err)
	}
	defer engine.Close()

	counts := make([]int64, 3)
	for index, table := range []string{"tasks", "projects", "users"} {
		counts[index], err = engine.Table(table).Count()
		if err != nil {
			return TaskTraceDataCandidate{}, fmt.Errorf("inspect candidate table %s: %w", table, err)
		}
	}

	teamDirectory := filepath.Join(filepath.Dir(dataDirectory), "teamData")
	if teamInfo, teamErr := os.Stat(teamDirectory); teamErr != nil || !teamInfo.IsDir() {
		teamDirectory = ""
	}
	return TaskTraceDataCandidate{
		DataDirectory:     dataDirectory,
		TeamDataDirectory: teamDirectory,
		DatabaseSize:      info.Size(),
		ModifiedAt:        info.ModTime(),
		Tasks:             counts[0],
		Projects:          counts[1],
		Users:             counts[2],
	}, nil
}

func resolveCandidateDirectory(path string) (string, error) {
	path = strings.TrimSpace(os.ExpandEnv(path))
	if path == "" {
		return "", errors.New("select an old TaskTrace data directory")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve selected path: %w", err)
	}
	if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() && strings.EqualFold(filepath.Base(absolute), "tasktrace.db") {
		absolute = filepath.Dir(absolute)
	}
	for _, candidate := range []string{absolute, filepath.Join(absolute, "data")} {
		if info, statErr := os.Stat(filepath.Join(candidate, "tasktrace.db")); statErr == nil && info.Mode().IsRegular() {
			return canonicalDirectory(candidate)
		}
	}
	return "", fmt.Errorf("no tasktrace.db was found in %s or its data subdirectory", absolute)
}

func knownCandidatePaths() []string {
	root := packageRoot()
	paths := []string{filepath.Join(root, "data")}
	parents := []string{filepath.Dir(root), filepath.Dir(filepath.Dir(root))}
	for _, parent := range parents {
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() || strings.EqualFold(entry.Name(), ".local-build") || strings.EqualFold(entry.Name(), "node_modules") {
				continue
			}
			entryRoot := filepath.Join(parent, entry.Name())
			paths = append(paths,
				filepath.Join(entryRoot, "data"),
				filepath.Join(entryRoot, "TaskTrace-local", "data"),
				filepath.Join(entryRoot, "dist", "TaskTrace-local", "data"),
				filepath.Join(entryRoot, "Releases", "TaskTrace-local", "data"),
			)
		}
	}
	return paths
}

func writeImportedSettings(dataDirectory, teamDirectory, stamp string) (string, error) {
	settingsPath := filepath.Join(packageRoot(), "tasktrace-settings.json")
	settings := map[string]any{}
	content, err := os.ReadFile(settingsPath)
	backup := ""
	if err == nil {
		if err = json.Unmarshal(content, &settings); err != nil {
			return "", fmt.Errorf("read existing TaskTrace settings: %w", err)
		}
		backup = filepath.Join(packageRoot(), "tasktrace-settings.backup."+stamp+".json")
		if err = os.WriteFile(backup, content, 0o600); err != nil {
			return "", fmt.Errorf("back up existing TaskTrace settings: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read existing TaskTrace settings: %w", err)
	}

	settings["dataDirectory"] = portablePath(dataDirectory)
	settings["teamDataDirectory"] = portablePath(teamDirectory)
	next, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return backup, fmt.Errorf("encode imported TaskTrace settings: %w", err)
	}
	temporary := settingsPath + ".tmp"
	if err = os.WriteFile(temporary, next, 0o600); err != nil {
		return backup, fmt.Errorf("write imported TaskTrace settings: %w", err)
	}
	if err = os.Remove(settingsPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(temporary)
		return backup, fmt.Errorf("replace imported TaskTrace settings: %w", err)
	}
	if err = os.Rename(temporary, settingsPath); err != nil {
		_ = os.Remove(temporary)
		if backup != "" {
			if content, backupErr := os.ReadFile(backup); backupErr == nil {
				_ = os.WriteFile(settingsPath, content, 0o600)
			}
		}
		return backup, fmt.Errorf("activate imported TaskTrace settings: %w", err)
	}
	return backup, nil
}

func portablePath(path string) string {
	relative, err := filepath.Rel(packageRoot(), path)
	if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(relative)
	}
	return filepath.ToSlash(path)
}

func copyDirectory(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		temporary := target + ".tmp"
		output, err := os.OpenFile(temporary, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		closeOutputErr := output.Close()
		closeInputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeOutputErr != nil {
			return closeOutputErr
		}
		if closeInputErr != nil {
			return closeInputErr
		}
		return os.Rename(temporary, target)
	})
}

func canonicalDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", err
	}
	return filepath.Clean(absolute), nil
}

func samePath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func packageRoot() string  { return strings.TrimSpace(os.Getenv("TASKTRACE_PACKAGE_ROOT")) }
func dataRoot() string     { return strings.TrimSpace(os.Getenv("TASKTRACE_DATA_ROOT")) }
func teamDataRoot() string { return strings.TrimSpace(os.Getenv("TASKTRACE_TEAM_ROOT")) }
