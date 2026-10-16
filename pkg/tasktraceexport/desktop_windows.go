// SPDX-License-Identifier: AGPL-3.0-or-later
//
//nolint:gosmopolitan // These desktop-only messages follow the Chinese TaskTrace UI.
package tasktraceexport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const Supported = true
const saveDialogScript = `$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Windows.Forms
$owner=New-Object System.Windows.Forms.Form
$owner.TopMost=$true
$owner.ShowInTaskbar=$false
$owner.StartPosition='CenterScreen'
$dialog=New-Object System.Windows.Forms.SaveFileDialog
$dialog.Title='导出 Markdown'
$dialog.Filter='Markdown 文档 (*.md)|*.md'
$dialog.DefaultExt='md'
$dialog.AddExtension=$true
$dialog.OverwritePrompt=$true
$dialog.FileName=$env:TASKTRACE_EXPORT_FILENAME
try {
 $path=''
 if($dialog.ShowDialog($owner) -eq [Windows.Forms.DialogResult]::OK){$path=$dialog.FileName}
 [Console]::Write((@{path=$path}|ConvertTo-Json -Compress))
} finally {$dialog.Dispose();$owner.Dispose()}`

func choosePath(ctx context.Context, filename string) (string, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-STA", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", saveDialogScript)
	cmd.Env = append(os.Environ(), "TASKTRACE_EXPORT_FILENAME="+filename)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("无法打开系统保存窗口：%w", err)
	}
	var result struct {
		Path string `json:"path"`
	}
	if err = json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("读取保存位置失败：%w", err)
	}
	return result.Path, nil
}

var exportShell = windows.NewLazySystemDLL("shell32.dll")
var parseExportPath = exportShell.NewProc("SHParseDisplayName")
var openExportFolder = exportShell.NewProc("SHOpenFolderAndSelectItems")

func reveal(path string) error {
	// Shell operations require COM on the same OS thread through cleanup.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && !errors.Is(err, syscall.Errno(1)) {
		return fmt.Errorf("初始化文件夹窗口失败：%w", err)
	}
	defer windows.CoUninitialize()
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("导出路径无效：%w", err)
	}
	var item unsafe.Pointer
	result, _, _ := parseExportPath.Call(uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&item)), 0, 0)
	if result != 0 {
		return fmt.Errorf("无法定位导出文件（Windows 错误 0x%08X）", result)
	}
	defer windows.CoTaskMemFree(item)
	// A full file PIDL with cidl=0 opens its parent and selects the file. Avoid
	// inherited hidden-window flags and command-line quoting of user file names.
	result, _, _ = openExportFolder.Call(uintptr(item), 0, 0, 0)
	if result != 0 {
		return fmt.Errorf("无法打开所在文件夹（Windows 错误 0x%08X）", result)
	}
	return nil
}
