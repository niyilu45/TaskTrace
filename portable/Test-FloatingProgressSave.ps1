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
    readonly Dictionary<long,byte[]> attachments=new Dictionary<long,byte[]>();
    internal int AttachmentWrites;
    internal int DailyAttempts,DailyWrites,DailyPosts,DailyPuts,HistoryReads;
    internal bool FailNextProgress,FailNextOutstanding,FailNextAttachment;
    internal int OutstandingWrites,PriorityAttempts;
    internal TaskCompletionSource<bool> HeldHistory;
    internal const string AttachmentFailure = "fixture: attachment could not be written";
    internal const string Failure = "fixture write denied: progress remains editable";
    internal List<Dictionary<string,object>> Notes(long id) {
        List<Dictionary<string,object>> values;
        if(!comments.TryGetValue(id,out values)){values=new List<Dictionary<string,object>>();comments.Add(id,values);}
        return values;
    }
    internal void Seed(long taskId,string html) {Notes(taskId).Add(Note(++sequence,html));}
    internal void SeedAttachment(long id,byte[] bytes) {attachments[id]=bytes;}
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
        if(path=="/api/v2/projects" && method=="GET")return Reply(new {items=new object[0],total=0});
        if(path=="/api/v2/tasktrace/tasks/44/priority" && method=="PUT") {
            PriorityAttempts++;
            return Reply(new {message="fixture: secondary priority update failed"},HttpStatusCode.Forbidden);
        }
        var parts=path.Split('/');
        if(parts.Length<5 || parts[1]!="api" || parts[2]!="v2" || parts[3]!="tasks")
            throw new InvalidOperationException("Unexpected fake API route: "+method+" "+path);
        long taskId;
        if(!Int64.TryParse(parts[4],out taskId))throw new InvalidOperationException("Unexpected task id");
        if(parts.Length==5 && method=="GET")return Reply(new {id=taskId,title="Progress save fixture "+taskId,description=""});
        if(parts.Length==6 && parts[5]=="attachments" && method=="POST") {
            if(FailNextAttachment){FailNextAttachment=false;return Reply(new {success=(object)null,errors=new[]{new {message=AttachmentFailure}}},HttpStatusCode.Created);}
            var multipart=(MultipartFormDataContent)request.Content;
            long attachmentId=++sequence;
            attachments[attachmentId]=await multipart.First().ReadAsByteArrayAsync();
            AttachmentWrites++;
            return Reply(new {success=new[]{new {id=attachmentId}},errors=new object[0]});
        }
        if(parts.Length==7 && parts[5]=="attachments" && method=="GET") {
            return new HttpResponseMessage(HttpStatusCode.OK) {Content=new ByteArrayContent(attachments[Int64.Parse(parts[6])])};
        }
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
                bool outstanding=taskId==44 && html.StartsWith("<h3 data-tasktrace-comment-type=\"outstanding\">",StringComparison.Ordinal);
                if(outstanding && FailNextOutstanding) {
                    FailNextOutstanding=false;
                    return Reply(new {message="fixture: outstanding content write failed"},HttpStatusCode.Forbidden);
                }
                Dictionary<string,object> saved;
                if(method=="PUT") {
                    long id=Int64.Parse(parts[6]);
                    saved=Notes(taskId).Single(note=>Convert.ToInt64(note["id"])==id);saved["comment"]=html;
                } else {saved=Note(++sequence,html);Notes(taskId).Add(saved);}
                if(daily){DailyWrites++;if(method=="POST")DailyPosts++;else DailyPuts++;}
                if(outstanding)OutstandingWrites++;
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
    static async Task<Form> OpenPopout(Form owner) {
        SaveControls(owner).OfType<Button>().First(button=>button.Text=="窗口展示").PerformClick();
        Form popup=null;
        await SaveUntil(()=> {
            popup=owner.OwnedForms.FirstOrDefault(form=>SaveControls(form).OfType<ProgressHtmlEditor>().Any());
            return popup!=null;
        },"Window-display button did not open an owned editor");
        var mirror=SaveControls(popup).OfType<ProgressHtmlEditor>().Single();
        var browser=(WebBrowser)mirror.Controls[0];
        await SaveUntil(()=>browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Detached editor was not ready");
        SaveCheck(!popup.Modal && owner.Enabled,"Detached editor blocked the original window");
        return popup;
    }

    async Task TestProgressPopout(ProgressSaveHttp fake) {
        SaveUi ui=null,other=null;Exception failure=null;
        try {
            ui=await OpenSaveUi(45);
            var clipboard=new TaskCompletionSource<string>();
            ((ProgressHtmlEditorBridge)((WebBrowser)ui.Editor.Controls[0]).ObjectForScripting).ReadClipboardHtmlAsync=()=>clipboard.Task;
            int baselineVersion=ui.Editor.ChangeVersion;
            var popup=await OpenPopout(ui.Dialog);
            var mirror=SaveControls(popup).OfType<ProgressHtmlEditor>().Single();
            var browser=(ProgressEditorBrowser)mirror.Controls[0];
            SaveControls(ui.Dialog).OfType<Button>().Single(button=>button.Text=="窗口展示").PerformClick();
            SaveCheck(ui.Dialog.OwnedForms.Length==1 && ui.Save.Enabled && ui.Editor.ChangeVersion==baselineVersion,
                "Opening or activating a popout disabled Save, duplicated the window or dirtied unchanged content");

            var originalBrowser=(WebBrowser)ui.Editor.Controls[0];
            browser.Document.GetElementById("progress-editor").InnerHtml="<p>普通按键实时输入</p>";
            browser.Document.InvokeScript("eval",new object[]{"document.getElementById('progress-editor').fireEvent('onkeyup')"});
            await SaveUntil(()=>originalBrowser.Document.GetElementById("progress-editor").InnerText=="普通按键实时输入",
                "Detached typing without MSHTML input did not update the original within 500 ms",500);
            var typingClock=System.Diagnostics.Stopwatch.StartNew();
            int typingUpdates=0;
            while(typingClock.ElapsedMilliseconds<250) {
                browser.Document.GetElementById("progress-editor").InnerHtml="<p>持续输入 "+(++typingUpdates)+"</p>";
                browser.Document.InvokeScript("eval",new object[]{"document.getElementById('progress-editor').fireEvent('oninput')"});
                await Task.Delay(10);
            }
            SaveCheck(originalBrowser.Document.GetElementById("progress-editor").InnerText.Contains("持续输入"),
                "Continuous typing starved real-time synchronization until typing stopped");
            // Let the last simulated keystroke arrive before independently editing the other
            // hidden control; both directions still have their own 500 ms deadline.
            await SaveUntil(()=>originalBrowser.Document.GetElementById("progress-editor").InnerText=="持续输入 "+typingUpdates,
                "Final continuous input did not reach the original within 500 ms",500);
            originalBrowser.Document.GetElementById("progress-editor").InnerHtml="<p>原窗口实时输入</p>";
            originalBrowser.Document.InvokeScript("eval",new object[]{"document.getElementById('progress-editor').fireEvent('onkeyup')"});
            await SaveUntil(()=>browser.Document.GetElementById("progress-editor").InnerText=="原窗口实时输入",
                "Original typing did not update the detached editor within 500 ms",500);
            Console.WriteLine("PASS linked editor live DOM updates in both directions during native and continuous input.");

            browser.Document.GetElementById("progress-editor").InnerHtml="<p>小窗口立即输入</p>";
            int writes=fake.DailyWrites+1;
            await PressSave(ui,fake,writes,"Original Save missed detached input without an input event");
            SaveCheck(SavedDay(fake,45,DateTime.Today)=="小窗口立即输入","Detached text was not formally saved");
            ui.Editor.SimulateUserHtmlForTest("<p>原窗口修改</p>");
            SaveCheck(Plain(mirror.Html)=="原窗口修改","Original edits did not reach the detached editor");
            SaveControls(popup).OfType<Button>().Single(button=>button.Text=="保存").PerformClick();
            writes++;await SaveUntil(()=>fake.DailyWrites==writes && ui.Save.Enabled,"Detached Save did not reuse original persistence");
            SaveCheck(SavedDay(fake,45,DateTime.Today)=="原窗口修改","Detached Save used different content");

            browser.Document.InvokeScript("eval",new object[]{"var e=document.getElementById('progress-editor'),r=document.createRange(),s=window.getSelection();r.selectNodeContents(e);r.collapse(false);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange()"});
            browser.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}})"});
            await SaveUntil(()=>ui.Editor.IsPastePending,"Detached async paste was not visible to original Save");
            int before=fake.DailyWrites;
            ui.Save.PerformClick();
            SaveCheck(fake.DailyWrites==before && ui.Feedback.Text.Contains("正在读取粘贴"),"Original Save ignored detached paste still in progress");
            string picture;
            using(var bitmap=new Bitmap(40,24))
            using(var stream=new MemoryStream()) {
                using(var drawing=Graphics.FromImage(bitmap))drawing.Clear(Color.SteelBlue);
                bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);
                picture="data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());
            }
            clipboard.SetResult("<p>图文前<img src='"+picture+"'>图文后</p>");
            await SaveUntil(()=>!ui.Editor.IsPastePending && ui.Editor.Html.Contains("图文后") && ui.Editor.ImageCount==1,
                "Pasted picture and surrounding text did not synchronize");
            browser.ShortcutForTest(Keys.Control|Keys.Z);
            await SaveUntil(()=>ui.Editor.Html.Contains("原窗口修改") && ui.Editor.ImageCount==0,"Detached paste undo did not synchronize");
            browser.ShortcutForTest(Keys.Control|Keys.Y);
            await SaveUntil(()=>ui.Editor.Html.Contains("图文后") && ui.Editor.ImageCount==1,"Detached paste redo did not synchronize");
            writes=fake.DailyWrites+1;
            await PressSave(ui,fake,writes,"Original Save did not persist a detached pasted image");
            SaveCheck(fake.AttachmentWrites==1 && mirror.ImageCount==1 && mirror.Html.Contains("data-tasktrace-src"),
                "Saving detached media lost its image or failed to reload the saved attachment");
            Console.WriteLine("PASS actual modeless popout, duplicate activation, immediate bidirectional save and async image paste.");

            mirror.SimulateUserHtmlForTest("<p>失败后保留的小窗口内容</p>");
            fake.FailNextProgress=true;
            ui.Save.PerformClick();
            await SaveUntil(()=>!fake.FailNextProgress && ui.Save.Enabled,"Detached save failure did not finish");
            SaveCheck(SaveControls(popup).OfType<Label>().Any(label=>label.Text.Contains(ProgressSaveHttp.Failure)),
                "Detached window did not show the original save failure");
            SaveCheck(Plain(mirror.Html)=="失败后保留的小窗口内容" && ui.Feedback.Text.Contains(ProgressSaveHttp.Failure),
                "A failed original save lost detached changes");
            writes=fake.DailyWrites+1;
            await PressSave(ui,fake,writes,"Retry failed with the detached window open");
            DateTime yesterday=DateTime.Today.AddDays(-1);
            ui.Day.Value=yesterday;
            await SaveUntil(()=>Plain(mirror.Html)=="" && popup.Text.Contains(yesterday.ToString("yyyy-MM-dd")),
                "Detached editor retained the previous date after switching");
            mirror.SimulateUserHtmlForTest("<p>小窗口的昨日记录</p>");
            writes=fake.DailyWrites+1;
            await PressSave(ui,fake,writes,"Detached historical date could not be saved");
            SaveCheck(SavedDay(fake,45,yesterday)=="小窗口的昨日记录" && SavedDay(fake,45,DateTime.Today)=="失败后保留的小窗口内容",
                "Detached editor mixed progress from different dates");

            other=await OpenSaveUi(46);
            var otherPopup=await OpenPopout(other.Dialog);
            var otherEditor=SaveControls(otherPopup).OfType<ProgressHtmlEditor>().Single();
            otherEditor.SimulateUserHtmlForTest("<p>独立任务内容</p>");
            SaveCheck(Plain(ui.Editor.Html)=="小窗口的昨日记录","An unrelated task's popout overwrote the first task");
            await CloseSaveUi(other);
            other=null;
            SaveCheck(otherPopup.IsDisposed && !popup.IsDisposed,"Closing one original affected the wrong popout");
            popup.Close();
            SaveCheck(popup.IsDisposed && !ui.Dialog.IsDisposed && ui.Save.Enabled,"Closing the popout closed or blocked its original");
            popup=await OpenPopout(ui.Dialog);
            await CloseSaveUi(ui);
            ui=null;
            SaveCheck(popup.IsDisposed,"Closing the original left its popout alive");
            Console.WriteLine("PASS failed-save retention, date isolation, independent task windows and owner-close cleanup.");
        } catch(Exception error){failure=error;}
        if(other!=null)await CloseSaveUi(other);
        if(ui!=null)await CloseSaveUi(ui);
        if(failure!=null)throw failure;
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
    async Task TestOutstandingFromTaskWindow(ProgressSaveHttp fake) {
        const long id=44;
        var parent=await OpenSaveUi(id);
        Form child=null;Exception failure=null;
        autoSaveEnabled=true;
        autoSaveSeconds=1;
        Func<Form> findOutstanding=()=>Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="遗留事项 · 所有日期共享");
        try {
            SaveControls(parent.Dialog).OfType<Button>().Single(button=>button.Text=="遗留事项 · 所有日期共享").PerformClick();
            await SaveUntil(()=>(child=findOutstanding())!=null,"Task window did not open the outstanding editor");
            var controls=SaveControls(child).ToList();
            var input=controls.OfType<TextBox>().Single(box=>box.AccessibleName=="遗留事项内容");
            var note=controls.OfType<ProgressHtmlEditor>().Single();
            var priority=controls.OfType<ComboBox>().Single(box=>box.AccessibleName=="遗留事项优先级");
            var save=controls.OfType<Button>().Single(button=>button.Text=="添加一条");
            var feedback=child.Controls[0].Controls.OfType<Label>().Last();
            var list=controls.OfType<ListBox>().Single();
            SaveCheck(controls.OfType<Button>().Count(button=>button.Text=="窗口展示")==1,
                "Outstanding editor has duplicate window-display actions");
            var bulk=controls.OfType<Button>().Single(button=>button.AccessibleName=="批量导入遗留事项");
            child.PerformLayout();bulk.Parent.PerformLayout();
            SaveCheck(bulk.Text=="批量导入…" && bulk.Width>=80 && bulk.Height>=23 &&
                bulk.Parent.ClientRectangle.Contains(bulk.Bounds),"Bulk-import label or button is clipped");
            var editor=unsavedUpdateEditors.Last();
            var browser=(WebBrowser)note.Controls[0];
            await SaveUntil(()=>browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Outstanding note editor not ready");
            input.Text="从任务窗口新增";
            priority.SelectedIndex=3;
            var notePopup=await OpenPopout(child);
            var detachedNote=SaveControls(notePopup).OfType<ProgressHtmlEditor>().Single();
            var detachedContent=SaveControls(notePopup).OfType<TextBox>().Single();
            detachedContent.Text="从任务窗口新增并由小窗口修改";
            SaveCheck(input.Text==detachedContent.Text,"Detached item content did not synchronize to the original");
            input.Text="从任务窗口新增";
            SaveCheck(detachedContent.Text==input.Text,"Original item content did not synchronize to the popout");
            SaveCheck(detachedNote.SimulateUserHtmlForTest("<p>已经保存的备注</p>"),"Could not enter outstanding note in detached editor");
            SaveCheck(note.Html.Contains("已经保存的备注") && save.Enabled,"Detached outstanding note blocked saving or failed to synchronize");
            await SaveUntil(()=>HasDraftCache("outstanding",id,"new"),"Outstanding draft was not cached before saving");
            save.PerformClick();
            await SaveUntil(()=>save.Enabled,"Outstanding save button stayed disabled");
            SaveCheck(fake.OutstandingWrites==1,"Outstanding add wrote "+fake.OutstandingWrites+" records; feedback: "+feedback.Text);
            var savedItems=ReadShared(fake.Notes(id)).Items;
            SaveCheck(savedItems.Count==1,"Expected one persisted outstanding item, got "+savedItems.Count);
            var stored=savedItems[0];
            SaveCheck(OutstandingEditorText(stored.Html)=="从任务窗口新增" && Plain(stored.NoteHtml)=="已经保存的备注" && stored.Priority==3,
                "The single content write did not retain the text, note and selected priority");
            SaveCheck(fake.PriorityAttempts==0 && !editor.IsDirty() && !HasDraftCache("outstanding",id,"new") &&
                input.Text=="" && list.Items.Count==1 && feedback.Text.Contains("已正式保存"),
                "OUTSTANDING_SAVED_BUT_DIRTY: priority requests="+fake.PriorityAttempts+", draft="+HasDraftCache("outstanding",id,"new")+", input="+json.Serialize(input.Text)+", note="+json.Serialize(note.Html)+", feedback="+feedback.Text);
            child.Width=child.MinimumSize.Width;
            child.PerformLayout();
            save.Parent.PerformLayout();
            var visibleButtons=controls.OfType<Button>().Where(button=>button.Parent==save.Parent && button.Visible).ToArray();
            SaveCheck(visibleButtons.All(button=>save.Parent.ClientRectangle.Contains(button.Bounds)),
                "Bottom actions are clipped at minimum width");
            var taskButtons=visibleButtons.Select(button=>button==save?"save":button.Text).ToArray();
            SaveCheck(taskButtons.Contains("编辑所属任务"),"Task-entry outstanding window is missing the shared parent-edit action");

            // A later genuine change must still remain a draft if the content request fails.
            input.Text="写入失败时保留";
            fake.FailNextOutstanding=true;
            save.PerformClick();
            await SaveUntil(()=>!fake.FailNextOutstanding && save.Enabled,"Outstanding failure did not finish");
            SaveCheck(fake.OutstandingWrites==1 && input.Text=="写入失败时保留" && editor.IsDirty() &&
                feedback.Text.Contains("fixture: outstanding content write failed"),"A failed content write discarded the draft");
            save.PerformClick();
            await SaveUntil(()=>fake.OutstandingWrites==2 && save.Enabled,"Outstanding retry did not finish");
            // The browser can add this caret placeholder when the cleared editor regains focus.
            SaveCheck(note.SimulateUserHtmlForTest("<br>"),"Could not insert the browser's blank placeholder");
            await Task.Delay(1100);
            SaveCheck(ReadShared(fake.Notes(id)).Items.Count==2 && !editor.IsDirty() && !HasDraftCache("outstanding",id,"new") &&
                feedback.Text.Contains("已正式保存"),"Retry or subsequent autosave recreated a saved draft: draft="+HasDraftCache("outstanding",id,"new")+", input="+json.Serialize(input.Text)+", note="+json.Serialize(note.Html)+", feedback="+feedback.Text);
            SaveCheck(!unsavedUpdateEditors.First().IsDirty(),"Child save incorrectly marked the parent progress window dirty");

            child.Close();
            await SaveUntil(()=>child.IsDisposed,"Closing a saved outstanding window asked about a draft");
            SaveCheck(notePopup.IsDisposed,"Closing the outstanding original left its detached note window alive");
            await SaveUntil(()=>unsavedUpdateEditors.Count==1,"Outstanding window did not release its editor registration");
            WriteDraftCache("outstanding",id,"new",new OutstandingDraftCache{
                DraftId=stored.Id,Text=OutstandingEditorText(stored.Html),NoteHtml=stored.NoteHtml,Priority=stored.Priority
            });
            SaveControls(parent.Dialog).OfType<Button>().Single(button=>button.Text=="遗留事项 · 所有日期共享").PerformClick();
            await SaveUntil(()=>(child=findOutstanding())!=null,"Task window did not reopen outstanding editor");
            SaveCheck(!unsavedUpdateEditors.Last().IsDirty() && !HasDraftCache("outstanding",id,"new"),
                "An older draft for an already saved item was restored as unsaved content");
            child.Close();
            await SaveUntil(()=>child.IsDisposed && unsavedUpdateEditors.Count==1,"Reopened clean editor did not close");

            ShowOutstandingItem(id,stored.Id);
            await SaveUntil(()=>(child=findOutstanding())!=null,"Outstanding row did not open the editor");
            var directSave=SaveControls(child).OfType<Button>().Single(button=>button.Text=="保存修改");
            var directButtons=SaveControls(child).OfType<Button>().Where(button=>button.Parent==directSave.Parent && button.Visible)
                .Select(button=>button==directSave?"save":button.Text).ToArray();
            SaveCheck(taskButtons.SequenceEqual(directButtons) && !unsavedUpdateEditors.Last().IsDirty(),
                "The task and outstanding-row entry points disagree on actions or reload saved content as dirty");
            child.Close();
            await SaveUntil(()=>child.IsDisposed,"Closing a reopened saved outstanding item asked about a draft");
            Console.WriteLine("PASS nested outstanding save, genuine failure and retry, clean close/reopen, and identical actions from both entry points.");
        } catch(Exception error){failure=error;}
        if(child!=null && !child.IsDisposed){unsavedUpdateEditors.Last().Discard();child.Close();}
        await CloseSaveUi(parent);
        if(failure!=null)throw failure;
    }
    async Task TestOutstandingUploadFailure(ProgressSaveHttp fake) {
        const long id=49;
        var lifetime=EditOutstanding(id,false,false);
        Form dialog=null;Exception failure=null;
        try {
            await SaveUntil(()=> {dialog=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="遗留事项 · 所有日期共享");return dialog!=null;},
                "New outstanding editor did not open");
            var input=SaveControls(dialog).OfType<TextBox>().Single(box=>box.AccessibleName=="遗留事项内容");
            var editor=SaveControls(dialog).OfType<ProgressHtmlEditor>().Single();
            var save=SaveControls(dialog).OfType<Button>().Single(button=>button.Text=="添加一条");
            var feedback=dialog.Controls[0].Controls.OfType<Label>().Last();
            var browser=(WebBrowser)editor.Controls[0];
            await SaveUntil(()=>browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"New outstanding note editor not ready");
            byte[] picture;
            using(var bitmap=new Bitmap(8,8))using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);picture=stream.ToArray();}
            input.Text="上传失败也不能丢失的新遗留事项";
            editor.SimulateUserHtmlForTest("<p>新备注</p><img src=\"data:image/png;base64,"+Convert.ToBase64String(picture)+"\">");
            fake.FailNextAttachment=true;
            save.PerformClick();
            await SaveUntil(()=>!fake.FailNextAttachment && save.Enabled,"Failed upload did not finish");
            SaveCheck(feedback.Text.Contains(ProgressSaveHttp.AttachmentFailure) && input.Text.Contains("不能丢失") &&
                editor.ImageCount==1 && unsavedUpdateEditors.Last().IsDirty() && ReadShared(fake.Notes(id)).Items.Count==0,
                "Failed upload hid its actual cause or lost the draft: "+feedback.Text);
            save.PerformClick();
            await SaveUntil(()=>save.Enabled && ReadShared(fake.Notes(id)).Items.Count==1,"Upload retry did not save the retained draft: "+feedback.Text);
            SaveCheck(!unsavedUpdateEditors.Last().IsDirty() && ReadShared(fake.Notes(id)).Items.Single().NoteHtml.Contains("<img"),
                "Upload retry left a dirty editor or lost the saved image");
            var legacyPictures=new List<PastedImage>{new PastedImage{Bytes=picture}};
            fake.FailNextAttachment=true;Exception uploadError=null;
            try{await UploadOutstandingPictures(id,legacyPictures);}catch(Exception error){uploadError=error;}
            SaveCheck(uploadError!=null && uploadError.Message.Contains(ProgressSaveHttp.AttachmentFailure) && legacyPictures[0].Id==0,
                "Legacy outstanding picture upload threw a null reference or lost the retry state");
            SaveCheck((await UploadOutstandingPictures(id,legacyPictures)).Contains("/attachments/") && legacyPictures[0].Id>0,
                "Legacy outstanding picture upload did not retry");
            var progressPictures=new List<PastedImage>{new PastedImage{Bytes=picture}};
            fake.FailNextAttachment=true;uploadError=null;
            try{await SaveProgress(50,DateTime.Today,"旧版进展图片入口","",progressPictures);}catch(Exception error){uploadError=error;}
            SaveCheck(uploadError!=null && uploadError.Message.Contains(ProgressSaveHttp.AttachmentFailure) && fake.Notes(50).Count==0 &&
                progressPictures[0].Id==0,"Legacy daily progress upload threw a null reference or saved an incomplete record");
            await SaveProgress(50,DateTime.Today,"旧版进展图片入口","",progressPictures);
            SaveCheck(fake.Notes(50).Count==1 && progressPictures[0].Id>0,"Legacy daily progress upload retry failed");
            Console.WriteLine("PASS new outstanding image upload failure shows its cause, retains input/images and retries; legacy body images share the same behavior.");
        } catch(Exception error){failure=error;}
        if(dialog!=null && !dialog.IsDisposed){unsavedUpdateEditors.Last().Discard();dialog.Close();}
        await SaveUntil(()=>lifetime.IsCompleted,"New outstanding note editor did not close");await lifetime;
        if(failure!=null)throw failure;
    }
    async Task TestWebOutstandingNotes(ProgressSaveHttp fake) {
        long taskId=48;
        string prefix="<h3 data-tasktrace-comment-type=\"outstanding\">TaskTrace 遗留事项清单</h3><ul><li data-id=\"web-note\" data-priority=\"2\"><p>网页遗留事项</p><aside data-tasktrace-outstanding-note=\"true\" hidden>";
        string suffix="</aside></li></ul>";
        using(var bitmap=new Bitmap(8,8))
        using(var stream=new MemoryStream()) {
            bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);
            fake.SeedAttachment(9901,stream.ToArray());
        }
        fake.Seed(taskId,prefix+"<p>网页添加的备注</p><ul><li><p>嵌套说明</p></li></ul><p><img src=\"/api/v1/tasks/48/attachments/9901\"></p>"+suffix);
        var lifetime=EditOutstanding(taskId,false,false,"web-note");
        Form dialog=null;Exception failure=null;
        try {
            await SaveUntil(()=> {dialog=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="遗留事项 · 所有日期共享");return dialog!=null;},
                "Web-created outstanding note did not open its native editor");
            var editor=SaveControls(dialog).OfType<ProgressHtmlEditor>().Single();
            var browser=(WebBrowser)editor.Controls[0];
            await SaveUntil(()=>browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null &&
                (browser.Document.GetElementById("progress-editor").InnerText??"").Contains("网页添加的备注"),
                "Saved web note was not visible in the actual native editor");
            SaveCheck((browser.Document.GetElementById("progress-editor").InnerText??"").Contains("嵌套说明") && editor.ImageCount==1,
                "Web note lost list text or its embedded attachment image");
            SaveCheck(SaveControls(dialog).OfType<ListBox>().Single().Items.Count==1,"Note bullets became extra outstanding items");
            SaveCheck(!unsavedUpdateEditors.Last().IsDirty(),"Viewing a web-created note incorrectly marked the editor dirty");
            var save=SaveControls(dialog).OfType<Button>().Single(button=>button.Text=="保存修改");
            var feedback=dialog.Controls[0].Controls.OfType<Label>().Last();
            editor.SimulateUserHtmlForTest(editor.Html.Replace("网页添加的备注","悬浮窗修改网页备注"));
            save.PerformClick();
            await SaveUntil(()=>save.Enabled && feedback.Text!="正在保存遗留事项…","Saving the web note did not finish");
            var saved=ReadShared(fake.Notes(taskId)).Items.Single();
            SaveCheck(Plain(saved.NoteHtml).Contains("悬浮窗修改网页备注") && editor.ImageCount==1 &&
                !unsavedUpdateEditors.Last().IsDirty(),"Saving a web-created note failed: "+feedback.Text);
            Console.WriteLine("PASS editing and saving a web-created outstanding note retains images and a clean saved baseline.");
            fake.Seed(taskId,prefix+"<p>网页刚更新的备注</p><p><img src=\"/api/v1/tasks/48/attachments/9901\"></p>"+suffix);
            ShowOutstandingItem(taskId,"web-note");
            await SaveUntil(()=>(browser.Document.GetElementById("progress-editor").InnerText??"").Contains("网页刚更新的备注"),
                "Reopening an existing outstanding window kept stale notes instead of reading the web update",1500);
            SaveCheck(!unsavedUpdateEditors.Last().IsDirty(),"Refreshing web notes produced an unsaved-change prompt");
            fake.Notes(taskId).Last()["comment"]=prefix+suffix;
            ShowOutstandingItem(taskId,"web-note");
            await SaveUntil(()=>Plain(editor.Html)=="","Web note deletion was not refreshed");
            fake.Notes(taskId).Last()["comment"]=prefix+"<p>网页后来补充的备注</p>"+suffix;
            ShowOutstandingItem(taskId,"web-note");
            await SaveUntil(()=>Plain(editor.Html)=="网页后来补充的备注","A web note added after opening an empty native note stayed blank");
            SaveCheck(!unsavedUpdateEditors.Last().IsDirty(),"Web empty-to-filled note refresh created a false unsaved prompt");
            int beforeRefresh=fake.HistoryReads;
            var held=new TaskCompletionSource<bool>();fake.HeldHistory=held;
            ShowOutstandingItem(taskId,"web-note");
            ShowOutstandingItem(taskId,"web-note");
            SaveCheck(fake.HistoryReads==beforeRefresh+1,"Repeated reopening started duplicate in-flight refreshes");
            editor.SimulateUserHtmlForTest("<p>读取网页期间输入的本地备注</p>");
            held.SetResult(true);
            await Task.Delay(150);
            SaveCheck(Plain(editor.Html)=="读取网页期间输入的本地备注","An in-flight refresh overwrote newly typed notes");
            editor.SimulateUserHtmlForTest("<p>悬浮窗尚未保存的备注</p>");
            fake.Seed(taskId,prefix+"<p>网页的另一条修改</p>"+suffix);
            ShowOutstandingItem(taskId,"web-note");
            await Task.Delay(150);
            SaveCheck((browser.Document.GetElementById("progress-editor").InnerText??"").Contains("悬浮窗尚未保存的备注"),
                "External refresh overwrote an unsaved note");
            Console.WriteLine("PASS web-created notes, nested lists and inline images load; reopening refreshes clean windows and protects local drafts.");
        } catch(Exception error){failure=error;}
        if(dialog!=null && !dialog.IsDisposed){unsavedUpdateEditors.Last().Discard();dialog.Close();}
        await SaveUntil(()=>lifetime.IsCompleted,"Web note editor did not close");
        await lifetime;
        if(failure!=null)throw failure;
    }
    internal static int RunProgressSaveFixture(string directory,bool baseline) {
        int code=0;Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
        using(var fake=new ProgressSaveHttp())
        using(var host=new FloatingWindow(directory,fake)) {
            host.Shown+=async delegate {
                try {
                    await host.TestProgressSaveButtons(fake,baseline);
                    if(!baseline)await host.TestOutstandingFromTaskWindow(fake);
                    if(!baseline)await host.TestProgressPopout(fake);
                    if(!baseline)await host.TestOutstandingUploadFailure(fake);
                    if(!baseline)await host.TestWebOutstandingNotes(fake);
                }
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
$popoutSource=Join-Path $copiedSources 'FloatingEditorPopout.cs'
$popoutText=[IO.File]::ReadAllText($popoutSource)
if(!$popoutText.Contains('        window.Show(owner);')){throw 'Hidden popout instrumentation could not find modeless Show.'}
$popoutText=$popoutText.Replace('        window.Show(owner);','        window.Opacity=0;window.ShowInTaskbar=false;window.Show(owner);')
[IO.File]::WriteAllText($popoutSource,$popoutText,[Text.UTF8Encoding]::new($true))
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
