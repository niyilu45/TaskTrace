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
    internal float Zoom {get{return zoom;}}
    internal ZoomableImagePreview() {
        DoubleBuffered=true;BackColor=Color.FromArgb(30,30,30);TabStop=true;
        AccessibleName="大图预览";AccessibleDescription="使用鼠标滚轮缩放，放大后按住鼠标拖动查看";
        SetStyle(ControlStyles.ResizeRedraw|ControlStyles.Selectable,true);
    }
    internal void SetPreviewImage(Image value) {if(image!=null&&!Object.ReferenceEquals(image,value))image.Dispose();image=value;zoom=1f;panX=panY=0;Invalidate();}
    internal void ClearPreviewImage() {if(image!=null)image.Dispose();image=null;Invalidate();}
    SizeF FitSize() {
        if(image==null||ClientSize.Width<=0||ClientSize.Height<=0)return SizeF.Empty;
        float fit=Math.Min((float)ClientSize.Width/image.Width,(float)ClientSize.Height/image.Height);
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
    void AddImageThumbnail(FlowLayoutPanel panel,GalleryImage item,Form owner,Action remove=null) {
        var card=new Panel{Width=98,Height=66,Margin=new Padding(0,0,8,0),BackColor=Color.FromArgb(245,247,250)};
        var box=new PictureBox{Dock=DockStyle.Fill,SizeMode=PictureBoxSizeMode.CenterImage,BackColor=Color.White,Cursor=Cursors.Hand,AccessibleName="图片缩略图",AccessibleDescription="点击查看大图"};
        box.Click+=async delegate{await ShowImagePreview(item,owner);};card.Controls.Add(box);
        if(remove!=null) {
            var close=new Button{Text="×",Width=24,Height=24,Location=new Point(card.Width-24,0),Anchor=AnchorStyles.Top|AnchorStyles.Right,FlatStyle=FlatStyle.Flat,BackColor=Color.White,ForeColor=Color.Firebrick,TabStop=true,AccessibleName="移除待保存图片"};
            close.FlatAppearance.BorderColor=Color.FromArgb(210,214,220);close.Click+=delegate{remove();};card.Controls.Add(close);close.BringToFront();
        }
        panel.Controls.Add(card);box.Tag=LoadImageThumbnail(box,item);
    }
    Task ShowImagePreview(GalleryImage item,Form owner,bool verify=false) {
        bool loaded=false;Exception failure=null;
        using(var preview=DpiDialog(new Form{Text="查看大图 · "+(item.Caption??"图片"),Size=new Size(900,700),MinimumSize=new Size(420,320),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            string caption=item.Caption??"图片";
            var heading=new Label{Dock=DockStyle.Top,Height=40,Padding=new Padding(12,10,12,0),Text=caption+" · 滚轮缩放 · 100%",AutoEllipsis=true};
            var box=new ZoomableImagePreview{Dock=DockStyle.Fill};
            box.ZoomChanged+=delegate{heading.Text=caption+" · 滚轮缩放 · "+Math.Round(box.Zoom*100)+"%";};
            preview.Controls.Add(box);preview.Controls.Add(heading);preview.Shown+=async delegate {
                try {
                    byte[] bytes=item.Bytes??await DownloadImage(item.Source);
                    box.SetPreviewImage(ThumbnailImage(bytes,2200,1600));loaded=true;
                    if(verify||selfTest){box.SimulateWheelForTest(SystemInformation.MouseWheelScrollDelta);if(box.Zoom<=1f)throw new Exception("Image preview wheel zoom failed");using(var bitmap=new Bitmap(preview.Width,preview.Height)){preview.DrawToBitmap(bitmap,new Rectangle(Point.Empty,preview.Size));bitmap.Save(Path.Combine(data,"floating-image-preview-test.png"));}preview.Close();}
                } catch(Exception error){failure=error;heading.Text="图片读取失败："+error.Message;if(verify||selfTest)preview.Close();}
            };
            preview.FormClosed+=delegate{box.ClearPreviewImage();};
            preview.ShowDialog(owner);
            if((verify||selfTest) && (!loaded||failure!=null))throw new Exception("Image preview did not load",failure);
        }
        return Task.FromResult(0);
    }
}
