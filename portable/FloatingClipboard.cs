// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Net;
using System.Runtime.InteropServices;
using System.Text;
using System.Text.RegularExpressions;
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
            if(size==0 || size>64*1024*1024)return null;
            IntPtr pointer=GlobalLock(memory);if(pointer==IntPtr.Zero)return null;
            try{var bytes=new byte[(int)size];Marshal.Copy(pointer,bytes,0,bytes.Length);return bytes;}
            finally{GlobalUnlock(memory);}
        }finally{CloseClipboard();}
    }

    internal static string Html() {
        string text=null;
        try {
            text=Clipboard.ContainsText(TextDataFormat.UnicodeText)?Clipboard.GetText(TextDataFormat.UnicodeText):null;
            byte[] bytes=HtmlBytes();
            string html=bytes==null && Clipboard.ContainsText(TextDataFormat.Html)?Clipboard.GetText(TextDataFormat.Html):"";
            html=PrepareHtml(bytes,html,text,Encoding.Default.CodePage);
            if(html!="") {
                if(Clipboard.ContainsImage())html=Regex.Replace(html,@"<img\b[^>]*>",delegate(Match match){return Regex.IsMatch(match.Value,@"(?:data:image/|/tasks/\d+/attachments/)",RegexOptions.IgnoreCase)?match.Value:ImageHtml();},RegexOptions.IgnoreCase);
                return html;
            }
            if(text!=null)return TextHtml(text);
            if(Clipboard.ContainsText())return TextHtml(Clipboard.GetText());
            return Clipboard.ContainsImage()?ImageHtml():"";
        }catch{return text==null?"":TextHtml(text);}
    }

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

    static string ImageHtml(){using(var image=Clipboard.GetImage())using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return "<img src=\"data:image/png;base64,"+Convert.ToBase64String(stream.ToArray())+"\">";}}
    internal const string Script=@"
function cleanTaskTracePaste(html){
 var root=document.createElement('div');root.innerHTML=html;var elements=root.querySelectorAll('*');
 for(var i=elements.length-1;i>=0;i--){var el=elements[i],tag=el.tagName.toLowerCase();
  if(/^(script|style|meta|link|iframe|object|embed|svg|form|input|button)$/.test(tag)){el.parentNode.removeChild(el);continue;}
  var indent=el.style.marginLeft||el.style.paddingLeft,first=el.style.textIndent,src=el.getAttribute('data-tasktrace-src')||el.getAttribute('src'),cols=el.getAttribute('colspan'),rows=el.getAttribute('rowspan'),start=el.getAttribute('start');
  while(el.attributes.length)el.removeAttribute(el.attributes[0].name);
  if(tag==='img'){if(src && /^(data:image\/(png|jpe?g|gif|bmp);base64,|\/?api\/v[12]\/tasks\/\d+\/attachments\/\d+)/i.test(src))el.setAttribute('src',src);else el.parentNode.removeChild(el);continue;}
  if(/^(strong|b|em|i|u|s|strike|font|h[1-6])$/.test(tag)){var plain=document.createElement(/^h/.test(tag)?'p':'span');while(el.firstChild)plain.appendChild(el.firstChild);el.parentNode.replaceChild(plain,el);el=plain;}
  if(indent && /^\d+(\.\d+)?(px|pt|em|rem)$/.test(indent))el.style.marginLeft=indent;
  if(first && /^-?\d+(\.\d+)?(px|pt|em|rem)$/.test(first))el.style.textIndent=first;
  if(cols && /^\d+$/.test(cols))el.setAttribute('colspan',cols);if(rows && /^\d+$/.test(rows))el.setAttribute('rowspan',rows);if(start && /^\d+$/.test(start))el.setAttribute('start',start);
 }
 return root.innerHTML;
}
function pasteTaskTrace(e){e=e||window.event;if(e){if(e.preventDefault)e.preventDefault();e.returnValue=false;}
 var editor=document.getElementById('progress-editor');if(editor.getAttribute('contenteditable')==='false')return false;
 var html=cleanTaskTracePaste(window.external.PasteHtml());if(!html)return false;
 editor.focus();var selection=window.getSelection(),range=selection&&selection.rangeCount?selection.getRangeAt(0):null;
 if(!range||!editor.contains(range.commonAncestorContainer)){range=document.createRange();range.selectNodeContents(editor);range.collapse(false);}
 range.deleteContents();var holder=document.createElement('div');holder.innerHTML=html;var fragment=document.createDocumentFragment(),last;while(holder.firstChild){last=holder.firstChild;fragment.appendChild(last);}range.insertNode(fragment);if(last)range.setStartAfter(last);range.collapse(true);selection.removeAllRanges();selection.addRange(range);ensureTaskTraceImageCarets();window.external.NotifyPaste();
 return false;
}
";
}
