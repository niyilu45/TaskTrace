param([string]$OutputDirectory = '.local-build/floating-completion-tests')
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
using System.Drawing;
using System.Linq;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    internal FloatingWindow(string isolatedDirectory) {root=isolatedDirectory;data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=true;}
    static void AssertCompletion(bool condition,string message){if(!condition)throw new Exception(message);}
    internal void TestCompletion() {
        AssertCompletion(!showCompletion.Checked,"Completion visibility should default off");
        AssertCompletion(TaskManualCompletion(new Dictionary<string,object>())==0,"New or old tasks must default to zero");
        AssertCompletion(EffectiveCompletion(40,true)==100 && EffectiveCompletion(40,false)==40,"Done overwrote manual value");
        var menu=CreateProgressMenu();AssertCompletion(menu.DropDownItems.Count==11,"Expected 11 choices");
        for(int i=0;i<=10;i++)AssertCompletion((int)menu.DropDownItems[i].Tag==i*10,"Expected 10 percent steps");menu.Dispose();
        var all=new Dictionary<long,Dictionary<string,object>>{
            {1,new Dictionary<string,object>{{"title","Parent"},{"priority",3},{"percent_done",0.4}}},
            {2,new Dictionary<string,object>{{"title","Top"},{"priority",10},{"percent_done",0.6}}},
            {3,new Dictionary<string,object>{{"title","Child"},{"priority",3},{"percent_done",0.2}}}
        };
        var parent=new TaskNode("parent"){Tag=1L};var top=new TaskNode("top"){Tag=2L};var child=new TaskNode("child"){Tag=3L};parent.Nodes.Add(child);
        var leaf=new TreeNode("item"){Tag=new OutstandingLeaf{TaskId=1,Id="one",Completion=80,Priority=7}};
        var roots=new List<TreeNode>{parent,top};completionSort=new List<string>{"completion:desc"};SortCompletionHierarchy(roots,all);
        AssertCompletion(roots[0]==top && parent.Nodes[0]==child,"Completion sort broke hierarchy");
        completionSort=new List<string>{"priority:asc","completion:desc"};
        AssertCompletion(OrderCompletionNodes(new[]{parent,top,leaf},all).SequenceEqual(new[]{top,leaf,parent}),"Two-level sort incorrect");
        completionSort=new List<string>{"priority:desc","completion:asc"};
        AssertCompletion(OrderCompletionNodes(new[]{parent,top,leaf},all).SequenceEqual(new[]{parent,leaf,top}),"Reverse two-level sort incorrect");
        top.Checked=true;completionSort=new List<string>{"completion:desc"};AssertCompletion(OrderCompletionNodes(new[]{leaf,top},all)[0]==top,"Sort ignored Done overlay");
        top.Checked=false;AssertCompletion(OrderCompletionNodes(new[]{leaf,top},all)[0]==leaf,"Uncheck did not restore manual ordering");
        showCompletion.Checked=true;NumberTasks(roots,all);AssertCompletion(parent.Text.StartsWith("2. 40% [P7]"),"Task completion prefix missing");
        var list=ReadShared(new List<Dictionary<string,object>>{new Dictionary<string,object>{{"id",1},{"comment","<h3>"+SharedHeading+"</h3><ul><li data-id=\"one\" data-progress=\"40\" data-done=\"true\">Item</li><li data-id=\"two\">Old</li></ul>"}}});
        AssertCompletion(list.Items[0].Completion==40 && list.Items[0].Done && list.Items[1].Completion==0,"Outstanding parser lost stored completion");
        SaveSortPreference();showCompletion.Checked=false;completionSort.Clear();InitializeCompletion();
        AssertCompletion(showCompletion.Checked && completionSort.SequenceEqual(new[]{"completion:desc"}),"Preferences did not roundtrip");
        Console.WriteLine("PASS default zero, Done overlay, 11 choices, two-level ascending/descending and tree sorting, outstanding metadata and saved preferences.");
    }
}
internal sealed partial class TaskTreeSurface {
    internal void AssertCompletionGeometry(){foreach(var row in rows){if(row.PrefixText.Contains("%") && (row.Prefix.Right>row.Priority.Left || (row.ImageAvailable && row.Prefix.Right>row.Image.Left)))throw new Exception("Completion overlaps image or priority");}}
}
internal static class FloatingCompletionTests {
    [STAThread] static int Main() {
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            using(var window=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory))window.TestCompletion();
            using(var host=new Form{Text="TaskTrace completion preview",ShowInTaskbar=false,ClientSize=new Size(760,240)})
            using(var model=new TaskTreeView())
            using(var surface=new TaskTreeSurface{Dock=DockStyle.Fill}) {
                model.Font=new Font("Microsoft YaHei UI",10);model.WrapNodeText=true;model.ItemHeight=28;
                model.SetSimpleImageLinks(true);model.SimpleImageAvailable=node=>node.Tag is FloatingWindow.OutstandingLeaf;
                var task=new FloatingWindow.TaskNode("1. 40% [P7] 测试任务：完成勾选只覆盖显示值，保留填写的完成度"){Tag=1L};
                task.Nodes.Add(new TreeNode("1.1. 80% [P7] 带图片的遗留事项：长文字换行后，完成度与图片、优先级不重叠"){Tag=new FloatingWindow.OutstandingLeaf{TaskId=1,Id="one",Completion=80,Priority=7}});
                model.Nodes.Add(task);model.Nodes.Add(new FloatingWindow.TaskNode("2. 100% [P3] 已完成任务（手动填写 60%）"){Tag=2L,Checked=true});task.Expand();
                host.Controls.Add(surface);surface.Bind(model);host.Show();Application.DoEvents();
                foreach(int width in new[]{760,350}){host.ClientSize=new Size(width,240);Application.DoEvents();surface.Rebuild(true);surface.AssertCompletionGeometry();using(var bitmap=new Bitmap(width,240)){surface.DrawToBitmap(bitmap,new Rectangle(0,0,width,240));bitmap.Save(System.IO.Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"completion-"+width+".png"));}}
                host.Close();
            }
            Console.WriteLine("PASS completion/image/priority layout at wide and narrow widths.");return 0;
        }catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
'@
$harness=Join-Path $testRoot 'FloatingCompletionTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingCompletionTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingCompletionTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Floating completion test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){ $process.Kill();throw 'Floating completion test timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
$exitCode=$process.ExitCode;if($null -eq $exitCode){$exitCode=if((Get-Item -LiteralPath $stderr).Length -eq 0){0}else{1}}
if($exitCode -ne 0){Get-Content -LiteralPath $stderr;throw ('Floating completion test failed: '+$exitCode)}
Write-Output ('Results: '+$stdout)
