[CmdletBinding()]
param(
    [switch]$CheckOnly,
    [switch]$SkipFrontend,
    [string]$Version = 'v0.1.0-beta.11'
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

function Add-DependencyIssue([string]$Problem, [string]$Hint) {
    $issues.Add($Problem)
    if (![string]::IsNullOrWhiteSpace($Hint) -and !$hints.Contains($Hint)) { $hints.Add($Hint) }
}

function Find-Command([string]$Name) {
    return Get-Command $Name -ErrorAction SilentlyContinue | Select-Object -First 1
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

    if ($PSVersionTable.PSVersion -lt [Version]'5.1') {
        Add-DependencyIssue ('PowerShell 版本过低：' + $PSVersionTable.PSVersion) '请升级到 Windows PowerShell 5.1 或 PowerShell 7。'
    }
    if (![Environment]::Is64BitOperatingSystem) {
        Add-DependencyIssue '当前不是 64 位 Windows，TaskTrace 目前只生成 Windows x64 程序。' '请在 64 位 Windows 10/11 上运行此工具。'
    }
    if (!(Test-Path -LiteralPath (Join-Path $root '.git'))) {
        Add-DependencyIssue '源码目录缺少 .git 信息，无法记录成品对应的源码提交。' '请使用 git clone 获取仓库，不要使用 GitHub 的 Source code ZIP。'
    }
    foreach ($requiredFile in @('go.mod', 'frontend\package.json', 'frontend\pnpm-lock.yaml', 'portable\Build-Local.ps1')) {
        if (!(Test-Path -LiteralPath (Join-Path $root $requiredFile))) {
            Add-DependencyIssue ('源码不完整，缺少：' + $requiredFile) '请重新克隆完整的 TaskTrace 仓库。'
        }
    }

    $git = Find-Command 'git'
    if ($null -eq $git) {
        Add-DependencyIssue '未找到 Git。' '安装 Git for Windows：https://git-scm.com/download/win'
    } else {
        try { Write-InstallLine ('Git：' + (Read-CommandText $git.Source @('--version'))) } catch { Add-DependencyIssue ('Git 无法运行：' + $_.Exception.Message) '重新安装 Git for Windows：https://git-scm.com/download/win' }
    }

    if (!$SkipFrontend) {
        $node = Find-Command 'node'
        if ($null -eq $node) {
            Add-DependencyIssue '未找到 Node.js。' '安装 Node.js 24 或更新版本：https://nodejs.org/'
        } else {
            try {
                $nodeText = Read-CommandText $node.Source @('--version'); $nodeVersion = Read-Version $nodeText
                Write-InstallLine ('Node.js：' + $nodeText)
                if ($null -eq $nodeVersion -or $nodeVersion -lt [Version]'24.0.0') { Add-DependencyIssue ('Node.js 版本过低：' + $nodeText + '，要求 24.0.0 或更新版本。') '安装 Node.js 24 或更新版本：https://nodejs.org/' }
            } catch { Add-DependencyIssue ('Node.js 无法运行：' + $_.Exception.Message) '重新安装 Node.js 24 或更新版本：https://nodejs.org/' }
        }

        $pnpm = Find-Command 'pnpm'
        if ($null -eq $pnpm) {
            Add-DependencyIssue '未找到 pnpm。' '安装 pnpm 11.26.0：npm install -g pnpm@11.26.0'
        } else {
            try {
                $pnpmText = Read-CommandText $pnpm.Source @('--version'); $pnpmVersion = Read-Version $pnpmText
                Write-InstallLine ('pnpm：' + $pnpmText)
                if ($null -eq $pnpmVersion -or $pnpmVersion -lt [Version]'11.26.0') { Add-DependencyIssue ('pnpm 版本过低：' + $pnpmText + '，要求 11.26.0 或更新版本。') '升级 pnpm：npm install -g pnpm@11.26.0' }
            } catch { Add-DependencyIssue ('pnpm 无法运行：' + $_.Exception.Message) '重新安装 pnpm：npm install -g pnpm@11.26.0' }
        }
    } else {
        Write-InstallLine '已选择跳过前端构建；仅适用于 frontend/dist 已由本仓库成功构建的开发环境。' Yellow
    }

    $go = Find-Command 'go'
    if ($null -eq $go) {
        Add-DependencyIssue '未找到 Go。' '安装 Go 1.27.0 或更新版本：https://go.dev/dl/'
    } else {
        try {
            $goText = Read-CommandText $go.Source @('version'); $goVersion = Read-Version $goText
            Write-InstallLine ('Go：' + $goText)
            if ($null -eq $goVersion -or $goVersion -lt [Version]'1.27.0') { Add-DependencyIssue ('Go 版本过低：' + $goText + '，要求 1.27.0 或更新版本。') '安装 Go 1.27.0 或更新版本：https://go.dev/dl/' }
        } catch { Add-DependencyIssue ('Go 无法运行：' + $_.Exception.Message) '重新安装 Go：https://go.dev/dl/' }
    }

    $gcc = Find-Command 'gcc'
    if ($null -eq $gcc) {
        Add-DependencyIssue '未找到 x64 GCC，Go 的 SQLite 静态编译需要它。' '安装 MSYS2 UCRT64 GCC：https://www.msys2.org/，并把 mingw64\bin 或 ucrt64\bin 加入 PATH。'
    } else {
        try {
            $gccTarget = Read-CommandText $gcc.Source @('-dumpmachine')
            Write-InstallLine ('GCC 目标：' + $gccTarget)
            if ($gccTarget -notmatch 'x86_64.*(mingw|windows)') { Add-DependencyIssue ('GCC 目标不兼容：' + $gccTarget + '，要求 Windows x64 GCC。') '安装 MSYS2 UCRT64 GCC：https://www.msys2.org/' }
        } catch { Add-DependencyIssue ('GCC 无法运行：' + $_.Exception.Message) '重新安装 MSYS2 UCRT64 GCC：https://www.msys2.org/' }
    }

    $compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    if (!(Test-Path -LiteralPath $compiler -PathType Leaf)) {
        Add-DependencyIssue ('未找到 .NET Framework C# 编译器：' + $compiler) '在“启用或关闭 Windows 功能”中启用 .NET Framework 4.8，或安装 .NET Framework 4.8 Developer Pack。'
    } else { Write-InstallLine ('C# 编译器：' + $compiler) }

    foreach ($name in @('TaskTrace-server', 'TaskTrace-floating')) {
        if (Get-Process -Name $name -ErrorAction SilentlyContinue) {
            Add-DependencyIssue ('检测到 ' + $name + ' 正在运行，无法安全覆盖程序文件。') '请从 TaskTrace 系统托盘菜单选择“退出 TaskTrace”，然后重新运行安装工具。'
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
        exit 2
    }

    if ($CheckOnly) {
        Write-InstallLine ''
        Write-InstallLine '构建依赖检查通过。' Green
        exit 0
    }

    $stage = '生成免安装程序'
    Write-InstallLine ''
    Write-InstallLine '依赖检查通过，开始生成免安装程序。首次构建需要下载依赖，可能需要较长时间。' Green
    $buildScript = Join-Path $root 'portable\Build-Local.ps1'
    $buildArguments = @('-NoProfile', '-ExecutionPolicy', 'Bypass', '-File', $buildScript, '-PackageDirectory', 'dist/TaskTrace-local', '-Version', $Version, '-SkipArchive')
    if ($SkipFrontend) { $buildArguments += '-SkipFrontend' }
    $savedErrorPreference = $ErrorActionPreference
    try {
        # Build tools write warnings to stderr even when they succeed. Capture the
        # complete output without treating those warnings as installer failures.
        $ErrorActionPreference = 'Continue'
        $buildOutput = & powershell.exe @buildArguments 2>&1
        $buildExitCode = $LASTEXITCODE
    } finally { $ErrorActionPreference = $savedErrorPreference }
    $buildLines = @($buildOutput | ForEach-Object { [string]$_ })
    [IO.File]::AppendAllText($logFile, (($buildLines -join [Environment]::NewLine) + [Environment]::NewLine), [Text.UTF8Encoding]::new($true))
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
    exit 1
}
