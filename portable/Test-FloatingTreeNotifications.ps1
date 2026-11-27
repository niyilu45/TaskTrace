param([switch]$VerifyPreviousBehavior)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = Join-Path $repoRoot ('.local-build/floating-tree-notification-tests/' + [Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Production callbacks, isolated controls only: no app startup, tray, HTTP or user data.
$source = @'
using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Runtime.InteropServices;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    FloatingWindow(string directory) {
        root=data=directory;url="http://127.0.0.1:1";selfTest=true;
        Opacity=0;ShowInTaskbar=false;TopMost=false;Font=SystemFonts.MessageBoxFont;
        ClientSize=new Size(420,400);Controls.Add(tasks);Controls.Add(taskSurface);
        InitializeSimpleOutstanding();taskSurface.Bind(tasks);
        /* Production expansion preferences */
    }
    static void CheckTree(bool condition,string message) {if(!condition)throw new Exception(message);}
    static void TreeEvent(TaskTreeView tree,string name,object args) {
        try {typeof(TaskTreeView).GetMethod(name,BindingFlags.Instance|BindingFlags.NonPublic).Invoke(tree,new[]{args});}
        catch(TargetInvocationException error){System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(error.InnerException).Throw();}
    }
    [StructLayout(LayoutKind.Sequential)] struct NoticeHeader {internal IntPtr Window,Id;internal int Code;}
    [StructLayout(LayoutKind.Sequential)] struct NoticeItem {
        internal uint Mask;internal IntPtr Item;internal uint State,StateMask;
        internal IntPtr Text;internal int TextLength,Image,SelectedImage,Children;internal IntPtr Param;
    }
    [StructLayout(LayoutKind.Sequential)] struct TreeNotice {
        internal NoticeHeader Header;internal uint Action;internal NoticeItem OldItem,NewItem;internal Point Drag;
    }
    static void NativeExpansion(TaskTreeView tree,IntPtr expiredHandle,uint action,int code) {
        var notice=new TreeNotice{Header=new NoticeHeader{Window=tree.Handle,Code=code},Action=action,NewItem=new NoticeItem{Item=expiredHandle,Mask=8,StateMask=0x20,State=action==2?0x20u:0u}};
        IntPtr memory=Marshal.AllocHGlobal(Marshal.SizeOf(typeof(TreeNotice)));
        try {
            Marshal.StructureToPtr(notice,memory,false);
            object[] args={Message.Create(tree.Handle,0x204e,IntPtr.Zero,memory)};
            try {typeof(TaskTreeView).GetMethod("WndProc",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(tree,args);}
            catch(TargetInvocationException error){System.Runtime.ExceptionServices.ExceptionDispatchInfo.Capture(error.InnerException).Throw();}
        } finally {Marshal.FreeHGlobal(memory);}
    }
    void TestTreeNotifications() {
        int expanded=0,collapsed=0,before=0;
        tasks.AfterExpand+=delegate {expanded++;};tasks.AfterCollapse+=delegate {collapsed++;};
        tasks.BeforeExpand+=delegate {before++;};tasks.BeforeCollapse+=delegate {before++;};
        foreach(bool simple in new[]{false,true})foreach(bool flat in new[]{false,true}) {
            rendering=true;tasks.Nodes.Clear();collapsedTasks.Clear();simpleCollapsedDuringRead.Clear();
            simpleMode=simple;singleLine.Checked=flat;tasks.SingleLinePaths=flat;tasks.SetWrappedText(!flat);
            var parent=new TreeNode("1. [P7] Parent"){Tag=11L,Name="11"};
            var child=new TreeNode("1.1. [P7] Child"){Tag=12L,Name="12"};
            child.Nodes.Add(new TreeNode("1. [P7] Outstanding"){Tag=new OutstandingLeaf{TaskId=12,Id="note",Html="Outstanding"}});
            parent.Nodes.Add(child);tasks.Nodes.Add(parent);tasks.ExpandAll();rendering=false;
            int expectedExpanded=expanded,expectedCollapsed=collapsed,expectedBefore=before;
            NativeExpansion(tasks,parent.Handle,2,-455);
            CheckTree(expanded==expectedExpanded+1,"Native expansion test did not reach the actual WinForms callback");
            expectedExpanded=expanded;
            rendering=true;
            var removed=new TreeNode("expired native item"){Tag=9000L};tasks.Nodes.Add(removed);
            IntPtr removedHandle=removed.Handle;removed.Remove();rendering=false;
            NativeExpansion(tasks,removedHandle,2,-455);
            NativeExpansion(tasks,IntPtr.Zero,2,-455);
            foreach(string eventName in new[]{"OnAfterExpand","OnAfterCollapse"}) {
                TreeEvent(tasks,eventName,null);TreeEvent(tasks,eventName,new TreeViewEventArgs(null));
                TreeEvent(tasks,eventName,new TreeViewEventArgs(new TreeNode("detached"){Tag=9001L}));
            }
            foreach(string eventName in new[]{"OnBeforeExpand","OnBeforeCollapse"}) {
                TreeEvent(tasks,eventName,null);
                var invalid=new TreeViewCancelEventArgs(null,false,TreeViewAction.Unknown);TreeEvent(tasks,eventName,invalid);
                CheckTree(invalid.Cancel,"Invalid native expansion/collapse was not cancelled");
            }
            CheckTree(expanded==expectedExpanded && collapsed==expectedCollapsed && before==expectedBefore &&
                collapsedTasks.Count==0 && simpleCollapsedDuringRead.Count==0,"Invalid events changed production expansion preferences");
            using(var foreign=new TaskTreeView()) {
                var wrong=new TreeNode("other tree"){Tag=9002L};foreign.Nodes.Add(wrong);
                TreeEvent(tasks,"OnAfterExpand",new TreeViewEventArgs(wrong));TreeEvent(tasks,"OnAfterCollapse",new TreeViewEventArgs(wrong));
            }
            CheckTree(expanded==expectedExpanded && collapsed==expectedCollapsed,"Foreign tree event reached app callbacks");
            if(flat) {
                parent.Collapse();CheckTree(parent.IsExpanded && collapsed==expectedCollapsed,"Flat mode allowed a branch to collapse");
            } else {
                parent.Collapse();
                CheckTree(!parent.IsExpanded && collapsed>expectedCollapsed && collapsedTasks.Contains(11L) &&
                    simpleCollapsedDuringRead.Contains(11L),"Valid collapse failed: simple="+simple+", flat="+flat+", expanded="+parent.IsExpanded+", callbacks="+collapsed+"/"+expectedCollapsed+", saved="+collapsedTasks.Contains(11L)+", reading="+simpleCollapsedDuringRead.Contains(11L));
                parent.Expand();
                CheckTree(parent.IsExpanded && expanded>expectedExpanded && !collapsedTasks.Contains(11L) &&
                    !simpleCollapsedDuringRead.Contains(11L),"Valid expansion failed to update both production callbacks");
            }
            string saved=File.Exists(Path.Combine(data,"floating-tree.json"))?File.ReadAllText(Path.Combine(data,"floating-tree.json")):"";
            int callbackCount=expanded+collapsed+before;
            for(int iteration=0;iteration<30;iteration++) {
                rendering=true;
                var old=new TreeNode("removed task"){Tag=10000L+iteration};old.Nodes.Add(new TreeNode("old child"){Tag=20000L+iteration});
                tasks.Nodes.Add(old);IntPtr oldHandle=old.Handle;old.Remove();rendering=false;
                TreeEvent(tasks,"OnAfterExpand",new TreeViewEventArgs(old));TreeEvent(tasks,"OnAfterCollapse",new TreeViewEventArgs(old));
                // WinForms maps a removed or zero native item handle to e.Node == null.
                NativeExpansion(tasks,oldHandle,2,-455);NativeExpansion(tasks,IntPtr.Zero,1,-455);
                NativeExpansion(tasks,oldHandle,2,-454);NativeExpansion(tasks,IntPtr.Zero,1,-454);
            }
            CheckTree(expanded+collapsed+before==callbackCount,"Delayed native events reached production callbacks");
            CheckTree((File.Exists(Path.Combine(data,"floating-tree.json"))?File.ReadAllText(Path.Combine(data,"floating-tree.json")):"")==saved,
                "Delayed notifications rewrote saved expansion preferences");
            CheckTree(!collapsedTasks.Any(id=>id>=10000) && !simpleCollapsedDuringRead.Any(id=>id>=10000),"Removed task IDs leaked into preferences");
            Application.DoEvents();taskSurface.Rebuild(true);
            CheckTree(!taskSurface.NodeBounds(parent).IsEmpty,"Ignoring stale events left task rows blank");
        }
        tasks.Dispose();int count=expanded+collapsed+before;
        TreeEvent(tasks,"OnAfterExpand",new TreeViewEventArgs(null));TreeEvent(tasks,"OnAfterCollapse",new TreeViewEventArgs(null));
        CheckTree(count==expanded+collapsed+before,"Disposed tree dispatched notifications");
        Console.WriteLine("PASS null, removed, foreign and disposed nodes do not reach production expansion callbacks.");
        Console.WriteLine("PASS valid expand/collapse, flat-mode collapse protection and rendered rows in full/simple display flags.");
        Console.WriteLine("PASS 120 removed-node refresh cycles and delayed/zero-handle native notifications preserve expansion preferences.");
    }
    internal static int RunTreeNotifications(string directory) {
        int code=0;
        using(var host=new FloatingWindow(directory)) {
            host.Shown+=delegate {try{host.TestTreeNotifications();}catch(Exception error){code=1;Console.Error.WriteLine(error);}finally{host.Close();}};
            Application.Run(host);
        }
        return code;
    }
}
internal static class TreeNotificationTests {
    [STAThread] static int Main(){Application.EnableVisualStyles();return FloatingWindow.RunTreeNotifications(AppDomain.CurrentDomain.BaseDirectory);}
}
'@
$windowText=[IO.File]::ReadAllText((Join-Path $PSScriptRoot 'FloatingWindow.cs'))
$start=$windowText.IndexOf('        tasks.AfterCollapse +=')
$end=$windowText.IndexOf('        // Both close and minimize', $start)
if($start -lt 0 -or $end -le $start){throw 'Could not locate production expansion preference callbacks.'}
$source=$source.Replace('        /* Production expansion preferences */',$windowText.Substring($start,$end-$start))
$harness=Join-Path $testRoot 'TreeNotificationTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$copiedSources=Join-Path $testRoot 'sources'
[void][IO.Directory]::CreateDirectory($copiedSources)
foreach($file in Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs'){
    [IO.File]::WriteAllText((Join-Path $copiedSources $file.Name),[IO.File]::ReadAllText($file.FullName),[Text.UTF8Encoding]::new($true))
}
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
function Build-AndRunTreeNotifications([string]$Name,[bool]$ExpectedFailure) {
    $binary=Join-Path $testRoot ($Name+'.exe')
    $sources=@(Get-ChildItem -LiteralPath $copiedSources -Filter 'Floating*.cs' | ForEach-Object FullName)
    & $compiler /nologo /target:exe /platform:x64 /optimize+ /main:TreeNotificationTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
    if($LASTEXITCODE -ne 0){throw 'Tree notification test compilation failed.'}
    $stdout=Join-Path $testRoot ($Name+'-results.txt');$stderr=Join-Path $testRoot ($Name+'-errors.txt')
    $process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    $processHandle=$process.Handle # Retain the exit code even if the short fixture already ended.
    if(!$process.WaitForExit(30000)){$process.Kill();throw 'Tree notification tests timed out.'}
    $process.WaitForExit();$process.Refresh()
    $output=[IO.File]::ReadAllText($stdout);$errors=[IO.File]::ReadAllText($stderr)
    if($ExpectedFailure) {
        if($process.ExitCode -eq 0 -or !$errors.Contains('NullReferenceException')){throw ('Old behavior did not reproduce the user error: '+$errors)}
        Write-Output 'PASS previous behavior reproduces NullReferenceException in production expansion callbacks.'
    } else {
        Write-Output $output
        if($process.ExitCode -ne 0 -or $errors.Length -gt 0){Write-Output $errors;throw ('Tree notification fixtures failed: '+$process.ExitCode)}
    }
}
Build-AndRunTreeNotifications 'CurrentTreeNotifications' $false
if($VerifyPreviousBehavior) {
    $path=Join-Path $copiedSources 'FloatingPriorityLinks.cs';$text=[IO.File]::ReadAllText($path)
    $text=$text.Replace('if(!CurrentTreeEvent(e==null?null:e.Node))return;','')
    $text=$text.Replace('if(!CurrentTreeEvent(e==null?null:e.Node)){if(e!=null)e.Cancel=true;return;}','')
    [IO.File]::WriteAllText($path,$text,[Text.UTF8Encoding]::new($true))
    Build-AndRunTreeNotifications 'PreviousTreeNotifications' $true
}
Write-Output ('Results: '+$testRoot)
