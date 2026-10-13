$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = Join-Path $repoRoot ('.local-build/floating-visibility-tests/' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
$source = '
using System;
using System.IO;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Drawing;
using System.Net;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;
using System.Web;
using System.Web.Script.Serialization;
using System.Windows.Forms;
internal sealed class VisibilityApi : HttpMessageHandler {
    internal readonly List<Dictionary<string,object>> Tasks=new List<Dictionary<string,object>>();
    internal readonly Dictionary<long,List<Dictionary<string,object>>> Notes=new Dictionary<long,List<Dictionary<string,object>>>();
    internal int TaskPages,CommentReads;
    internal VisibilityApi(){
        for(int id=1;id<=210;id++)Tasks.Add(new Dictionary<string,object>{{"id",id},{"title","task "+id},{"done",id==10},{"priority",3},{"comment_count",0}});
        Tasks[1]["related_tasks"]=new Dictionary<string,object>{{"parenttask",new[]{new Dictionary<string,object>{{"id",1}}}}};
        Tasks[2]["related_tasks"]=new Dictionary<string,object>{{"parenttask",new[]{new Dictionary<string,object>{{"id",2}}}}};
        AddNote(1,"root-leaf","Root pending",1,false);AddNote(2,"child-leaf","Needle pending",2,false);AddNote(10,"done-owner-leaf","Still unfinished",2,false);
    }
    internal void AddNote(long task,string id,string title,int priority,bool done) {
        Notes[task]=new List<Dictionary<string,object>>{new Dictionary<string,object>{{"id",task*100},{"comment","<h3 data-tasktrace-comment-type=\"outstanding\">TaskTrace 遗留事项清单</h3><ul><li data-id=\""+id+"\" data-priority=\""+priority+"\" data-done=\""+done.ToString().ToLowerInvariant()+"\">"+title+"</li></ul>"}}};
        Tasks.Single(t=>Convert.ToInt64(t["id"])==task)["comment_count"]=1;
    }
    protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancellation) {
        if(request.Method!=HttpMethod.Get)throw new Exception("Visibility test tried to write data: "+request.RequestUri);
        string path=request.RequestUri.AbsolutePath;var q=HttpUtility.ParseQueryString(request.RequestUri.Query);int p=q["page"]==null?1:Int32.Parse(q["page"]);
        object result;
        if(path.EndsWith("/projects"))result=new{items=new[]{new{id=1,title="Fixture project"}},total=1};
        else if(path.EndsWith("/views"))result=new{items=new object[0]};
        else if(path.EndsWith("/tasks")){TaskPages++;result=new{items=Tasks.OrderByDescending(t=>Convert.ToInt64(t["id"])).Skip((p-1)*100).Take(100).ToArray(),total_pages=3,total=Tasks.Count};}
        else if(path.EndsWith("/comments")){
            CommentReads++;long id=Int64.Parse(path.Split(''/'')[4]);List<Dictionary<string,object>> history;
            if(!Notes.TryGetValue(id,out history))history=new List<Dictionary<string,object>>();
            result=new{items=history.ToArray(),total_pages=1,total=history.Count};
        } else throw new Exception("Unexpected fixture read: "+path);
        return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(new JavaScriptSerializer().Serialize(result))});
    }
}
internal sealed partial class FloatingWindow {
    internal FloatingWindow(string directory,VisibilityApi api) {
        root=directory;data=directory;url="http://127.0.0.1:1";selfTest=true;
        http.Dispose();http=new HttpClient(api);token="fixture";
        projects.Items.Add(new Project{Id=1,Title="Fixture project"});projects.SelectedIndex=0;projectsDirty=false;
        ShowInTaskbar=false;Opacity=0;ClientSize=new Size(450,300);
        Controls.Add(tasks);tasks.CreateControl();taskSurface.Bind(tasks);showCompleted.Checked=true;
    }
    bool HasVisibleLeaf(string id) {return OutstandingDescendants(tasks.Nodes).Any(node=>node.Tag is OutstandingLeaf && ((OutstandingLeaf)node.Tag).Id==id);}
    async Task ReadVisibility(bool background=false) {
        if(!await LoadTasks(background))throw new Exception("Current load was cancelled.");
        taskSurface.Rebuild(false);
        var nodes=OutstandingDescendants(tasks.Nodes).ToList();
        if(nodes.GroupBy(RefreshNodeKey).Any(group=>group.Count()>1))throw new Exception("Refresh duplicated task/outstanding entries.");
    }
    void ExpectTaskIds(params long[] expected) {
        var actual=SimpleTaskNodes(tasks.Nodes).Select(node=>(long)node.Tag).OrderBy(id=>id);
        if(!actual.SequenceEqual(expected.OrderBy(id=>id)))throw new Exception("Task ancestor/sibling filtering failed: "+String.Join(",",actual));
    }
    internal async Task TestNewChildrenAndLegacyLists(VisibilityApi api) {
        visiblePriorities.Clear();visiblePriorities.UnionWith(Enumerable.Range(0,10));showCompleted.Checked=true;search.Text="";
        singleLine.Checked=false;tasks.SingleLinePaths=false;
        await ReadVisibility();
        var failures=new List<string>();
        api.Tasks.Add(new Dictionary<string,object>{{"id",211},{"title","Newly shared child"},{"done",false},{"priority",3},{"comment_count",0},
            {"related_tasks",new Dictionary<string,object>{{"parenttask",new[]{new Dictionary<string,object>{{"id",4}}}}}}});
        api.AddNote(5,"new-leaf","Newly shared outstanding",7,false);
        await ReadVisibility(true);
        var child=tasks.Nodes.Find("211",true).Single();
        var outstanding=OutstandingDescendants(tasks.Nodes).Single(n=>n.Tag is OutstandingLeaf && ((OutstandingLeaf)n.Tag).Id=="new-leaf");
        if(taskSurface.NodeBounds(child).IsEmpty)failures.Add("First child under a previously childless task is incorrectly collapsed during background refresh.");
        if(taskSurface.NodeBounds(outstanding).IsEmpty)failures.Add("First outstanding item under a previously childless task is incorrectly collapsed during background refresh.");
        api.AddNote(6,"quoted","Single quoted item",2,false);
        api.Notes[6][0]["comment"]="<h3>TaskTrace 遗留事项清单</h3><ul><li data-id=''quoted'' data-priority=''2''>Single quoted item</li><li data-id=bare data-priority=3>Bare attribute item</li><li>Legacy ID-less item</li></ul>";
        api.AddNote(7,"mentioned-heading","Saved outstanding",7,false);
        api.Notes[7].Add(new Dictionary<string,object>{{"id",9999},{"comment","<p>检查 TaskTrace 遗留事项清单 后继续</p>"}});
        await ReadVisibility();
        foreach(string id in new[]{"quoted","bare","item-2","mentioned-heading"})if(!HasVisibleLeaf(id))failures.Add("Saved outstanding item is not parsed: "+id);
        if(failures.Count>0)throw new Exception(String.Join(Environment.NewLine,failures));
        var parent=tasks.Nodes.Find("4",true).Single();parent.Collapse();collapsedTasks.Add(4);
        api.AddNote(4,"collapsed-leaf","Keep manually collapsed",7,false);await ReadVisibility(true);
        if(tasks.Nodes.Find("4",true).Single().IsExpanded)throw new Exception("A user-collapsed branch was expanded by new outstanding content.");
        foreach(bool simple in new[]{false,true}) {
            simpleMode=simple;await ReadVisibility();
            foreach(string id in new[]{"quoted","bare","item-2","mentioned-heading","new-leaf"})
                if(!HasVisibleLeaf(id))throw new Exception("A parsed item is missing in full/simple tree mode: "+id);
            singleLine.Checked=true;tasks.SingleLinePaths=true;await ReadVisibility();
            foreach(string id in new[]{"quoted","bare","item-2","mentioned-heading","new-leaf"})
                if(!HasVisibleLeaf(id))throw new Exception("A parsed item is missing in flat mode: "+id);
            singleLine.Checked=false;tasks.SingleLinePaths=false;
        }
        var nested=ReadShared(new List<Dictionary<string,object>>{new Dictionary<string,object>{{"id",1},{"comment",
            "<h3>TaskTrace <span>遗留事项清单</span></h3><ul><li data-id=''a&amp;b'' title=''a > b'' data-done=true data-priority=2 data-progress=30 data-reminder=''2026-12-01T09:00:00Z''>Main<ul><li data-id=nested>Nested bullet</li></ul><img src=''/image/1''><aside data-tasktrace-outstanding-note=true hidden>Memo<ol><li>Memo bullet</li></ol><img src=''/image/2''></aside></li><li data-id=second>Next<li data-id=third>Last</ul>"}}});
        var first=nested.Items.First();
        if(nested.Items.Count!=3 || first.Id!="a&b" || !first.Done || first.Priority!=2 || first.Completion!=30 || first.ReminderAt!="2026-12-01T09:00:00Z" ||
            !first.Html.Contains("Nested bullet") || !first.Html.Contains("/image/1") || first.Html.Contains("Memo") || !first.NoteHtml.Contains("/image/2") ||
            nested.Items[1].Html!="Next" || nested.Items[2].Html!="Last")
            throw new Exception("Nested bullets/images/notes/metadata or optional list closing tags were lost.");
        if(IsSharedComment("<p title=''mention data-tasktrace-comment-type=outstanding''>Normal</p>"))
            throw new Exception("An attribute-looking phrase in a normal comment became a shared list.");
        Console.WriteLine("PASS: new synchronized children are visible, manual collapse survives; legacy lists, nested bullets, images, memo, metadata and ordinary heading mentions work in full/simple/tree/flat modes.");
    }
    internal async Task TestVisibility(VisibilityApi api) {
        await ReadVisibility();
        if(SimpleTaskNodes(tasks.Nodes).Count()!=210 || api.TaskPages!=3 || !HasVisibleLeaf("child-leaf"))throw new Exception("Task/comment pagination dropped entries.");
        Console.WriteLine("PASS: all 210 tasks and nested outstanding rows are loaded across three task pages.");
        var failures=new List<string>();
        showCompleted.Checked=false;await ReadVisibility();
        if(!HasVisibleLeaf("done-owner-leaf"))failures.Add("Hiding completed tasks also hides their unfinished outstanding items and their parent context.");
        showCompleted.Checked=true;search.Text="Needle";await ReadVisibility();
        if(!HasVisibleLeaf("child-leaf") || !tasks.Nodes.Find("1",true).Any())failures.Add("Searching outstanding text loses the item and its task ancestors.");
        search.Text="";showCompleted.Checked=false;visiblePriorities.Clear();visiblePriorities.Add(2);await ReadVisibility();
        if(!HasVisibleLeaf("done-owner-leaf") || !HasVisibleLeaf("child-leaf"))failures.Add("A matching outstanding priority is lost when its owner is completed.");
        if(failures.Count>0)throw new Exception(String.Join(Environment.NewLine,failures));
        ExpectTaskIds(1,2,10);
        if(HasVisibleLeaf("root-leaf"))throw new Exception("An unrelated-priority leaf leaked into the filtered tree.");
        Console.WriteLine("PASS: unfinished outstanding entries retain completed parents; outstanding search and priority filters preserve ancestors without unrelated siblings.");
        foreach(bool simple in new[]{false,true}) {
            simpleMode=simple;visiblePriorities.Clear();visiblePriorities.UnionWith(Enumerable.Range(0,10));showCompleted.Checked=true;search.Text="";
            await ReadVisibility();
            foreach(var node in OutstandingDescendants(tasks.Nodes))if(taskSurface.NodeBounds(node).IsEmpty)throw new Exception("An expanded tree entry has no rendered row.");
            var parent=tasks.Nodes.Find("1",true).Single();parent.Collapse();
            api.AddNote(10,"done-owner-leaf","Updated pending",2,false);
            await ReadVisibility(true);
            if(tasks.Nodes.Find("1",true).Single().IsExpanded)throw new Exception("Background refresh discarded intentional collapse state.");
            parent=tasks.Nodes.Find("1",true).Single();parent.Expand();
            api.AddNote(2,"child-leaf","Needle updated",2,false);
            await ReadVisibility(true);
            if(!HasVisibleLeaf("child-leaf") || !tasks.Nodes.Find("1",true).Single().IsExpanded)throw new Exception("Outstanding update lost an entry or expanded branch.");
            showCompleted.Checked=false;search.Text="Needle";await ReadVisibility();ExpectTaskIds(1,2);
            if(HasVisibleLeaf("root-leaf") || HasVisibleLeaf("done-owner-leaf"))throw new Exception("Search kept unrelated outstanding rows.");
            singleLine.Checked=true;tasks.SingleLinePaths=true;await ReadVisibility();
            if(tasks.Nodes.Count!=1 || !(tasks.Nodes[0].Tag is OutstandingLeaf) || ((OutstandingLeaf)tasks.Nodes[0].Tag).Id!="child-leaf")throw new Exception("Flat search lost a matching leaf or retained unmatched ancestors as rows.");
            search.Text="";visiblePriorities.Clear();visiblePriorities.Add(2);await ReadVisibility();
            if(tasks.Nodes.Count!=2 || !HasVisibleLeaf("done-owner-leaf"))throw new Exception("Flat priority filter dropped matching outstanding entries.");
            singleLine.Checked=false;tasks.SingleLinePaths=false;
        }
        simpleMode=false;search.Text="";visiblePriorities.Clear();visiblePriorities.UnionWith(Enumerable.Range(0,10));showCompleted.Checked=false;
        api.AddNote(10,"done-owner-leaf","Finished pending",2,true);await ReadVisibility();
        if(HasVisibleLeaf("done-owner-leaf") || tasks.Nodes.Find("10",true).Any())throw new Exception("An entirely completed branch should remain hidden.");
        completedHideDelayMinutes=5;RememberOutstandingCompletion(10,"done-owner-leaf",true);await ReadVisibility();
        if(!HasVisibleLeaf("done-owner-leaf"))throw new Exception("A newly completed outstanding item lost its grace period because its owner is completed.");
        recentlyCompletedOutstanding[OutstandingCompletionKey(10,"done-owner-leaf")]=DateTime.UtcNow.AddMinutes(-6);await ReadVisibility();
        if(HasVisibleLeaf("done-owner-leaf"))throw new Exception("Expired completion grace period retained a leaf.");
        showCompleted.Checked=true;await ReadVisibility();if(!HasVisibleLeaf("done-owner-leaf"))throw new Exception("Show completed did not restore the branch.");
        visiblePriorities.Clear();await ReadVisibility();if(tasks.Nodes.Count!=0)throw new Exception("Empty priority selection should show no rows.");
        Console.WriteLine("PASS: full/simple shared tree, rendered rows, flat search, completion grace period, explicit filters and repeated background updates preserve correct entries and expansion state.");
    }
}
internal static class FloatingVisibilityTests {
    [STAThread] static int Main(){
        Application.EnableVisualStyles();var api=new VisibilityApi();int result=0;
        using(var app=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory,api)){
            app.Shown+=async delegate{try{await app.TestVisibility(api);await app.TestNewChildrenAndLegacyLists(api);}catch(Exception ex){Console.Error.WriteLine(ex);result=1;}finally{app.Close();}};
            Application.Run(app);
        }
        return result;
    }
}
'
$harness=Join-Path $testRoot 'FloatingVisibilityTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingVisibilityTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingVisibilityTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Visibility test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(30000)){ $process.Kill();throw 'Visibility test timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr;throw 'Visibility test failed'}
Write-Output ('Results: '+$stdout)
