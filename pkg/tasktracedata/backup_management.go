// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

type TaskTraceBackupNote struct {
	Note string `json:"note" maxLength:"2000" doc:"User note for this backup, up to 2000 characters. An empty string clears it."`
}

type TaskTraceRecoverableBackup struct {
	ID        string                 `json:"id" doc:"Opaque identifier bound to this backup and its configured directory."`
	CreatedAt time.Time              `json:"created_at" doc:"Original backup creation time; editing a note does not change it."`
	Note      string                 `json:"note" doc:"User note stored with this backup."`
	Candidate TaskTraceDataCandidate `json:"candidate" doc:"Validated personal and team data available for recovery."`
}

type TaskTraceBackupList struct {
	Directory string                       `json:"directory" doc:"Configured absolute backup directory used for this listing."`
	Backups   []TaskTraceRecoverableBackup `json:"backups" doc:"Recoverable backups, newest first. Unrelated folders are excluded."`
}

// ListRecoverableBackups uses the same lock as backup creation and import so a
// backup cannot be pruned or deleted while its database is being inspected.
func ListRecoverableBackups() (TaskTraceBackupList, error) {
	backupRunMu.Lock()
	defer backupRunMu.Unlock()
	root, entries, err := managedBackups()
	if err != nil {
		return TaskTraceBackupList{}, err
	}
	result := TaskTraceBackupList{
		Directory: root,
		Backups:   []TaskTraceRecoverableBackup{},
	}
	for _, entry := range entries {
		if err = validateManagedBackup(root, entry.path); err != nil {
			continue
		}
		candidate, inspectErr := inspectCandidate(filepath.Join(entry.path, "data"))
		if inspectErr != nil {
			continue
		}
		if candidate.TeamDataDirectory != "" {
			candidate.TeamShares, err = countTeamShares(candidate.TeamDataDirectory)
			if err != nil {
				return TaskTraceBackupList{}, err
			}
		}
		note, readErr := readBackupNote(entry.path)
		if readErr != nil {
			return TaskTraceBackupList{}, readErr
		}
		result.Backups = append(result.Backups, TaskTraceRecoverableBackup{
			ID:        backupID(entry),
			CreatedAt: entry.manifest.CreatedAt,
			Note:      note.Note,
			Candidate: candidate,
		})
	}
	return result, nil
}

func SaveBackupNote(id string, note TaskTraceBackupNote) (TaskTraceBackupNote, error) {
	if utf8.RuneCountInString(note.Note) > 2000 {
		return TaskTraceBackupNote{}, errors.New("backup note exceeds 2000 characters")
	}
	backupRunMu.Lock()
	defer backupRunMu.Unlock()
	entry, err := findManagedBackup(id)
	if err != nil {
		return TaskTraceBackupNote{}, err
	}
	// Keep user metadata outside data/teamData and the content digest. Adding a
	// note must not create a new backup or alter retention/restore semantics.
	encoded, err := json.MarshalIndent(note, "", "  ")
	if err != nil {
		return TaskTraceBackupNote{}, fmt.Errorf("encode backup note: %w", err)
	}
	if err = writeFileAtomic(filepath.Join(entry.path, "backup-note.json"), encoded, 0o600); err != nil {
		return TaskTraceBackupNote{}, fmt.Errorf("save backup note: %w", err)
	}
	return note, nil
}

func DeleteBackup(id string) error {
	backupRunMu.Lock()
	defer backupRunMu.Unlock()
	entry, err := findManagedBackup(id)
	if err != nil {
		return err
	}
	// Refuse linked descendants as well as a linked backup root. Never follow a
	// directory junction into live data or another user's shared directory.
	if err = filepath.WalkDir(entry.path, func(path string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		return requireUnlinkedPath(path, item.IsDir())
	}); err != nil {
		return fmt.Errorf("validate backup before deletion: %w", err)
	}
	if err = os.RemoveAll(entry.path); err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

func managedBackups() (string, []backupEntry, error) {
	if !Enabled() {
		return "", nil, errors.New("TaskTrace local data backup is unavailable")
	}
	settings, err := ReadBackupSettings()
	if err != nil {
		return "", nil, err
	}
	root, err := resolveBackupDirectory(settings.Directory)
	if err != nil {
		return "", nil, err
	}
	resolved, err := filepath.EvalSymlinks(root)
	if errors.Is(err, os.ErrNotExist) {
		return root, nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("resolve backup directory: %w", err)
	}
	entries, err := listBackups(resolved)
	return resolved, entries, err
}

func backupID(entry backupEntry) string {
	digest := sha256.Sum256([]byte(entry.path + "\x00" + entry.manifest.CreatedAt.Format(time.RFC3339Nano) + "\x00" + entry.manifest.Digest))
	return hex.EncodeToString(digest[:])
}

func findManagedBackup(id string) (backupEntry, error) {
	root, entries, err := managedBackups()
	if err != nil {
		return backupEntry{}, err
	}
	for _, entry := range entries {
		if backupID(entry) == id {
			if err = validateManagedBackup(root, entry.path); err != nil {
				return backupEntry{}, err
			}
			return entry, nil
		}
	}
	return backupEntry{}, errors.New("backup is no longer available in the configured directory; refresh the backup list")
}

func validateManagedBackup(root, path string) error {
	if !samePath(filepath.Dir(path), root) || !strings.HasPrefix(filepath.Base(path), backupDirectoryPrefix) {
		return errors.New("not a managed backup directory")
	}
	if err := requireUnlinkedPath(path, true); err != nil {
		return err
	}
	for index, source := range []string{
		packageRoot(),
		dataRoot(),
		teamDataRoot(),
	} {
		resolved, err := filepath.EvalSymlinks(source)
		if err != nil {
			return fmt.Errorf("resolve active directory: %w", err)
		}
		// The backup may be under the program folder, but may never contain it.
		if pathContains(path, resolved) || (index > 0 && pathContains(resolved, path)) {
			return errors.New("backup overlaps the active program or data directory")
		}
	}
	for _, file := range []string{
		"backup-manifest.json",
		"backup-note.json",
		"data",
		"data/tasktrace.db",
		"teamData",
	} {
		full := filepath.Join(path, file)
		if _, err := os.Lstat(full); errors.Is(err, os.ErrNotExist) && (file == "backup-note.json" || file == "teamData") {
			continue
		}
		if err := requireUnlinkedPath(full, file == "data" || file == "teamData"); err != nil {
			return err
		}
	}
	return nil
}

func pathContains(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func requireUnlinkedPath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if (directory && !info.IsDir()) || (!directory && !info.Mode().IsRegular()) {
		return fmt.Errorf("unsupported backup entry: %s", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	if !samePath(resolved, path) {
		return fmt.Errorf("linked backup entries cannot be changed: %s", path)
	}
	return nil
}

func readBackupNote(path string) (TaskTraceBackupNote, error) {
	content, err := os.ReadFile(filepath.Join(path, "backup-note.json"))
	if errors.Is(err, os.ErrNotExist) {
		return TaskTraceBackupNote{}, nil
	}
	if err != nil {
		return TaskTraceBackupNote{}, fmt.Errorf("read backup note: %w", err)
	}
	var note TaskTraceBackupNote
	if err = json.Unmarshal(content, &note); err != nil {
		return TaskTraceBackupNote{}, fmt.Errorf("decode backup note: %w", err)
	}
	return note, nil
}
