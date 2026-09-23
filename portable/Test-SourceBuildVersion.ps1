$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$root = $repoRoot
function Write-InstallLog([string]$Text) { }
. (Join-Path $PSScriptRoot 'SourceBuildVersion.ps1')

function Assert-Version([bool]$Condition, [string]$Message) {
    if (!$Condition) { throw $Message }
}

$releases = @(
    [pscustomobject]@{ tag_name = 'v0.1.0-beta.11'; draft = $false },
    [pscustomobject]@{ tag_name = 'v0.1.0-beta.12'; draft = $false },
    [pscustomobject]@{ tag_name = 'v9.0.0'; draft = $true }
)
$base = Select-TaskTraceLatestReleaseVersion $releases 'v0.1.0-beta.10'
$source = New-TaskTraceSourceBuildVersion $base ([DateTime]'2026-09-23T15:30:00Z') '756bffd7b3a1d5191dbd77126ae5f0c02935fa30'
Assert-Version ($base -eq 'v0.1.0-beta.12') 'Did not select the newest non-draft Release.'
Assert-Version ($source -eq 'v0.1.0-beta.12.source.20260923153000.g756bffd7b3a1') 'Source version format is unstable.'
Assert-Version ((Compare-TaskTraceVersion $source 'v0.1.0-beta.12') -gt 0) 'Source build must be newer than its Release base.'
Assert-Version ((Compare-TaskTraceVersion 'v0.1.0-beta.13' $source) -gt 0) 'A future Release must still update the source build.'
Assert-Version ((Compare-TaskTraceVersion 'v0.1.0' $source) -gt 0) 'A stable Release must update a prerelease source build.'
Assert-Version ((Resolve-TaskTraceSourceBuildVersion 'v1.2.3' 'unused/repository') -eq 'v1.2.3') 'Explicit release versions must remain unchanged.'
Write-Output ('PASS source version: ' + $source + ' > ' + $base + '; future beta.13 remains newer.')

$testRoot = Join-Path $repoRoot ('.local-build\source-version-tests\' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
$harness = Join-Path $testRoot 'SourceVersionTests.cs'
$harnessSource = @'
using System;
internal sealed partial class FloatingWindow {
    internal static void TestSourceVersionOrdering() {
        string source="v0.1.0-beta.12.source.20260923153000.g756bffd7b3a1";
        if(CompareVersions(source,"v0.1.0-beta.12")<=0)throw new Exception("Desktop updater considered the source build older than beta.12");
        if(CompareVersions("v0.1.0-beta.13",source)<=0)throw new Exception("Desktop updater did not consider beta.13 newer than the source build");
		if(!IsUpdateTimeout(new System.Threading.Tasks.TaskCanceledException("request timed out")))throw new Exception("Desktop updater did not recognize a download timeout");
    }
}
internal static class SourceVersionTests {
    [STAThread] static int Main() {
        try { FloatingWindow.TestSourceVersionOrdering();Console.WriteLine("PASS desktop updater ordering matches the install version ordering.");return 0; }
        catch(Exception error) { Console.Error.WriteLine(error);return 1; }
    }
}
'@
[IO.File]::WriteAllText($harness, $harnessSource, [Text.UTF8Encoding]::new($false))
$compiler = Join-Path $env:WINDIR 'Microsoft.NET\Framework64\v4.0.30319\csc.exe'
$binary = Join-Path $testRoot 'SourceVersionTests.exe'
$sources = @(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:SourceVersionTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:' + $binary) @sources $harness
if ($LASTEXITCODE -ne 0) { throw 'Desktop updater version test compilation failed.' }
& $binary
if ($LASTEXITCODE -ne 0) { throw 'Desktop updater version ordering test failed.' }
