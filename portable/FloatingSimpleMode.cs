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
    readonly FlowLayoutPanel simpleActions = new FlowLayoutPanel {WrapContents=true, FlowDirection=FlowDirection.LeftToRight, Visible=false, Margin=Padding.Empty, Padding=Padding.Empty};
    readonly Button simpleToggleTask = new Button {Text="展开/收起", AccessibleName="展开或收起选中任务", Visible=false};
    readonly Button simpleAddOutstanding = new Button {Text="添加遗留事项", AccessibleName="为选中任务添加遗留事项"};
    readonly Button fullToggleTask = new Button {Text="展开/收起", AutoSize=true, Visible=false, AccessibleName="展开或收起选中任务"};
    readonly Button fullAddOutstanding = new Button {Text="添加遗留事项", AutoSize=true, AccessibleName="为选中任务添加遗留事项"};
    Action<long> addOutstandingRequested;
    bool simpleTaskCanExpand;
    readonly Button simplePriorityButton = new Button {Text="按优先级", AccessibleName="按优先级筛选"};
    ToolStripMenuItem trayRestoreFull, treeRestoreFull, simpleExpandTaskMenu, simpleCollapseTaskMenu;
    bool simpleLayout, simpleActionsShown, simpleWindowWasMinimized, simpleRestoreLayoutQueued;
    int simpleViewportHeight=200, simpleActionsHeight;
    const int SimpleEdge=7, SimpleActionGap=6, SimpleMinimumViewport=54;

    void InitializeSimpleModeRecovery(ContextMenuStrip trayMenu) {
        restoreSimple.AutoSize=false;restoreSimple.AccessibleName="回到完整悬浮窗";
        restoreSimple.Click+=delegate {RestoreFullFloatingWindow();};
        simpleToggleTask.Click+=delegate {SetSelectedSimpleTaskExpanded(null);};
        addOutstandingRequested=delegate(long id){ShowOutstanding(id);};
        simpleAddOutstanding.Click+=delegate {AddSelectedOutstanding();};
        fullToggleTask.Click+=delegate {SetSelectedSimpleTaskExpanded(null);};
        fullAddOutstanding.Click+=delegate {AddSelectedOutstanding();};
        toolbar.Controls.AddRange(new Control[]{fullToggleTask,fullAddOutstanding});
        var priorityMenu=CreatePriorityFilterDropDown();
        simplePriorityButton.Click+=delegate {if(!closing && !IsDisposed)priorityMenu.Show(simplePriorityButton,new Point(0,simplePriorityButton.Height));};
        foreach(var button in new[]{simpleToggleTask,simpleAddOutstanding,simplePriorityButton,restoreSimple}) {
            button.AutoSize=false;button.Margin=new Padding(0,0,6,4);simpleActions.Controls.Add(button);
        }
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
        simpleExpandTaskMenu=new ToolStripMenuItem("展开选中任务",null,delegate {SetSelectedSimpleTaskExpanded(true);});
        simpleCollapseTaskMenu=new ToolStripMenuItem("收起选中任务",null,delegate {SetSelectedSimpleTaskExpanded(false);});
        tasks.ContextMenuStrip.Items.Insert(0,treeRestoreFull);
        tasks.ContextMenuStrip.Items.Insert(1,simpleExpandTaskMenu);tasks.ContextMenuStrip.Items.Insert(2,simpleCollapseTaskMenu);
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
        bool layoutChanged=simpleTaskCanExpand!=canExpand;
        simpleTaskCanExpand=canExpand;simpleToggleTask.Visible=canExpand;
        bool available=!busy && canExpand;
        simpleToggleTask.Enabled=available;simplePriorityButton.Enabled=!busy;
        simpleAddOutstanding.Enabled=!busy && node!=null;
        fullAddOutstanding.Enabled=simpleAddOutstanding.Enabled;
        bool fullCanExpand=node!=null && node.Nodes.Count>0;
        fullToggleTask.Visible=fullCanExpand;fullToggleTask.Enabled=!busy && fullCanExpand;
        bool menuCanExpand=simpleMode?canExpand:fullCanExpand;
        if(simpleExpandTaskMenu!=null){simpleExpandTaskMenu.Visible=menuCanExpand;simpleExpandTaskMenu.Enabled=!busy && menuCanExpand && !node.IsExpanded;}
        if(simpleCollapseTaskMenu!=null){simpleCollapseTaskMenu.Visible=menuCanExpand;simpleCollapseTaskMenu.Enabled=!busy && menuCanExpand && node.IsExpanded;}
        if(layoutChanged && simpleMode && !simpleLayout)PositionSimpleModeControls();
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
        return base.ProcessCmdKey(ref message,keys);
    }
    int MeasureSimpleActions(int width) {
        int available=Math.Max(60,width),x=0,y=0,rowHeight=0;
        foreach(var button in new[]{simpleToggleTask,simpleAddOutstanding,simplePriorityButton,restoreSimple}) {
            if(button==simpleToggleTask && !simpleTaskCanExpand)continue;
            int preferred=TextRenderer.MeasureText(button.Text,Font).Width+18;
            int buttonWidth=Math.Min(Math.Max(68,preferred),Math.Max(50,available-6));
            var text=TextRenderer.MeasureText(button.Text,Font,new Size(Math.Max(30,buttonWidth-12),0),TextFormatFlags.WordBreak);
            button.Size=new Size(buttonWidth,Math.Max(28,text.Height+10));
            int outerWidth=button.Width+button.Margin.Horizontal,outerHeight=button.Height+button.Margin.Vertical;
            if(x>0 && x+outerWidth>available){y+=rowHeight;x=0;rowHeight=0;}
            x+=outerWidth;rowHeight=Math.Max(rowHeight,outerHeight);
        }
        return y+rowHeight;
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
            restoreSimple.Visible=true;
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
        WindowState=FormWindowState.Normal;SetSimpleMode(false);
        if(collapsed)ToggleFold();RestoreWindow();tasks.Focus();
    }
    void SetSimpleMode(bool enabled) {
        if(enabled==simpleMode){UpdateSimpleModeState();return;}
        SuspendLayout();content.SuspendLayout();simpleLayout=true;
        try {
            if(enabled) {
                if(collapsed)ToggleFold();fullBounds=Bounds;simpleMode=true;
                PrepareSimpleOutstandingTree();UpdateSimpleActionState();
                content.Controls.Remove(tasks);content.Visible=false;toolbar.Visible=false;
                FormBorderStyle=FormBorderStyle.None;Padding=Padding.Empty;
                Controls.Add(tasks);tasks.Dock=DockStyle.None;tasks.Visible=true;tasks.BringToFront();
                simpleActionsHeight=MeasureSimpleActions(Math.Max(160,simpleSize.Width)-2*SimpleEdge);
                simpleViewportHeight=Math.Max(SimpleMinimumViewport,simpleSize.Height-2*SimpleEdge-SimpleActionGap-simpleActionsHeight);
                simpleActionsShown=ContainsFocus || ActiveForm==this;
                MinimumSize=new Size(160,SimpleMinimumViewport+2*SimpleEdge);ClientSize=new Size(Math.Max(160,simpleSize.Width),Math.Max(120,simpleSize.Height));
            } else {
                // simpleSize already contains the stable expanded dimensions, even while unfocused.
                simpleMode=false;simpleWindowWasMinimized=false;Controls.Remove(tasks);Padding=Padding.Empty;simpleActions.Visible=false;simpleEmpty.Visible=false;
                FormBorderStyle=FormBorderStyle.Sizable;MinimumSize=new Size(350,420);
                content.Controls.Add(tasks,0,3);tasks.Dock=DockStyle.Fill;tasks.Visible=true;content.Visible=true;toolbar.Visible=true;Bounds=fullBounds;
            }
        } finally {simpleLayout=false;content.ResumeLayout(true);ResumeLayout(true);}
        PerformLayout();tasks.Invalidate();PositionSimpleModeControls();UpdateSimpleModeState();RefreshSimpleOutstandingInBackground();SaveSimpleMode();
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
            if(tasks.Visible || tasks.Width<100 || tasks.Height<54 || !simpleEmpty.Visible || !restoreSimple.Visible || !simpleEmpty.Text.Contains("搜索"))throw new Exception("Empty simple mode must show its reason and recovery controls");
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
        var owner=tasks.Nodes.Cast<TreeNode>().FirstOrDefault(node=>node.Tag is long);
        if(owner!=null) {
            bool wasExpanded=owner.IsExpanded;var leaf=new TreeNode("测试遗留项") {Tag=new OutstandingLeaf{TaskId=(long)owner.Tag,Id="simple-toggle-test",Html="测试"}};
            bool wasRendering=rendering;rendering=true;
            try {owner.Nodes.Add(leaf);owner.Expand();tasks.SelectedNode=leaf;SetSelectedSimpleTaskExpanded(false);if(owner.IsExpanded)throw new Exception("Selected outstanding item did not collapse its owning task");tasks.SelectedNode=owner;SetSelectedSimpleTaskExpanded(true);if(!owner.IsExpanded)throw new Exception("Selected task did not expand");}
            finally {leaf.Remove();if(!wasExpanded)owner.Collapse();rendering=wasRendering;}
        }
        SetSimpleMode(false);
        File.WriteAllText(Path.Combine(data,"floating-simple-recovery-test.txt"),"PASS: inactive mode has no reserved footer; focus/mousedown preserve viewport and persisted size; compact actions wrap; selected task and outstanding owner expand/collapse; empty explanations; Escape/context/tray recovery; selected project persists.");
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
            if(simpleToggleTask.Visible || simpleTaskCanExpand || owner.Nodes.Count!=0 || !simpleAddOutstanding.Enabled)throw new Exception("Empty task must offer add without an expansion control or placeholder");
            long requested=0;int clicks=0;addOutstandingRequested=delegate(long id){requested=id;clicks++;};
            simpleAddOutstanding.PerformClick();
            if(clicks!=1 || requested!=910001L)throw new Exception("Add outstanding button targeted the wrong task");
            var shared=new SharedList();shared.Items.Add(new PendingItem{Id="action-test",Html="核对遗留事项"});
            ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{910001L,shared}});UpdateSimpleActionState();
            if(!simpleToggleTask.Visible || !simpleToggleTask.Enabled || owner.Nodes.Count!=1 || !(owner.Nodes[0].Tag is OutstandingLeaf))throw new Exception("First outstanding item must create a direct leaf and expansion control");
            var leaf=owner.Nodes[0];tasks.SelectedNode=leaf;simpleAddOutstanding.PerformClick();
            if(clicks!=2 || requested!=910001L)throw new Exception("Selected outstanding leaf must add to its owning task");
            tasks.SelectedNode=owner;simpleToggleTask.PerformClick();
            if(owner.IsExpanded)throw new Exception("Expansion button failed to collapse its selected task");
            simpleToggleTask.PerformClick();if(!owner.IsExpanded)throw new Exception("Expansion button failed to open its selected task");
            ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{910001L,new SharedList()}});UpdateSimpleActionState();
            if(owner.Nodes.Count!=0 || simpleToggleTask.Visible || simpleTaskCanExpand)throw new Exception("Removing the last outstanding item must remove its arrow and expansion control");
            owner.Nodes.Add(child);owner.Expand();tasks.SelectedNode=owner;UpdateSimpleActionState();
            if(!simpleToggleTask.Visible || !simpleToggleTask.Enabled)throw new Exception("Actual subtasks must remain expandable without outstanding items");
            tasks.SelectedNode=child;UpdateSimpleActionState();
            if(simpleToggleTask.Visible || !simpleAddOutstanding.Enabled)throw new Exception("Empty child must support add without expansion");
            simpleAddOutstanding.PerformClick();if(clicks!=3 || requested!=910002L)throw new Exception("Add outstanding button did not target the selected child");
            var viewport=tasks.Bounds;var savedSize=simpleSize;
            HideSimpleModeActions();ShowSimpleModeRestore();
            if(tasks.Bounds!=viewport || simpleSize!=savedSize)throw new Exception("Conditional actions moved the viewport across focus changes");
            foreach(Control control in simpleActions.Controls)if(control.Visible && (control.Left<0 || control.Right>simpleActions.ClientSize.Width || control.Bottom>simpleActions.ClientSize.Height))throw new Exception("Compact outstanding action is clipped");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-simple-outstanding-actions-test.png"));}
            SetBusy(true);simpleAddOutstanding.PerformClick();if(clicks!=3)throw new Exception("Busy state accepted an add action");
            SetBusy(false);tasks.SelectedNode=null;UpdateSimpleActionState();simpleAddOutstanding.PerformClick();
            if(simpleAddOutstanding.Enabled || simpleToggleTask.Visible || clicks!=3)throw new Exception("No selection exposed an expansion or accepted an add action");
            SetSimpleMode(false);tasks.SelectedNode=child;UpdateSimpleActionState();
            if(!fullAddOutstanding.Visible || !fullAddOutstanding.Enabled || fullAddOutstanding.Parent!=toolbar || !priorityFilterButton.Visible || fullToggleTask.Visible)throw new Exception("Full mode is missing shared actions or exposes expansion for an empty task");
            fullAddOutstanding.PerformClick();if(clicks!=4 || requested!=910002L)throw new Exception("Full add action did not target the selected task");
            var branch=new TreeNode("遗留事项"){Tag=new OutstandingBranch{TaskId=910002L,Loaded=true}};
            var normalLeaf=new TreeNode("1. 待核对"){Tag=new OutstandingLeaf{TaskId=910002L,Id="full-action",Html="待核对"}};
            branch.Nodes.Add(normalLeaf);child.Nodes.Add(branch);owner.Expand();child.Expand();branch.Expand();
            tasks.SelectedNode=normalLeaf;UpdateSimpleActionState();fullAddOutstanding.PerformClick();
            if(clicks!=5 || requested!=910002L || !fullToggleTask.Visible || !fullToggleTask.Enabled)throw new Exception("Full actions do not support a task with only grouped outstanding items");
            tasks.SelectedNode=branch;fullAddOutstanding.PerformClick();if(clicks!=6 || requested!=910002L)throw new Exception("Full add action did not resolve the outstanding branch owner");
            fullToggleTask.PerformClick();if(child.IsExpanded)throw new Exception("Full collapse action did not target the selected outstanding owner");
            tasks.SelectedNode=child;fullToggleTask.PerformClick();if(!child.IsExpanded)throw new Exception("Full expansion action failed");
            SetBusy(true);fullAddOutstanding.PerformClick();if(clicks!=6 || fullAddOutstanding.Enabled || fullToggleTask.Enabled)throw new Exception("Full actions remained usable while busy");
            SetBusy(false);tasks.SelectedNode=null;UpdateSimpleActionState();fullAddOutstanding.PerformClick();
            if(clicks!=6 || fullAddOutstanding.Enabled || fullToggleTask.Visible)throw new Exception("Full actions accepted an empty selection");
            tasks.SelectedNode=child;UpdateSimpleActionState();
            Action assertToolbar=delegate {
                PerformLayout();toolbar.PerformLayout();
                var controls=toolbar.Controls.Cast<Control>().Where(control=>control.Visible).ToArray();
                foreach(var control in controls)if(control.Left<toolbar.Padding.Left || control.Right+control.Margin.Right>toolbar.ClientSize.Width-toolbar.Padding.Right || control.Bottom+control.Margin.Bottom>toolbar.ClientSize.Height-toolbar.Padding.Bottom)throw new Exception("A full toolbar action was clipped: "+control.Text);
                int bottom=controls.Max(control=>control.Bottom+control.Margin.Bottom)+toolbar.Padding.Bottom;
                if(Math.Abs(toolbar.Height-bottom)>1)throw new Exception("Full toolbar retained empty rows: height="+toolbar.Height+", used="+bottom);
                if(!collapsed && content.Top!=toolbar.Bottom)throw new Exception("Full content did not follow its fitted toolbar");
            };
            foreach(int width in new[]{350,400,900}) {
                Size=new Size(width,600);assertToolbar();
                if(width!=350)using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-full-toolbar-"+width+"-test.png"));}
            }
            string filterText=priorityFilterButton.Text;
            priorityFilterButton.Text="按优先级 (9/10)";assertToolbar();priorityFilterButton.Text=filterText;assertToolbar();
            int unfoldedHeight=Height;ToggleFold();assertToolbar();
            if(content.Visible || ClientSize.Height!=toolbar.Height)throw new Exception("Folded window did not fit all toolbar rows");
            Width=350;assertToolbar();if(ClientSize.Height!=toolbar.Height)throw new Exception("Narrow folded toolbar was clipped");
            Width=900;assertToolbar();if(ClientSize.Height!=toolbar.Height)throw new Exception("Wide folded toolbar retained blank rows");
            ToggleFold();if(Height!=unfoldedHeight || !content.Visible)throw new Exception("Full window lost its expanded height");
            var fullArea=Bounds;SetSimpleMode(true);SetSimpleMode(false);assertToolbar();
            if(Bounds!=fullArea || !fullAddOutstanding.Visible)throw new Exception("Mode switching lost full bounds or actions");
            File.WriteAllText(Path.Combine(data,"floating-full-actions-test.txt"),"PASS: full/simple add and selected-owner expand/collapse parity; grouped outstanding targets; busy/empty selection guards; 350/400/900 widths without clipped actions or reserved rows; priority label reflow; folded resizing fits toolbar; expanded size and mode switches retained.");
            File.WriteAllText(Path.Combine(data,"floating-simple-outstanding-actions-test.txt"),"PASS: add button targets selected task/child or outstanding owner; no placeholder or expansion control for empty tasks; actual child/outstanding remains expandable; deleting last item removes expansion; busy/no selection disabled; narrow actions fit; focus preserves viewport.");
        } finally {
            addOutstandingRequested=callback;rendering=true;if(collapsed)ToggleFold();SetSimpleMode(false);tasks.Nodes.Clear();tasks.Nodes.AddRange(originalNodes);
            if(originalSelection!=null && originalSelection.TreeView==tasks)tasks.SelectedNode=originalSelection;
            simpleSize=originalSize;if(originalMode)SetSimpleMode(true);Bounds=originalBounds;
            simpleDetailsTesting=originalTesting;rendering=originalRendering;SetBusy(originalBusy);UpdateSimpleModeState();
        }
    }

}