// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"xorm.io/xorm"
)

const (
	defaultBackupDirectory     = "backups"
	defaultBackupDailyTime     = "02:00"
	defaultBackupRetentionDays = 30
	defaultBackupMinimumCount  = 3
	backupDirectoryPrefix      = "TaskTrace-backup-"
)

type TaskTraceBackupSettings struct {
	Enabled        bool   `json:"enabled" doc:"Whether TaskTrace periodically checks for data changes and creates a backup."`
	Directory      string `json:"directory" minLength:"1" maxLength:"1024" doc:"Directory where TaskTrace backup folders are stored. Relative paths are resolved from the portable program directory."`
	DailyTime      string `json:"daily_time" pattern:"^(?:[01]\\d|2[0-3]):[0-5]\\d$" doc:"Local time of day when the automatic backup runs, in HH:mm format."`
	RetentionDays  int    `json:"retention_days" minimum:"1" maximum:"3650" doc:"Backups older than this many days may be removed."`
	MinimumBackups int    `json:"minimum_backups" minimum:"1" maximum:"1000" doc:"Minimum number of newest backups retained even when they are older than the retention period."`
}

type TaskTraceBackupStatus struct {
	Directory     string     `json:"directory" readOnly:"true" doc:"Resolved absolute backup directory."`
	BackupCount   int        `json:"backup_count" readOnly:"true" doc:"Number of valid backups currently stored in the configured directory."`
	LastBackupAt  *time.Time `json:"last_backup_at,omitempty" readOnly:"true" doc:"Creation time of the newest stored backup."`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty" readOnly:"true" doc:"Time of the most recent automatic or manual backup check in this process."`
	LastMessage   string     `json:"last_message,omitempty" readOnly:"true" doc:"Result of the most recent backup check."`
	NextCheckAt   *time.Time `json:"next_check_at,omitempty" readOnly:"true" doc:"Estimated time of the next automatic check when scheduled backups are enabled."`
	Running       bool       `json:"running" readOnly:"true" doc:"Whether a backup check is currently running."`
}

type TaskTraceBackupRunResult struct {
	Created   bool      `json:"created" readOnly:"true" doc:"Whether a new backup was created."`
	Skipped   bool      `json:"skipped" readOnly:"true" doc:"Whether creation was skipped because the data was unchanged."`
	Directory string    `json:"directory,omitempty" readOnly:"true" doc:"New backup directory, or the configured backup root when creation was skipped."`
	CreatedAt time.Time `json:"created_at" readOnly:"true" doc:"Time when this backup check completed."`
	Digest    string    `json:"digest,omitempty" readOnly:"true" doc:"SHA-256 content digest used to detect unchanged data."`
	Removed   int       `json:"removed" readOnly:"true" doc:"Number of expired backup folders removed after the check."`
	Message   string    `json:"message" readOnly:"true" doc:"Human-readable result of the backup check."`
}

type backupManifest struct {
	Version          int       `json:"version"`
	CreatedAt        time.Time `json:"created_at"`
	Digest           string    `json:"digest"`
	SourceData       string    `json:"source_data_directory"`
	SourceTeamData   string    `json:"source_team_data_directory"`
	TaskTraceVersion string    `json:"tasktrace_version,omitempty"`
}

type backupEntry struct {
	path     string
	manifest backupManifest
}

var (
	backupRunMu       sync.Mutex
	backupSettingsMu  sync.Mutex
	backupStatusMu    sync.RWMutex
	backupLastChecked time.Time
	backupLastMessage string
	backupRunning     bool
	backupScheduler   sync.Once
	backupWake        = make(chan struct{}, 1)
)

func DefaultBackupSettings() TaskTraceBackupSettings {
	return TaskTraceBackupSettings{
		Directory:      defaultBackupDirectory,
		DailyTime:      defaultBackupDailyTime,
		RetentionDays:  defaultBackupRetentionDays,
		MinimumBackups: defaultBackupMinimumCount,
	}
}

func ReadBackupSettings() (TaskTraceBackupSettings, error) {
	backupSettingsMu.Lock()
	defer backupSettingsMu.Unlock()
	return readBackupSettingsUnlocked()
}

func WriteBackupSettings(settings TaskTraceBackupSettings) (TaskTraceBackupSettings, error) {
	settings = normalizeBackupSettings(settings)
	if err := validateBackupSettings(settings); err != nil {
		return TaskTraceBackupSettings{}, err
	}
	directory, err := resolveBackupDirectory(settings.Directory)
	if err != nil {
		return TaskTraceBackupSettings{}, err
	}
	if err = ensureSafeBackupDirectory(directory); err != nil {
		return TaskTraceBackupSettings{}, err
	}

	backupSettingsMu.Lock()
	defer backupSettingsMu.Unlock()
	path := filepath.Join(packageRoot(), "tasktrace-settings.json")
	root := map[string]any{}
	content, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(content, &root); err != nil {
			return TaskTraceBackupSettings{}, fmt.Errorf("read TaskTrace settings: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return TaskTraceBackupSettings{}, fmt.Errorf("read TaskTrace settings: %w", err)
	}
	root["backup"] = settings
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return TaskTraceBackupSettings{}, fmt.Errorf("encode backup settings: %w", err)
	}
	if err = writeFileAtomic(path, encoded, 0o600); err != nil {
		return TaskTraceBackupSettings{}, fmt.Errorf("save backup settings: %w", err)
	}
	wakeBackupScheduler()
	return settings, nil
}

func BackupStatus() (TaskTraceBackupStatus, error) {
	settings, err := ReadBackupSettings()
	if err != nil {
		return TaskTraceBackupStatus{}, err
	}
	directory, err := resolveBackupDirectory(settings.Directory)
	if err != nil {
		return TaskTraceBackupStatus{}, err
	}
	entries, err := listBackups(directory)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return TaskTraceBackupStatus{}, err
	}
	backupStatusMu.RLock()
	lastChecked := backupLastChecked
	message := backupLastMessage
	running := backupRunning
	backupStatusMu.RUnlock()

	status := TaskTraceBackupStatus{Directory: directory, BackupCount: len(entries), LastMessage: message, Running: running}
	if len(entries) > 0 {
		latest := entries[0].manifest.CreatedAt
		status.LastBackupAt = &latest
	}
	if !lastChecked.IsZero() {
		status.LastCheckedAt = &lastChecked
	}
	if settings.Enabled {
		next := nextBackupTime(settings, time.Now())
		status.NextCheckAt = &next
	}
	return status, nil
}

func RunBackup() (TaskTraceBackupRunResult, error) {
	backupRunMu.Lock()
	defer backupRunMu.Unlock()
	setBackupRunState(true, "")
	result, err := runBackup(time.Now())
	if err != nil {
		setBackupRunState(false, err.Error())
		return TaskTraceBackupRunResult{}, err
	}
	setBackupRunState(false, result.Message)
	return result, nil
}

func StartBackupScheduler() {
	if !Enabled() {
		return
	}
	backupScheduler.Do(func() {
		go backupSchedulerLoop(context.Background())
	})
}

func backupSchedulerLoop(ctx context.Context) {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-backupWake:
		case <-timer.C:
		}
		settings, err := ReadBackupSettings()
		if err == nil && settings.Enabled && backupCheckDue(settings, time.Now()) {
			_, _ = RunBackup()
		}
		timer.Reset(30 * time.Second)
	}
}

func backupCheckDue(settings TaskTraceBackupSettings, now time.Time) bool {
	backupStatusMu.RLock()
	lastChecked := backupLastChecked
	running := backupRunning
	backupStatusMu.RUnlock()
	if running {
		return false
	}
	scheduled := scheduledBackupTime(settings, now)
	if now.Before(scheduled) {
		return false
	}
	if !lastChecked.IsZero() && !lastChecked.Before(scheduled) {
		return false
	}
	directory, err := resolveBackupDirectory(settings.Directory)
	if err != nil {
		return false
	}
	entries, _ := listBackups(directory)
	return len(entries) == 0 || entries[0].manifest.CreatedAt.Before(scheduled)
}

func scheduledBackupTime(settings TaskTraceBackupSettings, day time.Time) time.Time {
	hour, minute, _ := parseBackupDailyTime(settings.DailyTime)
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, day.Location())
}

func nextBackupTime(settings TaskTraceBackupSettings, now time.Time) time.Time {
	next := scheduledBackupTime(settings, now)
	if !now.Before(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

func parseBackupDailyTime(value string) (int, int, error) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, 0, errors.New("backup time must use HH:mm format")
	}
	return parsed.Hour(), parsed.Minute(), nil
}

func runBackup(now time.Time) (TaskTraceBackupRunResult, error) {
	if !Enabled() {
		return TaskTraceBackupRunResult{}, errors.New("TaskTrace local data backup is unavailable")
	}
	settings, err := ReadBackupSettings()
	if err != nil {
		return TaskTraceBackupRunResult{}, err
	}
	backupRoot, err := resolveBackupDirectory(settings.Directory)
	if err != nil {
		return TaskTraceBackupRunResult{}, err
	}
	if err = ensureSafeBackupDirectory(backupRoot); err != nil {
		return TaskTraceBackupRunResult{}, err
	}
	if err = os.MkdirAll(backupRoot, 0o700); err != nil {
		return TaskTraceBackupRunResult{}, fmt.Errorf("create backup directory: %w", err)
	}

	stamp := now.Format("20060102-150405.000000000")
	staging := filepath.Join(backupRoot, ".tasktrace-staging-"+stamp)
	final := filepath.Join(backupRoot, backupDirectoryPrefix+stamp)
	defer os.RemoveAll(staging)
	if err = snapshotData(filepath.Join(staging, "data")); err != nil {
		return TaskTraceBackupRunResult{}, err
	}
	if err = copyDirectoryFiltered(teamDataRoot(), filepath.Join(staging, "teamData"), nil); err != nil {
		return TaskTraceBackupRunResult{}, fmt.Errorf("copy team data: %w", err)
	}
	digest, err := directoryDigest(staging)
	if err != nil {
		return TaskTraceBackupRunResult{}, fmt.Errorf("calculate backup digest: %w", err)
	}
	entries, err := listBackups(backupRoot)
	if err != nil {
		return TaskTraceBackupRunResult{}, err
	}
	if len(entries) > 0 && entries[0].manifest.Digest == digest {
		removed, pruneErr := pruneBackups(backupRoot, settings, now)
		if pruneErr != nil {
			return TaskTraceBackupRunResult{}, pruneErr
		}
		return TaskTraceBackupRunResult{Skipped: true, Directory: backupRoot, CreatedAt: now, Digest: digest, Removed: removed, Message: "数据没有变化，已跳过本次备份。"}, nil
	}
	manifest := backupManifest{Version: 1, CreatedAt: now, Digest: digest, SourceData: dataRoot(), SourceTeamData: teamDataRoot()}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return TaskTraceBackupRunResult{}, err
	}
	if err = writeFileAtomic(filepath.Join(staging, "backup-manifest.json"), manifestBytes, 0o600); err != nil {
		return TaskTraceBackupRunResult{}, fmt.Errorf("write backup manifest: %w", err)
	}
	if err = os.Rename(staging, final); err != nil {
		return TaskTraceBackupRunResult{}, fmt.Errorf("publish backup: %w", err)
	}
	removed, pruneErr := pruneBackups(backupRoot, settings, now)
	if pruneErr != nil {
		return TaskTraceBackupRunResult{}, pruneErr
	}
	return TaskTraceBackupRunResult{Created: true, Directory: final, CreatedAt: now, Digest: digest, Removed: removed, Message: "备份已完成。"}, nil
}

func snapshotData(destination string) error {
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return fmt.Errorf("prepare data backup: %w", err)
	}
	databaseSource := filepath.Join(dataRoot(), "tasktrace.db")
	databaseDestination := filepath.Join(destination, "tasktrace.db")
	if err := snapshotSQLite(databaseSource, databaseDestination); err != nil {
		return err
	}
	// The launcher keeps its session lock and redirected logs open with
	// FileShare.None on Windows. They are transient process state, not task data.
	// Match only root-relative names so identically named attachments stay intact.
	skip := map[string]bool{
		"tasktrace.db":         true,
		"tasktrace.db-wal":     true,
		"tasktrace.db-shm":     true,
		"tasktrace.db-journal": true,
		"session.lock":         true,
		"server.log":           true,
		"server-error.log":     true,
	}
	if err := copyDirectoryFiltered(dataRoot(), destination, func(relative string, _ fs.DirEntry) bool {
		return !skip[strings.ToLower(filepath.ToSlash(relative))]
	}); err != nil {
		return fmt.Errorf("copy personal data: %w", err)
	}
	return nil
}

func snapshotSQLite(source, destination string) error {
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("read TaskTrace database: %w", err)
	}
	engine, err := xorm.NewEngine("sqlite3", "file:"+filepath.ToSlash(source)+"?_busy_timeout=10000")
	if err != nil {
		return fmt.Errorf("open TaskTrace database for backup: %w", err)
	}
	defer engine.Close()
	quoted := strings.ReplaceAll(filepath.ToSlash(destination), "'", "''")
	if _, err = engine.Exec("VACUUM INTO '" + quoted + "'"); err != nil {
		return fmt.Errorf("create consistent database backup: %w", err)
	}
	return nil
}

func copyDirectoryFiltered(source, destination string, include func(string, fs.DirEntry) bool) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative != "." && include != nil && !include(relative, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported: %s", path)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		outputErr := output.Close()
		inputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if outputErr != nil {
			return outputErr
		}
		return inputErr
	})
}

func directoryDigest(root string) (string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not supported: %s", path)
		}
		if !entry.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(hash, filepath.ToSlash(relative)+"\x00")
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func listBackups(root string) ([]backupEntry, error) {
	directoryEntries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read backup directory: %w", err)
	}
	backups := make([]backupEntry, 0)
	for _, entry := range directoryEntries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), backupDirectoryPrefix) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		content, readErr := os.ReadFile(filepath.Join(path, "backup-manifest.json"))
		if readErr != nil {
			continue
		}
		manifest := backupManifest{}
		if json.Unmarshal(content, &manifest) != nil || manifest.CreatedAt.IsZero() || manifest.Digest == "" {
			continue
		}
		backups = append(backups, backupEntry{path: path, manifest: manifest})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].manifest.CreatedAt.After(backups[j].manifest.CreatedAt) })
	return backups, nil
}

func pruneBackups(root string, settings TaskTraceBackupSettings, now time.Time) (int, error) {
	entries, err := listBackups(root)
	if err != nil {
		return 0, err
	}
	cutoff := now.Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)
	removed := 0
	for index, entry := range entries {
		if index < settings.MinimumBackups || !entry.manifest.CreatedAt.Before(cutoff) {
			continue
		}
		if err = os.RemoveAll(entry.path); err != nil {
			return removed, fmt.Errorf("remove expired backup %s: %w", entry.path, err)
		}
		removed++
	}
	return removed, nil
}

func readBackupSettingsUnlocked() (TaskTraceBackupSettings, error) {
	settings := DefaultBackupSettings()
	content, err := os.ReadFile(filepath.Join(packageRoot(), "tasktrace-settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return TaskTraceBackupSettings{}, fmt.Errorf("read TaskTrace settings: %w", err)
	}
	root := struct {
		Backup *TaskTraceBackupSettings `json:"backup"`
	}{}
	if err = json.Unmarshal(content, &root); err != nil {
		return TaskTraceBackupSettings{}, fmt.Errorf("read TaskTrace settings: %w", err)
	}
	if root.Backup != nil {
		settings = normalizeBackupSettings(*root.Backup)
	}
	return settings, validateBackupSettings(settings)
}

func normalizeBackupSettings(settings TaskTraceBackupSettings) TaskTraceBackupSettings {
	settings.Directory = strings.TrimSpace(settings.Directory)
	if settings.Directory == "" {
		settings.Directory = defaultBackupDirectory
	}
	settings.DailyTime = strings.TrimSpace(settings.DailyTime)
	if settings.DailyTime == "" {
		settings.DailyTime = defaultBackupDailyTime
	}
	if settings.RetentionDays == 0 {
		settings.RetentionDays = defaultBackupRetentionDays
	}
	if settings.MinimumBackups == 0 {
		settings.MinimumBackups = defaultBackupMinimumCount
	}
	return settings
}

func validateBackupSettings(settings TaskTraceBackupSettings) error {
	if _, _, err := parseBackupDailyTime(settings.DailyTime); err != nil {
		return err
	}
	if settings.RetentionDays < 1 || settings.RetentionDays > 3650 {
		return errors.New("backup retention must be between 1 and 3650 days")
	}
	if settings.MinimumBackups < 1 || settings.MinimumBackups > 1000 {
		return errors.New("minimum backups must be between 1 and 1000")
	}
	return nil
}

func resolveBackupDirectory(configured string) (string, error) {
	configured = strings.TrimSpace(os.ExpandEnv(configured))
	if configured == "" {
		configured = defaultBackupDirectory
	}
	if !filepath.IsAbs(configured) {
		configured = filepath.Join(packageRoot(), configured)
	}
	return canonicalDirectory(configured)
}

func ensureSafeBackupDirectory(directory string) error {
	for _, source := range []string{dataRoot(), teamDataRoot()} {
		canonicalSource, err := canonicalDirectory(source)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(canonicalSource, directory)
		if err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)))) {
			return fmt.Errorf("backup directory cannot be inside %s", canonicalSource)
		}
	}
	return nil
}

func writeFileAtomic(path string, content []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, content, mode); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err == nil {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func setBackupRunState(running bool, message string) {
	backupStatusMu.Lock()
	defer backupStatusMu.Unlock()
	backupRunning = running
	if !running {
		backupLastChecked = time.Now()
		backupLastMessage = message
	}
}

func wakeBackupScheduler() {
	select {
	case backupWake <- struct{}{}:
	default:
	}
}
