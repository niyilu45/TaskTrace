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
    internal FloatingWindow(string isolatedDirectory) {root=isolatedDirectory;data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=true;}
    static Dictionary<string,object> PerfNote(long id,string html) {return new Dictionary<string,object>{{"id",id},{"comment",html}};}
    static Dictionary<string,object> PerfPage(IEnumerable<Dictionary<string,object>> notes,int totalPages) {return new Dictionary<string,object>{{"items",notes.ToArray()},{"total_pages",totalPages}};}
    internal async Task TestOutstandingReadsForPerformance() {
        string shared="<h3>"+SharedHeading+"</h3><ul><li data-id=\"one\" data-priority=\"3\">Current <img src=\"/image/1\"><aside data-tasktrace-outstanding-note=\"true\" hidden>Memo</aside></li></ul>";
        var history=Enumerable.Range(1,1000).Select(id=>PerfNote(id,"<h3>\u6bcf\u65e5\u8fdb\u5c55: 2026-09-22</h3><p>Entry "+id+"</p>")).ToList();history.Add(PerfNote(1001,shared));
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
        history.Clear();history.AddRange(Enumerable.Range(1,150).Select(id=>PerfNote(id,"<p>Audit "+id+"</p>")));history.Add(PerfNote(151,"<h3>\u6bcf\u65e5\u8fdb\u5c55: 2026-09-22</h3><p><strong>\u9057\u7559\u95ee\u9898 / \u4e0b\u4e00\u6b65</strong></p><p>Legacy outstanding</p>"));
        calls=0;result=await ReadOutstandingList(1,151,read,()=>true);if(calls!=3 || result.Items.Count!=1 || result.Items[0].Html!="Legacy outstanding")throw new Exception("Old-format daily outstanding fallback failed");
        history.Clear();history.Add(PerfNote(1,shared));calls=0;result=await ReadOutstandingList(1,1,read,()=>true);if(calls!=1 || result.Items.Count!=1)throw new Exception("Small tasks gained an extra search request");
        bool current=true;calls=0;
        Func<string,Task<Dictionary<string,object>>> superseded=delegate(string path){calls++;current=false;return Task.FromResult(PerfPage(history,20));};
        result=await ReadOutstandingList(1,1000,superseded,()=>current);if(result!=null || calls!=1)throw new Exception("Stale filtered read continued into fallback/history pages");
        result=await ReadOutstandingList(1,1,superseded,()=>current);if(result!=null || calls!=1)throw new Exception("Stale queued read issued a request");
        current=true;calls=0;result=await ReadOutstandingList(1,-1,superseded,()=>current);if(result!=null || calls!=1)throw new Exception("Stale legacy pagination kept downloading pages");
        Console.WriteLine("PASS empty/small/legacy/concurrent-count-change behavior; stale queued reads and pagination stop.");
    }
    internal void TestLocalAppearanceAndPriorityOrdering() {
        var all=new Dictionary<long,Dictionary<string,object>>{
            {1,new Dictionary<string,object>{{"priority",3}}},{2,new Dictionary<string,object>{{"priority",10}}},{3,new Dictionary<string,object>{{"priority",7}}},{4,new Dictionary<string,object>{{"priority",8}}}
        };
        var low=new TaskNode("low"){Tag=1L};var high=new TaskNode("high"){Tag=2L};var childLow=new TaskNode("child low"){Tag=3L};var childHigh=new TaskNode("child high"){Tag=4L};low.Nodes.Add(childLow);low.Nodes.Add(childHigh);
        var roots=new List<TreeNode>{low,high};SortTaskHierarchy(roots,all);
        if((long)roots[0].Tag!=2 || (long)low.Nodes[0].Tag!=4)throw new Exception("Priority ordering did not sort root siblings and children independently");
        var parents=new Dictionary<long,long>{{3,1}};SetLocalTaskAppearance(1,"mint");
        if(EffectiveTaskAppearance(3,parents)!="mint")throw new Exception("Child task did not inherit the local parent background");
        SetLocalTaskAppearance(3,"soft-rose");if(EffectiveTaskAppearance(3,parents)!="soft-rose")throw new Exception("Child task local background did not override inheritance");
        SetLocalOutstandingAppearance(3,"item","lavender");
        string saved=System.IO.File.ReadAllText(AppearancePath);if(!saved.Contains("soft-rose") || !saved.Contains("3:item") || saved.Contains(SharedHeading))throw new Exception("Local colors were not isolated in the local appearance file");
        var menu=CreateAppearanceMenuItem();var menuParent=new TaskNode("parent"){Tag=11L};var menuChild=new TaskNode("child"){Tag=12L};var menuLeaf=new TreeNode("outstanding"){Tag=new OutstandingLeaf{TaskId=12L,Id="menu-item",Html="outstanding"}};menuChild.Nodes.Add(menuLeaf);menuParent.Nodes.Add(menuChild);tasks.Nodes.Add(menuParent);taskParents[12]=11;
        tasks.SelectedNode=menuChild;RefreshAppearanceMenu(menu);if(!menu.Enabled || menu.DropDownItems[0].Text!="\u7ee7\u627f\u7236\u4efb\u52a1")throw new Exception("Child task background menu did not offer parent inheritance");((ToolStripMenuItem)menu.DropDownItems[2]).PerformClick();if(LocalTaskAppearance(12)!="mint" || !((ToolStripMenuItem)menu.DropDownItems[2]).Image.Size.Equals(new Size(18,18)))throw new Exception("Task background menu did not apply the selected color or show its swatch");
        tasks.SelectedNode=menuLeaf;RefreshAppearanceMenu(menu);if(menu.DropDownItems[0].Text!="\u7ee7\u627f\u6240\u5c5e\u4efb\u52a1")throw new Exception("Outstanding background menu did not offer task inheritance");((ToolStripMenuItem)menu.DropDownItems[4]).PerformClick();if(LocalOutstandingAppearance(12,"menu-item")!="soft-rose")throw new Exception("Outstanding background menu did not apply the selected color");((ToolStripMenuItem)menu.DropDownItems[0]).PerformClick();RefreshAppearanceMenu(menu);if(LocalOutstandingAppearance(12,"menu-item")!="" || !((ToolStripMenuItem)menu.DropDownItems[0]).Checked)throw new Exception("Outstanding background menu did not restore inherited color");
        menu.Dispose();tasks.Nodes.Clear();taskParents.Remove(12);
        Console.WriteLine("PASS local appearance: parent inheritance, child override, outstanding color and direct right-click palette remain local; root and child priority ordering are independent.");
    }
    internal void TestEditorDirtyEquivalence() {
        string retained="/api/v1/tasks/5/attachments/9";
        string before="<P style='margin: 0'>Saved text</P><IMG src='data:image/png;base64,AA==' data-tasktrace-src='"+retained+"'>";
        string after="<p>Saved text</p><img data-tasktrace-src='"+retained+"' src='data:image/png;base64,changed'>";
        Func<string,string> snapshot=html=>ProgressSnapshotHtml(html,5)+"|"+String.Join(",",ProgressEditorImageKeys(html));
        if(snapshot(before)!=snapshot(after) || !SameProgressEditorContent(before,after) || SameProgressEditorContent(before,"<p>Changed text</p>"))throw new Exception("Editor save baseline did not ignore browser-only HTML normalization");
        Console.WriteLine("PASS editor state: browser HTML normalization and retained image previews do not create false unsaved prompts after saving.");
    }
}
internal static class FloatingPerformanceTests {
    [STAThread] static int Main() {
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            using(var window=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory)){window.TestOutstandingReadsForPerformance().GetAwaiter().GetResult();window.TestLocalAppearanceAndPriorityOrdering();window.TestEditorDirtyEquivalence();}
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
                ((FloatingWindow.TaskNode)node).EffectiveColorKey="mist-blue";surface.Rebuild(true);if(surface.LayoutBuildCount!=builds || FloatingWindow.AppearanceColor("mist-blue").IsEmpty)throw new Exception("Local background color remeasured text or was not resolved");
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
                model.Nodes.Clear();model.SingleLinePaths=false;model.SetWrappedText(true);host.ClientSize=new Size(520,210);
                var parent=new FloatingWindow.TaskNode("1. [P2] \u672c\u673a\u914d\u8272\u7684\u7236\u4efb\u52a1"){Tag=2001L,CurrentTextLength=17,EffectiveColorKey="mist-blue"};
                var child=new FloatingWindow.TaskNode("1.1. [P4] \u7ee7\u627f\u7236\u4efb\u52a1\u80cc\u666f\u8272\u7684\u5b50\u4efb\u52a1"){Tag=2002L,CurrentTextLength=24,EffectiveColorKey="mist-blue"};
                var outstanding=new TreeNode("1. [P1] \u4f7f\u7528\u72ec\u7acb\u67d4\u7c89\u80cc\u666f\u7684\u9057\u7559\u4e8b\u9879"){Tag=new FloatingWindow.OutstandingLeaf{TaskId=2002L,Id="preview",Html="local outstanding",Priority=1,CurrentTextLength=23,LocalColorKey="soft-rose",EffectiveColorKey="soft-rose"}};
                child.Nodes.Add(outstanding);parent.Nodes.Add(child);model.Nodes.Add(parent);parent.Expand();child.Expand();surface.Rebuild(false);Application.DoEvents();
                using(var preview=new Bitmap(surface.Width,surface.Height)){surface.DrawToBitmap(preview,new Rectangle(Point.Empty,preview.Size));preview.Save(System.IO.Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"floating-appearance-preview.png"));}
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
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
$exitCode=$process.ExitCode;if($null -eq $exitCode){$exitCode=if((Get-Item -LiteralPath $stderr).Length -eq 0){0}else{1}}
if($exitCode -ne 0){Get-Content -LiteralPath $stderr;throw ('Floating performance test failed: '+$exitCode)}
Write-Output ('Results: '+$stdout)
