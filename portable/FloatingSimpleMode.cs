// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    long preferredProjectId;
    readonly Label simpleEmpty = new Label {BorderStyle=BorderStyle.FixedSingle, Padding=new Padding(8), TextAlign=ContentAlignment.MiddleCenter, BackColor=Color.White, ForeColor=Color.FromArgb(90,100,115), Visible=false};
    ToolStripMenuItem trayRestoreFull, treeRestoreFull;

    void InitializeSimpleModeRecovery(ContextMenuStrip trayMenu) {
        restoreSimple.AutoSize=false;
        restoreSimple.AccessibleName="回到完整悬浮窗";
        restoreSimple.Click+=delegate {RestoreFullFloatingWindow();};
        Controls.Add(restoreSimple);Controls.Add(simpleEmpty);
        Resize+=delegate {PositionSimpleModeControls();};
        Activated+=delegate {ShowSimpleModeRestore();};
        Deactivate+=delegate {if(simpleMode && tasks.Nodes.Count>0){restoreSimple.Visible=false;PositionSimpleModeControls();}};
        tasks.MouseDown+=SimpleSurfaceMouseDown;
        tasks.MouseClick+=delegate {ShowSimpleModeRestore();};
        MouseDown+=SimpleSurfaceMouseDown;simpleEmpty.MouseDown+=SimpleSurfaceMouseDown;
        trayRestoreFull=new ToolStripMenuItem("回到完整悬浮窗",null,delegate {RestoreFullFloatingWindow();});
        trayMenu.Items.Insert(1,trayRestoreFull);
        treeRestoreFull=new ToolStripMenuItem("回到完整悬浮窗 (Esc)",null,delegate {RestoreFullFloatingWindow();});
        tasks.ContextMenuStrip.Items.Insert(0,treeRestoreFull);
        tasks.ContextMenuStrip.Opening+=delegate {treeRestoreFull.Visible=simpleMode;};
        var recoveryMenu=new ContextMenuStrip();
        recoveryMenu.Items.Add("回到完整悬浮窗 (Esc)",null,delegate {RestoreFullFloatingWindow();});
        recoveryMenu.Opening+=delegate(object sender,System.ComponentModel.CancelEventArgs e){e.Cancel=!simpleMode;};
        ContextMenuStrip=recoveryMenu;simpleEmpty.ContextMenuStrip=recoveryMenu;
        PositionSimpleModeControls();
    }
    void SimpleSurfaceMouseDown(object sender,MouseEventArgs e) {
        if(!simpleMode)return;
        ShowSimpleModeRestore();
        if(e.Button==MouseButtons.Left && (sender!=tasks || tasks.GetNodeAt(e.Location)==null)) {
            ReleaseCapture();SendMessage(Handle,0xA1,new IntPtr(2),null);
        }
    }
    protected override bool ProcessCmdKey(ref Message message,Keys keys) {
        if(simpleMode && keys==Keys.Escape){RestoreFullFloatingWindow();return true;}
        return base.ProcessCmdKey(ref message,keys);
    }
    void PositionSimpleModeControls() {
        int width=Math.Max(80,Math.Min(restoreSimple.GetPreferredSize(Size.Empty).Width,ClientSize.Width-20));
        var text=TextRenderer.MeasureText(restoreSimple.Text,Font,new Size(Math.Max(60,width-12),0),TextFormatFlags.WordBreak);
        restoreSimple.Size=new Size(width,Math.Max(28,text.Height+10));
        restoreSimple.Location=new Point(Math.Max(7,ClientSize.Width-width-10),Math.Max(7,ClientSize.Height-restoreSimple.Height-10));
        // Keep node positions fixed while recovery controls appear or disappear during a click.
        int top=7, bottom=restoreSimple.Top-8;
        if(simpleMode)tasks.Bounds=new Rectangle(7,top,Math.Max(60,ClientSize.Width-14),Math.Max(20,bottom-top));
        simpleEmpty.Bounds=new Rectangle(7,top,Math.Max(60,ClientSize.Width-14),Math.Max(20,bottom-top));
    }
    void ShowSimpleModeRestore() {
        if(!simpleMode || closing || IsDisposed)return;
        restoreSimple.Visible=true;PositionSimpleModeControls();restoreSimple.BringToFront();
    }
    void UpdateSimpleModeState() {
        if(closing || IsDisposed)return;
        simpleEmpty.Visible=simpleMode && tasks.Nodes.Count==0;
        if(simpleMode)tasks.Visible=tasks.Nodes.Count>0;
        if(!simpleEmpty.Visible){PositionSimpleModeControls();return;}
        if(busy)simpleEmpty.Text="正在读取事项…";
        else if(status.Text.StartsWith("操作失败"))simpleEmpty.Text="读取失败，请返回后重试。";
        else if(projects.SelectedItem==null)simpleEmpty.Text="暂无项目。";
        else if(search.Text.Trim().Length>0)simpleEmpty.Text="搜索无匹配事项。";
        else if(!showCompleted.Checked)simpleEmpty.Text="暂无未完成事项。";
        else simpleEmpty.Text="当前项目暂无事项。";
        PositionSimpleModeControls();simpleEmpty.BringToFront();ShowSimpleModeRestore();
    }
    void RestoreFullFloatingWindow() {
        if(closing || IsDisposed)return;
        // Restore the size before leaving simple mode if the window was minimized to the tray.
        WindowState=FormWindowState.Normal;
        SetSimpleMode(false);
        if(collapsed)ToggleFold();
        RestoreWindow();tasks.Focus();
    }
    void SetSimpleMode(bool enabled) {
        if(enabled==simpleMode){UpdateSimpleModeState();return;}
        SuspendLayout();content.SuspendLayout();
        try {
            if(enabled) {
                if(collapsed)ToggleFold();fullBounds=Bounds;simpleMode=true;
                content.Controls.Remove(tasks);content.Visible=false;toolbar.Visible=false;
                FormBorderStyle=FormBorderStyle.None;MinimumSize=new Size(160,120);Padding=new Padding(7);
                Controls.Add(tasks);tasks.Dock=DockStyle.None;tasks.Visible=true;tasks.BringToFront();Size=simpleSize;
            } else {
                simpleSize=Size;simpleMode=false;Controls.Remove(tasks);Padding=Padding.Empty;
                FormBorderStyle=FormBorderStyle.Sizable;MinimumSize=new Size(350,420);
                content.Controls.Add(tasks,0,3);tasks.Dock=DockStyle.Fill;tasks.Visible=true;content.Visible=true;toolbar.Visible=true;Bounds=fullBounds;
            }
            restoreSimple.Visible=false;
        } finally {content.ResumeLayout(true);ResumeLayout(true);}
        PerformLayout();tasks.Invalidate();PositionSimpleModeControls();UpdateSimpleModeState();SaveSimpleMode();
    }
    void SaveSimpleMode() {
        try {if(simpleMode && WindowState==FormWindowState.Normal)simpleSize=Size;File.WriteAllText(Path.Combine(data,"simple-window.json"),json.Serialize(new {enabled=simpleMode,width=simpleSize.Width,height=simpleSize.Height}));}catch{}
    }
    void TestSimpleModeRecovery() {
        var originalNodes=tasks.Nodes.Cast<TreeNode>().ToArray();
        var originalBounds=Bounds;string originalStatus=status.Text,originalSearch=search.Text;bool originalCompleted=showCompleted.Checked;
        rendering=true;
        try {
            tasks.Nodes.Clear();search.Text="无匹配的查询";status.Text="没有匹配事项，可清空搜索或显示已完成。";
            SetSimpleMode(true);Size=new Size(230,220);UpdateSimpleModeState();
            if(tasks.Visible || tasks.Width<100 || tasks.Height<100 || !simpleEmpty.Visible || !restoreSimple.Visible || !simpleEmpty.Text.Contains("搜索"))throw new Exception("Empty simple mode must show its reason and a reachable restore button");
            if(GetChildAtPoint(new Point(restoreSimple.Left+restoreSimple.Width/2,restoreSimple.Top+restoreSimple.Height/2))!=restoreSimple || simpleEmpty.Bounds.IntersectsWith(restoreSimple.Bounds))throw new Exception("Recovery button is obscured in empty simple mode");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-simple-empty-test.png"));}
            restoreSimple.PerformClick();if(simpleMode || Bounds!=originalBounds)throw new Exception("Empty simple mode button failed to restore original bounds");
            SetSimpleMode(true);restoreSimple.Visible=false;
            typeof(Control).GetMethod("OnMouseClick",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(tasks,new object[]{new MouseEventArgs(MouseButtons.Left,1,20,80,0)});
            if(!restoreSimple.Visible)throw new Exception("Blank surface click failed to expose the restore button");
            Message key=Message.Create(Handle,0x100,IntPtr.Zero,IntPtr.Zero);if(!ProcessCmdKey(ref key,Keys.Escape) || simpleMode)throw new Exception("Escape failed to recover empty simple mode");
            SetSimpleMode(true);treeRestoreFull.PerformClick();if(simpleMode)throw new Exception("Simple context menu failed to restore full mode");
            SetSimpleMode(true);HideToTray();trayRestoreFull.PerformClick();if(simpleMode || !Visible || ShowInTaskbar)throw new Exception("Tray recovery failed for hidden simple mode");
            search.Text="";showCompleted.Checked=false;SetSimpleMode(true);UpdateSimpleModeState();
            if(!simpleEmpty.Text.Contains("未完成"))throw new Exception("Completed-only filter has no empty-state explanation");
            showCompleted.Checked=true;UpdateSimpleModeState();if(!simpleEmpty.Text.Contains("暂无事项"))throw new Exception("Empty project has no explanation");
            var sourceProject=projects.SelectedItem as Project;if(sourceProject!=null){SaveBounds();var saved=ReadObject(File.ReadAllText(Path.Combine(data,"floating-window.json")));if(Convert.ToInt64(saved["projectId"])!=sourceProject.Id)throw new Exception("Selected project was not persisted");}
            SetSimpleMode(false);
        } finally {
            search.Text=originalSearch;showCompleted.Checked=originalCompleted;status.Text=originalStatus;
            if(simpleMode)SetSimpleMode(false);tasks.Nodes.Clear();tasks.Nodes.AddRange(originalNodes);rendering=false;UpdateSimpleModeState();
        }
        SetSimpleMode(true);Size=new Size(230,220);
        if(!tasks.Visible || tasks.Nodes.Count!=originalNodes.Length || tasks.Bounds.Width<100 || tasks.GetNodeAt(tasks.Nodes[0].Bounds.Location)==null)throw new Exception("Populated simple tree is not visible and hit-testable");
        restoreSimple.Visible=false;PositionSimpleModeControls();var taskArea=tasks.Bounds;OnActivated(EventArgs.Empty);if(tasks.Bounds!=taskArea || !restoreSimple.Visible || tasks.Bounds.IntersectsWith(restoreSimple.Bounds) || GetChildAtPoint(new Point(restoreSimple.Left+restoreSimple.Width/2,restoreSimple.Top+restoreSimple.Height/2))!=restoreSimple)throw new Exception("Activating simple mode did not expose a hit-testable recovery button");
        SetSimpleMode(false);
        File.WriteAllText(Path.Combine(data,"floating-simple-recovery-test.txt"),"PASS: empty project and search/filter explanations; empty-area click, activation, Escape, context menu and tray recovery; original bounds restored; populated tree visible and hit-testable; selected project persisted.");
    }
}
