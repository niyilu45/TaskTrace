param([switch]$SkipFrontend, [string]$PackageDirectory = 'Releases/TaskTrace-local')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$packageRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $PackageDirectory))
function Assert-Exit([string]$Step) {
    if ($LASTEXITCODE -ne 0) { throw "$Step failed with exit code $LASTEXITCODE" }
}
Push-Location $repoRoot
try {
    if (!$SkipFrontend) {
        $nodeVersion = & node -p 'process.versions.node'
        Assert-Exit 'Node version check'
        if ([int]($nodeVersion.Split('.')[0]) -lt 24) { throw 'Building requires Node.js 24 or newer. Running the finished package does not.' }
        $oldLocalMode = $env:VITE_TASKTRACE_LOCAL
        Push-Location (Join-Path $repoRoot 'frontend')
        try {
            & pnpm install --frozen-lockfile
            Assert-Exit 'Frontend dependency installation'
            $env:VITE_TASKTRACE_LOCAL = 'true'
            & pnpm run build
            Assert-Exit 'Frontend build'
        } finally { $env:VITE_TASKTRACE_LOCAL = $oldLocalMode; Pop-Location }
    }
    if (!(Test-Path -LiteralPath 'frontend/dist/index.html')) { throw 'Build frontend/dist first.' }
    New-Item -ItemType Directory -Path $packageRoot -Force | Out-Null
    $oldCGO = $env:CGO_ENABLED
    $oldCC = $env:CC
    try {
        $env:CGO_ENABLED = '1'
        $env:CC = 'gcc'
        & go build -tags 'osusergo,timetzdata' -ldflags '-s -w -linkmode external -extldflags "-static" -X code.vikunja.io/api/pkg/version.Version=tasktrace-local' -o (Join-Path $packageRoot 'TaskTrace-server.exe') .
        Assert-Exit 'Windows server build'
    } finally { $env:CGO_ENABLED = $oldCGO; $env:CC = $oldCC }
    $compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
    & $compiler /nologo /target:winexe /platform:x64 /optimize+ ('/win32icon:' + (Join-Path $repoRoot 'frontend/public/favicon.ico')) ('/win32manifest:' + (Join-Path $repoRoot 'portable/FloatingWindow.manifest')) /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:' + (Join-Path $packageRoot 'TaskTrace-floating.exe')) (Join-Path $repoRoot 'portable/FloatingWindow.cs')
    Assert-Exit 'Floating window build'
    & $compiler /nologo /target:winexe /platform:x64 /optimize+ ('/win32icon:' + (Join-Path $repoRoot 'frontend/public/favicon.ico')) ('/win32manifest:' + (Join-Path $repoRoot 'portable/FloatingWindow.manifest')) /reference:System.Windows.Forms.dll ('/out:' + (Join-Path $packageRoot 'TaskTrace.exe')) (Join-Path $repoRoot 'portable/Launcher.cs')
    Assert-Exit 'Launcher build'
    Copy-Item -LiteralPath 'portable/Configure-TaskTrace.cmd','portable/Configure-TaskTrace.ps1','portable/tasktrace-settings.example.json','portable/Start-TaskTrace.cmd','portable/Launch-TaskTrace.ps1','portable/README.md','LICENSE' -Destination $packageRoot -Force
    '5f3504827990df58bef84b3a5d8c6ab398534c0b' | Set-Content -LiteralPath (Join-Path $packageRoot 'UPSTREAM-COMMIT.txt') -Encoding ASCII
    New-Item -ItemType Directory -Path (Join-Path $repoRoot 'Releases') -Force | Out-Null
    & git rev-parse HEAD | Set-Content -LiteralPath (Join-Path $packageRoot 'SOURCE-COMMIT.txt') -Encoding ASCII
    Assert-Exit 'Source revision'
    # Only package public application files. Never include runtime data.
    $packageFiles = @('TaskTrace.exe','Configure-TaskTrace.cmd','Configure-TaskTrace.ps1','tasktrace-settings.example.json','TaskTrace-server.exe','TaskTrace-floating.exe','Start-TaskTrace.cmd','Launch-TaskTrace.ps1','README.md','LICENSE','UPSTREAM-COMMIT.txt','SOURCE-COMMIT.txt') | ForEach-Object { Join-Path $packageRoot $_ }
    Compress-Archive -LiteralPath $packageFiles -DestinationPath (Join-Path $repoRoot 'Releases/TaskTrace-local-windows-x64.zip') -Force
    # Retire the old entry point when rebuilding an existing package.
    $obsoleteLauncher = Join-Path $packageRoot 'Start-Floating.cmd'
    if (Test-Path -LiteralPath $obsoleteLauncher) { Remove-Item -LiteralPath $obsoleteLauncher -Force }
    Write-Host ('Package ready: ' + $packageRoot)
} finally { Pop-Location }
