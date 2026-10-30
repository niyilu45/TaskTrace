param([string]$OutputDirectory='.local-build/floating-paste-undo-tests')
$ErrorActionPreference='Stop'
$repoRoot=Split-Path $PSScriptRoot -Parent
$testRoot=[IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot=[IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\')+'\'
if(!$testRoot.StartsWith($expectedRoot,[StringComparison]::OrdinalIgnoreCase)){throw 'Test output must remain inside .local-build.'}
$testRoot=Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# The clipboard reader/writer are injected; neither the installed app nor the system clipboard is used.
$source=@"
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
static class FloatingPasteUndoTests {
    static void Require(bool ok,string reason){if(!ok)throw new Exception(reason);}
    static object Eval(WebBrowser browser,string code){return browser.Document.InvokeScript("eval",new object[]{code});}
    static string Picture(Color color){using(var bitmap=new Bitmap(32,24))using(var stream=new MemoryStream()){using(var graphics=Graphics.FromImage(bitmap))graphics.Clear(color);bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return "data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());}}
    static int Scroll(WebBrowser browser){return Convert.ToInt32(Eval(browser,"Math.round(window.pageYOffset||document.documentElement.scrollTop||document.body.scrollTop||0)"));}
    static void Caret(WebBrowser browser,bool end){Eval(browser,"(function(){taskTraceSelectedImage=null;var e=document.getElementById('progress-editor'),r=document.createRange(),s=window.getSelection();r.selectNodeContents(e);r.collapse("+(end?"false":"true")+");s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();})()");}
    static void Paste(WebBrowser browser){Eval(browser,"pasteTaskTrace({preventDefault:function(){}})");}
    static async Task Shortcut(ProgressEditorBrowser browser,Keys key){browser.ShortcutForTest(key);await Task.Delay(60);}
    static void Typing(WebBrowser browser,string text){
        string escaped=new System.Web.Script.Serialization.JavaScriptSerializer().Serialize(text);
        Eval(browser,"(function(){var e=document.getElementById('progress-editor');function signal(kind,key){var x=document.createEvent('Event');x.initEvent(kind,true,true);Object.defineProperty(x,'keyCode',{value:key});e.dispatchEvent(x);}signal('keydown',65);var r=taskTraceRange();r.deleteContents();var t=document.createTextNode("+escaped+");r.insertNode(t);r.setStartAfter(t);r.collapse(true);var s=window.getSelection();s.removeAllRanges();s.addRange(r);signal('input',65);signal('keyup',65);})()");
    }
    static async Task Ready(WebBrowser browser){for(int i=0;i<100&&(browser.Document==null||browser.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(25);Require(browser.Document!=null&&browser.Document.GetElementById("progress-editor")!=null,"Editor not ready");await Task.Delay(30);}
    static async Task Run(string name,Func<Task> action,List<string> failures){try{await action();Console.WriteLine("PASS "+name);}catch(Exception error){failures.Add(name+": "+error.Message);Console.WriteLine("FAIL "+name+": "+error.Message);}}
    static async Task Cases(Form host){
        var failures=new List<string>();var queue=new Queue<TaskCompletionSource<string>>();string first=Picture(Color.Red),second=Picture(Color.Blue);
        using(var editor=new ProgressHtmlEditor(delegate{var completion=new TaskCompletionSource<string>();queue.Enqueue(completion);return completion.Task;},true){Dock=DockStyle.Fill}){
            host.Controls.Add(editor);var browser=(ProgressEditorBrowser)editor.Controls[0];await Ready(browser);
            ((ProgressHtmlEditorBridge)browser.ObjectForScripting).WriteClipboardHtml=delegate(string html,string text){return true;};
            await Run("mixed text/two images paste is one undo/redo and retains original text",async delegate{
                editor.Html="<p>原文左边右边</p>";string original=editor.Html;Eval(browser,"var t=document.getElementById('progress-editor').firstChild.firstChild,r=document.createRange(),s=window.getSelection();r.setStart(t,4);r.collapse(true);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange()");Paste(browser);
                Require(queue.Count==1&&editor.IsPastePending,"Async paste did not wait for data");queue.Dequeue().SetResult("<span>图文前<img src='"+first+"'>图文中<img src='"+second+"'>图文后</span>");await Task.Delay(80);
                string pasted=editor.Html;Require(editor.ImageCount==2&&pasted.IndexOf("原文左边")<pasted.IndexOf("图文前")&&pasted.IndexOf("图文后")<pasted.IndexOf("右边"),"Mixed paste lost position or content");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==original&&editor.ImageCount==0,"Ctrl+Z did not remove the complete mixed paste while keeping original content");
                await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.Html==pasted&&editor.ImageCount==2,"Ctrl+Y did not restore the complete mixed paste");
            },failures);
            await Run("undo cancels a pending async paste so its late result cannot appear",async delegate{
                editor.Html="<p>等待前原文</p>";string original=editor.Html;Caret(browser,true);Paste(browser);var pending=queue.Dequeue();Require(editor.IsPastePending,"Pending fixture not pending");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(!editor.IsPastePending,"Ctrl+Z did not cancel pending paste/save state");pending.SetResult("<span>不应迟到<img src='"+first+"'></span>");await Task.Delay(80);
                Require(editor.Html==original&&editor.ImageCount==0,"Cancelled paste inserted after Ctrl+Z");
            },failures);
            await Run("concurrent async paste order and one transaction per paste",async delegate{
                editor.Html="<p>起点</p>";string original=editor.Html;Caret(browser,true);Paste(browser);Paste(browser);var earlier=queue.Dequeue();var later=queue.Dequeue();later.SetResult("<span>第二次<img src='"+second+"'></span>");await Task.Delay(40);Require(editor.ImageCount==0,"Later paste overtook earlier paste");earlier.SetResult("<span>第一次<img src='"+first+"'></span>");await Task.Delay(90);
                string combined=editor.Html;Require(editor.ImageCount==2&&combined.IndexOf("第一次")<combined.IndexOf("第二次"),"Concurrent paste order/content changed");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.ImageCount==1&&editor.Html.Contains("第一次")&&!editor.Html.Contains("第二次"),"First undo did not remove only second paste");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==original,"Second undo did not restore original");
                await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.ImageCount==1&&editor.Html.Contains("第一次"),"First redo did not restore only first paste");
                await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.Html==combined,"Second redo did not restore ordered combined paste");
            },failures);
            await Run("typing, mixed paste and cut undo in user action order",async delegate{
                editor.Html="<p>初始</p>";string initial=editor.Html;Caret(browser,true);Typing(browser,"键入前");await Task.Delay(40);string typedBefore=editor.Html;
                Paste(browser);queue.Dequeue().SetResult("<span>粘贴中<img src='"+first+"'></span>");await Task.Delay(80);string pasted=editor.Html;Caret(browser,true);Typing(browser,"键入后");await Task.Delay(40);string typedAfter=editor.Html;
                Eval(browser,"(function(){taskTraceSelectedImage=null;var e=document.getElementById('progress-editor'),r=document.createRange(),s=window.getSelection();r.selectNode(e.lastChild);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange()})()");await Shortcut(browser,Keys.Control|Keys.X);string cut=editor.Html;Require(!cut.Contains("键入后")&&cut.Contains("粘贴中"),"Cut fixture failed");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==typedAfter,"Undo cut did not restore latest typing");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==pasted,"Undo typing did not preserve preceding paste");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==typedBefore,"Undo paste did not preserve preceding typing");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==initial,"Undo initial typing did not return to original");
                for(int i=0;i<4;i++)await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.Html==cut,"Redo sequence did not restore final cut state");
            },failures);
            await Run("empty, filtered and failed paste preserve selection/content and add no undo",async delegate{
                int errors=0;editor.ClipboardFailed+=delegate(object sender,ProgressClipboardErrorEventArgs error){errors++;};
                foreach(string kind in new[]{"empty","filtered","failed"}){
                    editor.Html="<p>原始内容</p>";string original=editor.Html;Caret(browser,true);Paste(browser);queue.Dequeue().SetResult("<span>已粘贴图文<img src='"+first+"'>结束</span>");await Task.Delay(80);string pasted=editor.Html;int version=editor.ChangeVersion;
                    Eval(browser,"selectAllTaskTrace()");Paste(browser);var completion=queue.Dequeue();
                    if(kind=="failed")completion.SetException(new IOException("expected clipboard conversion failure"));
                    else completion.SetResult(kind=="filtered"?"<script>alert('removed')</script><style>p{color:white}</style>":"");
                    await Task.Delay(80);Require(editor.Html==pasted&&editor.ImageCount==1,"No-content paste modified the selected content: "+kind);Require(!editor.IsPastePending&&editor.ChangeVersion==version,"No-content paste left pending state or dirtied the draft: "+kind);
                    await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==original,"No-content paste added an undo transaction instead of preserving previous paste history: "+kind);
                }
                Require(errors==1,"Failed conversion did not emit exactly one error");
            },failures);
            await Run("deleting a selected original image has ordered undo/redo with mixed paste",async delegate{
                editor.Html="<p>原文前<img id='original-photo' src='"+first+"'>原文后</p>";string original=editor.Html;Caret(browser,true);Paste(browser);queue.Dequeue().SetResult("<span>新图前<img src='"+second+"'>新图后</span>");await Task.Delay(80);string pasted=editor.Html;
                Eval(browser,"(function(){var e=document.getElementById('progress-editor'),image=document.getElementById('original-photo');setTaskTraceImageRange(image);function signal(kind){var x=document.createEvent('Event');x.initEvent(kind,true,true);Object.defineProperty(x,'keyCode',{value:46});e.dispatchEvent(x);}signal('keydown');var r=taskTraceRange();r.deleteContents();r.collapse(true);var s=window.getSelection();s.removeAllRanges();s.addRange(r);signal('input');signal('keyup');})()");await Task.Delay(60);string deleted=editor.Html;
                Require(editor.ImageCount==1&&!deleted.Contains(first)&&deleted.Contains(second),"Image deletion fixture removed the wrong image");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==pasted&&editor.ImageCount==2,"Undo image deletion did not restore the original picture beside the newer pasted picture");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==original&&editor.ImageCount==1,"Undo paste after image deletion also removed the original picture");
                await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.Html==pasted,"Redo paste did not restore both pictures");
                await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.Html==deleted&&editor.ImageCount==1,"Redo deletion did not remove only the original picture");
            },failures);
            await Run("in-place image source change undo/redo retains exact images and HTML comments",async delegate{
                editor.Html="<p>图片前<!--hidden marker--><img id='mutable-photo' src='"+first+"'>图片中<img id='stable-photo' src='"+second+"'>图片后</p>";string original=editor.Html;Caret(browser,true);
                string replacement=new System.Web.Script.Serialization.JavaScriptSerializer().Serialize(second);
                Eval(browser,"(function(){var e=document.getElementById('progress-editor');function signal(kind){var x=document.createEvent('Event');x.initEvent(kind,true,true);Object.defineProperty(x,'keyCode',{value:65});e.dispatchEvent(x);}signal('keydown');document.getElementById('mutable-photo').setAttribute('src',"+replacement+");signal('input');signal('keyup');})()");await Task.Delay(60);string changed=editor.Html;
                Require(editor.ImageCount==2&&!changed.Contains(first)&&changed.Contains("<!--hidden marker-->"),"In-place source change fixture failed or changed HTML comment");
                await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==original&&editor.Html.Contains("<!--hidden marker-->"),"Undo in-place source change restored the wrong image or lost HTML comment");
                Require(Convert.ToString(Eval(browser,"document.getElementById('mutable-photo').getAttribute('src')"))==first&&Convert.ToString(Eval(browser,"document.getElementById('stable-photo').getAttribute('src')"))==second,"Undo changed media identity of an unrelated image");
                await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.Html==changed&&editor.ImageCount==2&&editor.Html.Contains("<!--hidden marker-->"),"Redo in-place source change restored stale media or lost HTML comment");
            },failures);
            await Run("task/date load clears history and ignores stale pending results",async delegate{
                editor.Html="<p>昨天</p>";Caret(browser,true);Paste(browser);queue.Dequeue().SetResult("<span>昨天图<img src='"+first+"'></span>");await Task.Delay(70);Caret(browser,true);Paste(browser);var stale=queue.Dequeue();
                editor.Html="<p>今天独立内容</p>";string today=editor.Html;await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==today&&!editor.IsPastePending,"Undo crossed task/date boundary");stale.SetResult("<img src='"+second+"'>");await Task.Delay(80);Require(editor.Html==today&&editor.ImageCount==0,"Stale result crossed task/date boundary");await Shortcut(browser,Keys.Control|Keys.Y);Require(editor.Html==today,"Redo crossed task/date boundary");
            },failures);
            await Run("image-only and mixed paste retain caret and viewport at the insertion paragraph",async delegate{
                foreach(bool mixed in new[]{false,true}) {
                    var fixture=new StringBuilder();for(int n=0;n<80;n++)fixture.Append("<p id='line-"+n+"'>Line "+n+" - left:right</p>");editor.Html=fixture.ToString();
                    Eval(browser,"(function(){var p=document.getElementById('line-45'),r=document.createRange(),s=window.getSelection();r.setStart(p.firstChild,14);r.collapse(true);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();p.scrollIntoView(true);window.scrollBy(0,-20)})()");
                    await Task.Delay(30);int before=Scroll(browser);Require(before>400,"Paste viewport fixture did not scroll");
                    Paste(browser);queue.Dequeue().SetResult(mixed?"<span>before<img src='"+first+"'>between<img src='"+second+"'>after</span>":"<img src='"+first+"'>");await Task.Delay(160);
                    string actual=Convert.ToString(Eval(browser,"(function(){var s=window.getSelection();if(!s.rangeCount)return 'no range';var r=s.getRangeAt(0),n=r.startContainer;while(n&&n.id!=='line-45')n=n.parentNode;return n&&r.collapsed?'line-45':'wrong paragraph'})()"));
                    Require(actual=="line-45","Pasted caret jumped away from insertion paragraph: "+actual);
                    Require(Math.Abs(Scroll(browser)-before)<=2,"Paste changed scroll from "+before+" to "+Scroll(browser));
                    Typing(browser,"NEXT-TYPED");await Task.Delay(40);
                    string paragraph=Convert.ToString(Eval(browser,"document.getElementById('line-45').innerHTML"));
                    Require(paragraph.Contains("NEXT-TYPED")&&paragraph.IndexOf("NEXT-TYPED")>paragraph.LastIndexOf("<img",StringComparison.OrdinalIgnoreCase),"Typing after paste did not continue after the pasted image/block");
                }
            },failures);
            await Run("direct image insertion keeps the current caret and viewport",async delegate{
                var fixture=new StringBuilder();for(int n=0;n<80;n++)fixture.Append("<p id='direct-"+n+"'>Line "+n+" - image here</p>");editor.Html=fixture.ToString();
                Eval(browser,"(function(){var p=document.getElementById('direct-45'),r=document.createRange(),s=window.getSelection();r.setStart(p.firstChild,10);r.collapse(true);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();p.scrollIntoView(true);window.scrollBy(0,-20)})()");await Task.Delay(30);int before=Scroll(browser);
                editor.InsertImage(Convert.FromBase64String(first.Substring(first.IndexOf(',')+1)));await Task.Delay(160);
                Require(Math.Abs(Scroll(browser)-before)<=2,"Direct image insertion changed scroll from "+before+" to "+Scroll(browser));
                Typing(browser,"DIRECT-NEXT");await Task.Delay(40);string paragraph=Convert.ToString(Eval(browser,"document.getElementById('direct-45').innerHTML"));
                Require(paragraph.Contains("DIRECT-NEXT")&&paragraph.IndexOf("DIRECT-NEXT")>paragraph.LastIndexOf("<img",StringComparison.OrdinalIgnoreCase),"Direct insertion lost the caret after its image");
            },failures);
            await Run("context-menu paste restores the saved caret without jumping to the beginning",async delegate{
                var fixture=new StringBuilder();for(int n=0;n<80;n++)fixture.Append("<p id='menu-"+n+"'>Line "+n+" - image here</p>");editor.Html=fixture.ToString();
                Eval(browser,"(function(){var p=document.getElementById('menu-45'),r=document.createRange(),s=window.getSelection();r.setStart(p.firstChild,10);r.collapse(true);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();p.scrollIntoView(true);window.scrollBy(0,-20);p.parentNode.blur();s.removeAllRanges()})()");await Task.Delay(30);int before=Scroll(browser);
                var menu=(ContextMenuStrip)typeof(ProgressHtmlEditor).GetField("clipboardMenu",System.Reflection.BindingFlags.Instance|System.Reflection.BindingFlags.NonPublic).GetValue(editor);
                foreach(ToolStripItem item in menu.Items)if(item.Text=="\u7c98\u8d34")((ToolStripMenuItem)item).PerformClick();
                Require(queue.Count==1,"Context-menu paste did not request clipboard content");queue.Dequeue().SetResult("<img src='"+first+"'>");await Task.Delay(160);
                Require(Math.Abs(Scroll(browser)-before)<=2,"Context-menu image paste reset the viewport");Typing(browser,"MENU-NEXT");await Task.Delay(40);
                Require(Convert.ToString(Eval(browser,"document.getElementById('menu-45').innerHTML")).Contains("MENU-NEXT"),"Context-menu paste lost its saved caret");
            },failures);
            await Run("deferred paste viewport never overrides newer user scrolling",async delegate{
                var fixture=new StringBuilder();for(int n=0;n<80;n++)fixture.Append("<p>Line "+n+" - original content</p>");editor.Html=fixture.ToString();Caret(browser,true);Eval(browser,"window.scrollTo(0,800)");
                Eval(browser,"insertTaskTraceHtml(\"<img src='"+first+"'>\",taskTraceRange());var wheel=document.createEvent('Event');wheel.initEvent('mousewheel',true,true);document.getElementById('progress-editor').dispatchEvent(wheel);window.scrollTo(0,180)");
                await Task.Delay(100);Require(Math.Abs(Scroll(browser)-180)<=2,"A stale paste viewport overwrote newer user scrolling");
            },failures);
            await Run("paste retains captured caret and undo restores editor scroll",async delegate{
                var fixture=new StringBuilder();for(int n=0;n<80;n++)fixture.Append("<p>滚动行 "+n+"：保留原文</p>");editor.Html=fixture.ToString();Eval(browser,"(function(){var e=document.getElementById('progress-editor'),p=e.children[45],r=document.createRange(),s=window.getSelection();r.selectNodeContents(p);r.collapse(false);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();p.scrollIntoView(true);window.scrollBy(0,-20)})()");await Task.Delay(30);int before=Scroll(browser);Require(before>400,"Scroll fixture not scrolled");string original=editor.Html;
                Paste(browser);Caret(browser,false);queue.Dequeue().SetResult("<span>定位图片<img src='"+first+"'></span>");await Task.Delay(80);string html=editor.Html;Require(html.IndexOf("滚动行 45")<html.IndexOf("定位图片")&&html.IndexOf("定位图片")<html.IndexOf("滚动行 46"),"Paste followed later caret");await Shortcut(browser,Keys.Control|Keys.Z);Require(editor.Html==original,"Undo scroll fixture did not restore content");Require(Math.Abs(Scroll(browser)-before)<=2,"Undo changed scroll from "+before+" to "+Scroll(browser));
            },failures);
            host.Controls.Remove(editor);
        }
        if(failures.Count>0)throw new Exception(String.Join(Environment.NewLine,failures.ToArray()));
    }
    [STAThread]static int Main(){Console.OutputEncoding=Encoding.UTF8;Exception error=null;Application.EnableVisualStyles();using(var host=new Form{Opacity=0,ShowInTaskbar=false,Width=620,Height=360}){host.Shown+=async delegate{try{await Cases(host);}catch(Exception failure){error=failure;}finally{host.Close();}};Application.Run(host);}if(error!=null){Console.Error.WriteLine(error);return 1;}return 0;}
}
"@
$harness=Join-Path $testRoot 'FloatingPasteUndoTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingPasteUndoTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingPasteUndoTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Paste undo regression compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(45000)){$process.Kill();throw ('Paste undo tests timed out: '+$testRoot)}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw ('Paste undo tests failed: '+$testRoot)}
if($process.ExitCode){throw ('Paste undo tests failed: '+$process.ExitCode+'; '+$testRoot)}
Write-Output ('Results: '+$testRoot)