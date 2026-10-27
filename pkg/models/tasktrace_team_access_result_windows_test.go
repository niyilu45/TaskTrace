//go:build windows

// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"encoding/json"
	"os"
	osuser "os/user"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The real grant script runs, but every SMB command and filesystem ACL setter
// is replaced by an in-memory mock. No share, directory ACL or UAC is changed.
func taskTraceGrantResultMockScript(t *testing.T) string {
	t.Helper()
	fakeDirectory := `$directory=[pscustomobject]@{}
$directory | Add-Member -MemberType ScriptMethod -Name GetAccessControl -Value {param($sections) [Security.AccessControl.DirectorySecurity]::new()}
$directory | Add-Member -MemberType ScriptMethod -Name SetAccessControl -Value {param($acl) $global:mockACLChanged=$true;[pscustomobject]@{unexpected_acl_result='must be suppressed'}}`
	require.Contains(t, taskTraceTeamGrantAccessScript, `$directory=[IO.DirectoryInfo]::new($rootPath)`)
	grant := strings.Replace(taskTraceTeamGrantAccessScript, `$directory=[IO.DirectoryInfo]::new($rootPath)`, fakeDirectory, 1)
	return `$global:mockACLChanged=$false
$global:mockShareRight=''
function Get-SmbShare {
 [pscustomobject]@{Name='test-team';Path=$env:TASKTRACE_TEAM_ROOT;Special=$false}
}
function Get-SmbShareAccess {
 if($env:TASKTRACE_TEST_PREVIOUS_RIGHT){[pscustomobject]@{AccountName=$env:TASKTRACE_TEAM_MEMBER;AccessControlType='Allow';AccessRight=$env:TASKTRACE_TEST_PREVIOUS_RIGHT}}
}
function Revoke-SmbShareAccess {
 param($Name,$AccountName,[switch]$Force)
 $cim=[Microsoft.Management.Infrastructure.CimInstance]::new('MSFT_SmbShareAccessControlEntry','root/Microsoft/Windows/SMB')
 $cim.CimInstanceProperties.Add([Microsoft.Management.Infrastructure.CimProperty]::Create('Name',$Name,[Microsoft.Management.Infrastructure.CimType]::String,[Microsoft.Management.Infrastructure.CimFlags]::Key))
 $cim.CimInstanceProperties.Add([Microsoft.Management.Infrastructure.CimProperty]::Create('AccountName',$AccountName,[Microsoft.Management.Infrastructure.CimType]::String,[Microsoft.Management.Infrastructure.CimFlags]::Key))
 $cim
}
function Grant-SmbShareAccess {
 param($Name,$AccountName,$AccessRight,[switch]$Force)
 $global:mockShareRight=$AccessRight
 [pscustomobject]@{unexpected_grant_result='must be suppressed';AccountName=$AccountName}
}
` + grant + `
if(-not $global:mockACLChanged){throw 'grant did not reach the mock ACL setter'}
$expectedRight=if($env:TASKTRACE_TEAM_ACCESS -eq 'read'){'Read'}else{'Change'}
if($global:mockShareRight -ne $expectedRight){throw 'grant did not use the selected share right'}
`
}

func TestTaskTraceTeamGrantedAccountSuppressesMutationOutput(t *testing.T) {
	current, err := osuser.Current()
	require.NoError(t, err)
	for _, test := range []struct{ name, previous, access string }{
		{"first read", "", "read"}, {"first write", "", "write"},
		{"read upgraded to write", "Read", "write"}, {"write changed to read", "Change", "read"},
	} {
		t.Run(test.name, func(t *testing.T) {
			output, err := taskTraceTeamPowerShell(taskTraceGrantResultMockScript(t), "TASKTRACE_TEAM_ROOT=C:\\tasktrace-mock-team", "TASKTRACE_TEAM_MEMBER="+current.Username, "TASKTRACE_TEAM_ACCESS="+test.access, "TASKTRACE_TEST_PREVIOUS_RIGHT="+test.previous)
			require.NoError(t, err, string(output))
			require.NotContains(t, string(output), "unexpected_", "cmdlet or ACL return values must not pollute the grant result")
			require.True(t, strings.HasPrefix(strings.TrimSpace(string(output)), taskTraceTeamGrantResultPrefix), "mutation output preceded the acknowledgement: %s", output)
			account, err := taskTraceTeamDecodeGrantedAccount(output)
			require.NoError(t, err, string(output))
			require.Equal(t, current.Username, account)
		})
	}
}

func TestTaskTraceTeamElevatedGrantResultUsesAccountFrame(t *testing.T) {
	current, err := osuser.Current()
	require.NoError(t, err)
	for _, access := range []string{"read", "write"} {
		t.Run(access, func(t *testing.T) {
			directory := t.TempDir()
			inputPath := filepath.Join(directory, "request.json")
			outputPath := filepath.Join(directory, "result.json")
			payload, err := json.Marshal(map[string]string{"root": `C:\tasktrace-mock-team`, "member": current.Username, "access": access})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(inputPath, payload, 0600))
			wrapper := strings.Replace(taskTraceTeamGrantElevatedScript(), taskTraceTeamGrantAccessScript, taskTraceGrantResultMockScript(t), 1)
			host := `$wrapper=[ScriptBlock]::Create($env:TASKTRACE_TEST_WRAPPER)
& $wrapper $env:TASKTRACE_TEST_INPUT $env:TASKTRACE_TEST_OUTPUT`
			output, err := taskTraceTeamPowerShell(host, "TASKTRACE_TEST_WRAPPER="+wrapper, "TASKTRACE_TEST_INPUT="+inputPath, "TASKTRACE_TEST_OUTPUT="+outputPath, "TASKTRACE_TEST_PREVIOUS_RIGHT=Read")
			require.NoError(t, err, string(output))
			resultBytes, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			var result struct {
				OK    bool   `json:"ok"`
				Value string `json:"value"`
				Error string `json:"error"`
			}
			require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(string(resultBytes), "\ufeff")), &result))
			require.True(t, result.OK, result.Error)
			account, err := taskTraceTeamDecodeGrantedAccount([]byte(result.Value))
			require.NoError(t, err, result.Value)
			require.Equal(t, current.Username, account)
		})
	}
}

func TestTaskTraceTeamLegacyGrantOutputReproducesAccountFailure(t *testing.T) {
	// Previously a successful Revoke-SmbShareAccess result was emitted before
	// the JSON. Casting that array to string is not a valid JSON document.
	output, err := taskTraceTeamPowerShell(`$cim=[Microsoft.Management.Infrastructure.CimInstance]::new('MSFT_SmbShareAccessControlEntry','root/Microsoft/Windows/SMB')
$cim.CimInstanceProperties.Add([Microsoft.Management.Infrastructure.CimProperty]::Create('Name','test-team',[Microsoft.Management.Infrastructure.CimType]::String,[Microsoft.Management.Infrastructure.CimFlags]::Key))
$cim.CimInstanceProperties.Add([Microsoft.Management.Infrastructure.CimProperty]::Create('AccountName','DOMAIN\alice',[Microsoft.Management.Infrastructure.CimType]::String,[Microsoft.Management.Infrastructure.CimFlags]::Key))
$value=@($cim,'{"account_name":"DOMAIN\\alice"}')
[string]$value`)
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "account_name")
	require.Contains(t, string(output), "MSFT_SmbShareAccessControlEntry")
	require.False(t, json.Valid(output))
}

func TestTaskTraceTeamDecodeGrantedAccountProtocol(t *testing.T) {
	for _, test := range []struct {
		name, body, want string
		wantError        bool
	}{
		{name: "legacy JSON", body: `{"account_name":"DOMAIN\\alice"}`, want: `DOMAIN\alice`},
		{name: "framed JSON", body: taskTraceTeamGrantResultPrefix + `{"account_name":"DOMAIN\\alice"}`, want: `DOMAIN\alice`},
		{name: "warning before frame", body: "WARNING: CIM formatting output\r\n" + taskTraceTeamGrantResultPrefix + `{"account_name":"DOMAIN\\alice"}` + "\r\n", want: `DOMAIN\alice`},
		{name: "SMB object text beside frame", body: "MSFT_SmbShareAccessControlEntry " + taskTraceTeamGrantResultPrefix + `{"account_name":"DOMAIN\\alice"}`, want: `DOMAIN\alice`},
		{name: "BOM legacy JSON", body: "\ufeff" + `{"account_name":"DOMAIN\\alice"}`, want: `DOMAIN\alice`},
		{name: "missing output", wantError: true},
		{name: "malformed frame", body: taskTraceTeamGrantResultPrefix + `{`, wantError: true},
		{name: "empty account", body: taskTraceTeamGrantResultPrefix + `{"account_name":"  "}`, wantError: true},
		{name: "missing account", body: taskTraceTeamGrantResultPrefix + `{"username":"alice"}`, wantError: true},
		{name: "unframed noise is not success", body: `MSFT_SmbShareAccessControlEntry {"account_name":"DOMAIN\\alice"}`, wantError: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			account, err := taskTraceTeamDecodeGrantedAccount([]byte(test.body))
			if test.wantError {
				require.Error(t, err)
				require.Empty(t, account)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, account)
		})
	}
}
