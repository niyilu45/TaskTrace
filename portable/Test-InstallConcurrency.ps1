$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$sourceScript = Join-Path $repoRoot 'dist\Install-TaskTrace-Engine.ps1'
$tokens = $null; $errors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($sourceScript, [ref]$tokens, [ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
$template = [IO.File]::ReadAllText($sourceScript)
function Replace-Function([string]$Text, [string]$Name, [scriptblock]$Replacement) {
    $definition = $ast.Find({ param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $Name }, $true)
    if ($null -eq $definition) { throw ('Missing helper: ' + $Name) }
    return $Text.Replace($definition.Extent.Text, $Replacement.ToString().Trim())
}
$template = Replace-Function $template 'Show-InstallResult' {
    function Show-InstallResult([string]$Title, [string]$Message, [bool]$IsError = $false) {
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot ($PID.ToString() + '.result')), $Message)
    }
}
$template = Replace-Function $template 'Confirm-StopRunningInstallation' {
    function Confirm-StopRunningInstallation([string]$Directory, [object[]]$Processes) {
        throw 'A competing install must not reach the stop prompt.'
    }
}
$template = Replace-Function $template 'Import-FreshBuildEnvironment' {
    function Import-FreshBuildEnvironment {
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot ($PID.ToString() + '.ready')), $outputDirectory)
        $until = [DateTime]::UtcNow.AddSeconds(90)
        while (!(Test-Path -LiteralPath (Join-Path $PSScriptRoot 'release'))) {
            if ([DateTime]::UtcNow -gt $until) { throw 'Timed out at the isolated build boundary.' }
            Start-Sleep -Milliseconds 20
        }
        # Exercise the real transactional copy under both directory locks.
        Install-ProgramFiles (Join-Path $PSScriptRoot 'program') $outputDirectory @('VERSION.txt')
        exit 77
    }
}
$testRoot = Join-Path $env:TEMP ('TaskTrace-install-concurrency-' + [Guid]::NewGuid().ToString('N'))
$children = New-Object 'Collections.Generic.List[Diagnostics.Process]'
$junctions = New-Object 'Collections.Generic.List[string]'
$powershell64 = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
$powershell32 = Join-Path $env:WINDIR 'SysWOW64\WindowsPowerShell\v1.0\powershell.exe'
function New-Source([string]$Name) {
    $dist = Join-Path (Join-Path $testRoot $Name) 'dist'
    [void][IO.Directory]::CreateDirectory((Join-Path $dist 'program'))
    [IO.File]::WriteAllText((Join-Path $dist 'program\VERSION.txt'), $Name)
    [IO.File]::WriteAllText((Join-Path $dist 'Install-TaskTrace-Engine.ps1'), $template, [Text.UTF8Encoding]::new($true))
    Copy-Item -LiteralPath (Join-Path $repoRoot 'dist\Install-TaskTrace.cmd') -Destination $dist
    return $dist
}
function Start-Installer([string]$Dist, [string]$Target, [switch]$Wrapper, [switch]$X86) {
    $start = New-Object Diagnostics.ProcessStartInfo
    if ($Wrapper) {
        $start.FileName = $env:ComSpec
        $start.Arguments = '/d /c ""' + (Join-Path $Dist 'Install-TaskTrace.cmd') + '" -InstallDirectory "' + $Target + '""'
    } else {
        $start.FileName = if ($X86) { $powershell32 } else { $powershell64 }
        $start.Arguments = '-NoProfile -ExecutionPolicy Bypass -File "' + (Join-Path $Dist 'Install-TaskTrace-Engine.ps1') + '" -Interactive -InstallDirectory "' + $Target + '"'
    }
    $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true; $start.RedirectStandardInput = $true
    $p = [Diagnostics.Process]::Start($start)
    $children.Add($p)
    return [pscustomobject]@{ Process=$p; Dist=$Dist; Out=$p.StandardOutput.ReadToEndAsync(); Err=$p.StandardError.ReadToEndAsync() }
}
function Wait-Ready($Worker) {
    $path = Join-Path $Worker.Dist ($Worker.Process.Id.ToString() + '.ready')
    $until = [DateTime]::UtcNow.AddSeconds(15)
    while (!(Test-Path -LiteralPath $path)) {
        if ($Worker.Process.HasExited) { throw ('Failed to reach build boundary: ' + $Worker.Out.Result + $Worker.Err.Result) }
        if ([DateTime]::UtcNow -gt $until) { throw 'Timed out waiting for build boundary.' }
        Start-Sleep -Milliseconds 20
    }
}
function Wait-Exit($Worker, [int]$Code) {
    if (!$Worker.Process.WaitForExit(15000)) { throw 'Installer did not exit automatically.' }
    $output = $Worker.Out.Result + $Worker.Err.Result
    if ($Worker.Process.ExitCode -ne $Code) { throw ('Unexpected exit ' + $Worker.Process.ExitCode + ': ' + $output) }
    if ($Code -eq 1223 -and $output -match 'Press any key|Build failed|Installation completed') { throw 'Duplicate wrapper waited for another key.' }
    return $output
}
function Assert-Refused($Worker) {
    $output = Wait-Exit $Worker 1223
    # The x86 host uses the Windows console code page; inspect the captured UI
    # message as Unicode rather than guessing redirected stdout's encoding.
    $messages = @(Get-ChildItem -LiteralPath $Worker.Dist -Filter '*.result' | ForEach-Object { [IO.File]::ReadAllText($_.FullName) })
    if (!($messages | Where-Object { $_ -match '已有 install 正在运行' })) { throw ('Missing conflict explanation: ' + $output) }
    if (Test-Path -LiteralPath (Join-Path $Worker.Dist ($Worker.Process.Id.ToString() + '.ready'))) { throw 'Duplicate reached dependency/build work.' }
}
function Release-Installer($Worker) {
    [IO.File]::WriteAllText((Join-Path $Worker.Dist 'release'), 'release')
    $null = Wait-Exit $Worker 77
}
try {
    [void][IO.Directory]::CreateDirectory($testRoot)
    $a = New-Source 'source a'
    $b = New-Source 'source b'
    $c = New-Source 'source c'
    $parent = Join-Path $testRoot 'targets'
    [void][IO.Directory]::CreateDirectory($parent)
    $alias = Join-Path $testRoot 'target alias'
    New-Item -ItemType Junction -Path $alias -Target $parent | Out-Null
    $junctions.Add($alias)
    # Target deliberately does not exist yet; both paths must acquire one lock.
    $target = Join-Path $parent 'new target'
    $first = Start-Installer $a $target
    Wait-Ready $first
    $logHash = (Get-FileHash -LiteralPath (Join-Path $a 'install.log')).Hash
    Assert-Refused (Start-Installer $a (Join-Path $parent 'different target') -Wrapper)
    if ((Get-FileHash -LiteralPath (Join-Path $a 'install.log')).Hash -ne $logHash) { throw 'Duplicate overwrote the active installer log.' }
    Write-Host 'PASS: duplicate source exits its real command wrapper before work without touching the active log.'
    Assert-Refused (Start-Installer $b (Join-Path $alias 'new target') -X86)
    if (Test-Path -LiteralPath $target) { throw 'Rejected installer created or wrote the destination.' }
    Write-Host 'PASS: another source and x86 host cannot install to the same future directory through a junction.'
    $independent = Start-Installer $c (Join-Path $parent 'independent target')
    Wait-Ready $independent
    Release-Installer $independent
    if ([IO.File]::ReadAllText((Join-Path $parent 'independent target\VERSION.txt')) -ne 'source c') { throw 'Independent install did not finish.' }
    Write-Host 'PASS: unrelated source/target pairs can proceed independently.'
    Release-Installer $first
    [IO.File]::WriteAllText((Join-Path $b 'release'), 'release')
    $restart = Start-Installer $b $target
    $null = Wait-Exit $restart 77
    if ([IO.File]::ReadAllText((Join-Path $target 'VERSION.txt')) -ne 'source b') { throw 'Locks did not release after completion.' }
    Write-Host 'PASS: completion releases both locks and another source can then update the target.'

    $abandon = New-Source 'abandon source'
    $abandonTarget = Join-Path $parent 'abandon target'
    $owner = Start-Installer $abandon $abandonTarget
    Wait-Ready $owner
    $owner.Process.Kill(); $owner.Process.WaitForExit()
    [IO.File]::WriteAllText((Join-Path $abandon 'release'), 'release')
    $retry = Start-Installer $abandon $abandonTarget
    $null = Wait-Exit $retry 77
    Write-Host 'PASS: an interrupted preflight/build-boundary owner does not leave stale directory locks.'

    $race = New-Source 'race source'
    $raceTarget = Join-Path $parent 'race target'
    $racers = @(1..4 | ForEach-Object { Start-Installer $race $raceTarget })
    $until = [DateTime]::UtcNow.AddSeconds(20)
    do {
        $ready = @($racers | Where-Object { Test-Path -LiteralPath (Join-Path $race ($_.Process.Id.ToString() + '.ready')) })
        $exited = @($racers | Where-Object { $_.Process.HasExited })
        if ($ready.Count -eq 1 -and $exited.Count -eq 3) { break }
        if ([DateTime]::UtcNow -gt $until) { throw 'Simultaneous installers failed to produce exactly one owner.' }
        Start-Sleep -Milliseconds 20
    } while ($true)
    foreach ($loser in $exited) { Assert-Refused $loser }
    Release-Installer $ready[0]
    Write-Host 'PASS: four simultaneous installers produce one owner and three immediate refusals.'
} finally {
    foreach ($p in $children) { try { if (!$p.HasExited) { $p.Kill(); [void]$p.WaitForExit(5000) } } finally { $p.Dispose() } }
    $resolved = [IO.Path]::GetFullPath($testRoot)
    if (!$resolved.StartsWith(([IO.Path]::GetFullPath($env:TEMP).TrimEnd('\') + '\TaskTrace-install-concurrency-'), [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe test root.' }
    foreach ($junction in $junctions) { [IO.Directory]::Delete($junction) }
    if (Test-Path -LiteralPath $resolved) { Remove-Item -LiteralPath $resolved -Recurse -Force }
}
