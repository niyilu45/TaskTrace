param([string]$OutputDirectory = '.local-build/floating-update-window-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Update-window test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# The private test constructor bypasses startup, data, sessions, tray, network and watchers.
# Only this process's own forms are exercised; download and save completion use controlled tasks.
# No download, archive, installation handoff or real program exit is performed.
$source = @'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    FloatingWindow(string isolatedDirectory) {
        root=data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=true;
        Text="TaskTrace isolated update owner";Font=SystemFonts.MessageBoxFont;Icon=SystemIcons.Application;
        Opacity=0;TopMost=false;ShowInTaskbar=false;StartPosition=FormStartPosition.Manual;
        Bounds=new Rectangle(-20000,-20000,420,580);
    }
    protected override bool ShowWithoutActivation {get{return true;}}
    protected override CreateParams CreateParams {get{var value=base.CreateParams;value.ExStyle|=0x08000000;return value;}}
    static void UpdateAssert(bool value,string message){if(!value)throw new Exception(message);}
    static async Task UpdateTick(Action action) {
        using(var tick=new System.Windows.Forms.Timer{Interval=25}) {
            var completion=new TaskCompletionSource<bool>();
            tick.Tick+=delegate {tick.Stop();try{action();completion.TrySetResult(true);}catch(Exception error){completion.TrySetException(error);}};
            tick.Start();await completion.Task;
        }
    }
    static IEnumerable<Control> UpdateControls(Control root) {
        foreach(Control child in root.Controls){yield return child;foreach(var descendant in UpdateControls(child))yield return descendant;}
    }
    static void PrepareProgressForTest(UpdateDownloadDialog progress) {
        progress.Opacity=0;progress.Location=new Point(-20000,-20000);
    }

    sealed class UpdateDecisionDriver : IDisposable {
        readonly System.Windows.Forms.Timer timer=new System.Windows.Forms.Timer{Interval=15};
        readonly Queue<DialogResult> choices;
        readonly HashSet<Form> handled=new HashSet<Form>();
        Exception failure;
        internal int Count;
        internal readonly List<string> Labels=new List<string>();
        internal UpdateDecisionDriver(params DialogResult[] decisions) {
            choices=new Queue<DialogResult>(decisions);timer.Tick+=Tick;timer.Start();
        }
        void Tick(object sender,EventArgs args) {
            var dialog=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="TaskTrace · 更新前检查" && form.Visible && !form.IsDisposed && !handled.Contains(form));
            if(dialog==null)return;handled.Add(dialog);
            try {
                dialog.Opacity=0;dialog.Location=new Point(-20000,-20000);
                Labels.Add(String.Join("\n",UpdateControls(dialog).OfType<TextBox>().Select(box=>box.Text)));
                if(choices.Count==0)throw new Exception("Unexpected additional unsaved-content decision");
                var choice=choices.Dequeue();var button=UpdateControls(dialog).OfType<Button>().Single(item=>item.DialogResult==choice);
                Count++;button.PerformClick();
            } catch(Exception error) {failure=error;dialog.DialogResult=DialogResult.Cancel;dialog.Close();}
        }
        internal void Check(int expected) {
            if(failure!=null)throw failure;
            UpdateAssert(Count==expected && choices.Count==0,"Unsaved-content decision count was "+Count+", expected "+expected);
        }
        public void Dispose(){timer.Stop();timer.Dispose();}
    }

    async Task TestUpdateSuccess() {
        using(var editor=new IsolatedUpdateEditor()) {
            editor.Show();int clicks=0;var button=new Button{Text="Editor action",Dock=DockStyle.Fill};button.Click+=delegate{clicks++;};editor.Controls.Add(button);
            var release=new TaskCompletionSource<bool>();UpdateDownloadDialog shown=null;
            var downloading=ShowUpdateDownloadProgress("v99.0.0",async delegate(UpdateDownloadDialog progress){shown=progress;PrepareProgressForTest(progress);await release.Task;});
            await UpdateTick(delegate {
                UpdateAssert(shown!=null && !downloading.IsCompleted,"Controlled download was not held open");
                UpdateAssert(Enabled && editor.Enabled && !shown.Modal && shown.Owner==null,"Progress blocked the main window or another editor");
                var original=Location;Location=new Point(original.X+17,original.Y+19);UpdateAssert(Location!=original,"Main window could not move while downloading");
                editor.Text="Independent editor remains usable";button.PerformClick();UpdateAssert(clicks==1,"Independent editor could not use its action");
                UpdateAssert(shown.ShowInTaskbar && shown.MinimizeBox && shown.Icon!=null,"Progress window cannot be recovered from the taskbar");
                shown.WindowState=FormWindowState.Minimized;UpdateAssert(shown.WindowState==FormWindowState.Minimized,"Progress did not minimize");
                shown.WindowState=FormWindowState.Normal;UpdateAssert(shown.Visible && shown.WindowState==FormWindowState.Normal,"Progress did not restore");
                release.TrySetResult(true);
            });
            UpdateAssert(await downloading,"Successful operation did not report success");
            UpdateAssert(shown.IsDisposed && Enabled && editor.Enabled,"Success left a window disabled or progress undisposed");
            editor.Close();
        }
        Console.WriteLine("PASS modeless success: main move, independent editor action, taskbar icon, minimize/restore and disposal.");
    }

    async Task TestUpdateCancellation(string action) {
        var cancelled=new TaskCompletionSource<bool>();var cleanup=new TaskCompletionSource<bool>();
        UpdateDownloadDialog shown=null;CancellationToken captured=CancellationToken.None;
        var downloading=ShowUpdateDownloadProgress("v99.0.0",async delegate(UpdateDownloadDialog progress) {
            shown=progress;PrepareProgressForTest(progress);captured=progress.Token;
            using(captured.Register(delegate{cancelled.TrySetResult(true);})) {
                await cancelled.Task;await cleanup.Task;captured.ThrowIfCancellationRequested();
            }
        });
        await UpdateTick(delegate {
            UpdateAssert(shown!=null && Enabled,"Cancellation fixture did not open modeless");
            if(action=="button")UpdateControls(shown).OfType<Button>().Single(button=>button.Text=="取消更新").PerformClick();
            else if(action=="close")shown.Close();
            else {
                allowExit=true;OnFormClosing(new FormClosingEventArgs(CloseReason.UserClosing,false));allowExit=false;
            }
            UpdateAssert(captured.IsCancellationRequested,"Cancellation did not signal the operation: "+action);
            UpdateAssert(!shown.IsDisposed && shown.Visible && !downloading.IsCompleted,"Progress closed before operation cleanup completed: "+action);
            UpdateAssert(Enabled,"Cancellation disabled the main window");
            cleanup.TrySetResult(true);
        });
        UpdateAssert(!await downloading,"Cancelled operation reported success: "+action);
        UpdateAssert(shown.IsDisposed && Enabled,"Cancellation did not dispose progress safely: "+action);
        Console.WriteLine("PASS cancellation: "+action+" signals token, waits for cleanup and returns false.");
    }

    async Task TestUpdateFailure() {
        var release=new TaskCompletionSource<bool>();UpdateDownloadDialog shown=null;Exception caught=null;
        var downloading=ShowUpdateDownloadProgress("v99.0.0",async delegate(UpdateDownloadDialog progress){shown=progress;PrepareProgressForTest(progress);await release.Task;throw new InvalidOperationException("isolated download failure");});
        await UpdateTick(delegate{UpdateAssert(Enabled && !shown.Modal,"Failed-operation fixture was modal");release.TrySetResult(true);});
        try{await downloading;}catch(Exception error){caught=error;}
        UpdateAssert(caught is InvalidOperationException && caught.Message=="isolated download failure","Download failure was swallowed or changed");
        UpdateAssert(shown.IsDisposed && Enabled,"Failure left progress open or main window disabled");
        Console.WriteLine("PASS failure: operation error propagates after progress disposal.");
    }

    async Task TestNewDirtyEditorDecision(DialogResult choice) {
        UpdateAssert((await PrepareUnsavedContentForUpdate()).Count==0,"Initial clean preparation unexpectedly prompted");
        var release=new TaskCompletionSource<bool>();IDisposable registration=null;
        bool dirty=true;int saved=0,discarded=0;
        var downloading=ShowUpdateDownloadProgress("v99.0.0",async delegate(UpdateDownloadDialog progress){PrepareProgressForTest(progress);await release.Task;});
        try {
            await UpdateTick(delegate {
                registration=RegisterUnsavedUpdateEditor("Created during download",()=>dirty,delegate{saved++;dirty=false;return Task.FromResult(true);},delegate{discarded++;dirty=false;});
                release.TrySetResult(true);
            });
            UpdateAssert(await downloading,"Dirty-editor fixture download failed");
            using(var decisions=new UpdateDecisionDriver(choice)) {
                var result=await PrepareUnsavedContentForUpdate();decisions.Check(1);
                UpdateAssert(decisions.Labels[0].Contains("Created during download"),"New editor was missing from final update preparation");
                if(choice==DialogResult.Cancel)UpdateAssert(result==null && dirty && saved==0 && discarded==0,"Cancel lost the new draft");
                else if(choice==DialogResult.Yes)UpdateAssert(result!=null && result.Count==0 && !dirty && saved==1 && discarded==0,"Save did not save the new draft");
                else {
                    UpdateAssert(result!=null && result.Count==1 && dirty && saved==0 && discarded==0,"Force discarded before the caller committed its decision");
                    foreach(var discard in result)discard();
                    UpdateAssert(!dirty && discarded==1,"Force did not supply the new draft's discard action");
                }
            }
        } finally {if(registration!=null)registration.Dispose();}
        Console.WriteLine("PASS final draft check: new editor during download / "+choice+".");
    }

    async Task TestDirtyEditorAddedDuringSave() {
        bool firstDirty=true,secondDirty=true;int savedFirst=0,savedSecond=0;
        var saveStarted=new TaskCompletionSource<bool>();var releaseSave=new TaskCompletionSource<bool>();IDisposable second=null;
        using(RegisterUnsavedUpdateEditor("First asynchronous draft",()=>firstDirty,async delegate{savedFirst++;saveStarted.TrySetResult(true);await releaseSave.Task;firstDirty=false;return true;},delegate{firstDirty=false;}))
        using(var decisions=new UpdateDecisionDriver(DialogResult.Yes,DialogResult.Yes)) {
            var preparing=PrepareUnsavedContentForUpdate();await saveStarted.Task;
            await UpdateTick(delegate {
                second=RegisterUnsavedUpdateEditor("Added while saving",()=>secondDirty,delegate{savedSecond++;secondDirty=false;return Task.FromResult(true);},delegate{secondDirty=false;});
                releaseSave.TrySetResult(true);
            });
            try {
                var result=await preparing;decisions.Check(2);
                UpdateAssert(result!=null && result.Count==0 && savedFirst==1 && savedSecond==1 && !firstDirty && !secondDirty,"Final preparation missed an editor created while another save awaited");
                UpdateAssert(decisions.Labels[1].Contains("Added while saving"),"Repeated decision did not show the newly dirty editor");
            } finally {if(second!=null)second.Dispose();}
        }
        Console.WriteLine("PASS save race: asynchronous save re-enumerates and separately handles newly dirty editors.");
    }

    async Task TestUpdateBusyBeforeAwait() {
        bool dirty=true;var started=new TaskCompletionSource<bool>();var release=new TaskCompletionSource<bool>();
        using(RegisterUnsavedUpdateEditor("Busy guard draft",()=>dirty,async delegate{started.TrySetResult(true);await release.Task;dirty=false;return true;},delegate{dirty=false;}))
        using(var decisions=new UpdateDecisionDriver(DialogResult.Yes)) {
            var first=DownloadAndStartUpdate("isolated-no-network");await started.Task;
            UpdateAssert(updateBusy && !first.IsCompleted,"Update busy guard was not acquired before the first asynchronous save");
            var second=DownloadAndStartUpdate("isolated-no-network");UpdateAssert(second.IsCompleted,"A second update entered while the first preparation was pending");await second;
            closing=true;release.TrySetResult(true);
            try{await first;decisions.Check(1);UpdateAssert(!updateBusy,"Aborted preparation did not release the update busy guard");}
            finally{closing=false;}
        }
        Console.WriteLine("PASS reentry: update busy acquired before first await; second update rejected; shutdown aborts before network or handoff.");
    }

    internal static async Task TestUpdateWindows(FloatingWindow window) {
        await window.TestUpdateSuccess();
        await window.TestUpdateCancellation("button");await window.TestUpdateCancellation("close");await window.TestUpdateCancellation("shutdown");
        await window.TestUpdateFailure();
        await window.TestNewDirtyEditorDecision(DialogResult.Cancel);await window.TestNewDirtyEditorDecision(DialogResult.Yes);await window.TestNewDirtyEditorDecision(DialogResult.Ignore);
        await window.TestDirtyEditorAddedDuringSave();await window.TestUpdateBusyBeforeAwait();
        UpdateAssert(window.unsavedUpdateEditors.Count==0,"Unsaved editor registrations leaked between cases");
        UpdateAssert(!Directory.EnumerateFileSystemEntries(window.data).Any(path=>Path.GetFileName(path)=="update-ready.json" || Path.GetFileName(path)=="update-state.json" || Path.GetFileName(path)=="updates"),"Isolated test wrote an update handoff or download");
        Console.WriteLine("PASS isolation: no normal startup, data, sessions, clipboard, network, archives, install handoff or real application exit.");
    }
    internal static FloatingWindow CreateUpdateTestWindow(string directory){return new FloatingWindow(directory);}
}
internal sealed class IsolatedUpdateEditor : Form {
    internal IsolatedUpdateEditor(){Opacity=0;ShowInTaskbar=false;StartPosition=FormStartPosition.Manual;Bounds=new Rectangle(-20000,-20000,300,200);}
    protected override bool ShowWithoutActivation {get{return true;}}
    protected override CreateParams CreateParams {get{var value=base.CreateParams;value.ExStyle|=0x08000000;return value;}}
}
internal static class FloatingUpdateWindowTests {
    [STAThread] static int Main(string[] args) {
        Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);int exitCode=0;
        using(var window=FloatingWindow.CreateUpdateTestWindow(args[0])) {
            window.Shown+=async delegate {
                try{await FloatingWindow.TestUpdateWindows(window);}
                catch(Exception error){Console.Error.WriteLine(error);exitCode=1;}
                finally{window.Close();}
            };
            Application.Run(window);
        }
        return exitCode;
    }
}
'@
$harness = Join-Path $testRoot 'FloatingUpdateWindowTests.cs'
[IO.File]::WriteAllText($harness, $source, [Text.UTF8Encoding]::new($false))
$compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary = Join-Path $testRoot 'FloatingUpdateWindowTests.exe'
$sources = @(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingUpdateWindowTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/win32manifest:' + (Join-Path $PSScriptRoot 'FloatingWindow.manifest')) ('/out:' + $binary) @sources $harness
if ($LASTEXITCODE -ne 0) { throw 'Update-window regression compilation failed.' }
$stdout = Join-Path $testRoot 'results.txt'
$stderr = Join-Path $testRoot 'errors.txt'
$process = Start-Process -FilePath $binary -ArgumentList ('"' + $testRoot + '"') -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if (!$process.WaitForExit(60000)) { $process.Kill(); throw ('Update-window tests timed out: ' + $testRoot) }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if ((Get-Item -LiteralPath $stderr).Length -gt 0) { Get-Content -LiteralPath $stderr -Encoding UTF8; throw ('Update-window tests failed: ' + $testRoot) }
if ($process.ExitCode) { throw ('Update-window tests failed: ' + $process.ExitCode + '; ' + $testRoot) }
Write-Output ('Results: ' + $stdout)
