param([string]$OutputDirectory = '.local-build/startup-guard-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'Startup guard test output must remain inside the repository .local-build directory.'
}
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
$tokens = $null; $parseErrors = $null
$launchAst = [Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot 'Launch-TaskTrace.ps1'), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
$helpers = foreach ($name in @('Enter-TaskTraceInstanceLock', 'Test-TaskTraceRunningProcess')) {
    $functionAst = $launchAst.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $false)
    if ($null -eq $functionAst) { throw ('Missing production helper: ' + $name) }
    $functionAst.Extent.Text
}
# Extract only production helpers. Never execute the application's main script.
$workerScript = @'
param([string]$MutexName, [string]$ResultFile, [string]$GateFile, [string]$ReleaseFile)
$ErrorActionPreference = 'Stop'
__HELPERS__
$held = $null
try {
    [IO.File]::WriteAllText(($ResultFile + '.ready'), 'ready')
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    while (!(Test-Path -LiteralPath $GateFile)) {
        if ([DateTime]::UtcNow -gt $deadline) { throw 'Timed out waiting for the test start gate.' }
        Start-Sleep -Milliseconds 20
    }
    $held = Enter-TaskTraceInstanceLock -Name $MutexName
    $resultJson = @{ acquired = ($null -ne $held); pid = $PID } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText(($ResultFile + '.tmp'), $resultJson, [Text.UTF8Encoding]::new($false))
    [IO.File]::Move(($ResultFile + '.tmp'), $ResultFile)
    if ($null -ne $held) {
        $deadline = [DateTime]::UtcNow.AddSeconds(30)
        while (!(Test-Path -LiteralPath $ReleaseFile)) {
            if ([DateTime]::UtcNow -gt $deadline) { throw 'Timed out waiting for the test release signal.' }
            Start-Sleep -Milliseconds 20
        }
    }
} catch {
    [IO.File]::WriteAllText(($ResultFile + '.error'), $_.Exception.ToString())
    exit 1
} finally {
    if ($null -ne $held) { $held.ReleaseMutex(); $held.Dispose() }
}
exit 0
'@
$workerScript = $workerScript.Replace('__HELPERS__', ($helpers -join [Environment]::NewLine))
$workerFile = Join-Path $testRoot 'worker.ps1'
[IO.File]::WriteAllText($workerFile, $workerScript, [Text.UTF8Encoding]::new($true))
$powershell = Join-Path $env:WINDIR 'System32/WindowsPowerShell/v1.0/powershell.exe'
$children = New-Object 'Collections.Generic.List[Diagnostics.Process]'
$assertions = New-Object 'Collections.Generic.List[string]'
$abandonedHandle = $null
function Wait-TestFile([string]$Path) {
    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    while (!(Test-Path -LiteralPath $Path)) {
        if (Test-Path -LiteralPath ($Path + '.error')) { throw ([IO.File]::ReadAllText($Path + '.error')) }
        if ([DateTime]::UtcNow -gt $deadline) { throw ('Timed out waiting for test result: ' + $Path) }
        Start-Sleep -Milliseconds 20
    }
}
function Start-TestWorker([string]$Case, [string]$MutexName, [string]$GateFile, [string]$ReleaseFile) {
    $result = Join-Path $testRoot ($Case + '.json')
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $powershell
    $start.Arguments = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $workerFile + '" -MutexName "' + $MutexName + '" -ResultFile "' + $result + '" -GateFile "' + $GateFile + '" -ReleaseFile "' + $ReleaseFile + '"'
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.WorkingDirectory = $testRoot
    $process = [Diagnostics.Process]::Start($start)
    if ($null -eq $process) { throw 'Could not start an isolated guard test worker.' }
    $children.Add($process)
    return [pscustomobject]@{ Process = $process; Result = $result }
}
function Read-TestResult($Worker) {
    Wait-TestFile $Worker.Result
    return ([IO.File]::ReadAllText($Worker.Result) | ConvertFrom-Json)
}
function Wait-TestExit($Worker) {
    if (!$Worker.Process.WaitForExit(10000)) { throw 'Test worker failed to exit.' }
    if ($Worker.Process.ExitCode -ne 0) {
        if (Test-Path -LiteralPath ($Worker.Result + '.error')) { throw ([IO.File]::ReadAllText($Worker.Result + '.error')) }
        throw ('Guard test worker exited with code ' + $Worker.Process.ExitCode)
    }
}
try {
    $prefix = 'Local\TaskTrace.StartupGuard.Test.' + [Guid]::NewGuid().ToString('N')
    $gate = Join-Path $testRoot 'open.gate'; [IO.File]::WriteAllText($gate, 'go')
    $release = Join-Path $testRoot 'serial.release'
    $first = Start-TestWorker 'first-owner' ($prefix + '.serial') $gate $release
    if (!(Read-TestResult $first).acquired) { throw 'First launch failed to acquire the lock.' }
    $second = Start-TestWorker 'second-rejected' ($prefix + '.serial') $gate $release
    if ((Read-TestResult $second).acquired) { throw 'Duplicate launch acquired a held lock.' }
    Wait-TestExit $second
    [IO.File]::WriteAllText($release, 'release'); Wait-TestExit $first
    $restart = Start-TestWorker 'normal-restart' ($prefix + '.serial') $gate $release
    if (!(Read-TestResult $restart).acquired) { throw 'Lock remained held after normal exit.' }
    Wait-TestExit $restart
    $assertions.Add('PASS: first launch succeeds, duplicate is rejected, normal exit permits restart.')

    $abandonRelease = Join-Path $testRoot 'abandoned.release'
    $owner = Start-TestWorker 'abandoned-owner' ($prefix + '.abandoned') $gate $abandonRelease
    if (!(Read-TestResult $owner).acquired) { throw 'Abandonment test could not acquire its lock.' }
    # Keep the kernel object alive so this exercises an abandoned, not recreated, mutex.
    $abandonedHandle = [Threading.Mutex]::OpenExisting($prefix + '.abandoned')
    $owner.Process.Kill(); $owner.Process.WaitForExit()
    [IO.File]::WriteAllText($abandonRelease, 'release')
    $recovery = Start-TestWorker 'abandoned-recovery' ($prefix + '.abandoned') $gate $abandonRelease
    if (!(Read-TestResult $recovery).acquired) { throw 'Abandoned mutex prevented restart.' }
    Wait-TestExit $recovery
    $abandonedHandle.Dispose(); $abandonedHandle = $null
    $assertions.Add('PASS: interrupted owner permits restart while its abandoned mutex still exists.')

    $parallelGate = Join-Path $testRoot 'parallel.gate'
    $parallelRelease = Join-Path $testRoot 'parallel.release'
    $parallel = @(foreach ($index in 1..4) { Start-TestWorker ('parallel-' + $index) ($prefix + '.parallel') $parallelGate $parallelRelease })
    foreach ($worker in $parallel) { Wait-TestFile ($worker.Result + '.ready') }
    [IO.File]::WriteAllText($parallelGate, 'go')
    $results = @(foreach ($worker in $parallel) { Read-TestResult $worker })
    if (@($results | Where-Object { $_.acquired }).Count -ne 1) { throw 'Simultaneous launches did not produce exactly one lock owner.' }
    [IO.File]::WriteAllText($parallelRelease, 'release')
    foreach ($worker in $parallel) { Wait-TestExit $worker }
    $assertions.Add('PASS: four simultaneous launches produce exactly one owner and three refusals.')
    $report = ($assertions -join [Environment]::NewLine) + [Environment]::NewLine
    [IO.File]::WriteAllText((Join-Path $testRoot 'result.txt'), $report, [Text.UTF8Encoding]::new($false))
    Write-Output $report
    Write-Output ('Results: ' + $testRoot)
} finally {
    if ($null -ne $abandonedHandle) { $abandonedHandle.Dispose() }
    # Only stop hidden workers created above, never existing TaskTrace processes.
    foreach ($child in $children) {
        try { if (!$child.HasExited) { $child.Kill(); $child.WaitForExit(5000) | Out-Null } } finally { $child.Dispose() }
    }
}
