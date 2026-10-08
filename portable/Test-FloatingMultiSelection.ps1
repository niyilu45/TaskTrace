param([string]$OutputDirectory = '.local-build/floating-multiselection-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Isolated native surface and in-memory HTTP handler only; never starts or writes to a user server/share.
$source = @'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;

internal sealed class SelectionApi : HttpMessageHandler {
    internal readonly Dictionary<long,Dictionary<string,object>> Tasks=new Dictionary<long,Dictionary<string,object>>();
    internal readonly Dictionary<long,string> Lists=new Dictionary<long,string>();
    internal readonly List<string> Writes=new List<string>(),Groups=new List<string>();
    internal readonly HashSet<long> Denied=new HashSet<long>();
    readonly JavaScriptSerializer json=new JavaScriptSerializer();
    internal int Reads;
    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancel) {
        string path=request.RequestUri.AbsolutePath.Substring("/api/v2".Length);string method=request.Method.Method;
        object response;
        if(path=="/tasktrace/undo")response=new{id=1,label="batch"};
        else {
            var parts=path.Split('/');if(parts.Length<3||parts[1]!="tasks")throw new Exception("Unexpected request: "+path);
            long id=Int64.Parse(parts[2]);
            if(method!="GET") {
                if(Denied.Contains(id))return new HttpResponseMessage(HttpStatusCode.Forbidden){Content=new StringContent("{}")};
                Writes.Add(method+" "+path);Groups.Add(request.Headers.GetValues("X-TaskTrace-Undo-Group").Single());
            }
            if(parts.Length==3) {
                if(method=="PATCH")foreach(var pair in json.Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync()))Tasks[id][pair.Key]=pair.Value;
                response=Tasks[id];
            } else if(parts[3]=="comments") {
                if(method=="GET") {Reads++;response=new{items=new[]{new{id=100+id,comment=Lists[id]}},total_pages=1};}
                else {Lists[id]=Convert.ToString(json.Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync())["comment"]);response=new{id=100+id};}
            } else throw new Exception("Unexpected request: "+path);
        }
        return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(json.Serialize(response))};
    }
}
internal sealed partial class FloatingWindow {
    internal FloatingWindow(string isolatedDirectory,SelectionApi api) {
        root=isolatedDirectory;data=isolatedDirectory;url="http://isolated.invalid";selfTest=true;token="test";
        http.Dispose();http=new HttpClient(api);taskSurface.ModelUpdating=delegate{return rendering;};taskSurface.Bind(tasks);
    }
    static void Check(bool condition,string message){if(!condition)throw new Exception(message);}
    static TreeNode SelectionTask(long id,string title){return new TaskNode(title){Tag=id,CurrentTextLength=title.Length};}
    static TreeNode SelectionLeaf(long id,string key,string title){return new TreeNode(title){Tag=new OutstandingLeaf{TaskId=id,Id=key,Html=title,CurrentTextLength=title.Length}};}
    internal void TestSelectionAndColors() {
        using(var host=new Form{ShowInTaskbar=false,Opacity=0,ClientSize=new Size(570,270)}) {
            host.Controls.Add(taskSurface);tasks.CreateControl();host.Show();Application.DoEvents();
            var one=SelectionTask(1,"1. [P7] 任务一");var two=SelectionLeaf(1,"two","2. [P7] 遗留事项二");var three=SelectionTask(3,"3. [P2] 任务三");var four=SelectionLeaf(3,"four","4. [P3] 遗留事项四");
            var hidden=SelectionTask(99,"不在查找结果中的任务");taskCache[99]=new Dictionary<string,object>{{"title",hidden.Text}};
            tasks.SingleLinePaths=true;tasks.Nodes.AddRange(new[]{one,two,three,four});taskSurface.Rebuild(false);
            Action<TreeNode,Keys> click=(node,keys)=>taskSurface.TestSelectionClick(taskSurface.TitleBounds(node),MouseButtons.Left,keys);
            click(one,Keys.None);click(three,Keys.Control);Check(SelectedActionNodes().SequenceEqual(new[]{one,three}),"Ctrl click did not add a disjoint row");
            click(three,Keys.Control);Check(SelectedActionNodes().SequenceEqual(new[]{one}),"Ctrl click did not deselect");
            click(one,Keys.None);click(four,Keys.Shift);Check(SelectedActionNodes().Count==4,"Shift range missed endpoints or rows");
            click(two,Keys.Shift);Check(SelectedActionNodes().SequenceEqual(new[]{one,two}),"Shift range did not keep its original anchor");
            click(four,Keys.Control);click(three,Keys.Control|Keys.Shift);Check(SelectedActionNodes().Count==4,"Ctrl+Shift range failed to add to the selection");
            taskSurface.TestSelectionClick(taskSurface.TitleBounds(two),MouseButtons.Right,Keys.None);Check(SelectedActionNodes().Count==4&&tasks.SelectedNode==two,"Right click collapsed a selected group");
            int completions=0,priorities=0;taskSurface.CompletionClicked=delegate{completions++;};taskSurface.PriorityClicked=delegate{priorities++;};
            taskSurface.TestSelectionClick(taskSurface.CheckBounds(two),MouseButtons.Left,Keys.Control);
            taskSurface.TestSelectionClick(taskSurface.PriorityBounds(three),MouseButtons.Left,Keys.Control);
            Check(completions==0&&priorities==0,"Modifier click changed completion/priority instead of selecting");
            click(one,Keys.None);taskSurface.TestSelectionClick(taskSurface.TitleBounds(two),MouseButtons.Right,Keys.None);Check(SelectedActionNodes().SequenceEqual(new[]{two}),"Right click on an unselected row retained unrelated selection");
            taskSurface.TestKey(Keys.Control|Keys.A);Check(SelectedActionNodes().Count==4&&!SelectedActionNodes().Contains(hidden),"Select all reached an unfiltered task cache");
            // Filtering replaces nodes while the refresh guard is active; retained IDs stay selected.
            rendering=true;tasks.Nodes.Clear();var freshOne=SelectionTask(1,one.Text);var freshTwo=SelectionLeaf(1,"two",two.Text);tasks.Nodes.AddRange(new[]{freshOne,freshTwo});tasks.SelectedNode=freshOne;rendering=false;
            taskSurface.Rebuild(true);Check(SelectedActionNodes().SequenceEqual(new[]{freshOne,freshTwo}),"Refresh/filter did not preserve and trim stable identities");
            taskSurface.TestKey(Keys.Control|Keys.A);Check(SelectedActionNodes().Count==2,"Filtered Ctrl+A selected hidden entries");
            tasks.SelectedNode=freshTwo;rendering=true;tasks.Nodes.Remove(freshTwo);rendering=false;taskSurface.Rebuild(true);Check(SelectedActionNodes().SequenceEqual(new[]{freshOne})&&tasks.SelectedNode==freshOne,"Filtering out the caret lost remaining selection or its keyboard target");
            tasks.Nodes.Add(freshTwo);taskSurface.Rebuild(true);taskSurface.TestKey(Keys.Control|Keys.A);
            using(var palette=CreateAppearanceMenuItem()) {
                RefreshAppearanceMenu(palette);((ToolStripMenuItem)palette.DropDownItems[2]).PerformClick();
                Check(((TaskNode)freshOne).EffectiveColorKey=="mint"&&((OutstandingLeaf)freshTwo.Tag).EffectiveColorKey=="mint","Batch color missed a flat root outstanding item");
                Check(LocalTaskAppearance(99)==""&&LocalTaskAppearance(3)==""&&LocalOutstandingAppearance(3,"four")=="","Batch color changed filtered-out rows");
                taskSurface.SelectItem(freshTwo,Keys.None,MouseButtons.Left);RefreshAppearanceMenu(palette);((ToolStripMenuItem)palette.DropDownItems[4]).PerformClick();
                Check(((OutstandingLeaf)freshTwo.Tag).EffectiveColorKey=="soft-rose","Flat outstanding right-click color did not repaint");
                ((ToolStripMenuItem)palette.DropDownItems[0]).PerformClick();Check(((OutstandingLeaf)freshTwo.Tag).EffectiveColorKey=="mint","Flat outstanding lost hidden-owner inheritance");
            }
            taskSurface.SelectItem(freshOne,Keys.None,MouseButtons.Left);taskSurface.SelectItem(freshOne,Keys.Control,MouseButtons.Left);taskSurface.Rebuild(true);Check(SelectedActionNodes().Count==0,"Deselected caret was reselected by a refresh");
            // Select-all covers offscreen rows too, but not children hidden by collapsing a branch.
            rendering=true;tasks.Nodes.Clear();tasks.SingleLinePaths=false;freshOne.Nodes.Add(freshTwo);tasks.Nodes.Add(freshOne);freshOne.Collapse();rendering=false;taskSurface.Rebuild(false);taskSurface.TestKey(Keys.Control|Keys.A);Check(SelectedActionNodes().SequenceEqual(new[]{freshOne}),"Collapsed children were modified by visible-row select-all");
            freshOne.Expand();taskSurface.Rebuild(true);taskSurface.TestKey(Keys.Control|Keys.A);ApplySelectedAppearance("lavender");Check(((OutstandingLeaf)freshTwo.Tag).EffectiveColorKey=="lavender","Tree and flat color behavior differs");
            LoadLocalAppearance();Check(LocalTaskAppearance(1)=="lavender"&&LocalOutstandingAppearance(1,"two")=="lavender","Local appearance did not survive reload");
            for(int i=0;i<40;i++)tasks.Nodes.Add(SelectionTask(200+i,(i+2)+". [P7] 用于验证多选与滚动的条目 "+i));
            taskSurface.Rebuild(true);taskSurface.TestScrollTo(0,120);var scroll=taskSurface.ScrollOffset;taskSurface.TestKey(Keys.Control|Keys.A);Check(SelectedActionNodes().Count==42&&taskSurface.ScrollOffset==scroll,"Select all omitted offscreen rows or moved scroll position");
            taskSurface.TestScrollTo(0,0);taskSurface.ClearSelection();taskSurface.SelectItem(freshOne,Keys.None,MouseButtons.Left);taskSurface.SelectItem(freshTwo,Keys.Control,MouseButtons.Left);
            Application.DoEvents();using(var bitmap=new Bitmap(taskSurface.Width,taskSurface.Height)){taskSurface.DrawToBitmap(bitmap,new Rectangle(Point.Empty,bitmap.Size));bitmap.Save(Path.Combine(data,"multiselection-preview.png"));}
            host.Controls.Remove(taskSurface);host.Close();
        }
        tasks.Nodes.Clear();taskSurface.Rebuild(false);
        Console.WriteLine("PASS Ctrl/Shift/Ctrl+Shift, right-click selection, modified hit targets, filtered/visible/offscreen Ctrl+A, refresh identities, deselection, flat/root color, inheritance and local persistence.");
    }
    internal async Task TestBatchWrites(SelectionApi api) {
        for(long id=1;id<=3;id++)api.Tasks[id]=new Dictionary<string,object>{{"id",id},{"title","Task "+id},{"done",false},{"priority",3},{"description","Keep description"}};
        string list="<h3 "+SharedTypeMarker+">"+SharedHeading+"</h3><ul><li data-id=\"a\" data-priority=\"7\">Alpha<img src=\"/image/a\"><aside data-tasktrace-outstanding-note=\"true\" hidden>Keep note</aside></li><li data-id=\"b\" data-priority=\"7\">Beta</li><li data-id=\"keep\" data-priority=\"4\" data-reminder=\"2030-01-01T12:00:00Z\">Do not change</li></ul>";
        api.Lists[1]=list;api.Lists[3]=list;
        var one=SelectionTask(1,"Task one");var a=SelectionLeaf(1,"a","Alpha");var b=SelectionLeaf(1,"b","Beta");
        var selected=new List<TreeNode>{one,a,b};
        var result=await SaveSelectedStates(selected,null,0);Check(result.Saved==3&&result.Failures.Count==0&&api.Writes.Count==2&&api.Reads==1,"Batch priority did not group outstanding writes by task");
        Check(Convert.ToInt32(api.Tasks[1]["priority"])==10&&api.Groups.Distinct().Count()==1&&!String.IsNullOrWhiteSpace(api.Groups[0])&&undoGroup==null,"Priority API conversion or undo grouping failed");
        var shared=ReadShared(await ReadHistory(1));Check(shared.Items[0].Priority==0&&shared.Items[1].Priority==0&&shared.Items[0].Html.Contains("<img")&&shared.Items[0].NoteHtml=="Keep note"&&shared.Items[2].Priority==4&&shared.Items[2].ReminderAt=="2030-01-01T12:00:00Z","Batch priority lost images, memo, reminders or unselected item content");
        int writes=api.Writes.Count;result=await SaveSelectedStates(selected,null,0);Check(result.Unchanged==3&&result.Saved==0&&api.Writes.Count==writes,"Unchanged priority generated duplicate writes");
        result=await SaveSelectedStates(selected,true,null);Check(result.Saved==3&&Convert.ToBoolean(api.Tasks[1]["done"]),"Batch completion missed task or outstanding entries");
        shared=ReadShared(await ReadHistory(1));Check(shared.Items[0].Done&&shared.Items[1].Done&&!shared.Items[2].Done&&!String.IsNullOrEmpty(shared.Items[0].CompletedAt),"Completion changed unselected entry or missed completion timestamps");
        result=await SaveSelectedStates(selected,false,null);shared=ReadShared(await ReadHistory(1));Check(result.Saved==3&&!shared.Items[0].Done&&String.IsNullOrEmpty(shared.Items[0].CompletedAt),"Batch reopen did not clear completion time");
        api.Denied.Add(2);api.Denied.Add(3);selected.Add(SelectionTask(2,"Read-only task"));selected.Add(SelectionLeaf(3,"a","Read-only outstanding"));selected.Add(SelectionLeaf(1,"missing","Moved outstanding"));
        result=await SaveSelectedStates(selected,null,9);Check(result.Saved==3&&result.Failures.Count==3&&Convert.ToInt32(api.Tasks[2]["priority"])==3,"Partial permission/missing-item failure was hidden or discarded valid writes");
        Check(!api.Lists[1].Contains("mint")&&!api.Lists[1].Contains("lavender")&&!api.Lists[1].Contains("soft-rose"),"Local appearance leaked into shared content");
        Console.WriteLine("PASS mixed task/outstanding batches, priorities 0/9, one owner write, no-op suppression, one undo group, completion/reopen, image/note/reminder preservation, per-item failures and local-only colors.");
    }
}
internal static class FloatingMultiSelectionTests {
    [STAThread] static int Main() {
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            using(var api=new SelectionApi())using(var window=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory,api)){window.TestSelectionAndColors();window.TestBatchWrites(api).GetAwaiter().GetResult();}
            return 0;
        }catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
'@
$harness=Join-Path $testRoot 'FloatingMultiSelectionTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingMultiSelectionTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingMultiSelectionTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Floating multiselection test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){$process.Kill();throw 'Floating multiselection test timed out.'}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
$exitCode=$process.ExitCode;if($null -eq $exitCode){$exitCode=if((Get-Item -LiteralPath $stderr).Length -eq 0){0}else{1}}
if($exitCode -ne 0){Get-Content -LiteralPath $stderr;throw ('Floating multiselection test failed: '+$exitCode)}
Write-Output ('Results: '+$stdout)
