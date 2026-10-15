param([string]$OutputDirectory = '.local-build/quick-alarm-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$allowed = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($allowed, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Only the tray timer component runs. No server, task database, team share or installed app is used.
$source = @'
using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Windows.Forms;
internal static class QuickAlarmTests {
    static string output;
    static void Assert(bool condition,string message){if(!condition)throw new Exception(message);}
    static ToolStripMenuItem Item(FloatingQuickAlarms alarms,int index){return (ToolStripMenuItem)alarms.Menu.DropDownItems["quick-alarm-"+index];}
    static Control Find(Control parent,string name){return parent.Controls.Find(name,true).Single();}
    static void ClickConfirm(Form dialog){((Button)Find(dialog,"alarm-confirm")).PerformClick();Application.DoEvents();}
    static void SetTime(Form dialog,string name,int hours,int minutes,int seconds){
        ((NumericUpDown)Find(dialog,name+"小时")).Value=hours;
        ((NumericUpDown)Find(dialog,name+"分钟")).Value=minutes;
        ((NumericUpDown)Find(dialog,name+"秒")).Value=seconds;
    }
    static void Display(Form dialog){
        var area=Screen.FromPoint(Cursor.Position).WorkingArea;
        Assert(Math.Abs(dialog.Left-(area.Left+(area.Width-dialog.Width)/2))<=1 && Math.Abs(dialog.Top-(area.Top+(area.Height-dialog.Height)/2))<=1,"Popup is not centered on the active screen");
        dialog.ShowInTaskbar=false;dialog.Location=new Point(-30000,-30000);dialog.Show();Application.DoEvents();
    }
    static void Capture(Control control,string name){
        control.PerformLayout();Application.DoEvents();
        using(var bitmap=new Bitmap(control.Width,control.Height)){control.DrawToBitmap(bitmap,new Rectangle(Point.Empty,bitmap.Size));bitmap.Save(Path.Combine(output,name+".png"));}
    }
    static void AssertRows(Form dialog){
        var description=Find(dialog,"alarm-description");
        Assert(description.Height>=description.GetPreferredSize(new Size(description.Width,0)).Height,"Alarm instructions are clipped");
        for(int i=0;i<5;i++){
            var label=Find(dialog,"alarm-name-"+i);
            var input=Find(dialog,"闹钟"+(i+1)+"小时");
            int labelY=label.PointToScreen(Point.Empty).Y+label.Height/2;
            int inputY=input.PointToScreen(Point.Empty).Y+input.Height/2;
            Assert(Math.Abs(labelY-inputY)<=4,"Alarm name and duration are vertically misaligned: "+i);
        }
    }
    static int CompletionCount(FloatingQuickAlarms alarms){return alarms.CompletionDialog.Controls[0].Controls.OfType<ListBox>().Single().Items.Count;}
    [STAThread] static int Main(){
        try {
            Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
            output=AppDomain.CurrentDomain.BaseDirectory;string state=Path.Combine(output,"state");DateTime now=new DateTime(2026,10,15,0,0,0,DateTimeKind.Utc);
            using(var font=new Font(SystemFonts.MessageBoxFont.FontFamily,9F))
            using(var alarms=new FloatingQuickAlarms(state,font,SystemIcons.Information,()=>now,Display)){
                Assert(alarms.Alarms.Select(a=>a.Seconds).SequenceEqual(new[]{300,600,900,1800,3600,900}),"Incorrect defaults");
                Assert(alarms.Menu.DropDownItems.Count==9 && alarms.Alarms.All(a=>!a.Running),"Incorrect initial menu");
                alarms.SaveDurations(new[]{10,10,20,30,40,50});
                for(int i=0;i<5;i++)Item(alarms,i).PerformClick();
                Item(alarms,5).PerformClick();
                Assert(alarms.CustomDialog!=null && !alarms.Alarms[5].Running && !Item(alarms,5).Checked,"Custom alarm started before confirmation");
                Capture(alarms.CustomDialog,"custom-alarm");
                ClickConfirm(alarms.CustomDialog);
                Assert(alarms.Alarms.All(a=>a.Running) && Enumerable.Range(0,6).All(i=>Item(alarms,i).Checked),"Six alarms cannot run together");
                now=now.AddSeconds(2.1);alarms.Poll();
                Assert(Item(alarms,0).Text.Contains("00:00:08") && Item(alarms,5).Text.Contains("00:00:48"),"Countdown rounding or independent deadlines failed");
                alarms.Menu.DropDown.Show(new Point(-30000,-30000));Capture(alarms.Menu.DropDown,"alarm-menu");alarms.Menu.DropDown.Close();
                DateTime? untouched=alarms.Alarms[3].DueUtc;
                Item(alarms,2).PerformClick();
                Assert(!alarms.Alarms[2].Running && Item(alarms,2).Text.Contains("00:00:20") && alarms.Alarms[3].DueUtc==untouched && alarms.CompletionDialog==null,"Cancel changed another alarm or displayed completion");
                now=now.AddSeconds(7.9);alarms.Poll();
                Assert(!Item(alarms,0).Checked&&!Item(alarms,1).Checked&&Item(alarms,0).Text.Contains("00:00:10")&&CompletionCount(alarms)==2,"Simultaneous expiry did not reset and notify");
                alarms.Poll();Assert(CompletionCount(alarms)==2,"Completion was repeated");
                Item(alarms,0).PerformClick();Item(alarms,5).PerformClick();
                Item(alarms,5).PerformClick();var canceled=alarms.CustomDialog;canceled.Close();Application.DoEvents();
                Assert(!alarms.Alarms[5].Running && !Item(alarms,5).Checked,"Canceling custom entry started the alarm");
                now=now.AddSeconds(45);alarms.Poll();
                Assert(alarms.Alarms.All(a=>!a.Running)&&CompletionCount(alarms)==5,"Other timers stopped while popup was open or delayed tick lost elapsed time");
                Capture(alarms.CompletionDialog,"alarms-completed");
                ClickConfirm(alarms.CompletionDialog);Assert(alarms.CompletionDialog==null,"Completion confirmation did not close");
                Console.WriteLine("PASS six simultaneous timers, live countdown, independent cancellation, elapsed-time expiry, reset, one-shot and grouped modeless notification.");

                Item(alarms,0).PerformClick();DateTime? originalDeadline=alarms.Alarms[0].DueUtc;
                ((ToolStripMenuItem)alarms.Menu.DropDownItems["quick-alarm-edit"]).PerformClick();
                AssertRows(alarms.SettingsDialog);Capture(alarms.SettingsDialog,"edit-alarms");
                SetTime(alarms.SettingsDialog,"闹钟1",0,0,25);SetTime(alarms.SettingsDialog,"闹钟2",0,0,0);
                ClickConfirm(alarms.SettingsDialog);
                Assert(alarms.SettingsDialog!=null && Find(alarms.SettingsDialog,"alarm-error").Visible && alarms.Alarms[0].Seconds==10,"Invalid duration partially changed configuration");
                SetTime(alarms.SettingsDialog,"闹钟2",0,0,5);ClickConfirm(alarms.SettingsDialog);
                Assert(alarms.SettingsDialog==null && alarms.Alarms[0].Seconds==25 && alarms.Alarms[0].DueUtc==originalDeadline,"Editing restarted active countdown");
                now=now.AddSeconds(10);alarms.Poll();
                Assert(Item(alarms,0).Text.Contains("00:00:25") && !Item(alarms,0).Checked,"Finish did not restore newly configured time");
                ClickConfirm(alarms.CompletionDialog);
                Item(alarms,5).PerformClick();SetTime(alarms.CustomDialog,"任意闹钟",0,0,0);ClickConfirm(alarms.CustomDialog);
                Assert(!alarms.Alarms[5].Running && alarms.CustomDialog!=null,"Zero custom duration accepted");
                var minutes=(NumericUpDown)Find(alarms.CustomDialog,"任意闹钟分钟");minutes.Text="1";
                ClickConfirm(alarms.CustomDialog);
                Assert(alarms.Alarms[5].Running && alarms.Alarms[5].Seconds==60,"Typed duration was not committed");
                Item(alarms,5).PerformClick();Assert(Item(alarms,5).Text.Contains("00:01:00"),"Custom cancel did not restore chosen duration");

                string path=Path.Combine(state,"quick-alarms.json");byte[] before=File.ReadAllBytes(path);bool failed=false;
                using(var locked=new FileStream(path,FileMode.Open,FileAccess.ReadWrite,FileShare.None)){
                    try{alarms.SaveDurations(new[]{100,100,100,100,100,100});}catch(IOException){failed=true;}
                }
                Assert(failed&&alarms.Alarms[0].Seconds==25&&File.ReadAllBytes(path).SequenceEqual(before)&&Directory.GetFiles(state,"*.tmp").Length==0,"Failed settings write damaged preferences");
                using(var restored=new FloatingQuickAlarms(state,font,SystemIcons.Information,()=>now,Display)){
                    Assert(restored.Alarms.Select(a=>a.Seconds).SequenceEqual(new[]{25,5,20,30,40,60})&&restored.Alarms.All(a=>!a.Running),"Preferences did not persist or timers restarted after program exit");
                }
                string damaged=Path.Combine(output,"damaged");Directory.CreateDirectory(damaged);File.WriteAllText(Path.Combine(damaged,"quick-alarms.json"),"{invalid");
                using(var safe=new FloatingQuickAlarms(damaged,font,null,()=>now,Display))Assert(safe.Alarms[0].Seconds==300,"Damaged preference blocked startup");
                ((ToolStripMenuItem)alarms.Menu.DropDownItems["quick-alarm-edit"]).PerformClick();
                alarms.SettingsDialog.Font=new Font(font.FontFamily,14F);
                alarms.SettingsDialog.Size=new Size(800,540);
                alarms.SettingsDialog.PerformLayout();AssertRows(alarms.SettingsDialog);Capture(alarms.SettingsDialog,"edit-alarms-large-text");
                bool menuDisposed=false;alarms.Menu.Disposed+=delegate {menuDisposed=true;};
                Item(alarms,0).PerformClick();alarms.Dispose();alarms.Poll();
                Assert(menuDisposed&&alarms.Alarms.All(a=>!a.Running)&&Application.OpenForms.Count==0,"Shutdown leaked timers or windows: disposed="+menuDisposed+", running="+alarms.Alarms.Count(a=>a.Running)+", forms="+String.Join(", ",Application.OpenForms.Cast<Form>().Select(f=>f.Text+" disposed="+f.IsDisposed)));
                Console.WriteLine("PASS edit validation, preserve active deadlines, typed custom duration, cancel, persistence, locked-file rollback, corrupt settings and shutdown cleanup.");
            }
            using(var live=new FloatingQuickAlarms(Path.Combine(output,"live-clock"),SystemFonts.MessageBoxFont,SystemIcons.Information,null,Display)){
                live.SaveDurations(new[]{1,10,10,10,10,10});Item(live,0).PerformClick();
                System.Threading.Thread.Sleep(1200);Application.DoEvents();
                Assert(!live.Alarms[0].Running && live.CompletionDialog!=null,"Real Windows timer did not deliver elapsed alarm");
            }
            Console.WriteLine("PASS actual timer callback, uptime clock and normal/large-text row alignment.");
            return 0;
        }catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
'@
$harness = Join-Path $testRoot 'QuickAlarmTests.cs'
[IO.File]::WriteAllText($harness, $source, [Text.UTF8Encoding]::new($false))
$compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary = Join-Path $testRoot 'QuickAlarmTests.exe'
& $compiler /nologo /target:exe /platform:x64 /optimize+ /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) (Join-Path $PSScriptRoot 'FloatingQuickAlarms.cs') $harness
if ($LASTEXITCODE -ne 0) { throw 'Quick alarm test compilation failed.' }
$stdout = Join-Path $testRoot 'results.txt'
$stderr = Join-Path $testRoot 'errors.txt'
$process = Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if (!$process.WaitForExit(60000)) { $process.Kill(); throw 'Quick alarm tests timed out.' }
$process.WaitForExit(); $process.Refresh()
Get-Content -LiteralPath $stdout
$exitCode = $process.ExitCode
if ($null -eq $exitCode) { $exitCode = if ((Get-Item -LiteralPath $stderr).Length -eq 0) {0} else {1} }
if ($exitCode -ne 0) { Get-Content -LiteralPath $stderr; throw ('Quick alarm tests failed: '+$exitCode) }
Write-Output ('Results: '+$testRoot)
