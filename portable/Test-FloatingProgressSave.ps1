param(
    [string]$OutputDirectory = '.local-build/floating-progress-save-tests',
    [switch]$VerifyPreviousGetter
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Progress-save test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Only the test partial constructor is used: no app startup, session, tray, watcher,
# user database, system clipboard or real network. A private handler rejects all
# unexpected HTTP requests. Test-local copied sources hide child forms before Show;
# the actual EditProgress, HTML editor, Save button, API and save logic are unchanged.
$source = @'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;

sealed class ProgressSaveHttp : HttpMessageHandler {
    readonly JavaScriptSerializer json = new JavaScriptSerializer { MaxJsonLength=8388608 };
    readonly Dictionary<long,List<Dictionary<string,object>>> comments = new Dictionary<long,List<Dictionary<string,object>>>();
    long sequence=100;
    internal int DailyAttempts,DailyWrites,DailyPosts,DailyPuts,HistoryReads;
    internal bool FailNextProgress;
    internal TaskCompletionSource<bool> HeldHistory;
    internal const string Failure = "fixture write denied: progress remains editable";
    internal List<Dictionary<string,object>> Notes(long id) {
        List<Dictionary<string,object>> values;
        if(!comments.TryGetValue(id,out values)){values=new List<Dictionary<string,object>>();comments.Add(id,values);}
        return values;
    }
    internal void Seed(long taskId,string html) {Notes(taskId).Add(Note(++sequence,html));}
    Dictionary<string,object> Note(long id,string html) {
        return new Dictionary<string,object>{{"id",id},{"comment",html},{"created",DateTime.UtcNow.AddSeconds(id).ToString("o")},
            {"author",new Dictionary<string,object>{{"username",Environment.UserName}}}};
    }
    HttpResponseMessage Reply(object value,HttpStatusCode status=HttpStatusCode.OK) {
        return new HttpResponseMessage(status) { Content=new StringContent(json.Serialize(value),Encoding.UTF8,"application/json") };
    }
    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancellation) {
        string path=request.RequestUri.AbsolutePath,method=request.Method.Method;
        if(path=="/api/v2/tasktrace/team" && method=="GET")return Reply(new {enabled=false});
        if(path=="/api/v2/tasktrace/undo" && method=="GET")return Reply(new {id=0,label=""});
        var parts=path.Split('/');
        if(parts.Length<5 || parts[1]!="api" || parts[2]!="v2" || parts[3]!="tasks")
            throw new InvalidOperationException("Unexpected fake API route: "+method+" "+path);
        long taskId;
        if(!Int64.TryParse(parts[4],out taskId))throw new InvalidOperationException("Unexpected task id");
        if(parts.Length==5 && method=="GET")return Reply(new {id=taskId,title="Progress save fixture "+taskId,description=""});
        if(parts.Length>=6 && parts[5]=="comments") {
            if(method=="GET") {
                HistoryReads++;
                var gate=HeldHistory;
                if(gate!=null){HeldHistory=null;await gate.Task;}
                return Reply(new {items=Notes(taskId).OrderByDescending(note=>Convert.ToInt64(note["id"])).ToArray(),total_pages=1});
            }
            if(method=="POST" || method=="PUT") {
                var payload=json.Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync());
                string html=Convert.ToString(payload["comment"]);
                bool daily=html.Contains("每日进展");
                if(daily) {
                    DailyAttempts++;
                    if(FailNextProgress){FailNextProgress=false;return Reply(new {message=Failure},HttpStatusCode.Forbidden);}
                }
                Dictionary<string,object> saved;
                if(method=="PUT") {
                    long id=Int64.Parse(parts[6]);
                    saved=Notes(taskId).Single(note=>Convert.ToInt64(note["id"])==id);saved["comment"]=html;
                } else {saved=Note(++sequence,html);Notes(taskId).Add(saved);}
                if(daily){DailyWrites++;if(method=="POST")DailyPosts++;else DailyPuts++;}
                return Reply(saved);
            }
        }
        throw new InvalidOperationException("Unexpected fake API route: "+method+" "+path);
    }
}

internal sealed partial class FloatingWindow {
    FloatingWindow(string isolatedDirectory,ProgressSaveHttp fake) {
        root=data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=true;
        http.Dispose();http=new HttpClient(fake);token="isolated-test-token";autoSaveEnabled=false;
        Text="Isolated progress-save owner";Font=SystemFonts.MessageBoxFont;Icon=SystemIcons.Application;
        Opacity=0;TopMost=false;ShowInTaskbar=false;StartPosition=FormStartPosition.Manual;
        Bounds=new Rectangle(-20000,-20000,420,580);
    }
    protected override bool ShowWithoutActivation {get{return true;}}
    protected override CreateParams CreateParams {get{var value=base.CreateParams;value.ExStyle|=0x08000000;return value;}}
    sealed class SaveUi {
        internal Form Dialog;
        internal ProgressHtmlEditor Editor;
        internal ProgressDatePicker Day;
        internal Button Save;
        internal Label Feedback;
        internal Task Lifetime;
    }
    static IEnumerable<Control> SaveControls(Control parent) {
        foreach(Control child in parent.Controls){yield return child;foreach(var nested in SaveControls(child))yield return nested;}
    }
    static void SaveCheck(bool valid,string message){if(!valid)throw new Exception(message);}
    static async Task SaveUntil(Func<bool> condition,string message,int milliseconds=5000) {
        int elapsed=0;
        while(!condition() && elapsed<milliseconds){await Task.Delay(20);elapsed+=20;}
        SaveCheck(condition(),message);
    }
    async Task<SaveUi> OpenSaveUi(long taskId) {
        var lifetime=EditProgress(taskId,"unused fixture title");
        Form dialog=null;
        await SaveUntil(()=> {
            dialog=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="每日进展 · Progress save fixture "+taskId);
            return dialog!=null;
        },"Actual progress dialog did not open");
        var controls=SaveControls(dialog).ToList();
        var editor=controls.OfType<ProgressHtmlEditor>().Single(control=>control.AccessibleName!=null && control.AccessibleName.StartsWith("当天进展与更正"));
        var save=controls.OfType<Button>().Single(control=>control.Text.StartsWith("保存当天进展"));
        var day=controls.OfType<ProgressDatePicker>().Single();
        var feedback=controls.OfType<Label>().Single(label=>label.Parent is TableLayoutPanel && label.Dock==DockStyle.Fill && label.Parent.Controls.IndexOf(label)>=0 && ((TableLayoutPanel)label.Parent).GetRow(label)==8);
        var browser=(WebBrowser)editor.Controls[0];
        await SaveUntil(()=>browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Actual HTML editor did not become ready");
        await Task.Delay(60);
        SaveCheck(dialog.Opacity==0 && !dialog.ShowInTaskbar,"Test instrumentation did not keep child dialog hidden");
        return new SaveUi{Dialog=dialog,Editor=editor,Save=save,Day=day,Feedback=feedback,Lifetime=lifetime};
    }
    static void SilentProgressEdit(SaveUi ui,string text) {
        var browser=(WebBrowser)ui.Editor.Controls[0];
        // Deliberately do not call SimulateUserHtmlForTest, NotifyChanged or dispatch input.
        // This is the MSHTML missed-event path that the prior getter ignored.
        browser.Document.GetElementById("progress-editor").InnerHtml="<p>"+WebUtility.HtmlEncode(text)+"</p>";
    }
    async Task CloseSaveUi(SaveUi ui) {
        var registration=unsavedUpdateEditors.LastOrDefault();
        if(registration!=null)registration.Discard();
        ui.Dialog.Close();
        await SaveUntil(()=>ui.Lifetime.IsCompleted,"Actual progress dialog did not close");
        await ui.Lifetime;
    }
    static async Task PressSave(SaveUi ui,ProgressSaveHttp fake,int expectedWrites,string label) {
        ui.Save.PerformClick();
        await SaveUntil(()=>fake.DailyWrites>=expectedWrites && ui.Save.Enabled,label);
        SaveCheck(ui.Feedback.Text.Contains("正式保存"),"Actual save button did not report formal persistence: "+ui.Feedback.Text);
    }
    static string SavedDay(ProgressSaveHttp fake,long taskId,DateTime day) {
        var selected=DailyHistory(fake.Notes(taskId)).Where(note=>DayOf(note)==day.ToString("yyyy-MM-dd")).ToList();
        SaveCheck(selected.Count==1,"Expected one current progress record for "+taskId+" "+day.ToString("yyyy-MM-dd")+", got "+selected.Count);
        return Plain(ProgressBody((string)selected[0]["comment"],taskId));
    }
    internal async Task TestProgressSaveButtons(ProgressSaveHttp fake,bool previousGetter) {
        SaveUi active=null;Exception testFailure=null;
        try {
            active=await OpenSaveUi(42);
            int unchanged=fake.DailyAttempts;active.Save.PerformClick();await Task.Delay(60);
            SaveCheck(fake.DailyAttempts==unchanged,"Opening an unchanged blank date posted progress");
            SilentProgressEdit(active,"新填写的每日进展");
            await PressSave(active,fake,1,"MISSED_EVENT_SAVE_NOT_SENT: changed DOM was not sent by the actual Save button");
            SaveCheck(SavedDay(fake,42,DateTime.Today)=="新填写的每日进展","First button save did not persist entered text");
            SaveCheck(fake.DailyPosts==1 && fake.DailyPuts==0,"First button save used wrong persistence method");
            Console.WriteLine("PASS actual EditProgress Save button persists DOM changes with no input event.");
            if(previousGetter)throw new Exception("Previous getter unexpectedly saved the missed-event input");

            await CloseSaveUi(active);active=null;active=await OpenSaveUi(42);
            SaveCheck(Plain(active.Editor.Html)=="新填写的每日进展","Reopening did not show saved progress");
            unchanged=fake.DailyAttempts;active.Save.PerformClick();await Task.Delay(60);
            SaveCheck(fake.DailyAttempts==unchanged,"Unchanged reopened progress posted another comment");
            SilentProgressEdit(active,"同日第二次修改");
            await PressSave(active,fake,2,"Same-day button edit did not persist");
            SaveCheck(SavedDay(fake,42,DateTime.Today)=="同日第二次修改" && fake.DailyPosts==1 && fake.DailyPuts==1,
                "Same-day edit created a duplicate or failed to update the existing comment");
            Console.WriteLine("PASS reopen persisted content; unchanged save is a no-op; same-day edit updates one record.");

            fake.FailNextProgress=true;SilentProgressEdit(active,"失败后保留的输入");int beforeFailure=fake.DailyAttempts;
            active.Save.PerformClick();
            await SaveUntil(()=>fake.DailyAttempts==beforeFailure+1 && active.Save.Enabled,"Denied save did not complete");
            SaveCheck(active.Feedback.Text.Contains(ProgressSaveHttp.Failure),"API denial detail was hidden: "+active.Feedback.Text);
            SaveCheck(Plain(active.Editor.Html)=="失败后保留的输入" && fake.DailyWrites==2,"Denied save lost editor content or changed persisted data");
            SaveCheck(SavedDay(fake,42,DateTime.Today)=="同日第二次修改","Denied save changed backend state");
            await PressSave(active,fake,3,"Permission retry did not persist retained input");
            SaveCheck(SavedDay(fake,42,DateTime.Today)=="失败后保留的输入","Retry saved the wrong content");
            Console.WriteLine("PASS concrete API failure remains visible; input stays editable and retries successfully.");

            var loading=new TaskCompletionSource<bool>();fake.HeldHistory=loading;int beforeSwitch=fake.DailyAttempts,reads=fake.HistoryReads;
            DateTime yesterday=DateTime.Today.AddDays(-1);active.Day.Value=yesterday;
            await SaveUntil(()=>fake.HistoryReads>reads,"Date-switch history read was not intercepted");
            active.Save.PerformClick();await Task.Delay(50);
            SaveCheck(fake.DailyAttempts==beforeSwitch,"Save during date loading submitted old text under the new date");
            loading.SetResult(true);
            await SaveUntil(()=>active.Feedback.Text.Contains("此日期尚无进展") && active.Save.Enabled,"New date failed to finish loading");
            SaveCheck(Plain(active.Editor.Html)=="","Old-day editor content remained after switching to a blank date");
            SilentProgressEdit(active,"昨日独立的进展");
            await PressSave(active,fake,4,"New-day save failed after loading");
            SaveCheck(SavedDay(fake,42,yesterday)=="昨日独立的进展" && SavedDay(fake,42,DateTime.Today)=="失败后保留的输入","Date switch mixed old and new content");
            active.Day.Value=DateTime.Today;
            await SaveUntil(()=>Plain(active.Editor.Html)=="失败后保留的输入","Switching back did not restore the saved original date");
            Console.WriteLine("PASS Save is guarded during asynchronous date loading; both dates retain separate records.");

            await CloseSaveUi(active);active=null;
            fake.Seed(43,"<h3>每日进展 · "+DateTime.Today.ToString("yyyy-MM-dd")+"</h3><p>协作原进展</p>"+FloatingTeamMarkerHtml("seed-own-progress",Environment.UserName));
            active=await OpenSaveUi(43);SaveCheck(Plain(active.Editor.Html)=="协作原进展","Collaborative marker progress did not load");
            SilentProgressEdit(active,"协作窗口中新填写的进展");
            await PressSave(active,fake,5,"Collaborative Save button did not persist the changed DOM");
            SaveCheck(SavedDay(fake,43,DateTime.Today)=="协作窗口中新填写的进展","Collaborative current progress did not reflect the button save");
            await CloseSaveUi(active);active=null;active=await OpenSaveUi(43);
            SaveCheck(Plain(active.Editor.Html)=="协作窗口中新填写的进展","Reopening collaborative progress lost the latest revision");
            unchanged=fake.DailyAttempts;active.Save.PerformClick();await Task.Delay(60);
            SaveCheck(fake.DailyAttempts==unchanged,"Unchanged collaborative progress generated another revision");
            Console.WriteLine("PASS collaborative marker uses the actual append/merge path, survives reopening and skips unchanged revisions.");
        } catch(Exception error){testFailure=error;}
        if(active!=null)await CloseSaveUi(active);
        if(testFailure!=null)throw testFailure;
    }
    internal static int RunProgressSaveFixture(string directory,bool baseline) {
        int code=0;Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
        using(var fake=new ProgressSaveHttp())
        using(var host=new FloatingWindow(directory,fake)) {
            host.Shown+=async delegate {
                try {await host.TestProgressSaveButtons(fake,baseline);}
                catch(Exception error){code=1;Console.Error.WriteLine(error);}
                finally{host.Close();}
            };
            Application.Run(host);
        }
        return code;
    }
}
internal static class FloatingProgressSaveTests {
    [STAThread] static int Main(string[] args) {
        return FloatingWindow.RunProgressSaveFixture(AppDomain.CurrentDomain.BaseDirectory,args.Contains("--previous-getter"));
    }
}
'@
$harness=Join-Path $testRoot 'ProgressSaveHarness.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$copiedSources=Join-Path $testRoot 'sources'
[void][IO.Directory]::CreateDirectory($copiedSources)
foreach($file in Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs') {
    [IO.File]::WriteAllText((Join-Path $copiedSources $file.Name),[IO.File]::ReadAllText($file.FullName),[Text.UTF8Encoding]::new($true))
}
$windowSource=Join-Path $copiedSources 'FloatingWindow.cs'
$windowText=[IO.File]::ReadAllText($windowSource).Replace([string][char]13,'')
$displayPattern='            dialog.Show();'+[char]10+'            dialog.Activate();'
if(!$windowText.Contains($displayPattern)){throw 'Hidden test dialog instrumentation could not find ShowEditorWindowAsync.'}
$windowText=$windowText.Replace($displayPattern,'            dialog.Opacity=0;dialog.ShowInTaskbar=false;dialog.Show();')
[IO.File]::WriteAllText($windowSource,$windowText,[Text.UTF8Encoding]::new($true))
function Build-ProgressFixture([string]$Binary) {
    $sources=@(Get-ChildItem -LiteralPath $copiedSources -Filter 'Floating*.cs' | ForEach-Object FullName)
    & $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingProgressSaveTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$Binary) @sources $harness
    if($LASTEXITCODE -ne 0){throw 'Progress-save fixture compilation failed.'}
}
function Run-ProgressFixture([string]$Binary,[bool]$PreviousGetter) {
    $prefix=if($PreviousGetter){'previous-getter'}else{'current'}
    $stdout=Join-Path $testRoot ($prefix+'-results.txt')
    $stderr=Join-Path $testRoot ($prefix+'-errors.txt')
    $start=@{FilePath=$Binary;WindowStyle='Hidden';PassThru=$true;RedirectStandardOutput=$stdout;RedirectStandardError=$stderr}
    if($PreviousGetter){$start.ArgumentList='--previous-getter'}
    $process=Start-Process @start
    if(!$process.WaitForExit(90000)){$process.Kill();throw 'Progress-save fixtures timed out.'}
    $process.WaitForExit();$process.Refresh()
    $out=[IO.File]::ReadAllText($stdout);$errors=[IO.File]::ReadAllText($stderr)
    if($PreviousGetter) {
        if(!$errors.Contains('MISSED_EVENT_SAVE_NOT_SENT')){Write-Output $out;Write-Output $errors;throw 'Previous-getter baseline did not reproduce the missed-event save failure.'}
        Write-Output 'PASS previous getter fails at MISSED_EVENT_SAVE_NOT_SENT without modifying production files.'
    } else {
        Write-Output $out
        if($errors.Length -gt 0 -or $process.ExitCode){Write-Output $errors;throw ('Progress-save fixtures failed: '+$process.ExitCode)}
    }
}
$binary=Join-Path $testRoot 'ProgressSaveTests.exe'
Build-ProgressFixture $binary
Run-ProgressFixture $binary $false
if($VerifyPreviousGetter) {
    $editorSource=Join-Path $copiedSources 'FloatingProgressEditor.cs'
    $text=[IO.File]::ReadAllText($editorSource)
    $pattern='public int ChangeVersion \{ get \{ [^\r\n]+ \} \}'
    $matches=[regex]::Matches($text,$pattern)
    if($matches.Count -ne 1){throw 'Previous getter instrumentation could not find the isolated editor property.'}
    $text=[regex]::Replace($text,$pattern,'public int ChangeVersion { get { if(changeTimer.Enabled)NotifyChanged();return changeVersion; } }')
    [IO.File]::WriteAllText($editorSource,$text,[Text.UTF8Encoding]::new($true))
    $oldBinary=Join-Path $testRoot 'PreviousGetterSaveTests.exe'
    Build-ProgressFixture $oldBinary
    Run-ProgressFixture $oldBinary $true
}
Write-Output ('Results: '+$testRoot)
