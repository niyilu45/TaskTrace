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
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;

internal sealed class SelectionApi : HttpMessageHandler {
    internal readonly Dictionary<long,Dictionary<string,object>> Tasks=new Dictionary<long,Dictionary<string,object>>();
    internal readonly Dictionary<long,string> Lists=new Dictionary<long,string>();
    internal readonly List<string> Writes=new List<string>(),Groups=new List<string>();
    internal readonly HashSet<long> Denied=new HashSet<long>(),PriorityDenied=new HashSet<long>();
    readonly JavaScriptSerializer json=new JavaScriptSerializer();
    internal int Reads;
    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancel) {
        string path=request.RequestUri.AbsolutePath.Substring("/api/v2".Length);string method=request.Method.Method;
        object response;
        if(path=="/tasktrace/undo")response=new{id=1,label="batch"};
        else if(path.StartsWith("/tasktrace/tasks/")&&path.EndsWith("/priority")) {
            var parts=path.Split('/');long id=Int64.Parse(parts[3]);
            if(method!="PUT")throw new Exception("Personal priority must use PUT");
            if(PriorityDenied.Contains(id))return new HttpResponseMessage(HttpStatusCode.Forbidden){Content=new StringContent("{}")};
            var payload=json.Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync());int priority=Convert.ToInt32(payload["priority"]);object key;
            if(payload.TryGetValue("outstanding_id",out key)) {
                string pattern="(<li[^>]*data-id=\""+Regex.Escape(Convert.ToString(key))+"\"[^>]*data-priority=\")"+"[0-9]+"+"(\")";
                Lists[id]=Regex.Replace(Lists[id],pattern,match=>match.Groups[1].Value+priority+match.Groups[2].Value);
            } else Tasks[id]["priority"]=10-priority;
            Writes.Add(method+" "+path);Groups.Add(request.Headers.GetValues("X-TaskTrace-Undo-Group").Single());
            response=new{task_id=id,priority=priority,stored_priority=10-priority};
        }
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

    internal void TestTaskDragRedirect(SelectionApi api) {
        InitializeTaskSurface();
        int notices=0,reads=api.Reads,writes=api.Writes.Count;
        taskDragNotice=delegate(string text){Check(text=="请在网页模式概览中拖动改变任务所属关系","Wrong task drag guidance");notices++;};
        var flags=System.Reflection.BindingFlags.Instance|System.Reflection.BindingFlags.NonPublic;
        var down=typeof(TaskTreeSurface).GetMethod("OnMouseDown",flags);
        var move=typeof(TaskTreeSurface).GetMethod("OnMouseMove",flags);
        var up=typeof(TaskTreeSurface).GetMethod("OnMouseUp",flags);
        using(var host=new Form{ShowInTaskbar=false,Opacity=0,ClientSize=new Size(570,270)}) {
            host.Controls.Add(taskSurface);host.Show();Application.DoEvents();
            foreach(bool simple in new[]{false,true})foreach(bool flat in new[]{false,true}) {
                simpleMode=simple;tasks.SingleLinePaths=flat;rendering=true;tasks.Nodes.Clear();
                var task=SelectionTask(81,"1. [P7] 拖动任务提示");var leaf=SelectionLeaf(81,"item","2. [P7] 遗留事项");
                tasks.Nodes.AddRange(new[]{task,leaf});rendering=false;taskSurface.Rebuild(false);
                Rectangle title=taskSurface.TitleBounds(task);int x=title.Left+8,y=title.Top+title.Height/2,before=notices;
                down.Invoke(taskSurface,new object[]{new MouseEventArgs(MouseButtons.Left,1,x,y,0)});
                move.Invoke(taskSurface,new object[]{new MouseEventArgs(MouseButtons.Left,0,x+30,y,0)});
                move.Invoke(taskSurface,new object[]{new MouseEventArgs(MouseButtons.Left,0,x+40,y,0)});
                up.Invoke(taskSurface,new object[]{new MouseEventArgs(MouseButtons.Left,1,x+40,y,0)});
                Check(notices==before+1,"A single task drag must show one notice, even with continued mouse movement");
                Check(!dragging&&task.Parent==null,"Rejected task drag left active dragging or changed the tree");
                Check(AllowFloatingItemDrag(leaf),"Outstanding item dragging was disabled");
                before=notices;
                typeof(TreeView).GetMethod("OnItemDrag",flags).Invoke(tasks,new object[]{new ItemDragEventArgs(MouseButtons.Left,task)});
                Check(notices==before+1,"Legacy tree did not show task drag guidance");
            }
            host.Controls.Remove(taskSurface);host.Close();
        }
        Check(api.Reads==reads&&api.Writes.Count==writes,"A rejected task drag accessed the server");
        taskDragNotice=null;simpleMode=false;tasks.Nodes.Clear();taskSurface.Rebuild(false);
        Console.WriteLine("PASS full/simple and tree/flat task drag guidance once per gesture, legacy tree, no API writes and retained outstanding dragging.");
    }
    internal void TestBranchMenu(SelectionApi api) {
        InitializeInteractions();taskSurface.ContextMenuStrip=tasks.ContextMenuStrip;
        var menu=tasks.ContextMenuStrip;
        var expand=menu.Items.Cast<ToolStripItem>().Single(item=>item.Text=="全部展开");
        var collapse=menu.Items.Cast<ToolStripItem>().Single(item=>item.Text=="全部收起");
        Action prepare=delegate {
            var args=new System.ComponentModel.CancelEventArgs();
            typeof(ContextMenuStrip).GetMethod("OnOpening",System.Reflection.BindingFlags.Instance|System.Reflection.BindingFlags.NonPublic).Invoke(menu,new object[]{args});
        };
        int writes=api.Writes.Count,reads=api.Reads;
        using(var host=new Form{ShowInTaskbar=false,Opacity=0,ClientSize=new Size(570,300)}) {
            host.Controls.Add(taskSurface);host.Show();Application.DoEvents();
            foreach(bool simple in new[]{false,true}) {
                simpleMode=simple;rendering=true;tasks.Nodes.Clear();collapsedTasks.Clear();simpleCollapsedDuringRead.Clear();
                singleLine.Checked=false;tasks.SingleLinePaths=false;
                var root=SelectionTask(11,"1. [P7] 选中任务");
                var child=SelectionTask(12,"1.1. [P7] 子任务");
                var grandchild=SelectionTask(13,"1.1.1. [P7] 下级任务");
                var leaf=SelectionLeaf(13,"nested","1. [P7] 遗留事项");
                grandchild.Nodes.Add(leaf);child.Nodes.Add(grandchild);root.Nodes.Add(child);
                var other=SelectionTask(21,"2. [P7] 未选中任务");var otherLeaf=SelectionLeaf(21,"other","1. [P7] 另一条遗留事项");other.Nodes.Add(otherLeaf);
                tasks.Nodes.AddRange(new[]{root,other});tasks.CollapseAll();rendering=false;taskSurface.Rebuild(false);
                taskSurface.SelectItem(root,Keys.None,MouseButtons.Left);prepare();
                Check(expand.Enabled&&collapse.Enabled,"Branch commands are not available for a selected collapsed task.");
                expand.PerformClick();
                Check(root.IsExpanded&&child.IsExpanded&&grandchild.IsExpanded&&!other.IsExpanded&&!taskSurface.NodeBounds(leaf).IsEmpty,"Expand did not include descendants or changed an unselected branch.");
                taskSurface.SelectItem(child,Keys.Control,MouseButtons.Left);prepare();collapse.PerformClick();
                Check(!root.IsExpanded&&!child.IsExpanded&&!grandchild.IsExpanded&&!other.IsExpanded&&SelectedActionNodes().SequenceEqual(new[]{root}),"Collapse lost parent selection, skipped overlapping descendants, or changed unselected branch.");
                collapsedTasks.Clear();LoadTreePreferences();
                Check(new long[]{11,12,13}.All(collapsedTasks.Contains)&&!collapsedTasks.Contains(21),"Branch preferences were not saved locally.");
                prepare();expand.PerformClick();Check(!collapsedTasks.Contains(11)&&!collapsedTasks.Contains(12)&&!simpleCollapsedDuringRead.Contains(13),"Expanded branches would be collapsed again by refresh.");

                // Ctrl+A selects displayed roots even when their descendants are folded.
                taskSurface.TestKey(Keys.Control|Keys.A);prepare();collapse.PerformClick();
                Check(!root.IsExpanded&&!other.IsExpanded,"Select-all collapse missed a root.");
                taskSurface.TestKey(Keys.Control|Keys.A);prepare();expand.PerformClick();
                Check(root.IsExpanded&&child.IsExpanded&&grandchild.IsExpanded&&other.IsExpanded,"Select-all expand missed folded descendants.");

                // A filtered-out task remains untouched, including its saved state.
                rendering=true;tasks.Nodes.Remove(other);rendering=false;taskSurface.Rebuild(true);
                collapsedTasks.Add(99);taskCache[99]=new Dictionary<string,object>{{"title","Filtered out"}};
                var savedBefore=collapsedTasks.Contains(21);
                search.Text="选中";taskSurface.TestKey(Keys.Control|Keys.A);prepare();collapse.PerformClick();
                Check(other.IsExpanded&&collapsedTasks.Contains(99)&&collapsedTasks.Contains(21)==savedBefore,"Filtered select-all changed a hidden task.");
                prepare();expand.PerformClick();search.Clear();
                taskSurface.SelectItem(leaf,Keys.None,MouseButtons.Left);prepare();Check(!expand.Enabled&&!collapse.Enabled,"Leaf-only selection has misleading enabled branch commands.");
                taskSurface.ClearSelection();prepare();Check(!expand.Enabled&&!collapse.Enabled,"Empty selection enables branch commands.");
                taskSurface.SelectItem(root,Keys.None,MouseButtons.Left);singleLine.Checked=true;prepare();Check(!expand.Enabled&&!collapse.Enabled,"Flat mode enables tree-only commands.");
                singleLine.Checked=false;
            }
            host.Controls.Remove(taskSurface);host.Close();
        }
        Check(api.Writes.Count==writes&&api.Reads==reads,"Expand/collapse touched shared task data.");
        tasks.Nodes.Clear();taskSurface.Rebuild(false);simpleMode=false;collapsedTasks.Clear();simpleCollapsedDuringRead.Clear();
        Console.WriteLine("PASS branch menu: partial/Ctrl+A selections, nested/overlapping tasks, filtered scope, local preference reload, full/simple parity, selection retention, leaf/empty/flat disabled states; no API writes.");
    }

    internal async Task TestStateMenu(SelectionApi api) {
        api.Tasks[51]=new Dictionary<string,object>{{"id",51},{"title","State task"},{"done",false},{"status","to-do"},{"percent_done",0.4}};
        var task=SelectionTask(51,"State task");tasks.Nodes.Clear();tasks.Nodes.Add(task);taskSurface.Rebuild(false);taskSurface.SelectItem(task,Keys.None,MouseButtons.Left);
        taskCache[51]=api.Tasks[51];using(var menu=CreateStateMenu()) {
            RefreshStateMenu(menu);Check(menu.DropDownItems.Count==4,"State menu must have four direct actions");
            foreach(string state in displayStates) {
                var result=await SaveSelectedStates(new List<TreeNode>{task},null,null,state);
                Check(result.Failures.Count==0&&TaskStatusValue(api.Tasks[51])==state&&Convert.ToBoolean(api.Tasks[51]["done"])==(state=="done"),"State action did not persist "+state);
                Check(Convert.ToDouble(api.Tasks[51]["percent_done"])==0.4,"State action erased manual completion");
                RefreshStateMenu(menu);Check(menu.DropDownItems.OfType<ToolStripMenuItem>().Single(item=>item.Checked).Tag.ToString()==state,"Menu checkmark does not reflect saved state");
                int before=api.Writes.Count;await SaveSelectedStates(new List<TreeNode>{task},null,null,state);Check(api.Writes.Count==before,"Repeated status creates duplicate writes");
            }
            var leaf=SelectionLeaf(51,"x","Outstanding");tasks.Nodes.Add(leaf);taskSurface.Rebuild(false);taskSurface.SelectItem(leaf,Keys.Control,MouseButtons.Left);
            RefreshStateMenu(menu);Check(menu.Text=="批量设置状态"&&menu.DropDownItems.OfType<ToolStripMenuItem>().Where(item=>item.Enabled).Select(item=>item.Tag.ToString()).SequenceEqual(new[]{"to-do","done"}),"Mixed selection offers unsupported outstanding statuses");
        }
        Check(!tasks.ContextMenuStrip.Items.OfType<ToolStripMenuItem>().Any(item=>item.Text.Contains("设置任务状态")||item.Text.Contains("设置完成状态")),"Duplicate modal status menu remains");
        Console.WriteLine("PASS direct four-state menu, saved checkmarks, batch compatibility, no-op writes, preserved manual completion and no duplicate status action.");
    }
    internal async Task TestBatchWrites(SelectionApi api) {
        for(long id=1;id<=3;id++)api.Tasks[id]=new Dictionary<string,object>{{"id",id},{"title","Task "+id},{"done",false},{"priority",3},{"description","Keep description"}};
        string list="<h3 "+SharedTypeMarker+">"+SharedHeading+"</h3><ul><li data-id=\"a\" data-priority=\"7\">Alpha<img src=\"/image/a\"><aside data-tasktrace-outstanding-note=\"true\" hidden>Keep note</aside></li><li data-id=\"b\" data-priority=\"7\">Beta</li><li data-id=\"keep\" data-priority=\"4\" data-reminder=\"2030-01-01T12:00:00Z\">Do not change</li></ul>";
        api.Lists[1]=list;api.Lists[3]=list;
        var one=SelectionTask(1,"Task one");var a=SelectionLeaf(1,"a","Alpha");var b=SelectionLeaf(1,"b","Beta");
        var selected=new List<TreeNode>{one,a,b};
        var result=await SaveSelectedStates(selected,null,0);Check(result.Saved==3&&result.Failures.Count==0&&api.Writes.Count==3&&api.Reads==1,"Batch priority did not use isolated per-item personal settings");
        Check(Convert.ToInt32(api.Tasks[1]["priority"])==10&&api.Groups.Distinct().Count()==1&&!String.IsNullOrWhiteSpace(api.Groups[0])&&undoGroup==null,"Priority API conversion or undo grouping failed");
        var shared=ReadShared(await ReadHistory(1));Check(shared.Items[0].Priority==0&&shared.Items[1].Priority==0&&shared.Items[0].Html.Contains("<img")&&shared.Items[0].NoteHtml=="Keep note"&&shared.Items[2].Priority==4&&shared.Items[2].ReminderAt=="2030-01-01T12:00:00Z","Batch priority lost images, memo, reminders or unselected item content");
        int writes=api.Writes.Count;result=await SaveSelectedStates(selected,null,0);Check(result.Unchanged==3&&result.Saved==0&&api.Writes.Count==writes,"Unchanged priority generated duplicate writes");
        result=await SaveSelectedStates(selected,true,null);Check(result.Saved==3&&Convert.ToBoolean(api.Tasks[1]["done"]),"Batch completion missed task or outstanding entries");
        shared=ReadShared(await ReadHistory(1));Check(shared.Items[0].Done&&shared.Items[1].Done&&!shared.Items[2].Done&&!String.IsNullOrEmpty(shared.Items[0].CompletedAt),"Completion changed unselected entry or missed completion timestamps");
        result=await SaveSelectedStates(selected,false,null);shared=ReadShared(await ReadHistory(1));Check(result.Saved==3&&!shared.Items[0].Done&&String.IsNullOrEmpty(shared.Items[0].CompletedAt),"Batch reopen did not clear completion time");
        api.Denied.Add(2);api.Denied.Add(3);selected.Add(SelectionTask(2,"Read-only task"));selected.Add(SelectionLeaf(3,"a","Read-only outstanding"));selected.Add(SelectionLeaf(1,"missing","Moved outstanding"));
        api.PriorityDenied.Add(3);
        result=await SaveSelectedStates(selected,null,9);Check(result.Saved==4&&result.Failures.Count==2&&Convert.ToInt32(api.Tasks[2]["priority"])==1,"Personal priority was blocked by read-only content permission, or per-item failure was hidden");
        api.PriorityDenied.Clear();
        var readOnlyItems=new List<TreeNode>{SelectionTask(2,"Read-only task"),SelectionLeaf(3,"a","Read-only outstanding")};
        result=await SaveSelectedStates(readOnlyItems,null,2);Check(result.Saved==2&&result.Failures.Count==0,"Read-only task/outstanding personal priority was denied");
        result=await SaveSelectedStates(readOnlyItems,true,null);Check(result.Saved==0&&result.Failures.Count==2,"Read-only personal priority support accidentally allowed shared content writes");
        Check(!api.Lists[1].Contains("mint")&&!api.Lists[1].Contains("lavender")&&!api.Lists[1].Contains("soft-rose"),"Local appearance leaked into shared content");
        Console.WriteLine("PASS mixed task/outstanding batches, personal priorities 0/9 for writable and read-only shared entries, no-op suppression, one undo group, completion/reopen, image/note/reminder preservation, per-item failures and local-only colors.");
    }
}
internal static class FloatingMultiSelectionTests {
    [STAThread] static int Main() {
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            using(var api=new SelectionApi())using(var window=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory,api)){
                int result=0;window.ShowInTaskbar=false;window.Opacity=0;
                window.Shown+=async delegate {
                    try{window.TestSelectionAndColors();window.TestBranchMenu(api);window.TestTaskDragRedirect(api);await window.TestBatchWrites(api);await window.TestStateMenu(api);}
                    catch(Exception error){Console.Error.WriteLine(error);result=1;}finally{window.Close();}
                };
                Application.Run(window);if(result!=0)return result;
            }
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
