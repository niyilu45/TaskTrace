// SPDX-License-Identifier: AGPL-3.0-or-later
//
//nolint:gosmopolitan // These desktop-only messages follow the Chinese TaskTrace UI.
package tasktraceexport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"
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
func reveal(path string) error {
	// Explorer must outlive the HTTP request and remains under the user's control.
	cmd := exec.Command("explorer.exe") //nolint:noctx
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
		CmdLine:    "explorer.exe /select," + syscall.EscapeArg(path),
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法打开文件夹：%w", err)
	}
	return cmd.Process.Release()
}
