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

// Only relocate bindings whose matching, authenticated repository was copied.
// Unrelated remote repositories and all opaque sync state remain untouched.
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
