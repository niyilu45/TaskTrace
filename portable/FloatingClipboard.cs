// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.IO;
using System.Net;
using System.Text.RegularExpressions;
using System.Windows.Forms;

internal static class FloatingClipboard {
    internal static string Html() {
        try {
            string html=Clipboard.ContainsText(TextDataFormat.Html)?Clipboard.GetText(TextDataFormat.Html):"";
            if(html!="") {
                var fragment=Regex.Match(html,@"<!--StartFragment-->([\s\S]*?)<!--EndFragment-->",RegexOptions.IgnoreCase);
                if(fragment.Success)html=fragment.Groups[1].Value;
                else {int start=html.IndexOf('<');if(start>=0)html=html.Substring(start);}
                if(Clipboard.ContainsImage())html=Regex.Replace(html,@"<img\b[^>]*>",delegate(Match match){return Regex.IsMatch(match.Value,@"(?:data:image/|/tasks/\d+/attachments/)",RegexOptions.IgnoreCase)?match.Value:ImageHtml();},RegexOptions.IgnoreCase);
                return html;
            }
            if(Clipboard.ContainsText())return "<p>"+WebUtility.HtmlEncode(Clipboard.GetText()).Replace("\r\n","\n").Replace("\r","\n").Replace("\n","<br>").Replace("\t","&emsp;")+"</p>";
            return Clipboard.ContainsImage()?ImageHtml():"";
        }catch{return "";}
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
function pasteTaskTrace(e){var editor=document.getElementById('progress-editor');if(editor.getAttribute('contenteditable')==='false')return false;e=e||window.event;
 var html=cleanTaskTracePaste(window.external.PasteHtml());if(!html)return false;
 editor.focus();var selection=window.getSelection(),range=selection&&selection.rangeCount?selection.getRangeAt(0):null;
 if(!range||!editor.contains(range.commonAncestorContainer)){range=document.createRange();range.selectNodeContents(editor);range.collapse(false);}
 range.deleteContents();var holder=document.createElement('div');holder.innerHTML=html;var fragment=document.createDocumentFragment(),last;while(holder.firstChild){last=holder.firstChild;fragment.appendChild(last);}range.insertNode(fragment);if(last)range.setStartAfter(last);range.collapse(true);selection.removeAllRanges();selection.addRange(range);ensureTaskTraceImageCarets();window.external.NotifyPaste();
 if(e.preventDefault)e.preventDefault();e.returnValue=false;return false;
}
";
}
