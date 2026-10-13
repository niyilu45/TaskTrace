param([string]$OutputDirectory = '.local-build/floating-progress-sync-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Performance test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Only pure production readers and an invisible in-memory task surface run. No app startup,
# server, user database, session, watcher, team share or installed process is touched.
$source = @'
using System;
using System.Collections.Generic;
using System.Linq;
internal sealed partial class FloatingWindow {
    static Dictionary<string,object> ProgressTestRecord(long id,string day,string author,string shared,string merged,string team,string created) {
        return new Dictionary<string,object>{{"id",id},{"created",created},{"comment","<h3 data-tasktrace-merged=\""+merged+"\" data-tasktrace-team-merged=\""+team+"\">每日进展 · "+day+"</h3><p>"+shared+"</p>"+(String.IsNullOrEmpty(shared)?"":FloatingTeamMarkerHtml(shared,author))},{"author",new Dictionary<string,object>{{"username",author}}}};
    }
    internal static void TestProgressSync() {
        string day="2026-10-13";
        var history=new List<Dictionary<string,object>>{
            ProgressTestRecord(40,day,"member","first","","","2026-10-13T08:00:00Z"),
            ProgressTestRecord(41,day,"member","second","42","first","2026-10-13T09:00:00Z"),
            ProgressTestRecord(42,day,"member","final","42,43","first,second","2026-10-13T10:00:00Z"),
            ProgressTestRecord(43,day,"owner","owner-final","42","first,second","2026-10-13T11:00:00Z"),
            ProgressTestRecord(44,"2026-10-12","member","yesterday","42","final","2026-10-13T12:00:00Z")
        };
        var result=DailyHistory(history);
        if(!result.Select(note=>Convert.ToInt64(note["id"])).SequenceEqual(new long[]{43,42,44}))throw new Exception("Cross-device ids hid a member, another date, or the final revision");
        if(ProgressDates(history).Count!=2)throw new Exception("Calendar lost a progress date");
        var local=new List<Dictionary<string,object>>{
            ProgressTestRecord(8,day,"local","","9","","2026-10-13T09:00:00Z"),
            ProgressTestRecord(9,day,"local","","8","","2026-10-13T10:00:00Z")
        };
        if(DailyHistory(local).Count!=1 || Convert.ToInt64(DailyHistory(local)[0]["id"])!=9)throw new Exception("Legacy numeric references hid their successor");
        var oldPeer=new List<Dictionary<string,object>>{
            ProgressTestRecord(90,day,"member","previous","","","2026-10-13T09:00:00Z"),
            ProgressTestRecord(20,day,"CHINA\\\\member","newest","90","","2026-10-13T10:00:00Z")
        };
        oldPeer.Add(oldPeer[1]); // A concurrent paginated read can repeat a row.
        if(DailyHistory(oldPeer).Count!=1 || Convert.ToInt64(DailyHistory(oldPeer)[0]["id"])!=20)throw new Exception("Legacy peer final progress or duplicate page handling failed");
        Console.WriteLine("PASS independent comment ids, owner/member isolation, stable revision history, calendar dates and legacy numeric references.");
    }
}
internal static class FloatingProgressSyncTests {
    [STAThread] static int Main() {
        try {FloatingWindow.TestProgressSync();return 0;}
        catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
'@
$harness=Join-Path $testRoot 'FloatingProgressSyncTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingProgressSyncTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingProgressSyncTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Floating progress synchronization test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){ $process.Kill();throw 'Floating progress synchronization test timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
$exitCode=$process.ExitCode;if($null -eq $exitCode){$exitCode=if((Get-Item -LiteralPath $stderr).Length -eq 0){0}else{1}}
if($exitCode -ne 0){Get-Content -LiteralPath $stderr;throw ('Floating progress synchronization test failed: '+$exitCode)}
Write-Output ('Results: '+$stdout)
