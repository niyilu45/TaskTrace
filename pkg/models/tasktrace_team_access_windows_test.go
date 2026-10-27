//go:build windows

// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	osuser "os/user"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		{name: "security privilege", message: "The process does not possess the SeSecurityPrivilege privilege which is required for this operation", want: true},
		{name: "unrelated failure", message: "teamData 尚未创建 Windows 文件共享", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, taskTraceTeamAccessNeedsElevation(errors.New(test.message)))
		})
	}
}

func TestTaskTraceTeamAccessScriptsOnlyWriteAccessRules(t *testing.T) {
	for name, script := range map[string]string{
		"grant":  taskTraceTeamGrantAccessScript,
		"remove": taskTraceTeamRemoveAccessScript,
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, script, "AccessControlSections]::Access")
			assert.NotContains(t, script, "Set-Acl")
		})
	}
}

func TestTaskTraceTeamPowerShellScriptsParse(t *testing.T) {
	for name, script := range map[string]string{
		"search":          taskTraceTeamSearchScript,
		"list":            taskTraceTeamListAccessScript,
		"grant":           taskTraceTeamGrantAccessScript,
		"ensure share":    taskTraceTeamEnsureShareScript,
		"find share":      taskTraceTeamFindShareScript,
		"elevated grant":  taskTraceTeamGrantElevatedScript(),
		"remove":          taskTraceTeamRemoveAccessScript,
		"elevated remove": taskTraceTeamRemoveElevatedScript(),
	} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "[ScriptBlock]::Create($env:TASKTRACE_SCRIPT) | Out-Null")
			cmd.Env = append(os.Environ(), "TASKTRACE_SCRIPT="+script)
			output, err := cmd.CombinedOutput()
			assert.NoError(t, err, string(output))
		})
	}
}

func TestTaskTraceTeamAccessListFiltersBuiltInSIDs(t *testing.T) {
	assert.Contains(t, taskTraceTeamListAccessScript, "S-1-5-32-")
	assert.Contains(t, taskTraceTeamListAccessScript, "Test-SystemPrincipal")
	assert.Contains(t, taskTraceTeamListAccessScript, "account_name=$name;access=$access")
}

func TestTaskTraceTeamAccessSupportsReadAndWriteMaximums(t *testing.T) {
	assert.Contains(t, taskTraceTeamGrantAccessScript, "TASKTRACE_TEAM_ACCESS")
	assert.Contains(t, taskTraceTeamGrantAccessScript, "AccessRight $shareRight")
	assert.Contains(t, taskTraceTeamGrantAccessScript, "ReadAndExecute")
	assert.Contains(t, taskTraceTeamGrantAccessScript, "PurgeAccessRules")
	assert.Contains(t, taskTraceTeamGrantAccessScript, "previousRight")
}
func TestTaskTraceTeamSystemAccountPattern(t *testing.T) {
	script := `$ErrorActionPreference='Stop'
$pattern=$env:TASKTRACE_TEAM_PATTERN
if('BUILTIN\Administrators' -notmatch $pattern){throw 'system account was not matched'}
if('DOMAIN\alice' -match $pattern){throw 'collaboration account was matched'}`
	output, err := taskTraceTeamPowerShell(script, "TASKTRACE_TEAM_PATTERN="+taskTraceTeamSystemAccountPattern)
	assert.NoError(t, err, string(output))
}

func TestTaskTraceTeamWindowsAccessClassifiesConfirmation(t *testing.T) {
	shareErr := taskTraceTeamClassifyWindowsAccessError(errors.New("PowerShell: TASKTRACE_TEAM_SHARE_REQUIRED: missing"))
	assert.True(t, IsTaskTraceTeamShareRequired(fmt.Errorf("grant member: %w", shareErr)))
	assert.False(t, IsTaskTraceTeamAdminRequired(shareErr))
	adminErr := taskTraceTeamClassifyWindowsAccessError(errors.New("Grant-SmbShareAccess: Access is denied"))
	assert.True(t, IsTaskTraceTeamAdminRequired(fmt.Errorf("grant member: %w", adminErr)))
	assert.False(t, IsTaskTraceTeamShareRequired(adminErr))
	assert.NoError(t, taskTraceTeamClassifyWindowsAccessError(nil))
}

func TestTaskTraceTeamWindowsAccessOptionsAreExplicit(t *testing.T) {
	for _, confirmed := range []bool{false, true} {
		environment := taskTraceTeamAccessEnvironment(`C:\test-team`, `DOMAIN\alice`, "read", TaskTraceTeamAccessOptions{CreateShare: confirmed})
		assert.Contains(t, environment, "TASKTRACE_TEAM_CREATE_SHARE="+map[bool]string{false: "0", true: "1"}[confirmed])
		assert.Contains(t, environment, "TASKTRACE_TEAM_ACCESS=read")
		assert.Contains(t, strings.Join(environment, "\n"), "TASKTRACE_TEAM_SHARE_OWNER=")
	}
}

func TestTaskTraceTeamShareCreationRequiresConfirmation(t *testing.T) {
	current, err := osuser.Current()
	require.NoError(t, err)
	for _, test := range []struct {
		name        string
		mode        string
		confirm     bool
		wantError   bool
		wantCreated bool
		wantName    string
	}{
		{name: "missing without confirmation", mode: "missing", wantError: true},
		{name: "ancestor share applies to imports", mode: "ancestor", wantName: "parent-team"},
		{name: "similar path prefix is not an ancestor", mode: "prefix", wantError: true},
		{name: "administrative share is not collaboration sharing", mode: "admin", wantError: true},
		{name: "missing with confirmation", mode: "missing", confirm: true, wantCreated: true, wantName: "teamData"},
		{name: "existing does not need confirmation", mode: "existing", wantName: "original-share"},
		{name: "existing is not expanded by confirmation", mode: "existing", confirm: true, wantName: "original-share"},
		{name: "different path keeps old share", mode: "collision", confirm: true, wantCreated: true},
		{name: "all names occupied does not replace share", mode: "all-collisions", confirm: true, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Every SMB command is replaced locally. No real share or ACL is changed.
			script := `$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$rootPath='C:\tasktrace-test-team'
$global:created=$false
$global:createdOwner=''
function Get-SmbShare {
 param([string]$Name)
 if($env:TASKTRACE_TEST_SHARE_MODE -eq 'existing' -and -not $Name){return [pscustomobject]@{Name='original-share';Path=$rootPath}}
 if($env:TASKTRACE_TEST_SHARE_MODE -eq 'ancestor' -and -not $Name){return [pscustomobject]@{Name='parent-team';Path='C:\'}}
 if($env:TASKTRACE_TEST_SHARE_MODE -eq 'prefix' -and -not $Name){return [pscustomobject]@{Name='other-team';Path=$rootPath+'2'}}
 if($env:TASKTRACE_TEST_SHARE_MODE -eq 'admin' -and -not $Name){return [pscustomobject]@{Name='C$';Path='C:\';Special=$true}}
 if(($env:TASKTRACE_TEST_SHARE_MODE -in @('collision','all-collisions')) -and $Name -eq 'teamData'){return [pscustomobject]@{Name='teamData';Path='C:\othere-team'}}
 if($env:TASKTRACE_TEST_SHARE_MODE -eq 'all-collisions' -and $Name){return [pscustomobject]@{Name=$Name;Path='C:\othere-team'}}
}
function New-SmbShare {
 param([string]$Name,[string]$Path,[string]$FullAccess)
 $global:created=$true
 $global:createdOwner=$FullAccess
 return [pscustomobject]@{Name=$Name;Path=$Path}
}
` + taskTraceTeamEnsureShareScript + `
[ordered]@{created=$global:created;owner=$global:createdOwner;name=$share.Name;path=$share.Path} | ConvertTo-Json -Compress`
			createValue := "0"
			if test.confirm {
				createValue = "1"
			}
			output, err := taskTraceTeamPowerShell(script, "TASKTRACE_TEAM_CREATE_SHARE="+createValue, "TASKTRACE_TEAM_SHARE_OWNER="+current.Username, "TASKTRACE_TEST_SHARE_MODE="+test.mode)
			if test.wantError {
				assert.Error(t, err)
				if !test.confirm {
					assert.Contains(t, err.Error(), "TASKTRACE_TEAM_SHARE_REQUIRED")
				}
				return
			}
			if !assert.NoError(t, err, string(output)) {
				return
			}
			var result struct {
				Created bool   `json:"created"`
				Owner   string `json:"owner"`
				Name    string `json:"name"`
				Path    string `json:"path"`
			}
			if !assert.NoError(t, json.Unmarshal(output, &result)) {
				return
			}
			assert.Equal(t, test.wantCreated, result.Created)
			if test.mode == "ancestor" {
				assert.Equal(t, `C:\`, result.Path)
			} else {
				assert.Equal(t, `C:\tasktrace-test-team`, result.Path)
			}
			if test.wantName != "" {
				assert.Equal(t, test.wantName, result.Name)
			} else {
				assert.True(t, strings.HasPrefix(result.Name, "TaskTrace-teamData-"))
			}
			if result.Created {
				assert.NotEmpty(t, result.Owner)
				assert.NotEqual(t, "Everyone", result.Owner)
			}
		})
	}
}

func TestTaskTraceTeamAccessScanRequiresBothPermissionLayers(t *testing.T) {
	current, err := osuser.Current()
	require.NoError(t, err)
	for _, test := range []struct {
		name       string
		shared     bool
		fileRight  string
		ancestor   bool
		denyRight  string
		denyShare  bool
		extraRight string
		wantAccess string
	}{
		{name: "local NTFS access is not a network share", fileRight: "Modify"},
		{name: "SMB without NTFS does not grant access", shared: true},
		{name: "read NTFS limits write SMB", shared: true, fileRight: "ReadAndExecute", wantAccess: "read"},
		{name: "both layers writable", shared: true, fileRight: "Modify", wantAccess: "write"},
		{name: "ancestor UNC keeps relative path", shared: true, ancestor: true, fileRight: "Modify", wantAccess: "write"},
		{name: "explicit NTFS read deny excludes member", shared: true, fileRight: "Modify", denyRight: "Read", wantAccess: ""},
		{name: "explicit NTFS write deny limits member", shared: true, fileRight: "Modify", denyRight: "Write", wantAccess: "read"},
		{name: "explicit SMB deny excludes member", shared: true, fileRight: "Modify", denyShare: true, wantAccess: ""},
		{name: "allow rules combine", shared: true, fileRight: "ReadAndExecute", extraRight: "Write", wantAccess: "write"},
	} {
		t.Run(test.name, func(t *testing.T) {
			script := `$ErrorActionPreference='Stop'
function Get-Acl {
 param($LiteralPath)
 $rules=@()
 if($env:TASKTRACE_TEST_FILE_RIGHT){$rules+= [pscustomobject]@{AccessControlType='Allow';IdentityReference=[Security.Principal.NTAccount]::new($env:TASKTRACE_TEST_ACCOUNT);FileSystemRights=[Security.AccessControl.FileSystemRights]$env:TASKTRACE_TEST_FILE_RIGHT}}
 if($env:TASKTRACE_TEST_DENY_RIGHT){$rules+= [pscustomobject]@{AccessControlType='Deny';IdentityReference=[Security.Principal.NTAccount]::new($env:TASKTRACE_TEST_ACCOUNT);FileSystemRights=[Security.AccessControl.FileSystemRights]$env:TASKTRACE_TEST_DENY_RIGHT}}
 if($env:TASKTRACE_TEST_EXTRA_RIGHT){$rules+= [pscustomobject]@{AccessControlType='Allow';IdentityReference=[Security.Principal.NTAccount]::new($env:TASKTRACE_TEST_ACCOUNT);FileSystemRights=[Security.AccessControl.FileSystemRights]$env:TASKTRACE_TEST_EXTRA_RIGHT}}
 return [pscustomobject]@{Access=$rules}
}
function Get-SmbShare {
 if($env:TASKTRACE_TEST_SHARED -eq '1'){
  $shareRoot=if($env:TASKTRACE_TEST_ANCESTOR -eq '1'){'C:\'}else{$env:TASKTRACE_TEAM_ROOT}
  return [pscustomobject]@{Name='test';Path=$shareRoot}
 }
}
function Get-SmbShareAccess {
 if($env:TASKTRACE_TEST_DENY_SHARE -eq '1'){[pscustomobject]@{AccessControlType='Deny';AccountName=$env:TASKTRACE_TEST_ACCOUNT;AccessRight='Full'}}
 [pscustomobject]@{AccessControlType='Allow';AccountName=$env:TASKTRACE_TEST_ACCOUNT;AccessRight='Change'}
}
` + taskTraceTeamListAccessScript
			sharedValue := "0"
			if test.shared {
				sharedValue = "1"
			}
			output, err := taskTraceTeamPowerShell(script, `TASKTRACE_TEAM_ROOT=C:\tasktrace-test-team`, "TASKTRACE_TEST_SHARED="+sharedValue, "TASKTRACE_TEST_ANCESTOR="+map[bool]string{false: "0", true: "1"}[test.ancestor], "TASKTRACE_TEST_FILE_RIGHT="+test.fileRight, "TASKTRACE_TEST_ACCOUNT="+current.Username, "TASKTRACE_TEST_DENY_RIGHT="+test.denyRight, "TASKTRACE_TEST_EXTRA_RIGHT="+test.extraRight, "TASKTRACE_TEST_DENY_SHARE="+map[bool]string{false: "0", true: "1"}[test.denyShare])
			if !assert.NoError(t, err, string(output)) {
				return
			}
			var snapshot struct {
				Shared  bool                            `json:"shared"`
				Path    string                          `json:"path"`
				Members []TaskTraceTeamRepositoryMember `json:"members"`
			}
			if !assert.NoError(t, json.Unmarshal(output, &snapshot)) {
				return
			}
			assert.Equal(t, test.shared, snapshot.Shared)
			if test.ancestor {
				assert.True(t, strings.HasSuffix(snapshot.Path, `\test\tasktrace-test-team`), snapshot.Path)
			}
			if test.wantAccess == "" {
				assert.Empty(t, snapshot.Members)
			} else if assert.Len(t, snapshot.Members, 1) {
				assert.Equal(t, test.wantAccess, snapshot.Members[0].Access)
			}
		})
	}
}
