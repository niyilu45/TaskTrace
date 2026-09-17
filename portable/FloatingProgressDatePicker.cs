// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Linq;
using System.Windows.Forms;

internal sealed class ProgressDatePicker : UserControl {
    readonly Button open=new Button {Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft,AccessibleName="记录日期",UseVisualStyleBackColor=true};
    readonly HashSet<DateTime> marked=new HashSet<DateTime>();
    DateTime value=DateTime.Today;
    internal event EventHandler ValueChanged;

    internal ProgressDatePicker() {
        MinimumSize=new Size(170,30);TabStop=false;Controls.Add(open);open.Click+=delegate{OpenCalendar();};UpdateText();
    }
    internal DateTime Value {
        get{return value;}
        set{var next=value.Date;if(next==this.value)return;this.value=next;UpdateText();if(ValueChanged!=null)ValueChanged(this,EventArgs.Empty);}
    }
    internal void SetMarkedDates(IEnumerable<DateTime> dates) {
        marked.Clear();foreach(var date in dates??Enumerable.Empty<DateTime>())marked.Add(date.Date);UpdateText();
    }
    void UpdateText(){open.Text=value.ToString("yyyy-MM-dd")+(marked.Contains(value.Date)?"    ●":"    选择日期");open.ForeColor=marked.Contains(value.Date)?Color.FromArgb(36,94,210):SystemColors.ControlText;}
    void OpenCalendar() {
        if(!Enabled)return;
        using(var popup=new ProgressCalendarPopup(value,marked)) {
            popup.DatePicked+=delegate(DateTime date){Value=date;popup.Close();};
            var screen=PointToScreen(new Point(0,Height));var area=Screen.FromControl(this).WorkingArea;
            popup.Location=new Point(Math.Max(area.Left,Math.Min(screen.X,area.Right-popup.Width)),Math.Max(area.Top,Math.Min(screen.Y,area.Bottom-popup.Height)));
            popup.ShowDialog(FindForm());
        }
    }
    internal Bitmap RenderCalendarForTest() {
        using(var popup=new ProgressCalendarPopup(value,marked)) {
            popup.Location=new Point(-10000,-10000);popup.Show();Application.DoEvents();
            var bitmap=new Bitmap(popup.Width,popup.Height);popup.DrawToBitmap(bitmap,new Rectangle(Point.Empty,popup.Size));popup.Hide();return bitmap;
        }
    }
    internal int VisibleMarkedDatesForTest(){using(var popup=new ProgressCalendarPopup(value,marked))return popup.VisibleMarkedDates;}
}

internal sealed class ProgressCalendarPopup : Form {
    readonly TableLayoutPanel grid=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=7,RowCount=7,Padding=new Padding(8,2,8,8)};
    readonly Label heading=new Label {Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleCenter,Font=new Font(SystemFonts.MessageBoxFont,FontStyle.Bold)};
    readonly DateTime selected;
    readonly HashSet<DateTime> marked;
    DateTime month;
    internal event Action<DateTime> DatePicked;
    internal int VisibleMarkedDates {get;private set;}

    internal ProgressCalendarPopup(DateTime selected,IEnumerable<DateTime> markedDates) {
        this.selected=selected.Date;marked=new HashSet<DateTime>((markedDates??Enumerable.Empty<DateTime>()).Select(date=>date.Date));month=new DateTime(selected.Year,selected.Month,1);
        FormBorderStyle=FormBorderStyle.FixedSingle;ControlBox=false;ShowInTaskbar=false;StartPosition=FormStartPosition.Manual;Size=new Size(318,300);MinimumSize=MaximumSize=Size;KeyPreview=true;Font=SystemFonts.MessageBoxFont;
        var previous=new Button {Text="上月",Dock=DockStyle.Fill,AccessibleName="上一个月"};var next=new Button {Text="下月",Dock=DockStyle.Fill,AccessibleName="下一个月"};
        var header=new TableLayoutPanel {Dock=DockStyle.Top,Height=42,ColumnCount=3,Padding=new Padding(8,6,8,0)};header.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,62));header.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));header.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,62));
        header.Controls.Add(previous,0,0);header.Controls.Add(heading,1,0);header.Controls.Add(next,2,0);Controls.Add(grid);Controls.Add(header);
        for(int column=0;column<7;column++)grid.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100f/7));
        grid.RowStyles.Add(new RowStyle(SizeType.Absolute,25));for(int row=1;row<7;row++)grid.RowStyles.Add(new RowStyle(SizeType.Percent,100f/6));
        previous.Click+=delegate{month=month.AddMonths(-1);RenderMonth();};next.Click+=delegate{month=month.AddMonths(1);RenderMonth();};
        KeyDown+=delegate(object sender,KeyEventArgs e){if(e.KeyCode==Keys.Escape){e.Handled=true;Close();}};RenderMonth();
    }
    void RenderMonth() {
        grid.SuspendLayout();grid.Controls.Clear();VisibleMarkedDates=0;heading.Text=month.ToString("yyyy 年 M 月");
        string[] weekdays={"一","二","三","四","五","六","日"};for(int column=0;column<7;column++)grid.Controls.Add(new Label {Text=weekdays[column],Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleCenter,ForeColor=SystemColors.GrayText},column,0);
        int mondayOffset=((int)month.DayOfWeek+6)%7;DateTime first=month.AddDays(-mondayOffset);
        for(int index=0;index<42;index++) {
            DateTime date=first.AddDays(index);bool hasProgress=marked.Contains(date);if(hasProgress)VisibleMarkedDates++;
            var button=new ProgressDayButton {Date=date,HasProgress=hasProgress,Selected=date==selected,CurrentMonth=date.Month==month.Month,Dock=DockStyle.Fill,Margin=new Padding(1),AccessibleName=date.ToString("yyyy-MM-dd")+(hasProgress?"，有进展":"")};
            button.Click+=delegate(object sender,EventArgs e){var picked=((ProgressDayButton)sender).Date;if(DatePicked!=null)DatePicked(picked);};grid.Controls.Add(button,index%7,index/7+1);
        }
        grid.ResumeLayout();
    }
}

internal sealed class ProgressDayButton : Button {
    internal DateTime Date;internal bool HasProgress,Selected,CurrentMonth;
    internal ProgressDayButton(){FlatStyle=FlatStyle.Flat;FlatAppearance.BorderSize=0;UseVisualStyleBackColor=false;TabStop=true;}
    protected override void OnPaint(PaintEventArgs e) {
        Color background=Selected?Color.FromArgb(36,94,210):SystemColors.Window;Color foreground=Selected?Color.White:CurrentMonth?SystemColors.WindowText:SystemColors.GrayText;
        e.Graphics.Clear(background);TextRenderer.DrawText(e.Graphics,Date.Day.ToString(),Font,ClientRectangle,foreground,TextFormatFlags.HorizontalCenter|TextFormatFlags.VerticalCenter|TextFormatFlags.NoPadding);
        if(Date.Date==DateTime.Today && !Selected)using(var pen=new Pen(Color.FromArgb(36,94,210)))e.Graphics.DrawRectangle(pen,1,1,Math.Max(1,Width-3),Math.Max(1,Height-3));
        if(HasProgress)using(var brush=new SolidBrush(Selected?Color.White:Color.FromArgb(36,94,210)))e.Graphics.FillEllipse(brush,Math.Max(1,Width/2-3),Math.Max(1,Height-8),6,6);
        if(Focused)ControlPaint.DrawFocusRectangle(e.Graphics,new Rectangle(3,3,Math.Max(1,Width-7),Math.Max(1,Height-7)),foreground,background);
    }
}
