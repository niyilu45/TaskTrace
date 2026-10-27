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
	osuser "os/user"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

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

const taskTraceTeamFindShareScript = `function Find-TaskTraceShare([string]$Directory) {
  Get-SmbShare -ErrorAction Stop | Where-Object {
    if(-not $_.Path -or $_.Special -eq $true -or $_.Name -in @('ADMIN$','IPC$') -or $_.Name -match '^[a-zA-Z]\$$'){return $false}
    $sharedDirectory=[IO.Path]::GetFullPath([string]$_.Path).TrimEnd('\')
    $Directory -ieq $sharedDirectory -or $Directory.StartsWith($sharedDirectory+'\',[StringComparison]::OrdinalIgnoreCase)
  } | Sort-Object @{Expression={([IO.Path]::GetFullPath([string]$_.Path).TrimEnd('\')).Length};Descending=$true} | Select-Object -First 1
}
function Get-TaskTraceShareNetworkPath($Share,[string]$Directory) {
  if($null -eq $Share){return ''}
  $sharedDirectory=[IO.Path]::GetFullPath([string]$Share.Path).TrimEnd('\')
  $relative=$Directory.Substring($sharedDirectory.Length).TrimStart('\')
  $networkPath='\\'+[string]$env:COMPUTERNAME+'\'+[string]$Share.Name
  if($relative){$networkPath+='\'+$relative}
  return $networkPath
}
`

const taskTraceTeamListAccessScript = taskTraceTeamFindShareScript + `$Root=$env:TASKTRACE_TEAM_ROOT
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$resolvedRoot=[IO.Path]::GetFullPath($Root).TrimEnd('\')
$fileAccess=@{}
$fileRights=@{}
$fileDeniedRights=@{}
$shareAccess=@{}
$shareDenied=@{}
function Test-SystemPrincipal($Identity) {
  try {
    $sid=$Identity.Translate([Security.Principal.SecurityIdentifier]).Value
    if($sid -in @('S-1-5-18','S-1-5-19','S-1-5-20') -or $sid.StartsWith('S-1-5-32-')){return $true}
  } catch {}
  $name=[string]$Identity.Value
  return $name -match '` + taskTraceTeamSystemAccountPattern + `'
}
$acl=Get-Acl -LiteralPath $resolvedRoot -ErrorAction Stop
foreach($entry in $acl.Access){
  if(Test-SystemPrincipal $entry.IdentityReference){continue}
  $name=$entry.IdentityReference.Value
  if(-not $name -or $name -in @('Everyone','Authenticated Users')){continue}
  if($entry.AccessControlType -eq 'Deny'){
    $fileDeniedRights[$name]=([int64]$fileDeniedRights[$name] -bor [int64]$entry.FileSystemRights)
  } elseif($entry.AccessControlType -eq 'Allow'){
    $fileRights[$name]=([int64]$fileRights[$name] -bor [int64]$entry.FileSystemRights)
  }
}
foreach($name in @($fileRights.Keys)){
  $rights=[int64]$fileRights[$name]
  $denied=[int64]$fileDeniedRights[$name]
  $readMask=[int64][Security.AccessControl.FileSystemRights]::Read
  $writeMask=[int64][Security.AccessControl.FileSystemRights]::Write
  $read=(($rights -band $readMask) -eq $readMask) -and (($denied -band $readMask) -eq 0)
  $write=(($rights -band $writeMask) -eq $writeMask) -and (($denied -band $writeMask) -eq 0)
  if($read){$fileAccess[$name]=if($write){'write'}else{'read'}}
}
$share=Find-TaskTraceShare $resolvedRoot
if($null -ne $share){
  foreach($entry in @(Get-SmbShareAccess -Name $share.Name -ErrorAction Stop)){
    $identity=[Security.Principal.NTAccount]::new([string]$entry.AccountName)
    if((Test-SystemPrincipal $identity) -or $entry.AccountName -in @('Everyone','Authenticated Users')){continue}
    $name=[string]$entry.AccountName
    if($entry.AccessControlType -eq 'Deny'){$shareDenied[$name]=$true;continue}
    if($entry.AccessControlType -ne 'Allow'){continue}
    $level=if($entry.AccessRight -in @('Change','Full')){'write'}else{'read'}
    if(-not $shareAccess.ContainsKey($name) -or $level -eq 'write'){$shareAccess[$name]=$level}
  }
}
$values=@()
if($null -ne $share){
  foreach($name in @($shareAccess.Keys | Sort-Object)){
    $access=$shareAccess[$name]
    if($shareDenied.ContainsKey($name) -or -not $fileAccess.ContainsKey($name)){continue}
    if($fileAccess[$name] -eq 'read'){$access='read'}
    $values+=[ordered]@{account_name=$name;access=$access}
  }
}
$networkPath=Get-TaskTraceShareNetworkPath $share $resolvedRoot
[ordered]@{shared=($null -ne $share);path=$networkPath;members=([object[]]@($values))} | ConvertTo-Json -Depth 4 -Compress`

//nolint:gosmopolitan // Windows sharing failures must remain actionable in the local Chinese UI.
const taskTraceTeamEnsureShareScript = taskTraceTeamFindShareScript + `$CreateShare=$env:TASKTRACE_TEAM_CREATE_SHARE -eq '1'
$ShareOwner=[string]$env:TASKTRACE_TEAM_SHARE_OWNER
$shareWasCreated=$false
$share=Find-TaskTraceShare $rootPath
if($null -eq $share){
  if(-not $CreateShare){throw 'TASKTRACE_TEAM_SHARE_REQUIRED: teamData 尚未创建 Windows 文件共享'}
  if($rootPath.StartsWith('\\')){throw 'teamData 是远程目录，请在共享文件夹所属电脑上启用共享'}
  if([string]::IsNullOrWhiteSpace($ShareOwner)){throw '无法识别 teamData 所属 Windows 用户，未创建共享'}
  $shareOwnerAccount=([Security.Principal.NTAccount]::new($ShareOwner)).Translate([Security.Principal.SecurityIdentifier]).Translate([Security.Principal.NTAccount]).Value
  $shareName='teamData'
  $existingName=Get-SmbShare -Name $shareName -ErrorAction SilentlyContinue
  if($null -ne $existingName){
    $hash=[Security.Cryptography.SHA256]::Create()
    try {$suffix=([BitConverter]::ToString($hash.ComputeHash([Text.Encoding]::UTF8.GetBytes($rootPath.ToLowerInvariant())))).Replace('-','').Substring(0,8).ToLowerInvariant()} finally {$hash.Dispose()}
    $shareName='TaskTrace-teamData-'+$suffix
    if($null -ne (Get-SmbShare -Name $shareName -ErrorAction SilentlyContinue)){throw 'teamData 共享名称已被其他目录占用，未修改现有共享'}
  }
  $share=New-SmbShare -Name $shareName -Path $rootPath -FullAccess $shareOwnerAccount -ErrorAction Stop
  $shareWasCreated=$true
}`

const taskTraceTeamGrantAccessScript = `$Root=$env:TASKTRACE_TEAM_ROOT
$Member=$env:TASKTRACE_TEAM_MEMBER
$Access=([string]$env:TASKTRACE_TEAM_ACCESS).ToLowerInvariant()
if($Access -ne 'read'){$Access='write'}
$ErrorActionPreference='Stop'
[Console]::OutputEncoding=[Text.Encoding]::UTF8
$rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\')
$sid=([Security.Principal.NTAccount]::new($Member)).Translate([Security.Principal.SecurityIdentifier])
$canonical=$sid.Translate([Security.Principal.NTAccount]).Value
$separator=$canonical.LastIndexOf([char]92)
$username=if($separator -ge 0){$canonical.Substring($separator+1)}else{$canonical}
` + taskTraceTeamEnsureShareScript + `
$previousRight=''
$sharePermissionChanged=$false
try {
$directory=[IO.DirectoryInfo]::new($rootPath)
$legacyAclApi=@($directory.PSObject.Methods.Name) -contains 'GetAccessControl'
$acl=if($legacyAclApi){$directory.GetAccessControl([Security.AccessControl.AccessControlSections]::Access)}else{[IO.FileSystemAclExtensions]::GetAccessControl($directory,[Security.AccessControl.AccessControlSections]::Access)}
$existing=@(Get-SmbShareAccess -Name $share.Name -ErrorAction Stop | Where-Object {$_.AccountName -ieq $canonical -and $_.AccessControlType -eq 'Allow'})
$previousRight=if($existing.AccessRight -contains 'Full'){'Full'}elseif($existing.AccessRight -contains 'Change'){'Change'}elseif($existing.Count -gt 0){'Read'}else{''}
  $sharePermissionChanged=$true
  if($existing.Count -gt 0){Revoke-SmbShareAccess -Name $share.Name -AccountName $canonical -Force -ErrorAction Stop | Out-Null}
  $shareRight=if($Access -eq 'read'){'Read'}else{'Change'}
  Grant-SmbShareAccess -Name $share.Name -AccountName $canonical -AccessRight $shareRight -Force -ErrorAction Stop | Out-Null
  $inherit=[Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit'
  $rights=if($Access -eq 'read'){[Security.AccessControl.FileSystemRights]::ReadAndExecute}else{[Security.AccessControl.FileSystemRights]::Modify}
  [void]$acl.PurgeAccessRules($sid)
  $rule=[Security.AccessControl.FileSystemAccessRule]::new($sid,$rights,$inherit,[Security.AccessControl.PropagationFlags]::None,[Security.AccessControl.AccessControlType]::Allow)
  [void]$acl.AddAccessRule($rule)
  if($legacyAclApi){[void]$directory.SetAccessControl($acl)}else{[void][IO.FileSystemAclExtensions]::SetAccessControl($directory,$acl)}
} catch {
  if($sharePermissionChanged){
    Revoke-SmbShareAccess -Name $share.Name -AccountName $canonical -Force -ErrorAction SilentlyContinue | Out-Null
    if($previousRight){Grant-SmbShareAccess -Name $share.Name -AccountName $canonical -AccessRight $previousRight -Force -ErrorAction SilentlyContinue | Out-Null}
  }
  if($shareWasCreated){Remove-SmbShare -Name $share.Name -Force -ErrorAction SilentlyContinue | Out-Null}
  throw
}
$grantResult=[ordered]@{account_name=$canonical;username=$username;access=$Access} | ConvertTo-Json -Compress
Write-Output ('` + taskTraceTeamGrantResultPrefix + `'+$grantResult)`

const taskTraceTeamRemoveAccessScript = taskTraceTeamFindShareScript + `$Root=$env:TASKTRACE_TEAM_ROOT
$Member=$env:TASKTRACE_TEAM_MEMBER
$ErrorActionPreference='Stop'
$rootPath=[IO.Path]::GetFullPath($Root).TrimEnd('\')
$sid=([Security.Principal.NTAccount]::new($Member)).Translate([Security.Principal.SecurityIdentifier])
$canonical=$sid.Translate([Security.Principal.NTAccount]).Value
$share=Find-TaskTraceShare $rootPath
if($null -ne $share){
  $existing=@(Get-SmbShareAccess -Name $share.Name -ErrorAction Stop | Where-Object {$_.AccountName -ieq $canonical -and $_.AccessControlType -eq 'Allow'})
  if($existing.Count -gt 0){Revoke-SmbShareAccess -Name $share.Name -AccountName $canonical -Force -ErrorAction Stop | Out-Null}
}
$directory=[IO.DirectoryInfo]::new($rootPath)
$legacyAclApi=@($directory.PSObject.Methods.Name) -contains 'GetAccessControl'
$acl=if($legacyAclApi){$directory.GetAccessControl([Security.AccessControl.AccessControlSections]::Access)}else{[IO.FileSystemAclExtensions]::GetAccessControl($directory,[Security.AccessControl.AccessControlSections]::Access)}
[void]$acl.PurgeAccessRules($sid)
if($legacyAclApi){[void]$directory.SetAccessControl($acl)}else{[void][IO.FileSystemAclExtensions]::SetAccessControl($directory,$acl)}`

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

func taskTraceTeamListWindowsAccess(root string) ([]TaskTraceTeamRepositoryMember, error) {
	return taskTraceWindowsAccessCache.read(root, func() ([]TaskTraceTeamRepositoryMember, error) {
		return taskTraceTeamReadWindowsAccess(root)
	})
}

func taskTraceTeamReadWindowsAccess(root string) ([]TaskTraceTeamRepositoryMember, error) {
	output, err := taskTraceTeamPowerShell(taskTraceTeamListAccessScript, "TASKTRACE_TEAM_ROOT="+root)
	if err != nil {
		return nil, err
	}
	var snapshot struct {
		Shared  bool                            `json:"shared"`
		Path    string                          `json:"path"`
		Members []TaskTraceTeamRepositoryMember `json:"members"`
	}
	if err := json.Unmarshal(output, &snapshot); err != nil {
		return nil, fmt.Errorf("parse teamData access list: %w", err)
	}
	taskTraceWindowsAccessCache.recordShared(root, snapshot.Shared)
	taskTraceWindowsAccessCache.recordSharePath(root, snapshot.Path)
	if snapshot.Members == nil {
		snapshot.Members = []TaskTraceTeamRepositoryMember{}
	}
	return snapshot.Members, nil
}

func taskTraceTeamGrantWindowsAccess(root, member string) (string, error) {
	return taskTraceTeamSetWindowsAccessWithOptions(root, member, TaskTraceTeamAccessWrite, TaskTraceTeamAccessOptions{})
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

func taskTraceTeamSetWindowsAccessWithOptions(root, member, access string, options TaskTraceTeamAccessOptions) (string, error) {
	access = strings.ToLower(strings.TrimSpace(access))
	if access != TaskTraceTeamAccessRead {
		access = TaskTraceTeamAccessWrite
	}
	if options.Elevate {
		return taskTraceTeamGrantWindowsAccessElevatedWithOptions(root, member, access, options)
	}
	output, err := taskTraceTeamPowerShell(taskTraceTeamGrantAccessScript, taskTraceTeamAccessEnvironment(root, member, access, options)...)
	if err != nil {
		return "", taskTraceTeamClassifyWindowsAccessError(err)
	}
	taskTraceWindowsAccessCache.invalidate(root)
	return taskTraceTeamDecodeGrantedAccount(output)
}

// A dedicated result frame separates the acknowledgement from PowerShell
// warnings, formatting output and CIM objects returned by permission cmdlets.
const taskTraceTeamGrantResultPrefix = "TASKTRACE_TEAM_ACCESS_RESULT:"

func taskTraceTeamDecodeGrantedAccount(output []byte) (string, error) {
	payload := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(output)), "\ufeff"))
	if position := strings.LastIndex(payload, taskTraceTeamGrantResultPrefix); position >= 0 {
		payload = strings.TrimSpace(strings.SplitN(payload[position+len(taskTraceTeamGrantResultPrefix):], "\n", 2)[0])
	}
	var value struct {
		AccountName string `json:"account_name"`
	}
	if err := json.Unmarshal([]byte(payload), &value); err != nil {
		return "", fmt.Errorf("decode Windows permission acknowledgement: %w", err)
	}
	account := strings.TrimSpace(value.AccountName)
	if account == "" {
		return "", errors.New("windows permission acknowledgement did not contain an authorized account")
	}
	return account, nil
}

func taskTraceTeamAccessEnvironment(root, member, access string, options TaskTraceTeamAccessOptions) []string {
	createShare := "0"
	if options.CreateShare {
		createShare = "1"
	}
	owner := ""
	if current, err := osuser.Current(); err == nil {
		owner = current.Username
	}
	return []string{"TASKTRACE_TEAM_ROOT=" + root, "TASKTRACE_TEAM_MEMBER=" + member, "TASKTRACE_TEAM_ACCESS=" + access, "TASKTRACE_TEAM_CREATE_SHARE=" + createShare, "TASKTRACE_TEAM_SHARE_OWNER=" + owner}
}

func taskTraceTeamClassifyWindowsAccessError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "TASKTRACE_TEAM_SHARE_REQUIRED") {
		//nolint:gosmopolitan // The confirmation dialog displays this localized explanation to the user.
		return fmt.Errorf("%w：teamData 尚未创建 Windows 文件共享", errTaskTraceTeamShareRequired)
	}
	if taskTraceTeamAccessNeedsElevation(err) {
		//nolint:gosmopolitan // The portable Windows UI displays this actionable authorization message in Chinese.
		return fmt.Errorf("%w：Windows 拒绝了共享权限修改（系统错误 5）", errTaskTraceTeamAdminRequired)
	}
	return err
}

func taskTraceTeamGrantElevatedScript() string {
	return `$ErrorActionPreference='Stop'
$result=[ordered]@{ok=$false;value='';error=''}
try {
  $request=Get-Content -LiteralPath $args[0] -Raw -Encoding UTF8 | ConvertFrom-Json
  $env:TASKTRACE_TEAM_ROOT=[string]$request.root
  $env:TASKTRACE_TEAM_MEMBER=[string]$request.member
  $env:TASKTRACE_TEAM_ACCESS=[string]$request.access
  $env:TASKTRACE_TEAM_CREATE_SHARE=[string]$request.create_share
  $env:TASKTRACE_TEAM_SHARE_OWNER=[string]$request.share_owner
  $value=& {
` + taskTraceTeamGrantAccessScript + `
  }
  $result.ok=$true
  $result.value=[string]$value
} catch {
  $result.error=($_ | Out-String).Trim()
}
$resultPath=[string]$args[1]
$temporaryResult=$resultPath+'.tmp'
$result | ConvertTo-Json -Compress | Set-Content -LiteralPath $temporaryResult -Encoding UTF8
[IO.File]::Move($temporaryResult,$resultPath)`
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
$resultPath=[string]$args[1]
$temporaryResult=$resultPath+'.tmp'
$result | ConvertTo-Json -Compress | Set-Content -LiteralPath $temporaryResult -Encoding UTF8
[IO.File]::Move($temporaryResult,$resultPath)`
}

func taskTraceTeamRunWindowsAccessElevated(root, member, access, scriptName, wrapper string) (string, error) {
	return taskTraceTeamRunWindowsAccessElevatedWithOptions(root, member, access, scriptName, wrapper, TaskTraceTeamAccessOptions{})
}

func taskTraceTeamRunWindowsAccessElevatedWithOptions(root, member, access, scriptName, wrapper string, options TaskTraceTeamAccessOptions) (string, error) {
	tempDir, err := os.MkdirTemp("", "tasktrace-team-access-")
	if err != nil {
		return "", fmt.Errorf("创建管理员授权请求失败：%w", err)
	}
	defer os.RemoveAll(tempDir)

	inputPath := filepath.Join(tempDir, "request.json")
	outputPath := filepath.Join(tempDir, "result.json")
	scriptPath := filepath.Join(tempDir, scriptName)
	environment := taskTraceTeamAccessEnvironment(root, member, access, options)
	payloadValues := map[string]string{"root": root, "member": member, "access": access}
	for _, item := range environment {
		if strings.HasPrefix(item, "TASKTRACE_TEAM_CREATE_SHARE=") {
			payloadValues["create_share"] = strings.TrimPrefix(item, "TASKTRACE_TEAM_CREATE_SHARE=")
		}
		if strings.HasPrefix(item, "TASKTRACE_TEAM_SHARE_OWNER=") {
			payloadValues["share_owner"] = strings.TrimPrefix(item, "TASKTRACE_TEAM_SHARE_OWNER=")
		}
	}
	payload, err := json.Marshal(payloadValues)
	if err != nil {
		return "", fmt.Errorf("encode Windows authorization request: %w", err)
	}
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
				return "", taskTraceTeamClassifyWindowsAccessError(errors.New(result.Error))
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

func taskTraceTeamGrantWindowsAccessElevatedWithOptions(root, member, access string, options TaskTraceTeamAccessOptions) (string, error) {
	result, err := taskTraceTeamRunWindowsAccessElevatedWithOptions(root, member, access, "grant-team-access.ps1", taskTraceTeamGrantElevatedScript(), options)
	if err != nil {
		return "", err
	}
	taskTraceWindowsAccessCache.invalidate(root)
	return taskTraceTeamDecodeGrantedAccount([]byte(result))
}

func taskTraceTeamRemoveWindowsAccessWithElevation(root, member string, elevate bool) error {
	if elevate {
		_, err := taskTraceTeamRunWindowsAccessElevated(root, member, "", "remove-team-access.ps1", taskTraceTeamRemoveElevatedScript())
		if err == nil {
			taskTraceWindowsAccessCache.invalidate(root)
		}
		return err
	}
	_, err := taskTraceTeamPowerShell(taskTraceTeamRemoveAccessScript, "TASKTRACE_TEAM_ROOT="+root, "TASKTRACE_TEAM_MEMBER="+member)
	if taskTraceTeamAccessNeedsElevation(err) {
		//nolint:gosmopolitan // The portable Windows UI displays this actionable authorization message in Chinese.
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
