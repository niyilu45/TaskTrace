param([string]$OutputDirectory = '.local-build/floating-toolbar-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot,[StringComparison]::OrdinalIgnoreCase)) {throw 'Toolbar test output must remain inside .local-build.'}
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Isolated fixture: no normal startup, server, session or user database/settings writes.
# The thread-local activation guard lets native layouts run without taking focus.
$source = @'
using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    FloatingWindow(string directory) {
        root=data=directory;url="http://127.0.0.1:1";selfTest=true;
        Font=new Font("Microsoft YaHei UI",10);Text="TaskTrace";ShowInTaskbar=false;Opacity=0;TopMost=false;StartPosition=FormStartPosition.Manual;Location=new Point(70,70);Size=new Size(480,400);
        toolbar=new FlowLayoutPanel{Dock=DockStyle.Top,AutoSize=true};
        toolbar.Controls.Add(new Button{Text="网页界面",AutoSize=true});toolbar.Controls.Add(new Button{Text="简洁模式",AutoSize=true});toolbar.Controls.Add(new Button{Text="排序方式",AutoSize=true});
        content.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));
        for(int row=0;row<6;row++)content.RowStyles.Add(new RowStyle(row==3?SizeType.Percent:row==2?SizeType.AutoSize:SizeType.Absolute,row==3?100:0));
        searchRow=CreateFilterRow();content.Controls.Add(searchRow,0,2);content.Controls.Add(taskSurface,0,3);content.Controls.Add(status,0,5);InitializeStatusRow();Controls.Add(content);Controls.Add(toolbar);
        tasks.ContextMenuStrip=new ContextMenuStrip();taskSurface.Bind(tasks);tasks.Nodes.Add(new TaskNode("1. [P7] 标题栏与筛选测试任务"){Tag=1L});
        tray.ContextMenuStrip=new ContextMenuStrip();tray.ContextMenuStrip.Items.Add("Fixture");InitializeEdgeHide(tray.ContextMenuStrip);InitializeSimpleModeRecovery(tray.ContextMenuStrip);InitializeStartWithWindows(tray.ContextMenuStrip);InitializeCaptionSettings();
    }
    protected override bool ShowWithoutActivation{get{return true;}}
    protected override CreateParams CreateParams{get{var value=base.CreateParams;value.ExStyle|=0x08000000;return value;}}
    [DllImport("user32.dll")]static extern bool PrintWindow(IntPtr window,IntPtr dc,uint flags);
    void SaveHeader(string path){using(var bitmap=new Bitmap(Width,Height))using(var graphics=Graphics.FromImage(bitmap)){var dc=graphics.GetHdc();try{if(!PrintWindow(Handle,dc,2))throw new Exception("PrintWindow failed");}finally{graphics.ReleaseHdc(dc);}if(captionSettingsWindow!=null && captionSettingsWindow.Visible){var bounds=CaptionSettingsBounds();var native=bitmap.GetPixel(Math.Min(bitmap.Width-1,bounds.Right+Math.Max(2,bounds.Width/8)),bounds.Top+Math.Max(2,bounds.Height/6));using(var gear=new Bitmap(bounds.Width,bounds.Height)){captionSettingsWindow.Settings.DrawToBitmap(gear,new Rectangle(Point.Empty,gear.Size));if((native.ToArgb()&0xFFFFFF)!=(gear.GetPixel(3,3).ToArgb()&0xFFFFFF))throw new Exception("Gear background differs from the native minimize button: "+native+" / "+gear.GetPixel(3,3));}captionSettingsWindow.DrawToBitmap(bitmap,bounds);}bitmap.Save(path);}}
    internal static void RunHeaderTests(string directory) {
        using(var window=new FloatingWindow(directory)) {
            window.Show();Application.DoEvents();window.UpdateSearchCount(100,100);
            foreach(int width in new[]{640,350}) {
                window.Width=width;Application.DoEvents();window.SyncCaptionSettings();
                var bounds=window.CaptionSettingsBounds();
                if(bounds.IsEmpty || bounds.Left<0 || bounds.Right>=window.Width || bounds.Bottom>window.Height-window.ClientSize.Height)throw new Exception("Caption gear overlaps client area or native caption buttons: "+bounds);
                if(window.searchRow.Height>=45 || window.searchCountLabel.Text!="100项" || window.searchCountLabel.Right>window.searchRow.ClientSize.Width)throw new Exception("Filter row is not compact: "+window.searchRow.Bounds);
                window.SaveHeader(Path.Combine(directory,"header-"+width+".png"));
                Console.WriteLine("PASS caption and filter geometry at "+width+"px: "+bounds+"; filter="+window.searchRow.Bounds);
            }
            int reads=window.captionBackgroundReadCount;for(int repeat=0;repeat<100;repeat++)window.SyncCaptionSettings();
            if(window.captionBackgroundReadCount!=reads)throw new Exception("Unchanged caption repeatedly captures the window");
            using(var palette=new CaptionSettingsWindow(delegate{})) {
                palette.ClientSize=new Size(48,30);palette.PerformLayout();
                foreach(var background in new[]{Color.White,Color.FromArgb(243,243,243),Color.FromArgb(32,48,64)}) {
                    palette.ApplyCaptionBackground(background);
                    using(var bitmap=new Bitmap(48,30)) {
                        palette.Settings.DrawToBitmap(bitmap,new Rectangle(Point.Empty,bitmap.Size));
                        if(bitmap.GetPixel(3,3).ToArgb()!=background.ToArgb())throw new Exception("Caption button kept its own themed background after the caption color changed");
                    }
                }
            }
            Console.WriteLine("PASS caption colors: gear pixels match native minimize background; light, inactive and dark palettes apply to the button; unchanged sync reuses the captured color.");
            window.search.Text="测试";Application.DoEvents();
            var clear=window.search.Parent.Controls.OfType<FloatingFilterClearButton>().Single();
            if(!clear.Visible || clear.Left<window.search.Right || !window.search.Parent.ClientRectangle.Contains(clear.Bounds))throw new Exception("Inline clear overlaps typed text or lies outside the input");
            window.projectsDirty=false;window.filterDataReady=true;window.filterProjectId=0;window.filterSnapshotContext=window.TaskViewContext(false);
            clear.PerformClick();Application.DoEvents();if(window.search.Text!="" || clear.Visible)throw new Exception("Clear did not reset text and hide itself");
            int opened=0;Exception error=null;
            using(var close=new Timer{Interval=20}) {
                close.Tick+=delegate {var dialog=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="设置");if(dialog==null)return;try{var top=dialog.Controls.OfType<CheckBox>().Single(box=>box.AccessibleName=="悬浮窗置顶");if(top.Checked!=window.pin.Checked || !dialog.ClientRectangle.Contains(top.Bounds))throw new Exception("Settings topmost preference is missing or clipped");opened++;}catch(Exception e){error=e;}finally{dialog.Close();close.Stop();}};
                var bounds=window.CaptionSettingsBounds();int x=window.Left+bounds.Left+bounds.Width/2,y=window.Top+bounds.Top+bounds.Height/2;var point=new IntPtr((y<<16)|(x&0xffff));
                if(window.captionSettingsWindow==null || window.captionSettingsWindow.Owner!=window || window.captionSettingsWindow.Bounds!=new Rectangle(window.Left+bounds.Left,window.Top+bounds.Top,bounds.Width,bounds.Height))throw new Exception("Caption settings did not follow the native button bounds");
                close.Start();window.captionSettingsWindow.Settings.PerformClick();Application.DoEvents();
                if(error!=null)throw error;if(opened!=1)throw new Exception("Caption gear did not open settings exactly once");
            }
            window.SetSimpleMode(true);Application.DoEvents();if(!window.CaptionSettingsBounds().IsEmpty || window.captionSettingsWindow.Visible)throw new Exception("Compact mode retained a caption gear");
            window.SetSimpleMode(false);Application.DoEvents();if(window.CaptionSettingsBounds().IsEmpty || !window.captionSettingsWindow.Visible)throw new Exception("Returning to full mode lost the caption gear");
            window.SetSimpleMode(true);window.SetBusy(true);window.SetSimpleMode(false);Application.DoEvents();
            if(!window.captionSettingsWindow.Visible || window.captionSettingsWindow.Settings.Enabled)throw new Exception("Busy mode switch did not show a disabled caption action");
            window.SetBusy(false);if(!window.captionSettingsWindow.Settings.Enabled)throw new Exception("Caption action remained disabled after saving");
            window.WindowState=FormWindowState.Minimized;Application.DoEvents();if(window.captionSettingsWindow.Visible)throw new Exception("Minimizing left a detached settings button");window.RestoreWindow();Application.DoEvents();
            if(window.EdgeHideInteractionActive())throw new Exception("Caption settings incorrectly blocks automatic edge hiding");
            using(var editor=new Form{ShowInTaskbar=false,Opacity=0}){editor.Show(window);Application.DoEvents();if(!window.EdgeHideInteractionActive())throw new Exception("A real editor no longer protects edge hiding");editor.Close();}
            window.edgeHideEnabled=true;var area=Screen.PrimaryScreen.WorkingArea;
            foreach(var edge in new[]{EdgeDock.Left,EdgeDock.Right,EdgeDock.Top,EdgeDock.Bottom}) {
                window.RestoreFromEdge(true);window.ArmEdgeHideAfterLayoutChange(edge,area);window.edgeHideTimer.Stop();window.edgePointerLeftUtc=DateTime.MinValue;var outside=new Point(area.Right+100,area.Bottom+100);var now=DateTime.UtcNow;window.PollEdgeHide(outside,now,window.EdgeHideInteractionActive());window.PollEdgeHide(outside,now.AddMilliseconds(501),window.EdgeHideInteractionActive());Application.DoEvents();
                if(!window.edgeHidden)throw new Exception("The production interaction guard prevented timed edge hiding");
                if(window.captionSettingsWindow.Visible)throw new Exception("Edge hiding left a detached settings button");
                window.RestoreFromEdge(false);Application.DoEvents();
                var caption=window.CaptionSettingsBounds();
                if(!window.captionSettingsWindow.Visible || window.captionSettingsWindow.Location!=new Point(window.Left+caption.Left,window.Top+caption.Top))throw new Exception("Edge restoration did not restore the caption button position");
            }
            window.RestoreFromEdge(true);window.edgeHideEnabled=false;
            window.HideToTray();Application.DoEvents();if(window.captionSettingsWindow.Visible)throw new Exception("Tray hiding left a detached settings button");window.RestoreWindow();Application.DoEvents();if(!window.captionSettingsWindow.Visible)throw new Exception("Tray restoration lost settings");
            window.Hide();Console.WriteLine("PASS inline clear, native caption click opens settings, topmost control, full/simple caption lifecycle; no user data or settings writes.");
        }
    }
}
internal static class HeaderTests {
    [STAThread]static int Main(string[] args){try{Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);using(var guard=new TestForegroundGuard()){FloatingWindow.RunHeaderTests(args[0]);Application.DoEvents();if(guard.Activated)throw new Exception("Test activated a window");}return 0;}catch(Exception e){Console.Error.WriteLine(e);return 1;}}
}internal sealed class TestForegroundGuard : IDisposable {
    delegate void ForegroundCallback(IntPtr hook,uint eventType,IntPtr window,int objectId,int childId,uint thread,uint time);
    [DllImport("user32.dll")]static extern IntPtr SetWinEventHook(uint first,uint last,IntPtr module,ForegroundCallback callback,uint process,uint thread,uint flags);
    [DllImport("user32.dll")]static extern bool UnhookWinEvent(IntPtr hook);
    delegate IntPtr ActivationCallback(int code,IntPtr value,IntPtr details);
    [DllImport("user32.dll")]static extern IntPtr SetWindowsHookEx(int type,ActivationCallback callback,IntPtr module,uint thread);
    [DllImport("user32.dll")]static extern IntPtr CallNextHookEx(IntPtr hook,int code,IntPtr value,IntPtr details);
    [DllImport("user32.dll")]static extern bool UnhookWindowsHookEx(IntPtr hook);
    [DllImport("kernel32.dll")]static extern uint GetCurrentThreadId();
    readonly ActivationCallback activationCallback;
    readonly IntPtr activationHook;
    readonly ForegroundCallback callback;
    readonly IntPtr hook;
    internal bool Activated;
    internal TestForegroundGuard() {
        // Thread-local CBT hook vetoes activation/focus for test forms only.
        // Native minimize/maximize/resize still run against the real HWND.
        activationCallback=delegate(int code,IntPtr value,IntPtr details){return code==5 || code==9?new IntPtr(1):CallNextHookEx(activationHook,code,value,details);};
        activationHook=SetWindowsHookEx(5,activationCallback,IntPtr.Zero,GetCurrentThreadId());
        if(activationHook==IntPtr.Zero)throw new Exception("Cannot protect desktop focus during native layout tests");
        callback=delegate {Activated=true;};
        hook=SetWinEventHook(3,3,IntPtr.Zero,callback,(uint)System.Diagnostics.Process.GetCurrentProcess().Id,0,0);
        if(hook==IntPtr.Zero){UnhookWindowsHookEx(activationHook);throw new Exception("Cannot observe test-process foreground activation");}
    }
    public void Dispose(){UnhookWinEvent(hook);UnhookWindowsHookEx(activationHook);GC.KeepAlive(callback);GC.KeepAlive(activationCallback);}
}

'@
$harness = Join-Path $testRoot 'FloatingToolbarTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary = Join-Path $testRoot 'FloatingToolbarTests.exe'
$sources = @(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /main:HeaderTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Toolbar tests failed to compile.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -ArgumentList ('"'+$testRoot+'"') -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(45000)){$process.Kill();throw 'Toolbar tests timed out.'}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
if($process.ExitCode -ne 0){Get-Content -LiteralPath $stderr;throw 'Toolbar tests failed.'}
Write-Output ('Results: '+$testRoot)
