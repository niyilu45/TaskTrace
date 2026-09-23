[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [switch]$SkipFrontend,
    [switch]$Interactive,
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
$outputDirectory = Join-Path $PSScriptRoot 'TaskTrace-local'
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
    foreach ($requiredFile in @('go.mod', 'frontend\package.json', 'frontend\pnpm-lock.yaml', 'portable\Build-Local.ps1', 'portable\DependencyBootstrap.ps1', 'portable\SourceBuildVersion.ps1')) {
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

    # Dependency inspection does not write package files. A full build only
    # conflicts with an instance launched from the package directory which is
    # about to be replaced; TaskTrace copies in other folders may keep running.
    if (!$CheckOnly) {
        $outputPrefix = [IO.Path]::GetFullPath($outputDirectory).TrimEnd('\') + '\'
        $runningPackageProcesses = New-Object 'System.Collections.Generic.List[string]'
        foreach ($name in @('TaskTrace', 'TaskTrace-server', 'TaskTrace-floating')) {
            foreach ($running in @(Get-Process -Name $name -ErrorAction SilentlyContinue)) {
                try { $runningPath = $running.MainModule.FileName } catch { $runningPath = $null }
                if ($runningPath -and $runningPath.StartsWith($outputPrefix, [StringComparison]::OrdinalIgnoreCase)) {
                    $runningPackageProcesses.Add($name)
                    break
                }
            }
        }
        if ($runningPackageProcesses.Count -gt 0) {
            Add-DependencyIssue ('检测到本次输出目录中的 TaskTrace 正在运行（' + ($runningPackageProcesses -join '、') + '），无法安全覆盖程序文件。') '请从该 TaskTrace 的系统托盘菜单选择“退出 TaskTrace”，然后重新运行安装工具。'
        }
    }

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
    $buildArguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $buildScript, '-PackageDirectory', 'dist/TaskTrace-local', '-Version', $Version, '-SkipArchive')
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
        if (!(Test-Path -LiteralPath (Join-Path $outputDirectory $file) -PathType Leaf)) { throw ('生成结果不完整，缺少 ' + $file) }
    }
    Write-InstallLine ''
    Write-InstallLine '免安装程序生成成功。' Green
    Write-InstallLine ('程序位置：' + (Join-Path $outputDirectory 'TaskTrace.exe')) Green
    Write-InstallLine '运行程序不再需要 Node.js、pnpm、Go、GCC 或 C# 编译器。'
    Write-InstallLine ('详细记录：' + $logFile)
    Show-InstallResult 'TaskTrace：生成成功' ("免安装程序已经生成。`r`n`r`n双击运行：`r`n" + (Join-Path $outputDirectory 'TaskTrace.exe') + "`r`n`r`n详细记录：" + $logFile)
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
}
