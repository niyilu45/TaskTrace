param([string]$OutputDirectory = '.local-build/floating-clipboard-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# In-memory clipboard payloads exercise the real editor without reading/replacing
# the user's clipboard, starting an installed app, or accessing personal data.
$source = @"
using System;
using System.Drawing;
using System.Collections.Specialized;
using System.Net;
using System.Net.Http;
using System.Threading;
using System.Collections.Generic;
using System.IO;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
static class FloatingClipboardTests {
    static void Require(bool ok,string message){if(!ok)throw new Exception(message);}
    static byte[] CfHtml(string fragment,Encoding encoding,bool markers) {
        string header="Version:1.0\r\nStartHTML:{0:D10}\r\nEndHTML:{1:D10}\r\nStartFragment:{2:D10}\r\nEndFragment:{3:D10}\r\n";
        string prefix="<html><head><meta charset='"+encoding.WebName+"'></head><body>片段之前"+(markers?"<!--StartFragment-->":"");
        string suffix=(markers?"<!--EndFragment-->":"")+"片段之后</body></html>";
        int startHtml=encoding.GetByteCount(String.Format(header,0,0,0,0));
        int startFragment=startHtml+encoding.GetByteCount(prefix),endFragment=startFragment+encoding.GetByteCount(fragment);
        return encoding.GetBytes(String.Format(header,startHtml,endFragment+encoding.GetByteCount(suffix),startFragment,endFragment)+prefix+fragment+suffix+"\0");
    }
    static string Read(byte[] bytes,string text,int codePage){return FloatingClipboard.PrepareHtml(bytes,null,text,codePage);}
    static void DecodeCases() {
        string text="中文、标点‘引号’和 emoji 😀\n下一行",fragment="<p>中文、标点‘引号’和 emoji 😀<br>下一行</p>";
        foreach(bool markers in new[]{true,false})Require(Read(CfHtml(fragment,Encoding.UTF8,markers),text,936)==fragment,"CF_HTML byte offsets damaged Chinese/emoji or included unselected context");
        string chinese="设备检查\n第二行",legacy="<p style='margin-left:36pt'>设备检查<br>第二行</p>";
        var gb=Encoding.GetEncoding(936);byte[] raw=CfHtml(legacy,gb,true);
        Require(Encoding.UTF8.GetString(raw).Contains("\ufffd"),"Legacy encoding fixture does not reproduce the previous lossy decoder");
        Require(Read(raw,chinese,1252)==legacy,"Declared GBK charset did not recover the original Unicode text");
        Require(Read(raw,null,936)==legacy,"HTML-only legacy source could not be decoded");
        foreach(var encoding in new[]{Encoding.Unicode,Encoding.BigEndianUnicode})Require(Read(encoding.GetBytes("<html><!--StartFragment-->"+fragment+"<!--EndFragment--></html>\0"),text,1252)==fragment,"UTF-16 clipboard HTML was read as UTF-8");
        string corrupt=Encoding.GetEncoding(28591).GetString(Encoding.UTF8.GetBytes(fragment));
        Require(Read(CfHtml(corrupt,Encoding.UTF8,true),text,936)==fragment,"Reversible mojibake was not repaired against Unicode clipboard text");
        Require(FloatingClipboard.PrepareHtml(null,"<p>\ufffd\ufffd</p>","中文",936)=="<p>中文</p>","Broken HTML did not fall back to intact Unicode text");
        Require(Read(gb.GetBytes("<p>设备检查</p>"),"设备检查",1252)=="<p>设备检查</p>","Undeclared legacy charset left mojibake instead of using Unicode text");
        Require(FloatingClipboard.PrepareHtml(null,"Header\r\n<html><!-- StartFragment -->"+fragment+"<!-- EndFragment --></html>\0",text,936)==fragment,"Spaced fragment markers or clipboard NUL leaked into content");
        Require(FloatingClipboard.PrepareHtml(null,"<p>UTF-8 &amp; 中文</p>","UTF-8 & 中文",936)=="<p>UTF-8 &amp; 中文</p>","HTML entities or valid Unicode text changed");
        Require(FloatingClipboard.PrepareHtml(null,"<p>????</p>","中文测试",936)=="<p>中文测试</p>","Lossy question marks overrode intact Unicode text");
        Require(FloatingClipboard.PrepareHtml(null,"<p>真的??</p>","真的??",936)=="<p>真的??</p>","Intentional question marks were mistaken for mojibake");
        byte[] terminated=CfHtml(fragment,Encoding.UTF8,true),padded=new byte[terminated.Length+4];Array.Copy(terminated,padded,terminated.Length);padded[padded.Length-1]=255;
        Require(Read(padded,text,936)==fragment,"Bytes after the clipboard NUL corrupted a valid HTML payload");
        Require(FloatingClipboard.PrepareHtml(null,null,null,936)=="","Empty clipboard produced content");
        Require(FloatingClipboard.TextHtml("one two  three")=="<p>one two\u00a0 three</p>","Ordinary word spaces became nonbreaking and prevented wrapping");
        string lines=FloatingClipboard.TextHtml("  缩进\r\n\r\n\t下一行 <>&\r\n");
        Require(lines=="<p>\u00a0\u00a0缩进<br><br>&emsp;下一行 &lt;&gt;&amp;<br></p>","Plain text lost spaces/indentation/empty lines or was interpreted as markup");
        Console.WriteLine("PASS UTF-8/GBK/UTF-16, byte offsets, Unicode fallback, reversible mojibake, entities, fragment markers, blank lines and indentation.");
    }
    static async Task EditorCases(Form host) {
        string payload="",copiedHtml="",copiedText="";DataObject copiedData=null;int copyWrites=0;bool allowCopy=true,disturbSelection=false;WebBrowser currentBrowser=null;
        using(var editor=new ProgressHtmlEditor(()=>{if(disturbSelection && currentBrowser!=null)currentBrowser.Document.InvokeScript("eval",new object[]{"var r=document.createRange();r.selectNodeContents(document.body);r.collapse(false);var s=window.getSelection();s.removeAllRanges();s.addRange(r);"});return payload;},(html,text)=>{if(!allowCopy)return false;if(copiedData!=null)FloatingClipboard.DisposeCopyData(copiedData);var exported=copiedData=FloatingClipboard.CopyData(html,text);copiedHtml=FloatingClipboard.HtmlData(exported);copiedText=(string)exported.GetData(DataFormats.UnicodeText);copyWrites++;return true;}){Dock=DockStyle.Fill}) {
            host.Controls.Add(editor);var browser=(WebBrowser)editor.Controls[0];currentBrowser=browser;
            for(int i=0;i<100 && (browser.Document==null || browser.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(40);
            Require(browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Editor did not load");
            string picture;using(var image=new Bitmap(2,2))using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);picture="data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());}
            string original="<p style='font-size:40pt;color:white;background:black;margin-left:36pt;text-indent:12pt'><b>中文😀在图片前</b><br><img src='"+picture+"'><br>中文在图片后</p>";
            payload=Read(CfHtml(original,Encoding.UTF8,true),"中文😀在图片前\n中文在图片后",936);
            editor.Html="<p>已有文字</p>";
            browser.Document.InvokeScript("eval",new object[]{"var e=document.getElementById('progress-editor'),r=document.createRange(),s=window.getSelection();r.selectNodeContents(e);r.collapse(false);s.removeAllRanges();s.addRange(r);pasteTaskTrace({preventDefault:function(){window.pasteCancelled=true;}});"});
            await Task.Delay(80);
            string saved=editor.Html;
            Require(saved.IndexOf("已有文字")<saved.IndexOf("中文😀在图片前") && saved.IndexOf("中文😀在图片前")<saved.IndexOf("data:image/png") && saved.IndexOf("data:image/png")<saved.IndexOf("中文在图片后"),"Paste changed text/image order, damaged Unicode or overwrote existing text");
            Require(editor.ImageCount==1 && editor.ChangeVersion>0,"Paste duplicated/lost its image or did not update the draft");
            Require(!saved.Contains("font-size") && !saved.Contains("background") && !saved.Contains("color:") && !saved.Contains("<b>") && saved.Contains("36pt") && saved.Contains("12pt") && saved.Contains("<br"),"Paste retained source fonts/theme or lost structure");
            Require(Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled"})),"Native paste was not cancelled");
            editor.Html=saved;Require(editor.Html.Contains("中文😀在图片前") && editor.ImageCount==1,"Reopening saved HTML changed Unicode or lost images");
            payload=FloatingClipboard.TextHtml("中文\r\n\r\n下一行");editor.Html="";
            browser.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});
            Require(editor.Html.Contains("中文<br><br>下一行"),"Plain Unicode paste lost blank lines in the real editor");
            // Clipboard access can temporarily disturb selection; the captured range must win.
            editor.Html="<p>左边右边</p>";payload="<img src='"+picture+"'>";disturbSelection=true;
            browser.Document.InvokeScript("eval",new object[]{"var t=document.getElementById('progress-editor').firstChild.firstChild,r=document.createRange(),s=window.getSelection();r.setStart(t,2);r.collapse(true);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();pasteTaskTrace({preventDefault:function(){}});"});
            disturbSelection=false;
            Require(editor.Html.IndexOf("左边")<editor.Html.IndexOf("<img") && editor.Html.IndexOf("<img")<editor.Html.IndexOf("右边"),"Clipboard read/focus moved an image away from the caret: "+editor.Html);
            editor.Html="<p>前面后面</p>";
            browser.Document.InvokeScript("eval",new object[]{"var t=document.getElementById('progress-editor').firstChild.firstChild,r=document.createRange(),s=window.getSelection();r.setStart(t,2);r.collapse(true);s.removeAllRanges();s.addRange(r);rememberTaskTraceRange();s.removeAllRanges();"});
            editor.InsertImage(Convert.FromBase64String(picture.Substring(picture.IndexOf(',')+1)));
            Require(editor.Html.IndexOf("前面")<editor.Html.IndexOf("<img") && editor.Html.IndexOf("<img")<editor.Html.IndexOf("后面"),"Toolbar image insertion forgot the saved caret after losing focus");
            editor.Html="<p>甲删除乙</p>";payload="替换";
            browser.Document.InvokeScript("eval",new object[]{"var t=document.getElementById('progress-editor').firstChild.firstChild,r=document.createRange(),s=window.getSelection();r.setStart(t,1);r.setEnd(t,3);s.removeAllRanges();s.addRange(r);pasteTaskTrace({preventDefault:function(){}});"});
            Require(editor.Html.Contains("甲替换乙") && !editor.Html.Contains("删除"),"Pasting did not replace exactly the selected text");
            string fileData="data:application/octet-stream;base64,AQIDBA==";
            editor.Html="<p>图前<img src='"+picture+"' data-tasktrace-src='/api/v1/tasks/3/attachments/8'>图中<img src='"+picture+"'>图后<a href='"+fileData+"' download='检查记录.txt'>检查记录.txt</a></p>";
            browser.Document.InvokeScript("eval",new object[]{"var outside=document.createElement('p');outside.id='outside-editor';outside.innerText='不应复制的窗口文字';document.body.appendChild(outside);var r=document.createRange(),s=window.getSelection();r.selectNodeContents(document.body);s.removeAllRanges();s.addRange(r);var copy=document.createEvent('Event');copy.initEvent('copy',true,true);window.nativeCopyContinued=document.body.dispatchEvent(copy);"});
            Require(copyWrites==1 && !copiedHtml.Contains("不应复制的窗口文字") && !Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.nativeCopyContinued"})),"Page-wide copy escaped the editor, ran twice or fell through to native text-only copy");
            Require(copiedHtml.Contains(picture) && System.Text.RegularExpressions.Regex.Matches(copiedHtml,"<img\\b",System.Text.RegularExpressions.RegexOptions.IgnoreCase).Count==2,"Page-wide select-all/copy lost inline images or bypassed the editor clipboard handler");
            Require(copiedHtml.Contains(fileData) && copiedHtml.Contains("download=") && !copiedHtml.Contains("data-tasktrace-src"),"Copy leaked a source-task attachment address or lost a file");
            Require(copiedText.Contains("图前") && copiedText.Contains("检查记录.txt") && !copiedText.Contains("\u200b"),"Copy lost Unicode text or included invisible caret markers");
            using(var destination=new ProgressHtmlEditor(()=>copiedHtml){Dock=DockStyle.Fill}) {
                host.Controls.Add(destination);destination.BringToFront();var target=(WebBrowser)destination.Controls[0];
                for(int i=0;i<100 && (target.Document==null || target.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(40);
                destination.Html="<p>目标开头目标结尾</p>";
                target.Document.InvokeScript("eval",new object[]{"var t=document.getElementById('progress-editor').firstChild.firstChild,r=document.createRange(),s=window.getSelection();r.setStart(t,4);r.collapse(true);s.removeAllRanges();s.addRange(r);pasteTaskTrace({preventDefault:function(){}});"});
                string pasted=destination.Html;
                Require(destination.ImageCount==2 && pasted.Contains(fileData) && pasted.Contains("检查记录.txt") && !pasted.Contains("/tasks/3/"),"Cross-window paste lost/duplicated images or files, or retained source-task dependencies");
                Require(pasted.IndexOf("目标开头")<pasted.IndexOf("图前") && pasted.IndexOf("图后")<pasted.IndexOf("目标结尾"),"Cross-window paste changed the caret position or interleaved layout");
                host.Controls.Remove(destination);
            }
            var nativeBrowser=(ProgressEditorBrowser)browser;string allContent=editor.Html;int allVersion=editor.ChangeVersion;
            foreach(bool readOnlyAll in new[]{false,true})foreach(Keys allKey in new[]{Keys.Control|Keys.A,Keys.Alt|Keys.A}) {
                editor.SetReadOnly(readOnlyAll);copiedHtml="";int previousWrites=copyWrites;
                Require(nativeBrowser.ShortcutForTest(allKey),"Select-all shortcut was not handled: "+allKey);
                Require(nativeBrowser.ShortcutForTest(Keys.Control|Keys.C),"Copy shortcut was not handled after select-all");
                Require(copyWrites==previousWrites+1 && System.Text.RegularExpressions.Regex.Matches(copiedHtml,"<img\\b",System.Text.RegularExpressions.RegexOptions.IgnoreCase).Count==2 && copiedHtml.Contains(fileData),"Shortcut select-all lost images/files or copied twice: "+allKey);
                Require(copiedText.Contains("图前") && copiedText.Contains("图后") && !copiedHtml.Contains("不应复制的窗口文字"),"Select-all shortcut included unrelated page content or lost text");
                Require(editor.Html==allContent && editor.ChangeVersion==allVersion,"Select-all/copy changed source content or its dirty state");
            }
            foreach(bool altAll in new[]{false,true}) {
                copiedHtml="";int previousWrites=copyWrites;
                browser.Document.InvokeScript("eval",new object[]{"function testClipboardKey(code,ctrl,alt){var e=document.createEvent('Event');e.initEvent('keydown',true,true);Object.defineProperty(e,'keyCode',{value:code});Object.defineProperty(e,'ctrlKey',{value:ctrl});Object.defineProperty(e,'altKey',{value:alt});return document.getElementById('progress-editor').dispatchEvent(e);}window.allKeyContinued=testClipboardKey(65,"+(altAll?"false,true":"true,false")+");window.copyKeyContinued=testClipboardKey(67,true,false);"});
                Require(copyWrites==previousWrites+1 && copiedHtml.Contains(picture) && copiedHtml.Contains(fileData),"Browser DOM keyboard route lost select-all media");
                Require(!Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.allKeyContinued||window.copyKeyContinued"})),"Browser keyboard route fell through to native select-all/copy");
            }
            editor.SetReadOnly(false);
            browser.Document.InvokeScript("eval",new object[]{"document.getElementById('outside-editor').parentNode.removeChild(document.getElementById('outside-editor'));"});
            using(var destination=new ProgressHtmlEditor(()=>copiedHtml){Dock=DockStyle.Fill}) {
                host.Controls.Add(destination);destination.BringToFront();var target=(WebBrowser)destination.Controls[0];
                for(int i=0;i<100 && (target.Document==null || target.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(40);
                target.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});
                string result=destination.Html;
                Require(destination.ImageCount==2 && result.Contains(fileData) && result.IndexOf("图前")<result.IndexOf("<img") && result.LastIndexOf("<img")<result.IndexOf("图后"),"Pasting shortcut select-all content into another editor lost images/files or changed their layout");
                host.Controls.Remove(destination);
            }
            browser.Document.InvokeScript("eval",new object[]{"var t=document.getElementById('progress-editor').firstChild.firstChild,r=document.createRange(),s=window.getSelection();r.setStart(t,0);r.setEnd(t,2);s.removeAllRanges();s.addRange(r);"});
            nativeBrowser.ShortcutForTest(Keys.Control|Keys.C);
            Require(copiedText=="图前" && !copiedHtml.Contains("<img"),"Partial text copy unexpectedly included the full document or images");
            nativeBrowser.ShortcutForTest(Keys.Control|Keys.A);allowCopy=false;nativeBrowser.ShortcutForTest(Keys.Control|Keys.X);allowCopy=true;
            Require(editor.Html==allContent && editor.ChangeVersion==allVersion,"Failed cut deleted unsaved text or images");
            int beforeCut=copyWrites;
            browser.Document.InvokeScript("eval",new object[]{"var cut=document.createEvent('Event');cut.initEvent('cut',true,true);document.body.dispatchEvent(cut);"});
            await Task.Delay(40);
            Require(copyWrites==beforeCut+1 && editor.ImageCount==0 && !editor.Html.Contains("检查记录.txt") && copiedHtml.Contains(fileData),"Whole-document cut ran twice, failed to clear selection or lost file/image bytes");
            Console.WriteLine("PASS Ctrl+A/Alt+A plus Ctrl+C through native and browser keys, bounded page-wide copy/cut, read-only select-all, partial selection, failed-cut recovery and two-image/file paste into another editor.");
            editor.Html="<p>备注前<img src='"+picture+"'>备注后</p>";int beforeCopy=editor.ChangeVersion;string beforeHtml=editor.Html;
            browser.Document.InvokeScript("eval",new object[]{"var image=document.getElementById('progress-editor').getElementsByTagName('img')[0],mouse=document.createEvent('MouseEvents');mouse.initMouseEvent('mousedown',true,true,window,1,0,0,0,0,false,false,false,false,0,null);image.dispatchEvent(mouse);"});
            Require(Convert.ToBoolean(browser.Document.InvokeScript("taskTraceHasSelection")),"Clicking a note image left a collapsed selection: "+Convert.ToString(browser.Document.InvokeScript("eval",new object[]{"String(document.getElementById('progress-editor').getAttribute('onmousedown'))+' range='+window.getSelection().rangeCount+' saved='+(taskTraceSavedRange?taskTraceSavedRange.collapsed:'none')"})));
            browser.Document.InvokeScript("eval",new object[]{"var copy=document.createEvent('Event');copy.initEvent('copy',true,true);document.getElementById('progress-editor').getElementsByTagName('img')[0].dispatchEvent(copy);"});
            Require(copiedData!=null && copiedData.GetDataPresent("PNG",false) && copiedData.GetDataPresent(DataFormats.Bitmap,false),"Copying a selected inline image did not publish PNG and bitmap formats");
            Require(!copiedHtml.Contains("备注前") && !copiedHtml.Contains("备注后"),"A single-image copy unexpectedly copied adjacent note text");
            Require(editor.Html==beforeHtml && editor.ChangeVersion==beforeCopy,"Selecting/copying an inline image changed the saved note or dirty state");
            editor.SetReadOnly(true);Require(editor.CopyFirstImageForTest(),"Read-only image context menu could not copy an image");
            Require(copiedData.GetDataPresent("PNG",false) && editor.Html==beforeHtml,"Read-only image copy lost its native image formats or modified the document");
            editor.SetReadOnly(false);
            FloatingClipboard.DisposeCopyData(copiedData);copiedData=null;
            string folder=Path.Combine(Path.GetDirectoryName(Application.ExecutablePath),"clipboard-files");Directory.CreateDirectory(folder);
            string filePath=Path.Combine(folder,"检查记录.txt"),imagePath=Path.Combine(folder,"截图.png");File.WriteAllBytes(filePath,new byte[]{1,2,3,4});File.WriteAllBytes(imagePath,Convert.FromBase64String(picture.Substring(picture.IndexOf(',')+1)));
            var paths=new StringCollection();paths.Add(filePath);paths.Add(imagePath);string fileHtml=FloatingClipboard.FilesHtml(paths);
            Require(fileHtml.Contains(fileData) && fileHtml.Contains("检查记录.txt") && fileHtml.Contains("data:image/png"),"Explorer file/image paste lost bytes or names");
            Console.WriteLine("PASS caret preservation, selected-text replacement, CF_HTML cross-window text/two images/file roundtrip, file-drop bytes and Unicode filenames.");
            editor.SetReadOnly(true);string readOnly=editor.Html;
            browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled=false;pasteTaskTrace({preventDefault:function(){window.pasteCancelled=true;}});"});
            Require(editor.Html==readOnly && Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled"})),"Read-only editor allowed native paste");
            editor.SetReadOnly(false);payload="";
            browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled=false;pasteTaskTrace({preventDefault:function(){window.pasteCancelled=true;}});"});
            Require(editor.Html==readOnly && Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled"})),"Empty/unavailable clipboard changed content or fell through to native paste");
        }
        Console.WriteLine("PASS actual editor Unicode/image paste, formatting cleanup, existing text, draft change, roundtrip, read-only and empty clipboard.");
    }
    static async Task ImageCopyCases(Form host) {
        byte[] bytes;using(var original=new Bitmap(640,480)){using(var graphics=Graphics.FromImage(original))graphics.Clear(Color.FromArgb(120,40,120,200));bytes=FloatingClipboard.PngBytes(original);}
        using(var imageData=new CopyDataLease(FloatingClipboard.CopyImageData(bytes))) {
            var data=imageData.Value;Require(data.GetDataPresent("PNG",false) && data.GetDataPresent(DataFormats.Bitmap,false) && data.GetDataPresent(DataFormats.Html,false),"Image copy omitted a native PNG, bitmap, or HTML format");
            byte[] roundtrip=FloatingClipboard.ImageOnlyBytes(data);Require(roundtrip!=null,"Empty Unicode text incorrectly hid the copied bitmap");
            using(var stream=new MemoryStream(roundtrip))using(var image=Image.FromStream(stream))Require(image.Width==640 && image.Height==480,"Copied only the thumbnail or clipped window, not the original image");
            var onlyPng=new DataObject();onlyPng.SetData("PNG",false,new MemoryStream(roundtrip));Require(FloatingClipboard.ImageOnlyBytes(onlyPng)!=null && FloatingClipboard.BitmapHtml(onlyPng).Contains("data:image/png;base64,"),"PNG-only clipboard from another application could not be pasted");
            onlyPng.SetData(DataFormats.UnicodeText,"保留文字");Require(FloatingClipboard.ImageOnlyBytes(onlyPng)==null,"Image detection swallowed a normal text paste");
        }
        DataObject copied=null;string feedback="";int writes=0,reads=0;
        Func<byte[],bool> writer=delegate(byte[] content){if(copied!=null)FloatingClipboard.DisposeCopyData(copied);copied=FloatingClipboard.CopyImageData(content);writes++;return true;};
        using(var picture=new ClipboardPictureBox{Size=new Size(88,58)})using(var action=new FloatingImageCopy(picture,delegate{reads++;return Task.FromResult(bytes);},message=>feedback=message,writer)) {
            host.Controls.Add(picture);((ToolStripMenuItem)picture.ContextMenuStrip.Items[0]).PerformClick();await action.Pending;
            Require(reads==1 && writes==1 && feedback.Contains("已复制"),"Outstanding thumbnail context copy did not read/write the original image");
            picture.CopyShortcutForTest();await action.Pending;Require(writes==2,"Thumbnail Ctrl+C did not copy the image");
            using(var bitmap=(Image)copied.GetData(DataFormats.Bitmap,false))Require(bitmap.Width==640,"Thumbnail shortcut copied the reduced preview");
            host.Controls.Remove(picture);
        }
        using(var zoom=new ZoomableImagePreview(writer){Size=new Size(160,100)}) {
            host.Controls.Add(zoom);using(var stream=new MemoryStream(bytes))using(var original=Image.FromStream(stream))zoom.SetPreviewImage(new Bitmap(original));
            zoom.CopyFeedback=message=>feedback=message;zoom.SimulateWheelForTest(SystemInformation.MouseWheelScrollDelta);zoom.CopyShortcutForTest();await zoom.ImageCopy.Pending;
            Require(writes==3 && copied.GetDataPresent("PNG",false),"Zoomed preview Ctrl+C failed");
            byte[] full=FloatingClipboard.ImageOnlyBytes(copied);using(var stream=new MemoryStream(full))using(var original=Image.FromStream(stream))Require(original.Width==640 && original.Height==480,"Zoomed preview copied its visible viewport instead of the complete original image");
            host.Controls.Remove(zoom);zoom.ClearPreviewImage();
        }
        using(var target=new ClipboardPictureBox())using(var action=new FloatingImageCopy(target,delegate{return Task.FromResult(bytes);},message=>feedback=message,delegate{return false;})) {
            await action.Copy();Require(feedback.Contains("失败") && ((ToolStripMenuItem)target.ContextMenuStrip.Items[0]).Enabled,"Clipboard failure was not reported or left copy disabled");
        }
        if(copied!=null)FloatingClipboard.DisposeCopyData(copied);
        Console.WriteLine("PASS native PNG/bitmap/HTML formats, PNG-only import, empty-text handling, original-resolution thumbnail/zoom menu and Ctrl+C, read-only inline copy, and clipboard error recovery.");
    }
    sealed class CopyDataLease : IDisposable {internal readonly DataObject Value;internal CopyDataLease(DataObject value){Value=value;}public void Dispose(){FloatingClipboard.DisposeCopyData(Value);}}
    static async Task WindowCases(Form host) {
        var registry=new FloatingEditorWindows();
        using(var first=registry.Open("progress:1"))using(var second=registry.Open("outstanding:1:item-a"))
        using(var a=new Form{Opacity=0,ShowInTaskbar=false,MinimizeBox=true})using(var b=new Form{Opacity=0,ShowInTaskbar=false,MinimizeBox=true}) {
            first.Attach(a);second.Attach(b);
            var waitA=FloatingWindow.ShowEditorForClipboardTest(a,host);var waitB=FloatingWindow.ShowEditorForClipboardTest(b,host);
            Require(a.Visible && b.Visible && host.Enabled && a.Enabled && b.Enabled && !a.Modal && !b.Modal && a.Owner==null && b.Owner==null,"Opening editors disabled/owned another window");
            var old=host.Location;host.Location=new Point(old.X+12,old.Y+8);Require(host.Location!=old,"Parent window could not be moved with two editors open");
            Require(registry.Open("progress:1")==null && registry.Open("outstanding:1:item-a")==null,"Reopening the same item created another editor");
            a.WindowState=FormWindowState.Minimized;Require(registry.Open("progress:1")==null && a.WindowState==FormWindowState.Normal,"Reopening an editor did not restore its minimized window");
            Require(!second.ChangeKey("progress:1") && second.Key=="outstanding:1:item-a","Switching editors replaced another item's registry entry");
            Require(second.ChangeKey("outstanding:1:item-b"),"Could not switch to an unoccupied outstanding item");
            using(var oldItem=registry.Open("outstanding:1:item-a"))Require(oldItem!=null,"Switching items did not release the old identity");
            a.Close();await waitA;first.Dispose();Require(b.Visible && b.Enabled,"Closing one editor closed/disabled another");
            using(var reopened=registry.Open("progress:1"))Require(reopened!=null,"A closed editor could not be reopened");
            b.Close();await waitB;
        }
        Console.WriteLine("PASS simultaneous modeless editors, enabled/movable parent, per-item identity, minimized restore, collision handling and independent close/reopen.");
    }
    [STAThread] static int Main(){Console.OutputEncoding=Encoding.UTF8;int code=0;Application.EnableVisualStyles();using(var host=new Form{Opacity=0,ShowInTaskbar=false}){host.Shown+=async delegate{try{DecodeCases();await EditorCases(host);await ImageCopyCases(host);await WindowCases(host);FloatingWindow.TestOutstandingEditorMerge();await FloatingWindow.TestEditorAttachments();}catch(Exception error){code=1;Console.Error.WriteLine(error);}finally{host.Close();}};Application.Run(host);}return code;}
}
internal sealed partial class FloatingWindow {
    internal FloatingWindow(HttpMessageHandler handler){http=new HttpClient(handler);url="http://127.0.0.1:1";}
    internal sealed class EditorAttachmentHandler : HttpMessageHandler {
        internal readonly Dictionary<long,byte[]> Files=new Dictionary<long,byte[]>();
        internal readonly List<string> Names=new List<string>();
        protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancellation) {
            string path=request.RequestUri.AbsolutePath;
            if(request.Method==HttpMethod.Post){var body=(MultipartFormDataContent)request.Content;foreach(var part in body){long id=1001+Files.Count;Files.Add(id,await part.ReadAsByteArrayAsync());Names.Add(part.Headers.ContentDisposition.FileName);return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent("{\"success\":[{\"id\":"+id+"}],\"errors\":[]}")};}}
            if(path.Contains("/attachments/")){long id=Int64.Parse(path.Substring(path.LastIndexOf('/')+1));return new HttpResponseMessage(HttpStatusCode.OK){Content=new ByteArrayContent(Files[id])};}
            return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent("{\"id\":12}")};
        }
    }
    internal static async Task TestEditorAttachments(){
        var handler=new EditorAttachmentHandler();using(var window=new FloatingWindow(handler)) {
            string image;using(var bitmap=new Bitmap(2,2))using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);image="data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());}
            string source="<p>前<a href='data:application/octet-stream;base64,AQIDBA==' download='检查记录.txt'>检查记录.txt</a>中<img src='"+image+"'>后</p>";
            string saved=await window.PersistProgressEditorImages(12,source);
            if(handler.Files.Count!=2 || !saved.Contains("href=\"/api/v1/tasks/12/attachments/1001\"") || !saved.Contains("src=\"/api/v1/tasks/12/attachments/1002\"") || saved.Contains("base64"))throw new Exception("Explicit save did not persist files and inline images separately");
            if(!handler.Names[0].Contains("检查记录") || !handler.Names[1].Contains("tasktrace-progress-"))throw new Exception("File names and inline image attachment types were mixed");
            string prepared=await window.PrepareProgressEditorHtml(12,saved);
            if(!prepared.Contains("data:application/octet-stream;base64,AQIDBA==") || !prepared.Contains(image) || !prepared.Contains("data-tasktrace-file"))throw new Exception("Reopening attachments did not prepare portable file/image bytes");
            await window.PersistProgressEditorImages(12,prepared);if(handler.Files.Count!=2)throw new Exception("Saving unchanged prepared attachments uploaded duplicates");
            prepared=System.Text.RegularExpressions.Regex.Replace(prepared,"\\sdata-tasktrace-(?:file|src)=\\\"[^\\\"]*\\\"","");
            string copied=await window.PersistProgressEditorImages(13,prepared);
            if(handler.Files.Count!=4 || !copied.Contains("/tasks/13/attachments/") || copied.Contains("/tasks/12/"))throw new Exception("Cross-task save retained source-task attachment dependencies");
        }
        Console.WriteLine("PASS staged file/image save, distinct attachment types, reopen, unchanged-save deduplication and cross-task attachment ownership.");
    }
    internal static Task ShowEditorForClipboardTest(Form dialog,Form owner){return ShowEditorWindowAsync(dialog,owner);}
    internal static void TestOutstandingEditorMerge(){
        var original=new System.Collections.Generic.List<PendingItem>{new PendingItem{Id="a",Html="甲",NoteHtml="备注",Priority=7},new PendingItem{Id="b",Html="乙",Priority=7}};
        Func<System.Collections.Generic.List<PendingItem>> snapshot=()=>original.ConvertAll(item=>item.Copy());
        var first=snapshot();first[0].NoteHtml="甲备注修改";var second=snapshot();second[1].Html="乙修改";
        var saved=MergeOutstandingEdits(original,first,original);saved=MergeOutstandingEdits(original,second,saved);
        if(saved[0].NoteHtml!="甲备注修改" || saved[1].Html!="乙修改")throw new Exception("Saving a second outstanding window overwrote the first");
        var priority=snapshot();priority[0].Priority=2;saved=MergeOutstandingEdits(original,priority,saved);
        if(saved[0].NoteHtml!="甲备注修改" || saved[0].Priority!=2)throw new Exception("Unrelated same-item fields were overwritten");
        var removed=snapshot();removed.RemoveAt(1);var latest=snapshot();latest.RemoveAt(1);
        var unchanged=snapshot();unchanged[0].NoteHtml="其他备注";var merged=MergeOutstandingEdits(original,unchanged,latest);if(merged.Count!=1)throw new Exception("An older window resurrected a deleted item");
        bool conflict=false;var overlap=snapshot();overlap[0].NoteHtml="冲突备注";try{MergeOutstandingEdits(original,overlap,saved);}catch{conflict=true;}
        if(!conflict)throw new Exception("Overlapping changes silently overwrote saved notes");
        var added=snapshot();added.Add(new PendingItem{Id="c",Html="新增"});merged=MergeOutstandingEdits(original,added,saved);if(merged.Count!=3 || merged[0].NoteHtml!="甲备注修改")throw new Exception("Adding an item replaced another saved edit");
        Console.WriteLine("PASS independent outstanding editor deltas, disjoint same-item fields, additions, deletion preservation and overlap rejection.");
    }
}
"@
$harness=Join-Path $testRoot 'FloatingClipboardTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingClipboardTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingClipboardTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Clipboard test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(30000)){ $process.Kill();throw 'Clipboard tests timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw 'Clipboard tests failed.'}
if($process.ExitCode){throw ('Clipboard tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)