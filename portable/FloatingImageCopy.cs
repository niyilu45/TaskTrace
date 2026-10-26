// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed class ClipboardPictureBox : PictureBox {
    internal Action CopyRequested;
    internal ClipboardPictureBox(){SetStyle(ControlStyles.Selectable,true);TabStop=true;}
    protected override bool ProcessCmdKey(ref Message message,Keys keyData){if(keyData==(Keys.Control|Keys.C)){if(CopyRequested!=null)CopyRequested();return true;}return base.ProcessCmdKey(ref message,keyData);}
    protected override void OnMouseDown(MouseEventArgs e){Focus();base.OnMouseDown(e);}
    internal void CopyShortcutForTest(){var message=new Message();ProcessCmdKey(ref message,Keys.Control|Keys.C);}
}

internal sealed class FloatingImageCopy : IDisposable {
    readonly Control target;
    readonly Func<Task<byte[]>> read;
    readonly Func<byte[],bool> write;
    readonly Action<string> report;
    readonly ContextMenuStrip menu=new ContextMenuStrip();
    readonly ToolStripMenuItem copy;
    bool copying;
    internal Task Pending=Task.FromResult(0);
    internal FloatingImageCopy(Control target,Func<Task<byte[]>> read,Action<string> report,Func<byte[],bool> write=null) {
        this.target=target;this.read=read;this.report=report;this.write=write??FloatingClipboard.CopyImage;
        copy=new ToolStripMenuItem("复制图片"){ShortcutKeyDisplayString="Ctrl+C"};copy.Click+=delegate{Request();};menu.Items.Add(copy);target.ContextMenuStrip=menu;
        var picture=target as ClipboardPictureBox;if(picture!=null)picture.CopyRequested=Request;
        var preview=target as ZoomableImagePreview;if(preview!=null)preview.CopyRequested=Request;
        target.Disposed+=delegate{Dispose();};
    }
    void Request(){if(!copying)Pending=Copy();}
    internal async Task Copy() {
        if(copying || target.IsDisposed)return;copying=true;copy.Enabled=false;
        try {var bytes=await read();if(target.IsDisposed)return;if(bytes==null || bytes.Length==0)throw new Exception("图片尚未加载，请稍后重试。");if(!write(bytes))throw new Exception("剪贴板正被其他程序占用，请重试。");if(report!=null)report("图片已复制，可粘贴到其他窗口或网页编辑框。");}
        catch(Exception error){if(!target.IsDisposed && report!=null)report("复制图片失败："+error.Message);}
        finally{copying=false;if(!copy.IsDisposed)copy.Enabled=true;}
    }
    public void Dispose(){menu.Dispose();}
}
