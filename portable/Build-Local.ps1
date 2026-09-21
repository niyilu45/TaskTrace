param([switch]$SkipFrontend, [switch]$SkipArchive, [string]$PackageDirectory = 'dist/TaskTrace-local', [string]$Version = 'v0.1.0-beta.11')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$packageRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $PackageDirectory))
. (Join-Path $PSScriptRoot 'DependencyBootstrap.ps1')
if ($Version -notmatch '^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { throw 'Version must be a semantic version such as v0.1.0-beta.11.' }
function Assert-Exit([string]$Step) {
    if ($LASTEXITCODE -ne 0) { throw "$Step failed with exit code $LASTEXITCODE" }
}
function Source-Identifier {
    if (![string]::IsNullOrWhiteSpace($env:TASKTRACE_SOURCE_COMMIT)) { return $env:TASKTRACE_SOURCE_COMMIT.Trim() }
    $saved = Join-Path $repoRoot 'SOURCE-COMMIT.txt'
    if (Test-Path -LiteralPath $saved -PathType Leaf) { $value = [IO.File]::ReadAllText($saved).Trim(); if ($value) { return $value } }
    if ((Test-Path -LiteralPath (Join-Path $repoRoot '.git')) -and (Get-Command git -ErrorAction SilentlyContinue)) {
        $value = (& git -C $repoRoot rev-parse HEAD 2>$null)
        if ($LASTEXITCODE -eq 0 -and ![string]::IsNullOrWhiteSpace($value)) { return $value.Trim() }
    }
    return ('source-archive-' + $Version.TrimStart('v'))
}
Push-Location $repoRoot
try {
	$dependencyManifest = Join-Path $repoRoot 'dist\install-dependencies.txt'
	$dependencies = Write-TaskTraceDependencyManifest $repoRoot $dependencyManifest
    if (!$SkipFrontend) {
        $nodeVersion = & node -p 'process.versions.node'
        Assert-Exit 'Node version check'
        if ([int]($nodeVersion.Split('.')[0]) -lt 24) { throw 'Building requires Node.js 24 or newer. Running the finished package does not.' }
        $oldLocalMode = $env:VITE_TASKTRACE_LOCAL
        Push-Location (Join-Path $repoRoot 'frontend')
        try {
            Set-TaskTraceDependencyProxy $env:TASKTRACE_NPM_PROXY
            Install-TaskTraceFrontendDependencies $dependencies.Frontend.Count
            $env:VITE_TASKTRACE_LOCAL = 'true'
            & pnpm run build
            Assert-Exit 'Frontend build'
        } finally { $env:VITE_TASKTRACE_LOCAL = $oldLocalMode; Pop-Location }
    }
    if (!(Test-Path -LiteralPath 'frontend/dist/index.html')) { throw 'Build frontend/dist first.' }
    $localFrontendMarker = 'frontend/dist/tasktrace-local-build.txt'
    if (!(Test-Path -LiteralPath $localFrontendMarker) -or ([IO.File]::ReadAllText((Join-Path $repoRoot $localFrontendMarker)).Trim() -ne 'tasktrace-local')) {
        throw 'frontend/dist is not a TaskTrace local build. Run Build-Local.ps1 without -SkipFrontend before packaging.'
    }
    New-Item -ItemType Directory -Path $packageRoot -Force | Out-Null
    $oldCGO = $env:CGO_ENABLED
    $oldCC = $env:CC
    try {
        Set-TaskTraceDependencyProxy $env:TASKTRACE_GO_PROXY
        Download-TaskTraceGoDependencies $dependencies.GoModules
        $env:CGO_ENABLED = '1'
        $env:CC = 'gcc'
        $ldflags = '-s -w -linkmode external -extldflags "-static" -X code.vikunja.io/api/pkg/version.Version=' + $Version
        $serverBinary = Join-Path $packageRoot 'TaskTrace-server.exe'
        $strippedServerBinary = Join-Path $packageRoot '.TaskTrace-server.stripped.exe'
        & go build -tags 'osusergo,timetzdata' -ldflags $ldflags -o $serverBinary .
        Assert-Exit 'Windows server build'
        # Older MinGW versions leave Go's external-link sections in a layout
        # which Windows may reject with error 193. Binutils strip rewrites the
        # PE section table while retaining the statically linked SQLite code.
        # Write a separate output first because some MinGW builds cannot rename
        # their temporary file over an existing executable on Windows.
        try {
            Remove-Item -LiteralPath $strippedServerBinary -Force -ErrorAction SilentlyContinue
            & strip --strip-all -o $strippedServerBinary $serverBinary
            Assert-Exit 'Windows server PE cleanup'
            Copy-Item -LiteralPath $strippedServerBinary -Destination $serverBinary -Force
        } finally {
            Remove-Item -LiteralPath $strippedServerBinary -Force -ErrorAction SilentlyContinue
        }
        & $serverBinary version | Out-Null
        Assert-Exit 'Windows server executable check'
    } finally { $env:CGO_ENABLED = $oldCGO; $env:CC = $oldCC }
    $compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
    & $compiler /nologo /target:winexe /platform:x64 /optimize+ ('/win32icon:' + (Join-Path $repoRoot 'frontend/public/favicon.ico')) ('/win32manifest:' + (Join-Path $repoRoot 'portable/FloatingWindow.manifest')) /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:' + (Join-Path $packageRoot 'TaskTrace-floating.exe')) (Join-Path $repoRoot 'portable/FloatingWindow.cs') (Join-Path $repoRoot 'portable/FloatingInteractions.cs') (Join-Path $repoRoot 'portable/FloatingImageThumbnails.cs') (Join-Path $repoRoot 'portable/FloatingDraftCache.cs') (Join-Path $repoRoot 'portable/FloatingUndo.cs') (Join-Path $repoRoot 'portable/FloatingProgressReferences.cs') (Join-Path $repoRoot 'portable/FloatingProgressEditor.cs') (Join-Path $repoRoot 'portable/FloatingProgressDatePicker.cs') (Join-Path $repoRoot 'portable/FloatingSimpleMode.cs') (Join-Path $repoRoot 'portable/FloatingTaskSurface.cs') (Join-Path $repoRoot 'portable/FloatingReminders.cs') (Join-Path $repoRoot 'portable/FloatingEdgeHide.cs') (Join-Path $repoRoot 'portable/FloatingPriorityFilter.cs') (Join-Path $repoRoot 'portable/FloatingPriorityLinks.cs') (Join-Path $repoRoot 'portable/FloatingSimpleDetails.cs') (Join-Path $repoRoot 'portable/FloatingTaskRefresh.cs') (Join-Path $repoRoot 'portable/FloatingAutoRefresh.cs') (Join-Path $repoRoot 'portable/FloatingUpdates.cs')
    Assert-Exit 'Floating window build'
    & $compiler /nologo /target:winexe /platform:x64 /optimize+ ('/win32icon:' + (Join-Path $repoRoot 'frontend/public/favicon.ico')) ('/win32manifest:' + (Join-Path $repoRoot 'portable/FloatingWindow.manifest')) /reference:System.Windows.Forms.dll ('/out:' + (Join-Path $packageRoot 'TaskTrace.exe')) (Join-Path $repoRoot 'portable/Launcher.cs')
    Assert-Exit 'Launcher build'
    & $compiler /nologo /target:winexe /platform:x64 /optimize+ ('/win32icon:' + (Join-Path $repoRoot 'frontend/public/favicon.ico')) ('/win32manifest:' + (Join-Path $repoRoot 'portable/FloatingWindow.manifest')) /reference:System.Windows.Forms.dll /reference:System.IO.Compression.dll /reference:System.IO.Compression.FileSystem.dll ('/out:' + (Join-Path $packageRoot 'TaskTrace-updater.exe')) (Join-Path $repoRoot 'portable/TaskTraceUpdater.cs')
    Assert-Exit 'Updater build'
    Copy-Item -LiteralPath 'portable/Configure-TaskTrace.cmd','portable/Configure-TaskTrace.ps1','portable/tasktrace-settings.example.json','portable/Launch-TaskTrace.ps1','portable/README.md','LICENSE' -Destination $packageRoot -Force
    '5f3504827990df58bef84b3a5d8c6ab398534c0b' | Set-Content -LiteralPath (Join-Path $packageRoot 'UPSTREAM-COMMIT.txt') -Encoding ASCII
    $sourceCommit = Source-Identifier
    $sourceCommit | Set-Content -LiteralPath (Join-Path $packageRoot 'SOURCE-COMMIT.txt') -Encoding ASCII
    $Version | Set-Content -LiteralPath (Join-Path $packageRoot 'VERSION.txt') -Encoding ASCII
    if (!$SkipArchive) {
        New-Item -ItemType Directory -Path (Join-Path $repoRoot 'Releases') -Force | Out-Null
        $releaseItems = @()
        if ((Test-Path -LiteralPath (Join-Path $repoRoot '.git')) -and (Get-Command git -ErrorAction SilentlyContinue)) {
            $previousTag = (& git -C $repoRoot describe --tags --abbrev=0 HEAD 2>$null)
            if ($LASTEXITCODE -eq 0 -and ![string]::IsNullOrWhiteSpace($previousTag)) {
                $releaseItems = @(& git -C $repoRoot log ($previousTag.Trim() + '..HEAD') --pretty=format:'- %s' --no-merges)
            } else {
                $releaseItems = @(& git -C $repoRoot log -1 --pretty=format:'- %s' --no-merges)
            }
        }
        if ($LASTEXITCODE -ne 0 -or $releaseItems.Count -eq 0) { $releaseItems = @('- 程序更新和问题修复') }
        $releaseNotes = @(
            ('# TaskTrace ' + $Version)
            ''
            ('发布日期：' + (Get-Date -Format 'yyyy-MM-dd'))
            ''
            '## 更新内容'
            ''
        )
        $releaseNotes += $releaseItems
        $releaseNotes += @('', '## 版本对应', '', ('- 源代码提交：' + $sourceCommit))
        $releaseNotes | Set-Content -LiteralPath (Join-Path $repoRoot 'Releases/RELEASE-NOTES.md') -Encoding UTF8
        # Only package public application files. Never include runtime data.
        $packageFiles = @('TaskTrace.exe','TaskTrace-updater.exe','Configure-TaskTrace.cmd','Configure-TaskTrace.ps1','tasktrace-settings.example.json','TaskTrace-server.exe','TaskTrace-floating.exe','Launch-TaskTrace.ps1','README.md','LICENSE','UPSTREAM-COMMIT.txt','SOURCE-COMMIT.txt','VERSION.txt') | ForEach-Object { Join-Path $packageRoot $_ }
        Compress-Archive -LiteralPath $packageFiles -DestinationPath (Join-Path $repoRoot 'Releases/TaskTrace-local-windows-x64.zip') -Force
    }
    # Retire the old entry point when rebuilding an existing package.
    foreach ($obsoleteName in @('Start-Floating.cmd', 'Start-TaskTrace.cmd')) {
        $obsoleteLauncher = Join-Path $packageRoot $obsoleteName
        if (Test-Path -LiteralPath $obsoleteLauncher) { Remove-Item -LiteralPath $obsoleteLauncher -Force }
    }
    Write-Host ('Package ready: ' + $packageRoot)
} finally { Pop-Location }
