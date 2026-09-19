// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Windows.Forms;

internal sealed partial class TaskTreeView {
    internal Func<bool> WindowDragEnabled;
    internal Action<Point> WindowDragRequested;
    bool windowDragGesture;

    internal bool IsWindowDragSpace(Point point) {
        if(!ClientRectangle.Contains(point))return false;
        // Owner-drawn links are interactive even when they extend beyond native label bounds.
        if(PriorityNodeAt(point)!=null || ImageNodeAt(point)!=null)return false;
        var hit=HitTest(point);
        const TreeViewHitTestLocations interactive=TreeViewHitTestLocations.Label|TreeViewHitTestLocations.Image|TreeViewHitTestLocations.StateImage|TreeViewHitTestLocations.PlusMinus;
        return (hit.Location&interactive)==0;
    }
    internal bool HandleWindowDragMessage(ref Message message) {
        const int LeftDown=0x201,LeftUp=0x202,LeftDoubleClick=0x203,MouseMove=0x200;
        if(windowDragGesture && (message.Msg==MouseMove || message.Msg==LeftUp || message.Msg==LeftDoubleClick))return true;
        if((message.Msg!=LeftDown && message.Msg!=LeftDoubleClick) || !Enabled || WindowDragEnabled==null || !WindowDragEnabled() || WindowDragRequested==null)return false;
        long coordinates=message.LParam.ToInt64();
        var point=new Point(unchecked((short)(coordinates&0xffff)),unchecked((short)((coordinates>>16)&0xffff)));
        if(!IsWindowDragSpace(point))return false;
        // Intercept before native TreeView processing so blank-row dragging cannot select/reorder a task.
        windowDragGesture=true;
        try {WindowDragRequested(point);}finally{windowDragGesture=false;}
        return true;
    }
}
internal sealed partial class FloatingWindow {
    long preferredProjectId;
    readonly Label simpleEmpty = new Label {BorderStyle=BorderStyle.FixedSingle, Padding=new Padding(8), TextAlign=ContentAlignment.MiddleCenter, BackColor=Color.White, ForeColor=Color.FromArgb(90,100,115), Visible=false};
    readonly FlowLayoutPanel simpleActions = new FlowLayoutPanel {WrapContents=false, FlowDirection=FlowDirection.LeftToRight, Visible=false, Margin=Padding.Empty, Padding=Padding.Empty};
    readonly Button fullToggleTask = new Button {Text="展开/收起", AutoSize=true, Visible=false, AccessibleName="展开或收起选中任务"};
    readonly Button fullAddOutstanding = new Button {Text="新遗留", AutoSize=true, AccessibleName="为选中任务添加遗留事项"};
    Action<long> addOutstandingRequested;
    ToolStripMenuItem trayRestoreFull, treeRestoreFull;
    bool simpleLayout, simpleActionsShown, simpleWindowWasMinimized, simpleRestoreLayoutQueued;
    int simpleViewportHeight=200, simpleActionsHeight;
    const int SimpleEdge=7, SimpleActionGap=6, SimpleMinimumViewport=54;

    void InitializeSimpleModeRecovery(ContextMenuStrip trayMenu) {
        restoreSimple.AutoSize=false;restoreSimple.AccessibleName="回到完整悬浮窗";
        minimizeSimple.AutoSize=false;minimizeSimple.AccessibleName="最小化到系统托盘";
        restoreSimple.Click+=delegate {RestoreFullFloatingWindow();};
        minimizeSimple.Click+=delegate {HideToTray();};
        addOutstandingRequested=delegate(long id){ShowOutstanding(id);};
        fullToggleTask.Click+=delegate {SetSelectedSimpleTaskExpanded(null);};
        fullAddOutstanding.Click+=delegate {AddSelectedOutstanding();};
        restoreSimple.Margin=new Padding(0,0,6,4);minimizeSimple.Margin=new Padding(0,0,0,4);simpleActions.Controls.AddRange(new Control[]{restoreSimple,minimizeSimple});
        Controls.Add(simpleActions);Controls.Add(simpleEmpty);
        Resize+=delegate {SimpleModeResized();};
        Activated+=delegate {ShowSimpleModeRestore();};
        VisibleChanged+=delegate {QueueSimpleRestoreLayout();};
        Deactivate+=delegate {HideSimpleModeActions();};
        tasks.WindowDragEnabled=delegate {return simpleMode && !closing && !dragging;};
        tasks.WindowDragRequested=delegate(Point point) {StartSimpleWindowDrag(tasks,point);};
        tasks.MouseDown+=delegate {ShowSimpleModeRestore();};
        tasks.MouseClick+=delegate {ShowSimpleModeRestore();};
        tasks.AfterSelect+=delegate {UpdateSimpleActionState();};
        tasks.AfterExpand+=delegate {UpdateSimpleActionState();};
        tasks.AfterCollapse+=delegate {UpdateSimpleActionState();};
        MouseDown+=SimpleSurfaceMouseDown;simpleEmpty.MouseDown+=SimpleSurfaceMouseDown;simpleActions.MouseDown+=SimpleSurfaceMouseDown;
        trayRestoreFull=new ToolStripMenuItem("回到完整悬浮窗",null,delegate {RestoreFullFloatingWindow();});
        trayMenu.Items.Insert(1,trayRestoreFull);
        treeRestoreFull=new ToolStripMenuItem("回到完整悬浮窗 (Esc)",null,delegate {RestoreFullFloatingWindow();});
        tasks.ContextMenuStrip.Items.Insert(0,treeRestoreFull);
        tasks.ContextMenuStrip.Opening+=delegate {treeRestoreFull.Visible=simpleMode;UpdateSimpleActionState();};
        var recoveryMenu=new ContextMenuStrip();
        recoveryMenu.Items.Add("回到完整悬浮窗 (Esc)",null,delegate {RestoreFullFloatingWindow();});
        recoveryMenu.Opening+=delegate(object sender,System.ComponentModel.CancelEventArgs e){e.Cancel=!simpleMode;};
        ContextMenuStrip=recoveryMenu;simpleEmpty.ContextMenuStrip=recoveryMenu;
    }
    TreeNode SelectedSimpleTaskNode() {
        long id=SelectedTaskId();if(id<=0)return null;
        return tasks.Nodes.Find(id.ToString(),true).FirstOrDefault(node=>node.Tag is long && (long)node.Tag==id);
    }
    void SetSelectedSimpleTaskExpanded(bool? expanded) {
        var node=SelectedSimpleTaskNode();if(busy || node==null || node.Nodes.Count==0)return;
        bool shouldExpand=expanded ?? !node.IsExpanded;
        if(shouldExpand)node.Expand();else node.Collapse();
        UpdateSimpleActionState();
    }
    void AddSelectedOutstanding() {
        if(busy || closing)return;
        var node=SelectedSimpleTaskNode();if(node==null)return;
        hoverTimer.Stop();progressTip.Hide(tasks);
        if(addOutstandingRequested!=null)addOutstandingRequested((long)node.Tag);
    }
    void UpdateSimpleActionState() {
        var node=SelectedSimpleTaskNode();
        bool canExpand=node!=null && node.Nodes.Cast<TreeNode>().Any(child=>child.Tag is long || child.Tag is OutstandingLeaf);
        bool available=!busy && canExpand;
        fullAddOutstanding.Enabled=!busy && node!=null;
        fullToggleTask.Visible=canExpand;fullToggleTask.Enabled=available;
        restoreSimple.Enabled=!closing;minimizeSimple.Enabled=!closing;
    }
    bool IsSimpleSurfaceDragSpace(Control surface,Point point) {
        if(surface==null || !surface.ClientRectangle.Contains(point))return false;
        if(surface==tasks)return tasks.IsWindowDragSpace(point);
        if(surface==simpleEmpty)return true;
        if(surface==this || surface==simpleActions)return surface.GetChildAtPoint(point,GetChildAtPointSkip.Invisible)==null;
        return false;
    }
    void SimpleSurfaceMouseDown(object sender,MouseEventArgs e) {
        if(!simpleMode)return;
        ShowSimpleModeRestore();
        var surface=sender as Control;
        if(e.Button==MouseButtons.Left && IsSimpleSurfaceDragSpace(surface,e.Location))StartSimpleWindowDrag(surface,e.Location);
    }
    void StartSimpleWindowDrag(Control surface,Point point) {
        if(!simpleMode || closing || dragging || WindowState!=FormWindowState.Normal)return;
        var screenPoint=surface.PointToScreen(point);
        ShowSimpleModeRestore();hoverTimer.Stop();hoverNode=null;progressTip.Hide(tasks);
        ReleaseCapture();
        int coordinates=(screenPoint.Y<<16)|(screenPoint.X&0xffff);
        SendSimpleMessage(Handle,0xA1,new IntPtr(2),new IntPtr(coordinates));
    }
    protected override bool ProcessCmdKey(ref Message message,Keys keys) {
        if(simpleMode && keys==Keys.Escape){RestoreFullFloatingWindow();return true;}
        if(!simpleMode && keys==Keys.Escape && addRow!=null && addRow.Visible){HideNewTaskEditor();tasks.Focus();return true;}
        return base.ProcessCmdKey(ref message,keys);
    }
    int MeasureSimpleActions(int width) {
        int available=Math.Max(120,width);
        int restoreWidth=Math.Max(56,TextRenderer.MeasureText(restoreSimple.Text,Font).Width+18);
        int minimizeWidth=Math.Max(64,TextRenderer.MeasureText(minimizeSimple.Text,Font).Width+18);
        int required=restoreWidth+restoreSimple.Margin.Horizontal+minimizeWidth+minimizeSimple.Margin.Horizontal;
        if(required>available) {
            int overflow=required-available;minimizeWidth=Math.Max(52,minimizeWidth-(overflow+1)/2);restoreWidth=Math.Max(48,restoreWidth-overflow/2);
        }
        int height=Math.Max(28,TextRenderer.MeasureText("最小化",Font).Height+10);
        restoreSimple.Size=new Size(restoreWidth,height);minimizeSimple.Size=new Size(minimizeWidth,height);
        return height+Math.Max(restoreSimple.Margin.Vertical,minimizeSimple.Margin.Vertical);
    }
    void SimpleModeResized() {
        if(!simpleMode || simpleLayout)return;
        if(WindowState!=FormWindowState.Normal){simpleWindowWasMinimized=true;return;}
        if(simpleWindowWasMinimized){QueueSimpleRestoreLayout();return;}
        // User resizing changes the viewport; focus only adds/removes the action strip below it.
        int previousExtra=simpleActionsShown?SimpleActionGap+simpleActionsHeight:0;
        simpleViewportHeight=Math.Max(SimpleMinimumViewport,ClientSize.Height-2*SimpleEdge-previousExtra);
        simpleActionsHeight=MeasureSimpleActions(ClientSize.Width-2*SimpleEdge);
        simpleSize=new Size(Math.Max(160,ClientSize.Width),simpleSize.Height);
        PositionSimpleModeControls();
    }
    void PositionSimpleModeControls() {
        if(!simpleMode || simpleLayout || simpleWindowWasMinimized || closing || IsDisposed || WindowState!=FormWindowState.Normal)return;
        simpleLayout=true;
        try {
            int width=Math.Max(160,simpleSize.Width);
            simpleActionsHeight=MeasureSimpleActions(width-2*SimpleEdge);
            int extra=SimpleActionGap+simpleActionsHeight;
            simpleViewportHeight=Math.Max(SimpleMinimumViewport,simpleViewportHeight);
            int plainHeight=simpleViewportHeight+2*SimpleEdge;
            simpleSize=new Size(width,plainHeight+extra);
            MinimumSize=new Size(160,SimpleMinimumViewport+2*SimpleEdge+(simpleActionsShown?extra:0));
            ClientSize=new Size(width,plainHeight+(simpleActionsShown?extra:0));
            var viewport=new Rectangle(SimpleEdge,SimpleEdge,width-2*SimpleEdge,simpleViewportHeight);
            tasks.Bounds=viewport;simpleEmpty.Bounds=viewport;
            simpleActions.Bounds=new Rectangle(SimpleEdge,viewport.Bottom+SimpleActionGap,width-2*SimpleEdge,simpleActionsHeight);
            simpleActions.Visible=simpleActionsShown;
            restoreSimple.Visible=true;minimizeSimple.Visible=true;
            simpleActions.PerformLayout();
            if(simpleActionsShown)simpleActions.BringToFront();
        } finally {simpleLayout=false;}
    }
    void QueueSimpleRestoreLayout() {
        if(!simpleMode || !simpleWindowWasMinimized || simpleRestoreLayoutQueued || closing || IsDisposed || !IsHandleCreated || !Visible || WindowState!=FormWindowState.Normal)return;
        simpleRestoreLayoutQueued=true;
        BeginInvoke(new Action(delegate {
            simpleRestoreLayoutQueued=false;
            if(!simpleMode || closing || IsDisposed || !Visible || WindowState!=FormWindowState.Normal)return;
            simpleWindowWasMinimized=false;PositionSimpleModeControls();UpdateSimpleActionState();
        }));
    }
    void ShowSimpleModeRestore() {
        if(!simpleMode || closing || IsDisposed)return;
        simpleActionsShown=true;QueueSimpleRestoreLayout();PositionSimpleModeControls();UpdateSimpleActionState();
    }
    void HideSimpleModeActions() {
        if(!simpleMode || simpleLayout || closing || IsDisposed)return;
        simpleActionsShown=false;PositionSimpleModeControls();
    }
    void UpdateSimpleModeState() {
        if(closing || IsDisposed)return;
        simpleEmpty.Visible=simpleMode && tasks.Nodes.Count==0;
        if(simpleMode)tasks.Visible=tasks.Nodes.Count>0;
        if(simpleEmpty.Visible) {
            if(busy)simpleEmpty.Text="正在读取事项…";
            else if(status.Text.StartsWith("操作失败"))simpleEmpty.Text="读取失败，请返回后重试。";
            else if(projects.SelectedItem==null)simpleEmpty.Text="暂无项目。";
            else if(search.Text.Trim().Length>0)simpleEmpty.Text="搜索无匹配事项。";
            else if(PriorityFilterActive)simpleEmpty.Text=PriorityFilterEmptyMessage;
            else if(!showCompleted.Checked)simpleEmpty.Text="暂无未完成事项。";
            else simpleEmpty.Text="当前项目暂无事项。";
            simpleEmpty.BringToFront();
        }
        PositionSimpleModeControls();UpdateSimpleActionState();
    }
    void RestoreFullFloatingWindow() {
        if(closing || IsDisposed)return;
        WindowState=FormWindowState.Normal;RestoreFromEdge(true);SetSimpleMode(false);
        if(collapsed)ToggleFold();RestoreWindow();tasks.Focus();
    }
    void SetSimpleMode(bool enabled) {
        if(enabled==simpleMode){UpdateSimpleModeState();return;}
        RestoreFromEdge(true);
        var selected=tasks.SelectedNode;var top=tasks.TopNode;
        SuspendLayout();content.SuspendLayout();simpleLayout=true;
        try {
            if(enabled) {
                if(collapsed)ToggleFold();fullBounds=Bounds;simpleMode=true;
                UpdateSimpleActionState();
                content.Controls.Remove(tasks);content.Visible=false;toolbar.Visible=false;
                FormBorderStyle=FormBorderStyle.None;Padding=Padding.Empty;
                Controls.Add(tasks);tasks.Dock=DockStyle.None;tasks.Visible=true;tasks.BringToFront();
                tasks.SetWrappedText(!singleLine.Checked);
                simpleActionsHeight=MeasureSimpleActions(Math.Max(160,simpleSize.Width)-2*SimpleEdge);
                simpleViewportHeight=Math.Max(SimpleMinimumViewport,simpleSize.Height-2*SimpleEdge-SimpleActionGap-simpleActionsHeight);
                simpleActionsShown=ContainsFocus || ActiveForm==this;
                MinimumSize=new Size(160,SimpleMinimumViewport+2*SimpleEdge);ClientSize=new Size(Math.Max(160,simpleSize.Width),Math.Max(120,simpleSize.Height));
            } else {
                // simpleSize already contains the stable expanded dimensions, even while unfocused.
                simpleMode=false;simpleWindowWasMinimized=false;tasks.SetWrappedText(false);Controls.Remove(tasks);Padding=Padding.Empty;simpleActions.Visible=false;simpleEmpty.Visible=false;
                FormBorderStyle=FormBorderStyle.Sizable;MinimumSize=new Size(350,420);
                content.Controls.Add(tasks,0,3);tasks.Dock=DockStyle.Fill;tasks.Visible=true;content.Visible=true;toolbar.Visible=true;Bounds=fullBounds;
            }
        } finally {simpleLayout=false;content.ResumeLayout(true);ResumeLayout(true);}
        PerformLayout();PositionSimpleModeControls();tasks.RefreshWrappedLayout();UpdateSimpleModeState();
        if(selected!=null && selected.TreeView==tasks)tasks.SelectedNode=selected;
        if(top!=null && top.TreeView==tasks)tasks.TopNode=top;
        tasks.Invalidate();SaveSimpleMode();
        if(edgeHideEnabled && !Bounds.Contains(Cursor.Position))BeginInvoke(new Action(delegate {TryHideAtTouchedEdge();}));
    }
    void SaveSimpleMode() {
        try {File.WriteAllText(Path.Combine(data,"simple-window.json"),json.Serialize(new {enabled=simpleMode,width=simpleSize.Width,height=simpleSize.Height}));}catch{}
    }
    void TestSimpleModeRecovery() {
        var originalNodes=tasks.Nodes.Cast<TreeNode>().ToArray();
        var originalBounds=Bounds;string originalStatus=status.Text,originalSearch=search.Text;bool originalCompleted=showCompleted.Checked;
        rendering=true;
        try {
            tasks.Nodes.Clear();search.Text="无匹配的查询";status.Text="没有匹配事项，可清空搜索或显示已完成。";
            SetSimpleMode(true);Size=new Size(230,260);ShowSimpleModeRestore();UpdateSimpleModeState();
            if(tasks.Visible || tasks.Width<100 || tasks.Height<54 || !simpleEmpty.Visible || !restoreSimple.Visible || !minimizeSimple.Visible || !simpleEmpty.Text.Contains("搜索"))throw new Exception("Empty simple mode must show its reason and recovery controls");
            var buttonPoint=simpleActions.PointToClient(restoreSimple.PointToScreen(new Point(restoreSimple.Width/2,restoreSimple.Height/2)));
            if(simpleActions.GetChildAtPoint(buttonPoint)!=restoreSimple || simpleEmpty.Bounds.IntersectsWith(simpleActions.Bounds))throw new Exception("Recovery button is obscured in empty simple mode");
            var emptyArea=simpleEmpty.Bounds;var savedSize=simpleSize;OnDeactivate(EventArgs.Empty);
            if(simpleActions.Visible || ClientSize.Height-simpleEmpty.Bottom!=SimpleEdge || simpleEmpty.Bounds!=emptyArea || simpleSize!=savedSize)throw new Exception("Unselected empty mode retained a footer or changed its viewport");
            OnActivated(EventArgs.Empty);
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-simple-empty-test.png"));}
            restoreSimple.PerformClick();if(simpleMode || Bounds!=originalBounds)throw new Exception("Empty simple mode button failed to restore original bounds");
            SetSimpleMode(true);HideSimpleModeActions();
            typeof(Control).GetMethod("OnMouseClick",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(tasks,new object[]{new MouseEventArgs(MouseButtons.Left,1,20,80,0)});
            if(!restoreSimple.Visible)throw new Exception("Blank surface click failed to expose recovery controls");
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
        SetSimpleMode(true);Size=new Size(230,260);ShowSimpleModeRestore();
        if(!tasks.Visible || tasks.Nodes.Count!=originalNodes.Length || tasks.Bounds.Width<100 || tasks.GetNodeAt(tasks.Nodes[0].Bounds.Location)==null)throw new Exception("Populated simple tree is not visible and hit-testable");
        var taskArea=tasks.Bounds;var row=tasks.Nodes[0];var rowPoint=tasks.PointToScreen(row.Bounds.Location);var topNode=tasks.TopNode;var stableSize=simpleSize;
        for(int i=0;i<3;i++) {
            OnDeactivate(EventArgs.Empty);
            if(simpleActions.Visible || ClientSize.Height-tasks.Bottom!=SimpleEdge || tasks.Bounds!=taskArea || tasks.TopNode!=topNode || tasks.PointToScreen(row.Bounds.Location)!=rowPoint)throw new Exception("Unselected mode moved a task or retained its footer");
            OnActivated(EventArgs.Empty);
            if(!restoreSimple.Visible || tasks.Bounds!=taskArea || tasks.TopNode!=topNode || tasks.PointToScreen(row.Bounds.Location)!=rowPoint || simpleSize!=stableSize)throw new Exception("Activating simple mode moved a task or changed saved dimensions");
        }
        WindowState=FormWindowState.Minimized;RestoreWindow();ShowSimpleModeRestore();
        if(simpleSize!=stableSize || tasks.Bounds!=taskArea)throw new Exception("Minimize/tray restore changed the saved size or viewport");
        minimizeSimple.PerformClick();
        if(Visible || !simpleMode || ShowInTaskbar)throw new Exception("Simple minimize button did not hide to the notification area");
        typeof(NotifyIcon).GetMethod("OnDoubleClick",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(tray,new object[]{EventArgs.Empty});
        ShowSimpleModeRestore();
        if(!Visible || !simpleMode || WindowState!=FormWindowState.Normal || ShowInTaskbar)throw new Exception("Tray double-click did not restore the simple floating window");
        var owner=tasks.Nodes.Cast<TreeNode>().FirstOrDefault(node=>node.Tag is long);
        if(owner!=null) {
            bool wasExpanded=owner.IsExpanded;var leaf=new TreeNode("测试遗留项") {Tag=new OutstandingLeaf{TaskId=(long)owner.Tag,Id="simple-toggle-test",Html="测试"}};
            bool wasRendering=rendering;rendering=true;
            try {owner.Nodes.Add(leaf);owner.Expand();tasks.SelectedNode=leaf;SetSelectedSimpleTaskExpanded(false);if(owner.IsExpanded)throw new Exception("Selected outstanding item did not collapse its owning task");tasks.SelectedNode=owner;SetSelectedSimpleTaskExpanded(true);if(!owner.IsExpanded)throw new Exception("Selected task did not expand");}
            finally {leaf.Remove();if(!wasExpanded)owner.Collapse();rendering=wasRendering;}
        }
        SetSimpleMode(false);
        File.WriteAllText(Path.Combine(data,"floating-simple-recovery-test.txt"),"PASS: inactive mode has no reserved footer; focus/mousedown preserve viewport and persisted size; full/minimize footer fits; minimize hides to tray and tray double-click restores; selected task and outstanding owner expand/collapse; empty explanations; Escape/context/tray recovery; selected project persists.");
    }
    void TestSimpleOutstandingActions() {
        var originalNodes=tasks.Nodes.Cast<TreeNode>().ToArray();var originalSelection=tasks.SelectedNode;
        bool originalMode=simpleMode,originalBusy=busy,originalTesting=simpleDetailsTesting,originalRendering=rendering;
        var originalBounds=Bounds;var originalSize=simpleSize;var callback=addOutstandingRequested;
        simpleDetailsTesting=true;InvalidateSimpleOutstanding();rendering=true;
        try {
            SetSimpleMode(false);tasks.Nodes.Clear();
            var owner=new TreeNode("1. [P9] 待办任务"){Name="910001",Tag=910001L};
            var child=new TreeNode("1.1. [P9] 子任务"){Name="910002",Tag=910002L};
            tasks.Nodes.Add(owner);SetSimpleMode(true);SetBusy(false);ShowSimpleModeRestore();Size=new Size(230,300);
            tasks.SelectedNode=owner;UpdateSimpleActionState();
            if(simpleActions.Controls.Count!=2 || simpleActions.Controls[0]!=restoreSimple || simpleActions.Controls[1]!=minimizeSimple || !restoreSimple.Visible || !restoreSimple.Enabled)throw new Exception("Simple mode must expose the full and minimize buttons");
            long requested=0;int clicks=0;addOutstandingRequested=delegate(long id){requested=id;clicks++;};
            var shared=new SharedList();shared.Items.Add(new PendingItem{Id="action-test",Html="核对遗留事项"});
            ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{910001L,shared}});UpdateSimpleActionState();
            if(owner.Nodes.Count!=1 || !(owner.Nodes[0].Tag is OutstandingLeaf) || simpleActions.Controls.Count!=2)throw new Exception("Direct outstanding item changed the two-button footer");
            var leaf=owner.Nodes[0];tasks.SelectedNode=leaf;owner.Collapse();
            if(owner.IsExpanded)throw new Exception("Task tree failed to collapse in simple mode");
            owner.Expand();if(!owner.IsExpanded)throw new Exception("Task tree failed to expand in simple mode");
            ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{910001L,new SharedList()}});UpdateSimpleActionState();
            if(owner.Nodes.Count!=0 || simpleActions.Controls.Count!=2)throw new Exception("Removing the last outstanding item changed the two-button footer");
            owner.Nodes.Add(child);owner.Expand();tasks.SelectedNode=owner;UpdateSimpleActionState();
            if(!owner.IsExpanded || simpleActions.Controls.Count!=2)throw new Exception("Actual subtasks must remain expandable without changing simple-mode buttons");
            tasks.SelectedNode=child;UpdateSimpleActionState();
            var viewport=tasks.Bounds;var savedSize=simpleSize;
            HideSimpleModeActions();ShowSimpleModeRestore();
            if(tasks.Bounds!=viewport || simpleSize!=savedSize)throw new Exception("Conditional actions moved the viewport across focus changes");
            if(!simpleActions.Controls.Cast<Control>().Where(control=>control.Visible).SequenceEqual(new Control[]{restoreSimple,minimizeSimple}) || restoreSimple.Left<0 || minimizeSimple.Right+minimizeSimple.Margin.Right>simpleActions.ClientSize.Width || Math.Max(restoreSimple.Bottom+restoreSimple.Margin.Bottom,minimizeSimple.Bottom+minimizeSimple.Margin.Bottom)>simpleActions.ClientSize.Height)throw new Exception("Simple footer is clipped or contains another action");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-simple-outstanding-actions-test.png"));}
            SetBusy(true);if(!restoreSimple.Enabled || !minimizeSimple.Enabled || simpleActions.Controls.Count!=2)throw new Exception("Busy state disabled a simple-mode window action");
            SetBusy(false);
            SetSimpleMode(false);tasks.SelectedNode=child;UpdateSimpleActionState();
            var bottomLabels=bottomActions.Controls.Cast<Control>().Select(control=>control.Text).ToArray();
            if(!bottomLabels.SequenceEqual(new[]{"记录进展","新事项","新任务","新遗留"}) || !fullAddOutstanding.Visible || !fullAddOutstanding.Enabled || fullAddOutstanding.Parent!=bottomActions || priorityFilterButton.Parent!=null || fullToggleTask.Parent!=null)throw new Exception("Full mode actions or compact toolbar controls are incorrect");
            fullAddOutstanding.PerformClick();if(clicks!=1 || requested!=910002L)throw new Exception("Full add action did not target the selected task");
            var normalLeaf=new TreeNode("1. 待核对"){Tag=new OutstandingLeaf{TaskId=910002L,Id="full-action",Html="待核对"}};
            child.Nodes.Add(normalLeaf);owner.Expand();child.Expand();
            tasks.SelectedNode=normalLeaf;UpdateSimpleActionState();fullAddOutstanding.PerformClick();
            if(clicks!=2 || requested!=910002L)throw new Exception("Full actions do not support a task with only direct outstanding items");
            tasks.SelectedNode=child;fullAddOutstanding.PerformClick();if(clicks!=3 || requested!=910002L)throw new Exception("Full add action did not resolve the selected child");
            SetSelectedSimpleTaskExpanded(false);if(child.IsExpanded)throw new Exception("Task collapse action did not target the selected outstanding owner");
            tasks.SelectedNode=child;SetSelectedSimpleTaskExpanded(true);if(!child.IsExpanded)throw new Exception("Task expansion action failed");
            SetBusy(true);fullAddOutstanding.PerformClick();if(clicks!=3 || fullAddOutstanding.Enabled)throw new Exception("Full actions remained usable while busy");
            SetBusy(false);tasks.SelectedNode=null;UpdateSimpleActionState();fullAddOutstanding.PerformClick();
            if(clicks!=3 || fullAddOutstanding.Enabled)throw new Exception("Full actions accepted an empty selection");
            tasks.SelectedNode=child;UpdateSimpleActionState();
            Action assertToolbar=delegate {
                PerformLayout();toolbar.PerformLayout();
                var controls=toolbar.Controls.Cast<Control>().Where(control=>control.Visible).ToArray();
                if(!controls.Select(control=>control.Text).SequenceEqual(new[]{"完整界面","置顶","设置","简洁模式"}) || controls.Select(control=>control.Top).Distinct().Count()!=1)throw new Exception("Full toolbar is not a single compact row");
                foreach(var control in controls)if(control.Left<toolbar.Padding.Left || control.Right+control.Margin.Right>toolbar.ClientSize.Width-toolbar.Padding.Right || control.Bottom+control.Margin.Bottom>toolbar.ClientSize.Height-toolbar.Padding.Bottom)throw new Exception("A full toolbar action was clipped: "+control.Text);
                int bottom=controls.Max(control=>control.Bottom+control.Margin.Bottom)+toolbar.Padding.Bottom;
                if(Math.Abs(toolbar.Height-bottom)>1)throw new Exception("Full toolbar retained empty rows: height="+toolbar.Height+", used="+bottom);
                if(!collapsed && content.Top!=toolbar.Bottom)throw new Exception("Full content did not follow its fitted toolbar");
            };
            foreach(int width in new[]{350,400,900}) {
                Size=new Size(width,600);assertToolbar();
                if(width!=350)using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-full-toolbar-"+width+"-test.png"));}
            }
            if(toolbar.Controls.Cast<Control>().Any(control=>new[]{"收起","展开","展开/收起","刷新","撤销","查看图片","新事项","按优先级"}.Contains(control.Text)))throw new Exception("A removed toolbar action is still visible");
            var fullArea=Bounds;tasks.SelectedNode=normalLeaf;tasks.TopNode=owner;
            var top=tasks.TopNode;int loads=taskLoadVersion,reads=simpleOutstandingVersion;
            string context=TaskViewContext();
            SetSimpleMode(true);
            if(tasks.SelectedNode!=normalLeaf || normalLeaf.Parent!=child || child.Parent!=owner || !owner.IsExpanded || !child.IsExpanded || tasks.TopNode!=top || !simpleActions.Controls.Cast<Control>().Where(control=>control.Visible).SequenceEqual(new Control[]{restoreSimple,minimizeSimple}))throw new Exception("Simple layout switch changed shared tree state or exposed another footer action");
            SetSimpleMode(false);assertToolbar();
            if(tasks.SelectedNode!=normalLeaf || normalLeaf.Parent!=child || !child.IsExpanded || tasks.TopNode!=top || taskLoadVersion!=loads || simpleOutstandingVersion!=reads || TaskViewContext()!=context)throw new Exception("Mode switching refreshed data or lost shared tree state");
            if(Bounds!=fullArea || !fullAddOutstanding.Visible)throw new Exception("Mode switching lost full bounds or actions");
            File.WriteAllText(Path.Combine(data,"floating-full-actions-test.txt"),"PASS: exactly four bottom actions for progress, new item, new task and new outstanding; task-tree expand/collapse; direct outstanding targets; shared node identity, selection, scroll and expansion across layout-only switches without new reads; busy/empty selection guards; compact/wide layouts with one compact toolbar row and no clipped actions; removed toolbar actions absent; mode switches retained.");
            File.WriteAllText(Path.Combine(data,"floating-simple-outstanding-actions-test.txt"),"PASS: simple mode exposes full and minimize buttons; task-tree expand/collapse and direct outstanding items remain usable; full and minimize remain available while busy; narrow footer fits; focus preserves viewport.");
        } finally {
            addOutstandingRequested=callback;rendering=true;if(collapsed)ToggleFold();SetSimpleMode(false);tasks.Nodes.Clear();tasks.Nodes.AddRange(originalNodes);
            if(originalSelection!=null && originalSelection.TreeView==tasks)tasks.SelectedNode=originalSelection;
            simpleSize=originalSize;if(originalMode)SetSimpleMode(true);Bounds=originalBounds;
            simpleDetailsTesting=originalTesting;rendering=originalRendering;SetBusy(originalBusy);UpdateSimpleModeState();
        }
    }

}
