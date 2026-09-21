//go:build windows

// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTaskTraceTeamAccessNeedsElevation(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "smb cmdlet access denied", message: "Grant-SmbShareAccess: Access is denied", want: true},
		{name: "windows system error", message: "Windows System Error 5", want: true},
		{name: "localized access denied", message: "拒绝访问", want: true},
		{name: "cim exception", message: "CimException 0x80070005", want: true},
		{name: "unrelated failure", message: "teamData 尚未创建 Windows 文件共享", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, taskTraceTeamAccessNeedsElevation(errors.New(test.message)))
		})
	}
}

func TestTaskTraceTeamPowerShellScriptsParse(t *testing.T) {
	for name, script := range map[string]string{
		"search":         taskTraceTeamSearchScript,
		"grant":          taskTraceTeamGrantAccessScript,
		"elevated grant": taskTraceTeamGrantElevatedScript(),
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "[ScriptBlock]::Create($env:TASKTRACE_SCRIPT) | Out-Null")
			cmd.Env = append(os.Environ(), "TASKTRACE_SCRIPT="+script)
			output, err := cmd.CombinedOutput()
			assert.NoError(t, err, string(output))
		})
	}
}
