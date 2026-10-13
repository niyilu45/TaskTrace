$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$tokens = $null; $errors = $null
$installer = [Management.Automation.Language.Parser]::ParseFile((Join-Path $repoRoot 'dist\Install-TaskTrace-Engine.ps1'), [ref]$tokens, [ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
$helpers = foreach ($name in @('Initialize-InstallProcessInspection', 'Get-InstallProcessImagePath', 'Get-RunningInstallationProcesses', 'Stop-RunningInstallationProcesses')) {
    $definition = $installer.Find({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name }, $true)
    $definition.Extent.Text
}
foreach ($helper in $helpers) { . ([scriptblock]::Create($helper)) }
function Write-InstallLine([string]$Text, $Color) { Write-Host $Text }
$testRoot = Join-Path $env:TEMP ('TaskTrace-install-supervisor-' + [Guid]::NewGuid().ToString('N'))
$children = New-Object 'Collections.Generic.List[Diagnostics.Process]'
$powershell64 = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
$powershell32 = Join-Path $env:WINDIR 'SysWOW64\WindowsPowerShell\v1.0\powershell.exe'
function Wait-File([string]$Path) {
    $until = [DateTime]::UtcNow.AddSeconds(15)
    while (!(Test-Path -LiteralPath $Path)) {
        if ([DateTime]::UtcNow -gt $until) { throw ('Timed out: ' + $Path) }
        Start-Sleep -Milliseconds 20
    }
}
function Start-Hidden([string]$Exe, [string]$Arguments) {
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $Exe; $start.Arguments = $Arguments
    $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
    $p = [Diagnostics.Process]::Start($start)
    $children.Add($p)
    return $p
}
function New-Fixture([string]$Name) {
    $dir = Join-Path $testRoot $Name
    [void][IO.Directory]::CreateDirectory($dir)
    foreach ($name in @('TaskTrace.exe', 'TaskTrace-floating.exe', 'TaskTrace-server.exe')) {
        Copy-Item -LiteralPath (Join-Path $testRoot $name) -Destination $dir
    }
    [IO.File]::WriteAllText((Join-Path $dir 'Launch-TaskTrace.ps1'), $fixtureScript, [Text.UTF8Encoding]::new($true))
    return $dir
}
function Start-Fixture([string]$Dir) {
    $launcher = Start-Hidden (Join-Path $Dir 'TaskTrace.exe') ''
    Wait-File (Join-Path $Dir 'ready')
    $script:fixturePids = [IO.File]::ReadAllText((Join-Path $Dir 'ready')) | ConvertFrom-Json
    foreach ($id in @($fixturePids.hostId, $fixturePids.floating, $fixturePids.server)) { $children.Add([Diagnostics.Process]::GetProcessById($id)) }
    return $launcher
}
try {
    [void][IO.Directory]::CreateDirectory($testRoot)
    # Reuse production job/exit logic without any real data, API or error dialogs.
    $launch = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'Launch-TaskTrace.ps1'))
    $jobStart = $launch.IndexOf('    # A Windows job')
    $jobEnd = $launch.IndexOf('    $stage = ''Start local server''', $jobStart)
    $waitStart = $launch.IndexOf('        $stage = ''Wait for floating window exit''')
    $waitEnd = $launch.IndexOf([char]10 + '    }', $waitStart)
    $fixtureScript = @'
param([switch]$Floating)
$ErrorActionPreference = 'Stop'
__JOB__
$server = [Diagnostics.Process]::Start((Join-Path $PSScriptRoot 'TaskTrace-server.exe'))
$processJob.Attach($server.Handle)
$window = [Diagnostics.Process]::Start((Join-Path $PSScriptRoot 'TaskTrace-floating.exe'))
$processJob.Attach($window.Handle)
[IO.File]::WriteAllText((Join-Path $PSScriptRoot 'ready'), (@{hostId=$PID;floating=$window.Id;server=$server.Id} | ConvertTo-Json))
try {
__WAIT__
} catch {
    [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'startup-failure'), $_.Exception.Message)
    Start-Sleep -Seconds 120
} finally { $processJob.Dispose() }
'@
    $fixtureScript = $fixtureScript.Replace('__JOB__', $launch.Substring($jobStart, $jobEnd - $jobStart)).Replace('__WAIT__', $launch.Substring($waitStart, $waitEnd - $waitStart))
    $compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    $workerSource = Join-Path $testRoot 'worker.cs'
    [IO.File]::WriteAllText($workerSource, 'static class Worker { static void Main() { System.Threading.Thread.Sleep(120000); } }')
    & $compiler /nologo /target:winexe ('/out:' + (Join-Path $testRoot 'TaskTrace-floating.exe')) $workerSource
    if ($LASTEXITCODE) { throw 'Worker compile failed.' }
    Copy-Item -LiteralPath (Join-Path $testRoot 'TaskTrace-floating.exe') -Destination (Join-Path $testRoot 'TaskTrace-server.exe')
    & $compiler /nologo /target:winexe /reference:System.Windows.Forms.dll ('/out:' + (Join-Path $testRoot 'TaskTrace.exe')) (Join-Path $PSScriptRoot 'Launcher.cs')
    if ($LASTEXITCODE) { throw 'Launcher compile failed.' }
    $control = New-Fixture 'control'
    $controlLauncher = Start-Fixture $control
    $controlHostId = $fixturePids.hostId
    $controlFloating = [Diagnostics.Process]::GetProcessById($fixturePids.floating)
    try { $controlFloating.Kill(); $controlFloating.WaitForExit() } finally { $controlFloating.Dispose() }
    Wait-File (Join-Path $control 'startup-failure')
    $controlHost = [Diagnostics.Process]::GetProcessById($controlHostId)
    try { $controlHost.Kill(); $controlHost.WaitForExit() } finally { $controlHost.Dispose() }
    if (!$controlLauncher.WaitForExit(10000)) { throw 'Control launcher did not exit.' }
    Write-Host 'PASS: production wait logic reproduces startup failure when only the floating child is killed.'

    $other = New-Fixture 'other program'
    $otherLauncher = Start-Fixture $other
    foreach ($hostExe in @($powershell64, $powershell32)) {
        $dir = New-Fixture ([Guid]::NewGuid().ToString('N') + ' target')
        $launcher = Start-Fixture $dir
        $ids = $fixturePids
        $helperText = @('param([string]$Directory)', '$ErrorActionPreference = ''Stop''') + $helpers
        $helperText += 'function Write-InstallLine([string]$Text, $Color) { Write-Output $Text }'
        $helperText += 'Stop-RunningInstallationProcesses $Directory'
        $helper = Join-Path $testRoot 'stop.ps1'
        [IO.File]::WriteAllText($helper, ($helperText -join [Environment]::NewLine), [Text.UTF8Encoding]::new($true))
        $stopper = Start-Hidden $hostExe ('-NoProfile -ExecutionPolicy Bypass -File "' + $helper + '" -Directory "' + $dir + '"')
        if (!$stopper.WaitForExit(15000) -or $stopper.ExitCode -ne 0) { throw ('Stop helper failed: ' + $stopper.StandardOutput.ReadToEnd() + $stopper.StandardError.ReadToEnd()) }
        if (!$launcher.WaitForExit(10000)) { throw 'Native launcher survived stop.' }
        foreach ($id in @($ids.hostId, $ids.floating, $ids.server)) {
            if (Get-Process -Id $id -ErrorAction SilentlyContinue) { throw ('Target process survived: ' + $id) }
        }
        if (Test-Path -LiteralPath (Join-Path $dir 'startup-failure')) { throw 'Intentional stop triggered a startup failure.' }
        if ($otherLauncher.HasExited) { throw 'Another installation was stopped.' }
        Write-Host ('PASS: supervisor and job children exit without startup failure; other installation survives (' + $hostExe + ').')
    }
    $pending = Join-Path $testRoot 'pending'
    [void][IO.Directory]::CreateDirectory($pending)
    $pendingScript = Join-Path $pending 'Launch-TaskTrace.ps1'
    [IO.File]::WriteAllText($pendingScript, '[IO.File]::WriteAllText((Join-Path $PSScriptRoot ''ready''), ''ready''); Start-Sleep -Seconds 120')
    $pendingHost = Start-Hidden $powershell64 ('-NoProfile -File "' + $pendingScript + '"')
    Wait-File (Join-Path $pending 'ready')
    Stop-RunningInstallationProcesses $pending
    if (!$pendingHost.WaitForExit(5000)) { throw 'Pending launcher survived.' }
    Write-Host 'PASS: launch can be stopped before any server/floating process exists.'
} finally {
    foreach ($p in $children) { try { if (!$p.HasExited) { $p.Kill(); [void]$p.WaitForExit(5000) } } finally { $p.Dispose() } }
    $resolved = [IO.Path]::GetFullPath($testRoot)
    if (!$resolved.StartsWith(([IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\TaskTrace-install-supervisor-'), [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe test root.' }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
