// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/user"
	"github.com/stretchr/testify/require"
)

func TestTaskTraceTeamAttachmentImportIntegrity(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	task := &Task{Title: "attachment import test", ProjectID: 1}
	require.NoError(t, task.Create(s, actor))
	binding := &TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "share"}
	content := []byte("verified shared attachment")
	sum := sha256.Sum256(content)
	shared := TaskTraceTeamAttachment{ID: hex.EncodeToString(sum[:]), Name: "attachment.txt"}
	path := taskTraceTeamAttachmentBlobPath(binding, shared.ID)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, bytes.Repeat([]byte("X"), len(content)), 0o600))

	_, err := taskTraceTeamImportAttachment(s, actor, binding, task.ID, shared)
	require.ErrorContains(t, err, "failed integrity check")
	count, err := s.Where("task_id = ?", task.ID).Count(&TaskAttachment{})
	require.NoError(t, err)
	require.Zero(t, count)

	require.NoError(t, os.WriteFile(path, content, 0o600))
	id, err := taskTraceTeamImportAttachment(s, actor, binding, task.ID, shared)
	require.NoError(t, err)
	attachments, err := getTaskAttachmentsByTaskIDs(s, []int64{task.ID})
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	require.Equal(t, id, attachments[0].ID)
	require.Equal(t, uint64(len(content)), attachments[0].File.Size)
}

func TestTaskTraceTeamAttachmentExportAllocation(t *testing.T) {
	db.LoadAndAssertFixtures(t)
	s := db.NewSession()
	defer s.Close()
	actor := &user.User{ID: 1, Username: "user1"}
	task := &Task{Title: "streamed attachment export", ProjectID: 1}
	require.NoError(t, task.Create(s, actor))
	const size = 16 << 20
	content := bytes.Repeat([]byte("a"), size)
	attachment := &TaskAttachment{TaskID: task.ID}
	require.NoError(t, attachment.NewAttachment(s, bytes.NewReader(content), "large.txt", size, actor))
	binding := &TaskTraceTeamBinding{Repository: t.TempDir(), ShareID: "share"}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	rows, err := taskTraceTeamExportAttachments(s, binding, task.ID, "node")
	runtime.ReadMemStats(&after)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, uint64(size), rows[0].Size)
	sum := sha256.Sum256(content)
	require.Equal(t, hex.EncodeToString(sum[:]), rows[0].ID)
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("allocated %.3f MiB exporting a 16 MiB attachment", float64(allocated)/(1<<20))
	require.Less(t, allocated, uint64(size/2), "export must not allocate a whole-file buffer")
}
