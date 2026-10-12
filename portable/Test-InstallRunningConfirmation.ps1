$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$installerFile = Join-Path $repoRoot 'dist\Install-TaskTrace-Engine.ps1'
$tokens = $null; $parseErrors = $null
$installer = [Management.Automation.Language.Parser]::ParseFile($installerFile, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
$text = [IO.File]::ReadAllText($installerFile)
function Replace-TestFunction([string]$Text, [string]$Name, [scriptblock]$Replacement) {
    $definition = $installer.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $Name }, $true)
    if ($null -eq $definition) { throw ('Missing installer function: ' + $Name) }
    return $Text.Replace($definition.Extent.Text, $Replacement.ToString().Trim())
}
# Only replace UI decisions and stop at the dependency boundary. Keep the real
# entry point, process inspection/termination and command wrapper under test.
$template = Replace-TestFunction $text 'Confirm-StopRunningInstallation' {
    function Confirm-StopRunningInstallation([string]$Directory, [object[]]$Processes) {
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'prompt-shown'), (($Processes | ForEach-Object { $_.ProcessId }) -join ','))
        return [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'choice')) -eq 'Yes'
    }
}
$template = Replace-TestFunction $template 'Confirm-ExistingInstallation' {
    function Confirm-ExistingInstallation([string]$Directory) { return $true }
}
$template = Replace-TestFunction $template 'Show-InstallResult' {
    function Show-InstallResult([string]$Title, [string]$Message, [bool]$IsError = $false) {
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'result-shown'), $Message)
    }
}
$earlyTemplate = Replace-TestFunction $template 'Import-FreshBuildEnvironment' {
    function Import-FreshBuildEnvironment {
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'dependency-reached'), 'reached')
        exit 77
    }
}
$lateTemplate = Replace-TestFunction $template 'Import-FreshBuildEnvironment' {
    function Import-FreshBuildEnvironment {
        # Signal the parent to start a copy after the early preflight. Reuse the
        # production copy routine to test the final pre-overwrite confirmation.
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'dependency-reached'), 'reached')
        $deadline = [DateTime]::UtcNow.AddSeconds(10)
        while (!(Test-Path -LiteralPath (Join-Path $PSScriptRoot 'late-ready'))) {
            if ([DateTime]::UtcNow -gt $deadline) { throw 'Late worker did not start.' }
            Start-Sleep -Milliseconds 20
        }
        Install-ProgramFiles (Join-Path $PSScriptRoot 'new program') $outputDirectory @('VERSION.txt')
        exit 77
    }
}
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('TaskTrace-install-confirmation-' + [Guid]::NewGuid().ToString('N'))
$dist = Join-Path $testRoot 'dist'
$target = Join-Path $testRoot 'selected program'
$other = Join-Path $testRoot 'another program'
$scriptFile = Join-Path $dist 'Install-TaskTrace-Engine.ps1'
$workers = New-Object 'System.Collections.Generic.List[Diagnostics.Process]'
$powershell64 = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
$powershell32 = Join-Path $env:WINDIR 'SysWOW64\WindowsPowerShell\v1.0\powershell.exe'
function Start-TestWorker([string]$Executable) {
    $ready = Join-Path $testRoot ([Guid]::NewGuid().ToString('N') + '.ready')
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = $Executable
    $start.Arguments = '"' + $ready + '"'
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $process = [Diagnostics.Process]::Start($start)
    $workers.Add($process)
    $deadline = [DateTime]::UtcNow.AddSeconds(10)
    while (!(Test-Path -LiteralPath $ready)) {
        if ($process.HasExited -or [DateTime]::UtcNow -gt $deadline) { throw 'Isolated worker failed to start.' }
        Start-Sleep -Milliseconds 20
    }
    return $process
}
function Run-ConfirmationTest([string]$Choice, [int]$ExpectedExit, [bool]$ExpectedEnvironment, [switch]$Wrapper, [switch]$Late, [string]$HostPath = $powershell64, [string]$ScriptText = $earlyTemplate, [bool]$ExpectedPrompt = $true) {
    foreach ($name in @('prompt-shown', 'result-shown', 'dependency-reached', 'late-ready')) {
        $marker = Join-Path $dist $name
        if (Test-Path -LiteralPath $marker) { Remove-Item -LiteralPath $marker -Force }
    }
    [IO.File]::WriteAllText((Join-Path $dist 'choice'), $Choice)
    [IO.File]::WriteAllText($scriptFile, $ScriptText, [Text.UTF8Encoding]::new($true))
    $start = New-Object Diagnostics.ProcessStartInfo
    if ($Wrapper) {
        $start.FileName = $env:ComSpec
        $start.Arguments = '/d /c ""' + (Join-Path $dist 'Install-TaskTrace.cmd') + '" -InstallDirectory "' + $target + '""'
    } else {
        $start.FileName = $HostPath
        $start.Arguments = '-NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $scriptFile + '" -Interactive -InstallDirectory "' + $target + '"'
    }
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.RedirectStandardInput = $true
    $process = [Diagnostics.Process]::Start($start)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if ($Late) {
            $deadline = [DateTime]::UtcNow.AddSeconds(10)
            while (!(Test-Path -LiteralPath (Join-Path $dist 'dependency-reached'))) {
                if ($process.HasExited -or [DateTime]::UtcNow -gt $deadline) { throw 'Installer did not reach the simulated build.' }
                Start-Sleep -Milliseconds 20
            }
            $script:lateWorker = Start-TestWorker (Join-Path $target 'TaskTrace-server.exe')
            [IO.File]::WriteAllText((Join-Path $dist 'late-ready'), 'ready')
        }
        if (!$process.WaitForExit(20000)) { $process.Kill(); throw 'Installer did not exit automatically.' }
        $output = $stdout.Result + $stderr.Result
        if ($process.ExitCode -ne $ExpectedExit) { throw ('Wrong exit code ' + $process.ExitCode + ': ' + $output) }
        if ((Test-Path -LiteralPath (Join-Path $dist 'dependency-reached')) -ne $ExpectedEnvironment) { throw ('Wrong dependency ordering: ' + $output) }
        if ((Test-Path -LiteralPath (Join-Path $dist 'prompt-shown')) -ne $ExpectedPrompt) { throw ('Wrong confirmation behavior: ' + $output) }
        if ($ExpectedExit -eq 1223) {
            if (Test-Path -LiteralPath (Join-Path $dist 'result-shown')) { throw 'Cancellation showed another result dialog.' }
            if ($output -match 'Press any key|Build failed|Installation completed') { throw ('Cancellation reached a normal wrapper pause: ' + $output) }
        }
    } finally {
        if (!$process.HasExited) { $process.Kill(); $process.WaitForExit(5000) | Out-Null }
        $process.Dispose()
    }
}
try {
    foreach ($directory in @($dist, $target, $other, (Join-Path $dist 'new program'))) { [void][IO.Directory]::CreateDirectory($directory) }
    Copy-Item -LiteralPath (Join-Path $repoRoot 'dist\Install-TaskTrace.cmd') -Destination $dist
    $source = Join-Path $testRoot 'worker.cs'
    [IO.File]::WriteAllText($source, 'using System.IO; using System.Threading; static class Worker { static void Main(string[] args) { File.WriteAllText(args[0], "ready"); Thread.Sleep(120000); } }')
    $compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
    $executable = Join-Path $target 'TaskTrace-server.exe'
    & $compiler /nologo /target:winexe /platform:x64 ('/out:' + $executable) $source
    if ($LASTEXITCODE -ne 0) { throw 'Could not compile isolated worker.' }
    foreach ($name in @('TaskTrace.exe', 'TaskTrace-floating.exe', 'TaskTrace-updater.exe')) { Copy-Item -LiteralPath $executable -Destination (Join-Path $target $name) }
    Copy-Item -LiteralPath $executable -Destination (Join-Path $other 'TaskTrace-server.exe')
    $otherWorker = Start-TestWorker (Join-Path $other 'TaskTrace-server.exe')
    $targetWorkers = @('TaskTrace.exe', 'TaskTrace-floating.exe', 'TaskTrace-server.exe', 'TaskTrace-updater.exe') | ForEach-Object { Start-TestWorker (Join-Path $target $_) }
    [IO.File]::WriteAllText((Join-Path $target 'VERSION.txt'), 'original')
    [IO.File]::WriteAllText((Join-Path $dist 'new program\VERSION.txt'), 'updated')
    $hashBefore = (Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash
    Run-ConfirmationTest No 1223 $false -Wrapper
    if (@($targetWorkers | Where-Object { $_.HasExited }).Count -gt 0) { throw 'Declining stopped a target process.' }
    Write-Host 'PASS: No closes the real command wrapper without a pause, result dialog, dependency work or process termination.'

    $deniedTemplate = Replace-TestFunction $earlyTemplate 'Get-InstallProcessImagePath' ([scriptblock]::Create('function Get-InstallProcessImagePath([Diagnostics.Process]$Process) { if ($Process.Id -eq ' + $targetWorkers[0].Id + ') { return $null }; Initialize-InstallProcessInspection; return [TaskTraceInstallNative]::ProcessPath($Process.Id) }'))
    Run-ConfirmationTest Yes 2 $false -ScriptText $deniedTemplate -ExpectedPrompt $false
    if (@($targetWorkers | Where-Object { $_.HasExited }).Count -gt 0) { throw 'Unknown paths triggered unsafe termination.' }
    Write-Host 'PASS: unverified process paths fail safely without terminating anything.'

    Run-ConfirmationTest Yes 77 $true
    if (@($targetWorkers | Where-Object { !$_.HasExited }).Count -ne 0) { throw 'Confirmed target process survived.' }
    if ($otherWorker.HasExited) { throw 'Another installation was incorrectly stopped.' }
    if ((Get-FileHash -LiteralPath $executable -Algorithm SHA256).Hash -ne $hashBefore) { throw 'Preflight changed installed program bytes.' }
    Write-Host 'PASS: Yes stops all four target process types before dependencies, leaves another installation running and preserves program bytes.'

    $worker32Check = Start-TestWorker $executable
    Run-ConfirmationTest Yes 77 $true -HostPath $powershell32
    if (!$worker32Check.HasExited) { throw '32-bit installer failed to stop a 64-bit process.' }
    Write-Host 'PASS: 32-bit installer can confirm and stop the 64-bit target.'

    Run-ConfirmationTest No 1223 $true -Wrapper -Late -ScriptText $lateTemplate
    if ($lateWorker.HasExited) { throw 'Late refusal stopped the running program.' }
    if ([IO.File]::ReadAllText((Join-Path $target 'VERSION.txt')) -ne 'original') { throw 'Late cancellation modified the installed program.' }
    $lateWorker.Kill(); [void]$lateWorker.WaitForExit(5000)
    Write-Host 'PASS: a copy started during build triggers another prompt; No closes the wrapper and preserves the installed program.'

    Run-ConfirmationTest Yes 77 $true -Late -ScriptText $lateTemplate
    if (!$lateWorker.HasExited) { throw 'Late confirmation failed to stop the program.' }
    if ([IO.File]::ReadAllText((Join-Path $target 'VERSION.txt')) -ne 'updated') { throw 'Late confirmation did not continue program installation.' }
    if ($otherWorker.HasExited) { throw 'Other installation was stopped during the final check.' }
    Write-Host 'PASS: late Yes stops only the selected copy and continues the real program-file installation.'
} finally {
    foreach ($worker in $workers) {
        if (!$worker.HasExited) { $worker.Kill(); [void]$worker.WaitForExit(5000) }
        $worker.Dispose()
    }
    $resolvedRoot = [IO.Path]::GetFullPath($testRoot)
    $expectedPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\TaskTrace-install-confirmation-'
    if (!$resolvedRoot.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Unsafe temporary test directory.' }
    if (Test-Path -LiteralPath $resolvedRoot) { Remove-Item -LiteralPath $resolvedRoot -Recurse -Force }
}
