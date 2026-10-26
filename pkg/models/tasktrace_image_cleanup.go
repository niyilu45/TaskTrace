// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/cron"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/user"

	"xorm.io/builder"
	"xorm.io/xorm"
	"xorm.io/xorm/schemas"
)

const taskTraceImageCleanupGrace = 24 * time.Hour
const taskTraceImageCleanupWriteBudget = 200 * time.Millisecond

var taskTraceImageCleanupRun sync.Mutex
var taskTraceImageCleanupCursor int64

var taskTraceGeneratedImageName = regexp.MustCompile(`^(?:tasktrace-progress-(?:[0-9a-f]{32}|[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\.(?:png|jpe?g|gif|webp|bmp)|outstanding-[0-9a-f]{32}\.png)$`)
var taskTraceCleanupAttachmentReference = regexp.MustCompile(`(?:api/v[12]/)?tasks/[0-9]+/attachments/([0-9]+)(?:[^0-9]|$)`)

// RegisterTaskTraceImageCleanupCron reclaims only images whose references have expired.
func RegisterTaskTraceImageCleanupCron() {
	if err := cron.Schedule("17 * * * *", func() {
		if err := taskTraceCleanupImages(time.Now(), files.DeleteBlob); err != nil {
			log.Errorf("TaskTrace image cleanup: %s", err)
		}
	}); err != nil {
		log.Fatalf("Could not register TaskTrace image cleanup cron: %s", err)
	}
}

func taskTraceGeneratedImage(file *files.File, cutoff time.Time) bool {
	return strings.HasPrefix(strings.ToLower(file.Mime), "image/") &&
		file.Created.Before(cutoff) && taskTraceGeneratedImageName.MatchString(strings.ToLower(file.Name))
}

func taskTraceCleanupImages(now time.Time, removeBlob func(int64) error) error {
	// The desktop's SQLite writer reservation serializes reference checks with saves.
	// Other database engines need their own reference locking before enabling this.
	if db.Type() != schemas.SQLITE || !taskTraceImageCleanupRun.TryLock() {
		return nil
	}
	defer taskTraceImageCleanupRun.Unlock()
	s := db.NewReadSession()
	candidates := []*files.File{}
	err := s.Where(builder.Or(
		builder.Like{
			"name",
			"tasktrace-progress-",
		},
		builder.Like{
			"name",
			"outstanding-",
		},
	)).And(builder.Lt{"created": now.Add(-taskTraceImageCleanupGrace)}).And(builder.Gt{"id": taskTraceImageCleanupCursor}).Asc("id").Limit(100).Find(&candidates)
	_ = s.Close()
	if err != nil {
		return err
	}
	if len(candidates) == 0 {
		taskTraceImageCleanupCursor = 0
		return nil
	}
	drafts, err := taskTraceCleanupDraftReferences()
	if err != nil {
		return err
	}
	read := db.NewReadSession()
	referencedAttachments, referencedFiles, err := taskTraceCleanupReadReferences(read)
	if err != nil {
		_ = read.Close()
		return err
	}
	links := []*TaskAttachment{}
	fileIDs := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		fileIDs = append(fileIDs, candidate.ID)
	}
	err = read.In("file_id", fileIDs).Find(&links)
	_ = read.Close()
	if err != nil {
		return err
	}
	for _, link := range links {
		if referencedAttachments[link.ID] || drafts[link.ID] {
			referencedFiles[link.FileID] = true
		}
	}
	started := time.Now()
	for _, file := range candidates {
		if time.Since(started) > time.Second {
			break
		}
		if referencedFiles[file.ID] {
			taskTraceImageCleanupCursor = file.ID
			continue
		}
		if !taskTraceGeneratedImage(file, now.Add(-taskTraceImageCleanupGrace)) {
			taskTraceImageCleanupCursor = file.ID
			continue
		}
		if err := taskTraceCleanupImage(file.ID, now, drafts, removeBlob); err != nil {
			if !errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("file %d: %w", file.ID, err)
			}
		}
		taskTraceImageCleanupCursor = file.ID
	}
	return nil
}

func taskTraceCleanupBindings() (map[int64]bool, map[int64]bool, error) {
	tasks, attachments := map[int64]bool{}, map[int64]bool{}
	path := taskTraceTeamStatePath()
	if path == "" {
		return tasks, attachments, nil
	}
	var state taskTraceTeamState
	if err := taskTraceTeamReadJSON(path, &state); err != nil {
		if os.IsNotExist(err) {
			return tasks, attachments, nil
		}
		return nil, nil, err
	}
	if state.Schema != taskTraceTeamSchema || state.Bindings == nil {
		return nil, nil, errors.New("invalid collaboration state; image cleanup skipped")
	}
	// Offline bindings and local attachment mappings remain authoritative too.
	for _, binding := range state.Bindings {
		tasks[binding.RootTaskID] = true
		for _, id := range binding.NodeTasks {
			tasks[id] = true
		}
		for _, id := range binding.LocalAttachments {
			attachments[id] = true
		}
	}
	return tasks, attachments, nil
}

func taskTraceCleanupReferences(value string, attachmentID int64) bool {
	references := map[int64]bool{}
	taskTraceCleanupCollectReferences(value, references)
	return references[attachmentID]
}

func taskTraceCleanupCollectReferences(value string, references map[int64]bool) {
	value = html.UnescapeString(strings.ReplaceAll(value, `\/`, `/`))
	for i := 0; i < 3; i++ {
		decoded, err := url.PathUnescape(value)
		if err != nil || decoded == value {
			break
		}
		value = decoded
	}
	for _, match := range taskTraceCleanupAttachmentReference.FindAllStringSubmatch(value, -1) {
		id, _ := strconv.ParseInt(match[1], 10, 64)
		references[id] = true
	}
}

func taskTraceCleanupCollectUndo(payload string, attachmentIDs, fileIDs map[int64]bool) error {
	var value any
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var collect func(any)
	collect = func(value any) {
		switch item := value.(type) {
		case string:
			taskTraceCleanupCollectReferences(item, attachmentIDs)
		case []any:
			for _, child := range item {
				collect(child)
			}
		case map[string]any:
			for key, child := range item {
				lower := strings.ToLower(key)
				if number, ok := child.(json.Number); ok {
					id, _ := strconv.ParseInt(number.String(), 10, 64)
					if strings.HasSuffix(lower, "fileid") || strings.HasSuffix(lower, "file_id") {
						fileIDs[id] = true
					}
					if lower == "coverimageattachmentid" || lower == "cover_image_attachment_id" {
						attachmentIDs[id] = true
					}
				}
				if lower == "attachments" {
					if rows, ok := child.([]any); ok {
						for _, row := range rows {
							if fields, ok := row.(map[string]any); ok {
								if id, ok := fields["ID"].(json.Number); ok {
									numeric, _ := strconv.ParseInt(id.String(), 10, 64)
									attachmentIDs[numeric] = true
								}
							}
						}
					}
				}
				collect(child)
			}
		}
	}
	collect(value)
	return nil
}

func taskTraceCleanupUndoReference(payload string, attachmentID int64) (bool, error) {
	attachments := map[int64]bool{}
	err := taskTraceCleanupCollectUndo(payload, attachments, map[int64]bool{})
	return attachments[attachmentID], err
}

func taskTraceCleanupReadReferences(s *xorm.Session) (map[int64]bool, map[int64]bool, error) {
	attachments, files := map[int64]bool{}, map[int64]bool{}
	scans := []struct {
		bean    any
		columns []string
		read    func(any)
	}{
		{
			bean: &Task{},
			columns: []string{
				"description",
				"cover_image_attachment_id",
			},
			read: func(v any) {
				task := v.(*Task)
				taskTraceCleanupCollectReferences(task.Description, attachments)
				attachments[task.CoverImageAttachmentID] = true
			},
		},
		{
			bean: &Project{},
			columns: []string{
				"description",
				"background_file_id",
			},
			read: func(v any) {
				project := v.(*Project)
				taskTraceCleanupCollectReferences(project.Description, attachments)
				files[project.BackgroundFileID] = true
			},
		},
		{
			bean:    &TaskComment{},
			columns: []string{"comment"},
			read:    func(v any) { taskTraceCleanupCollectReferences(v.(*TaskComment).Comment, attachments) },
		},
		{
			bean: &user.User{},
			columns: []string{
				"avatar_file_id",
				"export_file_id",
			},
			read: func(v any) {
				account := v.(*user.User)
				files[account.AvatarFileID], files[account.ExportFileID] = true, true
			},
		},
	}
	for _, scan := range scans {
		if err := s.Cols(scan.columns...).Iterate(scan.bean, func(_ int, v any) error {
			scan.read(v)
			return nil
		}); err != nil {
			return nil, nil, err
		}
	}
	err := s.Cols("payload").Iterate(&TaskTraceUndoEntry{}, func(_ int, v any) error {
		return taskTraceCleanupCollectUndo(v.(*TaskTraceUndoEntry).Payload, attachments, files)
	})
	return attachments, files, err
}

func taskTraceCleanupDraftReferences() (map[int64]bool, error) {
	references := map[int64]bool{}
	roots := map[string]bool{}
	for _, root := range []string{
		os.Getenv("TASKTRACE_PACKAGE_ROOT"),
		config.ServiceRootpath.GetString(),
	} {
		if strings.TrimSpace(root) == "" {
			continue
		}
		root = filepath.Clean(root)
		if strings.EqualFold(filepath.Base(root), "data") {
			root = filepath.Dir(root)
		}
		roots[filepath.Join(root, ".cache", "tasktrace-drafts")] = true
	}
	for root := range roots {
		directory, err := os.OpenRoot(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		err = fs.WalkDir(directory.FS(), ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			content, err := directory.ReadFile(path)
			if err != nil {
				return err
			}
			taskTraceCleanupCollectReferences(string(content), references)
			return nil
		})
		_ = directory.Close()
		if err != nil {
			return nil, fmt.Errorf("check unsaved drafts: %w", err)
		}
	}
	return references, nil
}

var errTaskTraceImageReferenced = errors.New("image remains referenced")

func taskTraceImageReferenced(s *xorm.Session, attachmentID int64) (bool, error) {
	covered, err := s.Where(builder.Eq{"cover_image_attachment_id": attachmentID}).Exist(&Task{})
	if err != nil || covered {
		return covered, err
	}
	// Encoded decimal digits always contain hex 3x; HTML entities and JSON
	// escapes need their own conservative fallback before exact decoding.
	check := func(value string) error {
		if taskTraceCleanupReferences(value, attachmentID) {
			return errTaskTraceImageReferenced
		}
		return nil
	}
	scans := []struct {
		bean   any
		column string
		value  func(any) string
	}{
		{
			bean:   &Task{},
			column: "description",
			value:  func(v any) string { return v.(*Task).Description },
		},
		{
			bean:   &Project{},
			column: "description",
			value:  func(v any) string { return v.(*Project).Description },
		},
		{
			bean:   &TaskComment{},
			column: "comment",
			value:  func(v any) string { return v.(*TaskComment).Comment },
		},
	}
	for _, scan := range scans {
		err := s.Where(taskTraceCleanupReferenceFilter(scan.column, attachmentID)).Cols(scan.column).Iterate(scan.bean, func(_ int, value any) error { return check(scan.value(value)) })
		if errors.Is(err, errTaskTraceImageReferenced) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
	}
	err = s.Where(taskTraceCleanupReferenceFilter("payload", attachmentID)).Cols("payload").Iterate(&TaskTraceUndoEntry{}, func(_ int, value any) error {
		referenced, err := taskTraceCleanupUndoReference(value.(*TaskTraceUndoEntry).Payload, attachmentID)
		if err != nil {
			return err
		}
		if referenced {
			return errTaskTraceImageReferenced
		}
		return nil
	})
	if errors.Is(err, errTaskTraceImageReferenced) {
		return true, nil
	}
	return false, err
}

func taskTraceCleanupReferenceFilter(column string, id int64) builder.Cond {
	decimal := strconv.FormatInt(id, 10)
	conditions := []builder.Cond{
		builder.Like{
			column,
			decimal,
		},
		builder.Like{
			column,
			"&#",
		},
	}
	seen := map[rune]bool{}
	for _, digit := range decimal {
		if !seen[digit] {
			// A URL-encoded decimal digit is %30..%39; matching its hex pair
			// also covers JSON unicode escapes without a broad LIKE "3".
			conditions = append(conditions, builder.Like{
				column,
				"3" + string(digit),
			})
			seen[digit] = true
		}
	}
	return builder.Or(conditions...)
}

func taskTraceImageFileReferenced(s *xorm.Session, fileID int64) (bool, error) {
	used, err := s.Where(builder.Or(
		builder.Eq{"avatar_file_id": fileID},
		builder.Eq{"export_file_id": fileID},
	)).Exist(&user.User{})
	if err != nil || used {
		return used, err
	}
	used, err = s.Where(builder.Eq{"background_file_id": fileID}).Exist(&Project{})
	if err != nil || used {
		return used, err
	}
	err = s.Where(builder.Like{
		"payload",
		strconv.FormatInt(fileID, 10),
	}).Cols("payload").Iterate(&TaskTraceUndoEntry{}, func(_ int, value any) error {
		files := map[int64]bool{}
		if err := taskTraceCleanupCollectUndo(value.(*TaskTraceUndoEntry).Payload, map[int64]bool{}, files); err != nil {
			return err
		}
		if files[fileID] {
			return errTaskTraceImageReferenced
		}
		return nil
	})
	if errors.Is(err, errTaskTraceImageReferenced) {
		return true, nil
	}
	return false, err
}

type taskTraceImageCleanupPending struct {
	FileID        int64   `json:"file_id"`
	AttachmentIDs []int64 `json:"attachment_ids"`
	TaskIDs       []int64 `json:"task_ids"`
}

func taskTraceImageCleanupPendingPath(fileID int64) string {
	root := os.Getenv("TASKTRACE_PACKAGE_ROOT")
	if root == "" {
		root = config.ServiceRootpath.GetString()
		if strings.EqualFold(filepath.Base(root), "data") {
			root = filepath.Dir(root)
		}
	}
	return filepath.Join(root, ".cache", "tasktrace-image-cleanup", strconv.FormatInt(fileID, 10)+".json")
}

func taskTraceImageCleanupSession(ctx context.Context) (*xorm.Session, error) {
	s := db.NewReadSession()
	db.SetSessionContext(ctx, s)
	if err := s.Begin(); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}

func taskTraceCleanupImage(fileID int64, now time.Time, drafts map[int64]bool, removeBlob func(int64) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), taskTraceImageCleanupWriteBudget)
	defer cancel()
	s, err := taskTraceImageCleanupSession(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	// Acquire the writer before reading; a long-lived WAL read snapshot cannot
	// safely be promoted after another editor commits.
	if _, err := s.Table("users").Where(builder.In("id", []int64{})).Update(map[string]any{"id": int64(0)}); err != nil {
		return err
	}
	// Team synchronization sometimes obtains its mutex before writing to SQLite.
	// Never wait for it while holding the writer and create a lock inversion.
	if !taskTraceTeamMu.TryLock() {
		return nil
	}
	teamLocked := true
	defer func() {
		if teamLocked {
			taskTraceTeamMu.Unlock()
		}
	}()
	sharedTasks, sharedAttachments, err := taskTraceCleanupBindings()
	if err != nil {
		return err
	}
	file := &files.File{ID: fileID}
	exists, err := s.ID(fileID).Get(file)
	if err != nil || !exists || !taskTraceGeneratedImage(file, now.Add(-taskTraceImageCleanupGrace)) {
		return err
	}
	used, err := taskTraceImageFileReferenced(s, fileID)
	if err != nil || used {
		return err
	}
	attachments := []*TaskAttachment{}
	if err := s.Where(builder.Eq{"file_id": fileID}).Find(&attachments); err != nil {
		return err
	}
	// Duplicated tasks can share the same stored file; never infer ownership.
	if len(attachments) > 1 {
		return nil
	}
	pending := taskTraceImageCleanupPending{FileID: fileID}
	pendingPath := taskTraceImageCleanupPendingPath(fileID)
	if len(attachments) == 0 {
		if err := taskTraceTeamReadJSON(pendingPath, &pending); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if pending.FileID != fileID || len(pending.AttachmentIDs) == 0 || len(pending.TaskIDs) == 0 {
			return errors.New("invalid image cleanup checkpoint")
		}
	}
	for _, attachment := range attachments {
		if sharedTasks[attachment.TaskID] || sharedAttachments[attachment.ID] || drafts[attachment.ID] {
			return nil
		}
		referenced, err := taskTraceImageReferenced(s, attachment.ID)
		if err != nil || referenced {
			return err
		}
		pending.AttachmentIDs = append(pending.AttachmentIDs, attachment.ID)
		pending.TaskIDs = append(pending.TaskIDs, attachment.TaskID)
		if err := taskTraceTeamWriteJSON(pendingPath, &pending); err != nil {
			return err
		}
		if _, err := s.ID(attachment.ID).Delete(&TaskAttachment{}); err != nil {
			return err
		}
	}
	if err := s.Commit(); err != nil {
		return err
	}
	taskTraceTeamMu.Unlock()
	teamLocked = false
	return taskTraceFinishImageCleanup(fileID, pending, pendingPath, drafts, removeBlob)
}

func taskTraceFinishImageCleanup(fileID int64, pending taskTraceImageCleanupPending, pendingPath string, drafts map[int64]bool, removeBlob func(int64) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), taskTraceImageCleanupWriteBudget)
	defer cancel()
	finish, err := taskTraceImageCleanupSession(ctx)
	if err != nil {
		return err
	}
	defer finish.Close()
	if _, err := finish.Table("users").Where(builder.In("id", []int64{})).Update(map[string]any{"id": int64(0)}); err != nil {
		return err
	}
	if !taskTraceTeamMu.TryLock() {
		return nil
	}
	sharedTasks, sharedAttachments, err := taskTraceCleanupBindings()
	taskTraceTeamMu.Unlock()
	if err != nil {
		return err
	}
	for _, id := range pending.TaskIDs {
		if sharedTasks[id] {
			return nil
		}
	}
	for _, id := range pending.AttachmentIDs {
		if sharedAttachments[id] {
			return nil
		}
	}
	// Recheck after detaching: a restore or a new background/avatar may have
	// acquired this file between transactions, including after a restart.
	referenced, err := taskTraceImageFileReferenced(finish, fileID)
	if err != nil || referenced {
		return err
	}
	attached, err := finish.Where(builder.Eq{"file_id": fileID}).Exist(&TaskAttachment{})
	if err != nil || attached {
		return err
	}
	for _, id := range pending.AttachmentIDs {
		if id <= 0 || drafts[id] {
			return nil
		}
		referenced, err := taskTraceImageReferenced(finish, id)
		if err != nil || referenced {
			return err
		}
	}
	// Keep metadata/checkpoint on sharing errors. Attachment deletion was already
	// committed, so rollback cannot resurrect a live attachment without its bytes.
	if err := removeBlob(fileID); err != nil && !os.IsNotExist(err) {
		return err
	}
	if _, err := finish.ID(fileID).Delete(&files.File{}); err != nil {
		return err
	}
	if err := finish.Commit(); err != nil {
		return err
	}
	if err := os.Remove(pendingPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
