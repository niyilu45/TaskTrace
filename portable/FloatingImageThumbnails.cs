// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed class ZoomableImagePreview : Control {
    Image image;
    float zoom=1f,panX,panY;
    bool panning;
    Point panStart;
    float panStartX,panStartY;
    internal event EventHandler ZoomChanged;
    internal Action CopyRequested;
    internal Action<string> CopyFeedback;
    internal readonly FloatingImageCopy ImageCopy;
    internal float Zoom {get{return zoom;}}
    internal ZoomableImagePreview(Func<byte[],bool> writeClipboard=null) {
        DoubleBuffered=true;BackColor=Color.FromArgb(30,30,30);TabStop=true;
        AccessibleName="大图预览";AccessibleDescription="使用鼠标滚轮缩放，放大后按住鼠标拖动查看";
        SetStyle(ControlStyles.ResizeRedraw|ControlStyles.Selectable,true);
        ImageCopy=new FloatingImageCopy(this,delegate{return Task.FromResult(FloatingClipboard.PngBytes(image));},delegate(string message){if(CopyFeedback!=null)CopyFeedback(message);},writeClipboard);
    }
    protected override bool ProcessCmdKey(ref Message message,Keys keyData){if(keyData==(Keys.Control|Keys.C)){if(CopyRequested!=null)CopyRequested();return true;}return base.ProcessCmdKey(ref message,keyData);}
    internal void CopyShortcutForTest(){var message=new Message();ProcessCmdKey(ref message,Keys.Control|Keys.C);}
    internal void SetPreviewImage(Image value) {if(image!=null&&!Object.ReferenceEquals(image,value))image.Dispose();image=value;zoom=1f;panX=panY=0;Invalidate();}
    internal void ClearPreviewImage() {if(image!=null)image.Dispose();image=null;Invalidate();}
    SizeF FitSize() {
        if(image==null||ClientSize.Width<=0||ClientSize.Height<=0)return SizeF.Empty;
        float fit=Math.Min(1f,Math.Min((float)ClientSize.Width/image.Width,(float)ClientSize.Height/image.Height));
        return new SizeF(image.Width*fit,image.Height*fit);
    }
    void ClampPan() {
        var fit=FitSize();float maxX=Math.Max(0,(fit.Width*zoom-ClientSize.Width)/2f),maxY=Math.Max(0,(fit.Height*zoom-ClientSize.Height)/2f);
        panX=Math.Max(-maxX,Math.Min(maxX,panX));panY=Math.Max(-maxY,Math.Min(maxY,panY));
        if(zoom<=1f){panX=0;panY=0;}
    }
    void SetZoom(float next,Point anchor) {
        next=Math.Max(1f,Math.Min(8f,next));if(Math.Abs(next-zoom)<0.001f)return;
        float cx=ClientSize.Width/2f,cy=ClientSize.Height/2f;
        float imageX=(anchor.X-cx-panX)/zoom,imageY=(anchor.Y-cy-panY)/zoom;
        panX=anchor.X-cx-imageX*next;panY=anchor.Y-cy-imageY*next;zoom=next;ClampPan();Invalidate();
        var changed=ZoomChanged;if(changed!=null)changed(this,EventArgs.Empty);
    }
    protected override void OnPaint(PaintEventArgs e) {
        base.OnPaint(e);if(image==null)return;var fit=FitSize();float width=fit.Width*zoom,height=fit.Height*zoom;
        var rectangle=new RectangleF((ClientSize.Width-width)/2f+panX,(ClientSize.Height-height)/2f+panY,width,height);
        e.Graphics.InterpolationMode=System.Drawing.Drawing2D.InterpolationMode.HighQualityBicubic;e.Graphics.DrawImage(image,rectangle);
    }
    protected override void OnMouseEnter(EventArgs e) {base.OnMouseEnter(e);Focus();}
    protected override void OnMouseWheel(MouseEventArgs e) {base.OnMouseWheel(e);if(image==null||e.Delta==0)return;SetZoom(zoom*(float)Math.Pow(1.25,(double)e.Delta/SystemInformation.MouseWheelScrollDelta),e.Location);}
    protected override void OnMouseDown(MouseEventArgs e) {base.OnMouseDown(e);Focus();if(e.Button!=MouseButtons.Left||zoom<=1f)return;panning=true;Capture=true;panStart=e.Location;panStartX=panX;panStartY=panY;Cursor=Cursors.SizeAll;}
    protected override void OnMouseMove(MouseEventArgs e) {base.OnMouseMove(e);if(!panning)return;panX=panStartX+e.X-panStart.X;panY=panStartY+e.Y-panStart.Y;ClampPan();Invalidate();}
    protected override void OnMouseUp(MouseEventArgs e) {base.OnMouseUp(e);if(!panning)return;panning=false;Capture=false;Cursor=Cursors.Default;}
    protected override void OnMouseCaptureChanged(EventArgs e) {base.OnMouseCaptureChanged(e);if(Capture)return;panning=false;Cursor=Cursors.Default;}
    protected override void OnResize(EventArgs e) {base.OnResize(e);ClampPan();Invalidate();}
    internal void SimulateWheelForTest(int delta) {OnMouseWheel(new MouseEventArgs(MouseButtons.None,0,ClientSize.Width/2,ClientSize.Height/2,delta));}
}

internal sealed partial class FloatingWindow {
    static void ClearImageThumbnails(FlowLayoutPanel panel) {
        foreach(Control control in panel.Controls.Cast<Control>().ToArray()) {
            var box=control.Controls.OfType<PictureBox>().FirstOrDefault();
            if(box!=null && box.Image!=null){box.Image.Dispose();box.Image=null;}
            control.Dispose();
        }
        panel.Controls.Clear();
    }
    static Image ThumbnailImage(byte[] bytes,int width,int height) {
        using(var stream=new MemoryStream(bytes))using(var source=Image.FromStream(stream)) {
            double scale=Math.Min(1.0,Math.Min((double)width/source.Width,(double)height/source.Height));
            int targetWidth=Math.Max(1,(int)Math.Round(source.Width*scale)),targetHeight=Math.Max(1,(int)Math.Round(source.Height*scale));
            return new Bitmap(source,new Size(targetWidth,targetHeight));
        }
    }
    async Task LoadImageThumbnail(PictureBox box,GalleryImage item) {
        try {
            byte[] bytes=item.Bytes??await DownloadImage(item.Source);
            var thumbnail=ThumbnailImage(bytes,88,58);
            if(box.IsDisposed){thumbnail.Dispose();return;}
            var previous=box.Image;box.Image=thumbnail;if(previous!=null)previous.Dispose();
        } catch {if(!box.IsDisposed){box.BackColor=Color.MistyRose;box.AccessibleDescription="图片缩略图加载失败";}}
    }
    void AddImageThumbnail(FlowLayoutPanel panel,GalleryImage item,Form owner,Action remove=null,Action<string> feedback=null) {
        var card=new Panel{Width=98,Height=66,Margin=new Padding(0,0,8,0),BackColor=Color.FromArgb(245,247,250)};
        var box=new ClipboardPictureBox{Dock=DockStyle.Fill,SizeMode=PictureBoxSizeMode.CenterImage,BackColor=Color.White,Cursor=Cursors.Hand,AccessibleName="图片缩略图",AccessibleDescription="点击查看大图"};
        box.Click+=async delegate{await ShowImagePreview(item,owner);};card.Controls.Add(box);
        new FloatingImageCopy(box,async delegate{return item.Bytes??await DownloadImage(item.Source);},feedback??delegate(string message){status.Text=message;});
        if(remove!=null) {
            var close=new Button{Text="×",Width=24,Height=24,Location=new Point(card.Width-24,0),Anchor=AnchorStyles.Top|AnchorStyles.Right,FlatStyle=FlatStyle.Flat,BackColor=Color.White,ForeColor=Color.Firebrick,TabStop=true,AccessibleName="移除待保存图片"};
            close.FlatAppearance.BorderColor=Color.FromArgb(210,214,220);close.Click+=delegate{remove();};card.Controls.Add(close);close.BringToFront();
        }
        panel.Controls.Add(card);box.Tag=LoadImageThumbnail(box,item);
    }
    async Task ShowImagePreview(GalleryImage item,Form owner,bool verify=false) {
        bool loaded=false;Exception failure=null;
        using(var preview=DpiDialog(new Form{Text="查看大图 · "+(item.Caption??"图片"),Size=new Size(900,700),MinimumSize=new Size(420,320),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            string caption=item.Caption??"图片";
            var heading=new Label{Dock=DockStyle.Top,Height=40,Padding=new Padding(12,10,12,0),Text=caption+" · 滚轮缩放 · 100%",AutoEllipsis=true};
            var box=new ZoomableImagePreview{Dock=DockStyle.Fill};box.CopyFeedback=delegate(string message){heading.Text=message;};
            box.ZoomChanged+=delegate{heading.Text=caption+" · 滚轮缩放 · "+Math.Round(box.Zoom*100)+"%";};
            preview.Controls.Add(box);preview.Controls.Add(heading);preview.Shown+=async delegate {
                try {
                    byte[] bytes=item.Bytes??await DownloadImage(item.Source);
                    using(var stream=new MemoryStream(bytes))using(var original=Image.FromStream(stream)) {
                        var screen=Screen.FromControl(owner).WorkingArea;
                        Size frame=preview.Size-preview.ClientSize;
                        preview.MinimumSize=new Size(Math.Min(320,screen.Width),Math.Min(240,screen.Height));
                        float fit=Math.Min(1f,Math.Min((float)(screen.Width-frame.Width-32)/original.Width,(float)(screen.Height-frame.Height-heading.Height-32)/original.Height));
                        preview.ClientSize=new Size(Math.Max(300,(int)Math.Ceiling(original.Width*fit)),Math.Max(160,(int)Math.Ceiling(original.Height*fit))+heading.Height);
                        preview.Size=new Size(Math.Min(screen.Width,preview.Width),Math.Min(screen.Height,preview.Height));
                        preview.Location=new Point(screen.Left+(screen.Width-preview.Width)/2,screen.Top+(screen.Height-preview.Height)/2);
                        box.SetPreviewImage(new Bitmap(original));
                    }
                    loaded=true;
                    if(verify||selfTest){if(preview.Modal || preview.Owner!=null || (owner!=null && !owner.Enabled))throw new Exception("Image preview disabled another editor");box.SimulateWheelForTest(SystemInformation.MouseWheelScrollDelta);if(box.Zoom<=1f)throw new Exception("Image preview wheel zoom failed");using(var bitmap=new Bitmap(preview.Width,preview.Height)){preview.DrawToBitmap(bitmap,new Rectangle(Point.Empty,preview.Size));bitmap.Save(Path.Combine(data,"floating-image-preview-test.png"));}preview.Close();}
                } catch(Exception error){failure=error;heading.Text="图片读取失败："+error.Message;if(verify||selfTest)preview.Close();}
            };
            preview.FormClosed+=delegate{box.ClearPreviewImage();};
            await ShowEditorWindowAsync(preview,owner);
            if((verify||selfTest) && (!loaded||failure!=null))throw new Exception("Image preview did not load",failure);
        }
    }
}
