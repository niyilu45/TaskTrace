// SPDX-License-Identifier: AGPL-3.0-or-later
package apiv2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"github.com/danielgtaylor/huma/v2"
)

const taskTraceDraftLimit = 64 << 20

var taskTraceDraftKey = regexp.MustCompile(`^[A-Za-z0-9._-]{1,120}$`)

type taskTraceDraftState struct {
	Exists  bool      `json:"exists"`
	Content string    `json:"content,omitempty"`
	Updated time.Time `json:"updated,omitempty"`
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
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, huma.Error500InternalServerError("create draft cache", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(in.Body.Content), 0o600); err != nil {
		return nil, huma.Error500InternalServerError("write cached draft", err)
	}
	_ = os.Remove(path)
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
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
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, huma.Error500InternalServerError("delete cached draft", err)
	}
	return &singleBody[taskTraceDraftState]{Body: &taskTraceDraftState{}}, nil
}
