$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$root = $repoRoot
function Write-InstallLog([string]$Text) { }
. (Join-Path $PSScriptRoot 'SourceBuildVersion.ps1')

function Assert-Version([bool]$Condition, [string]$Message) {
    if (!$Condition) { throw $Message }
}

Assert-Version ($TaskTraceSourceFallbackVersion -eq ([IO.File]::ReadAllText((Join-Path $PSScriptRoot 'LATEST-RELEASE.txt'))).Trim()) 'Bundled Release baseline is stale.'
$feedVersion = Select-TaskTraceLatestReleaseVersionFromFeed '<entry><link href="https://github.com/niyilu45/TaskTrace/releases/tag/v0.1.0-beta.15" /></entry>' 'v0.1.0-beta.14'
Assert-Version ($feedVersion -eq 'v0.1.0-beta.15') 'Release feed fallback did not select the newest version.'

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
$stableSource = New-TaskTraceSourceBuildVersion 'v1.0.0' ([DateTime]'2026-10-26T03:00:00Z') '6e7c8c6af44ebb6d48d641dfb64bad6e555810c1'
Assert-Version ($stableSource -eq 'v1.0.0+source.20261026030000.g6e7c8c6af44e') 'Stable source build is not valid SemVer metadata.'
Assert-Version ((Compare-TaskTraceVersion $stableSource 'v1.0.0') -eq 0) 'Stable source metadata must not trigger a downgrade to its own Release.'
Assert-Version ((Compare-TaskTraceVersion 'v1.0.1' $stableSource) -gt 0) 'The next patch Release must update a stable source build.'
Assert-Version ((Compare-TaskTraceVersion 'v1.1.0-beta.1' $stableSource) -gt 0) 'A higher minor prerelease must update a stable source build.'
Assert-Version ((New-TaskTraceSourceBuildVersion '1.0.0+previous.build' ([DateTime]'2026-10-26T03:00:00Z')) -eq 'v1.0.0+source.20261026030000') 'Existing metadata produced more than one plus sign.'
Write-Output ('PASS source version: ' + $source + ' > ' + $base + '; future beta.13 remains newer.')

$testRoot = Join-Path $repoRoot ('.local-build\source-version-tests\' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Exercise the actual build entry guard, stopping before any dependency/build/install writes.
$tokens = $null; $parseErrors = $null
$buildPath = Join-Path $repoRoot 'portable\Build-Local.ps1'
$buildAst = [Management.Automation.Language.Parser]::ParseFile($buildPath, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count) { throw ($parseErrors | Out-String) }
$guard = $buildAst.Find({param($node) $node -is [Management.Automation.Language.IfStatementAst] -and $node.Extent.Text.Contains('Version must be a semantic version')}, $true)
if ($null -eq $guard) { throw 'Build entry version guard was not found.' }
$fixturePortable = Join-Path $testRoot 'portable'
[void][IO.Directory]::CreateDirectory($fixturePortable)
$fixtureBuild = Join-Path $fixturePortable 'Build-Local.ps1'
$prefix = [IO.File]::ReadAllText($buildPath).Substring(0, $guard.Extent.EndOffset)
[IO.File]::WriteAllText($fixtureBuild, $prefix + "`r`n[IO.File]::WriteAllText((Join-Path `$PSScriptRoot 'version.accepted'), `$Version)`r`nexit 77`r`n", [Text.UTF8Encoding]::new($true))
Copy-Item -LiteralPath (Join-Path $PSScriptRoot 'DependencyBootstrap.ps1') -Destination $fixturePortable
function Assert-BuildVersionAccepted([string]$Value, [bool]$Accepted) {
    $marker = Join-Path $fixturePortable 'version.accepted'
    if (Test-Path -LiteralPath $marker) { Remove-Item -LiteralPath $marker -Force }
    $start = New-Object Diagnostics.ProcessStartInfo
    $start.FileName = Join-Path $env:WINDIR 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $start.Arguments = '-NoProfile -NonInteractive -ExecutionPolicy Bypass -File "' + $fixtureBuild + '" -Version "' + $Value + '"'
    $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
    $process = [Diagnostics.Process]::Start($start)
    try {
        $stdout = $process.StandardOutput.ReadToEndAsync(); $stderr = $process.StandardError.ReadToEndAsync()
        if (!$process.WaitForExit(15000)) { $process.Kill(); throw 'Build version guard test timed out.' }
        $output = $stdout.Result + $stderr.Result
        if ($Accepted) {
            Assert-Version ($process.ExitCode -eq 77 -and (Test-Path -LiteralPath $marker)) ('Build rejected valid version ' + $Value + ': ' + $output)
            Assert-Version ([IO.File]::ReadAllText($marker) -eq $Value) 'Build arguments changed version metadata.'
        } else {
            Assert-Version ($process.ExitCode -ne 77 -and !(Test-Path -LiteralPath $marker) -and $output.Contains('Version must be a semantic version')) ('Invalid version bypassed the real build guard: ' + $Value)
        }
    } finally { $process.Dispose() }
}
foreach ($validVersion in @($stableSource, $source, 'v1.0.0', 'v1.0.1-beta.1+build.2')) { Assert-BuildVersionAccepted $validVersion $true }
foreach ($invalidVersion in @('v1.0.0.source.20261026030000', 'v1.0.0+', 'v1.0.0+source..123')) { Assert-BuildVersionAccepted $invalidVersion $false }

# A source ZIP has no Git metadata; API/feed/offline resolution must still return a scalar build version.
$savedRoot = $root
$root = $testRoot
try {
    function Invoke-RestMethod { param($Uri, $Headers, $TimeoutSec, $ErrorAction, $Proxy, $ProxyUseDefaultCredentials) return @([pscustomobject]@{tag_name=$TaskTraceSourceFallbackVersion;draft=$false}) }
    $resolved = Resolve-TaskTraceSourceBuildVersion '' 'unused/repository'
    Assert-Version ($resolved -is [string] -and $resolved -match ('^' + [regex]::Escape($TaskTraceSourceFallbackVersion) + '\+source\.[0-9]{14}$')) 'ZIP source resolution produced an invalid version or an array.'
    Assert-BuildVersionAccepted $resolved $true
    function Invoke-RestMethod { throw 'isolated API failure' }
    function Invoke-WebRequest { param($Uri, $Headers, $TimeoutSec, $UseBasicParsing, $ErrorAction, $Proxy, $ProxyUseDefaultCredentials) return [pscustomobject]@{Content='<entry><link href="https://github.com/unused/repository/releases/tag/v1.0.1" /></entry>'} }
    $resolved = Resolve-TaskTraceSourceBuildVersion '' 'unused/repository'
    Assert-Version ($resolved -match '^v1\.0\.1\+source\.[0-9]{14}$') 'Release feed fallback produced an invalid stable source version.'
    Assert-BuildVersionAccepted $resolved $true
    function Invoke-WebRequest { throw 'isolated offline failure' }
    $resolved = Resolve-TaskTraceSourceBuildVersion '' 'unused/repository'
    Assert-Version ($resolved -match ('^' + [regex]::Escape($TaskTraceSourceFallbackVersion) + '\+source\.[0-9]{14}$')) 'Offline bundled stable baseline produced an invalid source version.'
    Assert-BuildVersionAccepted $resolved $true
} finally { $root = $savedRoot; Remove-Item Function:Invoke-RestMethod, Function:Invoke-WebRequest -ErrorAction SilentlyContinue }
Write-Output 'PASS stable/prerelease SemVer; actual build argument validation; Git-free ZIP API/feed/offline fallback; future Release ordering.'

$harness = Join-Path $testRoot 'SourceVersionTests.cs'
$harnessSource = @'
using System;
internal sealed partial class FloatingWindow {
    internal static void TestSourceVersionOrdering() {
        string source="v0.1.0-beta.12.source.20260923153000.g756bffd7b3a1";
        if(CompareVersions(source,"v0.1.0-beta.12")<=0)throw new Exception("Desktop updater considered the source build older than beta.12");
        if(CompareVersions("v0.1.0-beta.13",source)<=0)throw new Exception("Desktop updater did not consider beta.13 newer than the source build");
        string stable="v1.0.0+source.20261026030000.g6e7c8c6af44e";
        if(CompareVersions(stable,"v1.0.0")!=0 || CompareVersions("v1.0.0",stable)!=0)throw new Exception("Desktop updater suggested downgrading stable source metadata to its own Release");
        if(CompareVersions("v1.0.1",stable)<=0 || CompareVersions("v1.1.0-beta.1",stable)<=0)throw new Exception("A future Release could not update the stable source build");
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
