// SPDX-License-Identifier: AGPL-3.0-or-later
//
//nolint:gosmopolitan // Local recovery errors are shown directly in TaskTrace's Chinese desktop UI.
package tasktracedata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func detectionRoot(path string) (string, error) {
	path = strings.TrimSpace(path)
	if len(path) >= 2 && ((path[0] == '"' && path[len(path)-1] == '"') || (path[0] == '\'' && path[len(path)-1] == '\'')) {
		path = path[1 : len(path)-1]
	}
	if strings.TrimSpace(path) == "" {
		return "", errors.New("请选择包含 TaskTrace 数据的目录")
	}
	root, err := canonicalDirectory(os.ExpandEnv(path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("无法读取所选目录，请检查路径和访问权限：%w", err)
	}
	if !info.IsDir() {
		if !strings.EqualFold(info.Name(), "tasktrace.db") {
			return "", errors.New("请选择数据目录或 tasktrace.db 文件")
		}
		root = filepath.Dir(root)
	}
	return root, nil
}

// Follow only TaskTrace's data containers. Do not walk attachments, source trees,
// symlinks or arbitrary disk contents when users paste a broad parent directory.
func resolveCandidateDirectories(path string) ([]string, error) {
	root, err := detectionRoot(path)
	if err != nil {
		return nil, err
	}
	for ancestor := root; filepath.Dir(ancestor) != ancestor; ancestor = filepath.Dir(ancestor) {
		if strings.EqualFold(filepath.Base(ancestor), "teamData") {
			relative, relErr := filepath.Rel(ancestor, root)
			if relErr != nil {
				return nil, relErr
			}
			root = filepath.Join(filepath.Dir(ancestor), "data", relative)
			if _, statErr := os.Stat(root); statErr != nil {
				return nil, errors.New("这是 teamData 团队数据目录，旁边未找到配套 data。请在第一个输入框填写个人 data 目录，并在团队数据输入框填写此路径")
			}
			break
		}
	}
	queue := []string{root}
	seen := map[string]bool{}
	var candidates []string
	for len(queue) > 0 {
		directory := queue[0]
		queue = queue[1:]
		key := strings.ToLower(directory)
		if seen[key] {
			continue
		}
		seen[key] = true
		if len(seen) > 4096 {
			return nil, errors.New("所选目录包含过多数据目录，请选择具体的 data 或 imports 目录后重试")
		}
		entries, readErr := os.ReadDir(directory)
		if readErr != nil {
			return nil, fmt.Errorf("无法读取数据目录 %s：%w", directory, readErr)
		}
		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}
			if !entry.IsDir() {
				if strings.EqualFold(entry.Name(), "tasktrace.db") && entry.Type().IsRegular() {
					candidates = append(candidates, directory)
				}
				continue
			}
			name := strings.ToLower(entry.Name())
			if name == "data" || name == "imports" || name == "current" || name == "tasktrace-local" || name == "dist" || name == "releases" || name == "backups" ||
				strings.EqualFold(filepath.Base(directory), "imports") || strings.HasPrefix(name, strings.ToLower(backupDirectoryPrefix)) {
				queue = append(queue, filepath.Join(directory, entry.Name()))
			}
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("在 %s 中未找到 TaskTrace 数据库。支持新版和旧版 data 目录；请确认已完整复制包含 tasktrace.db 的文件夹", root)
	}
	return candidates, nil
}

// A copied data/imports/<stamp> pairs with teamData/imports/<stamp>, not with
// another import or the outer teamData container. Renamed data containers work too.
func dataLayout(directory string) (base, relative string) {
	base = directory
	for ancestor := directory; filepath.Dir(ancestor) != ancestor; ancestor = filepath.Dir(ancestor) {
		name := filepath.Base(ancestor)
		if strings.EqualFold(name, "data") {
			base = ancestor
			break
		}
		if strings.EqualFold(name, "imports") || strings.EqualFold(name, "current") {
			base = filepath.Dir(ancestor)
			break
		}
	}
	relative, _ = filepath.Rel(base, directory)
	return base, relative
}

func resolveTeamDataDirectory(dataDirectory, selected string) (string, error) {
	root, err := detectionRoot(selected)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(filepath.Join(root, "teamData")); statErr == nil && info.IsDir() {
		root = filepath.Join(root, "teamData")
	}
	_, relative := dataLayout(dataDirectory)
	if relative == "." {
		return root, nil
	}
	matched := filepath.Join(root, relative)
	if strings.EqualFold(filepath.Base(root), "imports") && strings.EqualFold(filepath.Base(filepath.Dir(relative)), "imports") {
		matched = filepath.Join(root, filepath.Base(relative))
	}
	if info, statErr := os.Stat(matched); statErr == nil && info.IsDir() {
		return matched, nil
	}
	if info, statErr := os.Stat(filepath.Join(root, "imports")); (statErr == nil && info.IsDir()) || strings.EqualFold(filepath.Base(root), "imports") {
		return "", errors.New("所选 teamData 包含多份数据，但没有找到与所选个人数据对应的目录，请填写配套的具体团队数据目录")
	}
	return root, nil
}
