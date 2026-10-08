$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$installerFile = Join-Path $repoRoot 'dist\Install-TaskTrace-Engine.ps1'
$tokens = $null; $parseErrors = $null
$installer = [Management.Automation.Language.Parser]::ParseFile($installerFile, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
$text = [IO.File]::ReadAllText($installerFile)
# Use the real install entry point and stop exactly when dependency inspection
# would begin. No dependency downloads, compilation or real installation run.
$environmentFunction = $installer.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Import-FreshBuildEnvironment' }, $true)
if ($null -eq $environmentFunction) { throw 'Missing environment inspection entry point.' }
$testText = $text.Remove($environmentFunction.Extent.StartOffset, $environmentFunction.Extent.EndOffset - $environmentFunction.Extent.StartOffset).Insert($environmentFunction.Extent.StartOffset, @'
function Import-FreshBuildEnvironment {
    [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'dependency-stage-reached'), 'reached')
    exit 77
}
'@)
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('TaskTrace-install-preflight-' + [Guid]::NewGuid().ToString('N'))
$dist = Join-Path $testRoot 'dist'
$target = Join-Path $testRoot 'selected program'
$other = Join-Path $testRoot 'another program'
$alias = Join-Path $testRoot 'program alias'
$worker = $null; $locked = $null; $junction = $false
$powershell64 = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
$powershell32 = Join-Path $env:WINDIR 'SysWOW64\WindowsPowerShell\v1.0\powershell.exe'
$scriptFile = Join-Path $dist 'Install-TaskTrace-Engine.ps1'
$marker = Join-Path $dist 'dependency-stage-reached'
function Run-InstallTest([string]$InstallPath, [int]$ExpectedExit, [bool]$ExpectedEnvironment, [string]$HostPath = $powershell64, [string]$ExpectedMessage = '') {
    if (Test-Path -LiteralPath $marker) { Remove-Item -LiteralPath $marker -Force }
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $HostPath
    $start.Arguments = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $scriptFile + '" -InstallDirectory "' + $InstallPath + '"'
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $process = [Diagnostics.Process]::Start($start)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (!$process.WaitForExit(20000)) { $process.Kill(); throw 'Installer preflight timed out.' }
        $output = $stdout.Result + $stderr.Result
        if ($process.ExitCode -ne $ExpectedExit) { throw ('Unexpected installer exit ' + $process.ExitCode + ': ' + $output) }
        if ((Test-Path -LiteralPath $marker) -ne $ExpectedEnvironment) { throw ('Wrong dependency-stage ordering: ' + $output) }
        $log = [IO.File]::ReadAllText((Join-Path $dist 'install.log'))
        if ($ExpectedMessage -and !$log.Contains($ExpectedMessage)) { throw ('Missing preflight explanation: ' + $log) }
        if (Test-Path -LiteralPath (Join-Path $dist 'TaskTrace-program-files.zip')) { throw 'Preflight unexpectedly produced an installation archive.' }
    } finally { $process.Dispose() }
}
try {
    foreach ($directory in @($dist, $target, $other)) { [void][IO.Directory]::CreateDirectory($directory) }
    [IO.File]::WriteAllText($scriptFile, $testText, [Text.UTF8Encoding]::new($true))
    $source = Join-Path $testRoot 'worker.cs'
    [IO.File]::WriteAllText($source, @'
using System;
using System.IO;
using System.Threading;
static class InstallProbeWorker {
    static void Main(string[] args) {
        File.WriteAllText(args[0], "ready");
        while(!File.Exists(args[1])) Thread.Sleep(20);
    }
}
'@)
    $executable = Join-Path $target 'TaskTrace-server.exe'
    $compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    & $compiler /nologo /target:winexe /platform:x64 ('/out:' + $executable) $source
    if ($LASTEXITCODE -ne 0) { throw 'Could not compile isolated preflight worker.' }
    $originalHash = (Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash
    $ready = Join-Path $testRoot 'worker.ready'
    $release = Join-Path $testRoot 'worker.release'
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $executable
    $start.Arguments = '"' + $ready + '" "' + $release + '"'
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $worker = [Diagnostics.Process]::Start($start)
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    while (!(Test-Path -LiteralPath $ready)) {
        if ($worker.HasExited -or [DateTime]::UtcNow -gt $deadline) { throw 'Isolated process failed to start.' }
        Start-Sleep -Milliseconds 20
    }
    Run-InstallTest $target 2 $false $powershell64 '正在运行'
    Run-InstallTest $target 2 $false $powershell32 '正在运行'
    Write-Host 'PASS: both x64 and x86 installers detect the x64 running target before dependency inspection.'
    New-Item -ItemType Junction -Path $alias -Target $target | Out-Null
    $junction = $true
    Run-InstallTest $alias 2 $false $powershell64 '正在运行'
    Write-Host 'PASS: an alternate junction path still identifies the running installation.'
    Run-InstallTest $other 77 $true
    Write-Host 'PASS: a running copy in another known directory does not block this installation.'

    # Model an access-denied process query for this test worker only.
    $query = $installer.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Get-InstallProcessImagePath' }, $true)
    $deniedQuery = 'function Get-InstallProcessImagePath([Diagnostics.Process]$Process) { if ($Process.Id -eq ' + $worker.Id + ') { return $null }; Initialize-InstallProcessInspection; return [TaskTraceInstallNative]::ProcessPath($Process.Id) }'
    [IO.File]::WriteAllText($scriptFile, $testText.Replace($query.Extent.Text, $deniedQuery), [Text.UTF8Encoding]::new($true))
    Run-InstallTest $target 2 $false $powershell64 '无法确认运行路径'
    [IO.File]::WriteAllText($scriptFile, $testText, [Text.UTF8Encoding]::new($true))
    Write-Host 'PASS: an unreadable process path is reported before any dependency/build work.'

    [IO.File]::WriteAllText($release, 'release')
    if (!$worker.WaitForExit(10000)) { throw 'Isolated process did not stop.' }
    $worker.Dispose(); $worker = $null
    $locked = [IO.File]::Open($executable, 'Open', 'Read', 'Read')
    Run-InstallTest $target 2 $false $powershell64 '程序文件被占用'
    $locked.Dispose(); $locked = $null
    Write-Host 'PASS: target file locks are rejected early even with no matching process.'
    Run-InstallTest $target 77 $true
    if ((Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash -ne $originalHash) { throw 'Preflight changed the installed program.' }
    Write-Host 'PASS: after exit/unlock, the target reaches dependency inspection with its bytes unchanged.'
} finally {
    if ($null -ne $locked) { $locked.Dispose() }
    if ($null -ne $worker) {
        if (!$worker.HasExited) { $worker.Kill(); $worker.WaitForExit() }
        $worker.Dispose()
    }
    $resolvedRoot = [IO.Path]::GetFullPath($testRoot)
    $expectedPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\TaskTrace-install-preflight-'
    if (!$resolvedRoot.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe temporary test directory.' }
    # Remove the junction itself, never recurse through its target.
    if ($junction) { [IO.Directory]::Delete($alias) }
    if (Test-Path -LiteralPath $resolvedRoot) { Remove-Item -LiteralPath $resolvedRoot -Recurse -Force }
}
