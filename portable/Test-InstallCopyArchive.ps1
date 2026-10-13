$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$tokens = $null; $errors = $null
$installerPath = Join-Path $repoRoot 'dist\Install-TaskTrace-Engine.ps1'
$ast = [Management.Automation.Language.Parser]::ParseFile($installerPath, [ref]$tokens, [ref]$errors)
if ($errors.Count) { throw ($errors | Out-String) }
foreach ($name in @('New-InstallCopyArchive','Install-ProgramFiles')) {
    $definition = $ast.Find({param($n) $n -is [Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -eq $name}, $true)
    . ([scriptblock]::Create($definition.Extent.Text))
}
$allowlist = $ast.Find({param($n) $n -is [Management.Automation.Language.AssignmentStatementAst] -and $n.Left.Extent.Text -eq '$programFileNames'}, $true)
$programFileNames = @(& ([scriptblock]::Create($allowlist.Right.Extent.Text)))
$testRoot = Join-Path $repoRoot ('.local-build\install-archive-tests\' + [Guid]::NewGuid().ToString('N'))
$buildOutputDirectory = Join-Path $testRoot 'program [中文]'
$fixtureDist = Join-Path $testRoot 'dist [copy]'
$outputDirectory = Join-Path $testRoot 'installed'
foreach ($directory in @($buildOutputDirectory,$fixtureDist,$outputDirectory)) { [void][IO.Directory]::CreateDirectory($directory) }
$messages = New-Object 'Collections.Generic.List[string]'
$releaseOnRetry = $null
function Write-InstallLine([string]$Text, [ConsoleColor]$Color = [ConsoleColor]::Gray) {
    $messages.Add($Text)
    if ($script:releaseOnRetry -and $Text.Contains('正在重试')) { $script:releaseOnRetry.Dispose(); $script:releaseOnRetry = $null }
}
function Write-InstallLog([string]$Text) { $messages.Add($Text) }
function Show-InstallResult([string]$Title, [string]$Message, [bool]$IsError = $false) { $script:shownTitle = $Title; $script:shownMessage = $Message }
# Final availability is covered by Test-InstallRunningConfirmation. These tests
# only replace isolated text fixtures and never inspect/stop the real program.
function Assert-InstallationAvailable([string]$Directory, [switch]$PromptToStop) { }
function Assert([bool]$Condition, [string]$Message) { if (!$Condition) { throw $Message } }
function Read-Hash([string]$Path) { return (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash }
function Assert-Archive([string]$Path) {
    $file = [IO.File]::OpenRead($Path)
    try {
        $archive = [IO.Compression.ZipArchive]::new($file,[IO.Compression.ZipArchiveMode]::Read,$true)
        try {
            Assert ($archive.Entries.Count -eq $programFileNames.Count) 'Archive allowlist count mismatch'
            foreach ($entry in $archive.Entries) {
                Assert ($programFileNames -contains $entry.FullName) ('Unexpected archive entry: ' + $entry.FullName)
                $reader = [IO.StreamReader]::new($entry.Open())
                try { Assert ($reader.ReadToEnd() -eq [IO.File]::ReadAllText((Join-Path $buildOutputDirectory $entry.FullName))) ('Wrong entry bytes: ' + $entry.FullName) }
                finally { $reader.Dispose() }
            }
        } finally { $archive.Dispose() }
    } finally { $file.Dispose() }
}
foreach ($name in $programFileNames) { [IO.File]::WriteAllText((Join-Path $buildOutputDirectory $name),('program-v1: ' + $name)) }
foreach ($base in @($buildOutputDirectory,$outputDirectory)) {
    foreach ($name in @('data','teamData','.cache','backups')) {
        [void][IO.Directory]::CreateDirectory((Join-Path $base $name))
        [IO.File]::WriteAllText((Join-Path $base ($name + '\saved.txt')),('private ' + $name))
    }
    [IO.File]::WriteAllText((Join-Path $base 'tasktrace-settings.json'),'private settings')
}
$destination = Join-Path $fixtureDist 'TaskTrace-program-files.zip'
$held = $null
try {
    $actual = New-InstallCopyArchive $buildOutputDirectory $destination $programFileNames
    Assert ($actual -eq $destination) 'First archive used unexpected path'
    Assert-Archive $actual
    [IO.File]::WriteAllText((Join-Path $buildOutputDirectory 'VERSION.txt'),'program-v2')
    $actual = New-InstallCopyArchive $buildOutputDirectory $destination $programFileNames
    Assert ($actual -eq $destination) 'Normal replacement unexpectedly fell back'
    Assert-Archive $actual
    Write-Output 'PASS: first/replace ZIP, Unicode/bracket paths, exact program allowlist and entry bytes; no data/teamData/cache/backups/settings.'

    $oldHash = Read-Hash $destination
    $held = [IO.File]::Open($destination,'Open','Read','Read')
    [IO.File]::WriteAllText((Join-Path $buildOutputDirectory 'VERSION.txt'),'program-v3')
    $actual = New-InstallCopyArchive $buildOutputDirectory $destination $programFileNames
    Assert ($actual -ne $destination) 'Locked archive was overwritten'
    Assert-Archive $actual
    Assert ((Read-Hash $destination) -eq $oldHash) 'Locked last-good ZIP changed'
    $held.Dispose(); $held = $null
    Write-Output 'PASS: occupied ZIP retained byte-for-byte; new complete ZIP saved separately.'

    $releaseOnRetry = [IO.File]::Open((Join-Path $buildOutputDirectory 'TaskTrace.exe'),'Open','Read','None')
    $actual = New-InstallCopyArchive $buildOutputDirectory $destination $programFileNames
    Assert ($null -eq $releaseOnRetry) 'Transient source lock did not reach retry'
    Assert-Archive $actual
    $oldHash = Read-Hash $destination
    $held = [IO.File]::Open((Join-Path $buildOutputDirectory 'TaskTrace.exe'),'Open','Read','None')
    $failed = $false
    try { New-InstallCopyArchive $buildOutputDirectory $destination $programFileNames | Out-Null } catch { $failed = $true }
    Assert $failed 'Permanent source lock did not fail'
    Assert ((Read-Hash $destination) -eq $oldHash) 'Failed compression changed last-good ZIP'
    $held.Dispose(); $held = $null
    Assert (@(Get-ChildItem -LiteralPath $fixtureDist -Filter '.TaskTrace-copy-*.tmp' -Force).Count -eq 0) 'Partial ZIP was left behind'
    Write-Output 'PASS: transient source lock retried; permanent lock bounded, last-good ZIP preserved and partial files removed.'

    # Exercise the actual final installer flow, including optional packaging
    # failure and transactional program deployment, without rebuilding the app.
    $text = [IO.File]::ReadAllText($installerPath)
    $start = $text.IndexOf("    "+'$stage'+" = '生成安全复制包'")
    $end = $text.IndexOf('    exit 0', $start)
    Assert ($start -gt 0 -and $end -gt $start) 'Could not locate final install flow'
    $tail = $text.Substring($start,$end-$start).Replace('(Join-Path $PSScriptRoot ', '(Join-Path $fixtureDist ')
    $original = (Get-Item Function:New-InstallCopyArchive).ScriptBlock
    Set-Item Function:New-InstallCopyArchive { param($SourceDirectory,$DestinationPath,$Names) throw [IO.IOException]::new('fixture: copy archive directory unavailable') }
    try { . ([scriptblock]::Create($tail)) } finally { Set-Item Function:New-InstallCopyArchive $original }
    Assert ($copyArchive -eq '' -and $shownTitle.Contains('复制包未生成')) 'Archive failure was presented as archive success'
    Assert ($shownMessage.Contains('fixture:') -and $shownMessage.Contains('请勿使用旧复制包')) 'Failure details / old-package warning missing'
    foreach ($name in $programFileNames) { Assert ((Read-Hash (Join-Path $outputDirectory $name)) -eq (Read-Hash (Join-Path $buildOutputDirectory $name))) ('Program not installed: ' + $name) }
    foreach ($name in @('data','teamData','.cache','backups')) { Assert ([IO.File]::ReadAllText((Join-Path $outputDirectory ($name + '\saved.txt'))) -eq ('private ' + $name)) 'User files were modified' }
    Assert ([IO.File]::ReadAllText((Join-Path $outputDirectory 'tasktrace-settings.json')) -eq 'private settings') 'Settings were modified'
    Write-Output 'PASS: actual final installer flow installs compiled programs after ZIP failure, preserves user data, reports warning and does not advertise stale ZIP.'
    Write-Output ('Results: ' + $testRoot)
} finally {
    if ($held) { $held.Dispose() }
    if ($releaseOnRetry) { $releaseOnRetry.Dispose() }
}
