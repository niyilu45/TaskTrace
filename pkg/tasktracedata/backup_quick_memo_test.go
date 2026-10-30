// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBackupQuickMemoArtifactNames(t *testing.T) {
	for _, test := range []struct {
		name string
		skip bool
	}{
		{name: "quick-memo/memo.html.bak", skip: true},
		{name: `QUICK-MEMO\MEMO.HTML.BAK`, skip: true},
		{name: "quick-memo/memo.html.0123456789abcdef0123456789abcdef.tmp", skip: true},
		{name: `Quick-Memo\Memo.html.ABCDEF0123456789ABCDEF0123456789.TMP`, skip: true},
		{name: "quick-memo/memo.html"},
		{name: "memo.html.bak"},
		{name: "files/quick-memo/memo.html.bak"},
		{name: "quick-memo/nested/memo.html.bak"},
		{name: "quick-memo/memo.html.bak.extra"},
		{name: "quick-memo/memo.html.tmp"},
		{name: "quick-memo/memo.html.0123456789abcdef0123456789abcde.tmp"},
		{name: "quick-memo/memo.html.0123456789abcdef0123456789abcdef0.tmp"},
		{name: "quick-memo/memo.html.0123456789abcdef0123456789abcdeg.tmp"},
		{name: "quick-memo/memo.html.01234567-89ab-cdef-0123-456789abcdef.tmp"},
		{name: "quick-memo/memo.html.0123456789abcdef0123456789abcdef.tmp.extra"},
		{name: "quick-memo/other.html.0123456789abcdef0123456789abcdef.tmp"},
		{name: "attachments/quick-memo/memo.html.0123456789abcdef0123456789abcdef.tmp"},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.skip, quickMemoBackupArtifact(test.name))
		})
	}
}

func TestBackupQuickMemoPreservesCommittedHTML(t *testing.T) {
	recoveryEnvironment(t)
	memo := `<p>Local memo &#20013;&#25991;<br>Text before image</p><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jV1kAAAAASUVORK5CYII="><p>Text after image</p>`
	preserved := map[string]string{
		"quick-memo/memo.html":                                            memo,
		"quick-memo/memo.html.invalid.tmp":                                "not an internal temporary file",
		"quick-memo/memo.html.tmp":                                        "not an internal temporary file",
		"quick-memo/memo.html.bak.extra":                                  "user data",
		"quick-memo/nested/memo.html.bak":                                 "nested attachment",
		"files/quick-memo/memo.html.0123456789abcdef0123456789abcdef.tmp": "attachment with the same filename",
		"files/quick-memo/memo.html.bak":                                  "attachment with the same filename",
		"files/attachment.tmp":                                            "user attachment",
		"files/attachment.bak":                                            "user attachment backup",
	}
	transient := []string{
		"quick-memo/memo.html.0123456789abcdef0123456789abcdef.tmp",
		"quick-memo/memo.html.bak",
	}
	for relative, content := range preserved {
		path := filepath.Join(dataRoot(), relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	for _, relative := range transient {
		require.NoError(t, os.WriteFile(filepath.Join(dataRoot(), relative), []byte("in-flight or previous memo"), 0o600))
	}

	now := time.Now()
	backup, err := runBackup(now)
	require.NoError(t, err)
	require.True(t, backup.Created)
	for relative, content := range preserved {
		require.Equal(t, content, string(requireFile(t, filepath.Join(backup.Directory, "data", relative))))
	}
	for _, relative := range transient {
		require.NoFileExists(t, filepath.Join(backup.Directory, "data", relative))
		require.FileExists(t, filepath.Join(dataRoot(), relative))
		require.NoError(t, os.WriteFile(filepath.Join(dataRoot(), relative), []byte("changed transient bytes"), 0o600))
	}

	unchanged, err := runBackup(now.Add(time.Minute))
	require.NoError(t, err)
	require.True(t, unchanged.Skipped, "transient memo writes must not change the backup digest")
	require.NoError(t, os.WriteFile(filepath.Join(dataRoot(), "quick-memo", "memo.html"), []byte(memo+"<p>Edited</p>"), 0o600))
	changed, err := runBackup(now.Add(2 * time.Minute))
	require.NoError(t, err)
	require.True(t, changed.Created)
	require.Equal(t, memo+"<p>Edited</p>", string(requireFile(t, filepath.Join(changed.Directory, "data", "quick-memo", "memo.html"))))

	restored, err := Import(TaskTraceDataImportRequest{DataDirectory: filepath.Join(backup.Directory, "data")})
	require.NoError(t, err)
	require.Equal(t, memo, string(requireFile(t, filepath.Join(restored.DataDirectory, "quick-memo", "memo.html"))))
}

func TestBackupQuickMemoKeepsNamedDirectories(t *testing.T) {
	recoveryEnvironment(t)
	for _, relative := range []string{
		"quick-memo/memo.html.bak/image.png",
		"quick-memo/memo.html.0123456789abcdef0123456789abcdef.tmp/image.png",
	} {
		path := filepath.Join(dataRoot(), relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte("nested user data"), 0o600))
	}
	destination := filepath.Join(t.TempDir(), "snapshot")
	require.NoError(t, snapshotData(destination))
	for _, relative := range []string{
		"quick-memo/memo.html.bak/image.png",
		"quick-memo/memo.html.0123456789abcdef0123456789abcdef.tmp/image.png",
	} {
		require.Equal(t, "nested user data", string(requireFile(t, filepath.Join(destination, relative))))
	}
}
