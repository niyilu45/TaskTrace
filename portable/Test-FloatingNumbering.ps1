param([string]$OutputDirectory = '.local-build/floating-numbering-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Numbering test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# In-memory native controls only. No launcher, server, user data or team share is opened.
$source = @'
using System;
using System.Collections.Generic;
using System.Linq;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    internal FloatingWindow(string isolatedDirectory) {
        root=isolatedDirectory;data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=true;
        taskSurface.ModelUpdating=delegate{return rendering;};taskSurface.Bind(tasks);
    }
    static void CheckNumbering(bool condition,string message){if(!condition)throw new Exception(message);}
    TaskNode NumberingTask(long id,string title,bool done,int priority) {
        taskCache[id]=new Dictionary<string,object>{{"id",id},{"title",title},{"done",done},{"priority",10-priority}};
        return new TaskNode(title){Tag=id,Checked=done};
    }
    internal void TestCompletedTaskNumbering() {
        var done=NumberingTask(1,"Completed root",true,0);
        var low=NumberingTask(2,"Pending low",false,7);
        var high=NumberingTask(3,"Pending high",false,1);
        var childDone=NumberingTask(4,"Completed child",true,0);
        var childLow=NumberingTask(5,"Pending child low",false,7);
        var childHigh=NumberingTask(6,"Pending child high",false,1);
        low.Nodes.AddRange(new[]{childDone,childLow,childHigh});
        var contextChild=NumberingTask(7,"Pending below completed parent",false,7);done.Nodes.Add(contextChild);
        var roots=new List<TreeNode>{done,low,high};
        SortTaskHierarchy(roots,taskCache,false);NumberTasks(roots,taskCache);
        CheckNumbering(roots.SequenceEqual(new[]{low,high,done}),"Manual root order did not put completed last");
        CheckNumbering(low.Nodes.Cast<TreeNode>().SequenceEqual(new[]{childLow,childHigh,childDone}),"Manual child order did not put completed last");
        CheckNumbering(low.Text.StartsWith("1. ") && childLow.Text.StartsWith("1.1. ") && childHigh.Text.StartsWith("1.2. ") && childDone.Text.StartsWith("1.3. ") && done.Text.StartsWith("3. "),"Tree numbering has gaps before unfinished tasks");
        CheckNumbering(contextChild.Parent==done && contextChild.Text.StartsWith("3.1. "),"Completed parent context lost its child");
        SortTaskHierarchy(roots,taskCache,true);NumberTasks(roots,taskCache);
        CheckNumbering(roots.SequenceEqual(new[]{high,low,done}) && low.Nodes[0]==childHigh,"Priority sorting did not remain within completion groups");
        done.Checked=false;SortTaskHierarchy(roots,taskCache,true);NumberTasks(roots,taskCache);
        CheckNumbering(roots[0]==done && done.Text.StartsWith("1. "),"Reopened task did not regain its unfinished number");
        Console.WriteLine("PASS tree roots/children, manual and priority order, completed-parent context, reopen and contiguous numbering.");
    }
    internal void TestOutstandingNumbering() {
        var parent=NumberingTask(20,"Owner",false,7);tasks.Nodes.Add(parent);
        var firstDone=new PendingItem{Id="done",Html="Done",Done=true,Priority=0,Number=99};
        var low=new PendingItem{Id="low",Html="Low",Priority=7,Number=40};
        var lastDone=new PendingItem{Id="done-two",Html="Done two",Done=true,Priority=7,Number=55};
        var high=new PendingItem{Id="high",Html="High <img src='/image/1'>",Priority=1,Number=60};
        var shared=new SharedList{CommentId=1,Items=new List<PendingItem>{firstDone,low,lastDone,high}};
        foreach(bool simple in new[]{false,true}) {
            simpleMode=simple;prioritySort.Checked=false;showCompleted.Checked=true;parent.Nodes.Clear();
            CheckNumbering(ApplySimpleOutstanding(parent,shared),"Initial render skipped");
            var leaves=parent.Nodes.Cast<TreeNode>().Where(node=>node.Tag is OutstandingLeaf).ToArray();
            CheckNumbering(leaves.Select(node=>((OutstandingLeaf)node.Tag).Id).SequenceEqual(new[]{"low","high","done","done-two"}),"Completed outstanding items were not placed last");
            for(int i=0;i<leaves.Length;i++)CheckNumbering(leaves[i].Text.StartsWith((i+1)+". "),"Outstanding numbering reused saved/stale indexes");
            CheckNumbering(!ApplySimpleOutstanding(parent,shared),"Unchanged display was rebuilt");
            prioritySort.Checked=true;CheckNumbering(ApplySimpleOutstanding(parent,shared),"Priority switch did not reorder");
            CheckNumbering(((OutstandingLeaf)parent.Nodes[0].Tag).Id=="high" && parent.Nodes[0].Text.StartsWith("1. "),"Highest unfinished priority should be first");
            CheckNumbering(!ApplySimpleOutstanding(parent,shared),"Sorted shared list flickers on unchanged refresh");
            showCompleted.Checked=false;DateTime next=DateTime.MaxValue;
            var filtered=FilterCompletedOutstanding(20,shared,ref next);ApplySimpleOutstanding(parent,filtered);
            CheckNumbering(parent.Nodes.Count==2 && parent.Nodes[0].Text.StartsWith("1. ") && parent.Nodes[1].Text.StartsWith("2. "),"Hidden completed items consumed numbers");
            visiblePriorities.Clear();visiblePriorities.Add(7);ApplySimpleOutstanding(parent,FilterOutstandingPriorities(filtered));
            CheckNumbering(parent.Nodes.Count==1 && parent.Nodes[0].Text.StartsWith("1. "),"Priority filter left a numbering gap");
            visiblePriorities.Clear();foreach(int priority in Enumerable.Range(0,10))visiblePriorities.Add(priority);
            completedHideDelayMinutes=5;RememberOutstandingCompletion(20,"done",true);next=DateTime.MaxValue;
            filtered=FilterCompletedOutstanding(20,shared,ref next);ApplySimpleOutstanding(parent,filtered);
            CheckNumbering(parent.Nodes.Count==3 && !parent.Nodes[0].Checked && !parent.Nodes[1].Checked && parent.Nodes[2].Checked,"Delayed completed item displaced unfinished items");
            CheckNumbering(parent.Nodes[2].Text.StartsWith("3. "),"Delayed completed item number did not follow unfinished items");
            recentlyCompletedOutstanding.Clear();completedHideDelayMinutes=0;
        }
        CheckNumbering(shared.Items.SequenceEqual(new[]{firstDone,low,lastDone,high}),"Rendering rewrote shared data order");
        tasks.Nodes.Clear();simpleMode=false;
        Console.WriteLine("PASS full/simple outstanding numbering, hidden/completed/delayed/priority filtering, no-op refresh and source order preservation.");
    }
    internal void TestSingleLineCompletedNumbering() {
        singleLine.Checked=true;
        var owner=NumberingTask(40,"Ancestor",false,7);
        var done=NumberingTask(41,"Done task",true,0);
        var pending=NumberingTask(42,"Pending task",false,5);owner.Nodes.AddRange(new[]{done,pending});
        var roots=new List<TreeNode>{owner};NumberTasks(roots,taskCache);
        var lists=new Dictionary<long,SharedList>{{40,new SharedList{Items=new List<PendingItem>{
            new PendingItem{Id="done-leaf",Html="Done leaf",Done=true,Priority=0},
            new PendingItem{Id="pending-leaf",Html="Pending leaf <img src='/image/1'>",Priority=7}
        }}}};
        var parents=new Dictionary<long,long>{{41,40},{42,40}};
        var flat=PrepareSingleLineNodes(FlattenTaskNodes(roots,taskCache,parents,lists),taskCache);
        CheckNumbering(flat.Count==4 && flat[0]==pending && flat[2].Checked && flat[3]==done,"Single-line tasks and leaves did not sort unfinished before completed");
        for(int i=0;i<flat.Count;i++) {
            var task=flat[i] as TaskNode;var leaf=flat[i].Tag as OutstandingLeaf;int length=task!=null?task.CurrentTextLength:leaf.CurrentTextLength;
            CheckNumbering(flat[i].Text.StartsWith((i+1)+". "),"Single-line numbering has a gap");
            CheckNumbering(flat[i].Text.Substring(length).StartsWith(TaskTreeView.SingleLineSeparator),"Renumbering broke ancestor text boundary");
        }
        var original=flat.Select(node=>node.Text).ToArray();flat=PrepareSingleLineNodes(flat,taskCache);
        CheckNumbering(original.SequenceEqual(flat.Select(node=>node.Text)),"Repeated flat numbering changed text");
        var filtered=PrepareSingleLineNodes(flat.Where(node=>!node.Checked).ToList(),taskCache);
        CheckNumbering(filtered.Count==2 && filtered[0].Text.StartsWith("1. ") && filtered[1].Text.StartsWith("2. "),"Filtered flat rows retained hidden numbers");
        Console.WriteLine("PASS flat mixed task/outstanding priority order, image/path text boundaries, repeat refresh and hidden-completed numbering.");
    }
}
internal static class FloatingNumberingTests {
    [STAThread] static int Main() {
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            using(var window=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory)) {
                window.TestCompletedTaskNumbering();window.TestOutstandingNumbering();window.TestSingleLineCompletedNumbering();
            }
            return 0;
        }catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
'@
$harness=Join-Path $testRoot 'FloatingNumberingTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingNumberingTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingNumberingTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Floating numbering test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){$process.Kill();throw 'Floating numbering test timed out.'}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
$exitCode=$process.ExitCode;if($null -eq $exitCode){$exitCode=if((Get-Item -LiteralPath $stderr).Length -eq 0){0}else{1}}
if($exitCode -ne 0){Get-Content -LiteralPath $stderr;throw ('Floating numbering test failed: '+$exitCode)}
Write-Output ('Results: '+$stdout)
