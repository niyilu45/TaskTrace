//go:build windows

// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

const taskTraceTeamSearchScript = `$Keyword=$env:TASKTRACE_TEAM_KEYWORD
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$escaped=$Keyword.Replace("'","''")
$filter="Disabled = FALSE AND (Name LIKE '%$escaped%' OR FullName LIKE '%$escaped%')"
$result=@(Get-CimInstance -ClassName Win32_UserAccount -Filter $filter -ErrorAction Stop | Select-Object -First 25 | ForEach-Object {
  [ordered]@{username=[string]$_.Name;account_name=([string]$_.Domain+'\'+[string]$_.Name);display_name=[string]$_.FullName}
})
try {
  $resolved=([Security.Principal.NTAccount]::new($Keyword)).Translate([Security.Principal.SecurityIdentifier]).Translate([Security.Principal.NTAccount]).Value
  $short=($resolved -split '\')[-1]
  if(-not ($result | Where-Object {$_.account_name -ieq $resolved})) {$result+=,[ordered]@{username=$short;account_name=$resolved;display_name=''}}
} catch {}
ConvertTo-Json -InputObject ([object[]]$result) -Compress`

const taskTraceTeamListAccessScript = `$Root=$env:TASKTRACE_TEAM_ROOT
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$resolvedRoot=[IO.Path]::GetFullPath($Root).TrimEnd('\')
$values=@()
$acl=Get-Acl -LiteralPath $resolvedRoot -ErrorAction Stop
foreach($entry in $acl.Access){
  if($entry.AccessControlType -ne 'Allow'){continue}
  if(($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::Write) -eq 0 -and ($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::Modify) -eq 0 -and ($entry.FileSystemRights -band [Security.AccessControl.FileSystemRights]::FullControl) -eq 0){continue}
  $name=$entry.IdentityReference.Value
  if($name -and $name -notmatch '^(BUILTIN|NT AUTHORITY|CREATOR OWNER)\' -and $name -notin @('Everyone','Authenticated Users')){$values+=$name}
}
$share=Get-SmbShare -ErrorAction SilentlyContinue | Where-Object {$_.Path -and ([IO.Path]::GetFullPath([string]$_.Path).TrimEnd('\') -ieq $resolvedRoot)} | Select-Object -First 1
if($null -ne $share){
  foreach($entry in @(Get-SmbShareAccess -Name $share.Name -ErrorAction SilentlyContinue)){
    if($entry.AccessControlType -eq 'Allow' -and $entry.AccessRight -in @('Change','Full') -and $entry.AccountName -notin @('Everyone','Authenticated Users')){$values+=[string]$entry.AccountName}
  }
}
ConvertTo-Json -InputObject ([object[]]@($values | Sort-Object -Unique)) -Compress`

const taskTraceTeamGrantAccessScript = `$Root=$env:TASKTRACE_TEAM_ROOT
$Member=$env:TASKTRACE_TEAM_MEMBER
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\')
$sid=([Security.Principal.NTAccount]::new($Member)).Translate([Security.Principal.SecurityIdentifier])
$canonical=$sid.Translate([Security.Principal.NTAccount]).Value
$share=Get-SmbShare -ErrorAction Stop | Where-Object {$_.Path -and ([IO.Path]::GetFullPath([string]$_.Path).TrimEnd('\') -ieq $rootPath)} | Select-Object -First 1
if($null -eq $share){throw 'teamData 尚未创建 Windows 文件共享'}
$shareAdded=$false
try {
  $existing=@(Get-SmbShareAccess -Name $share.Name -ErrorAction Stop | Where-Object {$_.AccountName -ieq $canonical -and $_.AccessControlType -eq 'Allow' -and $_.AccessRight -in @('Change','Full')})
  if($existing.Count -eq 0){Grant-SmbShareAccess -Name $share.Name -AccountName $canonical -AccessRight Change -Force -ErrorAction Stop | Out-Null;$shareAdded=$true}
  $acl=Get-Acl -LiteralPath $rootPath -ErrorAction Stop
  $inherit=[Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit'
  $rule=[Security.AccessControl.FileSystemAccessRule]::new($sid,[Security.AccessControl.FileSystemRights]::Modify,$inherit,[Security.AccessControl.PropagationFlags]::None,[Security.AccessControl.AccessControlType]::Allow)
  $acl.SetAccessRule($rule)
  Set-Acl -LiteralPath $rootPath -AclObject $acl -ErrorAction Stop
} catch {
  if($shareAdded){Revoke-SmbShareAccess -Name $share.Name -AccountName $canonical -Force -ErrorAction SilentlyContinue}
  throw
}
[ordered]@{account_name=$canonical;username=($canonical -split '\')[-1]} | ConvertTo-Json -Compress`

const taskTraceTeamRemoveAccessScript = `$Root=$env:TASKTRACE_TEAM_ROOT
$Member=$env:TASKTRACE_TEAM_MEMBER
$ErrorActionPreference='Stop'
$rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\')
$sid=([Security.Principal.NTAccount]::new($Member)).Translate([Security.Principal.SecurityIdentifier])
$canonical=$sid.Translate([Security.Principal.NTAccount]).Value
$share=Get-SmbShare -ErrorAction SilentlyContinue | Where-Object {$_.Path -and ([IO.Path]::GetFullPath([string]$_.Path).TrimEnd('\') -ieq $rootPath)} | Select-Object -First 1
if($null -ne $share){Revoke-SmbShareAccess -Name $share.Name -AccountName $canonical -Force -ErrorAction SilentlyContinue}
$acl=Get-Acl -LiteralPath $rootPath -ErrorAction Stop
$acl.PurgeAccessRules($sid)
Set-Acl -LiteralPath $rootPath -AclObject $acl -ErrorAction Stop`

func taskTraceTeamPowerShell(script string, environment ...string) ([]byte, error) {
	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}
	cmd := exec.Command("powershell.exe", args...)
	cmd.Env = append(os.Environ(), environment...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}

func taskTraceTeamSearchWindowsMembers(query string) ([]TaskTraceTeamMemberCandidate, error) {
	output, err := taskTraceTeamPowerShell(taskTraceTeamSearchScript, "TASKTRACE_TEAM_KEYWORD="+query)
	if err != nil {
		return nil, err
	}
	result := []TaskTraceTeamMemberCandidate{}
	if len(strings.TrimSpace(string(output))) > 0 {
		if err := json.Unmarshal(output, &result); err != nil {
			return nil, fmt.Errorf("parse Windows account search: %w", err)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].AccountName) < strings.ToLower(result[j].AccountName)
	})
	return result, nil
}

func taskTraceTeamListWindowsAccess(root string) ([]string, error) {
	output, err := taskTraceTeamPowerShell(taskTraceTeamListAccessScript, "TASKTRACE_TEAM_ROOT="+root)
	if err != nil {
		return nil, err
	}
	result := []string{}
	if len(strings.TrimSpace(string(output))) > 0 {
		if err := json.Unmarshal(output, &result); err != nil {
			return nil, fmt.Errorf("parse teamData access list: %w", err)
		}
	}
	return result, nil
}

func taskTraceTeamGrantWindowsAccess(root, member string) (string, error) {
	output, err := taskTraceTeamPowerShell(taskTraceTeamGrantAccessScript, "TASKTRACE_TEAM_ROOT="+root, "TASKTRACE_TEAM_MEMBER="+member)
	if err != nil {
		return "", err
	}
	var value struct {
		AccountName string `json:"account_name"`
	}
	if err := json.Unmarshal(output, &value); err != nil || value.AccountName == "" {
		return "", fmt.Errorf("Windows did not return the granted account")
	}
	return value.AccountName, nil
}

func taskTraceTeamRemoveWindowsAccess(root, member string) error {
	_, err := taskTraceTeamPowerShell(taskTraceTeamRemoveAccessScript, "TASKTRACE_TEAM_ROOT="+root, "TASKTRACE_TEAM_MEMBER="+member)
	return err
}
