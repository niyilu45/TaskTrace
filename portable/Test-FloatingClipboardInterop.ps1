param([string]$OutputDirectory='.local-build/floating-clipboard-interop-tests')
$ErrorActionPreference='Stop'
$repoRoot=Split-Path $PSScriptRoot -Parent
$testRoot=[IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot=[IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\')+'\'
if(!$testRoot.StartsWith($expectedRoot,[StringComparison]::OrdinalIgnoreCase)){throw 'Test output must remain inside .local-build.'}
$testRoot=Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Exercise the real Windows OLE HGLOBAL transport without touching the user's clipboard.
$source=@"
using System;
using System.Drawing;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;
static class FloatingClipboardInteropTests {
    [DllImport("kernel32.dll")] static extern UIntPtr GlobalSize(IntPtr value);
    [DllImport("kernel32.dll")] static extern IntPtr GlobalLock(IntPtr value);
    [DllImport("kernel32.dll")] static extern bool GlobalUnlock(IntPtr value);
    [DllImport("ole32.dll")] static extern void ReleaseStgMedium(ref System.Runtime.InteropServices.ComTypes.STGMEDIUM medium);
    static void Require(bool ok,string reason){if(!ok)throw new Exception(reason);}
    static byte[] NativeBytes(DataObject data,string name) {
        var com=(System.Runtime.InteropServices.ComTypes.IDataObject)data;
        var format=new System.Runtime.InteropServices.ComTypes.FORMATETC{cfFormat=(short)DataFormats.GetFormat(name).Id,dwAspect=System.Runtime.InteropServices.ComTypes.DVASPECT.DVASPECT_CONTENT,lindex=-1,tymed=System.Runtime.InteropServices.ComTypes.TYMED.TYMED_HGLOBAL};
        System.Runtime.InteropServices.ComTypes.STGMEDIUM medium;com.GetData(ref format,out medium);
        try{int size=checked((int)GlobalSize(medium.unionmember).ToUInt64());var bytes=new byte[size];var pointer=GlobalLock(medium.unionmember);Require(pointer!=IntPtr.Zero,"Cannot read native clipboard memory");try{Marshal.Copy(pointer,bytes,0,size);}finally{GlobalUnlock(medium.unionmember);}return bytes;}finally{ReleaseStgMedium(ref medium);}
    }
    static DataObject Transfer(DataObject source) {
        var target=new DataObject();target.SetData(DataFormats.Html,false,new MemoryStream(NativeBytes(source,DataFormats.Html),false));target.SetData(DataFormats.UnicodeText,false,Encoding.Unicode.GetString(NativeBytes(source,DataFormats.UnicodeText)).TrimEnd('\0'));return target;
    }
    static string ImageUrl(int seed,int size) {
        using(var bitmap=new Bitmap(size,size))using(var stream=new MemoryStream()) {
            var random=new Random(seed);for(int y=0;y<size;y++)for(int x=0;x<size;x++)bitmap.SetPixel(x,y,Color.FromArgb(random.Next(256),random.Next(256),random.Next(256)));
            bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return "data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());
        }
    }
    static byte[] ImageBytes(string image){return Convert.FromBase64String(image.Substring(image.IndexOf(',')+1));}
    static int ImageCount(string html){return Regex.Matches(html,"<img\\b",RegexOptions.IgnoreCase).Count;}
    static string RtfPicture(string image){return @"{\pict\pngblip\picw300\pich300 "+BitConverter.ToString(ImageBytes(image)).Replace("-","")+"}";}
    static async Task Ready(WebBrowser browser){for(int i=0;i<100 && (browser.Document==null || browser.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(30);Require(browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Editor not ready");}
    static async Task Check(Form host) {
        string first=ImageUrl(1,300),second=ImageUrl(2,280),html="<p>第一段😀<img src='"+first+"'>第二段<img src='"+second+"'>第三段</p>";
        var original=FloatingClipboard.CopyData(html,"第一段😀第二段第三段");var transferred=Transfer(original);
        try{string result=FloatingClipboard.ReadData(transferred);Require(result.Contains(first) && result.Contains(second) && result.Contains("第一段😀"),"Windows OLE serialization damaged Chinese/emoji or lost inline images");}
        finally{FloatingClipboard.DisposeCopyData(original);FloatingClipboard.DisposeCopyData(transferred);}
        Console.WriteLine("PASS Windows OLE HGLOBAL export/import retains Chinese, emoji and two full-size images.");
        DataObject payload=null;
        using(var source=new ProgressHtmlEditor(()=>"",(value,text)=>{if(payload!=null)FloatingClipboard.DisposeCopyData(payload);var outgoing=FloatingClipboard.CopyData(value,text);try{payload=Transfer(outgoing);}finally{FloatingClipboard.DisposeCopyData(outgoing);}return true;}){Dock=DockStyle.Fill})
        using(var destination=new ProgressHtmlEditor(()=>FloatingClipboard.ReadData(payload)){Dock=DockStyle.Fill}) {
            host.Controls.Add(source);var browser=(ProgressEditorBrowser)source.Controls[0];await Ready(browser);source.Html=html;
            browser.ShortcutForTest(Keys.Control|Keys.A);browser.ShortcutForTest(Keys.Control|Keys.C);
            host.Controls.Add(destination);destination.BringToFront();var target=(WebBrowser)destination.Controls[0];await Ready(target);
            target.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});await Task.Delay(30);
            Require(destination.ImageCount==2 && destination.Html.Contains(first) && destination.Html.Contains(second) && destination.Html.Contains("第一段😀"),"Native copy after OLE transport lost text/media when pasted into another editor");
            Console.WriteLine("PASS native editor select-all/copy, Windows OLE transport and real destination editor paste.");
            string folder=Path.Combine(Path.GetDirectoryName(Application.ExecutablePath),"word-images");Directory.CreateDirectory(folder);string imagePath=Path.Combine(folder,"中文截图.png");File.WriteAllBytes(imagePath,ImageBytes(first));
            var external=new DataObject();external.SetData(DataFormats.Html,false,"<p style='font-size:40pt;color:white;background:black;margin-left:36pt'>文前<img src='"+new Uri(imagePath).AbsoluteUri+"'>文后</p>");external.SetData(DataFormats.UnicodeText,"文前文后");
            FloatingClipboard.DisposeCopyData(payload);payload=external;destination.Html="";
            target.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});
            Require(destination.ImageCount==1 && destination.Html.Contains("文前") && destination.Html.Contains("文后") && !destination.Html.Contains("file:") && !destination.Html.Contains("color:") && !destination.Html.Contains("font-size") && destination.Html.Contains("36pt"),"Word HTML file image was lost, retained a temporary path or imported source font/colors");
            Console.WriteLine("PASS Word-style temporary file image with text, indentation and source-format cleanup.");
            string rtf=@"{\rtf1\ansi\ansicpg936\uc1{\fonttbl{\f0 Arial;}}\f0\fs80\li720 \u20013?\u25991?before"+RtfPicture(first)+@"\line between"+RtfPicture(second)+@"\par after{\nonshppict"+RtfPicture(first)+"}}";
            external=new DataObject();external.SetData(DataFormats.Rtf,rtf);external.SetData(DataFormats.UnicodeText,"中文before\nbetween\nafter");FloatingClipboard.DisposeCopyData(payload);payload=external;destination.Html="";
            target.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});
            string pasted=destination.Html;Require(destination.ImageCount==2 && pasted.Contains("中文before") && pasted.IndexOf("before")<pasted.IndexOf("<img") && pasted.IndexOf("between")>pasted.IndexOf("<img") && pasted.IndexOf("between")<pasted.LastIndexOf("<img") && pasted.Contains("after") && pasted.Contains("36pt") && !pasted.Contains("Arial"),"RTF-only text/images lost position, Unicode, paragraph indentation or imported source fonts");
            Console.WriteLine("PASS RTF-only mixed text/two images, Unicode, line breaks, indentation and alternative-image deduplication.");
            external.SetData(DataFormats.Html,false,"<p>before<img src='cid:one'>between<img src='cid:two'>after</p>");external.SetData(DataFormats.UnicodeText,"beforebetweenafter");destination.Html="";
            target.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});Require(destination.ImageCount==2,"RTF backing images were not substituted into external HTML");
            Console.WriteLine("PASS external HTML resolves each unavailable image from its corresponding RTF image.");
            var web=new DataObject();web.SetData(DataFormats.Html,false,"SourceURL:https://clipboard.test/articles/page.html\r\n<html><!--StartFragment--><p>web前<img src='../one.png'>web中<img src='https://images.test/two.png'>web后</p><!--EndFragment--></html>");web.SetData(DataFormats.UnicodeText,"web前web中web后");int requests=0;
            string webHtml=FloatingClipboard.ReadData(web,null,delegate(Uri uri){requests++;Require(uri.AbsoluteUri=="https://clipboard.test/one.png" || uri.AbsoluteUri=="https://images.test/two.png","Web image URL was resolved against the wrong source");return ImageBytes(uri.Host=="images.test"?second:first);});
            Require(requests==2 && ImageCount(webHtml)==2 && !webHtml.Contains("https:"),"Browser HTML source URL/images were not embedded portably");
            var chat=new DataObject();chat.SetData(DataFormats.Html,false,"<p>聊天文字<img src='cid:chat-image'>后面的文字</p>");chat.SetData(DataFormats.UnicodeText,"聊天文字后面的文字");chat.SetData("PNG",false,new MemoryStream(ImageBytes(first),false));
            string chatHtml=FloatingClipboard.ReadData(chat);Require(ImageCount(chatHtml)==1 && chatHtml.Contains("聊天文字") && chatHtml.Contains("data:image/png"),"Chat software HTML plus PNG clipboard lost mixed text/image content");
            Console.WriteLine("PASS browser relative/absolute image resolution and chat-style HTML plus native PNG fallback.");
            var snapshot=FloatingClipboard.SnapshotData(web);web.SetData(DataFormats.UnicodeText,"changed clipboard");Require(!Convert.ToString(snapshot.GetData(DataFormats.UnicodeText)).Contains("changed clipboard"),"Async paste read a later clipboard instead of its original snapshot");FloatingClipboard.DisposeCopyData(snapshot);FloatingClipboard.DisposeCopyData(chat);
            external=new DataObject();external.SetData(DataFormats.Html,false,"<p>before<img src='file:///does-not-exist.png'>after</p>");external.SetData(DataFormats.UnicodeText,"beforeafter");FloatingClipboard.DisposeCopyData(payload);payload=external;destination.Html="<p>existing unsaved note</p>";string error="";destination.ClipboardFailed+=delegate(object sender,ProgressClipboardErrorEventArgs failure){error=failure.Message;};
            target.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});await Task.Delay(40);Require(destination.Html.Contains("existing unsaved note") && destination.ImageCount==0 && error!="","Unavailable clipboard image silently lost data or changed existing unsaved content");
            Console.WriteLine("PASS unavailable external image reports a paste error and preserves all existing unsaved content.");
        }
        if(payload!=null)FloatingClipboard.DisposeCopyData(payload);
        await AsyncPasteCases(host,first,second);
    }
    static async Task AsyncPasteCases(Form host,string first,string second) {
        var pending=new System.Collections.Generic.Queue<TaskCompletionSource<string>>();
        using(var editor=new ProgressHtmlEditor(delegate{var completion=new TaskCompletionSource<string>();pending.Enqueue(completion);return completion.Task;},true){Dock=DockStyle.Fill}) {
            host.Controls.Add(editor);editor.BringToFront();var browser=(WebBrowser)editor.Controls[0];await Ready(browser);
            editor.Html="<p>左边右边</p>";
            browser.Document.InvokeScript("eval",new object[]{"var t=document.getElementById('progress-editor').firstChild.firstChild,r=document.createRange(),s=window.getSelection();r.setStart(t,2);r.collapse(true);s.removeAllRanges();s.addRange(r);pasteTaskTrace({preventDefault:function(){}});r.selectNodeContents(document.getElementById('progress-editor'));r.collapse(false);s.removeAllRanges();s.addRange(r);"});
            Require(pending.Count==1 && editor.IsPastePending && editor.ImageCount==0,"Async clipboard did not return before data arrived");
            pending.Dequeue().SetResult("<p>图前<img src='"+first+"'>图后</p>");await Task.Delay(70);
            string html=editor.Html;Require(!editor.IsPastePending && editor.ImageCount==1 && html.IndexOf("左边")<html.IndexOf("<img") && html.IndexOf("<img")<html.IndexOf("右边"),"Async image paste followed a later caret instead of its captured position");
            editor.Html="<p>原任务</p>";browser.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});var stale=pending.Dequeue();editor.Html="<p>切换后的任务</p>";Require(!editor.IsPastePending,"Task/date switch retained pending paste/save state");stale.SetResult("<img src='"+first+"'>");await Task.Delay(70);Require(editor.ImageCount==0 && editor.Html.Contains("切换后的任务"),"A delayed clipboard result was inserted into a different task/date");
            editor.Html="";browser.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});pasteTaskTrace({preventDefault:function(){}});"});var earlier=pending.Dequeue();var later=pending.Dequeue();later.SetResult("<span>第二次<img src='"+second+"'></span>");await Task.Delay(30);Require(editor.ImageCount==0,"Later async paste overtook an earlier pending paste");earlier.SetResult("<span>第一次<img src='"+first+"'></span>");await Task.Delay(70);html=editor.Html;Require(editor.ImageCount==2 && html.IndexOf("第一次")<html.IndexOf("第二次"),"Concurrent async paste changed invocation order or lost images");
            editor.Html="<p>未保存的内容</p>";string failure="";editor.ClipboardFailed+=delegate(object sender,ProgressClipboardErrorEventArgs error){failure=error.Message;};browser.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});pending.Dequeue().SetException(new IOException("download failed"));await Task.Delay(70);Require(failure.Contains("download failed") && editor.Html.Contains("未保存的内容"),"Async image failure was silent or discarded existing input");
            host.Controls.Remove(editor);
        }
        Console.WriteLine("PASS async paste remains responsive, captures caret/snapshot, cancels on task/date changes, preserves multiple-paste ordering and reports errors without losing drafts.");
    }
    [STAThread] static int Main() {
        try{Application.EnableVisualStyles();Exception failure=null;using(var host=new Form{ShowInTaskbar=false,Opacity=0,Width=600,Height=450}){host.Shown+=async delegate{try{await Check(host);}catch(Exception error){failure=error;}finally{host.Close();}};Application.Run(host);}if(failure!=null)throw failure;return 0;}catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
"@
$harness=Join-Path $testRoot 'FloatingClipboardInteropTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingClipboardInteropTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingClipboardInteropTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Clipboard interoperability test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(30000)){$process.Kill();throw 'Clipboard interoperability tests timed out.'}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw 'Clipboard interoperability tests failed.'}
if($process.ExitCode){throw ('Clipboard interoperability tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)
