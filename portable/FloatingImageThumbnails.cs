// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

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
            var heading=new Label{Dock=DockStyle.Top,Height=40,Padding=new Padding(12,10,12,0),Text=item.Caption??"图片",AutoEllipsis=true};
            var box=new PictureBox{Dock=DockStyle.Fill,SizeMode=PictureBoxSizeMode.Zoom,BackColor=Color.FromArgb(30,30,30),AccessibleName="大图预览"};
            preview.Controls.Add(box);preview.Controls.Add(heading);preview.Shown+=async delegate {
                try {
                    byte[] bytes=item.Bytes??await DownloadImage(item.Source);
                    box.Image=ThumbnailImage(bytes,2200,1600);loaded=true;
                    if(verify||selfTest){using(var bitmap=new Bitmap(preview.Width,preview.Height)){preview.DrawToBitmap(bitmap,new Rectangle(Point.Empty,preview.Size));bitmap.Save(Path.Combine(data,"floating-image-preview-test.png"));}preview.Close();}
                } catch(Exception error){failure=error;heading.Text="图片读取失败："+error.Message;if(verify||selfTest)preview.Close();}
            };
            preview.FormClosed+=delegate{if(box.Image!=null){box.Image.Dispose();box.Image=null;}};
            preview.ShowDialog(owner);
            if((verify||selfTest) && (!loaded||failure!=null))throw new Exception("Image preview did not load",failure);
        }
        return Task.FromResult(0);
    }
}
