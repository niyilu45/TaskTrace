// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Net;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;

internal static class FloatingClipboard {
    static readonly Encoding Utf8 = new UTF8Encoding(false, true);
    [DllImport("user32.dll", CharSet=CharSet.Unicode)] static extern uint RegisterClipboardFormat(string format);
    [DllImport("user32.dll")] static extern bool OpenClipboard(IntPtr owner);
    [DllImport("user32.dll")] static extern bool CloseClipboard();
    [DllImport("user32.dll")] static extern IntPtr GetClipboardData(uint format);
    [DllImport("kernel32.dll")] static extern IntPtr GlobalLock(IntPtr memory);
    [DllImport("kernel32.dll")] static extern bool GlobalUnlock(IntPtr memory);
    [DllImport("kernel32.dll")] static extern UIntPtr GlobalSize(IntPtr memory);

    // CF_HTML is UTF-8, but some applications publish ANSI or UTF-16 bytes.
    // Read the original bytes before WinForms' UTF-8 decoder loses information.
    static byte[] HtmlBytes() {
        if(!OpenClipboard(IntPtr.Zero))return null;
        try {
            IntPtr memory=GetClipboardData(RegisterClipboardFormat("HTML Format"));
            if(memory==IntPtr.Zero)return null;
            ulong size=GlobalSize(memory).ToUInt64();
            if(size==0 || size>96*1024*1024)return null;
            IntPtr pointer=GlobalLock(memory);if(pointer==IntPtr.Zero)return null;
            try{var bytes=new byte[(int)size];Marshal.Copy(pointer,bytes,0,bytes.Length);return bytes;}
            finally{GlobalUnlock(memory);}
        }finally{CloseClipboard();}
    }

    internal static string Html(){return ReadData(Clipboard.GetDataObject(),HtmlBytes());}
    internal static Task<string> HtmlAsync() {
        var snapshot=SnapshotData(Clipboard.GetDataObject(),HtmlBytes());
        return Task.Run(delegate{try{return ReadData(snapshot);}finally{DisposeCopyData(snapshot);}});
    }
    internal static DataObject SnapshotData(IDataObject source,byte[] rawHtml=null) {
        var snapshot=new DataObject();if(source==null)return snapshot;
        foreach(string format in new[]{DataFormats.UnicodeText,DataFormats.Rtf,DataFormats.FileDrop}){var value=source.GetData(format,true);var paths=value as string[];if(paths!=null)value=paths.Clone();if(value!=null)snapshot.SetData(format,false,value);}
        var raw=rawHtml;if(raw==null){var value=source.GetData(DataFormats.Html,false);raw=StreamBytes(value as Stream);if(raw==null && value is string)snapshot.SetData(DataFormats.Html,false,value);}
        if(raw!=null)snapshot.SetData(DataFormats.Html,false,new MemoryStream(raw,false));
        var image=ImageBytes(source);if(image!=null)snapshot.SetData("PNG",false,new MemoryStream(image,false));
        return snapshot;
    }
    internal static string ReadData(IDataObject data,byte[] rawHtml=null,Func<Uri,byte[]> readRemote=null) {
        if(data==null)return "";
        string html=FloatingClipboardMedia.Expand(HtmlData(data,rawHtml),data,readRemote);
        if(html!="")return html;
        var files=data.GetData(DataFormats.FileDrop,true) as string[];
        if(files!=null){var paths=new System.Collections.Specialized.StringCollection();paths.AddRange(files);return FilesHtml(paths);}
        string text=Convert.ToString(data.GetData(DataFormats.UnicodeText,true));
        if(text!="")return TextHtml(text);
        return BitmapHtml(data);
    }

    internal static string FilesHtml(System.Collections.Specialized.StringCollection paths) {
        var html=new StringBuilder();long total=0;
        foreach(string path in paths) {
            var file=new FileInfo(path);if(!file.Exists)continue;total+=file.Length;
            if(total>64*1024*1024)throw new Exception("一次粘贴的文件不能超过 64 MB，请分次添加。");
            if(Regex.IsMatch(file.Extension,@"^\.(png|jpe?g|gif|bmp)$",RegexOptions.IgnoreCase)) {
                using(var image=Image.FromFile(path))using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);html.Append("<img src=\"data:image/png;base64,").Append(Convert.ToBase64String(stream.ToArray())).Append("\">");}
            } else html.Append("<a download=\"").Append(WebUtility.HtmlEncode(file.Name)).Append("\" href=\"data:application/octet-stream;base64,").Append(Convert.ToBase64String(File.ReadAllBytes(path))).Append("\">").Append(WebUtility.HtmlEncode(file.Name)).Append("</a> ");
        }
        return html.ToString();
    }
    internal static DataObject CopyData(string html,string text) {
        string prefix="<html><body><!--StartFragment-->",suffix="<!--EndFragment--></body></html>";
        string format="Version:1.0\r\nStartHTML:{0:D10}\r\nEndHTML:{1:D10}\r\nStartFragment:{2:D10}\r\nEndFragment:{3:D10}\r\n";
        int start=Encoding.UTF8.GetByteCount(String.Format(format,0,0,0,0)),fragment=start+Encoding.UTF8.GetByteCount(prefix),end=fragment+Encoding.UTF8.GetByteCount(html);
        var result=new DataObject();result.SetData(DataFormats.UnicodeText,true,text??"");
        // WinForms serializes HTML strings with the system ANSI encoding. CF_HTML offsets and content must stay UTF-8.
        result.SetData(DataFormats.Html,false,new MemoryStream(Encoding.UTF8.GetBytes(String.Format(format,start,end+Encoding.UTF8.GetByteCount(suffix),fragment,end)+prefix+html+suffix+"\0"),false));
        var images=Regex.Matches(html??"",@"<img\b[^>]*>",RegexOptions.IgnoreCase);
        if(images.Count==1 && String.IsNullOrWhiteSpace(text)) {
            var match=Regex.Match(images[0].Value,@"\bsrc\s*=\s*['""](?<src>data:image/(?:png|jpe?g|gif|bmp);base64,[A-Za-z0-9+/=\s]+)['""]",RegexOptions.IgnoreCase);
            if(match.Success)try{string value=match.Groups["src"].Value;AddImageFormats(result,Convert.FromBase64String(value.Substring(value.IndexOf(',')+1)));}catch(ArgumentException){}catch(FormatException){}
        }
        return result;
    }
    internal static byte[] PngBytes(Image image){if(image==null)return null;using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return stream.ToArray();}}
    static void AddImageFormats(DataObject data,byte[] bytes) {
        using(var stream=new MemoryStream(bytes))using(var original=Image.FromStream(stream)) {
            var png=PngBytes(original);data.SetData("PNG",false,new MemoryStream(png,false));data.SetData(DataFormats.Bitmap,true,new Bitmap(original));
        }
    }
    internal static DataObject CopyImageData(byte[] bytes){using(var stream=new MemoryStream(bytes))using(var image=Image.FromStream(stream)){return CopyData("<img src=\"data:image/png;base64,"+Convert.ToBase64String(PngBytes(image))+"\">","");}}
    internal static void DisposeCopyData(DataObject data){foreach(string format in new[]{DataFormats.Html,"PNG",DataFormats.Bitmap}){var value=data.GetData(format,false) as IDisposable;if(value!=null)value.Dispose();}}
    static bool WriteCopyData(DataObject data){try{Clipboard.SetDataObject(data,true,3,100);return true;}catch{return false;}finally{DisposeCopyData(data);}}
    internal static bool Copy(string html,string text){return WriteCopyData(CopyData(html,text));}
    internal static bool CopyImage(byte[] bytes){return WriteCopyData(CopyImageData(bytes));}
    internal static byte[] StreamBytes(Stream stream) {
        if(stream==null)return null;long position=stream.CanSeek?stream.Position:0;
        try{if(stream.CanSeek)stream.Position=0;using(var bytes=new MemoryStream()){stream.CopyTo(bytes);return bytes.ToArray();}}finally{if(stream.CanSeek)stream.Position=position;}
    }
    internal static string HtmlData(IDataObject data,byte[] rawHtml=null) {
        if(data==null)return "";var html=data.GetData(DataFormats.Html,false);
        return PrepareHtml(rawHtml??StreamBytes(html as Stream),html as string,Convert.ToString(data.GetData(DataFormats.UnicodeText,true)),Encoding.Default.CodePage);
    }
    internal static byte[] ImageBytes(IDataObject data) {
        if(data==null)return null;
        var png=data.GetData("PNG",false) as Stream;
        if(png!=null)return StreamBytes(png);
        var bitmap=data.GetData(DataFormats.Bitmap,true) as Image;return PngBytes(bitmap);
    }
    internal static byte[] ImageOnlyBytes(IDataObject data){string text=data==null?null:Convert.ToString(data.GetData(DataFormats.UnicodeText,true));return String.IsNullOrWhiteSpace(text)?ImageBytes(data):null;}
    internal static string BitmapHtml(IDataObject data){var bytes=ImageBytes(data);return bytes==null?"":"<img src=\"data:image/png;base64,"+Convert.ToBase64String(bytes)+"\">";}


    internal static string TextHtml(string text) {
        // Keep tabs, repeated spaces and empty lines without inheriting source fonts.
        string encoded=WebUtility.HtmlEncode((text??"").TrimEnd('\0')).Replace("\r\n","\n").Replace("\r","\n");
        encoded=Regex.Replace(encoded,@"(?m)^ +",match=>new string('\u00a0',match.Length));
        encoded=Regex.Replace(encoded,@" {2,}",match=>new string('\u00a0',match.Length-1)+" ").Replace("\t","&emsp;").Replace("\n","<br>");
        return encoded==""?"":"<p>"+encoded+"</p>";
    }

    static string ComparableText(string html) {
        string value=Regex.Replace(html??"",@"<!--[\s\S]*?-->|<(script|style|head)\b[^>]*>[\s\S]*?</\1\s*>","",RegexOptions.IgnoreCase);
        return CompactText(WebUtility.HtmlDecode(Regex.Replace(value,@"<[^>]*>","")));
    }
    static string CompactText(string text){return Regex.Replace(text??"",@"[\s\u200b\ufeff\0]","");}
    static Encoding StrictEncoding(int codePage){return Encoding.GetEncoding(codePage,EncoderFallback.ExceptionFallback,DecoderFallback.ExceptionFallback);}

    internal static string PrepareHtml(byte[] bytes,string decoded,string unicodeText,int ansiCodePage) {
        var candidates=new List<string>();bool hasUnicodeEncoding=false;
        if(bytes!=null && bytes.Length>0) {
            var encodings=new List<Encoding>{Utf8};
            // UTF-16 HTML is nonstandard, but still recoverable without guessing the text.
            if(bytes.Length>1 && ((bytes[0]==255 && bytes[1]==254) || (bytes[0]!=0 && bytes[1]==0)))encodings.Insert(0,StrictEncoding(1200));
            if(bytes.Length>1 && ((bytes[0]==254 && bytes[1]==255) || (bytes[0]==0 && bytes[1]!=0)))encodings.Insert(0,StrictEncoding(1201));
            var charset=Regex.Match(Encoding.ASCII.GetString(bytes),@"charset\s*=\s*['""\s]*(?<name>[a-zA-Z0-9_-]+)",RegexOptions.IgnoreCase);
            if(charset.Success)try{encodings.Add(StrictEncoding(Encoding.GetEncoding(charset.Groups["name"].Value).CodePage));}catch(ArgumentException){}
            if(ansiCodePage!=65001)encodings.Add(StrictEncoding(ansiCodePage));
            foreach(var encoding in encodings)try{
                int count=bytes.Length;
                if(encoding.CodePage==1200 || encoding.CodePage==1201){for(int i=0;i+1<bytes.Length;i+=2)if(bytes[i]==0 && bytes[i+1]==0){count=i;break;}}
                else {int terminator=Array.IndexOf(bytes,(byte)0);if(terminator>=0)count=terminator;}
                string value=encoding.GetString(bytes,0,count).TrimStart('\ufeff');
                // UTF-16 decoded as UTF-8 contains NULs throughout the markup.
                if(value.IndexOf('\0')>=0)continue;
                string fragment=Fragment(value,bytes,encoding);
                if(fragment!="" && !candidates.Contains(fragment)){candidates.Add(fragment);if(encoding.CodePage==65001 || encoding.CodePage==1200 || encoding.CodePage==1201)hasUnicodeEncoding=true;}
            }catch(DecoderFallbackException){}
        }
        if(candidates.Count==0 && !String.IsNullOrEmpty(decoded)){candidates.Add(Fragment(decoded.TrimEnd('\0'),null,Utf8));hasUnicodeEncoding=true;}
        string reference=CompactText(unicodeText);
        if(reference!="") {
            foreach(string candidate in candidates)if(ComparableText(candidate)==reference)return candidate;
            // A source can also put already-misdecoded UTF-8 into a valid CF_HTML payload.
            // Only accept a reversible repair when it agrees with the Unicode clipboard text.
            foreach(string candidate in candidates)foreach(int codePage in new[]{1252,28591,ansiCodePage})try{
                string repaired=Utf8.GetString(StrictEncoding(codePage).GetBytes(candidate));
                if(ComparableText(repaired)==reference)return repaired;
            }catch(ArgumentException){}
        }
        if(candidates.Count>0 && candidates[0].IndexOf('\ufffd')<0 && !(reference!="" && Regex.IsMatch(ComparableText(candidates[0]),@"\?{2,}")) && (hasUnicodeEncoding || reference==""))return candidates[0];
        return unicodeText==null?"":TextHtml(unicodeText);
    }

    static string Fragment(string html,byte[] bytes,Encoding encoding) {
        // Offsets are byte positions, not UTF-16 string indexes (Chinese and emoji differ).
        var start=Regex.Match(html,@"(?m)^StartFragment:\s*(\d+)");
        var end=Regex.Match(html,@"(?m)^EndFragment:\s*(\d+)");
        int from,to;
        if(bytes!=null && start.Success && end.Success && Int32.TryParse(start.Groups[1].Value,out from) && Int32.TryParse(end.Groups[1].Value,out to) && from>=0 && to>from && to<=bytes.Length) {
            try{return encoding.GetString(bytes,from,to-from).TrimEnd('\0');}catch(DecoderFallbackException){}
        }
        var fragment=Regex.Match(html,@"<!--\s*StartFragment\s*-->([\s\S]*?)<!--\s*EndFragment\s*-->",RegexOptions.IgnoreCase);
        if(fragment.Success)return fragment.Groups[1].Value;
        int first=html.IndexOf('<');return first<0?"":html.Substring(first);
    }

    internal const string Script=@"
function cleanTaskTracePaste(html){
 var root=document.createElement('div');root.innerHTML=html;var elements=root.querySelectorAll('*');
 for(var i=elements.length-1;i>=0;i--){var el=elements[i],tag=el.tagName.toLowerCase();
  if(/^(script|style|meta|link|iframe|object|embed|svg|form|input|button)$/.test(tag)){el.parentNode.removeChild(el);continue;}
  var indent=el.style.marginLeft||el.style.paddingLeft,first=el.style.textIndent,src=el.getAttribute('src')||el.getAttribute('data-tasktrace-src'),href=el.getAttribute('href'),download=el.getAttribute('download'),cols=el.getAttribute('colspan'),rows=el.getAttribute('rowspan'),start=el.getAttribute('start');
  while(el.attributes.length)el.removeAttribute(el.attributes[0].name);
  if(tag==='img'){if(src && /^(data:image\/(png|jpe?g|gif|bmp);base64,|\/?api\/v[12]\/tasks\/\d+\/attachments\/\d+)/i.test(src))el.setAttribute('src',src);else el.parentNode.removeChild(el);continue;}
  if(tag==='a' && href && /^(data:application\/octet-stream;base64,|\/?api\/v[12]\/tasks\/\d+\/attachments\/\d+)/i.test(href)){el.setAttribute('href',href);if(download)el.setAttribute('download',download);}
  if(/^(strong|b|em|i|u|s|strike|font|h[1-6])$/.test(tag)){var plain=document.createElement(/^h/.test(tag)?'p':'span');while(el.firstChild)plain.appendChild(el.firstChild);el.parentNode.replaceChild(plain,el);el=plain;}
  if(indent && /^\d+(\.\d+)?(px|pt|em|rem)$/.test(indent))el.style.marginLeft=indent;
  if(first && /^-?\d+(\.\d+)?(px|pt|em|rem)$/.test(first))el.style.textIndent=first;
  if(cols && /^\d+$/.test(cols))el.setAttribute('colspan',cols);if(rows && /^\d+$/.test(rows))el.setAttribute('rowspan',rows);if(start && /^\d+$/.test(start))el.setAttribute('start',start);
 }
 return root.innerHTML;
}
// Capture before focus changes, COM clipboard reads, or a toolbar/picker can disturb selection.
var taskTraceSavedRange=null,taskTraceSelectedImage=null,taskTraceImageInteraction=0,taskTraceImageViews={},taskTraceImageViewSequence=0;
function taskTraceViewport(image){var nodes=[],node=image?image.parentNode:document.getElementById('progress-editor');while(node&&node.nodeType===1){nodes.push({node:node,x:node.scrollLeft,y:node.scrollTop});node=node.parentNode;}return {nodes:nodes,x:window.pageXOffset||0,y:window.pageYOffset||0};}
function restoreTaskTraceViewport(view){for(var i=view.nodes.length-1;i>=0;i--){var item=view.nodes[i];item.node.scrollLeft=item.x;item.node.scrollTop=item.y;}window.scrollTo(view.x,view.y);}
function setTaskTraceImageRange(image){if(!image||!taskTraceInside(image))return;taskTraceSelectedImage=image;var range=document.createRange(),selection=window.getSelection();range.selectNode(image);selection.removeAllRanges();selection.addRange(range);taskTraceSavedRange=range.cloneRange();}
function retainTaskTraceImageView(image,view){var generation=taskTraceImageInteraction;restoreTaskTraceViewport(view);setTimeout(function(){if(generation===taskTraceImageInteraction&&taskTraceSelectedImage===image&&taskTraceInside(image))restoreTaskTraceViewport(view);},0);}
function resetTaskTraceImageViews(){taskTraceSelectedImage=null;taskTraceImageViews={};taskTraceImageInteraction++;}
function discardTaskTraceImageView(id){delete taskTraceImageViews[id];}
function restoreTaskTraceImageView(id,finish){var view=taskTraceImageViews[id];if(finish)delete taskTraceImageViews[id];if(!view||view.generation!==taskTraceImageInteraction||!taskTraceInside(view.image))return;setTaskTraceImageRange(view.image);retainTaskTraceImageView(view.image,view.viewport);}
function taskTraceOpenImage(e){e=e||window.event;var image=e.srcElement||e.target;if(!image||String(image.tagName).toLowerCase()!=='img')return true;var view=taskTraceViewport(image),id=++taskTraceImageViewSequence;setTaskTraceImageRange(image);taskTraceImageViews[id]={image:image,viewport:view,generation:taskTraceImageInteraction};if(e.preventDefault)e.preventDefault();e.returnValue=false;window.external.OpenImage(image.src||'',image.getAttribute('data-tasktrace-src')||'',id);retainTaskTraceImageView(image,view);return false;}
function finishTaskTraceImageSelection(e){e=e||window.event;var image=e.srcElement||e.target;if(taskTraceSelectedImage&&image===taskTraceSelectedImage){var view=taskTraceViewport(image);setTaskTraceImageRange(image);retainTaskTraceImageView(image,view);if(e.preventDefault)e.preventDefault();e.returnValue=false;return false;}rememberTaskTraceRange();return true;}
function taskTraceImageInteractionStarted(e){if(e.type==='keydown'){var code=e.keyCode||e.which;if(code===16||code===17||code===18||code===91||(e.ctrlKey&&(code===67||code===88)))return;}taskTraceImageInteraction++;if(e.type==='mousedown'||e.type==='keydown')taskTraceSelectedImage=null;}
document.addEventListener('mousedown',taskTraceImageInteractionStarted,true);
document.addEventListener('keydown',taskTraceImageInteractionStarted,true);
document.addEventListener('mousewheel',taskTraceImageInteractionStarted,true);
document.addEventListener('wheel',taskTraceImageInteractionStarted,true);
function taskTraceInside(node){var editor=document.getElementById('progress-editor');while(node){if(node===editor)return true;node=node.parentNode;}return false;}
function taskTraceRange(){if(taskTraceSelectedImage&&taskTraceInside(taskTraceSelectedImage)){var selected=document.createRange();selected.selectNode(taskTraceSelectedImage);return selected;}var editor=document.getElementById('progress-editor'),selection=window.getSelection(),range=selection&&selection.rangeCount?selection.getRangeAt(0):null;if(range){if(taskTraceInside(range.commonAncestorContainer))return range.cloneRange();var bounds=document.createRange();bounds.selectNodeContents(editor);if(range.compareBoundaryPoints(0,bounds)<=0&&range.compareBoundaryPoints(2,bounds)>=0)return bounds;}if(document.selection&&document.selection.type==='Control'){var control=document.selection.createRange();if(control.length===1&&taskTraceInside(control.item(0))){range=document.createRange();range.selectNode(control.item(0));return range;}}return null;}
function rememberTaskTraceRange(){var range=taskTraceRange();if(range)taskTraceSavedRange=range;}
function restoreTaskTraceRange(range){var editor=document.getElementById('progress-editor'),selection=window.getSelection();range=range||taskTraceRange()||taskTraceSavedRange;if(!range||!taskTraceInside(range.commonAncestorContainer)){range=document.createRange();range.selectNodeContents(editor);range.collapse(false);}editor.focus();selection.removeAllRanges();selection.addRange(range);return range;}
function insertTaskTraceHtml(html,range){var editor=document.getElementById('progress-editor');if(editor.getAttribute('contenteditable')==='false'||!html)return false;range=restoreTaskTraceRange(range);taskTraceSelectedImage=null;taskTraceImageInteraction++;range.deleteContents();var holder=document.createElement('div');holder.innerHTML=html;var fragment=document.createDocumentFragment(),last;while(holder.firstChild){last=holder.firstChild;fragment.appendChild(last);}range.insertNode(fragment);if(last)range.setStartAfter(last);range.collapse(true);ensureTaskTraceImageCarets();var selection=window.getSelection();selection.removeAllRanges();selection.addRange(range);rememberTaskTraceRange();window.external.NotifyPaste();return true;}
var taskTracePendingPastes={},taskTracePasteOrder=[],taskTracePasteSequence=0;
function completeTaskTracePaste(id,html){var pending=taskTracePendingPastes[id];if(!pending)return;pending.html=html;pending.ready=true;while(taskTracePasteOrder.length){var first=taskTracePasteOrder[0],next=taskTracePendingPastes[first];if(!next||!next.ready)break;taskTracePasteOrder.shift();delete taskTracePendingPastes[first];var following=[];if(next.range.collapsed)for(var key in taskTracePendingPastes){var other=taskTracePendingPastes[key];if(other.range.collapsed&&other.range.compareBoundaryPoints(0,next.range)===0)following.push(other);}if(insertTaskTraceHtml(cleanTaskTracePaste(next.html),next.range))for(var i=0;i<following.length;i++)following[i].range=taskTraceSavedRange.cloneRange();}}
function resetTaskTracePastes(){taskTracePendingPastes={};taskTracePasteOrder=[];}
function pasteTaskTrace(e){e=e||window.event;if(e){if(e.preventDefault)e.preventDefault();e.returnValue=false;}
 var editor=document.getElementById('progress-editor');if(editor.getAttribute('contenteditable')==='false')return false;
 var range=taskTraceRange()||taskTraceSavedRange;if(!range){range=document.createRange();range.selectNodeContents(editor);range.collapse(false);}
 if(window.external.UsesAsyncClipboard()){var id=++taskTracePasteSequence;taskTracePendingPastes[id]={range:range.cloneRange(),ready:false};taskTracePasteOrder.push(id);window.external.RequestPasteHtml(id);}else insertTaskTraceHtml(cleanTaskTracePaste(window.external.PasteHtml()),range);return false;
}
function copyTaskTrace(e,cut){var editor=document.getElementById('progress-editor'),range=taskTraceRange()||taskTraceSavedRange;if(!range||range.collapsed)return true;var holder=document.createElement('div');holder.appendChild(range.cloneContents());var images=holder.getElementsByTagName('img');for(var i=0;i<images.length;i++)images[i].removeAttribute('data-tasktrace-src');var html=holder.innerHTML.replace(/\u200b/g,''),text=(holder.innerText||holder.textContent||'').replace(/\u200b/g,'');if(!window.external.CopyHtml(html,text)){if(e){if(e.preventDefault)e.preventDefault();e.returnValue=false;}return false;}if(e){if(e.preventDefault)e.preventDefault();e.returnValue=false;}if(cut&&editor.getAttribute('contenteditable')!=='false'){taskTraceSelectedImage=null;taskTraceImageInteraction++;range.deleteContents();rememberTaskTraceRange();window.external.NotifyPaste();}return false;}
function selectTaskTraceImage(e){e=e||window.event;var image=e.srcElement||e.target;if(!image||String(image.tagName).toLowerCase()!=='img'||e.button===2)return true;var view=taskTraceViewport(image);document.getElementById('progress-editor').focus();setTaskTraceImageRange(image);retainTaskTraceImageView(image,view);if(e.preventDefault)e.preventDefault();e.returnValue=false;return false;}
function selectAllTaskTrace(){taskTraceSelectedImage=null;var editor=document.getElementById('progress-editor'),range=document.createRange(),selection=window.getSelection();editor.focus();range.selectNodeContents(editor);selection.removeAllRanges();selection.addRange(range);taskTraceSavedRange=range.cloneRange();}
function taskTraceHasSelection(){var range=taskTraceRange()||taskTraceSavedRange;return !!range&&!range.collapsed;}
function taskTraceClipboardKey(e){var key=e.keyCode||e.which;if(e.shiftKey||e.metaKey)return;var handled=false;if(key===65&&((e.ctrlKey&&!e.altKey)||(e.altKey&&!e.ctrlKey))){selectAllTaskTrace();handled=true;}else if(e.ctrlKey&&!e.altKey&&(key===67||key===88))handled=copyTaskTrace(null,key===88)===false;if(handled){if(e.preventDefault)e.preventDefault();e.returnValue=false;}}
document.addEventListener('keydown',taskTraceClipboardKey);
document.addEventListener('selectionchange',rememberTaskTraceRange);
// Browser-wide select-all can target body; copy through the bounded editor range.
document.addEventListener('copy',function(e){copyTaskTrace(e,false);});
document.addEventListener('cut',function(e){copyTaskTrace(e,true);});
";
}
