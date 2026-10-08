// SPDX-License-Identifier: AGPL-3.0-or-later
package webtests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/tasktracedata"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/require"
	"xorm.io/xorm"
)

func TestHumaTaskTraceBackupManagement(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	root := t.TempDir()
	data := filepath.Join(root, "data")
	team := filepath.Join(root, "teamData")
	require.NoError(t, os.MkdirAll(data, 0o700))
	require.NoError(t, os.MkdirAll(team, 0o700))
	engine, err := xorm.NewEngine("sqlite3", filepath.Join(data, "tasktrace.db"))
	require.NoError(t, err)
	require.NoError(t, engine.Sync(new(models.Task), new(models.Project), new(user.User)))
	require.NoError(t, engine.Close())
	t.Setenv("TASKTRACE_PACKAGE_ROOT", root)
	t.Setenv("TASKTRACE_DATA_ROOT", data)
	t.Setenv("TASKTRACE_TEAM_ROOT", team)
	created, err := tasktracedata.RunBackup()
	require.NoError(t, err)
	token := humaTokenFor(t, &testuser1)
	const base = "/api/v2/tasktrace/data-recovery/backups"
	rec := humaRequest(t, e, http.MethodGet, base, "", token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var listed tasktracedata.TaskTraceBackupList
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed.Backups, 1)
	path := base + "/" + listed.Backups[0].ID
	for _, request := range []struct{ method, path, body string }{
		{http.MethodGet, base, ""},
		{http.MethodPut, path + "/note", `{"note":"unauthorized"}`},
		{http.MethodDelete, path, ""},
	} {
		rec = humaRequest(t, e, request.method, request.path, request.body, "", "")
		require.Equal(t, http.StatusUnauthorized, rec.Code, rec.Body.String())
	}
	rec = humaRequest(t, e, http.MethodPut, path+"/note", `{"note":"before delivery\nkeep details"}`, token, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	rec = humaRequest(t, e, http.MethodGet, base, "", token, "")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Equal(t, "before delivery\nkeep details", listed.Backups[0].Note)
	rec = humaRequest(t, e, http.MethodPut, path+"/note", `{"note":"`+strings.Repeat("a", 2001)+`"}`, token, "")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = humaRequest(t, e, http.MethodDelete, base+"/"+strings.Repeat("0", 64), "", token, "")
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	t.Run("disabled outside portable mode", func(t *testing.T) {
		t.Setenv("TASKTRACE_PACKAGE_ROOT", "")
		for _, request := range []struct{ method, path, body string }{
			{http.MethodGet, base, ""},
			{http.MethodPut, path + "/note", `{"note":"disabled"}`},
			{http.MethodDelete, path, ""},
		} {
			rec = humaRequest(t, e, request.method, request.path, request.body, token, "")
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		}
	})
	require.DirExists(t, created.Directory)
	rec = humaRequest(t, e, http.MethodDelete, path, "", token, "")
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.NoDirExists(t, created.Directory)
	require.FileExists(t, filepath.Join(data, "tasktrace.db"))
	require.DirExists(t, team)
	rec = humaRequest(t, e, http.MethodGet, base, "", token, "")
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Empty(t, listed.Backups)
}
