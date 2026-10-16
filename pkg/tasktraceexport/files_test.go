// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktraceexport

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskTraceExportFiles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "report space.md")
	choose := func(context.Context, string) (string, error) { return path, nil }
	saved, err := save(context.Background(), "report.md", "first", choose)
	require.NoError(t, err)
	require.Equal(t, path, saved)
	saved, err = save(context.Background(), "report.md", "replacement", choose)
	require.NoError(t, err)
	require.Equal(t, path, saved)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(body))
	saved, err = save(context.Background(), "report.md", "cancel", func(context.Context, string) (string, error) { return "", nil })
	require.NoError(t, err)
	require.Empty(t, saved)
	saved, err = save(context.Background(), "report.md", "failed", func(context.Context, string) (string, error) { return "", errors.New("dialog unavailable") })
	require.Error(t, err)
	require.Empty(t, saved)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = save(ctx, "report.md", "cancelled context", choose)
	require.ErrorIs(t, err, context.Canceled)
	_, err = save(context.Background(), "report.md", "bad path", func(context.Context, string) (string, error) { return filepath.Join(root, "missing", "report.md"), nil })
	require.Error(t, err)
	_, err = save(context.Background(), "report.md", "bad extension", func(context.Context, string) (string, error) { return filepath.Join(root, "program.exe"), nil })
	require.Error(t, err)
	_, err = save(context.Background(), `C:\folder\report.md`, "basename", func(_ context.Context, name string) (string, error) {
		require.Equal(t, "report.md", name)
		return "", nil
	})
	require.NoError(t, err)
	require.Error(t, Reveal(filepath.Join(root, "missing.md")))
	body, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "replacement", string(body))
	files, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, files, 1, "failed writes leave no staging files")
}
func TestTaskTraceExportSerializesDialogs(t *testing.T) {
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		_, err := save(context.Background(), "one.md", "", func(context.Context, string) (string, error) { close(entered); <-release; return "", nil })
		finished <- err
	}()
	<-entered
	_, err := save(context.Background(), "two.md", "", func(context.Context, string) (string, error) { return "", nil })
	require.ErrorIs(t, err, ErrBusy)
	close(release)
	require.NoError(t, <-finished)
}
