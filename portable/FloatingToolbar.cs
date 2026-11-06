// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Runtime.InteropServices;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    CaptionSettingsWindow captionSettingsWindow;
    bool captionSettingsInitialized,captionSettingsPaintQueued,captionBackgroundDirty=true,captionBackgroundReading;
    Color captionBackground=SystemColors.Window;
    int captionBackgroundReadCount;
    [DllImport("user32.dll",EntryPoint="PrintWindow")]static extern bool PrintCaptionWindow(IntPtr window,IntPtr dc,uint flags);
    [DllImport("dwmapi.dll")]static extern int DwmGetWindowAttribute(IntPtr window,int attribute,out uint value,int size);
    void InitializeCaptionSettings() {
        captionSettingsInitialized=true;
        VisibleChanged+=delegate {captionBackgroundDirty=true;SyncCaptionSettings();};
        EnabledChanged+=delegate {SyncCaptionSettings();};
        HandleDestroyed+=delegate {if(captionSettingsWindow!=null)captionSettingsWindow.Hide();captionSettingsPaintQueued=false;captionBackgroundDirty=true;};
        Disposed+=delegate {if(captionSettingsWindow!=null)captionSettingsWindow.Dispose();};
    }
    Rectangle CaptionSettingsBounds() {
        if(simpleMode || !Enabled || !IsHandleCreated || !Visible || edgeHidden || WindowState==FormWindowState.Minimized)return Rectangle.Empty;
        EdgeFrameRect buttons;
        try {
            if(DwmGetWindowAttribute(Handle,5,out buttons,Marshal.SizeOf(typeof(EdgeFrameRect)))==0 && buttons.Right>buttons.Left && buttons.Bottom>buttons.Top) {
                int count=1+(MinimizeBox?1:0)+(MaximizeBox?1:0);
                int width=(buttons.Right-buttons.Left)/Math.Max(1,count);
                return new Rectangle(buttons.Left-width,buttons.Top,width,buttons.Bottom-buttons.Top);
            }
        }catch(DllNotFoundException){}catch(EntryPointNotFoundException){}
        int frame=SystemInformation.FrameBorderSize.Width,button=Math.Max(28,SystemInformation.CaptionButtonSize.Width);
        return new Rectangle(Width-frame-button*(2+(MinimizeBox?1:0)+(MaximizeBox?1:0)),frame,button,SystemInformation.CaptionHeight);
    }
    void RefreshCaptionSettings(int message) {
        if(!captionSettingsInitialized || captionBackgroundReading || IsDisposed || Disposing || closing)return;
        bool themeChanged=message==0x86 || message==0x15 || message==0x1A || message==0x31A || message==0x31E || message==0x320 || message==0x02E0;
        if(themeChanged)captionBackgroundDirty=true;
        if(!themeChanged && message!=0x85 && message!=0x0C && message!=0x47)return;
        SyncCaptionSettings();
    }
    void QueueCaptionBackgroundRefresh() {
        if(!captionBackgroundDirty || captionSettingsPaintQueued || !IsHandleCreated || !Visible || simpleMode || closing || Disposing || IsDisposed)return;
        captionSettingsPaintQueued=true;
        BeginInvoke(new Action(delegate {
            captionSettingsPaintQueued=false;
            if(!IsHandleCreated || !Visible || simpleMode || simpleLayout || RecreatingHandle || closing || Disposing || IsDisposed)return;
            var bounds=CaptionSettingsBounds();if(bounds.IsEmpty)return;
            // PrintWindow can dispatch native messages. Only capture after the
            // initiating caption/layout message has fully unwound.
            captionSettingsWindow.ApplyCaptionBackground(ReadCaptionBackground(bounds));
            SyncCaptionSettings();
        }));
    }
    Color ReadCaptionBackground(Rectangle bounds) {
        if(!captionBackgroundDirty)return captionBackground;
        captionBackgroundReading=true;
        try {
            // DWM's default color is a sentinel, not the rendered active/inactive
            // caption color. Render only a caption-height strip; owned windows are
            // excluded, so the gear can never sample its own previous background.
            using(var bitmap=new Bitmap(Math.Max(1,Width),Math.Max(1,bounds.Bottom)))
            using(var graphics=Graphics.FromImage(bitmap)) {
                graphics.Clear(Color.Magenta);IntPtr dc=graphics.GetHdc();bool printed;
                try {captionBackgroundReadCount++;printed=PrintCaptionWindow(Handle,dc,2);}
                finally {graphics.ReleaseHdc(dc);}
                int x=Math.Max(0,Math.Min(bitmap.Width-1,bounds.Left+bounds.Width/2));
                int y=Math.Max(0,Math.Min(bitmap.Height-1,bounds.Top+Math.Max(2,bounds.Height/6)));
                Color sample=bitmap.GetPixel(x,y);
                if(printed && (sample.R!=255 || sample.G!=0 || sample.B!=255)) {
                    captionBackground=Color.FromArgb(sample.R,sample.G,sample.B);
                    captionBackgroundDirty=false;return captionBackground;
                }
            }
            captionBackground=SystemInformation.HighContrast?(ContainsFocus?SystemColors.ActiveCaption:SystemColors.InactiveCaption):SystemColors.Window;
            try {
                uint color;if(!SystemInformation.HighContrast && DwmGetWindowAttribute(Handle,35,out color,4)==0 && color!=0xFFFFFFFF && color!=0xFFFFFFFE)
                    captionBackground=Color.FromArgb((int)(color&255),(int)((color>>8)&255),(int)((color>>16)&255));
            }catch(DllNotFoundException){}catch(EntryPointNotFoundException){}
            captionBackgroundDirty=false;return captionBackground;
        }finally {captionBackgroundReading=false;}
    }
    void SyncCaptionSettings() {
        if(!captionSettingsInitialized || IsDisposed || Disposing || closing || captionBackgroundReading)return;
        var bounds=(simpleLayout || RecreatingHandle || FormBorderStyle==FormBorderStyle.None)?Rectangle.Empty:CaptionSettingsBounds();
        if(bounds.IsEmpty){if(captionSettingsWindow!=null)captionSettingsWindow.Hide();return;}
        if(captionSettingsWindow==null)captionSettingsWindow=new CaptionSettingsWindow(delegate {if(!closing && !simpleMode && !busy && Enabled)ShowAutoSaveSettings();});
        var position=new Rectangle(Left+bounds.Left,Top+bounds.Top,bounds.Width,bounds.Height);
        if(captionSettingsWindow.Bounds!=position)captionSettingsWindow.Bounds=position;
        // A disabled Form cannot be shown. Keep the caption host enabled and
        // disable only its action while the workspace is busy.
        captionSettingsWindow.Enabled=true;
        captionSettingsWindow.Settings.Enabled=!busy;
        if(captionSettingsWindow.TopMost!=TopMost)captionSettingsWindow.TopMost=TopMost;
        captionSettingsWindow.ApplyCaptionBackground(captionBackground);
        if(!captionSettingsWindow.Visible)captionSettingsWindow.Show(this);
        QueueCaptionBackgroundRefresh();
    }
}
internal sealed class CaptionSettingsWindow : Form {
    internal readonly CaptionSettingsButton Settings=new CaptionSettingsButton();
    readonly ToolTip hint=new ToolTip();
    internal CaptionSettingsWindow(Action open) {
        AutoScaleMode=AutoScaleMode.None;FormBorderStyle=FormBorderStyle.None;ShowInTaskbar=false;
        StartPosition=FormStartPosition.Manual;MinimizeBox=MaximizeBox=false;Text="TaskTrace 设置";
        Settings.Dock=DockStyle.Fill;Controls.Add(Settings);hint.SetToolTip(Settings,"设置");
        Settings.Click+=delegate {open();};
    }
    internal void ApplyCaptionBackground(Color background) {
        if(BackColor!=background)BackColor=background;
        // Set the actual button too: its default themed fill is independent of
        // the owning form's background, especially for inactive captions.
        Settings.UseVisualStyleBackColor=false;
        if(Settings.BackColor!=background)Settings.BackColor=background;
        Color overlay=background.GetBrightness()<0.5f?Color.White:Color.Black;
        Settings.FlatAppearance.MouseOverBackColor=BlendCaptionColor(background,overlay,26);
        Settings.FlatAppearance.MouseDownBackColor=BlendCaptionColor(background,overlay,51);
    }
    static Color BlendCaptionColor(Color background,Color overlay,int alpha) {
        return Color.FromArgb((background.R*(255-alpha)+overlay.R*alpha)/255,(background.G*(255-alpha)+overlay.G*alpha)/255,(background.B*(255-alpha)+overlay.B*alpha)/255);
    }
    protected override bool ShowWithoutActivation{get{return true;}}
    protected override CreateParams CreateParams{get{var value=base.CreateParams;value.ExStyle|=0x08000080;return value;}}
    protected override void WndProc(ref Message message){if(message.Msg==0x21){message.Result=new IntPtr(3);return;}base.WndProc(ref message);}
    protected override void Dispose(bool disposing){if(disposing)hint.Dispose();base.Dispose(disposing);}
}
internal sealed class CaptionSettingsButton : Button {
    internal CaptionSettingsButton(){Text="";AccessibleName="设置";AccessibleDescription="打开悬浮窗设置";FlatStyle=FlatStyle.Flat;FlatAppearance.BorderSize=0;TabStop=true;}
    protected override void OnPaint(PaintEventArgs e) {
        base.OnPaint(e);var state=e.Graphics.Save();
        try {
            e.Graphics.SmoothingMode=SmoothingMode.AntiAlias;
            float size=Math.Min(ClientSize.Width,ClientSize.Height)*0.58f,x=ClientSize.Width/2f,y=ClientSize.Height/2f;
            var points=new PointF[32];
            for(int index=0;index<points.Length;index++) {
                double angle=(index+0.5)*Math.PI/16;float radius=size*(index%4==0 || index%4==3?0.38f:0.5f);
                points[index]=new PointF(x+(float)Math.Cos(angle)*radius,y+(float)Math.Sin(angle)*radius);
            }
            using(var path=new GraphicsPath(FillMode.Alternate))using(var brush=new SolidBrush(!Enabled?SystemColors.GrayText:BackColor.GetBrightness()<0.5f?Color.White:Color.FromArgb(45,52,60))) {
                path.AddPolygon(points);float hole=size*0.32f;path.AddEllipse(x-hole/2,y-hole/2,hole,hole);e.Graphics.FillPath(brush,path);
            }
        }finally{e.Graphics.Restore(state);}
    }
}
