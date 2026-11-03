param(
    [string]$OutputDirectory = '.local-build/floating-progress-dirty-tests',
    [switch]$Baseline
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Exercise the production rich editor in an owned, invisible native form.
# DOM edits intentionally do not dispatch input events: native input, IME and delete
# notifications can be missed, but save/close must still read the actual content.
# No real TaskTrace startup, API, session, database, clipboard or account is used.
$source = @"
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Reflection;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    internal static bool SameDirtyTestContent(string a,string b) {return SameProgressEditorContent(a,b);}
}

internal static class FloatingProgressDirtyTests {
    static void Check(bool condition,string message) {if(!condition)throw new Exception(message);}
    static object Eval(WebBrowser browser,string script) {return browser.Document.InvokeScript("eval",new object[]{script});}
    static Timer ChangeTimer(ProgressHtmlEditor editor) {return (Timer)typeof(ProgressHtmlEditor).GetField("changeTimer",BindingFlags.Instance|BindingFlags.NonPublic).GetValue(editor);}
    static void UnreportedEdit(WebBrowser browser,string html) {
        string encoded=new System.Web.Script.Serialization.JavaScriptSerializer().Serialize(html);
        Eval(browser,"document.getElementById('progress-editor').innerHTML="+encoded);
    }
    static string ImageSource() {
        using(var bitmap=new Bitmap(8,6))using(var stream=new MemoryStream()) {
            using(var graphics=Graphics.FromImage(bitmap))graphics.Clear(Color.CornflowerBlue);
            bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);
            return "data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());
        }
    }
    static async Task Ready(ProgressHtmlEditor editor,WebBrowser browser) {
        for(int attempt=0;attempt<160;attempt++) {
            bool ready=(bool)typeof(ProgressHtmlEditor).GetField("ready",BindingFlags.Instance|BindingFlags.NonPublic).GetValue(editor);
            if(ready && browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null)return;
            await Task.Delay(25);
        }
        throw new Exception("Rich editor did not become ready within four seconds");
    }
    static async Task Run(string name,Func<Task> test,List<string> failures) {
        try {await test();Console.WriteLine("PASS "+name);}
        catch(Exception error) {failures.Add(name+": "+error.Message);Console.WriteLine("FAIL "+name+": "+error.Message);}
    }
    static async Task Cases(Form host) {
        var failures=new List<string>();
        using(var editor=new ProgressHtmlEditor(()=>"",delegate(string html,string text){return true;}){Dock=DockStyle.Fill}) {
            int events=0;editor.HtmlChanged+=delegate{events++;};
            editor.Html="<p>最初已保存内容</p>";
            int beforeReady=editor.ChangeVersion;
            host.Controls.Add(editor);var browser=(WebBrowser)editor.Controls[0];
            await Ready(editor,browser);
            await Run("initial document load, programmatic HTML and read-only selection remain clean",async delegate {
                await Task.Delay(80);
                Check(editor.ChangeVersion==beforeReady && events==0,"Initial HTML normalization was reported as an edit");
                int version=editor.ChangeVersion;
                editor.Html="<P>加载下一天&nbsp;内容</P>";
                string saved=editor.Html;
                await Task.Delay(60);
                Check(editor.ChangeVersion==version && events==0,"Programmatic date load changed the saved version");
                editor.SetReadOnly(true);
                Eval(browser,"(function(){var e=document.getElementById('progress-editor'),r=document.createRange(),s=window.getSelection();r.selectNodeContents(e);s.removeAllRanges();s.addRange(r);})()");
                await Task.Delay(50);
                Check(editor.ChangeVersion==version && editor.Html==saved && events==0,"Read-only selection created unsaved content");
                editor.SetReadOnly(false);
            },failures);
            await Run("save read detects text edit without input notification or scheduled timer",delegate {
                editor.Html="<p>已保存的日报</p>";string original=editor.Html;
                int savedVersion=editor.ChangeVersion,eventCount=events;
                UnreportedEdit(browser,"<p>已填写但输入事件漏报的日报</p>");
                Check(!ChangeTimer(editor).Enabled,"Fixture unexpectedly scheduled an input event");
                Check(editor.Html.Contains("输入事件漏报"),"The DOM edit did not reach the real rich editor");
                int edited=editor.ChangeVersion;
                Check(edited>savedVersion,"Save would report no changes: actual text changed but ChangeVersion stayed at its saved baseline");
                Check(!FloatingWindow.SameDirtyTestContent(editor.Html,original),"Changed text was considered the saved content");
                Check(events==eventCount+1,"Actual content should notify once when it is detected");
                Check(editor.ChangeVersion==edited && events==eventCount+1,"Reading again duplicated the content-change event");
                return Task.FromResult(0);
            },failures);
            await Run("deletion without input notification is included in save",delegate {
                editor.Html="<p>删除前一行</p><p>删除后一行</p>";string original=editor.Html;int savedVersion=editor.ChangeVersion;
                UnreportedEdit(browser,"<p>删除前一行</p>");
                Check(!ChangeTimer(editor).Enabled,"Deletion fixture unexpectedly scheduled a change");
                Check(editor.ChangeVersion>savedVersion,"Deleting text was missed by save");
                Check(!FloatingWindow.SameDirtyTestContent(editor.Html,original),"Deletion was considered unchanged");
                editor.Html="<p>清空前</p>";savedVersion=editor.ChangeVersion;
                UnreportedEdit(browser,"");
                Check(editor.ChangeVersion>savedVersion && editor.Html=="","Clearing an existing record was missed by save");
                return Task.FromResult(0);
            },failures);
            await Run("mixed text and image DOM edit is detected immediately before save",delegate {
                editor.Html="<p>原文</p>";string original=editor.Html;int savedVersion=editor.ChangeVersion;
                UnreportedEdit(browser,"<p>粘贴前文<img src='"+ImageSource()+"'>粘贴后文</p>");
                Check(!ChangeTimer(editor).Enabled && editor.ImageCount==1,"Mixed-image fixture did not bypass the timer");
                Check(editor.ChangeVersion>savedVersion,"Mixed text/image edit was missed by immediate save");
                Check(!FloatingWindow.SameDirtyTestContent(editor.Html,original),"A new embedded image was considered unchanged");
                return Task.FromResult(0);
            },failures);
            await Run("latest unsignalled edit after an earlier change is not skipped",delegate {
                editor.Html="<p>已保存</p>";int baseline=editor.ChangeVersion;
                UnreportedEdit(browser,"<p>第一次修改</p>");int first=editor.ChangeVersion;
                UnreportedEdit(browser,"<p>第一次修改，最后输入的内容也要保存</p>");
                Check(!ChangeTimer(editor).Enabled,"Latest-edit fixture scheduled a timer");
                int last=editor.ChangeVersion;
                Check(first>baseline && last>first && editor.Html.Contains("最后输入"),"The last edit before save retained an older change version");
                return Task.FromResult(0);
            },failures);
            await Run("returning to saved content clears semantic dirty state without discarding the change history",delegate {
                editor.Html="<p>基线</p>";string original=editor.Html;int savedVersion=editor.ChangeVersion;
                UnreportedEdit(browser,"<p>临时修改</p>");int edited=editor.ChangeVersion;
                UnreportedEdit(browser,original);int restored=editor.ChangeVersion;
                Check(edited>savedVersion && restored>edited,"Edit/revert was not tracked");
                Check(FloatingWindow.SameDirtyTestContent(editor.Html,original),"Restored saved content would still prompt to save");
                int eventCount=events;
                editor.Html="<p>新日期程序载入</p>";int loaded=editor.ChangeVersion;
                Check(loaded==restored && events==eventCount,"A programmatic reload was mistaken for another user edit");
                return Task.FromResult(0);
            },failures);
            await Run("content change notification permits a reentrant version read",delegate {
                editor.Html="<p>重入前</p>";int savedVersion=editor.ChangeVersion,handlerCalls=0;
                EventHandler handler=delegate{handlerCalls++;Check(editor.ChangeVersion>savedVersion,"Reentrant handler saw the old version");string html=editor.Html;Check(html.Contains("重入后"),"Reentrant handler saw stale HTML");};
                editor.HtmlChanged+=handler;
                try {
                    UnreportedEdit(browser,"<p>重入后</p>");
                    Check(editor.ChangeVersion>savedVersion && handlerCalls==1,"Reentrant event caused duplicate notifications or missed the edit");
                }finally{editor.HtmlChanged-=handler;}
                return Task.FromResult(0);
            },failures);
            host.Controls.Remove(editor);
        }
        if(failures.Count>0)throw new Exception(String.Join(Environment.NewLine,failures.ToArray()));
    }
    [STAThread] static int Main() {
        Console.OutputEncoding=Encoding.UTF8;Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
        Exception failure=null;
        using(var host=new Form{Opacity=0,ShowInTaskbar=false,Width=620,Height=360,Text="Progress dirty regression fixture"}) {
            host.Shown+=async delegate{try{await Cases(host);}catch(Exception error){failure=error;}finally{host.Close();}};
            Application.Run(host);
        }
        if(failure!=null){Console.Error.WriteLine(failure);return 1;}return 0;
    }
}
"@
$harness=Join-Path $testRoot 'FloatingProgressDirtyTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingProgressDirtyTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
if($Baseline) {
    $previous=@(& git -C $repoRoot show HEAD:portable/FloatingProgressEditor.cs)
    if($LASTEXITCODE -ne 0){throw 'Could not read the committed rich editor source.'}
    $baselineSource=Join-Path $testRoot 'FloatingProgressEditor.baseline.cs'
    [IO.File]::WriteAllText($baselineSource,($previous -join [Environment]::NewLine),[Text.UTF8Encoding]::new($true))
    $sources=@($sources | Where-Object { [IO.Path]::GetFileName($_) -ne 'FloatingProgressEditor.cs' })+@($baselineSource)
    Write-Output ('Source mode: committed baseline '+(& git -C $repoRoot rev-parse HEAD))
} else { Write-Output 'Source mode: current working tree' }
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingProgressDirtyTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Progress dirty regression compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(45000)){$process.Kill();throw ('Progress dirty tests timed out: '+$testRoot)}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw ('Progress dirty tests failed: '+$testRoot)}
if($process.ExitCode){throw ('Progress dirty tests failed: '+$process.ExitCode+'; '+$testRoot)}
Write-Output ('Results: '+$testRoot)
