// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Text.RegularExpressions;
using System.Runtime.InteropServices;
using System.Windows.Forms;

[ComVisible(true)]
public sealed class ProgressHtmlEditorBridge {
    internal Action<string,string> ImageDoubleClicked;
    public void OpenImage(string source,string retainedSource) {
        var callback=ImageDoubleClicked;if(callback!=null)callback(source??"",retainedSource??"");
    }
}

internal sealed class ProgressEditorImageEventArgs : EventArgs {
    public readonly string Source,RetainedSource;
    public ProgressEditorImageEventArgs(string source,string retainedSource) {Source=source??"";RetainedSource=retainedSource??"";}
}

internal sealed class ProgressHtmlEditor : UserControl {
    readonly WebBrowser browser = new WebBrowser { Dock = DockStyle.Fill, AllowWebBrowserDrop = false, IsWebBrowserContextMenuEnabled = false, WebBrowserShortcutsEnabled = true, ScriptErrorsSuppressed = true };
    readonly ProgressHtmlEditorBridge bridge = new ProgressHtmlEditorBridge();
    string pendingHtml = "";
    string observedHtml = "";
    int changeVersion;
    int imageDoubleClickCount;
    bool ready, changing, readOnly;
    public event EventHandler HtmlChanged;
    public event EventHandler<ProgressEditorImageEventArgs> ImageDoubleClicked;

    public ProgressHtmlEditor() {
        bridge.ImageDoubleClicked=QueueImageDoubleClick;browser.ObjectForScripting=bridge;
        Controls.Add(browser);
        browser.DocumentCompleted += delegate {
            if(browser.Document == null || browser.Document.GetElementById("progress-editor") == null) return;
            ready = true;
            AttachChangeEvents();
            ApplyHtml();
            SetReadOnly(readOnly);
        };
        browser.DocumentText = "<!doctype html><html><head><meta http-equiv=\"X-UA-Compatible\" content=\"IE=edge\"><style>html,body{height:100%;margin:0;background:#fff;font-family:Segoe UI,Microsoft YaHei,sans-serif;font-size:14px}#progress-editor{box-sizing:border-box;min-height:100%;padding:10px;outline:0;overflow-wrap:anywhere}#progress-editor img{display:inline-block;max-width:96%;max-height:260px;margin:8px 2px;border:2px solid transparent;cursor:pointer;vertical-align:middle}#progress-editor img:hover,#progress-editor img:focus{border-color:#245ed2}p{margin:.35em 0}</style><script>var taskTraceCaret='\u200b';function taskTraceOpenImage(e){e=e||window.event;var image=e.srcElement||e.target;if(!image||String(image.tagName).toLowerCase()!=='img')return true;window.external.OpenImage(image.src||'',image.getAttribute('data-tasktrace-src')||'');if(e.preventDefault)e.preventDefault();e.returnValue=false;return false;}function taskTraceTestOpenFirstImage(){var images=document.getElementById('progress-editor').getElementsByTagName('img');if(!images.length)return false;var image=images[0];if(image.fireEvent)image.fireEvent('ondblclick');else{var e=document.createEvent('MouseEvents');e.initMouseEvent('dblclick',true,true,window,2,0,0,0,0,false,false,false,false,0,null);image.dispatchEvent(e);}return true;}function ensureTaskTraceImageCarets(){var e=document.getElementById('progress-editor'),images=e.getElementsByTagName('img');for(var i=0;i<images.length;i++){var image=images[i];if(!image.previousSibling||image.previousSibling.nodeName==='IMG')image.parentNode.insertBefore(document.createTextNode(taskTraceCaret),image);if(!image.nextSibling||image.nextSibling.nodeName==='IMG')image.parentNode.insertBefore(document.createTextNode(taskTraceCaret),image.nextSibling);}}function insertTaskTraceImage(src){var e=document.getElementById('progress-editor');e.focus();var selection=window.getSelection(),range;if(selection&&selection.rangeCount&&e.contains(selection.anchorNode)){range=selection.getRangeAt(0);}else{range=document.createRange();range.selectNodeContents(e);range.collapse(false);}range.deleteContents();var image=document.createElement('img'),after=document.createTextNode(taskTraceCaret);image.src=src;range.insertNode(after);range.insertNode(image);ensureTaskTraceImageCarets();range.setStartAfter(after);range.collapse(true);selection.removeAllRanges();selection.addRange(range);}</script></head><body><div id=\"progress-editor\" contenteditable=\"true\" ondblclick=\"return taskTraceOpenImage(event)\"></div></body></html>";
    }

    void QueueImageDoubleClick(string source,string retainedSource) {
        if(IsDisposed || !IsHandleCreated)return;
        BeginInvoke(new Action(delegate {
            if(IsDisposed)return;imageDoubleClickCount++;var handler=ImageDoubleClicked;
            if(handler!=null)handler(this,new ProgressEditorImageEventArgs(source,retainedSource));
        }));
    }

    static string CleanHtml(string value) {
        return Regex.Replace((value ?? "").Replace("\u200b", ""), @"&#(?:8203|x200b);", "", RegexOptions.IgnoreCase);
    }

    void AttachChangeEvents() {
        var editor = browser.Document.GetElementById("progress-editor");
        if(editor == null) return;
        EventHandler changed = delegate { NotifyChanged(); };
        editor.AttachEventHandler("oninput", changed);
        editor.AttachEventHandler("onkeyup", changed);
        editor.AttachEventHandler("oncut", changed);
        editor.AttachEventHandler("onpaste", changed);
    }

    void ApplyHtml() {
        if(!ready || browser.Document == null) return;
        var editor = browser.Document.GetElementById("progress-editor");
        if(editor == null) return;
        changing = true;
        editor.InnerHtml = pendingHtml ?? "";
        browser.Document.InvokeScript("ensureTaskTraceImageCarets");
        observedHtml = CleanHtml(editor.InnerHtml);
        changing = false;
    }

    void NotifyChanged() {
        if(changing || !ready || browser.Document == null)return;
        var editor=browser.Document.GetElementById("progress-editor");
        string current=editor==null?CleanHtml(pendingHtml):CleanHtml(editor.InnerHtml);
        if(String.Equals(current,observedHtml,StringComparison.Ordinal))return;
        observedHtml=current;changeVersion++;
        if(HtmlChanged != null)HtmlChanged(this,EventArgs.Empty);
    }

    public string Html {
        get {
            if(!ready || browser.Document == null) return pendingHtml ?? "";
            var editor = browser.Document.GetElementById("progress-editor");
            return editor == null ? CleanHtml(pendingHtml) : CleanHtml(editor.InnerHtml);
        }
        set { pendingHtml = value ?? ""; ApplyHtml(); }
    }

    public int ImageCount { get { return Regex.Matches(Html ?? "", @"<img\b", RegexOptions.IgnoreCase).Count; } }
    public int ChangeVersion { get { return changeVersion; } }
    internal int ImageDoubleClickCountForTest { get { return imageDoubleClickCount; } }

    public void InsertImage(byte[] bytes) {
        if(bytes == null || bytes.Length == 0 || !ready || browser.Document == null) return;
        browser.Document.InvokeScript("insertTaskTraceImage", new object[] { "data:image/png;base64," + Convert.ToBase64String(bytes) });
        NotifyChanged();
    }

    public void SetReadOnly(bool value) {
        readOnly = value;
        if(!ready || browser.Document == null) return;
        var editor = browser.Document.GetElementById("progress-editor");
        if(editor != null) editor.SetAttribute("contentEditable", value ? "false" : "true");
    }

    public void FocusEditor() {
        if(ready && browser.Document != null) {
            var editor = browser.Document.GetElementById("progress-editor");
            if(editor != null) editor.Focus();
        }
        browser.Focus();
    }

    internal bool SimulateFirstImageDoubleClickForTest() {
        if(!ready || browser.Document==null)return false;
        try{return Convert.ToBoolean(browser.Document.InvokeScript("taskTraceTestOpenFirstImage"));}catch{return false;}
    }

    internal bool SimulateUserHtmlForTest(string value) {
        if(!ready || browser.Document==null)return false;
        var editor=browser.Document.GetElementById("progress-editor");if(editor==null)return false;
        editor.InnerHtml=value??"";NotifyChanged();return true;
    }
}

internal sealed partial class FloatingWindow {
    GalleryImage ProgressEditorGalleryImage(ProgressEditorImageEventArgs image,string caption) {
        foreach(string raw in new[]{image.RetainedSource,image.Source}.Where(value=>!String.IsNullOrWhiteSpace(value)).Distinct()) {
            string path=AttachmentPath(raw);if(path!=null)return new GalleryImage{Source=path,Caption=caption};
            var dataImage=Regex.Match(raw,@"^data:image/(?:png|jpe?g|gif|bmp);base64,(?<data>[A-Za-z0-9+/=\s]+)$",RegexOptions.IgnoreCase);
            if(dataImage.Success)try{return new GalleryImage{Bytes=Convert.FromBase64String(Regex.Replace(dataImage.Groups["data"].Value,@"\s","")),Caption=caption};}catch{}
        }
        return null;
    }

    Task ShowProgressEditorImage(ProgressEditorImageEventArgs image,Form owner,string caption) {
        var galleryImage=ProgressEditorGalleryImage(image,caption);
        if(galleryImage==null)throw new Exception("图片无法读取，请保存其他内容后重试。");
        return ShowImagePreview(galleryImage,owner);
    }

    static string TaskDescriptionEditorHtml(string html) {
        string safe=html??"";
        safe=Regex.Replace(safe,@"<(script|style|iframe|object|embed|svg|math|template|noscript|textarea|select|button|form)\b[^>]*>.*?</\1\s*>","",RegexOptions.IgnoreCase|RegexOptions.Singleline);
        safe=Regex.Replace(safe,@"</?(?:script|style|iframe|object|embed|svg|math|template|noscript|textarea|select|button|form|input)\b[^>]*>","",RegexOptions.IgnoreCase|RegexOptions.Singleline);
        safe=Regex.Replace(safe,@"\s+(?:on[a-z0-9_-]+|style)\s*=\s*(?:""[^""]*""|'[^']*'|[^\s>]+)","",RegexOptions.IgnoreCase);
        safe=Regex.Replace(safe,@"\s+(href|src)\s*=\s*([""'])\s*javascript:[\s\S]*?\2","",RegexOptions.IgnoreCase);
        return safe;
    }

    async Task<string> PrepareInlineEditorHtml(long taskId,string html,bool description) {
        string safe=description?TaskDescriptionEditorHtml(html):ProgressSnapshotHtml(html??"",taskId);var output=new StringBuilder();int cursor=0;
        foreach(Match image in Regex.Matches(safe,@"<img\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append(safe.Substring(cursor,image.Index-cursor));cursor=image.Index+image.Length;
            string source=ReferenceAttribute(image.Value,"data-src")??ReferenceAttribute(image.Value,"src")??"";
            string retained=AttachmentPath(source);if(retained==null || !Regex.IsMatch(retained,@"^/api/v2/tasks/"+taskId+@"/attachments/[1-9][0-9]*$"))continue;
            try {
                byte[] bytes=await DownloadImage(retained);
                output.Append("<img src=\"data:image/png;base64,").Append(Convert.ToBase64String(bytes)).Append("\" data-tasktrace-src=\"").Append(WebUtility.HtmlEncode(retained.Replace("/api/v2/","/api/v1/"))).Append("\" alt=\"").Append(description?"任务描述图片":"进展图片").Append("\">");
            } catch { output.Append("<p>图片加载失败，请保存其他内容后重试。</p>"); }
        }
        output.Append(safe.Substring(cursor));return output.ToString();
    }

    Task<string> PrepareProgressEditorHtml(long taskId,string html) {return PrepareInlineEditorHtml(taskId,html,false);}
    Task<string> PrepareTaskDescriptionEditorHtml(long taskId,string html) {return PrepareInlineEditorHtml(taskId,html,true);}

    async Task<long> UploadProgressEditorImage(long taskId,byte[] bytes,string extension) {
        await Api("GET","/tasks/"+taskId,null);
        using(var body=new MultipartFormDataContent()) {
            var image=new ByteArrayContent(bytes);image.Headers.ContentType=new System.Net.Http.Headers.MediaTypeHeaderValue(extension=="jpg"?"image/jpeg":"image/"+extension);
            body.Add(image,"files","tasktrace-progress-"+Guid.NewGuid().ToString("N")+"."+extension);
            using(var request=new HttpRequestMessage(HttpMethod.Post,url+"/api/v2/tasks/"+taskId+"/attachments")) {
                request.Headers.Authorization=new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer",token);request.Content=body;
                using(var response=await http.SendAsync(request)) {
                    if(!response.IsSuccessStatusCode)throw new Exception("进展图片上传失败，请重试。");
                    var result=ReadObject(await response.Content.ReadAsStringAsync());
                    foreach(Dictionary<string,object> attached in (IEnumerable)result["success"])return Convert.ToInt64(attached["id"]);
                }
            }
        }
        throw new Exception("进展图片上传失败，请重试。");
    }

    async Task<string> PersistInlineEditorImages(long taskId,string html,bool description) {
        var output=new StringBuilder();int cursor=0;
        foreach(Match image in Regex.Matches(html??"",@"<img\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append((html??"").Substring(cursor,image.Index-cursor));cursor=image.Index+image.Length;
            string retained=ReferenceAttribute(image.Value,"data-tasktrace-src")??"";
            if(Regex.IsMatch(retained,@"^/api/v[12]/tasks/"+taskId+@"/attachments/[1-9][0-9]*$")) {
                output.Append("<img src=\"").Append(WebUtility.HtmlEncode(retained.Replace("/api/v2/","/api/v1/"))).Append("\" alt=\"").Append(description?"任务描述图片":"进展图片").Append("\">");continue;
            }
            string source=ReferenceAttribute(image.Value,"src")??"";
            var dataImage=Regex.Match(source,@"^data:image/(?<type>png|jpe?g|gif|bmp);base64,(?<data>[A-Za-z0-9+/=\s]+)$",RegexOptions.IgnoreCase);
            if(dataImage.Success) {
                string extension=dataImage.Groups["type"].Value.ToLowerInvariant();if(extension=="jpeg")extension="jpg";
                long attachmentId=await UploadProgressEditorImage(taskId,Convert.FromBase64String(Regex.Replace(dataImage.Groups["data"].Value,@"\s","")),extension);
                output.Append("<img src=\"/api/v1/tasks/").Append(taskId).Append("/attachments/").Append(attachmentId).Append("\" alt=\"").Append(description?"任务描述图片":"进展图片").Append("\">");
            }
        }
        output.Append((html??"").Substring(cursor));string saved=output.ToString();return description?TaskDescriptionEditorHtml(saved):ProgressSnapshotHtml(saved,taskId);
    }

    Task<string> PersistProgressEditorImages(long taskId,string html) {return PersistInlineEditorImages(taskId,html,false);}
    Task<string> PersistTaskDescriptionEditorImages(long taskId,string html) {return PersistInlineEditorImages(taskId,html,true);}
}

internal sealed partial class FloatingWindow {
    sealed class FloatingTeamMarker { public string id; public string author; }
    static FloatingTeamMarker ReadFloatingTeamMarker(string html) {
        var match=Regex.Match(html??"",@"<!--tasktrace-team:([A-Za-z0-9_-]+)-->");if(!match.Success)return null;
        try {string encoded=match.Groups[1].Value.Replace('-','+').Replace('_','/');encoded+=new string('=',(4-encoded.Length%4)%4);return new JavaScriptSerializer().Deserialize<FloatingTeamMarker>(Encoding.UTF8.GetString(Convert.FromBase64String(encoded)));}catch{return null;}
    }
    static string FloatingTeamMarkerHtml(string id,string author) {
        string value=new JavaScriptSerializer().Serialize(new FloatingTeamMarker{id=id,author=author??""});return "<!--tasktrace-team:"+Convert.ToBase64String(Encoding.UTF8.GetBytes(value)).TrimEnd('=').Replace('+','-').Replace('/','_')+"-->";
    }
    static List<string> TeamMergedIds(string html) {
        var match=Regex.Match(html??"","data-tasktrace-team-merged=\"([^\"]*)\"");return match.Success?match.Groups[1].Value.Split(',').Select(value=>value.Trim()).Where(value=>value!="").ToList():new List<string>();
    }
}
