[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [switch]$SkipFrontend,
    [switch]$Interactive,
    [string]$InstallDirectory = '',
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$defaultInstallDirectory = Join-Path $PSScriptRoot 'TaskTrace-local'
$outputDirectory = $defaultInstallDirectory
$buildStagingRoot = ''
$logFile = Join-Path $PSScriptRoot 'install.log'
$issues = New-Object 'System.Collections.Generic.List[string]'
$hints = New-Object 'System.Collections.Generic.List[string]'
$stage = '检查构建环境'

function Write-InstallLine([string]$Text, [ConsoleColor]$Color = [ConsoleColor]::Gray) {
    Write-Host $Text -ForegroundColor $Color
    try { [IO.File]::AppendAllText($logFile, $Text + [Environment]::NewLine, [Text.UTF8Encoding]::new($true)) } catch { }
}

function Write-InstallLog([string]$Text) {
	try { [IO.File]::AppendAllText($logFile, $Text + [Environment]::NewLine, [Text.UTF8Encoding]::new($true)) } catch { }
}

function Show-InstallResult([string]$Title, [string]$Message, [bool]$IsError = $false) {
    if (!$Interactive) { return }
    try {
        $icon = if ($IsError) { 16 } else { 64 }
        $shell = New-Object -ComObject WScript.Shell
        [void]$shell.Popup($Message, 0, $Title, $icon)
        [void][Runtime.InteropServices.Marshal]::ReleaseComObject($shell)
    } catch {
        Write-InstallLine ('无法显示结果窗口：' + $_.Exception.Message) Yellow
    }
}

function Select-InstallDirectory([string]$RequestedDirectory) {
    if (![string]::IsNullOrWhiteSpace($RequestedDirectory)) {
        return [IO.Path]::GetFullPath([Environment]::ExpandEnvironmentVariables($RequestedDirectory.Trim().Trim('"')))
    }
    if (!$Interactive) { return [IO.Path]::GetFullPath($defaultInstallDirectory) }
    Add-Type -AssemblyName System.Windows.Forms
    Add-Type -AssemblyName System.Drawing
    $form = New-Object System.Windows.Forms.Form
    $form.Text = '选择 TaskTrace 安装目录'
    $form.StartPosition = 'CenterScreen'
    $form.FormBorderStyle = 'FixedDialog'
    $form.MaximizeBox = $false
    $form.MinimizeBox = $false
    $form.ClientSize = New-Object System.Drawing.Size(680, 178)

    $label = New-Object System.Windows.Forms.Label
    $label.Text = '安装路径（可以直接粘贴）：'
    $label.AutoSize = $true
    $label.Location = New-Object System.Drawing.Point(16, 18)
    $form.Controls.Add($label)

    $pathBox = New-Object System.Windows.Forms.TextBox
    $pathBox.Text = [IO.Path]::GetFullPath($defaultInstallDirectory)
    $pathBox.Location = New-Object System.Drawing.Point(16, 43)
    $pathBox.Size = New-Object System.Drawing.Size(555, 27)
    $pathBox.SelectAll()
    $form.Controls.Add($pathBox)

    $browseButton = New-Object System.Windows.Forms.Button
    $browseButton.Text = '浏览...'
    $browseButton.Location = New-Object System.Drawing.Point(581, 41)
    $browseButton.Size = New-Object System.Drawing.Size(82, 29)
    $browseButton.Add_Click({
        $browser = New-Object System.Windows.Forms.FolderBrowserDialog
        $browser.Description = '选择 TaskTrace 安装目录'
        $browser.ShowNewFolderButton = $true
        $candidate = [Environment]::ExpandEnvironmentVariables($pathBox.Text.Trim().Trim('"'))
        if (Test-Path -LiteralPath $candidate -PathType Container) { $browser.SelectedPath = $candidate }
        try {
            if ($browser.ShowDialog($form) -eq [System.Windows.Forms.DialogResult]::OK) { $pathBox.Text = $browser.SelectedPath }
        } finally { $browser.Dispose() }
    })
    $form.Controls.Add($browseButton)

    $hint = New-Object System.Windows.Forms.Label
    $hint.Text = '覆盖已有程序时，只替换程序文件；配置、data、teamData、.cache 和 backups 均会保留。'
    $hint.AutoSize = $true
    $hint.Location = New-Object System.Drawing.Point(16, 84)
    $form.Controls.Add($hint)

    $okButton = New-Object System.Windows.Forms.Button
    $okButton.Text = '确定'
    $okButton.DialogResult = [System.Windows.Forms.DialogResult]::OK
    $okButton.Location = New-Object System.Drawing.Point(493, 128)
    $okButton.Size = New-Object System.Drawing.Size(80, 30)
    $form.Controls.Add($okButton)

    $cancelButton = New-Object System.Windows.Forms.Button
    $cancelButton.Text = '取消'
    $cancelButton.DialogResult = [System.Windows.Forms.DialogResult]::Cancel
    $cancelButton.Location = New-Object System.Drawing.Point(583, 128)
    $cancelButton.Size = New-Object System.Drawing.Size(80, 30)
    $form.Controls.Add($cancelButton)
    $form.AcceptButton = $okButton
    $form.CancelButton = $cancelButton
    try {
        if ($form.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK) { return '' }
        $selected = [Environment]::ExpandEnvironmentVariables($pathBox.Text.Trim().Trim('"'))
        if ([string]::IsNullOrWhiteSpace($selected)) { throw '安装路径不能为空。' }
        return [IO.Path]::GetFullPath($selected)
    } finally { $form.Dispose() }
}

function Confirm-ExistingInstallation([string]$Directory) {
    $launcher = Join-Path $Directory 'TaskTrace.exe'
    if (!(Test-Path -LiteralPath $launcher -PathType Leaf)) { return $true }
    $version = '未知版本'
    $versionFile = Join-Path $Directory 'VERSION.txt'
    if (Test-Path -LiteralPath $versionFile -PathType Leaf) {
        try {
            $storedVersion = [IO.File]::ReadAllText($versionFile).Trim()
            if (![string]::IsNullOrWhiteSpace($storedVersion)) { $version = $storedVersion }
        } catch { }
    }
    $message = "检测到该目录中已有 TaskTrace。`r`n`r`n现有版本：$version`r`n目录：$Directory`r`n`r`n继续后只覆盖程序文件，原有配置、data、teamData、.cache、backups 和其他用户文件均会保留。是否继续？"
    if (!$Interactive) {
        Write-InstallLine ('检测到已有 TaskTrace（' + $version + '），将只覆盖程序文件并保留配置和数据。') Yellow
        return $true
    }
    Add-Type -AssemblyName System.Windows.Forms
    $result = [System.Windows.Forms.MessageBox]::Show($message, 'TaskTrace：确认覆盖安装', [System.Windows.Forms.MessageBoxButtons]::YesNo, [System.Windows.Forms.MessageBoxIcon]::Information, [System.Windows.Forms.MessageBoxDefaultButton]::Button2)
    return $result -eq [System.Windows.Forms.DialogResult]::Yes
}

function Initialize-InstallProcessInspection {
    if ('TaskTraceInstallNative' -as [type]) { return }
    # Limited process queries work across x86/x64 and normally also for elevated
    # processes; MainModule requires more access and used to silently miss them.
    Add-Type -TypeDefinition @'
using System;
using System.IO;
using System.Text;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
public static class TaskTraceInstallNative {
    [DllImport("kernel32.dll", SetLastError=true)]
    static extern IntPtr OpenProcess(uint access, bool inherit, int id);
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
    static extern bool QueryFullProcessImageName(IntPtr process, uint flags, StringBuilder name, ref uint size);
    [DllImport("kernel32.dll")]
    static extern bool CloseHandle(IntPtr handle);
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
    static extern SafeFileHandle CreateFile(string path, uint access, uint share, IntPtr security, uint creation, uint flags, IntPtr template);
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
    static extern uint GetFinalPathNameByHandle(SafeFileHandle handle, StringBuilder path, uint size, uint flags);
    public static string ProcessPath(int id) {
        IntPtr handle = OpenProcess(0x1000, false, id);
        if(handle == IntPtr.Zero) return null;
        try {
            var path = new StringBuilder(32768);
            uint size = (uint)path.Capacity;
            return QueryFullProcessImageName(handle, 0, path, ref size) ? path.ToString() : null;
        } finally { CloseHandle(handle); }
    }
    public static string DirectoryPath(string directory) {
        string full = Path.GetFullPath(directory);
        // Resolve junctions, short names and mapped paths before comparison.
        using(var handle = CreateFile(full, 0, 7, IntPtr.Zero, 3, 0x02000000, IntPtr.Zero)) {
            if(handle.IsInvalid) return full.TrimEnd('\\');
            var path = new StringBuilder(32768);
            uint length = GetFinalPathNameByHandle(handle, path, (uint)path.Capacity, 0);
            if(length == 0 || length >= path.Capacity) return full.TrimEnd('\\');
            string resolved = path.ToString();
            if(resolved.StartsWith(@"\\?\UNC\", StringComparison.OrdinalIgnoreCase)) resolved = @"\\" + resolved.Substring(8);
            else if(resolved.StartsWith(@"\\?\", StringComparison.OrdinalIgnoreCase)) resolved = resolved.Substring(4);
            return resolved.TrimEnd('\\');
        }
    }
}
'@
}

function Get-InstallProcessImagePath([Diagnostics.Process]$Process) {
    Initialize-InstallProcessInspection
    return [TaskTraceInstallNative]::ProcessPath($Process.Id)
}

function Get-RunningInstallationProcesses([string]$Directory) {
    Initialize-InstallProcessInspection
    $targetDirectory = [TaskTraceInstallNative]::DirectoryPath($Directory)
    $found = New-Object 'System.Collections.Generic.List[string]'
    foreach ($name in @('TaskTrace', 'TaskTrace-server', 'TaskTrace-floating', 'TaskTrace-updater')) {
        foreach ($running in @(Get-Process -Name $name -ErrorAction SilentlyContinue)) {
            try {
                $runningPath = Get-InstallProcessImagePath $running
                if (!$runningPath) {
                    $running.Refresh()
                    try { if ($running.HasExited) { continue } } catch { }
                    $found.Add($name + '（PID ' + $running.Id + '，无法确认运行路径，请先退出该程序）')
                    continue
                }
                $runningDirectory = [TaskTraceInstallNative]::DirectoryPath([IO.Path]::GetDirectoryName($runningPath))
                if ([string]::Equals($runningDirectory, $targetDirectory, [StringComparison]::OrdinalIgnoreCase)) {
                    $found.Add($name + '（PID ' + $running.Id + '）')
                }
            } finally { $running.Dispose() }
        }
    }
    return $found.ToArray()
}

function Assert-InstallationAvailable([string]$Directory) {
    $runningNames = @(Get-RunningInstallationProcesses $Directory)
    if ($runningNames.Count -gt 0) {
        throw ('检测到 TaskTrace 正在运行或无法确认运行路径：' + ($runningNames -join '、') + '。请从系统托盘退出后重新安装；尚未覆盖程序文件。')
    }
    # Also catch a renamed launcher, updater or another tool holding a target
    # file. Opening existing program files here never changes their contents.
    foreach ($name in @('TaskTrace.exe', 'TaskTrace-server.exe', 'TaskTrace-floating.exe', 'TaskTrace-updater.exe', 'Launch-TaskTrace.ps1')) {
        $path = Join-Path $Directory $name
        if (!(Test-Path -LiteralPath $path -PathType Leaf)) { continue }
        $probe = $null
        try { $probe = [IO.File]::Open($path, [IO.FileMode]::Open, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None) }
        catch {
            throw ('程序文件被占用或没有写入权限：' + $path + '。请退出 TaskTrace，并检查该目录的写入权限后重试。原程序和数据尚未修改。原因：' + $_.Exception.Message)
        } finally { if ($null -ne $probe) { $probe.Dispose() } }
    }
}

function Install-ProgramFiles([string]$SourceDirectory, [string]$DestinationDirectory, [string[]]$Names) {
    # Recheck after the build in case the user started this copy in the meantime.
    Assert-InstallationAvailable $DestinationDirectory
    New-Item -ItemType Directory -Path $DestinationDirectory -Force | Out-Null
    $transactionRoot = Join-Path $env:TEMP ('TaskTrace-install-' + [Guid]::NewGuid().ToString('N'))
    $backupRoot = Join-Path $transactionRoot 'previous-program'
    New-Item -ItemType Directory -Path $backupRoot -Force | Out-Null
    $previous = @{}
    try {
        foreach ($name in $Names) {
            $source = Join-Path $SourceDirectory $name
            if (!(Test-Path -LiteralPath $source -PathType Leaf)) { throw ('安装文件不完整，缺少 ' + $name) }
            $destination = Join-Path $DestinationDirectory $name
            $previous[$name] = Test-Path -LiteralPath $destination -PathType Leaf
            if ($previous[$name]) { Copy-Item -LiteralPath $destination -Destination (Join-Path $backupRoot $name) -Force }
        }
        foreach ($name in $Names) {
            $destination = Join-Path $DestinationDirectory $name
            $pending = $destination + '.tasktrace-installing'
            try {
                Copy-Item -LiteralPath (Join-Path $SourceDirectory $name) -Destination $pending -Force
                if (Test-Path -LiteralPath $destination -PathType Leaf) {
                    [IO.File]::Delete($destination)
                }
                [IO.File]::Move($pending, $destination)
            } finally { Remove-Item -LiteralPath $pending -Force -ErrorAction SilentlyContinue }
        }
    } catch {
        foreach ($name in $Names) {
            $destination = Join-Path $DestinationDirectory $name
            if ($previous.ContainsKey($name) -and $previous[$name]) {
                Copy-Item -LiteralPath (Join-Path $backupRoot $name) -Destination $destination -Force -ErrorAction SilentlyContinue
            } elseif ($previous.ContainsKey($name)) {
                Remove-Item -LiteralPath $destination -Force -ErrorAction SilentlyContinue
            }
        }
        throw
    } finally { Remove-Item -LiteralPath $transactionRoot -Recurse -Force -ErrorAction SilentlyContinue }
}

function Add-DependencyIssue([string]$Problem, [string]$Hint) {
    $issues.Add($Problem)
    if (![string]::IsNullOrWhiteSpace($Hint) -and !$hints.Contains($Hint)) { $hints.Add($Hint) }
}

function Find-Command([string]$Name, [string[]]$Candidates = @()) {
    $command = Get-Command ($Name + '.cmd') -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $command) { $command = Get-Command ($Name + '.exe') -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1 }
    if ($null -eq $command) { $command = Get-Command $Name -CommandType Application,ExternalScript -ErrorAction SilentlyContinue | Select-Object -First 1 }
    if ($null -ne $command) { return $command }
    foreach ($candidate in $Candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) { continue }
        $expanded = [Environment]::ExpandEnvironmentVariables($candidate)
        if (Test-Path -LiteralPath $expanded -PathType Leaf) { return [pscustomobject]@{ Source = [IO.Path]::GetFullPath($expanded) } }
    }
    return $null
}

function Register-CommandPath($Command) {
    if ($null -eq $Command -or [string]::IsNullOrWhiteSpace($Command.Source)) { return }
    $directory = Split-Path $Command.Source -Parent
    $entries = @($env:Path -split ';' | Where-Object { ![string]::IsNullOrWhiteSpace($_) })
    if (!($entries | Where-Object { [string]::Equals($_.TrimEnd('\'), $directory.TrimEnd('\'), [StringComparison]::OrdinalIgnoreCase) })) {
        $env:Path = $directory + ';' + $env:Path
    }
}

function Join-OptionalPath([string]$Base, [string]$Child) {
    if ([string]::IsNullOrWhiteSpace($Base)) { return '' }
    return Join-Path $Base $Child
}

function Import-FreshBuildEnvironment {
    # Explorer and a terminal opened after installing a tool can have different
    # environment snapshots. Read the persisted values again so a double-clicked
    # installer sees the same tools as a newly opened command prompt.
    foreach ($name in @('PNPM_HOME', 'NVM_HOME', 'NVM_SYMLINK', 'GOROOT')) {
        $value = [Environment]::GetEnvironmentVariable($name, 'User')
        if ([string]::IsNullOrWhiteSpace($value)) { $value = [Environment]::GetEnvironmentVariable($name, 'Machine') }
        if (![string]::IsNullOrWhiteSpace($value)) { [Environment]::SetEnvironmentVariable($name, $value, 'Process') }
    }

    $paths = New-Object 'System.Collections.Generic.List[string]'
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($target in @('Process', 'User', 'Machine')) {
        $pathValue = [Environment]::GetEnvironmentVariable('Path', $target)
        foreach ($entry in @($pathValue -split ';')) {
            if ([string]::IsNullOrWhiteSpace($entry)) { continue }
            $expanded = [Environment]::ExpandEnvironmentVariables($entry.Trim().Trim('"'))
            if (![string]::IsNullOrWhiteSpace($expanded) -and $seen.Add($expanded.TrimEnd('\'))) { $paths.Add($expanded) }
        }
    }
    $env:Path = $paths -join ';'
}

function Find-SystemProxy([string]$TargetUrl) {
    try {
        $target = [Uri]$TargetUrl
        $proxy = [Net.WebRequest]::GetSystemWebProxy()
        $proxy.Credentials = [Net.CredentialCache]::DefaultCredentials
        $resolved = $proxy.GetProxy($target)
        if ($null -ne $resolved -and !$resolved.Equals($target)) { return $resolved.AbsoluteUri }
    } catch {
        Write-InstallLine ('读取系统代理失败：' + $_.Exception.Message) Yellow
    }
    return ''
}

function Select-DependencyProxy([string]$TargetUrl) {
    $systemProxy = Find-SystemProxy $TargetUrl
    if (![string]::IsNullOrWhiteSpace($systemProxy)) { return $systemProxy }
    foreach ($name in @('HTTPS_PROXY', 'https_proxy', 'HTTP_PROXY', 'http_proxy')) {
        $value = [Environment]::GetEnvironmentVariable($name, 'Process')
        if (![string]::IsNullOrWhiteSpace($value)) { return $value }
    }
    return '__TASKTRACE_DIRECT__'
}

function Format-ProxyForLog([string]$Proxy) {
    if ($Proxy -eq '__TASKTRACE_DIRECT__') { return '直接连接（系统未为目标地址配置代理）' }
    try {
        $uri = [UriBuilder]$Proxy
        $uri.UserName = ''
        $uri.Password = ''
        return $uri.Uri.AbsoluteUri
    } catch { return '系统代理（地址已隐藏）' }
}

function Get-CommandSources([string]$Name) {
    $sources = New-Object 'System.Collections.Generic.List[string]'
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($lookup in @(($Name + '.cmd'), ($Name + '.exe'), $Name)) {
        foreach ($command in @(Get-Command $lookup -CommandType Application,ExternalScript -All -ErrorAction SilentlyContinue)) {
            $source = if (![string]::IsNullOrWhiteSpace($command.Source)) { $command.Source } else { $command.Path }
            if (![string]::IsNullOrWhiteSpace($source) -and (Test-Path -LiteralPath $source -PathType Leaf)) {
                $fullPath = [IO.Path]::GetFullPath($source)
                if ($seen.Add($fullPath)) { $sources.Add($fullPath) }
            }
        }
    }
    try {
        foreach ($source in @(& where.exe $Name 2>$null)) {
            if (![string]::IsNullOrWhiteSpace($source) -and (Test-Path -LiteralPath $source.Trim() -PathType Leaf)) {
                $fullPath = [IO.Path]::GetFullPath($source.Trim())
                if ($seen.Add($fullPath)) { $sources.Add($fullPath) }
            }
        }
    } catch { }
    return @($sources)
}

function Find-CompatibleCommand([string]$Name, [string[]]$Candidates, [string[]]$Arguments, [Version]$MinimumVersion) {
    $commands = New-Object 'System.Collections.Generic.List[object]'
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
    foreach ($source in @(Get-CommandSources $Name)) {
        if ($seen.Add($source)) { $commands.Add([pscustomobject]@{ Source = $source }) }
    }
    foreach ($candidate in $Candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) { continue }
        $expanded = [Environment]::ExpandEnvironmentVariables($candidate)
        if ((Test-Path -LiteralPath $expanded -PathType Leaf) -and $seen.Add([IO.Path]::GetFullPath($expanded))) { $commands.Add([pscustomobject]@{ Source = [IO.Path]::GetFullPath($expanded) }) }
    }
    foreach ($command in $commands) {
        try {
            $text = Read-CommandText $command.Source $Arguments
            if ((Read-Version $text) -ge $MinimumVersion) { return $command }
        } catch {
            Write-InstallLine ('无法使用候选 ' + $Name + '：' + $command.Source + ' · ' + $_.Exception.Message) Yellow
        }
    }
    return $commands | Select-Object -First 1
}

function Read-Version([string]$Text) {
    $match = [regex]::Match($Text, '(?<!\d)(\d+)\.(\d+)(?:\.(\d+))?')
    if (!$match.Success) { return $null }
    $patch = 0
    if ($match.Groups[3].Success) { $patch = [int]$match.Groups[3].Value }
    return [Version]::new([int]$match.Groups[1].Value, [int]$match.Groups[2].Value, $patch)
}

function Read-CommandText([string]$Command, [string[]]$Arguments) {
    $result = & $Command @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw (($result | Out-String).Trim()) }
    return (($result | Out-String).Trim())
}

try {
    [IO.File]::WriteAllText($logFile, ('TaskTrace build started: ' + [DateTime]::Now.ToString('o') + [Environment]::NewLine), [Text.UTF8Encoding]::new($true))
    Write-InstallLine 'TaskTrace 免安装程序生成工具' Cyan
    Write-InstallLine ('源码目录：' + $root)
    if (!$CheckOnly) {
        $selectedDirectory = Select-InstallDirectory $InstallDirectory
        if ([string]::IsNullOrWhiteSpace($selectedDirectory)) {
            Write-InstallLine '用户已取消安装。' Yellow
            exit 0
        }
        $outputDirectory = $selectedDirectory
        Write-InstallLine ('安装目录：' + $outputDirectory) Cyan
        $stage = '检查程序运行状态及文件占用'
        Write-InstallLine '首先检查程序运行状态及安装目录文件占用...'
        try { Assert-InstallationAvailable $outputDirectory }
        catch {
            Write-InstallLine $_.Exception.Message Red
            Write-InstallLine '安装尚未开始，没有下载依赖、编译或覆盖任何程序和数据。' Yellow
            Show-InstallResult 'TaskTrace：请先解除程序占用' ($_.Exception.Message + "`r`n`r`n安装尚未开始，没有下载依赖、编译或覆盖任何程序和数据。") $true
            exit 2
        }
        if (!(Confirm-ExistingInstallation $outputDirectory)) {
            Write-InstallLine '用户已取消覆盖安装。' Yellow
            exit 0
        }
    }
    $stage = '检查构建环境'
    Import-FreshBuildEnvironment
	Write-InstallLog '已重新读取当前用户和系统的 PATH，避免双击安装器使用旧环境。'
    $env:TASKTRACE_NPM_PROXY = Select-DependencyProxy 'https://registry.npmjs.org/'
    $env:TASKTRACE_GO_PROXY = Select-DependencyProxy 'https://proxy.golang.org/'
	Write-InstallLog ('前端依赖网络：' + (Format-ProxyForLog $env:TASKTRACE_NPM_PROXY))
	Write-InstallLog ('Go 模块网络：' + (Format-ProxyForLog $env:TASKTRACE_GO_PROXY))

    if ($PSVersionTable.PSVersion -lt [Version]'5.1') {
        Add-DependencyIssue ('PowerShell 版本过低：' + $PSVersionTable.PSVersion) '请升级到 Windows PowerShell 5.1 或 PowerShell 7。'
    }
    if (![Environment]::Is64BitOperatingSystem) {
        Add-DependencyIssue '当前不是 64 位 Windows，TaskTrace 目前只生成 Windows x64 程序。' '请在 64 位 Windows 10/11 上运行此工具。'
    }
    foreach ($requiredFile in @('go.mod', 'frontend\package.json', 'frontend\pnpm-lock.yaml', 'portable\Build-Local.ps1', 'portable\DependencyBootstrap.ps1', 'portable\SourceBuildVersion.ps1', 'portable\LATEST-RELEASE.txt')) {
        if (!(Test-Path -LiteralPath (Join-Path $root $requiredFile))) {
            Add-DependencyIssue ('源码不完整，缺少：' + $requiredFile) '请重新下载或解压完整的 TaskTrace 源码。'
        }
    }

	Write-InstallLog 'Git 和 .git 信息不是生成免安装程序的必要条件。'

    if (!$SkipFrontend) {
        $nodeCandidates = @(
            (Join-Path $env:ProgramFiles 'nodejs\node.exe'),
            (Join-OptionalPath ${env:ProgramFiles(x86)} 'nodejs\node.exe'),
            (Join-Path $env:LOCALAPPDATA 'Programs\nodejs\node.exe'),
            (Join-Path $env:LOCALAPPDATA 'Volta\bin\node.exe'),
            (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Links\node.exe'),
            (Join-Path $env:USERPROFILE 'scoop\apps\nodejs\current\node.exe'),
            (Join-Path $env:USERPROFILE 'scoop\apps\nodejs-lts\current\node.exe'),
            (Join-OptionalPath $env:NVM_SYMLINK 'node.exe')
        )
        foreach ($registryKey in @('HKLM:\SOFTWARE\Node.js', 'HKCU:\SOFTWARE\Node.js')) {
            try { $installPath = (Get-ItemProperty -LiteralPath $registryKey -ErrorAction Stop).InstallPath; if ($installPath) { $nodeCandidates += (Join-Path $installPath 'node.exe') } } catch { }
        }
        $node = Find-CompatibleCommand 'node' $nodeCandidates @('--version') ([Version]'24.0.0')
        if ($null -eq $node) {
            Add-DependencyIssue '未找到 Node.js；已检查 PATH、Node.js 安装目录、Volta、Scoop 和 NVM_SYMLINK。' '安装 Node.js 24 或更新版本；安装完成后可直接重试，无需重启电脑：https://nodejs.org/'
        } else {
            try {
                Register-CommandPath $node
                $nodeText = Read-CommandText $node.Source @('--version'); $nodeVersion = Read-Version $nodeText
				Write-InstallLog ('Node.js：' + $nodeText + ' · ' + $node.Source)
                if ($null -eq $nodeVersion -or $nodeVersion -lt [Version]'24.0.0') { Add-DependencyIssue ('Node.js 版本过低：' + $nodeText + '，要求 24.0.0 或更新版本。') '安装 Node.js 24 或更新版本：https://nodejs.org/' }
            } catch { Add-DependencyIssue ('Node.js 无法运行：' + $_.Exception.Message) '重新安装 Node.js 24 或更新版本：https://nodejs.org/' }
        }

        $pnpmCandidates = @(
            (Join-OptionalPath $env:PNPM_HOME 'pnpm.cmd'),
            (Join-Path $env:LOCALAPPDATA 'pnpm\pnpm.cmd'),
            (Join-Path $env:APPDATA 'npm\pnpm.cmd'),
            (Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Links\pnpm.cmd'),
            (Join-Path $env:ProgramFiles 'nodejs\pnpm.cmd')
        )
        if ($null -ne $node) { $pnpmCandidates += (Join-Path (Split-Path $node.Source -Parent) 'pnpm.cmd') }
        $pnpm = Find-CompatibleCommand 'pnpm' $pnpmCandidates @('--version') ([Version]'11.26.0')
        if ($null -eq $pnpm) {
            $corepackCandidates = @((Join-Path $env:ProgramFiles 'nodejs\corepack.cmd'))
            if ($null -ne $node) { $corepackCandidates += (Join-Path (Split-Path $node.Source -Parent) 'corepack.cmd') }
            $corepack = Find-Command 'corepack' $corepackCandidates
            if ($null -ne $corepack) {
                try {
                    $corepackText = Read-CommandText $corepack.Source @('pnpm', '--version')
                    $shimDirectory = Join-Path $env:TEMP 'TaskTrace-build-tools'; New-Item -ItemType Directory -Path $shimDirectory -Force | Out-Null
                    $shim = Join-Path $shimDirectory 'pnpm.cmd'; [IO.File]::WriteAllText($shim, "@echo off`r`n`"$($corepack.Source)`" pnpm %*`r`n", [Text.Encoding]::ASCII)
                    $pnpm = [pscustomobject]@{ Source = $shim }
					Write-InstallLog ('pnpm 由 Corepack 提供：' + $corepackText + ' · ' + $corepack.Source)
                } catch { }
            }
        }
        if ($null -eq $pnpm) {
            Add-DependencyIssue '未找到 pnpm；已检查 PATH、PNPM_HOME、AppData npm/pnpm 和 Node.js Corepack。' '安装 pnpm 11.26.0：npm install -g pnpm@11.26.0'
        } else {
            try {
                Register-CommandPath $pnpm
                $pnpmText = Read-CommandText $pnpm.Source @('--version'); $pnpmVersion = Read-Version $pnpmText
				Write-InstallLog ('pnpm：' + $pnpmText + ' · ' + $pnpm.Source)
                if ($null -eq $pnpmVersion -or $pnpmVersion -lt [Version]'11.26.0') { Add-DependencyIssue ('pnpm 版本过低：' + $pnpmText + '，要求 11.26.0 或更新版本。') '升级 pnpm：npm install -g pnpm@11.26.0' }
            } catch { Add-DependencyIssue ('pnpm 无法运行：' + $_.Exception.Message) '重新安装 pnpm：npm install -g pnpm@11.26.0' }
        }
    } else {
        Write-InstallLine '已选择跳过前端构建；仅适用于 frontend/dist 已由本仓库成功构建的开发环境。' Yellow
    }

    $goCandidates = @((Join-Path $env:ProgramFiles 'Go\bin\go.exe'),(Join-Path $env:LOCALAPPDATA 'Programs\Go\bin\go.exe'),(Join-Path $env:LOCALAPPDATA 'Microsoft\WinGet\Links\go.exe'),(Join-OptionalPath $env:GOROOT 'bin\go.exe'),(Join-Path $env:USERPROFILE 'scoop\apps\go\current\bin\go.exe'))
    foreach ($registryKey in @('HKLM:\SOFTWARE\GoProgrammingLanguage', 'HKCU:\SOFTWARE\GoProgrammingLanguage', 'HKLM:\SOFTWARE\WOW6432Node\GoProgrammingLanguage')) {
        try { $installRoot = (Get-ItemProperty -LiteralPath $registryKey -ErrorAction Stop).InstallRoot; if ($installRoot) { $goCandidates += (Join-Path $installRoot 'bin\go.exe') } } catch { }
    }
    $go = Find-CompatibleCommand 'go' $goCandidates @('version') ([Version]'1.27.0')
    if ($null -eq $go) {
        Add-DependencyIssue '未找到 Go。' '安装 Go 1.27.0 或更新版本：https://go.dev/dl/'
    } else {
        try {
            Register-CommandPath $go
            $goText = Read-CommandText $go.Source @('version'); $goVersion = Read-Version $goText
			Write-InstallLog ('Go：' + $goText + ' · ' + $go.Source)
            if ($null -eq $goVersion -or $goVersion -lt [Version]'1.27.0') { Add-DependencyIssue ('Go 版本过低：' + $goText + '，要求 1.27.0 或更新版本。') '安装 Go 1.27.0 或更新版本：https://go.dev/dl/' }
        } catch { Add-DependencyIssue ('Go 无法运行：' + $_.Exception.Message) '重新安装 Go：https://go.dev/dl/' }
    }

    $gccCandidates = @('C:\msys64\ucrt64\bin\gcc.exe','C:\msys64\mingw64\bin\gcc.exe',(Join-Path $env:USERPROFILE 'scoop\apps\gcc\current\bin\gcc.exe'))
    $gcc = Find-Command 'gcc' $gccCandidates
    if ($null -eq $gcc) {
        Add-DependencyIssue '未找到 x64 GCC，Go 的 SQLite 静态编译需要它。' '安装 MSYS2 UCRT64 GCC：https://www.msys2.org/，并把 mingw64\bin 或 ucrt64\bin 加入 PATH。'
    } else {
        try {
            Register-CommandPath $gcc
            $gccTarget = Read-CommandText $gcc.Source @('-dumpmachine')
			Write-InstallLog ('GCC 目标：' + $gccTarget + ' · ' + $gcc.Source)
            if ($gccTarget -notmatch 'x86_64.*(mingw|windows)') { Add-DependencyIssue ('GCC 目标不兼容：' + $gccTarget + '，要求 Windows x64 GCC。') '安装 MSYS2 UCRT64 GCC：https://www.msys2.org/' }
        } catch { Add-DependencyIssue ('GCC 无法运行：' + $_.Exception.Message) '重新安装 MSYS2 UCRT64 GCC：https://www.msys2.org/' }
    }

    $stripCandidates = @('C:\msys64\ucrt64\bin\strip.exe','C:\msys64\mingw64\bin\strip.exe','C:\MinGW\bin\strip.exe',(Join-Path $env:USERPROFILE 'scoop\apps\gcc\current\bin\strip.exe'))
    $strip = Find-Command 'strip' $stripCandidates
    if ($null -eq $strip) {
        Add-DependencyIssue '未找到 GNU Binutils strip；Windows 服务端构建后需要它整理 PE 文件。' '安装 MSYS2 UCRT64 GCC（其中包含 strip）：https://www.msys2.org/'
    } else {
		try { Register-CommandPath $strip; Write-InstallLog ('strip：' + (Read-CommandText $strip.Source @('--version')).Split([Environment]::NewLine)[0] + ' · ' + $strip.Source) }
        catch { Add-DependencyIssue ('strip 无法运行：' + $_.Exception.Message) '重新安装 MSYS2 UCRT64 GCC：https://www.msys2.org/' }
    }

    $compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    if (!(Test-Path -LiteralPath $compiler -PathType Leaf)) {
        Add-DependencyIssue ('未找到 .NET Framework C# 编译器：' + $compiler) '在“启用或关闭 Windows 功能”中启用 .NET Framework 4.8，或安装 .NET Framework 4.8 Developer Pack。'
	} else { Write-InstallLog ('C# 编译器：' + $compiler) }

    if ($issues.Count -gt 0) {
        Write-InstallLine ''
        Write-InstallLine ('检测到 ' + $issues.Count + ' 个问题，尚未开始构建：') Red
        foreach ($issue in $issues) { Write-InstallLine ('  - ' + $issue) Red }
        Write-InstallLine ''
        Write-InstallLine '解决方法：' Yellow
        foreach ($hint in $hints) { Write-InstallLine ('  - ' + $hint) Yellow }
        Write-InstallLine ('详细记录：' + $logFile)
        $problemText = (($issues | ForEach-Object { '• ' + $_ }) -join [Environment]::NewLine)
        $hintText = (($hints | ForEach-Object { '• ' + $_ }) -join [Environment]::NewLine)
        Show-InstallResult 'TaskTrace：缺少构建条件' ("尚未开始生成程序。`r`n`r`n发现的问题：`r`n" + $problemText + "`r`n`r`n解决方法：`r`n" + $hintText + "`r`n`r`n详细记录：" + $logFile) $true
        exit 2
    }

    if ($CheckOnly) {
        Write-InstallLine ''
		Write-InstallLine '构建环境检查通过。' Green
		Show-InstallResult 'TaskTrace：检查完成' ("构建环境检查通过。`r`n`r`n详细记录：" + $logFile)
        exit 0
    }

    $stage = '生成免安装程序'
    Write-InstallLine ''
	Write-InstallLine '环境检查通过，开始生成免安装程序。' Green
    . (Join-Path $root 'portable\SourceBuildVersion.ps1')
    $Version = Resolve-TaskTraceSourceBuildVersion $Version 'niyilu45/TaskTrace' $env:TASKTRACE_NPM_PROXY
    Write-InstallLine ('源码构建版本：' + $Version) Cyan
    $buildScript = Join-Path $root 'portable\Build-Local.ps1'
    $buildStagingRoot = Join-Path $env:TEMP ('TaskTrace-build-' + [Guid]::NewGuid().ToString('N'))
    $buildOutputDirectory = Join-Path $buildStagingRoot 'program'
    $buildArguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $buildScript, '-PackageDirectory', $buildOutputDirectory, '-Version', $Version, '-SkipArchive')
    if ($SkipFrontend) { $buildArguments += '-SkipFrontend' }
    $savedErrorPreference = $ErrorActionPreference
    try {
        # Build tools write warnings to stderr even when they succeed. Capture the
        # complete output without treating those warnings as installer failures.
        $ErrorActionPreference = 'Continue'
        $buildOutput = @(& powershell.exe @buildArguments 2>&1 | ForEach-Object {
            $line = [string]$_
            if ($line -match '^(完整构建依赖清单|共 \d+ 项前端锁定依赖|前端锁定依赖|前端正在下载|前端下载完成|前端依赖|pnpm：|Go 模块|Go 正在下载|Go 下载完成|Go 下载连接中断|Package ready:)') { Write-Host $line }
            try { [IO.File]::AppendAllText($logFile, $line + [Environment]::NewLine, [Text.UTF8Encoding]::new($true)) } catch { }
            $line
        })
        $buildExitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedErrorPreference }
    $buildLines = @($buildOutput | ForEach-Object { [string]$_ })
    if ($buildExitCode -ne 0) {
        Write-InstallLine '构建工具最后输出：' Yellow
        foreach ($line in @($buildLines | Select-Object -Last 30)) { Write-Host $line }
        throw ('构建脚本返回错误代码 ' + $buildExitCode + '。请根据上方错误处理。')
    }
    Write-InstallLine '前端、后端和 Windows 启动程序编译完成。'

    foreach ($file in @('TaskTrace.exe', 'TaskTrace-server.exe', 'TaskTrace-floating.exe', 'Launch-TaskTrace.ps1', 'SOURCE-COMMIT.txt')) {
        if (!(Test-Path -LiteralPath (Join-Path $buildOutputDirectory $file) -PathType Leaf)) { throw ('生成结果不完整，缺少 ' + $file) }
    }
    $stage = '生成安全复制包'
    $copyArchive = Join-Path $PSScriptRoot 'TaskTrace-program-files.zip'
    $programFileNames = @(
        'TaskTrace.exe',
        'TaskTrace-updater.exe',
        'Configure-TaskTrace.cmd',
        'Configure-TaskTrace.ps1',
        'tasktrace-settings.example.json',
        'TaskTrace-server.exe',
        'TaskTrace-floating.exe',
        'Launch-TaskTrace.ps1',
        'README.md',
        'LICENSE',
        'UPSTREAM-COMMIT.txt',
        'SOURCE-COMMIT.txt',
        'VERSION.txt'
    )
    $programFiles = @($programFileNames | ForEach-Object {
        $programFile = Join-Path $buildOutputDirectory $_
        if (!(Test-Path -LiteralPath $programFile -PathType Leaf)) { throw ('无法生成安全复制包，缺少 ' + $_) }
        $programFile
    })
    Compress-Archive -LiteralPath $programFiles -DestinationPath $copyArchive -Force
    if (!(Test-Path -LiteralPath $copyArchive -PathType Leaf)) { throw '安全复制包生成失败。' }
    $stage = '安装程序文件'
    Install-ProgramFiles $buildOutputDirectory $outputDirectory $programFileNames
    Write-InstallLine ''
    Write-InstallLine '免安装程序生成并安装成功。' Green
    Write-InstallLine ('程序位置：' + (Join-Path $outputDirectory 'TaskTrace.exe')) Green
    Write-InstallLine ('复制到其他电脑请使用：' + $copyArchive) Green
    Write-InstallLine '本次安装只覆盖程序文件；配置、data、teamData、.cache、backups 和其他用户文件均未修改。' Yellow
    Write-InstallLine '运行程序不再需要 Node.js、pnpm、Go、GCC 或 C# 编译器。'
    Write-InstallLine ('详细记录：' + $logFile)
    Show-InstallResult 'TaskTrace：安装成功' ("TaskTrace 已安装到：`r`n" + $outputDirectory + "`r`n`r`n只覆盖了程序文件。原有配置、data、teamData、.cache、backups 和其他用户文件均已保留。`r`n`r`n同时生成安全复制包：`r`n" + $copyArchive + "`r`n`r`n详细记录：" + $logFile)
    exit 0
} catch {
    Write-InstallLine ''
    Write-InstallLine ('生成失败，阶段：' + $stage) Red
    Write-InstallLine ('错误：' + $_.Exception.Message) Red
    if ($stage -eq '生成免安装程序') {
        Write-InstallLine '如果错误中包含下载失败，请检查网络、系统代理以及 npm、pnpm、Go 的镜像配置。' Yellow
        Write-InstallLine '如果错误中包含拒绝访问，请退出正在运行的 TaskTrace，并确认源码目录可写。' Yellow
    }
    Write-InstallLine ('详细记录：' + $logFile)
    Show-InstallResult 'TaskTrace：生成失败' ("失败阶段：" + $stage + "`r`n`r`n错误：" + $_.Exception.Message + "`r`n`r`n详细记录：" + $logFile) $true
    exit 1
} finally {
    if (![string]::IsNullOrWhiteSpace($buildStagingRoot)) { Remove-Item -LiteralPath $buildStagingRoot -Recurse -Force -ErrorAction SilentlyContinue }
}
