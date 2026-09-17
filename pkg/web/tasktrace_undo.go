// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package web

import (
	"context"

	"xorm.io/xorm"
)

type taskTraceUndoContextKey struct{}
type TaskTraceUndoOptions struct {
	Enabled bool
	GroupID string
}

func WithTaskTraceUndo(ctx context.Context, groupID string) context.Context {
	return context.WithValue(ctx, taskTraceUndoContextKey{}, TaskTraceUndoOptions{
		Enabled: true,
		GroupID: groupID,
	})
}

func TaskTraceUndoFromContext(ctx context.Context) TaskTraceUndoOptions {
	options, _ := ctx.Value(taskTraceUndoContextKey{}).(TaskTraceUndoOptions)
	return options
}

// Unsupported models never reach Lock or Capture, even if a client supplies the opt-in header.
type MutationUndoHooks struct {
	Supports func(string, any) bool
	Lock     func(context.Context, *xorm.Session, Auth) error
	Capture  func(context.Context, *xorm.Session, Auth, string, any) (func() error, error)
}

var mutationUndoHooks MutationUndoHooks

func RegisterMutationUndoHooks(hooks MutationUndoHooks) { mutationUndoHooks = hooks }

func mutationUndoEnabled(ctx context.Context, operation string, model any) bool {
	return TaskTraceUndoFromContext(ctx).Enabled && mutationUndoHooks.Supports != nil && mutationUndoHooks.Supports(operation, model)
}

func LockMutationUndo(ctx context.Context, s *xorm.Session, a Auth, operation string, model any) error {
	if !mutationUndoEnabled(ctx, operation, model) || mutationUndoHooks.Lock == nil {
		return nil
	}
	return mutationUndoHooks.Lock(ctx, s, a)
}

func CaptureMutationUndo(ctx context.Context, s *xorm.Session, a Auth, operation string, model any) (func() error, error) {
	if !mutationUndoEnabled(ctx, operation, model) || mutationUndoHooks.Capture == nil {
		return nil, nil
	}
	return mutationUndoHooks.Capture(ctx, s, a, operation, model)
}
