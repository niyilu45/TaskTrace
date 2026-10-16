// SPDX-License-Identifier: AGPL-3.0-or-later
package tasktraceexport

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestTaskTraceExportLockedFilePreservesOriginal(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "locked.md")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0o600))
	ptr, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)
	handle, err := windows.CreateFile(ptr, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	defer func() { require.NoError(t, windows.CloseHandle(handle)) }()
	_, err = save(context.Background(), "locked.md", "replacement", func(context.Context, string) (string, error) { return path, nil })
	require.Error(t, err)
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "original", string(body))
	files, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, files, 1)
}

// Run explicitly on a Windows desktop; CI should not open user windows.
//nolint:gosmopolitan // Verify native paths containing Chinese characters.
func TestTaskTraceExportRevealExplorer(t *testing.T) {
	if os.Getenv("TASKTRACE_TEST_EXPORT_REVEAL") != "1" {
		t.Skip("requires an interactive Windows desktop")
	}
	root := filepath.Join(t.TempDir(), "导出，路径 % & 空格")
	require.NoError(t, os.Mkdir(root, 0o700))
	path := filepath.Join(root, "回顾 [1], 50%.md")
	require.NoError(t, os.WriteFile(path, []byte("# Export location test"), 0o600))
	require.NoError(t, Reveal(path))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Inspect only the new test directory, including visibility and selection.
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-STA", "-NonInteractive", "-Command", verifyExportExplorerScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Env = append(os.Environ(), "TASKTRACE_EXPORT_TEST_FILE="+path)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "VISIBLE_AND_SELECTED")
}

const verifyExportExplorerScript = `$ErrorActionPreference='Stop'
Add-Type -TypeDefinition 'using System; using System.Runtime.InteropServices; public static class ExportWindowCheck { [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hWnd); }'
$target=$env:TASKTRACE_EXPORT_TEST_FILE
$parent=[IO.Path]::GetDirectoryName($target)
$shell=New-Object -ComObject Shell.Application
$found=$false
try {
 $until=[DateTime]::UtcNow.AddSeconds(15)
 do {
  foreach($window in @($shell.Windows())) {
   try {
    if($window.Document.Folder.Self.Path -ne $parent){continue}
    if(-not [ExportWindowCheck]::IsWindowVisible([IntPtr]$window.HWND)){continue}
    foreach($item in @($window.Document.SelectedItems())) {
     if($item.Path -eq $target){$found=$true;break}
    }
   } catch {}
  }
  if($found){break}
  Start-Sleep -Milliseconds 200
 } while([DateTime]::UtcNow -lt $until)
 if(-not $found){throw 'Export directory did not open visibly with the exported file selected'}
 'VISIBLE_AND_SELECTED'
} finally {
 foreach($window in @($shell.Windows())) {
  try {if($window.Document.Folder.Self.Path -eq $parent){$window.Quit()}} catch {}
 }
 [void][Runtime.InteropServices.Marshal]::FinalReleaseComObject($shell)
}`
