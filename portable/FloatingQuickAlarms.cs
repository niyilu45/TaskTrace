// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Web.Script.Serialization;
using System.Windows.Forms;

// Local tray timers deliberately do not read or mutate tasks, reminders, or teamData.
internal sealed class FloatingQuickAlarms : IDisposable {
    internal sealed class Alarm {
        internal string Name;
        internal int Seconds, StartedSeconds;
        internal DateTime? DueUtc;
        internal bool Running { get { return DueUtc.HasValue; } }
        internal void Start(DateTime now) { StartedSeconds=Seconds; DueUtc=now.AddSeconds(Seconds); }
        internal void Cancel() { DueUtc=null; }
        internal int Remaining(DateTime now) {
            return Running ? Math.Max(0,(int)Math.Ceiling((DueUtc.Value-now).TotalSeconds)) : Seconds;
        }
    }
    sealed class Settings { public int[] Seconds {get;set;} }
    sealed class Completion { internal string Name; internal int Seconds; }
    [DllImport("kernel32.dll")] static extern ulong GetTickCount64();
    const int MaximumSeconds=999*3600+59*60+59;
    readonly string settingsPath;
    readonly Font font;
    readonly Icon icon;
    readonly Func<DateTime> clock;
    readonly Action<Form> display;
    readonly Timer timer=new Timer {Interval=250};
    readonly ToolStripMenuItem[] items=new ToolStripMenuItem[6];
    internal readonly Alarm[] Alarms=new Alarm[6];
    internal readonly ToolStripMenuItem Menu=new ToolStripMenuItem("便捷闹钟");
    internal Form SettingsDialog, CustomDialog, CompletionDialog;
    ListBox completedList;
    bool disposed;

    internal FloatingQuickAlarms(string dataDirectory,Font uiFont,Icon appIcon,Func<DateTime> utcNow=null,Action<Form> show=null) {
        settingsPath=Path.Combine(dataDirectory,"quick-alarms.json");
        font=uiFont;icon=appIcon;display=show;
        // Windows uptime includes sleep and does not jump when the system clock is adjusted.
        DateTime origin=DateTime.UtcNow;ulong started=GetTickCount64();
        clock=utcNow??(()=>origin.AddMilliseconds(GetTickCount64()-started));
        int[] defaults={300,600,900,1800,3600,900};
        for(int i=0;i<Alarms.Length;i++)Alarms[i]=new Alarm {Name=i<5?"闹钟"+(i+1):"任意闹钟",Seconds=defaults[i]};
        LoadSettings();
        for(int i=0;i<Alarms.Length;i++) {
            int index=i;
            items[i]=new ToolStripMenuItem {Name="quick-alarm-"+i,CheckOnClick=false};
            items[i].Click+=delegate {Toggle(index);};
            if(i==5)Menu.DropDownItems.Add(new ToolStripSeparator());
            Menu.DropDownItems.Add(items[i]);
        }
        Menu.DropDownItems.Add(new ToolStripSeparator());
        var edit=new ToolStripMenuItem("编辑闹钟") {Name="quick-alarm-edit"};
        edit.Click+=delegate {OpenSettings();};Menu.DropDownItems.Add(edit);
        Menu.DropDownOpening+=delegate {Poll();};
        timer.Tick+=delegate {Poll();};
        RefreshMenu(clock());
    }
    static bool ValidSeconds(int seconds) {return seconds>0&&seconds<=MaximumSeconds;}
    internal static string FormatDuration(int seconds) {
        return (seconds/3600).ToString("00",CultureInfo.InvariantCulture)+":"+
            (seconds/60%60).ToString("00",CultureInfo.InvariantCulture)+":"+(seconds%60).ToString("00",CultureInfo.InvariantCulture);
    }
    void LoadSettings() {
        try {
            if(!File.Exists(settingsPath))return;
            var saved=new JavaScriptSerializer().Deserialize<Settings>(File.ReadAllText(settingsPath));
            if(saved==null||saved.Seconds==null)return;
            for(int i=0;i<Math.Min(saved.Seconds.Length,Alarms.Length);i++)if(ValidSeconds(saved.Seconds[i]))Alarms[i].Seconds=saved.Seconds[i];
        } catch { /* A damaged preference must not prevent the workspace from opening. */ }
    }
    internal void SaveDurations(int[] values) {
        if(values==null||values.Length!=Alarms.Length||values.Any(value=>!ValidSeconds(value)))throw new ArgumentException("每个闹钟的时长必须大于 0，且不超过 999 小时 59 分 59 秒。");
        string temp=settingsPath+"."+Guid.NewGuid().ToString("N")+".tmp";
        try {
            Directory.CreateDirectory(Path.GetDirectoryName(settingsPath));
            File.WriteAllText(temp,new JavaScriptSerializer().Serialize(new Settings {Seconds=values}));
            if(File.Exists(settingsPath))File.Replace(temp,settingsPath,null);else File.Move(temp,settingsPath);
            // Change configured durations only. An already running alarm keeps its original deadline.
            for(int i=0;i<Alarms.Length;i++)Alarms[i].Seconds=values[i];
            RefreshMenu(clock());
        } finally {if(File.Exists(temp))File.Delete(temp);}
    }
    void Toggle(int index) {
        if(disposed)return;
        var alarm=Alarms[index];
        if(alarm.Running) {
            alarm.Cancel();
            RefreshMenu(clock());
            if(!Alarms.Any(value=>value.Running))timer.Stop();
        } else if(index==5)OpenCustom();
        else {alarm.Start(clock());RefreshMenu(clock());timer.Start();}
    }
    void RefreshMenu(DateTime now) {
        for(int i=0;i<Alarms.Length;i++) {
            var alarm=Alarms[i];
            string text=alarm.Name+"　"+(alarm.Running?"剩余 ":"时长 ")+FormatDuration(alarm.Remaining(now));
            if(items[i].Text!=text)items[i].Text=text;
            items[i].Checked=alarm.Running;
            items[i].ToolTipText=alarm.Running?"取消勾选可停止计时并恢复设置的时间":i==5?"勾选后填写本次时长，确认后开始倒计时":"勾选后开始倒计时";
        }
    }
    internal void Poll() {
        if(disposed)return;
        DateTime now=clock();
        var completed=new List<Completion>();
        foreach(var alarm in Alarms) {
            if(!alarm.Running||alarm.DueUtc.Value>now)continue;
            completed.Add(new Completion {Name=alarm.Name,Seconds=alarm.StartedSeconds});
            alarm.Cancel();
        }
        RefreshMenu(now);
        if(!Alarms.Any(value=>value.Running))timer.Stop();
        if(completed.Count>0)ShowCompleted(completed);
    }
    Form CreateDialog(string title,Size size) {
        return new Form {Text="TaskTrace · "+title,Font=font,Icon=icon,AutoScaleDimensions=new SizeF(96,96),AutoScaleMode=AutoScaleMode.Dpi,
            ClientSize=size,MinimumSize=new Size(size.Width,size.Height),FormBorderStyle=FormBorderStyle.Sizable,
            MaximizeBox=false,MinimizeBox=true,ShowInTaskbar=true,TopMost=true,StartPosition=FormStartPosition.Manual,
            BackColor=SystemColors.Window,ForeColor=SystemColors.WindowText};
    }
    void DisplayDialog(Form dialog) {
        var area=Screen.FromPoint(Cursor.Position).WorkingArea;
        dialog.Size=new Size(Math.Min(dialog.Width,area.Width),Math.Min(dialog.Height,area.Height));
        dialog.Location=new Point(area.Left+(area.Width-dialog.Width)/2,area.Top+(area.Height-dialog.Height)/2);
        if(display!=null)display(dialog);
        else {dialog.Show();dialog.Activate();}
    }
    static Label TextLabel(string text) {
        return new Label {Text=text,AutoSize=true,Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft,Margin=new Padding(0,4,8,4)};
    }
    static NumericUpDown DurationNumber(string name,int value,int max) {
        return new NumericUpDown {Name=name,AccessibleName=name,Minimum=0,Maximum=max,Value=value,DecimalPlaces=0,
            Dock=DockStyle.Fill,Margin=new Padding(0,5,12,5),TextAlign=HorizontalAlignment.Right,ThousandsSeparator=false};
    }
    TableLayoutPanel DurationTable(int[] indices,out NumericUpDown[][] inputs) {
        var table=new TableLayoutPanel {Dock=DockStyle.Top,AutoSize=true,AutoSizeMode=AutoSizeMode.GrowAndShrink,ColumnCount=4,RowCount=indices.Length+1,Padding=Padding.Empty,Margin=Padding.Empty};
        table.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,28));
        for(int row=0;row<=indices.Length;row++)table.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        for(int column=1;column<4;column++)table.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,24));
        table.Controls.Add(TextLabel("闹钟"),0,0);table.Controls.Add(TextLabel("小时"),1,0);table.Controls.Add(TextLabel("分钟"),2,0);table.Controls.Add(TextLabel("秒"),3,0);
        inputs=new NumericUpDown[indices.Length][];
        for(int row=0;row<indices.Length;row++) {
            var alarm=Alarms[indices[row]];
            var name=new Label {Name="alarm-name-"+indices[row],Text=alarm.Name,AutoSize=true,Anchor=AnchorStyles.Top|AnchorStyles.Left,Margin=new Padding(0,8,8,4)};
            table.Controls.Add(name,0,row+1);
            inputs[row]=new[]{DurationNumber(alarm.Name+"小时",alarm.Seconds/3600,999),DurationNumber(alarm.Name+"分钟",alarm.Seconds/60%60,59),DurationNumber(alarm.Name+"秒",alarm.Seconds%60,59)};
            for(int col=0;col<3;col++)table.Controls.Add(inputs[row][col],col+1,row+1);
        }
        return table;
    }
    static int InputSeconds(NumericUpDown[] inputs) {
        // ValidateEditText commits typed values even when Enter, rather than a click, submits.
        foreach(var input in inputs) {
            string value=input.Text.Trim();int parsed;
            if(!Int32.TryParse(value,NumberStyles.Integer,CultureInfo.CurrentCulture,out parsed)||parsed<input.Minimum||parsed>input.Maximum)
                throw new ArgumentException("请输入有效的整数时长：小时 0–999，分钟和秒 0–59。");
            input.Value=parsed;
        }
        int seconds=Decimal.ToInt32(inputs[0].Value)*3600+Decimal.ToInt32(inputs[1].Value)*60+Decimal.ToInt32(inputs[2].Value);
        if(!ValidSeconds(seconds))throw new ArgumentException("闹钟时长必须大于 0。");
        return seconds;
    }
    void OpenSettings() {
        if(SettingsDialog!=null&&!SettingsDialog.IsDisposed){SettingsDialog.WindowState=FormWindowState.Normal;SettingsDialog.Activate();return;}
        NumericUpDown[][] inputs;
        var table=DurationTable(new[]{0,1,2,3,4},out inputs);
        var dialog=BuildDurationDialog("编辑闹钟",table,
            "可同时开启多个闹钟。修改时长不会重置正在运行的倒计时，下次启动使用新时长。\r\n时长仅保存在本机；隐藏悬浮窗后继续计时，退出 TaskTrace 后停止计时。",
            "保存",delegate {var values=Alarms.Select(alarm=>alarm.Seconds).ToArray();for(int i=0;i<5;i++)values[i]=InputSeconds(inputs[i]);SaveDurations(values);});
        SettingsDialog=dialog;dialog.FormClosed+=delegate {SettingsDialog=null;};
        DisplayDialog(dialog);
    }
    void OpenCustom() {
        if(CustomDialog!=null&&!CustomDialog.IsDisposed){CustomDialog.WindowState=FormWindowState.Normal;CustomDialog.Activate();return;}
        NumericUpDown[][] inputs;
        var table=DurationTable(new[]{5},out inputs);
        var dialog=BuildDurationDialog("任意闹钟",table,"填写本次倒计时时长，确认后开始；取消不会开启闹钟。","确认",
            delegate {int seconds=InputSeconds(inputs[0]);var values=Alarms.Select(alarm=>alarm.Seconds).ToArray();values[5]=seconds;SaveDurations(values);Alarms[5].Start(clock());RefreshMenu(clock());timer.Start();});
        CustomDialog=dialog;dialog.FormClosed+=delegate {CustomDialog=null;};
        DisplayDialog(dialog);
    }
    Form BuildDurationDialog(string title,TableLayoutPanel table,string description,string action,Action save) {
        var dialog=CreateDialog(title,new Size(560,title=="编辑闹钟"?400:245));
        var layout=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=1,RowCount=4,Padding=new Padding(20),AutoScroll=true};
        layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.Controls.Add(table,0,0);
        var instructions=TextLabel(description);instructions.Name="alarm-description";layout.Controls.Add(instructions,0,1);
        var error=TextLabel("");error.Name="alarm-error";error.ForeColor=Color.Firebrick;error.Visible=false;
        layout.Controls.Add(error,0,2);
        var buttons=new FlowLayoutPanel {Dock=DockStyle.Fill,AutoSize=true,FlowDirection=FlowDirection.RightToLeft,WrapContents=false,Margin=new Padding(0,12,0,0)};
        var confirm=new Button {Name="alarm-confirm",Text=action,AutoSize=true,MinimumSize=new Size(90,32)};
        var cancel=new Button {Text="取消",AutoSize=true,MinimumSize=new Size(90,32),DialogResult=DialogResult.Cancel};
        confirm.Click+=delegate {
            try {save();dialog.Close();}
            catch(Exception failure){error.Text="未保存："+failure.Message;error.Visible=true;}
        };
        cancel.Click+=delegate {dialog.Close();};
        buttons.Controls.Add(confirm);buttons.Controls.Add(cancel);layout.Controls.Add(buttons,0,3);
        dialog.Controls.Add(layout);dialog.AcceptButton=confirm;dialog.CancelButton=cancel;
        return dialog;
    }
    void ShowCompleted(List<Completion> completed) {
        bool created=CompletionDialog==null||CompletionDialog.IsDisposed;
        if(created) {
            var dialog=CreateDialog("计时完成",new Size(450,270));CompletionDialog=dialog;
            var layout=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=1,RowCount=3,Padding=new Padding(20)};
            layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));
            layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
            layout.Controls.Add(TextLabel("以下闹钟已完成倒计时："),0,0);
            completedList=new ListBox {Dock=DockStyle.Fill,IntegralHeight=false,BorderStyle=BorderStyle.None,AccessibleName="已完成的闹钟",HorizontalScrollbar=true};
            layout.Controls.Add(completedList,0,1);
            var confirm=new Button {Text="确认",Name="alarm-confirm",AutoSize=true,MinimumSize=new Size(90,32),Anchor=AnchorStyles.Right,Margin=new Padding(0,14,0,0)};
            confirm.Click+=delegate {dialog.Close();};layout.Controls.Add(confirm,0,2);
            dialog.Controls.Add(layout);dialog.AcceptButton=confirm;
            dialog.FormClosed+=delegate {CompletionDialog=null;completedList=null;};
        }
        foreach(var completedAlarm in completed)completedList.Items.Add(completedAlarm.Name+"　"+FormatDuration(completedAlarm.Seconds)+"　计时完成");
        if(created||CompletionDialog.WindowState==FormWindowState.Minimized||!CompletionDialog.Visible) {
            CompletionDialog.WindowState=FormWindowState.Normal;DisplayDialog(CompletionDialog);
        } else if(display==null){CompletionDialog.BringToFront();CompletionDialog.Activate();}
        // Keep one non-modal popup: simultaneous completions never cover each other or block other timers.
    }
    public void Dispose() {
        if(disposed)return;disposed=true;timer.Stop();timer.Dispose();
        foreach(var alarm in Alarms)alarm.Cancel();
        foreach(var dialog in new[]{SettingsDialog,CustomDialog,CompletionDialog})if(dialog!=null&&!dialog.IsDisposed){dialog.Close();dialog.Dispose();}
        Menu.Dispose();
    }
}
