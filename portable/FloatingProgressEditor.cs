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
using System.Windows.Forms;

internal sealed class ProgressHtmlEditor : UserControl {
    readonly WebBrowser browser = new WebBrowser { Dock = DockStyle.Fill, AllowWebBrowserDrop = false, IsWebBrowserContextMenuEnabled = false, WebBrowserShortcutsEnabled = true, ScriptErrorsSuppressed = true };
    string pendingHtml = "";
    bool ready, changing, readOnly;
    public event EventHandler HtmlChanged;

    public ProgressHtmlEditor() {
        Controls.Add(browser);
        browser.DocumentCompleted += delegate {
            if(browser.Document == null || browser.Document.GetElementById("progress-editor") == null) return;
            ready = true;
            AttachChangeEvents();
            ApplyHtml();
            SetReadOnly(readOnly);
        };
        browser.DocumentText = "<!doctype html><html><head><meta http-equiv=\"X-UA-Compatible\" content=\"IE=edge\"><style>html,body{height:100%;margin:0;background:#fff;font-family:Segoe UI,Microsoft YaHei,sans-serif;font-size:14px}#progress-editor{box-sizing:border-box;min-height:100%;padding:10px;outline:0;overflow-wrap:anywhere}#progress-editor img{display:block;max-width:96%;max-height:260px;margin:8px 0;border:2px solid transparent;cursor:pointer}#progress-editor img:hover,#progress-editor img:focus{border-color:#245ed2}p{margin:.35em 0}</style><script>function insertTaskTraceImage(src){var e=document.getElementById('progress-editor');e.focus();document.execCommand('insertImage',false,src);}</script></head><body><div id=\"progress-editor\" contenteditable=\"true\"></div></body></html>";
    }

    void AttachChangeEvents() {
        var editor = browser.Document.GetElementById("progress-editor");
        if(editor == null) return;
        EventHandler changed = delegate { if(!changing && HtmlChanged != null) HtmlChanged(this, EventArgs.Empty); };
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
        changing = false;
    }

    public string Html {
        get {
            if(!ready || browser.Document == null) return pendingHtml ?? "";
            var editor = browser.Document.GetElementById("progress-editor");
            return editor == null ? pendingHtml ?? "" : editor.InnerHtml ?? "";
        }
        set { pendingHtml = value ?? ""; ApplyHtml(); }
    }

    public int ImageCount { get { return Regex.Matches(Html ?? "", @"<img\b", RegexOptions.IgnoreCase).Count; } }

    public void InsertImage(byte[] bytes) {
        if(bytes == null || bytes.Length == 0 || !ready || browser.Document == null) return;
        browser.Document.InvokeScript("insertTaskTraceImage", new object[] { "data:image/png;base64," + Convert.ToBase64String(bytes) });
        if(HtmlChanged != null) HtmlChanged(this, EventArgs.Empty);
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
}

internal sealed partial class FloatingWindow {
    async Task<string> PrepareProgressEditorHtml(long taskId,string html) {
        string safe=ProgressSnapshotHtml(html??"",taskId);var output=new StringBuilder();int cursor=0;
        foreach(Match image in Regex.Matches(safe,@"<img\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append(safe.Substring(cursor,image.Index-cursor));cursor=image.Index+image.Length;
            string source=ReferenceAttribute(image.Value,"data-src")??ReferenceAttribute(image.Value,"src")??"";
            if(!Regex.IsMatch(source,@"^/api/v[12]/tasks/"+taskId+@"/attachments/[1-9][0-9]*$"))continue;
            try {
                byte[] bytes=await DownloadImage(source);
                output.Append("<img src=\"data:image/png;base64,").Append(Convert.ToBase64String(bytes)).Append("\" data-tasktrace-src=\"").Append(WebUtility.HtmlEncode(source.Replace("/api/v2/","/api/v1/"))).Append("\" alt=\"进展图片\">");
            } catch { output.Append("<p>图片加载失败，请保存其他内容后重试。</p>"); }
        }
        output.Append(safe.Substring(cursor));return output.ToString();
    }

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

    async Task<string> PersistProgressEditorImages(long taskId,string html) {
        var output=new StringBuilder();int cursor=0;
        foreach(Match image in Regex.Matches(html??"",@"<img\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append((html??"").Substring(cursor,image.Index-cursor));cursor=image.Index+image.Length;
            string retained=ReferenceAttribute(image.Value,"data-tasktrace-src")??"";
            if(Regex.IsMatch(retained,@"^/api/v[12]/tasks/"+taskId+@"/attachments/[1-9][0-9]*$")) {
                output.Append("<img src=\"").Append(WebUtility.HtmlEncode(retained.Replace("/api/v2/","/api/v1/"))).Append("\" alt=\"进展图片\">");continue;
            }
            string source=ReferenceAttribute(image.Value,"src")??"";
            var dataImage=Regex.Match(source,@"^data:image/(?<type>png|jpe?g|gif|bmp);base64,(?<data>[A-Za-z0-9+/=\s]+)$",RegexOptions.IgnoreCase);
            if(dataImage.Success) {
                string extension=dataImage.Groups["type"].Value.ToLowerInvariant();if(extension=="jpeg")extension="jpg";
                long attachmentId=await UploadProgressEditorImage(taskId,Convert.FromBase64String(Regex.Replace(dataImage.Groups["data"].Value,@"\s","")),extension);
                output.Append("<img src=\"/api/v1/tasks/").Append(taskId).Append("/attachments/").Append(attachmentId).Append("\" alt=\"进展图片\">");
            }
        }
        output.Append((html??"").Substring(cursor));return ProgressSnapshotHtml(output.ToString(),taskId);
    }
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
