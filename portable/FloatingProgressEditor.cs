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

// Task names have their own saved baseline; editing a name must not create progress.
internal sealed class TaskNameEditor : UserControl {
    internal readonly TextBox Input = new TextBox {Dock=DockStyle.Fill,MaxLength=250,AccessibleName="任务名",AccessibleDescription="修改任务名后点击保存任务名"};
    internal readonly Button SaveButton = new Button {Text="保存任务名",Dock=DockStyle.Fill,AccessibleName="保存任务名"};
    readonly Func<string,string,Task<string>> write;
    readonly Action<string> feedback;
    internal Func<bool> CanSave;
    internal string SavedTitle {get;private set;}
    internal bool Saving {get;private set;}
    internal bool Dirty {get{return !String.Equals(Input.Text.Trim(),SavedTitle.Trim(),StringComparison.Ordinal);}}
    internal event EventHandler Saved;
    internal TaskNameEditor(string title,Func<string,string,Task<string>> write,Action<string> feedback) {
        this.write=write;this.feedback=feedback;SavedTitle=title??"";
        Dock=DockStyle.Fill;Margin=Padding.Empty;Padding=new Padding(14,8,14,0);
        var row=new TableLayoutPanel{Dock=DockStyle.Fill,ColumnCount=2,RowCount=2,Margin=Padding.Empty};
        row.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));row.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,112));
        row.RowStyles.Add(new RowStyle(SizeType.Absolute,20));row.RowStyles.Add(new RowStyle(SizeType.Percent,100));
        var label=new Label{Text="任务名",Dock=DockStyle.Fill,TextAlign=System.Drawing.ContentAlignment.MiddleLeft,AccessibleName="任务名说明"};
        row.Controls.Add(label,0,0);row.SetColumnSpan(label,2);row.Controls.Add(Input,0,1);row.Controls.Add(SaveButton,1,1);Controls.Add(row);
        Input.Text=SavedTitle;SaveButton.Enabled=false;
        Input.TextChanged+=delegate{SaveButton.Enabled=!Saving && Dirty;};
        Input.KeyDown+=delegate(object sender,KeyEventArgs e){if(e.KeyCode==Keys.Enter){e.SuppressKeyPress=true;SaveButton.PerformClick();}};
        SaveButton.Click+=async delegate{await Save();};
    }
    internal void Discard() {if(Saving)return;Input.Text=SavedTitle;SaveButton.Enabled=false;}
    internal async Task<bool> Save() {
        if(Saving)return false;
        if(!Dirty)return true;
        string title=Input.Text.Trim();
        if(title.Length==0){feedback("任务名不能为空，请填写后再保存。");Input.Focus();return false;}
        if(CanSave!=null && !CanSave()){feedback("正在保存其他内容，请稍后再保存任务名。");return false;}
        Saving=true;Input.ReadOnly=true;SaveButton.Enabled=false;
        try {
            SavedTitle=await write(SavedTitle,title);Input.Text=SavedTitle;
            feedback("任务名已保存。");var handler=Saved;if(handler!=null)handler(this,EventArgs.Empty);
            return true;
        } catch(Exception error){feedback("任务名未保存："+error.Message);return false;}
        finally{Saving=false;if(!IsDisposed){Input.ReadOnly=false;SaveButton.Enabled=Dirty;}}
    }
}

[ComVisible(true)]
public sealed class ProgressHtmlEditorBridge {
    internal Action<string,string> ImageDoubleClicked;
    internal Action Pasted;
    public void NotifyPaste(){var callback=Pasted;if(callback!=null)callback();}
    internal Func<string> ReadClipboardHtml;
    internal Action<string> ClipboardError;
    internal Func<Task<string>> ReadClipboardHtmlAsync;
    internal Action<int,string> PasteCompleted;
    public bool UsesAsyncClipboard(){return ReadClipboardHtmlAsync!=null;}
    public async void RequestPasteHtml(int id){string html="";try{html=await ReadClipboardHtmlAsync();}catch(Exception error){if(ClipboardError!=null)ClipboardError(error.Message);}finally{if(PasteCompleted!=null)PasteCompleted(id,html);}}
    public string PasteHtml(){try{return ReadClipboardHtml();}catch(Exception error){if(ClipboardError!=null)ClipboardError(error.Message);return "";}}
    internal Func<string,string,bool> WriteClipboardHtml;
    public bool CopyHtml(string html,string text){try{if(WriteClipboardHtml!=null && WriteClipboardHtml(html,text))return true;if(ClipboardError!=null)ClipboardError("复制失败，剪贴板可能被其他程序占用，请重试。");}catch(Exception error){if(ClipboardError!=null)ClipboardError("复制失败："+error.Message);}return false;}
    public void OpenImage(string source,string retainedSource) {
        var callback=ImageDoubleClicked;if(callback!=null)callback(source??"",retainedSource??"");
    }
}

internal sealed class ProgressClipboardErrorEventArgs : EventArgs {public readonly string Message;public ProgressClipboardErrorEventArgs(string message){Message=message;}}

internal sealed class ProgressEditorImageEventArgs : EventArgs {
    public readonly string Source,RetainedSource;
    public ProgressEditorImageEventArgs(string source,string retainedSource) {Source=source??"";RetainedSource=retainedSource??"";}
}

internal sealed class ProgressEditorBrowser : WebBrowser {
    internal Func<Keys,bool> ClipboardShortcut;
    protected override bool ProcessCmdKey(ref Message message,Keys keyData) {
        return (ClipboardShortcut!=null && ClipboardShortcut(keyData)) || base.ProcessCmdKey(ref message,keyData);
    }
    internal bool ShortcutForTest(Keys keyData){var message=new Message();return ProcessCmdKey(ref message,keyData);}
}

internal sealed class ProgressHtmlEditor : UserControl {
    readonly ProgressEditorBrowser browser = new ProgressEditorBrowser { Dock = DockStyle.Fill, AllowWebBrowserDrop = false, IsWebBrowserContextMenuEnabled = false, WebBrowserShortcutsEnabled = true, ScriptErrorsSuppressed = true };
    readonly ContextMenuStrip clipboardMenu=new ContextMenuStrip();
    readonly ToolStripSeparator imageCopySeparator=new ToolStripSeparator();
    readonly ToolStripMenuItem copyImageMenu=new ToolStripMenuItem("复制图片");
    readonly ToolStripMenuItem copyTextMenu=new ToolStripMenuItem("复制");
    readonly ToolStripMenuItem cutTextMenu=new ToolStripMenuItem("剪切");
    readonly ToolStripMenuItem pasteTextMenu=new ToolStripMenuItem("粘贴");
    string contextImageHtml="";
    readonly ProgressHtmlEditorBridge bridge = new ProgressHtmlEditorBridge();
    string pendingHtml = "";
    string observedHtml = "";
    int changeVersion;
    int imageDoubleClickCount;
    bool ready, changing, readOnly;
    public event EventHandler HtmlChanged;
    internal event EventHandler<ProgressClipboardErrorEventArgs> ClipboardFailed;
    public event EventHandler<ProgressEditorImageEventArgs> ImageDoubleClicked;

    public ProgressHtmlEditor():this(FloatingClipboard.HtmlAsync,true) {}
    internal ProgressHtmlEditor(Func<Task<string>> readClipboardHtmlAsync,bool asyncClipboard):this(new Func<string>(()=>"")){bridge.ReadClipboardHtmlAsync=asyncClipboard?readClipboardHtmlAsync:null;}
    internal ProgressHtmlEditor(Func<string> readClipboardHtml):this(readClipboardHtml,FloatingClipboard.Copy) {}
    internal ProgressHtmlEditor(Func<string> readClipboardHtml,Func<string,string,bool> writeClipboardHtml) {
        bridge.ReadClipboardHtml=readClipboardHtml;bridge.WriteClipboardHtml=writeClipboardHtml;
        bridge.PasteCompleted=delegate(int id,string html){if(!IsDisposed && IsHandleCreated)BeginInvoke(new Action(delegate{if(!IsDisposed && ready && browser.Document!=null)browser.Document.InvokeScript("completeTaskTracePaste",new object[]{id,html});}));};
        bridge.ClipboardError=delegate(string message){if(!IsDisposed && IsHandleCreated)BeginInvoke(new Action(delegate{var handler=ClipboardFailed;if(!IsDisposed && handler!=null)handler(this,new ProgressClipboardErrorEventArgs(message));}));};
        bridge.ImageDoubleClicked=QueueImageDoubleClick;bridge.Pasted=delegate{if(!IsDisposed && IsHandleCreated)BeginInvoke(new Action(NotifyChanged));};browser.ObjectForScripting=bridge;
        browser.ClipboardShortcut=HandleClipboardShortcut;
        Controls.Add(browser);
        clipboardMenu.Items.AddRange(new ToolStripItem[]{copyImageMenu,imageCopySeparator,copyTextMenu,cutTextMenu,pasteTextMenu});
        clipboardMenu.Items.Add("全选",null,delegate{browser.Document.InvokeScript("selectAllTaskTrace");});
        copyImageMenu.Click+=delegate{if(contextImageHtml!="")bridge.CopyHtml(contextImageHtml,"");};
        copyTextMenu.Click+=delegate{browser.Document.InvokeScript("copyTaskTrace",new object[]{null,false});};
        cutTextMenu.Click+=delegate{browser.Document.InvokeScript("copyTaskTrace",new object[]{null,true});};
        pasteTextMenu.Click+=delegate{browser.Document.InvokeScript("pasteTaskTrace",new object[]{null});};
        Disposed+=delegate{clipboardMenu.Dispose();};
        browser.Navigating+=delegate(object sender,WebBrowserNavigatingEventArgs e){if(ready)e.Cancel=true;};
        browser.DocumentCompleted += delegate {
            if(browser.Document == null || browser.Document.GetElementById("progress-editor") == null) return;
            ready = true;
            browser.Document.ContextMenuShowing+=delegate(object sender,HtmlElementEventArgs e){
                e.ReturnValue=false;browser.Document.InvokeScript("rememberTaskTraceRange");
                SetContextImage(browser.Document.GetElementFromPoint(e.ClientMousePosition));
                bool selected=Convert.ToBoolean(browser.Document.InvokeScript("taskTraceHasSelection"));
                copyTextMenu.Enabled=selected;cutTextMenu.Enabled=selected&&!readOnly;pasteTextMenu.Enabled=!readOnly;
                var point=System.Windows.Forms.Cursor.Position;BeginInvoke(new Action(delegate{if(!IsDisposed)clipboardMenu.Show(point);}));
            };
            AttachChangeEvents();
            ApplyHtml();
            SetReadOnly(readOnly);
        };
        browser.DocumentText = "<!doctype html><html><head><meta charset=\"utf-8\"><meta http-equiv=\"X-UA-Compatible\" content=\"IE=edge\"><style>html,body{height:100%;margin:0;background:#fff;font-family:Segoe UI,Microsoft YaHei,sans-serif;font-size:14px}#progress-editor{box-sizing:border-box;min-height:100%;padding:10px;outline:0;overflow-wrap:anywhere}#progress-editor img{display:inline-block;max-width:96%;max-height:260px;margin:8px 2px;border:2px solid transparent;cursor:pointer;vertical-align:middle}#progress-editor img:hover,#progress-editor img:focus{border-color:#245ed2}p{margin:.35em 0}</style><script>var taskTraceCaret='\u200b';function taskTraceOpenImage(e){e=e||window.event;var image=e.srcElement||e.target;if(!image||String(image.tagName).toLowerCase()!=='img')return true;window.external.OpenImage(image.src||'',image.getAttribute('data-tasktrace-src')||'');if(e.preventDefault)e.preventDefault();e.returnValue=false;return false;}function taskTraceTestOpenFirstImage(){var images=document.getElementById('progress-editor').getElementsByTagName('img');if(!images.length)return false;var image=images[0];if(image.fireEvent)image.fireEvent('ondblclick');else{var e=document.createEvent('MouseEvents');e.initMouseEvent('dblclick',true,true,window,2,0,0,0,0,false,false,false,false,0,null);image.dispatchEvent(e);}return true;}function ensureTaskTraceImageCarets(){var e=document.getElementById('progress-editor'),images=e.getElementsByTagName('img');for(var i=0;i<images.length;i++){var image=images[i];if(!image.previousSibling||image.previousSibling.nodeName==='IMG')image.parentNode.insertBefore(document.createTextNode(taskTraceCaret),image);if(!image.nextSibling||image.nextSibling.nodeName==='IMG')image.parentNode.insertBefore(document.createTextNode(taskTraceCaret),image.nextSibling);}}function insertTaskTraceImage(src){var range=taskTraceRange()||taskTraceSavedRange;insertTaskTraceHtml('<img src=\"'+src+'\">',range);}</script></head><body><div id=\"progress-editor\" contenteditable=\"true\" ondblclick=\"return taskTraceOpenImage(event)\"></div></body></html>".Replace("</head>","<script>"+FloatingClipboard.Script+"</script></head>").Replace("contenteditable=\"true\"", "contenteditable=\"true\" onpaste=\"return pasteTaskTrace(event)\" onmousedown=\"return selectTaskTraceImage(event)\" onmouseup=\"rememberTaskTraceRange()\" onkeyup=\"rememberTaskTraceRange()\"");
    }

    bool HandleClipboardShortcut(Keys keyData) {
        if(!ready || browser.Document==null)return false;
        if(keyData==(Keys.Control|Keys.A) || keyData==(Keys.Alt|Keys.A)){browser.Document.InvokeScript("selectAllTaskTrace");return true;}
        if(keyData==(Keys.Control|Keys.C) || keyData==(Keys.Control|Keys.X))return !Convert.ToBoolean(browser.Document.InvokeScript("copyTaskTrace",new object[]{null,keyData==(Keys.Control|Keys.X)}));
        return false;
    }

    void SetContextImage(HtmlElement element) {
        contextImageHtml="";
        if(element!=null && String.Equals(element.TagName,"IMG",StringComparison.OrdinalIgnoreCase)) {
            string source=element.GetAttribute("src");
            if(source.StartsWith("data:image/",StringComparison.OrdinalIgnoreCase))contextImageHtml="<img src=\""+WebUtility.HtmlEncode(source)+"\">";
        }
        imageCopySeparator.Visible=copyImageMenu.Visible=copyImageMenu.Enabled=contextImageHtml!="";
    }
    internal bool CopyFirstImageForTest(){if(!ready || browser.Document.Images.Count==0)return false;SetContextImage(browser.Document.Images[0]);if(!copyImageMenu.Enabled)return false;copyImageMenu.PerformClick();return true;}

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
        browser.Document.InvokeScript("eval",new object[]{"taskTraceSavedRange=null;resetTaskTracePastes();window.getSelection().removeAllRanges();"});
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

    internal bool IsPastePending {get{return ready && browser.Document!=null && Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"taskTracePasteOrder.length>0"}));}}
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
    internal static async Task<string> WriteTaskName(long id,string original,string title,Func<string,string,object,Task<Dictionary<string,object>>> request) {
        var latest=await request("GET","/tasks/"+id,null);
        string current=Convert.ToString(latest["title"]);
        if(String.Equals(current,title,StringComparison.Ordinal))return current;
        if(!String.Equals(current,original,StringComparison.Ordinal))throw new Exception("任务名已在其他窗口或由协作成员修改，请重新打开任务后再编辑。");
        // Patch only the name, preserving progress, description and outstanding items.
        var saved=await request("PATCH","/tasks/"+id,new{title=title});
        return saved.ContainsKey("title")?Convert.ToString(saved["title"]):title;
    }

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
        safe=Regex.Replace(safe,@"\s+(?:on[a-z0-9_-]+)\s*=\s*(?:""[^""]*""|'[^']*'|[^\s>]+)","",RegexOptions.IgnoreCase);
        safe=Regex.Replace(safe,@"\s+(href|src)\s*=\s*([""'])\s*javascript:[\s\S]*?\2","",RegexOptions.IgnoreCase);
        safe=Regex.Replace(safe,@"\s+style\s*=\s*(?:""([^""]*)""|'([^']*)'|([^\s>]+))",delegate(Match match){return SafeIndentStyle(match.Groups[1].Value+match.Groups[2].Value+match.Groups[3].Value);},RegexOptions.IgnoreCase);
        return safe;
    }

    static string SafeIndentStyle(string style) {
        var values=Regex.Matches(style??"",@"(?:^|;)\s*(margin-left|padding-left|text-indent)\s*:\s*(-?\d+(?:\.\d+)?(?:px|pt|em|rem))\s*(?=;|$)",RegexOptions.IgnoreCase).Cast<Match>().Select(match=>match.Groups[1].Value.ToLowerInvariant()+":"+match.Groups[2].Value.ToLowerInvariant());
        string safe=String.Join(";",values);return safe==""?"":" style=\""+safe+"\"";
    }
    async Task<string> PrepareInlineEditorHtml(long taskId,string html,bool description,bool allowOtherTaskAttachments=false) {
        string safe=description?TaskDescriptionEditorHtml(html):ProgressSnapshotHtml(html??"",taskId);var output=new StringBuilder();int cursor=0;
        foreach(Match image in Regex.Matches(safe,@"<img\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append(safe.Substring(cursor,image.Index-cursor));cursor=image.Index+image.Length;
            string source=ReferenceAttribute(image.Value,"data-src")??ReferenceAttribute(image.Value,"src")??"";
            string retained=AttachmentPath(source);if(retained==null || (!allowOtherTaskAttachments && !Regex.IsMatch(retained,@"^/api/v2/tasks/"+taskId+@"/attachments/[1-9][0-9]*$")))continue;
            try {
                byte[] bytes=await DownloadImage(retained);
                output.Append("<img src=\"data:image/png;base64,").Append(Convert.ToBase64String(bytes)).Append("\" data-tasktrace-src=\"").Append(WebUtility.HtmlEncode(retained.Replace("/api/v2/","/api/v1/"))).Append("\" alt=\"").Append(description?"任务描述图片":"进展图片").Append("\">");
            } catch { output.Append("<p>图片加载失败，请保存其他内容后重试。</p>"); }
        }
        output.Append(safe.Substring(cursor));return await PrepareInlineEditorFiles(output.ToString());
    }

    Task<string> PrepareProgressEditorHtml(long taskId,string html) {return PrepareInlineEditorHtml(taskId,html,false);}
    Task<string> PrepareTaskDescriptionEditorHtml(long taskId,string html) {return PrepareInlineEditorHtml(taskId,html,true);}
    Task<string> PrepareOutstandingNoteEditorHtml(long taskId,string html) {return PrepareInlineEditorHtml(taskId,html,true,true);}

    Task<long> UploadProgressEditorImage(long taskId,byte[] bytes,string extension) {
        return UploadEditorFile(taskId,bytes,"tasktrace-progress-"+Guid.NewGuid().ToString("N")+"."+extension,extension=="jpg"?"image/jpeg":"image/"+extension);
    }

    async Task<string> PersistInlineEditorImages(long taskId,string html,bool description) {
        html=await PersistInlineEditorFiles(taskId,html??"");
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
    Task<string> PersistOutstandingNoteEditorImages(long taskId,string html) {return PersistInlineEditorImages(taskId,html,true);}
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
