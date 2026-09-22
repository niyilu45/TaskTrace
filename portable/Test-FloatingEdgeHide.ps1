param([string]$OutputDirectory = '.local-build/floating-edge-hide-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Edge-hide test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Dedicated partial constructor bypasses normal startup: no database, session, network,
# tray icon, file watcher or installed process. All forms are transparent and nonactivating.
# Pointer/time inputs are simulated; the user's cursor and foreground window stay untouched.
$source = @'
using System;
using System.Drawing;
using System.IO;
using System.Runtime.InteropServices;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    FloatingWindow(string isolatedDirectory) {
        root=isolatedDirectory;data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=true;
        ShowInTaskbar=false;Opacity=0;TopMost=false;StartPosition=FormStartPosition.Manual;
        Size=new Size(420,580);MinimumSize=new Size(160,120);
        toolbar=new FlowLayoutPanel{Dock=DockStyle.Top,Height=32};
        content.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));
        for(int index=0;index<6;index++)content.RowStyles.Add(new RowStyle(index==3?SizeType.Percent:SizeType.Absolute,index==3?100:0));
        tasks.ContextMenuStrip=new ContextMenuStrip();
        content.Controls.Add(taskSurface,0,3);Controls.Add(content);Controls.Add(toolbar);
        taskSurface.Bind(tasks);
        tasks.Nodes.Add(new TaskNode("1. [P7] Isolated task"){Tag=1L});
        tasks.SelectedNode=tasks.Nodes[0];
        tray.ContextMenuStrip=new ContextMenuStrip();
        tray.ContextMenuStrip.Items.Add("Test only");
        InitializeSimpleModeRecovery(tray.ContextMenuStrip);
    }
    protected override bool ShowWithoutActivation {get{return true;}}
    static void EdgeAssert(bool pass,string message){if(!pass)throw new Exception(message);}
    static bool EdgeAligned(Rectangle visible,Rectangle area,EdgeDock edge) {
        return edge==EdgeDock.Left?visible.Left==area.Left:edge==EdgeDock.Right?visible.Right==area.Right:edge==EdgeDock.Top?visible.Top==area.Top:visible.Bottom==area.Bottom;
    }
    static bool EdgeStripOnly(Rectangle visible,Rectangle area,EdgeDock edge) {
        return edge==EdgeDock.Left?visible.Right==area.Left+EdgeRevealStrip:edge==EdgeDock.Right?visible.Left==area.Right-EdgeRevealStrip:edge==EdgeDock.Top?visible.Bottom==area.Top+EdgeRevealStrip:visible.Top==area.Bottom-EdgeRevealStrip;
    }
    internal static void TestEdgeGeometry() {
        var edges=new[]{EdgeDock.Left,EdgeDock.Right,EdgeDock.Top,EdgeDock.Bottom};int cases=0;
        foreach(var area in new[]{new Rectangle(0,0,1920,1040),new Rectangle(-1920,-240,1920,1080)})
        foreach(int border in new[]{0,8,12,16})
        foreach(var edge in edges) {
            var frame=new Padding(border,border/2,border,border);
            var initial=new Rectangle(area.Left+200,area.Top+120,420,580);
            var full=AlignBoundsToEdge(initial,area,edge,frame);
            EdgeAssert(EdgeAligned(VisibleEdgeBounds(full,frame),area,edge),"Visible full frame was not aligned: "+edge+" / "+border);
            EdgeAssert(TouchedFrameEdge(full,area,frame)==edge,"Visible full frame was not detected: "+edge+" / "+border);
            var hidden=HiddenEdgeBounds(full,area,edge,frame);
            EdgeAssert(EdgeStripOnly(VisibleEdgeBounds(hidden,frame),area,edge),"Hidden full frame reveal strip is wrong: "+edge+" / "+border);
            EdgeAssert(hidden.Size==full.Size,"Hiding changed window dimensions");
            var restarted=full;restarted.X=Math.Max(area.Left,Math.Min(restarted.X,area.Right-restarted.Width));restarted.Y=Math.Max(area.Top,Math.Min(restarted.Y,area.Bottom-restarted.Height));
            EdgeAssert(TouchedFrameEdge(restarted,area,frame)==edge,"Restart-clamped outer frame lost docking: "+edge+" / "+border);
            var simple=AlignBoundsToEdge(new Rectangle(full.Location,new Size(290,360)),area,edge,Padding.Empty);
            EdgeAssert(EdgeAligned(simple,area,edge) && TouchedFrameEdge(simple,area,Padding.Empty)==edge,"Full-to-simple geometry lost docking");
            var restored=AlignBoundsToEdge(new Rectangle(simple.Location,full.Size),area,edge,frame);
            EdgeAssert(EdgeAligned(VisibleEdgeBounds(restored,frame),area,edge),"Simple-to-full geometry lost docking");
            EdgeAssert(TouchedFrameEdge(initial,area,frame)==EdgeDock.None,"Centered window incorrectly docked");cases++;
        }
        Console.WriteLine("PASS geometry: "+cases+" cases across all four edges, 0/8/12/16 px borders, negative screen coordinates, restart clamping and full/simple dimensions.");
    }
    void SimulateLeaveAndHide(Point outside,DateTime now) {
        edgePointerLeftUtc=DateTime.MinValue;
        PollEdgeHide(outside,now,false);EdgeAssert(!edgeHidden,"Window hid without the departure delay");
        PollEdgeHide(outside,now.AddMilliseconds(499),false);EdgeAssert(!edgeHidden,"Window hid before 500 ms");
        PollEdgeHide(outside,now.AddMilliseconds(501),false);EdgeAssert(edgeHidden,"Docked window did not hide after pointer departure");
    }
    internal static void TestEdgeLifecycle(string isolatedDirectory) {
        using(var window=new FloatingWindow(isolatedDirectory)) {
            var area=Screen.PrimaryScreen.WorkingArea;
            window.Location=new Point(area.Left+100,area.Top+100);window.Show();Application.DoEvents();
            window.edgeHideEnabled=true;
            var now=new DateTime(2030,1,1,0,0,0,DateTimeKind.Utc);
            foreach(bool simple in new[]{false,true}) {
                if(window.simpleMode!=simple){window.SetSimpleMode(simple);Application.DoEvents();}
                foreach(var edge in new[]{EdgeDock.Left,EdgeDock.Right,EdgeDock.Top,EdgeDock.Bottom}) {
                    window.RestoreFromEdge(true);
                    window.Bounds=new Rectangle(area.Left+120,area.Top+80,simple?290:420,simple?360:580);
                    window.ArmEdgeHideAfterLayoutChange(edge,area);window.edgeHideTimer.Stop();
                    EdgeAssert(window.edgeDock==edge && !window.edgeHidden,"Docking failed in "+(simple?"simple":"full")+" mode");
                    var restore=window.Bounds;var outside=new Point(area.Left+area.Width+200,area.Top+area.Height+200);
                    window.SimulateLeaveAndHide(outside,now);
                    EdgeAssert(EdgeStripOnly(VisibleEdgeBounds(window.Bounds,window.EdgeFrameInsets()),area,edge),"Native bounds did not retain the expected reveal strip");
                    var trigger=window.EdgeTriggerBounds();
                    EdgeAssert(!trigger.IsEmpty && area.Contains(trigger),"Reveal trigger is outside its working area");
                    window.PollEdgeHide(outside,now.AddSeconds(1),false);EdgeAssert(window.edgeHidden,"Pointer outside the reveal strip restored the window");
                    window.PollEdgeHide(new Point(trigger.Left+trigger.Width/2,trigger.Top+trigger.Height/2),now.AddSeconds(2),false);
                    EdgeAssert(!window.edgeHidden && window.Bounds==restore,"Hover did not exactly restore the window");
                    var inside=new Point(restore.Left+restore.Width/2,restore.Top+restore.Height/2);
                    window.PollEdgeHide(inside,now.AddSeconds(3),false);window.PollEdgeHide(inside,now.AddSeconds(4),false);
                    EdgeAssert(!window.edgeHidden,"Window hid while the pointer remained inside");
                    window.PollEdgeHide(outside,now.AddSeconds(5),true);window.PollEdgeHide(outside,now.AddSeconds(6),true);
                    EdgeAssert(!window.edgeHidden,"Window hid during protected interaction");
                    window.SimulateLeaveAndHide(outside,now.AddSeconds(7));
                    window.RestoreFromEdge(false);window.Enabled=false;EdgeAssert(window.EdgeHideInteractionActive(),"Disabled modal owner was not protected");window.Enabled=true;
                    window.dragging=true;EdgeAssert(window.EdgeHideInteractionActive(),"Task dragging was not protected");window.dragging=false;
                    using(var editor=new InvisibleEdgeEditor()) {
                        editor.Show(window);Application.DoEvents();EdgeAssert(window.EdgeHideInteractionActive(),"Owned edit dialog was not protected");editor.Close();
                    }
                    EdgeAssert(!window.EdgeHideInteractionActive(),"Idle task selection blocked hiding");
                    window.RestoreFromEdge(true);EdgeAssert(!window.edgeHidden && window.edgeDock==EdgeDock.None,"Disarming kept the old docking state");
                }
            }
            if(window.simpleMode){window.SetSimpleMode(false);Application.DoEvents();}
            foreach(var edge in new[]{EdgeDock.Left,EdgeDock.Right,EdgeDock.Top,EdgeDock.Bottom}) {
                window.Bounds=new Rectangle(area.Left+120,area.Top+80,420,580);window.ArmEdgeHideAfterLayoutChange(edge,area);
                foreach(bool simple in new[]{true,false,true,false}) {
                    window.SetSimpleMode(simple);Application.DoEvents();window.edgeHideTimer.Stop();
                    EdgeAssert(window.simpleMode==simple && window.edgeDock==edge,"Production mode switch lost the docked edge");
                    EdgeAssert(EdgeAligned(VisibleEdgeBounds(window.Bounds,window.EdgeFrameInsets()),area,edge),"Production mode switch lost the visible edge alignment");
                    var outside=new Point(area.Right+200,area.Bottom+200);window.SimulateLeaveAndHide(outside,now);
                    var trigger=window.EdgeTriggerBounds();window.PollEdgeHide(new Point(trigger.Left,trigger.Top),now.AddSeconds(2),false);
                    EdgeAssert(!window.edgeHidden,"Mode-switched window failed hover recovery");
                }
            }
            window.RestoreFromEdge(true);window.Bounds=new Rectangle(area.Left,area.Top+80,420,580);
            window.TryHideAtTouchedEdge();window.edgeHideTimer.Stop();
            EdgeAssert(window.edgeDock==EdgeDock.Left && !window.edgeHidden,"Initial/manual docking must arm without immediately hiding");
            window.edgeHideEnabled=false;window.PollEdgeHide(new Point(area.Right+200,area.Bottom+200),now.AddSeconds(10),false);
            EdgeAssert(!window.edgeHidden,"Disabled preference still hid the window");
            window.Hide();
        }
        Console.WriteLine("PASS lifecycle: both layouts on all four edges; delayed hide, exact hover restore, repeat hide, pointer/interaction/modal/drag guards, 16 production mode switches and disabled preference.");
    }
}
internal sealed class InvisibleEdgeEditor : Form {
    internal InvisibleEdgeEditor(){Opacity=0;ShowInTaskbar=false;StartPosition=FormStartPosition.Manual;Location=new Point(-20000,-20000);}
    protected override bool ShowWithoutActivation {get{return true;}}
}
internal static class FloatingEdgeHideTests {
    [DllImport("user32.dll")] static extern IntPtr GetForegroundWindow();
    [STAThread] static int Main(string[] args) {
        var pointer=Cursor.Position;var foreground=GetForegroundWindow();
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            FloatingWindow.TestEdgeGeometry();FloatingWindow.TestEdgeLifecycle(args[0]);
            if(Cursor.Position!=pointer || GetForegroundWindow()!=foreground)throw new Exception("The isolated test disturbed the user's pointer or foreground window");
            Console.WriteLine("PASS isolation: cursor and foreground window unchanged; no normal startup, database, server, session or task edits.");return 0;
        }catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
'@
$harness = Join-Path $testRoot 'FloatingEdgeHideTests.cs'
[IO.File]::WriteAllText($harness, $source, [Text.UTF8Encoding]::new($false))
$compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary = Join-Path $testRoot 'FloatingEdgeHideTests.exe'
$sources = @(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingEdgeHideTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/win32manifest:' + (Join-Path $PSScriptRoot 'FloatingWindow.manifest')) ('/out:' + $binary) @sources $harness
if ($LASTEXITCODE -ne 0) { throw 'Edge-hide test compilation failed.' }
$stdout = Join-Path $testRoot 'results.txt'
$stderr = Join-Path $testRoot 'errors.txt'
$process = Start-Process -FilePath $binary -ArgumentList ('"' + $testRoot + '"') -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if (!$process.WaitForExit(60000)) { $process.Kill(); throw 'Edge-hide test timed out.' }
$process.WaitForExit()
Get-Content -LiteralPath $stdout
if ($process.ExitCode -ne 0) { Get-Content -LiteralPath $stderr; throw ('Edge-hide test failed: ' + $process.ExitCode) }
Write-Output ('Results: ' + $stdout)
