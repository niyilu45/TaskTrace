// SPDX-License-Identifier: AGPL-3.0-or-later

package webtests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"github.com/stretchr/testify/require"
)

func TestHumaSharedTaskAtomicSubtaskCreation(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	data, repository := t.TempDir(), t.TempDir()
	t.Setenv("TASKTRACE_DATA_ROOT", data)
	t.Setenv("TASKTRACE_TEAM_ROOT", repository)
	binding := models.TaskTraceTeamBinding{Repository: repository, ShareID: "atomic-child-http", Owner: "user2", Members: []string{"user1", "user2"}, RootTaskID: 1, ProjectID: 1, NodeTasks: map[string]int64{"root": 1}}
	write := func(path string, value interface{}) {
		encoded, marshalErr := json.Marshal(value)
		require.NoError(t, marshalErr)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, encoded, 0o600))
	}
	write(filepath.Join(data, "team-sync.json"), map[string]interface{}{"schema": 1, "device_id": "recipient", "bindings": []models.TaskTraceTeamBinding{binding}})
	manifest := models.TaskTraceTeamManifest{Schema: 1, ShareID: binding.ShareID, RootNode: "root", Owner: binding.Owner, Members: binding.Members,
		Permissions: map[string][]models.TaskTraceTeamMemberPermission{"root": {
			{Username: "user2", Owner: true, Read: true, Write: true}, {Username: "user1", Read: true},
		}}}
	manifestPath := filepath.Join(repository, "tasks", binding.ShareID, "manifest.json")
	write(manifestPath, manifest)
	token := humaTokenFor(t, &testuser1)
	assertMissing := func(title string) {
		s := db.NewSession()
		defer s.Close()
		count, countErr := s.Where("title = ?", title).Count(&models.Task{})
		require.NoError(t, countErr)
		require.Zero(t, count)
	}
	rec := humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/tasks", `{"title":"Read-only child","parent_task_id":1}`, token, "")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assertMissing("Read-only child")
	manifest.Permissions["root"][1].Write = true
	write(manifestPath, manifest)
	rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/tasks", `{"title":"Receiving member child","parent_task_id":1}`, token, "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var child models.Task
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &child))
	require.Positive(t, child.ID)
	s := db.NewSession()
	count, err := s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", 1, child.ID, models.RelationKindSubtask).Count(&models.TaskRelation{})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	count, err = s.Where("task_id = ? AND other_task_id = ? AND relation_kind = ?", child.ID, 1, models.RelationKindParenttask).Count(&models.TaskRelation{})
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	require.NoError(t, s.Close())
	for level := 3; level <= models.MaxTaskHierarchyDepth; level++ {
		body := fmt.Sprintf(`{"title":"Nested child %d","parent_task_id":%d}`, level, child.ID)
		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/tasks", body, token, "")
		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &child))
	}
	body := fmt.Sprintf(`{"title":"Too deep child","parent_task_id":%d}`, child.ID)
	rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/tasks", body, token, "")
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assertMissing("Too deep child")
	rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/tasks", `{"title":"Foreign parent child","parent_task_id":14}`, token, "")
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	assertMissing("Foreign parent child")
}
