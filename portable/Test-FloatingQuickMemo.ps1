param([string]$OutputDirectory = '.local-build/floating-quick-memo-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Quick-memo test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# The private owner constructor bypasses startup, sessions, tray, network and watchers.
# Clipboard and attachment downloads are injected. Every persisted memo and screenshot is isolated here.
$source = @'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    FloatingWindow(string isolatedDirectory) {
        root=data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=true;
        Text="TaskTrace isolated memo owner";Font=SystemFonts.MessageBoxFont;Icon=SystemIcons.Application;
        Opacity=0;TopMost=false;ShowInTaskbar=false;StartPosition=FormStartPosition.Manual;
        Bounds=new Rectangle(-20000,-20000,420,580);
    }
    protected override bool ShowWithoutActivation {get{return true;}}
    protected override CreateParams CreateParams {get{var value=base.CreateParams;value.ExStyle|=0x08000000;return value;}}
    static void MemoAssert(bool value,string message){if(!value)throw new Exception(message);}
    static object MemoEval(WebBrowser browser,string code){return browser.Document.InvokeScript("eval",new object[]{code});}
    static async Task MemoUntil(Func<bool> condition,string message,int milliseconds=4000) {
        int elapsed=0;while(!condition() && elapsed<milliseconds){await Task.Delay(25);elapsed+=25;}
        MemoAssert(condition(),message);
    }
    static async Task MemoReady(FloatingQuickMemoEditor memo) {
        var browser=(WebBrowser)memo.Editor.Controls[0];
        await MemoUntil(()=>browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Memo editor did not initialize");
        ((ProgressHtmlEditorBridge)browser.ObjectForScripting).WriteClipboardHtml=delegate(string html,string text){return true;};await Task.Delay(40);
    }
    static async Task MemoClose(FloatingQuickMemoEditor memo) {
        memo.Close();await MemoUntil(()=>memo.IsDisposed,"Memo did not close after saving");
    }
    static void MemoCaretEnd(ProgressHtmlEditor editor) {
        MemoEval((WebBrowser)editor.Controls[0],"(function(){var e=document.getElementById('progress-editor'),r=document.createRange(),s=window.getSelection();taskTraceSelectedImage=null;r.selectNodeContents(e);r.collapse(false);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();})()");
    }
    static void MemoPaste(ProgressHtmlEditor editor){MemoEval((WebBrowser)editor.Controls[0],"pasteTaskTrace({preventDefault:function(){}})");}
    static byte[] MemoPicture(Color color) {
        using(var bitmap=new Bitmap(160,72))using(var stream=new MemoryStream()) {
            using(var graphics=Graphics.FromImage(bitmap)){graphics.Clear(color);graphics.DrawString("TaskTrace memo",SystemFonts.MessageBoxFont,Brushes.Black,8,24);}
            bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return stream.ToArray();
        }
    }
    static string MemoImage(Color color){return "data:image/png;base64,"+Convert.ToBase64String(MemoPicture(color));}
    static string MemoDirectory(string root,string name){var directory=Path.Combine(root,name);Directory.CreateDirectory(directory);return directory;}
    FloatingQuickMemoEditor NewMemo(FloatingQuickMemoStore store,ProgressHtmlEditor editor) {
        return new FloatingQuickMemoEditor(store,Font,Icon,false,editor){Opacity=0,StartPosition=FormStartPosition.Manual,Location=new Point(-20000,-20000)};
    }
    static void MemoScreenshot(FloatingQuickMemoEditor memo,string path) {
        // WebBrowser does not reliably implement WM_PRINT; this captures only native layout.
        // Mixed-content correctness is verified through the real DOM and reopened storage below.
        memo.PerformLayout();
        using(var bitmap=new Bitmap(memo.Width,memo.Height)) {
            using(var measured=memo.CreateGraphics())bitmap.SetResolution(measured.DpiX,measured.DpiY);
            memo.DrawToBitmap(bitmap,new Rectangle(Point.Empty,memo.Size));bitmap.Save(path);
        }
    }

    async Task MemoPersistenceCase() {
        string directory=MemoDirectory(data,"persistence"),first=MemoImage(Color.LightBlue),second=MemoImage(Color.LightGreen),payload="<p>图片前的文字<br>保留空行和缩进</p><p><img src='"+first+"'>图片之间<img src='"+second+"'>图片之后</p>";
        var store=new FloatingQuickMemoStore(directory);
        using(var memo=NewMemo(store,new ProgressHtmlEditor(()=>payload))) {
            memo.Show();await MemoReady(memo);MemoCaretEnd(memo.Editor);MemoPaste(memo.Editor);
            await MemoUntil(()=>memo.Editor.ImageCount==2,"Mixed memo paste lost images");
            await MemoUntil(()=>File.Exists(store.FilePath) && !memo.IsDirty,"Debounced memo did not save automatically");
            string saved=store.Read();MemoAssert(saved.Contains("图片前的文字") && Regex.Matches(saved,"<img\\b",RegexOptions.IgnoreCase).Count==2,"Automatic save lost mixed content");
            MemoAssert(!saved.Contains("/api/") && saved.Contains("data:image/png;base64,"),"Saved memo is not self-contained");
            MemoScreenshot(memo,Path.Combine(data,"memo-window-full.png"));
            memo.Size=new Size(460,360);await Task.Delay(60);MemoScreenshot(memo,Path.Combine(data,"memo-window-narrow.png"));
            await MemoClose(memo);
        }
        using(var memo=NewMemo(new FloatingQuickMemoStore(directory),new ProgressHtmlEditor(()=>""))) {
            memo.Show();await MemoReady(memo);MemoAssert(memo.Editor.ImageCount==2 && memo.Editor.Html.Contains("图片之间"),"A new memo instance did not reload both images and text");
            var browser=(ProgressEditorBrowser)memo.Editor.Controls[0];
            MemoEval(browser,"(function(){var image=document.getElementById('progress-editor').getElementsByTagName('img')[1];setTaskTraceImageRange(image);copyTaskTrace(null,true);})()");
            await MemoUntil(()=>memo.Editor.ImageCount==1,"Image deletion did not remove the selected memo image");
            browser.ShortcutForTest(Keys.Control|Keys.Z);await MemoUntil(()=>memo.Editor.ImageCount==2,"Memo undo did not restore a removed image");
            MemoAssert(await memo.SaveAsync(),"Undo state could not be saved");await MemoClose(memo);
        }
        using(var memo=NewMemo(new FloatingQuickMemoStore(directory),new ProgressHtmlEditor(()=>""))) {
            memo.Show();await MemoReady(memo);MemoAssert(memo.Editor.ImageCount==2,"Undo-restored memo image did not survive reopening");await MemoClose(memo);
        }
        Console.WriteLine("PASS mixed text/two-image debounce save; self-contained reopen; image cut/undo; full/narrow native layout captures (ActiveX content verified separately).");
    }

    async Task MemoSaveRaceCase() {
        var store=new FloatingQuickMemoStore(MemoDirectory(data,"save-race"));
        using(var memo=NewMemo(store,new ProgressHtmlEditor(()=>""))) {
            memo.Show();await MemoReady(memo);memo.Editor.SimulateUserHtmlForTest("<p>First in-flight snapshot</p>");
            var gate=typeof(FloatingQuickMemoStore).GetField("writeGate",System.Reflection.BindingFlags.Instance|System.Reflection.BindingFlags.NonPublic).GetValue(store);
            Task<bool> saving;
            // Hold only the isolated store gate until the worker has been queued and another edit exists.
            lock(gate) {
                saving=memo.SaveAsync();MemoAssert(!saving.IsCompleted,"Save race did not hold an in-flight worker");
                memo.Editor.SimulateUserHtmlForTest("<p>Latest while saving<img src='"+MemoImage(Color.Azure)+"'></p>");
            }
            string latest=memo.Editor.Html;MemoAssert(await saving,"In-flight save failed");
            MemoAssert(!memo.IsDirty && store.Read()==latest,"An earlier snapshot marked later edits as saved or failed to persist them");
            await MemoClose(memo);
        }
        Console.WriteLine("PASS deterministic save race: edits made while the storage worker is pending finish in the latest saved revision.");
    }

    async Task MemoCloseFlushCase() {
        var store=new FloatingQuickMemoStore(MemoDirectory(data,"close-flush"));
        using(var memo=NewMemo(store,new ProgressHtmlEditor(()=>""))) {
            memo.Show();await MemoReady(memo);MemoAssert(memo.Editor.SimulateUserHtmlForTest("<p>关闭前最新内容<img src='"+MemoImage(Color.MistyRose)+"'></p>"),"Close fixture editor not ready");
            string current=memo.Editor.Html;MemoAssert(memo.IsDirty,"Immediate close fixture was not dirty");await MemoClose(memo);
            MemoAssert(store.Read()==current,"Closing before debounce elapsed did not flush the latest memo");
        }
        Console.WriteLine("PASS closing flushes the latest text and image before disposing.");
    }

    async Task MemoLockedStoreCase() {
        var store=new FloatingQuickMemoStore(MemoDirectory(data,"locked-store"));string original="<p>原有备忘内容</p>";store.Save(original,store.ReserveWrite());byte[] originalBytes=File.ReadAllBytes(store.FilePath);
        using(var memo=NewMemo(store,new ProgressHtmlEditor(()=>""))) {
            memo.Show();await MemoReady(memo);
            using(var locked=new FileStream(store.FilePath,FileMode.Open,FileAccess.Read,FileShare.None)) {
                MemoAssert(memo.Editor.SimulateUserHtmlForTest("<p>锁定期间的新内容<img src='"+MemoImage(Color.LightYellow)+"'></p>"),"Locked-store fixture did not change");
                MemoAssert(!await memo.SaveAsync(),"Saving to a locked memo file incorrectly succeeded");
                MemoAssert(memo.IsDirty && memo.Feedback.Text.Contains("保存失败"),"Save failure did not preserve dirty content and feedback");
                memo.Close();await Task.Delay(140);
                MemoAssert(!memo.IsDisposed && memo.Visible && memo.IsDirty && memo.SaveButton.Enabled,"Failed close disposed or disabled the memo");
                MemoAssert(!memo.SaveForExit() && !memo.IsDisposed && memo.Feedback.Text.Contains("退出已取消"),"Exit ignored a memo write failure");
            }
            MemoAssert(originalBytes.SequenceEqual(File.ReadAllBytes(store.FilePath)),"A failed save modified the old memo file");
            string latest=memo.Editor.Html;MemoAssert(await memo.SaveAsync(),"Unlocked memo did not retry successfully");
            MemoAssert(store.Read()==latest && !memo.IsDirty,"Retry did not persist the latest draft");
            await MemoClose(memo);
        }
        Console.WriteLine("PASS locked-file save/close/exit failure keeps the window and old bytes; unlocking permits retry.");
    }

    async Task MemoPendingPasteCase() {
        var queue=new Queue<TaskCompletionSource<string>>();var store=new FloatingQuickMemoStore(MemoDirectory(data,"pending-paste"));
        using(var memo=NewMemo(store,new ProgressHtmlEditor(delegate{var completion=new TaskCompletionSource<string>();queue.Enqueue(completion);return completion.Task;},true))) {
            memo.Show();await MemoReady(memo);MemoCaretEnd(memo.Editor);MemoPaste(memo.Editor);
            MemoAssert(queue.Count==1 && memo.Editor.IsPastePending && memo.IsDirty,"Pending paste was not treated as an unsaved draft");
            memo.Close();await Task.Delay(90);MemoAssert(!memo.IsDisposed && memo.Visible && memo.Editor.IsPastePending,"Pending paste was lost on close");
            MemoAssert(!memo.SaveForExit(),"Exit ignored a pending image paste");
            queue.Dequeue().SetResult("<p>异步图文完成<img src='"+MemoImage(Color.Lavender)+"'></p>");
            await MemoUntil(()=>!memo.Editor.IsPastePending && memo.Editor.ImageCount==1,"Pending memo paste did not finish");
            await MemoClose(memo);MemoAssert(store.Read().Contains("异步图文完成") && store.Read().Contains("data:image/"),"Completed pending paste was not retained");
        }
        Console.WriteLine("PASS pending image paste blocks close/exit until completed and then persists.");
    }

    void MemoCorruptReadCase() {
        var store=new FloatingQuickMemoStore(MemoDirectory(data,"corrupt-store"));Directory.CreateDirectory(Path.GetDirectoryName(store.FilePath));
        byte[] corrupt={0x3c,0x70,0x3e,0xc3,0x28};File.WriteAllBytes(store.FilePath,corrupt);bool failed=false;
        using(var editor=new ProgressHtmlEditor(()=>"")) {
            try{using(var memo=NewMemo(store,editor)){}}
            catch(DecoderFallbackException){failed=true;}
        }
        MemoAssert(failed && corrupt.SequenceEqual(File.ReadAllBytes(store.FilePath)),"Corrupt memo read opened a blank draft or overwrote original bytes");
        Console.WriteLine("PASS malformed UTF-8 refuses blank initialization and preserves the unreadable file.");
    }

    async Task MemoClipboardConversionCase() {
        byte[] bytes=MemoPicture(Color.PeachPuff);string inline=MemoImage(Color.LightCyan);int downloads=0;var paths=new List<string>();
        string converted=await QuickMemoClipboardHtml("<p>本地图<img src='"+inline+"' data-tasktrace-src='/api/v1/tasks/4/attachments/8'></p><p>附件<img src='/api/v2/tasks/7/attachments/11'></p>",delegate(string path){downloads++;paths.Add(path);return Task.FromResult(bytes);});
        MemoAssert(downloads==1 && paths[0]=="/api/v2/tasks/7/attachments/11","Memo attachment normalization used the wrong resource");
        MemoAssert(Regex.Matches(converted,"data:image/",RegexOptions.IgnoreCase).Count==2 && !converted.Contains("/api/") && !converted.Contains("data-tasktrace-src"),"Memo conversion retained non-portable image references");
        var store=new FloatingQuickMemoStore(MemoDirectory(data,"conversion-failure"));string original="<p>保持原文</p>";store.Save(original,store.ReserveWrite());
        using(var memo=NewMemo(store,new ProgressHtmlEditor(async delegate{return await QuickMemoClipboardHtml("<img src='/api/v1/tasks/7/attachments/11'>",delegate(string path){throw new IOException("isolated attachment unavailable");});},true))) {
            memo.Show();await MemoReady(memo);MemoCaretEnd(memo.Editor);MemoPaste(memo.Editor);
            await MemoUntil(()=>!memo.Editor.IsPastePending && memo.Feedback.Text.Contains("isolated attachment unavailable"),"Failed memo attachment did not show clipboard failure");
            MemoAssert(memo.Editor.ImageCount==0 && memo.Editor.Html.Contains("保持原文") && store.Read()==original,"Failed attachment conversion pasted partial content or destroyed the memo");
            await MemoClose(memo);
        }
        Console.WriteLine("PASS attachment-to-data conversion preserves local images; download failure retains the original memo.");
    }

    async Task MemoTraySingletonCase() {
        using(var menu=new ContextMenuStrip()) {
            InitializeQuickMemo(menu);InitializeUpdates();updateTimer.Start();MemoAssert(menu.Items.Cast<ToolStripItem>().Count(item=>item.Text=="快捷备忘")==1,"Tray quick memo entry is missing or duplicated");
            var opened=OpenQuickMemo();await MemoUntil(()=>quickMemo!=null,"Tray did not open memo");var first=quickMemo;first.Opacity=0;first.Location=new Point(-20000,-20000);
            await MemoReady(first);await OpenQuickMemo();MemoAssert(quickMemo==first && !opened.IsCompleted,"Repeated tray open created another memo or ended the existing window");
            first.WindowState=FormWindowState.Minimized;await OpenQuickMemo();
            MemoAssert(first.WindowState==FormWindowState.Normal && first.ShowInTaskbar && first.Icon!=null && first.Owner==null && !first.Modal && Enabled,"Tray reopening did not restore an independent memo");
            first.Editor.SimulateUserHtmlForTest("<p>托盘备忘<img src='"+MemoImage(Color.Honeydew)+"'></p>");MemoAssert(await first.SaveAsync(),"Tray memo did not save");
            string preview=Path.Combine(data,"floating-image-preview-test.png");int doubleClicks=first.Editor.ImageDoubleClickCountForTest;
            MemoAssert(first.Editor.SimulateFirstImageDoubleClickForTest(),"Memo image did not expose double-click");
            await MemoUntil(()=>first.Editor.ImageDoubleClickCountForTest>doubleClicks && File.Exists(preview),"Memo double-click was not connected to the real enlarged preview");
            await MemoClose(first);await opened;MemoAssert(quickMemo==null && unsavedUpdateEditors.Count==0,"Closed memo leaked singleton/update registrations");
            var reopened=OpenQuickMemo();await MemoUntil(()=>quickMemo!=null,"Tray did not reopen saved memo");var second=quickMemo;second.Opacity=0;second.Location=new Point(-20000,-20000);await MemoReady(second);
            MemoAssert(second!=first && second.Editor.ImageCount==1 && second.Editor.Html.Contains("托盘备忘"),"A fresh tray window did not retain saved memo contents");
            var store=new FloatingQuickMemoStore(data);using(var locked=new FileStream(store.FilePath,FileMode.Open,FileAccess.Read,FileShare.None)) {
                second.Editor.SimulateUserHtmlForTest("<p>退出保护中的备忘</p>");allowExit=true;
                var args=new FormClosingEventArgs(CloseReason.UserClosing,false);OnFormClosing(args);
                MemoAssert(args.Cancel && !allowExit && updateTimer.Enabled && !second.IsDisposed && second.IsDirty,"Cancelled memo exit did not retain normal window/update behavior");
            }
            MemoAssert(await second.SaveAsync(),"Memo could not save after protected exit failed");await MemoClose(second);await reopened;updateTimer.Stop();
        }
        Console.WriteLine("PASS tray singleton and minimize recovery; genuine image preview; persistent reopen; unsaved registration and main-exit protection.");
    }

    internal static async Task TestQuickMemo(FloatingWindow window) {
        await window.MemoPersistenceCase();await window.MemoSaveRaceCase();await window.MemoCloseFlushCase();await window.MemoLockedStoreCase();await window.MemoPendingPasteCase();
        window.MemoCorruptReadCase();await window.MemoClipboardConversionCase();await window.MemoTraySingletonCase();
        Console.WriteLine("PASS isolation: only dedicated test data, fake clipboard/downloads, native controls and local screenshots; no actual application startup or install.");
    }
    internal static FloatingWindow CreateMemoTestWindow(string directory){return new FloatingWindow(directory);}
}
internal static class FloatingQuickMemoTests {
    [STAThread] static int Main(string[] args) {
        Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);int exitCode=0;
        using(var window=FloatingWindow.CreateMemoTestWindow(args[0])) {
            window.Shown+=async delegate{try{await FloatingWindow.TestQuickMemo(window);}catch(Exception error){Console.Error.WriteLine(error);exitCode=1;}finally{window.Close();}};
            Application.Run(window);
        }
        return exitCode;
    }
}
'@
$harness = Join-Path $testRoot 'FloatingQuickMemoTests.cs'
[IO.File]::WriteAllText($harness, $source, [Text.UTF8Encoding]::new($false))
$compiler = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary = Join-Path $testRoot 'FloatingQuickMemoTests.exe'
$sources = @(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingQuickMemoTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/win32manifest:' + (Join-Path $PSScriptRoot 'FloatingWindow.manifest')) ('/out:' + $binary) @sources $harness
if ($LASTEXITCODE -ne 0) { throw 'Quick-memo regression compilation failed.' }
$stdout = Join-Path $testRoot 'results.txt';$stderr = Join-Path $testRoot 'errors.txt'
$process = Start-Process -FilePath $binary -ArgumentList ('"' + $testRoot + '"') -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if (!$process.WaitForExit(60000)) { $process.Kill(); throw ('Quick-memo tests timed out: ' + $testRoot) }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if ((Get-Item -LiteralPath $stderr).Length -gt 0) { Get-Content -LiteralPath $stderr -Encoding UTF8; throw ('Quick-memo tests failed: ' + $testRoot) }
if ($process.ExitCode) { throw ('Quick-memo tests failed: ' + $process.ExitCode + '; ' + $testRoot) }
Write-Output ('Results and screenshots: ' + $testRoot)
