// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"code.vikunja.io/api/pkg/files"
	"code.vikunja.io/api/pkg/web"
	"xorm.io/xorm"
)

func taskTraceTeamCopyAttachment(destination io.Writer, source io.Reader) (digest string, size uint64, err error) {
	hash := sha256.New()
	count, err := io.Copy(io.MultiWriter(destination, hash), source)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), uint64(count), nil
}

func taskTraceTeamPublishAttachment(path string, source *files.File, expected string, size uint64) error {
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := source.LoadFileByID(); err != nil {
		return err
	}
	defer source.File.Close()
	temp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	digest, count, copyErr := taskTraceTeamCopyAttachment(temp, source.File)
	closeErr := temp.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if digest != expected || count != size {
		return fmt.Errorf("local attachment %q changed during synchronization", source.Name)
	}
	if err := os.Rename(temp.Name(), path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return err
		}
	}
	return nil
}

func taskTraceTeamImportAttachment(s *xorm.Session, a web.Auth, binding *TaskTraceTeamBinding, taskID int64, shared TaskTraceTeamAttachment) (int64, error) {
	integrityError := func() error { return fmt.Errorf("shared attachment %s failed integrity check", shared.Name) }
	if _, err := hex.DecodeString(shared.ID); len(shared.ID) != sha256.Size*2 || err != nil {
		return 0, integrityError()
	}
	source, err := os.Open(taskTraceTeamAttachmentBlobPath(binding, shared.ID))
	if err != nil {
		return 0, fmt.Errorf("read shared attachment %s: %w", shared.Name, err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, integrityError()
	}
	if err := files.CheckSizeLimit(uint64(info.Size())); err != nil {
		return 0, ErrTaskAttachmentIsTooLarge{Size: uint64(info.Size())}
	}
	// Stage the same bytes that were hashed. Reading the shared file again
	// after validation would let a concurrent writer change the imported data.
	temp, err := os.CreateTemp("", "tasktrace-shared-attachment-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	digest, count, err := taskTraceTeamCopyAttachment(temp, io.LimitReader(source, info.Size()))
	if err != nil {
		return 0, err
	}
	if digest != shared.ID || count != uint64(info.Size()) {
		return 0, integrityError()
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	attachment := &TaskAttachment{TaskID: taskID}
	if err := attachment.NewAttachment(s, temp, shared.Name, count, a); err != nil {
		return 0, err
	}
	return attachment.ID, nil
}
