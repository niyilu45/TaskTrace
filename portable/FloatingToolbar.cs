// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Windows.Forms;

internal sealed class FloatingSettingsButton : Button {
    internal FloatingSettingsButton() {
        AutoSize=true;AutoSizeMode=AutoSizeMode.GrowAndShrink;
        MinimumSize=new Size(28,28);AccessibleName="设置";
        AccessibleDescription="打开悬浮窗设置";UseVisualStyleBackColor=true;
    }
    public override Size GetPreferredSize(Size proposedSize) {
        int side=Math.Max(Math.Max(MinimumSize.Width,MinimumSize.Height),TextRenderer.MeasureText("设置",Font).Height+8);
        return new Size(side,side);
    }
    protected override void OnPaint(PaintEventArgs e) {
        base.OnPaint(e);
        var state=e.Graphics.Save();
        try {
            e.Graphics.SmoothingMode=SmoothingMode.AntiAlias;
            float size=Math.Min(ClientSize.Width,ClientSize.Height)*0.58f;
            float x=ClientSize.Width/2f,y=ClientSize.Height/2f;
            var points=new PointF[32];
            for(int index=0;index<points.Length;index++) {
                double angle=(index+0.5)*Math.PI/16;
                float radius=size*(index%4==0 || index%4==3?0.38f:0.5f);
                points[index]=new PointF(x+(float)Math.Cos(angle)*radius,y+(float)Math.Sin(angle)*radius);
            }
            using(var path=new GraphicsPath(FillMode.Alternate))
            using(var brush=new SolidBrush(Enabled?ForeColor:SystemColors.GrayText)) {
                path.AddPolygon(points);float hole=size*0.32f;
                path.AddEllipse(x-hole/2,y-hole/2,hole,hole);e.Graphics.FillPath(brush,path);
            }
        } finally {e.Graphics.Restore(state);}
    }
}
