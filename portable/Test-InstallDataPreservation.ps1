$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$tokens = $null
$parseErrors = $null
$installer = [Management.Automation.Language.Parser]::ParseFile((Join-Path $repoRoot 'dist\Install-TaskTrace-Engine.ps1'), [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
# Exercise the production deployment and allowlist without building binaries or
# executing the interactive installer. All file operations use a fresh temp root.
foreach ($name in @('Get-RunningInstallationProcesses', 'Install-ProgramFiles')) {
    $definition = $installer.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
    if ($null -eq $definition) { throw ('Missing installer function: ' + $name) }
    . ([ScriptBlock]::Create($definition.Extent.Text))
}
$allowlist = $installer.Find({ param($node) $node -is [Management.Automation.Language.AssignmentStatementAst] -and $node.Left.Extent.Text -eq '$programFileNames' }, $true)
if ($null -eq $allowlist) { throw 'Missing installer program allowlist.' }
$programNames = @(& ([ScriptBlock]::Create($allowlist.Right.Extent.Text)))
foreach ($name in $programNames) {
    if ([IO.Path]::GetFileName($name) -ne $name -or $name -in @('tasktrace-settings.json', 'data', 'teamData', '.cache', 'backups')) { throw ('Unsafe program file: ' + $name) }
}
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('TaskTrace-install-preservation-' + [Guid]::NewGuid().ToString('N'))
$source = Join-Path $testRoot 'new program'
$target = Join-Path $testRoot 'existing program'
$locked = $null
function Write-TestFile([string]$Path, [string]$Content) {
    [void][IO.Directory]::CreateDirectory([IO.Path]::GetDirectoryName($Path))
    [IO.File]::WriteAllText($Path, $Content, [Text.UTF8Encoding]::new($false))
}
function Read-TestHashes([string]$Root) {
    $hashes = @{}
    foreach ($file in Get-ChildItem -LiteralPath $Root -File -Recurse -Force) {
        $relative = $file.FullName.Substring($Root.Length + 1)
        $hashes[$relative] = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash
    }
    return $hashes
}
function Assert-Hashes([hashtable]$Expected, [string]$Root) {
    $actual = Read-TestHashes $Root
    foreach ($relative in $Expected.Keys) {
        if ($actual[$relative] -ne $Expected[$relative]) { throw ('Existing file changed or disappeared: ' + $relative) }
    }
}
try {
    $protected = @(
        'tasktrace-settings.json', 'data\tasktrace.db', 'data\tasktrace.db-wal', 'data\tasktrace.db-shm',
        'data\files\outstanding-note.png', 'data\team-sync.json', 'data\storage\original\tasktrace.db',
        'teamData\storage\shared\manifest.json', 'teamData\storage\shared\members\alice.json',
        'teamData\storage\shared\attachments\picture.png', '.cache\outstanding.json', 'backups\previous\data\tasktrace.db', 'personal-note.txt'
    )
    foreach ($name in $protected) {
        Write-TestFile (Join-Path $target $name) ('saved data: ' + $name)
        # Even if a build directory accidentally contains another computer's
        # settings/database, only explicit program files may be installed.
        Write-TestFile (Join-Path $source $name) ('must not install: ' + $name)
    }
    $before = Read-TestHashes $target
    foreach ($name in $programNames) {
        Write-TestFile (Join-Path $source $name) ('new program: ' + $name)
        Write-TestFile (Join-Path $target $name) ('old program: ' + $name)
    }
    Install-ProgramFiles $source $target $programNames
    Assert-Hashes $before $target
    foreach ($name in $programNames) {
        if ([IO.File]::ReadAllText((Join-Path $target $name)) -ne ('new program: ' + $name)) { throw ('Program not upgraded: ' + $name) }
    }
    $installed = Read-TestHashes $target
    foreach ($name in $programNames) { Write-TestFile (Join-Path $source $name) ('next program: ' + $name) }
    $locked = [IO.File]::Open((Join-Path $target $programNames[1]), 'Open', 'Read', 'Read')
    $failed = $false
    try { Install-ProgramFiles $source $target $programNames } catch { $failed = $true }
    $locked.Dispose()
    $locked = $null
    if (!$failed) { throw 'Expected the locked program to reject replacement.' }
    Assert-Hashes $installed $target
    if (@(Get-ChildItem -LiteralPath $target -Filter '*.tasktrace-installing').Count -ne 0) { throw 'Pending installer files remain.' }
    Write-Host ('PASS: upgrade and rollback preserve all ' + $protected.Count + ' configuration/data/team/attachment/cache/backup files byte-for-byte.')
} finally {
    if ($null -ne $locked) { $locked.Dispose() }
    $resolvedTestRoot = [IO.Path]::GetFullPath($testRoot)
    $expectedPrefix = [IO.Path]::GetFullPath([IO.Path]::GetTempPath()).TrimEnd('\') + '\TaskTrace-install-preservation-'
    if (!$resolvedTestRoot.StartsWith($expectedPrefix, [StringComparison]::OrdinalIgnoreCase)) { throw 'Refusing to clean outside the test directory.' }
    if (Test-Path -LiteralPath $resolvedTestRoot) { Remove-Item -LiteralPath $resolvedTestRoot -Recurse -Force }
}
