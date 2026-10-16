// SPDX-License-Identifier: AGPL-3.0-or-later
//
//nolint:gosmopolitan // These desktop-only messages follow the Chinese TaskTrace UI.
package tasktraceexport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var ErrBusy = errors.New("已有保存窗口打开，请先完成或取消该窗口")
var saveMu sync.Mutex

// Save only writes a destination chosen in the native dialog. The browser cannot
// supply a destination path, and a failed replacement leaves the previous file.
func Save(ctx context.Context, filename, markdown string) (string, error) {
	return save(ctx, filename, markdown, choosePath)
}
func save(ctx context.Context, filename, markdown string, choose func(context.Context, string) (string, error)) (string, error) {
	if !saveMu.TryLock() {
		return "", ErrBusy
	}
	defer saveMu.Unlock()
	filename = filepath.Base(strings.ReplaceAll(filename, `\`, "/"))
	if filename == "." || filename == "" {
		filename = "export.md"
	}
	if !strings.EqualFold(filepath.Ext(filename), ".md") {
		filename += ".md"
	}
	path, err := choose(ctx, filename)
	if err != nil || path == "" {
		return "", err
	}
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".md") {
		return "", errors.New("请选择 Markdown（.md）文件")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".tasktrace-export-*.md")
	if err != nil {
		return "", fmt.Errorf("创建导出文件失败：%w", err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	defer file.Close()
	if _, err = file.WriteString(markdown); err != nil {
		return "", fmt.Errorf("写入导出文件失败：%w", err)
	}
	if err = file.Sync(); err != nil {
		return "", err
	}
	if err = file.Close(); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = os.Rename(temporary, path); err != nil {
		return "", fmt.Errorf("保存导出文件失败，原文件已保留：%w", err)
	}
	return filepath.Clean(path), nil
}
func Reveal(path string) error {
	if !filepath.IsAbs(path) || !strings.EqualFold(filepath.Ext(path), ".md") {
		return errors.New("导出文件路径无效")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("导出文件已移动、删除或暂时无法访问，请检查原保存位置")
	}
	return reveal(path)
}
