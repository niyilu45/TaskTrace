// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build windows

package tasktracedata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBackupQuickMemoIgnoresLockedArtifacts(t *testing.T) {
	recoveryEnvironment(t)
	memoDirectory := filepath.Join(dataRoot(), "quick-memo")
	require.NoError(t, os.MkdirAll(memoDirectory, 0o700))
	memo := `<p>Committed content</p><img src="data:image/png;base64,cGljdHVyZQ==">`
	require.NoError(t, os.WriteFile(filepath.Join(memoDirectory, "memo.html"), []byte(memo), 0o600))
	for _, name := range []string{"memo.html.0123456789abcdef0123456789abcdef.tmp", "memo.html.bak"} {
		holdBackupTestFile(t, filepath.Join(memoDirectory, name))
	}
	destination := filepath.Join(t.TempDir(), "snapshot")
	require.NoError(t, snapshotData(destination))
	require.Equal(t, memo, string(requireFile(t, filepath.Join(destination, "quick-memo", "memo.html"))))
	require.NoFileExists(t, filepath.Join(destination, "quick-memo", "memo.html.bak"))
	require.NoFileExists(t, filepath.Join(destination, "quick-memo", "memo.html.0123456789abcdef0123456789abcdef.tmp"))
}

func TestBackupQuickMemoDoesNotSkipLockedCommittedHTML(t *testing.T) {
	recoveryEnvironment(t)
	memoDirectory := filepath.Join(dataRoot(), "quick-memo")
	require.NoError(t, os.MkdirAll(memoDirectory, 0o700))
	holdBackupTestFile(t, filepath.Join(memoDirectory, "memo.html"))
	require.ErrorContains(t, snapshotData(filepath.Join(t.TempDir(), "snapshot")), "copy personal data")
}
