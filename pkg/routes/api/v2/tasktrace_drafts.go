// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"github.com/danielgtaylor/huma/v2"
)

const taskTraceDraftLimit = 64 << 20

var taskTraceDraftKey = regexp.MustCompile(`^[A-Za-z0-9._-]{1,120}$`)
var taskTraceDraftMu sync.Mutex

type taskTraceDraftState struct {
	Exists  bool      `json:"exists"`
	Content string    `json:"content,omitempty"`
	Updated time.Time `json:"updated,omitempty"`
}

func taskTraceDraftRecover(path string) error {
	backup := path + ".bak"
	if _, err := os.Stat(path); err == nil {
		_ = os.Remove(backup)
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(backup, path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func taskTraceDraftReplace(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := taskTraceDraftRecover(path); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err = tmp.Chmod(0o600); err == nil {
		var written int
		written, err = tmp.WriteString(content)
		if err == nil && written != len(content) {
			err = io.ErrShortWrite
		}
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	backup := path + ".bak"
	_ = os.Remove(backup)
	oldExists := false
	if _, statErr := os.Stat(path); statErr == nil {
		oldExists = true
		if err = os.Rename(path, backup); err != nil {
			return err
		}
	} else if !os.IsNotExist(statErr) {
		return statErr
	}
	if err = os.Rename(tmpPath, path); err != nil {
		if oldExists {
			_ = os.Rename(backup, path)
		}
		return err
	}
	_ = os.Remove(backup)
	return nil
}

func init() { AddRouteRegistrar(RegisterTaskTraceDraftRoutes) }

func RegisterTaskTraceDraftRoutes(api huma.API) {
	tags := []string{"tasktrace-drafts"}
	Register(api, huma.Operation{OperationID: "tasktrace-draft-read", Summary: "Read a local TaskTrace draft", Method: http.MethodGet, Path: "/tasktrace/drafts/{kind}/{task}/{key}", Tags: tags}, taskTraceDraftRead)
	Register(api, huma.Operation{OperationID: "tasktrace-draft-write", Summary: "Cache a local TaskTrace draft", Description: "Writes an uncommitted editor draft under the local .cache folder. It does not modify the task or its progress.", Method: http.MethodPut, Path: "/tasktrace/drafts/{kind}/{task}/{key}", Tags: tags}, taskTraceDraftWrite)
	Register(api, huma.Operation{OperationID: "tasktrace-draft-delete", Summary: "Delete a local TaskTrace draft", Method: http.MethodDelete, Path: "/tasktrace/drafts/{kind}/{task}/{key}", Tags: tags}, taskTraceDraftDelete)
}

func taskTraceDraftPath(ctx context.Context, kind string, taskID int64, key string) (string, error) {
	if kind != "progress" && kind != "outstanding" {
		return "", errors.New("unsupported draft kind")
	}
	if taskID <= 0 || !taskTraceDraftKey.MatchString(key) {
		return "", errors.New("invalid draft key")
	}
	a, err := authFromCtx(ctx)
	if err != nil {
		return "", err
	}
	s := db.NewReadSession()
	defer s.Close()
	can, err := (&models.Task{ID: taskID}).CanUpdate(s, a)
	if err != nil {
		return "", err
	}
	if !can {
		return "", models.ErrGenericForbidden{}
	}
	cacheRoot := filepath.Clean(config.ServiceRootpath.GetString())
	if strings.EqualFold(filepath.Base(cacheRoot), "data") {
		cacheRoot = filepath.Dir(cacheRoot)
	}
	return filepath.Join(cacheRoot, ".cache", "tasktrace-drafts", strconv.FormatInt(a.GetID(), 10), kind, strconv.FormatInt(taskID, 10), key+".json"), nil
}

func taskTraceDraftRead(ctx context.Context, in *struct {
	Kind   string `path:"kind"`
	TaskID int64  `path:"task"`
	Key    string `path:"key"`
}) (*singleBody[taskTraceDraftState], error) {
	path, err := taskTraceDraftPath(ctx, in.Kind, in.TaskID, in.Key)
	if err != nil {
		return nil, translateDomainError(err)
	}
	taskTraceDraftMu.Lock()
	defer taskTraceDraftMu.Unlock()
	if err = taskTraceDraftRecover(path); err != nil {
		return nil, huma.Error500InternalServerError("recover cached draft", err)
	}
	content, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &singleBody[taskTraceDraftState]{Body: &taskTraceDraftState{}}, nil
	}
	if err != nil {
		return nil, huma.Error500InternalServerError("read cached draft", err)
	}
	info, _ := os.Stat(path)
	state := taskTraceDraftState{Exists: true, Content: string(content)}
	if info != nil {
		state.Updated = info.ModTime()
	}
	return &singleBody[taskTraceDraftState]{Body: &state}, nil
}

func taskTraceDraftWrite(ctx context.Context, in *struct {
	Kind   string `path:"kind"`
	TaskID int64  `path:"task"`
	Key    string `path:"key"`
	Body   struct {
		Content string `json:"content"`
	}
}) (*singleBody[taskTraceDraftState], error) {
	path, err := taskTraceDraftPath(ctx, in.Kind, in.TaskID, in.Key)
	if err != nil {
		return nil, translateDomainError(err)
	}
	if len(in.Body.Content) > taskTraceDraftLimit {
		return nil, huma.Error422UnprocessableEntity("cached draft is too large", fmt.Errorf("draft exceeds %d bytes", taskTraceDraftLimit))
	}
	taskTraceDraftMu.Lock()
	defer taskTraceDraftMu.Unlock()
	if err := taskTraceDraftReplace(path, in.Body.Content); err != nil {
		return nil, huma.Error500InternalServerError("replace cached draft", err)
	}
	state := taskTraceDraftState{Exists: true, Content: in.Body.Content, Updated: time.Now()}
	return &singleBody[taskTraceDraftState]{Body: &state}, nil
}

func taskTraceDraftDelete(ctx context.Context, in *struct {
	Kind   string `path:"kind"`
	TaskID int64  `path:"task"`
	Key    string `path:"key"`
}) (*singleBody[taskTraceDraftState], error) {
	path, err := taskTraceDraftPath(ctx, in.Kind, in.TaskID, in.Key)
	if err != nil {
		return nil, translateDomainError(err)
	}
	taskTraceDraftMu.Lock()
	defer taskTraceDraftMu.Unlock()
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, huma.Error500InternalServerError("delete cached draft", err)
	}
	_ = os.Remove(path + ".bak")
	return &singleBody[taskTraceDraftState]{Body: &taskTraceDraftState{}}, nil
}
