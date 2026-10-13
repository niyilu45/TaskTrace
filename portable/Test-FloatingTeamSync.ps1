param([string]$OutputDirectory = '.local-build/floating-team-sync-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Performance test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Only result formatting and the in-flight request gate run. No UI, network request,
# server, database, team share or installed process is touched.
$source = @'
using System;
using System.Collections.Generic;
using System.Threading.Tasks;
internal sealed partial class FloatingWindow {
    internal static void TestTeamSyncSummary() {
        var bad=new Dictionary<string,object>{{"root_task_id",4},{"root_task_title","Shared task"},{"last_error","read member updates: denied"}};
        var good=new Dictionary<string,object>{{"root_task_id",5},{"conflicts",new object[]{new object()}}};
        var state=new Dictionary<string,object>{{"enabled",true},{"bindings",new object[]{bad,good}}};
        string text=TeamSyncResultText(state);
        if(!text.Contains("1 / 2") || !text.Contains("Shared task：read member updates: denied") || !text.Contains("1 项冲突") || text.Contains("同步完成"))throw new Exception("Per-binding failure was hidden by an HTTP-success summary");
        bad.Remove("root_task_title");
        if(!TeamSyncResultText(state).Contains("任务 #4"))throw new Exception("Task identity missing from error");
        bad["last_error"]="";
        if(!TeamSyncResultText(state).Contains("同步完成，共 2 项"))throw new Exception("Success summary missing");
        state["bindings"]=null;
        if(!TeamSyncResultText(state).Contains("没有需要同步"))throw new Exception("Empty state missing");
        state["enabled"]=false;
        if(!TeamSyncResultText(state).Contains("不可用") || !TeamSyncResultText(null).Contains("不可用"))throw new Exception("Unavailable sync reported as success");
    }
}
internal static class FloatingTeamSyncTests {
    static async Task TestSingleFlight() {
        var sync=new FloatingTeamSyncRequest();
        var pending=new TaskCompletionSource<Dictionary<string,object>>();
        var result=new Dictionary<string,object>{{"enabled",true}};
        int calls=0;
        Func<Task<Dictionary<string,object>>> send=delegate {calls++;return pending.Task;};
        var automatic=sync.Run(send);
        var manual=sync.Run(send);
        if(calls!=1 || !sync.InProgress)throw new Exception("Automatic/manual synchronization overlapped");
        pending.SetResult(result);
        if(!Object.ReferenceEquals(await automatic,result) || !Object.ReferenceEquals(await manual,result) || sync.InProgress)throw new Exception("Waiters did not receive the shared result");
        pending=new TaskCompletionSource<Dictionary<string,object>>();
        automatic=sync.Run(send);manual=sync.Run(send);
        pending.SetException(new Exception("network unavailable"));
        foreach(var waiter in new[]{automatic,manual}) {
            bool rejected=false;try {await waiter;}catch(Exception e){rejected=e.Message=="network unavailable";}
            if(!rejected)throw new Exception("Shared sync error not delivered");
        }
        if(sync.InProgress)throw new Exception("Error permanently blocked synchronization");
        await sync.Run(delegate {calls++;return Task.FromResult(result);});
        if(calls!=3 || sync.InProgress)throw new Exception("Immediate retry failed");
        try {await sync.Run(delegate {throw new Exception("request start failed");});}catch(Exception e){if(e.Message!="request start failed")throw;}
        if(sync.InProgress)throw new Exception("Synchronous request failure blocked retry");
    }
    [STAThread] static int Main() {
        try {
            FloatingWindow.TestTeamSyncSummary();TestSingleFlight().GetAwaiter().GetResult();
            Console.WriteLine("PASS automatic/manual single flight, all waiters receive failures, retry, partial errors, conflicts and empty/unavailable summaries.");return 0;
        } catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
'@
$harness=Join-Path $testRoot 'FloatingTeamSyncTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingTeamSyncTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingTeamSyncTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Floating team synchronization test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){ $process.Kill();throw 'Floating team synchronization test timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
$exitCode=$process.ExitCode;if($null -eq $exitCode){$exitCode=if((Get-Item -LiteralPath $stderr).Length -eq 0){0}else{1}}
if($exitCode -ne 0){Get-Content -LiteralPath $stderr;throw ('Floating team synchronization test failed: '+$exitCode)}
Write-Output ('Results: '+$stdout)
