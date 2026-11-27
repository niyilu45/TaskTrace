param([switch]$VerifyPreviousBehavior)
$ErrorActionPreference='Stop'
$repoRoot=Split-Path $PSScriptRoot -Parent
$testRoot=Join-Path $repoRoot ('.local-build/floating-crash-safety/'+[Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
$source=@'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed class CrashSafetyTests {
    static int reports;
    static string directory;
    static List<string> failures=new List<string>();
    static void Check(bool ok,string message){if(!ok)throw new Exception(message);}
    static async Task Run(string name,Func<Task> test){try{await test();Console.WriteLine("PASS "+name);}catch(Exception e){failures.Add(name+": "+e.Message);}}
    static async Task Until(Func<bool> ready,string message){for(int i=0;i<160 && !ready();i++)await Task.Delay(20);Check(ready(),message);}
    static TableLayoutPanel CalendarGrid(Form form){return form.Controls.OfType<TableLayoutPanel>().First(panel=>panel.RowCount==7);}
    static IEnumerable<Control> Descendants(Control root){foreach(Control item in root.Controls){yield return item;foreach(var child in Descendants(item))yield return child;}}
    static async Task Cases(Form host) {
        await Run("deferred UI work is dropped after its target closes; concurrent queue/dispose is safe",async delegate {
            int called=0;var control=new Panel();host.Controls.Add(control);var handle=control.Handle;
            Check(FloatingUi.Post(control,delegate{called++;}),"Live UI did not accept work");control.Dispose();await Task.Delay(60);
            Check(called==0,"Closed target executed deferred callback");Check(!FloatingUi.Post(control,delegate{called++;}),"Disposed control accepted work");
            for(int i=0;i<80;i++){
                var target=new Panel();host.Controls.Add(target);handle=target.Handle;
                Task posting=Task.Run(()=>{for(int j=0;j<12;j++)FloatingUi.Post(target,delegate{if(target.IsDisposed)throw new Exception("Disposed target reached callback");});});
                target.Dispose();await posting;
            }
            await Task.Delay(50);
        });
        await Run("late clipboard completion releases its editor and never runs after disposal",async delegate {
            int completed=0,errors=0;var read=new TaskCompletionSource<string>();
            using(var editor=new ProgressHtmlEditor(()=>read.Task,true)) {
                host.Controls.Add(editor);editor.Show();
                var browser=(WebBrowser)editor.Controls[0];
                await Until(()=>browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Editor not initialized");
                var bridge=(ProgressHtmlEditorBridge)browser.ObjectForScripting;
                bridge.PasteCompleted=delegate{completed++;};bridge.ClipboardError=delegate{errors++;};
                bridge.RequestPasteHtml(1);editor.Dispose();read.SetResult("<p>Late image</p>");await Task.Delay(80);
                Check(completed==0 && errors==0,"Disposed editor still received an async clipboard callback");
                Check(bridge.Pasted==null && bridge.ReadClipboardHtmlAsync==null,"Clipboard bridge retained closed-editor delegates");
            }
        });
        await Run("clipboard callback failure stays local; a read error clears its pending paste",async delegate {
            int errors=0,completed=0;int unhandled=reports;
            var bridge=new ProgressHtmlEditorBridge();bridge.ReadClipboardHtmlAsync=()=>Task.FromResult("ok");
            bridge.ClipboardError=delegate{errors++;};bridge.PasteCompleted=delegate{completed++;throw new InvalidOperationException("fixture paste callback failure");};
            bridge.RequestPasteHtml(2);await Until(()=>completed>0,"Paste callback did not run");await Task.Delay(40);
            Check(errors==1,"Callback failure escaped instead of reporting to the editor");
            Check(reports==unhandled+1,"Clipboard failure was not recorded once");
            var read=new TaskCompletionSource<string>();bridge.ReadClipboardHtmlAsync=()=>read.Task;bridge.PasteCompleted=delegate(int id,string html){Check(html=="","Failed clipboard returned nonempty HTML");completed++;};
            bridge.RequestPasteHtml(3);read.SetException(new IOException("fixture clipboard read failure"));
            await Until(()=>completed==2,"Read failure did not complete/clear pending paste");Check(errors==2,"Clipboard read failure was not shown");
        });
        await Run("calendar month changes dispose old native buttons and handle date boundaries",async delegate {
            using(var calendar=new ProgressCalendarPopup(new DateTime(2026,11,27),null)) {
                calendar.Opacity=0;calendar.ShowInTaskbar=false;calendar.Show();
                var grid=CalendarGrid(calendar);var previous=grid.Controls.Cast<Control>().ToArray();
                var next=Descendants(calendar).OfType<Button>().First(button=>button.Text=="下月");next.PerformClick();
                Check(previous.All(control=>control.IsDisposed),"Month navigation leaked the old calendar controls");
                for(int i=0;i<100;i++)next.PerformClick();Check(grid.Controls.Count<=49,"Calendar controls accumulate across months");
            }
            foreach(DateTime boundary in new[]{DateTime.MinValue,DateTime.MaxValue})using(var calendar=new ProgressCalendarPopup(boundary,null)) {
                calendar.Opacity=0;calendar.ShowInTaskbar=false;calendar.Show();
                string direction=boundary.Year==1?"上月":"下月";
                var button=Descendants(calendar).OfType<Button>().First(item=>item.Text==direction);
                Check(!button.Enabled,"Out-of-range month button is enabled");button.PerformClick();
                Check(CalendarGrid(calendar).Controls.OfType<ProgressDayButton>().Any(),"Boundary calendar is blank");
            }
            await Task.Delay(20);
        });
        await Run("stale/null native draw notifications cannot dereference removed tree nodes",delegate {
            using(var tree=new TaskTreeView())using(var image=new Bitmap(100,100))using(var graphics=Graphics.FromImage(image)) {
                var draw=typeof(TaskTreeView).GetMethod("OnDrawNode",BindingFlags.Instance|BindingFlags.NonPublic);
                draw.Invoke(tree,new object[]{null});
                draw.Invoke(tree,new object[]{new DrawTreeNodeEventArgs(graphics,null,Rectangle.Empty,TreeNodeStates.Default)});
                draw.Invoke(tree,new object[]{new DrawTreeNodeEventArgs(graphics,new TreeNode("removed"),Rectangle.Empty,TreeNodeStates.Default)});
            }
            return Task.FromResult(true);
        });
        await Run("queued errors and async UI event exceptions preserve open drafts and the message loop",async delegate {
            using(var draft=new TextBox{Multiline=true,Text="unsaved draft"}) {
                host.Controls.Add(draft);int before=reports;
                FloatingUi.Post(host,delegate{throw new NullReferenceException("fixture delayed UI error");});
                await Until(()=>reports==before+1,"Deferred error was not reported");
                host.BeginInvoke(new Action(async delegate{await Task.Delay(20);throw new InvalidOperationException("fixture async UI event");}));
                await Until(()=>reports==before+2,"Async event error was not caught by the UI exception handler");
                Check(!host.IsDisposed && !draft.IsDisposed && draft.Text=="unsaved draft","Error handler closed a window or altered its draft");
                bool alive=false;FloatingUi.Post(host,delegate{alive=true;});await Until(()=>alive,"Message loop stopped after exception");
            }
        });
        await Run("crash log redacts credentials, rotates, and tolerates unwritable paths",delegate {
            string path=Path.Combine(directory,"TaskTrace-crash.log");
            FloatingRuntimeErrors.Report(new Exception("#tasktrace-local=private-session Bearer private-bearer token=private-token refresh_token=private-refresh"),"fixture redaction");
            string text=File.ReadAllText(path);Check(!text.Contains("private-session")&&!text.Contains("private-bearer")&&!text.Contains("private-token")&&!text.Contains("private-refresh"),"Crash diagnostics leaked credentials");
            File.WriteAllText(path,new string('x',2*1024*1024+1));FloatingRuntimeErrors.Report(new Exception("rotation"),"fixture rotation");
            Check(File.Exists(path+".previous") && new FileInfo(path).Length<4096,"Crash logs are not bounded by rotation");
            string file=Path.Combine(directory,"not-a-directory");File.WriteAllText(file,"keep");
            using(var blocked=new FloatingRuntimeErrors(file,delegate{reports++;}))blocked.Record(new Exception("disk unavailable"),"fixture unwritable",true);
            Check(File.ReadAllText(file)=="keep","Error reporting overwrote an existing file");
            return Task.FromResult(true);
        });
        if(failures.Count>0)throw new Exception(String.Join(Environment.NewLine,failures));
    }
    [STAThread]static int Main(string[] args) {
        directory=args[0];Console.OutputEncoding=Encoding.UTF8;Exception error=null;
        Application.SetUnhandledExceptionMode(UnhandledExceptionMode.CatchException);Application.EnableVisualStyles();Control.CheckForIllegalCrossThreadCalls=true;
        using(var diagnostics=new FloatingRuntimeErrors(directory,delegate{reports++;}))
        using(var host=new Form{Opacity=0,ShowInTaskbar=false,Width=640,Height=480}) {
            host.Shown+=async delegate{try{await Cases(host);}catch(Exception e){error=e;}finally{host.Close();}};Application.Run(host);
        }
        if(error!=null){Console.Error.WriteLine(error);return 1;}
        Console.WriteLine("PASS isolated runtime safety tests; no installed program, task data, tray, LAN or clipboard changed.");return 0;
    }
}
'@
$harness=Join-Path $testRoot 'CrashSafetyTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$copied=Join-Path $testRoot 'sources';[void][IO.Directory]::CreateDirectory($copied)
foreach($file in Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs') {Copy-Item -LiteralPath $file.FullName -Destination (Join-Path $copied $file.Name)}
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
function Invoke-CrashFixture([string]$Name,[bool]$ExpectedFailure) {
    $binary=Join-Path $testRoot ($Name+'.exe');$fixture=Join-Path $testRoot $Name;[void][IO.Directory]::CreateDirectory($fixture)
    $sources=@(Get-ChildItem -LiteralPath $copied -Filter 'Floating*.cs' | ForEach-Object FullName)
    & $compiler /nologo /target:exe /platform:x64 /optimize+ /main:CrashSafetyTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
    if($LASTEXITCODE){throw 'Crash-safety test compilation failed.'}
    $stdout=Join-Path $testRoot ($Name+'-results.txt');$stderr=Join-Path $testRoot ($Name+'-errors.txt')
    $process=Start-Process -FilePath $binary -ArgumentList ('"'+$fixture+'"') -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
    $processHandle=$process.Handle # Keep a real handle so Windows retains the exit code.
    if(!$process.WaitForExit(60000)){$process.Kill();throw 'Crash-safety fixtures timed out.'}
    $process.WaitForExit();$process.Refresh();$output=[IO.File]::ReadAllText($stdout);$errors=[IO.File]::ReadAllText($stderr)
    Write-Output $output
    if($ExpectedFailure){if($process.ExitCode -eq 0 -or !$errors.Contains('Disposed editor still received')){throw ('Previous faults did not reproduce: '+$errors)};Write-Output 'PASS previous sources reproduce late clipboard callbacks, escaping callback failures, stale native draw and unreleased calendar controls.'}
    elseif($process.ExitCode -ne 0){Write-Output $errors;throw ('Crash-safety regression failed. Exit code: '+$process.ExitCode)}
}
Invoke-CrashFixture 'CurrentSafety' $false
if($VerifyPreviousBehavior) {
    # Optional comparison uses the pre-audit source revision, even after this fix is committed.
    foreach($name in @('FloatingProgressEditor.cs','FloatingProgressDatePicker.cs','FloatingSimpleDetails.cs')) {
        $previous=@(& git -C $repoRoot show ('a7b22f363:portable/'+$name))
        if($LASTEXITCODE){throw 'Could not read baseline sources.'}
        [IO.File]::WriteAllText((Join-Path $copied $name),($previous -join "`n"),[Text.UTF8Encoding]::new($true))
    }
    Invoke-CrashFixture 'PreviousSafety' $true
}

$updaterSource=@'
using System;
using System.Reflection;
internal static class UpdaterInvalidPathTests {
    [STAThread]static int Main() {
        try {
            var start=typeof(TaskTraceUpdater).GetMethod("Main",BindingFlags.Static|BindingFlags.NonPublic);
            foreach(var args in new[]{new[]{"missing.zip","invalid\0root","--self-test"},new[]{"invalid\0zip",Environment.CurrentDirectory,"--self-test"}}) {
                int result=(int)start.Invoke(null,new object[]{args});
                if(result!=1)throw new Exception("Invalid updater paths were not rejected safely");
            }
            Console.WriteLine("PASS updater rejects invalid paths inside its error boundary, without an unhandled startup exception.");return 0;
        }catch(Exception e){Console.Error.WriteLine(e);return 1;}
    }
}
'@
$updaterHarness=Join-Path $testRoot 'UpdaterInvalidPathTests.cs';[IO.File]::WriteAllText($updaterHarness,$updaterSource,[Text.UTF8Encoding]::new($true))
$updaterBinary=Join-Path $testRoot 'UpdaterInvalidPathTests.exe'
& $compiler /nologo /target:exe /platform:x64 /main:UpdaterInvalidPathTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.IO.Compression.dll /reference:System.IO.Compression.FileSystem.dll ('/out:'+$updaterBinary) (Join-Path $PSScriptRoot 'TaskTraceUpdater.cs') (Join-Path $PSScriptRoot 'TaskTraceUpdateProgress.cs') $updaterHarness
if($LASTEXITCODE){throw 'Updater invalid-path test compilation failed.'}
& $updaterBinary
if($LASTEXITCODE){throw 'Updater invalid-path fixture failed.'}

Write-Output ('Results: '+$testRoot)
