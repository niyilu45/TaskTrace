//go:build windows

// SPDX-License-Identifier: AGPL-3.0-or-later

package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

var errTaskTraceTeamAdminRequired = errors.New("设置 teamData 共享读写权限需要 Windows 管理员授权")

var taskTraceWindowsAccessCache taskTraceTeamAccessCache

const taskTraceTeamSearchScript = `$Keyword=$env:TASKTRACE_TEAM_KEYWORD
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$result=New-Object 'System.Collections.Generic.List[object]'
function Add-Candidate([string]$Username,[string]$AccountName,[string]$DisplayName,[string]$Email) {
  if([string]::IsNullOrWhiteSpace($Username)-or[string]::IsNullOrWhiteSpace($AccountName)){return}
  $existing=$result | Where-Object {$_.account_name -ieq $AccountName -or $_.username -ieq $Username} | Select-Object -First 1
  if($null -ne $existing){
    if($DisplayName -and -not $existing.display_name){$existing.display_name=$DisplayName}
    if($Email -and -not $existing.email){$existing.email=$Email}
    return
  }
  [void]$result.Add([ordered]@{username=$Username;account_name=$AccountName;display_name=$DisplayName;email=$Email})
}
try {
  $resolved=([Security.Principal.NTAccount]::new($Keyword)).Translate([Security.Principal.SecurityIdentifier]).Translate([Security.Principal.NTAccount]).Value
  $separator=$resolved.LastIndexOf([char]92)
  $short=if($separator -ge 0){$resolved.Substring($separator+1)}else{$resolved}
  Add-Candidate $short $resolved '' ''
} catch {}
$domainFailure=''
try {
  foreach($account in @(Get-CimInstance -ClassName Win32_UserAccount -Filter 'LocalAccount = TRUE AND Disabled = FALSE' -ErrorAction Stop)) {
    if(([string]$account.Name).IndexOf($Keyword,[StringComparison]::OrdinalIgnoreCase)-ge 0 -or ([string]$account.FullName).IndexOf($Keyword,[StringComparison]::OrdinalIgnoreCase)-ge 0) {
      Add-Candidate ([string]$account.Name) (([string]$account.Domain)+'\'+([string]$account.Name)) ([string]$account.FullName) ''
    }
  }
} catch {}
if($env:TASKTRACE_TEAM_QUICK -eq '1'){
  ConvertTo-Json -InputObject ([object[]]@($result | Sort-Object account_name | Select-Object -First 25)) -Compress
  exit 0
}
try {
  $computer=Get-CimInstance -ClassName Win32_ComputerSystem -Property PartOfDomain -ErrorAction Stop
  if($computer.PartOfDomain) {
    Add-Type -AssemblyName System.DirectoryServices
    $ldap=$Keyword.Replace([string][char]92,([char]92+'5c')).Replace('*',([char]92+'2a')).Replace('(',([char]92+'28')).Replace(')',([char]92+'29')).Replace([string][char]0,([char]92+'00'))
    $searcher=New-Object DirectoryServices.DirectorySearcher
	$rootDse=[ADSI]'LDAP://RootDSE'
	$searcher.SearchRoot=[ADSI]('LDAP://'+[string]$rootDse.defaultNamingContext)
    $searcher.PageSize=25
    $searcher.SizeLimit=25
	$searcher.ClientTimeout=[TimeSpan]::FromSeconds(15)
	$searcher.ServerTimeLimit=[TimeSpan]::FromSeconds(15)
    $searcher.Filter="(&(objectCategory=person)(objectClass=user)(!(userAccountControl:1.2.840.113556.1.4.803:=2))(|(sAMAccountName=*$ldap*)(displayName=*$ldap*)(mail=*$ldap*)))"
    foreach($property in @('sAMAccountName','displayName','mail','objectSid')){[void]$searcher.PropertiesToLoad.Add($property)}
    $found=$searcher.FindAll()
    try {
      foreach($entry in $found) {
        try {
          $username=[string]$entry.Properties['samaccountname'][0]
          $display=if($entry.Properties['displayname'].Count){[string]$entry.Properties['displayname'][0]}else{''}
          $email=if($entry.Properties['mail'].Count){[string]$entry.Properties['mail'][0]}else{''}
          $sid=[Security.Principal.SecurityIdentifier]::new([byte[]]$entry.Properties['objectsid'][0],0)
          $account=$sid.Translate([Security.Principal.NTAccount]).Value
          Add-Candidate $username $account $display $email
        } catch {}
      }
    } finally {$found.Dispose();$searcher.Dispose()}
  }
} catch {$domainFailure=$_.Exception.Message}
if($result.Count -eq 0 -and $domainFailure){throw ('域账户查询失败：'+$domainFailure)}
ConvertTo-Json -InputObject ([object[]]@($result | Sort-Object account_name | Select-Object -First 25)) -Compress`

const taskTraceTeamSystemAccountPattern = `^(BUILTIN|NT AUTHORITY|CREATOR OWNER)\\`

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
  if($name -and $name -notmatch '` + taskTraceTeamSystemAccountPattern + `' -and $name -notin @('Everyone','Authenticated Users')){$values+=$name}
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
$separator=$canonical.LastIndexOf([char]92)
$username=if($separator -ge 0){$canonical.Substring($separator+1)}else{$canonical}
$share=Get-SmbShare -ErrorAction Stop | Where-Object {$_.Path -and ([IO.Path]::GetFullPath([string]$_.Path).TrimEnd('\') -ieq $rootPath)} | Select-Object -First 1
if($null -eq $share){throw 'teamData 尚未创建 Windows 文件共享'}
$existing=@(Get-SmbShareAccess -Name $share.Name -ErrorAction Stop | Where-Object {$_.AccountName -ieq $canonical -and $_.AccessControlType -eq 'Allow' -and $_.AccessRight -in @('Change','Full')})
$directory=[IO.DirectoryInfo]::new($rootPath)
$legacyAclApi=@($directory.PSObject.Methods.Name) -contains 'GetAccessControl'
$acl=if($legacyAclApi){$directory.GetAccessControl([Security.AccessControl.AccessControlSections]::Access)}else{[IO.FileSystemAclExtensions]::GetAccessControl($directory,[Security.AccessControl.AccessControlSections]::Access)}
$aclExisting=@($acl.Access | Where-Object {
  $entrySid=$null
  try {$entrySid=$_.IdentityReference.Translate([Security.Principal.SecurityIdentifier])} catch {}
  $entrySid -eq $sid -and $_.AccessControlType -eq 'Allow' -and (($_.FileSystemRights -band [Security.AccessControl.FileSystemRights]::Modify) -ne 0)
})
if($existing.Count -gt 0 -and $aclExisting.Count -gt 0){[ordered]@{account_name=$canonical;username=$username} | ConvertTo-Json -Compress;exit 0}
$shareAdded=$false
try {
  if($existing.Count -eq 0){Grant-SmbShareAccess -Name $share.Name -AccountName $canonical -AccessRight Change -Force -ErrorAction Stop | Out-Null;$shareAdded=$true}
  $inherit=[Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit'
  $rule=[Security.AccessControl.FileSystemAccessRule]::new($sid,[Security.AccessControl.FileSystemRights]::Modify,$inherit,[Security.AccessControl.PropagationFlags]::None,[Security.AccessControl.AccessControlType]::Allow)
  $acl.SetAccessRule($rule)
  if($legacyAclApi){$directory.SetAccessControl($acl)}else{[IO.FileSystemAclExtensions]::SetAccessControl($directory,$acl)}
} catch {
  if($shareAdded){Revoke-SmbShareAccess -Name $share.Name -AccountName $canonical -Force -ErrorAction SilentlyContinue}
  throw
}
[ordered]@{account_name=$canonical;username=$username} | ConvertTo-Json -Compress`

const taskTraceTeamRemoveAccessScript = `$Root=$env:TASKTRACE_TEAM_ROOT
$Member=$env:TASKTRACE_TEAM_MEMBER
$ErrorActionPreference='Stop'
$rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\')
$sid=([Security.Principal.NTAccount]::new($Member)).Translate([Security.Principal.SecurityIdentifier])
$canonical=$sid.Translate([Security.Principal.NTAccount]).Value
$share=Get-SmbShare -ErrorAction Stop | Where-Object {$_.Path -and ([IO.Path]::GetFullPath([string]$_.Path).TrimEnd('\') -ieq $rootPath)} | Select-Object -First 1
if($null -ne $share){
  $existing=@(Get-SmbShareAccess -Name $share.Name -ErrorAction Stop | Where-Object {$_.AccountName -ieq $canonical -and $_.AccessControlType -eq 'Allow'})
  if($existing.Count -gt 0){Revoke-SmbShareAccess -Name $share.Name -AccountName $canonical -Force -ErrorAction Stop}
}
$directory=[IO.DirectoryInfo]::new($rootPath)
$legacyAclApi=@($directory.PSObject.Methods.Name) -contains 'GetAccessControl'
$acl=if($legacyAclApi){$directory.GetAccessControl([Security.AccessControl.AccessControlSections]::Access)}else{[IO.FileSystemAclExtensions]::GetAccessControl($directory,[Security.AccessControl.AccessControlSections]::Access)}
$acl.PurgeAccessRules($sid)
if($legacyAclApi){$directory.SetAccessControl($acl)}else{[IO.FileSystemAclExtensions]::SetAccessControl($directory,$acl)}`

func taskTraceTeamPowerShellContext(parent context.Context, timeout time.Duration, script string, environment ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	args := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script}
	cmd := exec.CommandContext(ctx, "powershell.exe", args...)
	cmd.Env = append(os.Environ(), environment...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, errors.New("Windows 操作超时，请检查域网络连接后重试")
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, ctx.Err()
		}
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}

func taskTraceTeamPowerShell(script string, environment ...string) ([]byte, error) {
	return taskTraceTeamPowerShellContext(context.Background(), 30*time.Second, script, environment...)
}

func taskTraceTeamSearchWindowsMembers(ctx context.Context, query string, quick bool) ([]TaskTraceTeamMemberCandidate, error) {
	timeout := 25 * time.Second
	quickValue := "0"
	if quick {
		timeout = 8 * time.Second
		quickValue = "1"
	}
	output, err := taskTraceTeamPowerShellContext(ctx, timeout, taskTraceTeamSearchScript, "TASKTRACE_TEAM_KEYWORD="+query, "TASKTRACE_TEAM_QUICK="+quickValue)
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
	return taskTraceWindowsAccessCache.read(root, func() ([]string, error) {
		return taskTraceTeamReadWindowsAccess(root)
	})
}

func taskTraceTeamReadWindowsAccess(root string) ([]string, error) {
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
	return taskTraceTeamGrantWindowsAccessWithElevation(root, member, false)
}

func taskTraceTeamAccessNeedsElevation(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"access is denied", "access denied", "拒绝访问", "windows system error 5", "system error 5", "unauthorizedaccessexception", "0x80070005", "sesecurityprivilege", "required privilege is not held", "所需的特权"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func IsTaskTraceTeamAdminRequired(err error) bool {
	return errors.Is(err, errTaskTraceTeamAdminRequired)
}

func taskTraceTeamGrantWindowsAccessWithElevation(root, member string, elevate bool) (string, error) {
	if elevate {
		return taskTraceTeamGrantWindowsAccessElevated(root, member)
	}
	output, err := taskTraceTeamPowerShell(taskTraceTeamGrantAccessScript, "TASKTRACE_TEAM_ROOT="+root, "TASKTRACE_TEAM_MEMBER="+member)
	if err != nil {
		if taskTraceTeamAccessNeedsElevation(err) {
			return "", fmt.Errorf("%w：Windows 拒绝了共享权限修改（系统错误 5）", errTaskTraceTeamAdminRequired)
		}
		return "", err
	}
	taskTraceWindowsAccessCache.invalidate(root)
	var value struct {
		AccountName string `json:"account_name"`
	}
	if err := json.Unmarshal(output, &value); err != nil || value.AccountName == "" {
		return "", fmt.Errorf("Windows did not return the granted account")
	}
	return value.AccountName, nil
}

func taskTraceTeamGrantElevatedScript() string {
	return `$ErrorActionPreference='Stop'
$result=[ordered]@{ok=$false;value='';error=''}
try {
  $request=Get-Content -LiteralPath $args[0] -Raw -Encoding UTF8 | ConvertFrom-Json
  $env:TASKTRACE_TEAM_ROOT=[string]$request.root
  $env:TASKTRACE_TEAM_MEMBER=[string]$request.member
  $value=& {
` + taskTraceTeamGrantAccessScript + `
  }
  $result.ok=$true
  $result.value=[string]$value
} catch {
  $result.error=($_ | Out-String).Trim()
}
$result | ConvertTo-Json -Compress | Set-Content -LiteralPath $args[1] -Encoding UTF8`
}

func taskTraceTeamRemoveElevatedScript() string {
	return `$ErrorActionPreference='Stop'
$result=[ordered]@{ok=$false;value='';error=''}
try {
  $request=Get-Content -LiteralPath $args[0] -Raw -Encoding UTF8 | ConvertFrom-Json
  $env:TASKTRACE_TEAM_ROOT=[string]$request.root
  $env:TASKTRACE_TEAM_MEMBER=[string]$request.member
  & {
` + taskTraceTeamRemoveAccessScript + `
  }
  $result.ok=$true
} catch {
  $result.error=($_ | Out-String).Trim()
}
$result | ConvertTo-Json -Compress | Set-Content -LiteralPath $args[1] -Encoding UTF8`
}

func taskTraceTeamRunWindowsAccessElevated(root, member, scriptName, wrapper string) (string, error) {
	tempDir, err := os.MkdirTemp("", "tasktrace-team-access-")
	if err != nil {
		return "", fmt.Errorf("创建管理员授权请求失败：%w", err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "request.json")
	outputPath := filepath.Join(tempDir, "result.json")
	scriptPath := filepath.Join(tempDir, scriptName)
	payload, _ := json.Marshal(map[string]string{"root": root, "member": member})
	if err := os.WriteFile(inputPath, payload, 0600); err != nil {
		return "", fmt.Errorf("写入管理员授权请求失败：%w", err)
	}
	if err := os.WriteFile(scriptPath, append([]byte{0xef, 0xbb, 0xbf}, []byte(wrapper)...), 0600); err != nil {
		return "", fmt.Errorf("写入管理员授权脚本失败：%w", err)
	}
	verb, file := windows.StringToUTF16Ptr("runas"), windows.StringToUTF16Ptr("powershell.exe")
	arguments := strings.Join([]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", windows.EscapeArg(scriptPath), windows.EscapeArg(inputPath), windows.EscapeArg(outputPath)}, " ")
	if err := windows.ShellExecute(0, verb, file, windows.StringToUTF16Ptr(arguments), nil, 0); err != nil {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return "", errors.New("已取消 Windows 管理员授权，未修改 teamData 权限")
		}
		return "", fmt.Errorf("启动 Windows 管理员授权失败：%w", err)
	}
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		contents, readErr := os.ReadFile(outputPath)
		if readErr == nil {
			var result struct {
				OK    bool   `json:"ok"`
				Value string `json:"value"`
				Error string `json:"error"`
			}
			contents = []byte(strings.TrimPrefix(string(contents), "\ufeff"))
			if err := json.Unmarshal(contents, &result); err != nil {
				return "", fmt.Errorf("读取管理员授权结果失败：%w", err)
			}
			if !result.OK {
				if result.Error == "" {
					result.Error = "Windows 未返回权限修改结果"
				}
				return "", errors.New(result.Error)
			}
			return result.Value, nil
		}
		if !errors.Is(readErr, os.ErrNotExist) {
			return "", fmt.Errorf("读取管理员授权结果失败：%w", readErr)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", errors.New("Windows 管理员授权超时，请重试")
}

func taskTraceTeamGrantWindowsAccessElevated(root, member string) (string, error) {
	result, err := taskTraceTeamRunWindowsAccessElevated(root, member, "grant-team-access.ps1", taskTraceTeamGrantElevatedScript())
	if err != nil {
		return "", err
	}
	taskTraceWindowsAccessCache.invalidate(root)
	var value struct {
		AccountName string `json:"account_name"`
	}
	if err := json.Unmarshal([]byte(result), &value); err != nil || value.AccountName == "" {
		return "", errors.New("Windows 未返回已授权的账户")
	}
	return value.AccountName, nil
}

func taskTraceTeamRemoveWindowsAccessWithElevation(root, member string, elevate bool) error {
	if elevate {
		_, err := taskTraceTeamRunWindowsAccessElevated(root, member, "remove-team-access.ps1", taskTraceTeamRemoveElevatedScript())
		if err == nil {
			taskTraceWindowsAccessCache.invalidate(root)
		}
		return err
	}
	_, err := taskTraceTeamPowerShell(taskTraceTeamRemoveAccessScript, "TASKTRACE_TEAM_ROOT="+root, "TASKTRACE_TEAM_MEMBER="+member)
	if taskTraceTeamAccessNeedsElevation(err) {
		return fmt.Errorf("%w：Windows 拒绝了共享权限修改（系统错误 5）", errTaskTraceTeamAdminRequired)
	}
	if err == nil {
		taskTraceWindowsAccessCache.invalidate(root)
	}
	return err
}

func taskTraceTeamRemoveWindowsAccess(root, member string) error {
	return taskTraceTeamRemoveWindowsAccessWithElevation(root, member, false)
}
