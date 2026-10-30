// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.IO;
using System.Net;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed class FloatingQuickMemoEditor : Form {
    readonly FloatingQuickMemoStore store;
    internal readonly ProgressHtmlEditor Editor;
    internal readonly Button SaveButton=new Button{Text="立即保存",AutoSize=true};
    internal readonly Label Feedback=new Label{AutoSize=true,MaximumSize=new Size(580,0)};
    readonly Timer saveTimer=new Timer{Interval=800};
    Task<bool> saveTask;
    int savedVersion;
    bool closeAllowed,closePending,discarded;
    internal bool IsDirty {get{return !discarded && (Editor.IsPastePending || Editor.ChangeVersion!=savedVersion);}}

    internal FloatingQuickMemoEditor(FloatingQuickMemoStore storage,Font font,Icon icon,bool topMost,ProgressHtmlEditor editor) {
        store=storage;string html=store.Read();
        Text="快捷备忘";Font=font;Icon=icon;TopMost=topMost;ShowInTaskbar=true;MinimizeBox=true;
        AutoScaleDimensions=new SizeF(96F,96F);AutoScaleMode=AutoScaleMode.Dpi;
        Size=new Size(700,620);MinimumSize=new Size(460,360);
        Editor=editor;Editor.Dock=DockStyle.Fill;Editor.AccessibleName="快捷备忘图文内容";Editor.Html=html;
        savedVersion=Editor.ChangeVersion;
        var layout=new TableLayoutPanel{Dock=DockStyle.Fill,ColumnCount=1,RowCount=3,Padding=new Padding(12)};
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        var hint=new Label{AutoSize=true,Dock=DockStyle.Fill,Padding=new Padding(0,0,0,10),Text="快捷备忘内容 · 文字和图片自动保存在本机\r\nCtrl+V 粘贴图文，Ctrl+Z 撤销；双击图片查看大图。"};
        var footer=new TableLayoutPanel{Dock=DockStyle.Fill,AutoSize=true,ColumnCount=2,RowCount=1,Padding=new Padding(0,10,0,0)};
        footer.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));footer.ColumnStyles.Add(new ColumnStyle(SizeType.AutoSize));
        Feedback.Dock=DockStyle.Fill;Feedback.Anchor=AnchorStyles.Left|AnchorStyles.Right;
        var buttons=new FlowLayoutPanel{AutoSize=true,Dock=DockStyle.Fill,WrapContents=false,FlowDirection=FlowDirection.LeftToRight};
        var close=new Button{Text="关闭",AutoSize=true};close.Click+=delegate{Close();};
        SaveButton.Click+=async delegate{await SaveAsync();};buttons.Controls.Add(SaveButton);buttons.Controls.Add(close);
        footer.Controls.Add(Feedback,0,0);footer.Controls.Add(buttons,1,0);
        layout.Controls.Add(hint,0,0);layout.Controls.Add(Editor,0,1);layout.Controls.Add(footer,0,2);Controls.Add(layout);
        SetFeedback("已载入备忘；修改后自动保存。",false);
        Editor.HtmlChanged+=delegate{if(discarded)return;SetFeedback("有修改，正在等待自动保存…",false);saveTimer.Stop();saveTimer.Start();};
        Editor.ClipboardFailed+=delegate(object sender,ProgressClipboardErrorEventArgs args){SetFeedback(args.Message,true);};
        saveTimer.Tick+=async delegate{saveTimer.Stop();await SaveAsync();};
        FormClosing+=CloseWithSave;
    }
    void SetFeedback(string message,bool error) {
        if(IsDisposed)return;Feedback.Text=message+" · "+Editor.ImageCount+" 张图片";
        Feedback.ForeColor=error?Color.Firebrick:Color.FromArgb(80,94,115);
    }
    internal Task<bool> SaveAsync() {
        if(saveTask!=null && !saveTask.IsCompleted)return saveTask;
        saveTask=SaveLoopAsync();return saveTask;
    }
    async Task<bool> SaveLoopAsync() {
        saveTimer.Stop();SaveButton.Enabled=false;
        try {
            while(!IsDisposed && IsDirty) {
                if(Editor.IsPastePending){SetFeedback("正在处理粘贴内容，请完成后再关闭或保存。",true);return false;}
                int version=Editor.ChangeVersion;string html=Editor.Html;long request=store.ReserveWrite();
                SetFeedback("正在保存…",false);
                bool written=await Task.Run(()=>store.Save(html,request));
                if(IsDisposed)return false;
                if(written && request==store.LatestVersion)savedVersion=version;
            }
            SetFeedback("已保存 "+DateTime.Now.ToString("HH:mm:ss"),false);return !IsDisposed;
        } catch(Exception error){SetFeedback("保存失败，内容仍保留在窗口中。请重试："+error.Message,true);return false;}
        finally {if(!IsDisposed)SaveButton.Enabled=true;}
    }
    internal bool SaveForExit() {
        if(discarded || !IsDirty)return true;
        if(Editor.IsPastePending){SetFeedback("图片仍在粘贴，退出已取消，请完成后重试。",true);return false;}
        try {
            int version=Editor.ChangeVersion;long request=store.ReserveWrite();
            if(!store.Save(Editor.Html,request))return false;
            savedVersion=version;SetFeedback("已保存",false);return true;
        }catch(Exception error){SetFeedback("保存失败，退出已取消。内容仍在窗口中："+error.Message,true);return false;}
    }
    internal void DiscardForUpdate(){discarded=true;saveTimer.Stop();store.ReserveWrite();}
    async void CloseWithSave(object sender,FormClosingEventArgs args) {
        if(closeAllowed || discarded)return;
        args.Cancel=true;if(closePending)return;
        if(Editor.IsPastePending){SetFeedback("图片仍在粘贴，请完成后再关闭。",true);return;}
        closePending=true;Editor.SetReadOnly(true);
        try {if(await SaveAsync()){closeAllowed=true;Close();}}
        finally {closePending=false;if(!IsDisposed)Editor.SetReadOnly(false);}
    }
    protected override void Dispose(bool disposing){if(disposing)saveTimer.Dispose();base.Dispose(disposing);}
}

internal sealed partial class FloatingWindow {
    FloatingQuickMemoEditor quickMemo;
    void InitializeQuickMemo(ContextMenuStrip menu) {
        menu.Items.Add("快捷备忘",null,async delegate{await OpenQuickMemo();});
        FormClosing+=delegate(object sender,FormClosingEventArgs args) {
            if(args.Cancel || (args.CloseReason==CloseReason.UserClosing && !allowExit) || quickMemo==null || quickMemo.IsDisposed)return;
            if(!quickMemo.SaveForExit()) {args.Cancel=true;if(args.CloseReason==CloseReason.UserClosing)allowExit=false;quickMemo.Show();quickMemo.BringToFront();quickMemo.Activate();}
        };
        Disposed+=delegate{if(quickMemo!=null && !quickMemo.IsDisposed)quickMemo.Dispose();};
    }
    async Task OpenQuickMemo() {
        if(closing || IsDisposed)return;
        if(quickMemo!=null && !quickMemo.IsDisposed) {
            if(quickMemo.WindowState==FormWindowState.Minimized)quickMemo.WindowState=FormWindowState.Normal;
            quickMemo.Show();quickMemo.BringToFront();quickMemo.Activate();return;
        }
        try {
            using(var editor=new ProgressHtmlEditor(async delegate{return await QuickMemoClipboardHtml(await FloatingClipboard.HtmlAsync(),DownloadImage);},true))
            using(var memo=new FloatingQuickMemoEditor(new FloatingQuickMemoStore(data),Font,Icon,TopMost,editor)) {
                quickMemo=memo;
                editor.ImageDoubleClicked+=async delegate(object sender,ProgressEditorImageEventArgs image){try{await ShowProgressEditorImage(image,memo,"快捷备忘图片");}catch(Exception error){memo.Feedback.Text="图片预览失败："+error.Message;}};
                using(RegisterUnsavedUpdateEditor("快捷备忘",()=>memo.IsDirty,()=>memo.SaveAsync(),memo.DiscardForUpdate))
                    await ShowEditorWindowAsync(memo,this);
            }
        }catch(Exception error){MessageBox.Show("快捷备忘未能打开，原有文件已保留。\r\n\r\n"+error.Message,"TaskTrace · 快捷备忘",MessageBoxButtons.OK,MessageBoxIcon.Warning);}
        finally{quickMemo=null;}
    }
    internal static async Task<string> QuickMemoClipboardHtml(string html,Func<string,Task<byte[]>> download) {
        var output=new StringBuilder();int cursor=0;
        foreach(Match image in Regex.Matches(html??"",@"<img\b[^>]*>",RegexOptions.IgnoreCase)) {
            output.Append(html.Substring(cursor,image.Index-cursor));cursor=image.Index+image.Length;
            string source=ReferenceAttribute(image.Value,"src")??"";
            if(Regex.IsMatch(source,@"^data:image/(?:png|jpe?g|gif|bmp);base64,",RegexOptions.IgnoreCase)) {
                output.Append(Regex.Replace(image.Value,@"\s+data-tasktrace-src\s*=\s*(?:""[^""]*""|'[^']*'|[^\s>]+)","",RegexOptions.IgnoreCase));continue;
            }
            string path=Regex.IsMatch(source,@"^/?api/v[12]/tasks/[1-9][0-9]*/attachments/[1-9][0-9]*$",RegexOptions.IgnoreCase)?"/"+source.TrimStart('/'):null;if(path==null)throw new IOException("备忘图片无法读取，未粘贴不完整的内容。");
            byte[] bytes=await download(path);
            string dataImage=await Task.Run(delegate{using(var stream=new MemoryStream(bytes))using(var bitmap=Image.FromStream(stream))return Convert.ToBase64String(FloatingClipboard.PngBytes(bitmap));});
            output.Append("<img src=\"data:image/png;base64,").Append(dataImage).Append("\">");
        }
        output.Append((html??"").Substring(cursor));return output.ToString();
    }
}