$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$enginePath = Join-Path $repoRoot 'dist\Install-TaskTrace-Engine.ps1'
$tokens = $null; $parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($enginePath, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
$text = [IO.File]::ReadAllText($enginePath)
function Replace-TestFunction([string]$Text, [string]$Name, [scriptblock]$Replacement) {
    $definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $Name }, $true)
    if (!$definition) { throw ('Missing function: ' + $Name) }
    return $Text.Replace($definition.Extent.Text, $Replacement.ToString().Trim())
}
# Exercise real success/finally/CMD code; stub build, UI and launch in fresh fixtures.
$template = Replace-TestFunction $text 'Confirm-LaunchInstalledApplication' {
    function Confirm-LaunchInstalledApplication([string]$Title, [string]$Message) {
        if (!$sourceInstallLock.SafeWaitHandle.IsClosed -or !$targetInstallLock.SafeWaitHandle.IsClosed) { throw 'Prompt before releasing locks.' }
        if (Test-Path -LiteralPath $buildStagingRoot) { throw 'Prompt before staging cleanup.' }
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'prompt.txt'), $Message)
        if ($testCase -eq 'PromptFailure') { throw 'Simulated popup failure.' }
        return $testCase -ne 'No'
    }
}
$template = Replace-TestFunction $template 'Show-InstallResult' {
    function Show-InstallResult([string]$Title, [string]$Message, [bool]$IsError = $false) {
        [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'result.txt'), $Message)
    }
}
$mainTry = $ast.EndBlock.Statements | Where-Object { $_ -is [Management.Automation.Language.TryStatementAst] } | Select-Object -First 1
if (!$mainTry) { throw 'Main try/finally missing.' }
$fakeBuild = @'
{
    $testCase = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'choice.txt'))
    $sourceInstallLock = [Threading.Mutex]::new($true)
    $targetInstallLock = [Threading.Mutex]::new($true)
    $buildStagingRoot = Join-Path $PSScriptRoot 'isolated staging'
    [void][IO.Directory]::CreateDirectory($buildStagingRoot)
    $outputDirectory = Join-Path $PSScriptRoot 'program [copy] 中文'
    [void][IO.Directory]::CreateDirectory($outputDirectory)
    if ($testCase -ne 'MissingLauncher') { [IO.File]::WriteAllText((Join-Path $outputDirectory 'TaskTrace.exe'), 'fixture only') }
    if ($testCase -eq 'Failed') { throw 'Simulated build failure.' }
    if ($testCase -eq 'Cancelled') { throw [OperationCanceledException]::new('User declined.') }
    $resultTitle = 'TaskTrace：安装成功'
    $resultMessage = 'Installed with data preserved.'
    $installationSucceeded = $true
}
'@
$template = $template.Replace($mainTry.Body.Extent.Text, $fakeBuild)
$launchStub = @'
function Start-Process {
    [CmdletBinding()]
    param([string]$FilePath, [string]$WorkingDirectory, [string]$WindowStyle)
    if ($testCase -eq 'LaunchFailure') { throw 'Simulated launch failure.' }
    if ($WindowStyle -ne 'Hidden') { throw 'Launch must not open a console.' }
    [IO.File]::WriteAllText((Join-Path $PSScriptRoot 'launch.json'), (@{File=$FilePath; Directory=$WorkingDirectory} | ConvertTo-Json))
}
'@
$template = $template.Insert($template.IndexOf('function Write-InstallLine'), $launchStub + [Environment]::NewLine)
$testRoot = Join-Path $repoRoot ('.local-build\install-completion-tests\' + [Guid]::NewGuid().ToString('N'))
foreach ($testCase in @('Yes','No','LaunchFailure','PromptFailure','MissingLauncher','NonInteractive','Failed','Cancelled')) {
    $fixture = Join-Path $testRoot $testCase
    [void][IO.Directory]::CreateDirectory($fixture)
    [IO.File]::WriteAllText((Join-Path $fixture 'Install-TaskTrace-Engine.ps1'), $template, [Text.UTF8Encoding]::new($true))
    [IO.File]::WriteAllBytes((Join-Path $fixture 'Install-TaskTrace.cmd'), [IO.File]::ReadAllBytes((Join-Path $repoRoot 'dist\Install-TaskTrace.cmd')))
    [IO.File]::WriteAllText((Join-Path $fixture 'choice.txt'), $testCase)
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $env:ComSpec
    $start.Arguments = '/d /c ""' + (Join-Path $fixture 'Install-TaskTrace.cmd') + '""'
    if ($testCase -eq 'NonInteractive') {
        $start.FileName = 'powershell.exe'
        $start.Arguments = '-NoProfile -ExecutionPolicy Bypass -File "' + (Join-Path $fixture 'Install-TaskTrace-Engine.ps1') + '"'
    }
    $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true; $start.RedirectStandardInput = $true
    $process = [Diagnostics.Process]::Start($start)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
        if ($testCase -eq 'Failed') { $process.StandardInput.WriteLine('x'); $process.StandardInput.Close() }
        if (!$process.WaitForExit(15000)) { $process.Kill(); throw ('Did not auto-close: ' + $testCase) }
        $expected = if ($testCase -eq 'Failed') { 1 } elseif ($testCase -eq 'Cancelled') { 1223 } else { 0 }
        if ($process.ExitCode -ne $expected) { throw ($testCase + ' wrong exit: ' + $process.ExitCode + ' ' + $stdout.Result + $stderr.Result) }
        $prompt = Test-Path -LiteralPath (Join-Path $fixture 'prompt.txt')
        $launched = Test-Path -LiteralPath (Join-Path $fixture 'launch.json')
        $result = Test-Path -LiteralPath (Join-Path $fixture 'result.txt')
        if ($prompt -ne ($testCase -notin @('NonInteractive','Failed','Cancelled'))) { throw ('Unexpected prompt: ' + $testCase) }
        if ($launched -ne ($testCase -eq 'Yes')) { throw ('Unexpected process launch: ' + $testCase) }
        if ($result -ne ($testCase -in @('LaunchFailure','PromptFailure','MissingLauncher','Failed'))) { throw ('Unexpected result: ' + $testCase) }
        if ($launched) {
            $launch = [IO.File]::ReadAllText((Join-Path $fixture 'launch.json')) | ConvertFrom-Json
            $target = Join-Path $fixture 'program [copy] 中文'
            if ($launch.File -ne (Join-Path $target 'TaskTrace.exe') -or $launch.Directory -ne $target) { throw 'Wrong installation launched.' }
        }
        if (Test-Path -LiteralPath (Join-Path $fixture 'isolated staging')) { throw 'Staging cleanup missing.' }
        Write-Output ('PASS: ' + $testCase + ', completion/cleanup/exit and target checks.')
    } finally { if (!$process.HasExited) { $process.Kill() }; $process.Dispose() }
}
Write-Output ('Results: ' + $testRoot)
