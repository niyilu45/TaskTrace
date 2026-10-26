// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/require"
	"xorm.io/builder"
	"xorm.io/xorm"
)

func taskTraceImageCleanupTest(t *testing.T) (*xorm.Session, *TaskAttachment, *files.File, time.Time) {
	t.Helper()
	db.LoadAndAssertFixtures(t)
	taskTraceImageCleanupCursor = 0
	root := t.TempDir()
	t.Setenv("TASKTRACE_PACKAGE_ROOT", root)
	t.Setenv("TASKTRACE_DATA_ROOT", filepath.Join(root, "data"))
	oldRoot := config.ServiceRootpath.GetString()
	config.ServiceRootpath.Set(root)
	t.Cleanup(func() { config.ServiceRootpath.Set(oldRoot) })
	s := db.NewSession()
	now := time.Now().UTC()
	content := []byte("isolated generated image bytes")
	file, err := files.CreateWithMimeAndSession(s, bytes.NewReader(content), "tasktrace-progress-0123456789abcdef0123456789abcdef.png", uint64(len(content)), &user.User{ID: 1}, "image/png", true)
	require.NoError(t, err)
	file.Created = now.Add(-48 * time.Hour)
	_, err = s.ID(file.ID).Cols("created").Update(file)
	require.NoError(t, err)
	attachment := &TaskAttachment{
		TaskID:      1,
		FileID:      file.ID,
		CreatedByID: 1,
		Created:     file.Created,
	}
	_, err = s.Insert(attachment)
	require.NoError(t, err)
	return s, attachment, file, now
}

func taskTraceImageCleanupAssertRetained(t *testing.T, s *xorm.Session, attachment *TaskAttachment, file *files.File, now time.Time) {
	t.Helper()
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	require.NoError(t, taskTraceCleanupImages(now, files.DeleteBlob))
	read := db.NewReadSession()
	defer read.Close()
	exists, err := read.ID(attachment.ID).Exist(&TaskAttachment{})
	require.NoError(t, err)
	require.True(t, exists)
	_, err = files.FileStat(file)
	require.NoError(t, err)
}

func TestTaskTraceImageCleanup(t *testing.T) {
	t.Run("orphan removes attachment metadata and stored bytes", func(t *testing.T) {
		s, attachment, file, now := taskTraceImageCleanupTest(t)
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		require.NoError(t, taskTraceCleanupImages(now, files.DeleteBlob))
		read := db.NewReadSession()
		defer read.Close()
		exists, err := read.ID(attachment.ID).Exist(&TaskAttachment{})
		require.NoError(t, err)
		require.False(t, exists)
		exists, err = read.ID(file.ID).Exist(&files.File{})
		require.NoError(t, err)
		require.False(t, exists)
		_, err = files.FileStat(file)
		require.Error(t, err)
	})
	t.Run("locked blob retries without dangling attachment or lost retry state", func(t *testing.T) {
		s, attachment, file, now := taskTraceImageCleanupTest(t)
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		require.Error(t, taskTraceCleanupImages(now, func(int64) error { return errors.New("sharing violation") }))
		read := db.NewReadSession()
		exists, err := read.ID(attachment.ID).Exist(&TaskAttachment{})
		require.NoError(t, err)
		require.False(t, exists)
		exists, err = read.ID(file.ID).Exist(&files.File{})
		require.NoError(t, err)
		require.True(t, exists)
		require.NoError(t, read.Close())
		_, err = files.FileStat(file)
		require.NoError(t, err)
		require.NoError(t, taskTraceCleanupImages(now, files.DeleteBlob))
		_, err = files.FileStat(file)
		require.Error(t, err)
	})
	for _, location := range []string{
		"task",
		"other task",
		"project",
		"comment",
		"history",
		"undo html",
		"undo attachment",
		"cover",
		"draft",
		"offline team",
		"team attachment mapping",
		"shared file",
		"avatar",
		"project background",
		"undo file",
		"encoded reference",
		"new upload",
		"manual image",
		"manual outstanding screenshot",
		"manual outstanding report",
		"manual progress screenshot",
	} {
		t.Run("preserves "+location, func(t *testing.T) {
			s, attachment, file, now := taskTraceImageCleanupTest(t)
			src := fmt.Sprintf("/api/v2/tasks/1/attachments/%d", attachment.ID)
			body := `<p>Kept image</p><img src="https://local.test` + src + `?preview=large">`
			var err error
			switch location {
			case "task", "other task":
				id := int64(1)
				if location == "other task" {
					id = 2
				}
				_, err = s.ID(id).Cols("description").Update(&Task{Description: body})
			case "project":
				_, err = s.ID(1).Cols("description").Update(&Project{Description: body})
			case "comment", "history":
				if location == "history" {
					body = `<aside data-tasktrace-review-item-snapshot hidden>` + body + `</aside>`
				}
				_, err = s.Insert(&TaskComment{
					TaskID:   2,
					AuthorID: 1,
					Comment:  body,
				})
			case "undo html", "undo attachment":
				capture := taskTraceUndoCapture{
					Version: 1,
					TaskID:  1,
				}
				if location == "undo html" {
					raw, marshalErr := json.Marshal(body)
					require.NoError(t, marshalErr)
					capture.Before.Task = taskTraceUndoRow{"Description": raw}
				} else {
					capture.Before.Parts = map[string][]taskTraceUndoRow{"attachments": {{"ID": json.RawMessage(fmt.Sprint(attachment.ID))}}}
				}
				payload, marshalErr := json.Marshal(capture)
				require.NoError(t, marshalErr)
				_, err = s.Insert(&TaskTraceUndoEntry{
					UserID:  1,
					GroupID: "image-cleanup",
					Label:   "Keep",
					Payload: string(payload),
				})
			case "cover":
				_, err = s.ID(2).Cols("cover_image_attachment_id").Update(&Task{CoverImageAttachmentID: attachment.ID})
			case "draft":
				path := filepath.Join(config.ServiceRootpath.GetString(), ".cache", "tasktrace-drafts", "outstanding", "1", "draft.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
				encoded, marshalErr := json.Marshal(map[string]string{"html": body})
				require.NoError(t, marshalErr)
				err = os.WriteFile(path, encoded, 0o600)
			case "offline team", "team attachment mapping":
				state := taskTraceTeamState{
					Schema:   taskTraceTeamSchema,
					DeviceID: "test",
					Bindings: []TaskTraceTeamBinding{
						{
							ShareID:          "offline",
							Repository:       filepath.Join(t.TempDir(), "unavailable-network-share"),
							RootTaskID:       99,
							NodeTasks:        map[string]int64{},
							LocalAttachments: map[string]int64{},
						},
					},
				}
				if location == "offline team" {
					state.Bindings[0].NodeTasks["child"] = attachment.TaskID
				} else {
					state.Bindings[0].LocalAttachments["node:hash"] = attachment.ID
				}
				err = taskTraceTeamSaveState(state)
			case "shared file":
				_, err = s.Insert(&TaskAttachment{
					TaskID:      2,
					FileID:      file.ID,
					CreatedByID: 1,
				})
			case "avatar":
				_, err = s.ID(2).Cols("avatar_file_id").Update(&user.User{AvatarFileID: file.ID})
			case "project background":
				_, err = s.ID(2).Cols("background_file_id").Update(&Project{BackgroundFileID: file.ID})
			case "undo file":
				payload := fmt.Sprintf(`{"before":{"task":{"BackgroundFileID":%d}}}`, file.ID)
				_, err = s.Insert(&TaskTraceUndoEntry{
					UserID:  1,
					GroupID: "file",
					Label:   "Keep",
					Payload: payload,
				})
			case "encoded reference":
				encoded := ""
				for _, character := range fmt.Sprint(attachment.ID) {
					encoded += fmt.Sprintf("%%%02X", character)
				}
				_, err = s.ID(2).Cols("description").Update(&Task{Description: `<img src="/api/v1/tasks/1/attachments/` + encoded + `">`})
			case "new upload":
				_, err = s.ID(file.ID).Cols("created").Update(&files.File{Created: now.Add(-time.Minute)})
			case "manual outstanding screenshot", "manual outstanding report", "manual progress screenshot":
				name := "outstanding-screenshot.png"
				switch location {
				case "manual outstanding report":
					name = "outstanding-report.png"
				case "manual progress screenshot":
					name = "tasktrace-progress-screenshot.png"
				}
				_, err = s.ID(file.ID).Cols("name").Update(&files.File{Name: name})
			case "manual image":
				_, err = s.ID(file.ID).Cols("name").Update(&files.File{Name: "manual-image.png"})
			}
			require.NoError(t, err)
			taskTraceImageCleanupAssertRetained(t, s, attachment, file, now)
		})
	}
	t.Run("retry preserves newly assigned project background after restart", func(t *testing.T) {
		s, _, file, now := taskTraceImageCleanupTest(t)
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		require.Error(t, taskTraceCleanupImages(now, func(int64) error { return errors.New("locked") }))
		update := db.NewSession()
		_, err := update.ID(1).Cols("background_file_id").Update(&Project{BackgroundFileID: file.ID})
		require.NoError(t, err)
		require.NoError(t, update.Commit())
		require.NoError(t, update.Close())
		taskTraceImageCleanupCursor = 0
		require.NoError(t, taskTraceCleanupImages(now, files.DeleteBlob))
		_, err = files.FileStat(file)
		require.NoError(t, err)
		read := db.NewReadSession()
		defer read.Close()
		exists, err := read.ID(file.ID).Exist(&files.File{})
		require.NoError(t, err)
		require.True(t, exists)
	})
	t.Run("bare file without an attachment or cleanup checkpoint is retained", func(t *testing.T) {
		s, attachment, file, now := taskTraceImageCleanupTest(t)
		_, err := s.ID(attachment.ID).Delete(&TaskAttachment{})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		require.NoError(t, taskTraceCleanupImages(now, files.DeleteBlob))
		_, err = files.FileStat(file)
		require.NoError(t, err)
	})

	t.Run("malformed collaboration state fails closed before mutation", func(t *testing.T) {
		s, attachment, file, now := taskTraceImageCleanupTest(t)
		path := taskTraceTeamStatePath()
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(`{"schema":1,"bindings":`), 0o600))
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		require.Error(t, taskTraceCleanupImages(now, files.DeleteBlob))
		read := db.NewReadSession()
		defer read.Close()
		exists, err := read.ID(attachment.ID).Exist(&TaskAttachment{})
		require.NoError(t, err)
		require.True(t, exists)
		_, err = files.FileStat(file)
		require.NoError(t, err)
	})
	t.Run("guard errors roll back without removing bytes", func(t *testing.T) {
		s, attachment, file, now := taskTraceImageCleanupTest(t)
		_, err := s.Insert(&TaskTraceUndoEntry{
			UserID:  1,
			GroupID: "corrupt",
			Label:   "Keep",
			Payload: fmt.Sprintf(`{"bad":%d`, attachment.ID),
		})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		require.NoError(t, s.Close())
		require.Error(t, taskTraceCleanupImages(now, files.DeleteBlob))
		read := db.NewReadSession()
		defer read.Close()
		exists, err := read.Where(builder.Eq{"id": attachment.ID}).Exist(&TaskAttachment{})
		require.NoError(t, err)
		require.True(t, exists)
		_, err = files.FileStat(file)
		require.NoError(t, err)
	})
}

func TestTaskTraceImageCleanupReferenceBoundaries(t *testing.T) {
	require.True(t, taskTraceCleanupReferences(`<img src="/api/v1/tasks/7/attachments/12?preview=large">`, 12))
	require.True(t, taskTraceCleanupReferences(`https://host/api/v2/tasks/7/attachments/12`, 12))
	require.True(t, taskTraceCleanupReferences(`\/api\/v1\/tasks\/7\/attachments\/12`, 12))
	require.True(t, taskTraceCleanupReferences(`/api/v1/tasks/7/attachments%2f12`, 12))
	require.True(t, taskTraceCleanupReferences(`/api/v1/tasks/7/attachments/%31%32`, 12))
	require.False(t, taskTraceCleanupReferences(`/api/v1/tasks/7/attachments/123`, 12))
}

func TestTaskTraceImageCleanupWriterBudget(t *testing.T) {
	s, attachment, file, now := taskTraceImageCleanupTest(t)
	require.NoError(t, s.Commit())
	require.NoError(t, s.Close())
	writer := db.NewSession()
	defer writer.Close()
	_, err := writer.Table("users").Where(builder.In("id", []int64{})).Update(map[string]any{"id": int64(0)})
	require.NoError(t, err)
	started := time.Now()
	err = taskTraceCleanupImage(file.ID, now, map[int64]bool{}, files.DeleteBlob)
	require.Error(t, err)
	require.Less(t, time.Since(started), 2*time.Second)
	require.NoError(t, writer.Rollback())
	require.NoError(t, writer.Close())
	read := db.NewReadSession()
	defer read.Close()
	exists, err := read.ID(attachment.ID).Exist(&TaskAttachment{})
	require.NoError(t, err)
	require.True(t, exists)
	_, err = files.FileStat(file)
	require.NoError(t, err)
}

func TestTaskTraceImageCleanupEncodedRecheck(t *testing.T) {
	s, attachment, _, _ := taskTraceImageCleanupTest(t)
	defer s.Close()
	percent, entity := "", ""
	for _, digit := range fmt.Sprint(attachment.ID) {
		percent += fmt.Sprintf("%%%02X", digit)
		entity += fmt.Sprintf("&#%d;", digit)
	}
	for _, encoded := range []string{
		percent,
		entity,
	} {
		_, err := s.ID(2).Cols("description").Update(&Task{Description: `<img src="/api/v1/tasks/1/attachments/` + encoded + `">`})
		require.NoError(t, err)
		referenced, err := taskTraceImageReferenced(s, attachment.ID)
		require.NoError(t, err)
		require.True(t, referenced)
	}
}
