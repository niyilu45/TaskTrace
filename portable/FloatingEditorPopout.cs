// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.Windows.Forms;

internal sealed class FloatingEditorPopout : IDisposable {
    readonly ProgressHtmlEditor source;
    readonly Form owner;
    readonly Button originalSave;
    readonly Label originalFeedback;
    readonly Func<string> title;
    readonly TextBox itemContent;
    TextBox windowItemContent;
    bool updatingItemContent;
    Form window;
    ProgressHtmlEditor editor;
    Button windowSave;
    Label feedback;
    bool updating, disposed;

    internal FloatingEditorPopout(ProgressHtmlEditor source,Form owner,Button originalSave,Label originalFeedback,Func<string> title,TextBox itemContent=null) {
        this.source=source;
        this.owner=owner;
        this.originalSave=originalSave;
        this.originalFeedback=originalFeedback;
        originalFeedback.TextChanged+=FeedbackChanged;
        this.title=title;
        this.itemContent=itemContent;
        if(itemContent!=null) {
            itemContent.TextChanged+=ItemContentChanged;
            itemContent.ReadOnlyChanged+=ItemReadOnlyChanged;
        }
        source.HtmlChanged+=SourceChanged;
        source.ContentReplaced+=SourceReplaced;
        source.ReadOnlyChanged+=ReadOnlyChanged;
        source.Disposed+=LifetimeEnded;
        owner.FormClosed+=OwnerClosed;
        originalSave.EnabledChanged+=SaveEnabledChanged;
    }

    internal Control CreateHeader(string label) {
        return CreateHeader(new Label {
            Text=label,Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft,AccessibleName=label
        });
    }

    internal Control CreateHeader(Label label) {
        var header=new TableLayoutPanel {
            Dock=DockStyle.Fill,ColumnCount=2,RowCount=1,Margin=Padding.Empty
        };
        header.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));
        header.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,96));
        header.Controls.Add(label,0,0);
        var open=new Button {
            Text="窗口展示",Dock=DockStyle.Fill,AccessibleName="窗口展示 "+label.Text
        };
        open.Click+=delegate { Show(); };
        header.Controls.Add(open,1,0);
        return header;
    }

    internal void Show() {
        if(disposed || source.IsDisposed || owner.IsDisposed)return;
        if(window!=null && !window.IsDisposed) {
            if(window.WindowState==FormWindowState.Minimized)window.WindowState=FormWindowState.Normal;
            window.Show();
            window.Activate();
            return;
        }
        source.FlushChangesForLink();
        editor=source.CreateLinkedEditor();
        editor.Dock=DockStyle.Fill;
        editor.AccessibleName="窗口展示图文编辑框";
        editor.Html=source.Html;
        editor.SetReadOnly(source.IsReadOnly);
        window=new Form {
            Text=title(),Font=owner.Font,Icon=owner.Icon,TopMost=owner.TopMost,
            Size=new Size(720,540),MinimumSize=new Size(440,320),
            StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=true,
            MinimizeBox=true,MaximizeBox=true,KeyPreview=true,
            AutoScaleDimensions=new SizeF(96F,96F),AutoScaleMode=AutoScaleMode.Dpi
        };
        var area=Screen.FromControl(owner).WorkingArea;
        window.Size=new Size(Math.Min(window.Width,area.Width),Math.Min(window.Height,area.Height));
        var layout=new TableLayoutPanel {
            Dock=DockStyle.Fill,ColumnCount=1,RowCount=3,Padding=new Padding(12)
        };
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        int contentRow=1;
        if(itemContent!=null) {
            layout.RowCount=6;
            layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,76));
            layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            layout.Controls.Add(new Label {
                Text="遗留事项内容",AutoSize=true,Dock=DockStyle.Fill
            },0,1);
            windowItemContent=new TextBox {
                Multiline=true,Dock=DockStyle.Fill,ScrollBars=ScrollBars.Vertical,
                Text=itemContent.Text,ReadOnly=itemContent.ReadOnly,
                MaxLength=itemContent.MaxLength,WordWrap=itemContent.WordWrap,
                AcceptsReturn=itemContent.AcceptsReturn,AcceptsTab=itemContent.AcceptsTab,
                AccessibleName="窗口展示遗留事项内容"
            };
            windowItemContent.TextChanged+=WindowItemContentChanged;
            layout.Controls.Add(windowItemContent,0,2);
            layout.Controls.Add(new Label {
                Text="遗留事项备注 · 支持粘贴文字和图片",AutoSize=true,Dock=DockStyle.Fill
            },0,3);
            contentRow=4;
        }
        layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        feedback=new Label {
            Text="与原编辑框实时同步。Ctrl+V 粘贴图文，Ctrl+Z 撤销；保存使用同一份内容。",
            Dock=DockStyle.Fill,AutoSize=true,Padding=new Padding(0,0,0,8)
        };
        var actions=new FlowLayoutPanel {Dock=DockStyle.Fill,AutoSize=true,WrapContents=false};
        windowSave=new Button {Text="保存",AutoSize=true,Enabled=originalSave.Enabled};
        var back=new Button {Text="返回原窗口",AutoSize=true};
        var close=new Button {Text="关闭",AutoSize=true};
        windowSave.Click+=delegate { Flush();originalSave.PerformClick(); };
        back.Click+=delegate {
            Flush();
            if(owner.WindowState==FormWindowState.Minimized)owner.WindowState=FormWindowState.Normal;
            owner.Show();
            owner.Activate();
            source.FocusEditor();
        };
        close.Click+=delegate { window.Close(); };
        actions.Controls.AddRange(new Control[]{windowSave,back,close});
        layout.Controls.Add(feedback,0,0);
        layout.Controls.Add(editor,0,contentRow);
        layout.Controls.Add(actions,0,contentRow+1);
        window.Controls.Add(layout);
        editor.HtmlChanged+=EditorChanged;
        editor.ImageDoubleClicked+=delegate(object sender,ProgressEditorImageEventArgs image) {
            source.ForwardLinkedImage(image);
        };
        editor.ClipboardFailed+=delegate(object sender,ProgressClipboardErrorEventArgs error) {
            feedback.Text=error.Message;
            source.ForwardLinkedClipboardError(error);
        };
        source.FlushLinkedChanges=FlushFromMirror;
        editor.FlushLinkedChanges=FlushFromSource;
        source.LinkedPastePending=delegate { return editor!=null && !editor.IsDisposed && editor.HasOwnPendingPaste; };
        editor.LinkedPastePending=delegate { return !source.IsDisposed && source.HasOwnPendingPaste; };
        window.FormClosing+=WindowClosing;
        window.FormClosed+=WindowClosed;
        window.Activated+=delegate { Flush();RefreshTitle(); };
        window.KeyDown+=delegate(object sender,KeyEventArgs args) {
            if(args.Control && args.KeyCode==Keys.Enter) {
                args.SuppressKeyPress=true;
                windowSave.PerformClick();
            }
        };
        window.Show(owner);
        editor.FocusEditor();
    }

    void CopyItemContent(TextBox from,TextBox to) {
        if(updatingItemContent || to==null || to.IsDisposed || from.IsDisposed)return;
        updatingItemContent=true;
        try { if(to.Text!=from.Text)to.Text=from.Text;RefreshTitle(); }
        finally { updatingItemContent=false; }
    }

    void ItemContentChanged(object sender,EventArgs args) { CopyItemContent(itemContent,windowItemContent); }
    void WindowItemContentChanged(object sender,EventArgs args) { CopyItemContent(windowItemContent,itemContent); }
    void ItemReadOnlyChanged(object sender,EventArgs args) {
        if(windowItemContent!=null && !windowItemContent.IsDisposed)windowItemContent.ReadOnly=itemContent.ReadOnly;
    }

    void Copy(ProgressHtmlEditor from,ProgressHtmlEditor to,bool userEdit) {
        if(disposed || updating || to==null || to.IsDisposed || from.IsDisposed)return;
        updating=true;
        try { to.ApplyLinkedHtml(from.Html,userEdit);RefreshTitle(); }
        finally { updating=false; }
    }

    void FlushFromMirror() {
        if(!updating && editor!=null && !editor.IsDisposed)editor.FlushChangesForLink();
    }

    void FlushFromSource() {
        if(!updating && !source.IsDisposed)source.FlushChangesForLink();
    }

    void Flush() { FlushFromSource();FlushFromMirror(); }
    void SourceChanged(object sender,EventArgs args) { Copy(source,editor,true); }
    void SourceReplaced(object sender,EventArgs args) { Copy(source,editor,false); }
    void EditorChanged(object sender,EventArgs args) { Copy(editor,source,true); }

    void RefreshTitle() {
        if(window!=null && !window.IsDisposed)window.Text=title();
    }

    void ReadOnlyChanged(object sender,EventArgs args) {
        if(editor!=null && !editor.IsDisposed)editor.SetReadOnly(source.IsReadOnly);
    }

    void FeedbackChanged(object sender,EventArgs args) {
        if(feedback!=null && !feedback.IsDisposed)feedback.Text=originalFeedback.Text;
    }

    void SaveEnabledChanged(object sender,EventArgs args) {
        if(windowSave!=null && !windowSave.IsDisposed)windowSave.Enabled=originalSave.Enabled;
    }

    void WindowClosing(object sender,FormClosingEventArgs args) {
        Flush();
        if(!disposed && args.CloseReason==CloseReason.UserClosing && editor!=null && editor.HasOwnPendingPaste) {
            args.Cancel=true;
            feedback.Text="正在读取粘贴的图文，请完成后再关闭。";
        }
    }

    void WindowClosed(object sender,FormClosedEventArgs args) {
        source.FlushLinkedChanges=null;
        source.LinkedPastePending=null;
        window=null;
        editor=null;
        windowSave=null;
        windowItemContent=null;
        feedback=null;
    }

    void OwnerClosed(object sender,FormClosedEventArgs args) { Dispose(); }
    void LifetimeEnded(object sender,EventArgs args) { Dispose(); }

    public void Dispose() {
        if(disposed)return;
        Flush();
        disposed=true;
        source.HtmlChanged-=SourceChanged;
        source.ContentReplaced-=SourceReplaced;
        source.ReadOnlyChanged-=ReadOnlyChanged;
        source.Disposed-=LifetimeEnded;
        owner.FormClosed-=OwnerClosed;
        originalSave.EnabledChanged-=SaveEnabledChanged;
        originalFeedback.TextChanged-=FeedbackChanged;
        if(itemContent!=null) {
            itemContent.TextChanged-=ItemContentChanged;
            itemContent.ReadOnlyChanged-=ItemReadOnlyChanged;
        }
        source.FlushLinkedChanges=null;
        source.LinkedPastePending=null;
        if(window!=null && !window.IsDisposed)window.Dispose();
        window=null;
        editor=null;
    }
}
