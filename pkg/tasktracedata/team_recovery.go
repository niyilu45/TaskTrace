// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func countTeamShares(root string) (int, error) {
	if root == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(filepath.Join(root, "tasks"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read team task directory: %w", err)
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		manifest, readErr := readRecoveryJSON(filepath.Join(root, "tasks", entry.Name(), "manifest.json"))
		if readErr != nil {
			return 0, fmt.Errorf("read team task %s: %w", entry.Name(), readErr)
		}
		if rawString(manifest["share_id"]) == entry.Name() {
			count++
		}
	}
	return count, nil
}

func readRecoveryJSON(path string) (map[string]json.RawMessage, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var document map[string]json.RawMessage
	err = json.Unmarshal(content, &document)
	return document, err
}

func rawString(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

// Preserve live collaboration endpoints when restoring personal data. A backup
// copy is not a replacement network repository: other members still use the
// original endpoint. Only relocate unavailable local repositories whose copied
// manifest matches the original binding.
func rebaseImportedTeamBindings(data, team, destination string) error {
	path := filepath.Join(data, "team-sync.json")
	state, err := readRecoveryJSON(path)
	if err != nil || state == nil {
		return err
	}
	var bindings []map[string]json.RawMessage
	if len(state["bindings"]) == 0 {
		return nil
	}
	if err = json.Unmarshal(state["bindings"], &bindings); err != nil {
		return err
	}
	destinationJSON, err := json.Marshal(destination)
	if err != nil {
		return err
	}
	changed := false
	for _, binding := range bindings {
		id := rawString(binding["share_id"])
		if id == "" || !filepath.IsLocal(id) || filepath.Base(id) != id || id == "." {
			continue
		}
		manifest, readErr := readRecoveryJSON(filepath.Join(team, "tasks", id, "manifest.json"))
		if readErr != nil {
			return readErr
		}
		if manifest == nil {
			continue
		}
		secret := rawString(binding["secret"])
		digest := sha256.Sum256([]byte(secret))
		if rawString(manifest["share_id"]) != id || secret == "" || rawString(manifest["token_hash"]) != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("team task %s does not match the selected personal collaboration data", id)
		}
		keep, keepErr := keepRecoveryTeamRepository(binding)
		if keepErr != nil {
			return keepErr
		}
		if keep {
			continue
		}
		binding["repository"] = destinationJSON
		delete(binding, "last_error")
		changed = true
	}
	if !changed {
		return nil
	}
	state["bindings"], err = json.Marshal(bindings)
	if err != nil {
		return err
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, content, 0o600)
}

// Network connections remain network connections even while disconnected. Do
// not probe UNC paths during recovery (Windows can block for a long timeout).
func keepRecoveryTeamRepository(binding map[string]json.RawMessage) (bool, error) {
	repository := strings.TrimSpace(rawString(binding["repository"]))
	if strings.HasPrefix(repository, `\\`) || strings.HasPrefix(repository, "//") {
		return true, nil
	}
	if repository == "" {
		return false, nil
	}
	id := rawString(binding["share_id"])
	manifest, err := readRecoveryJSON(filepath.Join(repository, "tasks", id, "manifest.json"))
	if err != nil {
		return false, fmt.Errorf("check original collaboration repository %s: %w", repository, err)
	}
	if manifest == nil {
		return false, nil
	}
	digest := sha256.Sum256([]byte(rawString(binding["secret"])))
	if rawString(manifest["share_id"]) != id || rawString(manifest["token_hash"]) != hex.EncodeToString(digest[:]) {
		return false, fmt.Errorf("original collaboration repository for %s has changed identity; restore must not create a separate collaboration", id)
	}
	return true, nil
}

// A restored personal dataset can intentionally keep a live local repository
// outside the new default imports directory. Back up that binding's actual
// data instead of the stale copy which happens to live under the default root.
func backupBoundLocalTeamShares(data, team, backupDirectory string) error {
	state, err := readRecoveryJSON(filepath.Join(data, "team-sync.json"))
	if err != nil || state == nil {
		return err
	}
	var bindings []map[string]json.RawMessage
	if len(state["bindings"]) == 0 {
		return nil
	}
	if err := json.Unmarshal(state["bindings"], &bindings); err != nil {
		return err
	}
	for _, binding := range bindings {
		repository, id := rawString(binding["repository"]), rawString(binding["share_id"])
		if repository == "" || strings.HasPrefix(repository, `\\`) || strings.HasPrefix(repository, "//") || samePath(repository, teamDataRoot()) {
			continue
		}
		if id == "" || !filepath.IsLocal(id) || filepath.Base(id) != id || id == "." {
			continue
		}
		live, err := keepRecoveryTeamRepository(binding)
		if err != nil {
			return err
		}
		if !live {
			continue
		}
		source := filepath.Join(repository, "tasks", id)
		if pathContains(source, backupDirectory) {
			return fmt.Errorf("backup directory cannot be inside active collaboration data %s", source)
		}
		target := filepath.Join(team, "tasks", id)
		// target is a validated single share ID inside this backup's staging
		// directory; never remove anything from the user's live repositories.
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		if err := copyDirectoryFiltered(source, target, nil); err != nil {
			return err
		}
	}
	return nil
}
