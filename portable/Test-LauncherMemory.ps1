param(
    [string]$ServerBinary = 'dist/TaskTrace-local/TaskTrace-server.exe',
    [string]$BaselineCommit
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
if (![IO.Path]::IsPathRooted($ServerBinary)) { $ServerBinary = Join-Path $repoRoot $ServerBinary }
$ServerBinary = (Resolve-Path -LiteralPath $ServerBinary).Path
$testRoot = Join-Path $repoRoot ('.local-build\launcher-memory-tests\' + [Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
$compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
$children = New-Object 'Collections.Generic.List[Diagnostics.Process]'

function Wait-Until([scriptblock]$Condition, [string]$Failure) {
    $timer = [Diagnostics.Stopwatch]::StartNew()
    while (!(& $Condition)) {
        if ($timer.Elapsed.TotalSeconds -gt 75) { throw $Failure }
        Start-Sleep -Milliseconds 100
    }
}

function Measure-ProcessTree([int]$RootId) {
    $all = @(Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name)
    $queue = [Collections.Generic.Queue[int]]::new()
    $seen = [Collections.Generic.HashSet[int]]::new()
    $queue.Enqueue($RootId)
    while ($queue.Count -gt 0) {
        $id = $queue.Dequeue()
        if (!$seen.Add($id)) { continue }
        foreach ($child in $all) { if ($child.ParentProcessId -eq $id) { $queue.Enqueue($child.ProcessId) } }
        $process = Get-Process -Id $id -ErrorAction SilentlyContinue
        if ($null -eq $process) { continue }
        try {
            [pscustomobject]@{
                Id = $process.Id; Name = $process.ProcessName
                PrivateMiB = [math]::Round($process.PrivateMemorySize64 / 1MB, 2)
                WorkingMiB = [math]::Round($process.WorkingSet64 / 1MB, 2)
            }
        } finally { $process.Dispose() }
    }
}

function New-TestInstance([string]$Name, [string]$LauncherSource, [string]$LaunchScript) {
    $root = Join-Path $testRoot $Name
    [void][IO.Directory]::CreateDirectory($root)
    Copy-Item -LiteralPath $ServerBinary -Destination (Join-Path $root 'TaskTrace-server.exe')
    Copy-Item -LiteralPath (Join-Path $testRoot 'TaskTrace-floating.exe'), $LaunchScript -Destination $root
    & $compiler /nologo /target:winexe /platform:x64 /reference:System.Windows.Forms.dll /reference:System.Web.Extensions.dll ('/out:' + (Join-Path $root 'TaskTrace.exe')) $LauncherSource
    if ($LASTEXITCODE) { throw 'Launcher compile failed.' }
    return $root
}

function Start-TestInstance([string]$Root) {
    $start = [Diagnostics.ProcessStartInfo]::new((Join-Path $Root 'TaskTrace.exe'), '--self-test')
    $start.WorkingDirectory = $Root
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $launcher = [Diagnostics.Process]::Start($start)
    $children.Add($launcher)
    $ready = Join-Path $Root 'data\test-ready'
    Wait-Until { $launcher.HasExited -or (Test-Path -LiteralPath $ready) } "Isolated launcher did not become ready in $Root"
    if ($launcher.HasExited) {
        $logs = @('TaskTrace-startup-error.log','server-error.log','workspace-setup.log') | ForEach-Object {
            $file = Join-Path $Root ('data\' + $_)
            if (Test-Path -LiteralPath $file) { Get-Content -LiteralPath $file -Tail 6 }
        }
        throw "Launcher exited with $($launcher.ExitCode) in $Root. $($logs -join ' ')"
    }
    return $launcher
}

function Assert-WorkspaceLocked([string]$Root, [bool]$Expected) {
    $path = Join-Path $Root 'data\session.lock'
    $file = $null
    try { $file = [IO.File]::Open($path, 'Open', 'ReadWrite', 'None') } catch [IO.IOException] {
        if (($_.Exception.HResult -band 65535) -notin @(32, 33)) { throw }
    }
    if ($null -ne $file) { $file.Dispose() }
    if (($null -eq $file) -ne $Expected) { throw 'Unexpected data-directory lock state.' }
}

function Assert-ChildExit($Members, [int]$LauncherId) {
    foreach ($member in $Members) {
        if ($member.Id -eq $LauncherId) { continue }
        Wait-Until { $null -eq (Get-Process -Id $member.Id -ErrorAction SilentlyContinue) } "Child $($member.Id) survived launcher exit."
    }
}

try {
    $workerSource = Join-Path $testRoot 'worker.cs'
    [IO.File]::WriteAllText($workerSource, @'
using System;
using System.IO;
using System.Threading;
static class Worker {
    static int Main(string[] args) {
        if (args.Length == 2) {
            File.WriteAllLines(Path.Combine(args[1], "update-test-result"), new[] {
                args[0], args[1], typeof(Worker).Assembly.Location, ProcessId().ToString()
            });
            return 0;
        }
        if (args.Length != 4 || args[3] != "--self-test") return 3;
        string data = Path.GetFullPath(args[2]);
        File.WriteAllText(Path.Combine(data, "test-ready"), args[0]);
        string exit = Path.Combine(data, "test-exit");
        for (int i = 0; i < 900; i++) {
            if (File.Exists(exit)) return Int32.Parse(File.ReadAllText(exit));
            Thread.Sleep(100);
        }
        return 4;
    }
    static int ProcessId() {
        using (var process = System.Diagnostics.Process.GetCurrentProcess()) return process.Id;
    }
}
'@)
    & $compiler /nologo /target:winexe /platform:x64 ('/out:' + (Join-Path $testRoot 'TaskTrace-floating.exe')) $workerSource
    if ($LASTEXITCODE) { throw 'Floating fixture compile failed.' }

    if ($BaselineCommit) {
        $baselineRoot = Join-Path $testRoot 'baseline source'
        [void][IO.Directory]::CreateDirectory($baselineRoot)
        foreach ($name in @('Launcher.cs', 'Launch-TaskTrace.ps1')) {
            $source = & git -C $repoRoot show ($BaselineCommit + ':portable/' + $name)
            if ($LASTEXITCODE) { throw 'Cannot read baseline source.' }
            [IO.File]::WriteAllText((Join-Path $baselineRoot $name), ($source -join "`r`n").TrimStart([char]0xfeff), [Text.UTF8Encoding]::new($true))
        }
        $baseline = New-TestInstance 'baseline' (Join-Path $baselineRoot 'Launcher.cs') (Join-Path $baselineRoot 'Launch-TaskTrace.ps1')
        $old = Start-TestInstance $baseline
        $baselineMembers = @(Measure-ProcessTree $old.Id)
        if (!($baselineMembers.Name -contains 'powershell')) { throw 'Baseline PowerShell supervisor was not observed.' }
        Write-Output 'Baseline processes'
        $baselineMembers | Format-Table -AutoSize | Out-String | Write-Output
        $baselinePrivate = ($baselineMembers.PrivateMiB | Measure-Object -Sum).Sum
        [IO.File]::WriteAllText((Join-Path $baseline 'data\test-exit'), '0')
        if (!$old.WaitForExit(15000) -or $old.ExitCode -ne 0) { throw 'Baseline did not close cleanly.' }
        Assert-ChildExit $baselineMembers $old.Id
    }

    $root = New-TestInstance '新版 TaskTrace' (Join-Path $PSScriptRoot 'Launcher.cs') (Join-Path $PSScriptRoot 'Launch-TaskTrace.ps1')
    $app = Start-TestInstance $root
    $members = @(Measure-ProcessTree $app.Id)
    if (@($members | Where-Object Name -Like 'TaskTrace*').Count -ne 3 -or $members.Name -contains 'powershell' -or
        !($members.Name -contains 'TaskTrace-server') -or !($members.Name -contains 'TaskTrace-floating')) {
        throw "Expected native launcher, server and floating child without a PowerShell host: $($members.Name -join ', ')"
    }
    Assert-WorkspaceLocked $root $true
    $apiUrl = [IO.File]::ReadAllText((Join-Path $root 'data\test-ready'))
    $http = New-Object Net.WebClient
    try {
        $http.Proxy = $null
        if (!(($http.DownloadString($apiUrl + '/api/v1/info') | ConvertFrom-Json).version)) { throw 'Server API not available.' }
    } finally { $http.Dispose() }
    Write-Output 'Current processes'
    $members | Format-Table -AutoSize | Out-String | Write-Output
    $currentPrivate = ($members.PrivateMiB | Measure-Object -Sum).Sum
    if ($BaselineCommit) {
        Write-Output ("Private memory, same server and floating fixture: {0:N1} -> {1:N1} MiB (saved {2:N1} MiB)." -f $baselinePrivate, $currentPrivate, ($baselinePrivate - $currentPrivate))
    }
    [IO.File]::WriteAllText((Join-Path $root 'data\test-exit'), '0')
    if (!$app.WaitForExit(15000) -or $app.ExitCode -ne 0) { throw 'Native launcher did not close cleanly.' }
    Assert-ChildExit $members $app.Id
    Assert-WorkspaceLocked $root $false

    Remove-Item -LiteralPath (Join-Path $root 'data\test-ready'),(Join-Path $root 'data\test-exit')
    $restarted = Start-TestInstance $root
    $restartMembers = @(Measure-ProcessTree $restarted.Id)
    $restarted.Kill()
    $restarted.WaitForExit()
    Assert-ChildExit $restartMembers $restarted.Id
    Assert-WorkspaceLocked $root $false
    Write-Output 'PASS: short-lived PowerShell, live server, continuous data lock, clean restart and child cleanup on both normal and forced exit.'

    Remove-Item -LiteralPath (Join-Path $root 'data\test-ready')
    $handoff = Start-TestInstance $root
    $handoffMembers = @(Measure-ProcessTree $handoff.Id)
    Copy-Item -LiteralPath (Join-Path $testRoot 'TaskTrace-floating.exe') -Destination (Join-Path $root 'TaskTrace-updater.exe')
    $archive = Join-Path $root 'fixture archive.zip'
    $request = @{archive = $archive; install_root = $root} | ConvertTo-Json -Compress
    $requestPath = Join-Path $root 'data\update-ready.json'
    [IO.File]::WriteAllText($requestPath, $request)
    [IO.File]::WriteAllText((Join-Path $root 'data\test-exit'), '10')
    if (!$handoff.WaitForExit(15000) -or $handoff.ExitCode -ne 0) { throw 'Update handoff failed.' }
    Assert-ChildExit $handoffMembers $handoff.Id
    Assert-WorkspaceLocked $root $false
    $updateResult = Join-Path $root 'update-test-result'
    Wait-Until { Test-Path -LiteralPath $updateResult } 'The updater did not receive its launch arguments.'
    $received = [IO.File]::ReadAllLines($updateResult)
    if ($received.Count -ne 4 -or $received[0] -ne $archive -or $received[1] -ne $root -or (Test-Path -LiteralPath $requestPath)) {
        throw 'The updater did not receive the expected paths or the update request was not consumed.'
    }
    $updaterId = [int]$received[3]
    Wait-Until { $null -eq (Get-Process -Id $updaterId -ErrorAction SilentlyContinue) } 'The updater fixture did not exit.'
    $updater = [IO.Path]::GetFullPath($received[2])
    if ([IO.Path]::GetDirectoryName($updater).TrimEnd('\') -ne [IO.Path]::GetTempPath().TrimEnd('\') -or
        [IO.Path]::GetFileName($updater) -notmatch '^TaskTrace-updater-[a-f0-9]{32}\.exe$') { throw 'Unexpected updater fixture path.' }
    Remove-Item -LiteralPath $updater
    Write-Output 'PASS: update handoff runs outside the application job after data and server shutdown.'
    Write-Output ('Results: ' + $testRoot)
} finally {
    if ($old -and !$old.HasExited -and (Test-Path -LiteralPath (Join-Path $baseline 'data'))) {
        [IO.File]::WriteAllText((Join-Path $baseline 'data\test-exit'), '0')
        [void]$old.WaitForExit(5000)
    }
    foreach ($child in $children) {
        try { if (!$child.HasExited) { $child.Kill(); [void]$child.WaitForExit(5000) } }
        finally { $child.Dispose() }
    }
}
