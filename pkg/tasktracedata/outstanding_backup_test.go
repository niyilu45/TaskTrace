// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktracedata

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestBackupAndRestorePreserveOutstandingCommentsAndImages(t *testing.T) {
	recoveryEnvironment(t)
	createRecoveryTeam(t, dataRoot(), teamDataRoot())
	engine, err := xorm.NewEngine("sqlite3", filepath.Join(dataRoot(), "tasktrace.db"))
	require.NoError(t, err)
	defer engine.Close()
	_, err = engine.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	_, err = engine.Exec("PRAGMA wal_autocheckpoint=0")
	require.NoError(t, err)
	_, err = engine.Exec("CREATE TABLE task_comments (id INTEGER PRIMARY KEY, task_id INTEGER, comment TEXT)")
	require.NoError(t, err)
	// Keep the live connection open: the saved list is still in SQLite's WAL
	// when the backup runs, just as it is during normal use of the application.
	body := `<h3 data-tasktrace-comment-type="outstanding">Outstanding</h3><ul><li data-id="saved" data-priority="7" data-done="false"><p>Saved before update</p><aside data-tasktrace-outstanding-note="true" hidden><p>Note</p><img src="/api/v1/tasks/1/attachments/2"></aside></li></ul>`
	_, err = engine.Exec("INSERT INTO task_comments (id, task_id, comment) VALUES (1, 1, ?)", body)
	require.NoError(t, err)
	image := filepath.Join(dataRoot(), "files", "2")
	require.NoError(t, os.MkdirAll(filepath.Dir(image), 0o700))
	require.NoError(t, os.WriteFile(image, []byte("image bytes"), 0o600))
	backup, err := RunBackup()
	require.NoError(t, err)
	require.True(t, backup.Created)
	assertOutstanding := func(directory string) {
		t.Helper()
		copyDB, openErr := xorm.NewEngine("sqlite3", filepath.Join(directory, "tasktrace.db"))
		require.NoError(t, openErr)
		defer copyDB.Close()
		rows, queryErr := copyDB.QueryString("SELECT comment FROM task_comments WHERE id = 1")
		require.NoError(t, queryErr)
		require.Len(t, rows, 1)
		require.Equal(t, body, rows[0]["comment"], "backup and import must preserve the entire saved list including note and image references")
		require.Equal(t, []byte("image bytes"), requireFile(t, filepath.Join(directory, "files", "2")))
	}
	assertOutstanding(filepath.Join(backup.Directory, "data"))
	_, err = engine.Exec("DELETE FROM task_comments")
	require.NoError(t, err)
	restored, err := Import(TaskTraceDataImportRequest{DataDirectory: filepath.Join(backup.Directory, "data")})
	require.NoError(t, err)
	assertOutstanding(restored.DataDirectory)
	require.FileExists(t, filepath.Join(restored.TeamDataDirectory, "tasks/shared-task/attachments/image"))
}
