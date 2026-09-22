param([string]$OutputDirectory = '.local-build/floating-performance-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Performance test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Only pure production readers and an invisible in-memory task surface run. No app startup,
# server, user database, session, watcher, team share or installed process is touched.
$source = @"
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    static Dictionary<string,object> PerfNote(long id,string html) {return new Dictionary<string,object>{{"id",id},{"comment",html}};}
    static Dictionary<string,object> PerfPage(IEnumerable<Dictionary<string,object>> notes,int totalPages) {return new Dictionary<string,object>{{"items",notes.ToArray()},{"total_pages",totalPages}};}
    internal static async Task TestOutstandingReadsForPerformance() {
        string shared="<h3>"+SharedHeading+"</h3><ul><li data-id=\"one\" data-priority=\"3\">Current <img src=\"/image/1\"><aside data-tasktrace-outstanding-note=\"true\" hidden>Memo</aside></li></ul>";
        var history=Enumerable.Range(1,1000).Select(id=>PerfNote(id,"<h3>每日进展 · 2026-09-22</h3><p>Entry "+id+"</p>")).ToList();history.Add(PerfNote(1001,shared));
        int calls=0;
        Func<string,Task<Dictionary<string,object>>> read=delegate(string path) {
            calls++;var query=System.Web.HttpUtility.ParseQueryString(new Uri("http://127.0.0.1"+path).Query);int page=Int32.Parse(query["page"]);
            string filter=query["q"];var matching=history.Where(note=>String.IsNullOrEmpty(filter)||Convert.ToString(note["comment"]).Contains(filter)).OrderByDescending(note=>Convert.ToInt64(note["id"])).ToArray();
            return Task.FromResult(PerfPage(matching.Skip((page-1)*100).Take(100),Math.Max(1,(matching.Length+99)/100)));
        };
        var result=await ReadOutstandingList(1,history.Count,read,()=>true);
        var expected=ReadShared(history);
        if(calls!=1 || result.CommentId!=expected.CommentId || result.Items.Count!=1 || result.Items[0].Priority!=3 || result.Items[0].Html!=expected.Items[0].Html || result.Items[0].NoteHtml!="Memo")throw new Exception("Filtered outstanding history lost data or fetched daily-progress pages");
        Console.WriteLine("PASS history: 1,001 comments, 11 pages reduced to 1 filtered page; title/image/memo/priority preserved.");
        history.Add(PerfNote(1002,"<h3>"+SharedHeading+"</h3><ul></ul>"));calls=0;result=await ReadOutstandingList(1,history.Count,read,()=>true);
        if(calls!=1 || result.CommentId!=1002 || result.Items.Count!=0)throw new Exception("A newer empty shared list restored deleted outstanding items from old records");
        history.RemoveAt(history.Count-1);
        calls=0;await ReadOutstandingList(1,0,read,()=>true);if(calls!=0)throw new Exception("Known-empty task requested comments");
        calls=0;await ReadOutstandingList(1,5,read,()=>true);if(calls!=11)throw new Exception("Small/unknown task must preserve complete-read fallback when the count changes concurrently");
        history.Clear();history.AddRange(Enumerable.Range(1,150).Select(id=>PerfNote(id,"<p>Audit "+id+"</p>")));history.Add(PerfNote(151,"<h3>每日进展 · 2026-09-22</h3><p><strong>遗留问题 / 下一步</strong></p><p>Legacy outstanding</p>"));
        calls=0;result=await ReadOutstandingList(1,151,read,()=>true);if(calls!=3 || result.Items.Count!=1 || result.Items[0].Html!="Legacy outstanding")throw new Exception("Old-format daily outstanding fallback failed");
        history.Clear();history.Add(PerfNote(1,shared));calls=0;result=await ReadOutstandingList(1,1,read,()=>true);if(calls!=1 || result.Items.Count!=1)throw new Exception("Small tasks gained an extra search request");
        bool current=true;calls=0;
        Func<string,Task<Dictionary<string,object>>> superseded=delegate(string path){calls++;current=false;return Task.FromResult(PerfPage(history,20));};
        result=await ReadOutstandingList(1,1000,superseded,()=>current);if(result!=null || calls!=1)throw new Exception("Stale filtered read continued into fallback/history pages");
        result=await ReadOutstandingList(1,1,superseded,()=>current);if(result!=null || calls!=1)throw new Exception("Stale queued read issued a request");
        current=true;calls=0;result=await ReadOutstandingList(1,-1,superseded,()=>current);if(result!=null || calls!=1)throw new Exception("Stale legacy pagination kept downloading pages");
        Console.WriteLine("PASS empty/small/legacy/concurrent-count-change behavior; stale queued reads and pagination stop.");
    }
}
internal static class FloatingPerformanceTests {
    [STAThread] static int Main() {
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            FloatingWindow.TestOutstandingReadsForPerformance().GetAwaiter().GetResult();
            using(var host=new Form{ShowInTaskbar=false,Opacity=0,ClientSize=new Size(400,300)})
            using(var model=new TaskTreeView())
            using(var surface=new TaskTreeSurface{Dock=DockStyle.Fill}) {
                model.SingleLinePaths=true;model.ItemHeight=28;model.Indent=20;
                for(int i=0;i<1000;i++){string text=(i+1)+". [P7] Task "+i+" - a moderately long task label for horizontal scrolling / ancestor";model.Nodes.Add(new FloatingWindow.TaskNode(text){Tag=(long)i+1,CurrentTextLength=text.Length});}
                host.Controls.Add(surface);surface.Bind(model);host.Show();Application.DoEvents();surface.Rebuild(true);Application.DoEvents();
                var measured=Stopwatch.StartNew();surface.Rebuild(true);measured.Stop();
                long builds=surface.LayoutBuildCount;int invalidations=0;InvalidateEventHandler invalidated=delegate{invalidations++;};surface.Invalidated+=invalidated;
                measured.Restart();for(int i=0;i<100;i++)surface.Rebuild(true);measured.Stop();
                if(surface.LayoutBuildCount!=builds || invalidations!=0)throw new Exception("Unchanged refresh remeasured or repainted the rows");
                Console.WriteLine("PASS surface: 100 unchanged refreshes over 1,000 rows: 0 layout builds, 0 repaint requests ("+measured.ElapsedMilliseconds+" ms).");
                surface.Invalidated-=invalidated;surface.TestScrollTo(120,160);var scroll=surface.ScrollOffset;var extent=surface.ContentExtent;
                model.SelectedNode=model.Nodes[6];surface.Rebuild(true);if(surface.LayoutBuildCount!=builds || surface.ScrollOffset!=scroll)throw new Exception("Selection remeasured the list or moved its scroll offsets");
                var node=model.Nodes[0];node.Text+=" updated";surface.Rebuild(true);if(surface.LayoutBuildCount!=++builds || surface.ScrollOffset!=scroll)throw new Exception("Content edit failed to remeasure while preserving scroll");
                node.Checked=true;surface.Rebuild(true);if(surface.LayoutBuildCount!=++builds)throw new Exception("Completion did not refresh row state");
                ((FloatingWindow.TaskNode)node).ReminderCount=1;surface.Rebuild(true);if(surface.LayoutBuildCount!=++builds || surface.ReminderBounds(node).IsEmpty)throw new Exception("Reminder button failed to update width/hit bounds");
                node.ForeColor=Color.Blue;surface.Rebuild(true);if(surface.LayoutBuildCount!=builds)throw new Exception("Color-only change unnecessarily measured text");
                var leaf=new TreeNode("1001. [P2] Outstanding"){Tag=new FloatingWindow.OutstandingLeaf{TaskId=1,Id="leaf",Html="Plain"}};model.Nodes.Add(leaf);model.SimpleImageAvailable=delegate(TreeNode value){var data=value.Tag as FloatingWindow.OutstandingLeaf;return data!=null && data.Html.Contains("<img");};model.SetSimpleImageLinks(true);surface.Rebuild(true);builds=surface.LayoutBuildCount;
                ((FloatingWindow.OutstandingLeaf)leaf.Tag).Html="<img src='image'>";surface.Rebuild(true);if(surface.LayoutBuildCount!=++builds || surface.ImageBounds(leaf).IsEmpty)throw new Exception("New image button did not remeasure row width");
                host.ClientSize=new Size(650,300);Application.DoEvents();surface.Rebuild(true);host.ClientSize=new Size(240,300);Application.DoEvents();surface.Rebuild(true);
                if(surface.ClientSize.Width>240 || surface.TitleBounds(node).IsEmpty)throw new Exception("Resize did not update viewport geometry");
                surface.Rebuild(false);if(surface.ScrollOffset!=Point.Empty)throw new Exception("Explicit scroll reset was skipped");
                var saved=model.Nodes.Cast<TreeNode>().ToArray();model.Nodes.Clear();model.SingleLinePaths=false;model.SetWrappedText(true);model.Nodes.Add(node);surface.Rebuild(false);Application.DoEvents();surface.Rebuild(true);
                int expectedWidth=Math.Max(80,surface.ClientSize.Width-SystemInformation.VerticalScrollBarWidth-6);
                if(surface.NodeBounds(node).Width!=expectedWidth)throw new Exception("Scrollbar disappearance reused measurements from the previous client width");
                model.Nodes.Clear();model.Nodes.AddRange(saved);surface.Rebuild(false);Application.DoEvents();surface.Rebuild(true);
                expectedWidth=Math.Max(80,surface.ClientSize.Width-SystemInformation.VerticalScrollBarWidth-6);
                if(surface.NodeBounds(node).Width!=expectedWidth)throw new Exception("Scrollbar appearance reused measurements from the previous client width");
                model.Font=new Font(model.Font.FontFamily,model.Font.Size+2);surface.Rebuild(true);
                Console.WriteLine("PASS selection, ancestor text/completion, image/reminder markers, color-only changes, wide/narrow resize, scrollbar appearance/disappearance, font changes and explicit scroll reset.");
                host.Close();
            }
            return 0;
        }catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
"@
$harness=Join-Path $testRoot 'FloatingPerformanceTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingPerformanceTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingPerformanceTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Floating performance test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){ $process.Kill();throw 'Floating performance test timed out.' }
$process.WaitForExit()
Get-Content -LiteralPath $stdout
if($process.ExitCode -ne 0){Get-Content -LiteralPath $stderr;throw ('Floating performance test failed: '+$process.ExitCode)}
Write-Output ('Results: '+$stdout)