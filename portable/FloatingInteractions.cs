// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;

// Paint the drop mark after the native tree paints; the mark also works in simple mode.
internal sealed partial class TaskTreeView : TreeView {
    internal TreeNode DropNode;
    internal int DropZone;
    internal bool Dropping;
    internal Action<TreeNode> CompletionClicked;
    internal Action<TreeNode> NodeDoubleClicked;
    readonly ImageList completionImages=CreateCompletionImages();
    TreeNode pressedCompletionNode;
    bool swallowCompletionUp;

    internal TaskTreeView() {
        DoubleBuffered=true;
        CheckBoxes=false;
        StateImageList=completionImages;
    }
    static ImageList CreateCompletionImages() {
        var images=new ImageList {ColorDepth=ColorDepth.Depth32Bit,ImageSize=new Size(16,16),TransparentColor=Color.Transparent};
        images.Images.Add(new Bitmap(16,16));
        foreach(var state in new[]{System.Windows.Forms.VisualStyles.CheckBoxState.UncheckedNormal,System.Windows.Forms.VisualStyles.CheckBoxState.CheckedNormal}) {
            var bitmap=new Bitmap(16,16,System.Drawing.Imaging.PixelFormat.Format32bppArgb);
            using(var canvas=Graphics.FromImage(bitmap)) {canvas.Clear(Color.Transparent);CheckBoxRenderer.DrawCheckBox(canvas,new Point(1,1),state);}
            images.Images.Add(bitmap);
        }
        return images;
    }
    internal void SyncCompletionState(TreeNode node) {
        if(node==null)return;
        var leaf=node.Tag as FloatingWindow.OutstandingLeaf;
        node.StateImageIndex=(node.Tag is long || leaf!=null) && SingleLinePaths?0:node.Tag is long?(node.Checked?2:1):leaf!=null?(leaf.Done?2:1):0;
        foreach(TreeNode child in node.Nodes)SyncCompletionState(child);
    }
    internal void SyncCompletionStates() {foreach(TreeNode node in Nodes)SyncCompletionState(node);}
    internal Rectangle CompletionVisualBounds(TreeNode node) {
        if(node==null || node.TreeView!=this || !node.IsVisible)return Rectangle.Empty;
        if(SingleLinePaths && (node.Tag is long || node.Tag is FloatingWindow.OutstandingLeaf))return new Rectangle(4,node.Bounds.Top,18,node.Bounds.Height);
        int y=node.Bounds.Top+node.Bounds.Height/2,left=-1,right=-1;
        for(int x=0;x<ClientSize.Width;x++) {
            var hit=HitTest(x,y);
            if(hit.Node==node && (hit.Location&TreeViewHitTestLocations.StateImage)!=0) {if(left<0)left=x;right=x;}
            else if(left>=0)break;
        }
        return left<0?Rectangle.Empty:new Rectangle(left,node.Bounds.Top,right-left+1,node.Bounds.Height);
    }
    TreeNode CompletionNodeAt(Point point) {
        if(SingleLinePaths && point.X>=4 && point.X<22) {
            for(var node=TopNode;node!=null;node=node.NextVisibleNode)if((node.Tag is long || node.Tag is FloatingWindow.OutstandingLeaf) && CompletionVisualBounds(node).Contains(point))return node;
        }
        var hit=HitTest(point);
        return hit.Node!=null && !SingleLinePaths && (hit.Node.Tag is long || hit.Node.Tag is FloatingWindow.OutstandingLeaf) && (hit.Location&TreeViewHitTestLocations.StateImage)!=0?hit.Node:null;
    }
    internal Rectangle CompletionBounds(TreeNode node) {return CompletionVisualBounds(node);}
    bool HandleCompletionMessage(ref Message message) {
        const int LeftDown=0x201,LeftUp=0x202,LeftDoubleClick=0x203,MouseMove=0x200,CaptureChanged=0x215;
        if(message.Msg==CaptureChanged){pressedCompletionNode=null;swallowCompletionUp=false;return false;}
        if(!Enabled || (message.Msg!=LeftDown && message.Msg!=LeftUp && message.Msg!=LeftDoubleClick && message.Msg!=MouseMove))return false;
        long coordinates=message.LParam.ToInt64();
        var point=new Point(unchecked((short)(coordinates&0xffff)),unchecked((short)((coordinates>>16)&0xffff)));
        if(message.Msg==LeftDown || message.Msg==LeftDoubleClick) {
            var node=CompletionNodeAt(point);if(node==null)return false;
            Focus();SelectedNode=node;pressedCompletionNode=message.Msg==LeftDown?node:null;swallowCompletionUp=true;Capture=true;
            return true;
        }
        if(message.Msg==MouseMove && swallowCompletionUp)return true;
        if(message.Msg==LeftUp && swallowCompletionUp) {
            var node=pressedCompletionNode;pressedCompletionNode=null;swallowCompletionUp=false;Capture=false;
            if(node!=null && CompletionNodeAt(point)==node && CompletionClicked!=null)CompletionClicked(node);
            return true;
        }
        return false;
    }
    bool HandleTaskDoubleClick(ref Message message) {
        if(message.Msg!=0x203 || !Enabled)return false;
        long coordinates=message.LParam.ToInt64();
        var point=new Point(unchecked((short)(coordinates&0xffff)),unchecked((short)((coordinates>>16)&0xffff)));
        var node=GetNodeAt(point);if(node==null)return false;
        Focus();SelectedNode=node;
        if(NodeDoubleClicked!=null)NodeDoubleClicked(node);
        return true;
    }
    protected override void OnKeyDown(KeyEventArgs e) {
        if(e.KeyCode==Keys.Space && Enabled && SelectedNode!=null && (SelectedNode.Tag is long || SelectedNode.Tag is FloatingWindow.OutstandingLeaf)) {
            e.Handled=true;e.SuppressKeyPress=true;if(CompletionClicked!=null)CompletionClicked(SelectedNode);return;
        }
        base.OnKeyDown(e);
    }
    protected override void WndProc(ref Message message) {
        bool repaintAfterScroll=message.Msg==0x114 || message.Msg==0x20E;
        if(HandleCompletionMessage(ref message)) return;
        if(HandleSimpleImageMessage(ref message)) return;
        if(HandleTaskDoubleClick(ref message)) return;
        if(HandleWindowDragMessage(ref message)) return;
        base.WndProc(ref message);
        if(repaintAfterScroll) {
            // Native TreeView does not reliably invalidate owner-drawn labels after horizontal
            // scrolling. Repaint now so text and clickable markers move without requiring a click.
            Invalidate();Update();
        }
        if(message.Msg != 0xF || !Dropping) return;
        using(var canvas = CreateGraphics()) using(var pen = new Pen(Color.FromArgb(36,94,210), 2)) {
            if(DropNode == null) canvas.DrawLine(pen, 8, ClientSize.Height-3, ClientSize.Width-8, ClientSize.Height-3);
            else {
                var bounds = DropNode.Bounds;
                int left = Math.Max(4, bounds.Left-2), right = Math.Max(left+4, ClientSize.Width-6);
                if(DropZone == 0) canvas.DrawRectangle(pen, left, bounds.Top, right-left, bounds.Height);
                else { int y = DropZone < 0 ? bounds.Top : bounds.Bottom; canvas.DrawLine(pen, left, y, right, y); }
            }
        }
    }
    protected override void Dispose(bool disposing) {if(disposing){completionImages.Dispose();DisposeDisplayFonts();}base.Dispose(disposing);}
}

internal sealed partial class FloatingWindow {
    readonly CheckBox prioritySort = new CheckBox {Text="按优先级",AutoSize=true,Padding=new Padding(0,6,0,0)};
    Dictionary<long,Dictionary<string,object>> taskCache = new Dictionary<long,Dictionary<string,object>>();
    Dictionary<long,long> taskParents = new Dictionary<long,long>();
    Dictionary<long,double> taskOrder = new Dictionary<long,double>();
    long taskViewId;
    bool dragging, editingOutstanding;
    TreeNode dropHover;
    DateTime dropHoverSince, scrollSince;
    sealed class DropPlan { public long TaskId, ParentId, BeforeId, TargetId; public string ItemId, BeforeItemId, Message; }

    static string OutstandingText(string html) {
        string text = Plain(html).Replace("\r\n"," ");
        int count = Regex.Matches(html ?? "", @"<img\b", RegexOptions.IgnoreCase).Count;
        if(count > 0) text += (text.Length == 0 ? "" : " · ") + count + " 张图片";
        return text;
    }
    static int PriorityNumber(Dictionary<string,object> task) {
        object value; long raw = task.TryGetValue("priority",out value) ? Convert.ToInt64(value) : 0;
        return raw >= 1 && raw <= 10 ? 10-(int)raw : 9;
    }
    static string TaskStatusValue(Dictionary<string,object> task) {
        object value;
        string status=task!=null && task.TryGetValue("status",out value)?Convert.ToString(value):"";
        if(status=="to-do" || status=="doing" || status=="done" || status=="hold")return status;
        return task!=null && task.ContainsKey("done") && Convert.ToBoolean(task["done"])?"done":"to-do";
    }
    static string TaskStatusText(string value) {
        return value=="doing"?"进行中":value=="done"?"已完成":value=="hold"?"暂停":"待办";
    }
    void InitializeInteractions() {
        tasks.PriorityClicked += async delegate(TreeNode node) {
            if(busy || closing || dragging || node == null || (!(node.Tag is long) && !(node.Tag is OutstandingLeaf)) || node.TreeView != tasks)return;
            tasks.SelectedNode=node;hoverTimer.Stop();progressTip.Hide(tasks);
            await ShowPriority();
        };
        tasks.SetSimpleImageLinks(true);
        try { var prefs=ReadObject(File.ReadAllText(Path.Combine(data,"floating-order.json")));prioritySort.Checked=Convert.ToBoolean(prefs["priority"]); } catch { }
        prioritySort.CheckedChanged += async delegate {SaveSortPreference();if(!rendering) await Reload();};
        InitializePriorityFilter();
        var menu=new ContextMenuStrip();
        menu.Items.Add(CreatePriorityFilterMenuItem());
        var setPriority=menu.Items.Add("设置优先级…",null,async delegate{await ShowPriority();});
        var setStatus=menu.Items.Add("设置任务状态…",null,async delegate{await ShowTaskStatus();});
        var setReminder=menu.Items.Add("设置提醒…",null,delegate{ShowReminderForSelected();});
        menu.Items.Add("查看图片…",null,async delegate{await ShowSelectedImages();});
        menu.Items.Add("管理遗留事项…",null,delegate {long id=SelectedTaskId();if(id>0)ShowOutstanding(id);});
        var toRoot=menu.Items.Add("移为顶层任务",null,async delegate{long id=SelectedTaskId();if(id>0)await ExecuteDrop(new DropPlan{TaskId=id,ParentId=0,BeforeId=0,Message="移为顶层任务"});});
        menu.Opening+=delegate(object sender,System.ComponentModel.CancelEventArgs e){e.Cancel=busy;setPriority.Enabled=tasks.SelectedNode!=null && (tasks.SelectedNode.Tag is long || tasks.SelectedNode.Tag is OutstandingLeaf);setStatus.Enabled=tasks.SelectedNode!=null && tasks.SelectedNode.Tag is long;setReminder.Enabled=tasks.SelectedNode!=null && (tasks.SelectedNode.Tag is long || tasks.SelectedNode.Tag is OutstandingLeaf);toRoot.Enabled=tasks.SelectedNode!=null && tasks.SelectedNode.Tag is long && taskParents.ContainsKey(SelectedTaskId());};
        InitializeUndo(menu);
        tasks.ContextMenuStrip=menu;
        tasks.NodeMouseClick+=delegate(object sender,TreeNodeMouseClickEventArgs e){if(e.Button==MouseButtons.Right)tasks.SelectedNode=e.Node;};
        tasks.AllowDrop=true;
        tasks.ItemDrag+=delegate(object sender,ItemDragEventArgs e){
            var node=e.Item as TreeNode;if(busy || node==null || (!(node.Tag is long) && !(node.Tag is OutstandingLeaf)))return;
            dragging=true;timer.Stop();hoverTimer.Stop();progressTip.Hide(tasks);
            try {tasks.DoDragDrop(node,DragDropEffects.Move);} finally {dragging=false;ClearDropMark();timer.Start();}
        };
        tasks.DragEnter+=delegate(object sender,DragEventArgs e){e.Effect=e.Data.GetDataPresent(typeof(TreeNode))?DragDropEffects.Move:DragDropEffects.None;};
        tasks.DragOver+=delegate(object sender,DragEventArgs e){
            var source=e.Data.GetData(typeof(TreeNode)) as TreeNode;var point=tasks.PointToClient(new Point(e.X,e.Y));var target=tasks.GetNodeAt(point);
            if(source!=null && source.Tag is long && target!=null && target.Tag is OutstandingLeaf)target=target.Parent;
            int zone;var plan=ResolveDropPlan(source,target,point.Y,out zone);
            e.Effect=plan==null?DragDropEffects.None:DragDropEffects.Move;
            bool markChanged=tasks.Dropping!=(plan!=null) || tasks.DropNode!=target || tasks.DropZone!=zone;
            tasks.Dropping=plan!=null;tasks.DropNode=target;tasks.DropZone=zone;if(markChanged)tasks.Invalidate();
            string message=plan==null?"此处不能放置（不能形成循环或超过 5 级）":plan.Message;
            if(status.Text!=message)status.Text=message;
            if(target!=dropHover){dropHover=target;dropHoverSince=DateTime.UtcNow;progressTip.Hide(tasks);progressTip.Show(message,tasks,Math.Min(point.X+12,Math.Max(6,tasks.Width-240)),Math.Max(0,point.Y-28),1500);}
            if(target!=null && !target.IsExpanded && (DateTime.UtcNow-dropHoverSince).TotalMilliseconds>700)target.Expand();
            if((DateTime.UtcNow-scrollSince).TotalMilliseconds>180){if(point.Y<24 && tasks.TopNode!=null && tasks.TopNode.PrevVisibleNode!=null)tasks.TopNode=tasks.TopNode.PrevVisibleNode;else if(point.Y>tasks.Height-24 && target!=null && target.NextVisibleNode!=null)target.NextVisibleNode.EnsureVisible();scrollSince=DateTime.UtcNow;}
        };
        tasks.DragLeave+=delegate{ClearDropMark();};
        tasks.DragDrop+=async delegate(object sender,DragEventArgs e){
            var source=e.Data.GetData(typeof(TreeNode)) as TreeNode;var point=tasks.PointToClient(new Point(e.X,e.Y));var target=tasks.GetNodeAt(point);
            if(source!=null && source.Tag is long && target!=null && target.Tag is OutstandingLeaf)target=target.Parent;
            int zone;var plan=ResolveDropPlan(source,target,point.Y,out zone);ClearDropMark();if(plan!=null)await ExecuteDrop(plan);
        };
    }
    void SaveSortPreference(){try{File.WriteAllText(Path.Combine(data,"floating-order.json"),json.Serialize(new{priority=prioritySort.Checked}));}catch{}}
    void ClearDropMark(){bool hadMark=tasks.Dropping || tasks.DropNode!=null;tasks.Dropping=false;tasks.DropNode=null;dropHover=null;if(hadMark)tasks.Invalidate();taskSurface.ClearDropMark();progressTip.Hide(tasks);progressTip.Hide(taskSurface);}
    long SelectedTaskId(){var node=tasks.SelectedNode;if(node==null)return 0;if(node.Tag is long)return (long)node.Tag;var leaf=node.Tag as OutstandingLeaf;return leaf==null?0:leaf.TaskId;}
    static int DropZone(TreeNode source,TreeNode target,int y){
        if(target==null)return 0;
        if(source!=null && source.Tag is OutstandingLeaf)return target.Tag is OutstandingLeaf?(y<target.Bounds.Top+target.Bounds.Height/2?-1:1):0;
        int edge=Math.Max(7,(int)Math.Round(target.Bounds.Height*.35));return y<target.Bounds.Top+edge?-1:y>=target.Bounds.Bottom-edge?1:0;
    }
    DropPlan ResolveDropPlan(TreeNode source,TreeNode target,int y,out int zone){
        zone=DropZone(source,target,y);var plan=MakeDropPlan(source,target,zone);
        // The centre means “make child”. Near a depth/cycle boundary, gracefully use the
        // closest sibling position so a normal reorder does not look permanently invalid.
        if(plan==null && source!=null && source.Tag is long && target!=null && target.Tag is long && source!=target && zone==0){
            zone=y<target.Bounds.Top+target.Bounds.Height/2?-1:1;plan=MakeDropPlan(source,target,zone);
        }
        return plan;
    }
    long ParentOf(long id){long parent;return taskParents.TryGetValue(id,out parent)?parent:0;}
    double PositionOf(long id){double value;return taskOrder.TryGetValue(id,out value)&&value>0?value:Double.MaxValue;}
    List<long> ManualSiblings(long parent){return taskCache.Keys.Where(id=>ParentOf(id)==parent).OrderBy(id=>PositionOf(id)).ThenBy(id=>id).ToList();}
    int SubtreeHeight(long id,HashSet<long> visited){if(visited.Count>=5 || !visited.Add(id))return 6;int height=1;foreach(long child in taskCache.Keys.Where(child=>ParentOf(child)==id)){height=Math.Max(height,1+SubtreeHeight(child,visited));if(height>5)break;}visited.Remove(id);return height;}
    bool ValidTaskParent(long id,long parent){var seen=new HashSet<long>();int depth=0;long cursor=parent;while(cursor>0){if(cursor==id || !seen.Add(cursor))return false;depth++;cursor=ParentOf(cursor);if(depth>=5)return false;}return depth+SubtreeHeight(id,new HashSet<long>())<=5;}
    DropPlan MakeDropPlan(TreeNode source,TreeNode target,int zone){
        if(busy || source==null || source.TreeView!=tasks || source==target)return null;
        if(source.Tag is long){
            if(taskViewId==0 || (target!=null && !(target.Tag is long)))return null;
            long id=(long)source.Tag,parent=0,before=0;
            if(target!=null){long anchor=(long)target.Tag;if(zone==0)parent=anchor;else{parent=ParentOf(anchor);if(zone<0)before=anchor;else{var siblings=ManualSiblings(parent).Where(value=>value!=id).ToList();int index=siblings.IndexOf(anchor);if(index>=0 && index+1<siblings.Count)before=siblings[index+1];}}}
            if(!ValidTaskParent(id,parent))return null;
            return new DropPlan{TaskId=id,ParentId=parent,BeforeId=before,Message=(zone==0?(parent==0?"移为顶层任务，放到末尾":"归入“"+TaskTitle(parent)+"”，放到末尾"):(zone<0?"插入任务之前":"插入任务之后"))+(prioritySort.Checked?" · 保存后按手动顺序显示":"")};
        }
        var item=source.Tag as OutstandingLeaf;if(item==null || target==null)return null;
        long destination=0;string beforeItem="";var targetItem=target.Tag as OutstandingLeaf;
        if(target.Tag is long)destination=(long)target.Tag;else if(targetItem!=null){destination=targetItem.TaskId;if(zone<0)beforeItem=targetItem.Id;else{var leaves=target.Parent.Nodes.Cast<TreeNode>().Select(node=>node.Tag as OutstandingLeaf).Where(leaf=>leaf!=null && !(leaf.TaskId==item.TaskId && leaf.Id==item.Id)).ToList();int index=leaves.FindIndex(leaf=>leaf.Id==targetItem.Id);if(index>=0 && index+1<leaves.Count)beforeItem=leaves[index+1].Id;}}
        if(destination==0 || (destination==item.TaskId && beforeItem==item.Id))return null;
        return new DropPlan{TaskId=item.TaskId,TargetId=destination,ItemId=item.Id,BeforeItemId=beforeItem,Message="遗留事项移至“"+TaskTitle(destination)+"”"+(targetItem==null?"末尾":zone<0?"所选项之前":"所选项之后")};
    }
    string TaskTitle(long id){return taskCache.ContainsKey(id)?(string)taskCache[id]["title"]:"任务";}
    async Task ExecuteDrop(DropPlan plan){
        if(busy || closing)return;SetBusy(true);timer.Stop();
        try {
            if(plan.ItemId!=null)await Api("POST","/tasks/"+plan.TaskId+"/outstanding/move",new{item_id=plan.ItemId,target_task_id=plan.TargetId,before_item_id=plan.BeforeItemId??""});
            else {await Api("POST","/tasks/"+plan.TaskId+"/move",new{parent_id=plan.ParentId,before_task_id=plan.BeforeId,project_view_id=taskViewId});rendering=true;try{prioritySort.Checked=false;SaveSortPreference();}finally{rendering=false;}}
            if(plan.ParentId>0)collapsedTasks.Remove(plan.ParentId);
            await LoadTasks();
            long selected=plan.ItemId==null?plan.TaskId:plan.TargetId;var matches=tasks.Nodes.Find(selected.ToString(),true);if(matches.Length>0){tasks.SelectedNode=matches[0];matches[0].EnsureVisible();if(plan.ItemId!=null)matches[0].Expand();}
            status.Text="归属和顺序已保存。";
        }catch(Exception e){Error(e);}finally{SetBusy(false);timer.Start();}
    }
    sealed class TaskOrderState {
        public long ViewId;
        public Dictionary<long,double> Positions = new Dictionary<long,double>();
    }
    async Task<TaskOrderState> FetchTaskOrder(long projectId,Dictionary<long,Dictionary<string,object>> all) {
        var result=new TaskOrderState();
        var views=await Api("GET","/projects/"+projectId+"/views",null);
        var options=((IEnumerable)views["items"]).Cast<Dictionary<string,object>>().ToList();
        var view=options.FirstOrDefault(item=>Convert.ToString(item["view_kind"])=="list")??options.FirstOrDefault();
        result.ViewId=view==null?0:Convert.ToInt64(view["id"]);
        foreach(var pair in all){object index;if(pair.Value.TryGetValue("index",out index))result.Positions[pair.Key]=Convert.ToDouble(index)*65536.0;}
        if(result.ViewId==0)return result;
        for(int p=1;;p++){
            var positions=await Api("GET","/projects/"+projectId+"/views/"+result.ViewId+"/tasktrace-positions?per_page=100&page="+p,null);
            foreach(Dictionary<string,object> item in (IEnumerable)positions["items"])result.Positions[Convert.ToInt64(item["task_id"])]=Convert.ToDouble(item["position"]);
            if(p>=Convert.ToInt32(positions["total_pages"]))break;
        }
        return result;
    }
    async Task ReadTaskOrder(long projectId,Dictionary<long,Dictionary<string,object>> all) {
        var result=await FetchTaskOrder(projectId,all);taskViewId=result.ViewId;taskOrder=result.Positions;
    }
    List<long> SortTaskIds(List<long> ids,Dictionary<long,Dictionary<string,object>> all){
        if(prioritySort.Checked)return ids.OrderBy(id=>PriorityNumber(all[id])).ThenBy(id=>PositionOf(id)).ThenBy(id=>id).ToList();
        return ids.OrderBy(id=>PositionOf(id)).ThenBy(id=>id).ToList();
    }
    void NumberTasks(List<TreeNode> roots,Dictionary<long,Dictionary<string,object>> all){for(int index=0;index<roots.Count;index++)NumberTask(roots[index],(index+1).ToString(),all,new List<string>());}
    void NumberTask(TreeNode node,string number,Dictionary<long,Dictionary<string,object>> all,List<string> ancestors){
        long id=(long)node.Tag;int priority=PriorityNumber(all[id]);string title=(string)all[id]["title"];
        string current=number+". "+"[P"+priority+"] ["+TaskStatusText(TaskStatusValue(all[id]))+"] "+title;
        var taskNode=node as TaskNode;if(taskNode!=null)taskNode.CurrentTextLength=current.Length;
        node.Text=current+(singleLine.Checked && ancestors.Count>0?TaskTreeView.SingleLineSeparator+String.Join(TaskTreeView.SingleLineSeparator,ancestors):"");
        var nextAncestors=new List<string>{title};nextAncestors.AddRange(ancestors);
        int index=0;foreach(TreeNode child in node.Nodes)if(child.Tag is long)NumberTask(child,number+"."+(++index),all,nextAncestors);
    }
    async Task UpdateOutstandingState(long taskId,string itemId,bool? done,int? priority) {
        var shared=ReadShared(await ReadHistory(taskId));var item=shared.Items.FirstOrDefault(value=>value.Id==itemId);
        if(item==null)throw new Exception("这条遗留事项已被移动或移除，请刷新后重试。");
        bool completionChanged=done.HasValue && item.Done!=done.Value;
        if(done.HasValue) {
            if(done.Value && !item.Done)item.CompletedAt=DateTimeOffset.UtcNow.ToString("o");
            else if(!done.Value)item.CompletedAt=null;
            item.Done=done.Value;
        }
        if(priority.HasValue)item.Priority=Math.Max(0,Math.Min(9,priority.Value));
        await WriteShared(taskId,shared);
        if(completionChanged)RememberOutstandingCompletion(taskId,itemId,done.Value);
    }
    async Task ShowPriority(){
        if(busy || closing)return;var selected=tasks.SelectedNode;long id=SelectedTaskId();var leaf=selected==null?null:selected.Tag as OutstandingLeaf;
        if(id==0 || selected==null || (!(selected.Tag is long) && leaf==null)){status.Text="请先选中一个任务或遗留事项。";return;}
        Point viewport=tasks.NativeScrollPosition();bool saved=false;SetBusy(true);timer.Stop();
        try {
            var current=leaf==null?await Api("GET","/tasks/"+id,null):null;
            string title=leaf==null?(string)current["title"]:OutstandingText(leaf.Html);int selectedPriority=leaf==null?PriorityNumber(current):leaf.Priority;
            using(var dialog=DpiDialog(new Form{Text="优先级 · "+title,Size=new Size(350,210),MinimumSize=new Size(350,210),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})){
                var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=3};
                layout.RowStyles.Add(new RowStyle(SizeType.Absolute,44));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,36));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
                var choice=new ComboBox{Dock=DockStyle.Fill,DropDownStyle=ComboBoxStyle.DropDownList};for(int value=0;value<=9;value++)choice.Items.Add(PriorityChoiceText(value)+(value==defaultPriority?" · 新增默认":""));choice.SelectedIndex=selectedPriority;
                var save=new Button{Text="保存优先级",AutoSize=true};bool writing=false;
                layout.Controls.Add(new Label{Text=leaf==null?"0 最高，9 最低；当前新增默认 "+defaultPriority+"。按优先级排列时，\r\n同级任务排序，下级任务保留在父任务下。":"0 最高，9 最低；当前新增默认 "+defaultPriority+"。",Dock=DockStyle.Fill});layout.Controls.Add(choice);layout.Controls.Add(save);dialog.Controls.Add(layout);
                save.Click+=async delegate{if(writing)return;writing=true;save.Enabled=false;choice.Enabled=false;try{if(leaf==null)await Api("PATCH","/tasks/"+id,new{priority=10-choice.SelectedIndex});else await UpdateOutstandingState(leaf.TaskId,leaf.Id,null,choice.SelectedIndex);saved=true;writing=false;dialog.Close();}catch(Exception e){MessageBox.Show(dialog,e.Message,"优先级未保存");}finally{writing=false;if(!dialog.IsDisposed){save.Enabled=true;choice.Enabled=true;}}};
                dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing)e.Cancel=true;};dialog.ShowDialog(this);
            }
            if(saved){await LoadTasks();tasks.RestoreNativeScroll(viewport);}
        }catch(Exception e){Error(e);}finally{SetBusy(false);timer.Start();}
    }

    async Task ShowTaskStatus(){
        if(busy || closing)return;var selected=tasks.SelectedNode;
        if(selected==null || !(selected.Tag is long)){status.Text="请先选中一个任务。";return;}
        long id=(long)selected.Tag;SetBusy(true);timer.Stop();
        try {
            var current=await Api("GET","/tasks/"+id,null);string currentStatus=TaskStatusValue(current);
            string[] values={"to-do","doing","done","hold"};string[] labels={"待办（to-do）","进行中（doing）","已完成（done）","暂停（hold）"};
            using(var dialog=DpiDialog(new Form{Text="任务状态 · "+(string)current["title"],Size=new Size(360,205),MinimumSize=new Size(360,205),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})){
                var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=3};
                layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,36));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
                var choice=new ComboBox{Dock=DockStyle.Fill,DropDownStyle=ComboBoxStyle.DropDownList,AccessibleName="任务状态"};choice.Items.AddRange(labels);choice.SelectedIndex=Math.Max(0,Array.IndexOf(values,currentStatus));
                var save=new Button{Text="保存状态",AutoSize=true};bool writing=false;
                layout.Controls.Add(new Label{Text="选择任务当前所处的状态",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft,AccessibleName="任务状态说明"});layout.Controls.Add(choice);layout.Controls.Add(save);dialog.Controls.Add(layout);
                save.Click+=async delegate{if(writing)return;writing=true;save.Enabled=false;choice.Enabled=false;try{await Api("PATCH","/tasks/"+id,new{status=values[choice.SelectedIndex]});writing=false;dialog.Close();}catch(Exception e){MessageBox.Show(dialog,e.Message,"任务状态未保存");}finally{writing=false;if(!dialog.IsDisposed){save.Enabled=true;choice.Enabled=true;}}};
                dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing)e.Cancel=true;};dialog.ShowDialog(this);
            }
            await LoadTasks();
        }catch(Exception e){Error(e);}finally{SetBusy(false);timer.Start();}
    }

    async Task<string> UploadOutstandingPictures(long id,List<PastedImage> pictures){
        var tags=new List<string>();
        foreach(var picture in pictures){
            if(picture.Id==0){
                await Api("GET","/tasks/"+id,null);
                using(var body=new MultipartFormDataContent()){
                    var bytes=new ByteArrayContent(picture.Bytes);bytes.Headers.ContentType=new System.Net.Http.Headers.MediaTypeHeaderValue("image/png");body.Add(bytes,"files","outstanding-"+Guid.NewGuid().ToString("N")+".png");
                    using(var request=new HttpRequestMessage(HttpMethod.Post,url+"/api/v2/tasks/"+id+"/attachments")){
                        request.Headers.Authorization=new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer",token);request.Content=body;
                        using(var response=await http.SendAsync(request)){
                            if(!response.IsSuccessStatusCode)throw new Exception("图片上传失败，图片已保留，请重试。");
                            var result=ReadObject(await response.Content.ReadAsStringAsync());foreach(Dictionary<string,object> attached in (IEnumerable)result["success"]){picture.Id=Convert.ToInt64(attached["id"]);break;}
                            if(picture.Id==0)throw new Exception("图片未上传，请检查大小后重试。");
                        }
                    }
                }
            }
            tags.Add("<p><img src=\"/api/v1/tasks/"+id+"/attachments/"+picture.Id+"\" alt=\"遗留事项图片\"></p>");
        }
        return String.Join("",tags);
    }
    sealed class OutstandingDraftCache {public string Text,DraftId;public int Priority;public bool RemoveExistingImages;public List<PastedImage> Pictures;}
    async void ShowOutstanding(long id,bool nested=false){ await EditOutstanding(id,nested,false); }
    async void ShowOutstandingItem(long id,string itemId){ await EditOutstanding(id,false,false,itemId); }
    async Task EditOutstanding(long id,bool nested,bool verify,string selectedItemId=null){
        if((busy && !nested)||closing)return;var owner=Form.ActiveForm??this;SetBusy(true);timer.Stop();editingOutstanding=true;
        try {
            var shared=ReadShared(await ReadHistory(id));
            using(var dialog=DpiDialog(new Form{Text="遗留事项 · 所有日期共享",Size=new Size(580,570),MinimumSize=new Size(500,480),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false,KeyPreview=true})){
                var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(12),ColumnCount=1,RowCount=7};
                layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,30));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,70));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,52));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,68));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,68));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,44));
                var list=new ListBox{Dock=DockStyle.Fill,HorizontalScrollbar=true,AccessibleName="遗留事项清单"};
                var mode=new Label{Text="遗留事项内容 · 新增（支持 Ctrl+V 粘贴图片）",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft,AccessibleName="遗留事项内容说明"};
                var input=new TextBox{Dock=DockStyle.Fill,Multiline=true,ScrollBars=ScrollBars.Vertical,AccessibleName="遗留事项内容",AccessibleDescription="遗留事项内容"};
                var priority=new ComboBox{Dock=DockStyle.Fill,DropDownStyle=ComboBoxStyle.DropDownList,AccessibleName="遗留事项优先级"};InitializePriorityChoice(priority,defaultPriority);
                var priorityRow=new TableLayoutPanel{Dock=DockStyle.Fill,ColumnCount=2,RowCount=1};priorityRow.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));priorityRow.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,115));
                priorityRow.Controls.Add(new Label{Text="优先级（0 最高，9 最低）",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft,AccessibleName="遗留事项优先级说明"},0,0);priorityRow.Controls.Add(priority,1,0);
                var previews=new FlowLayoutPanel{Dock=DockStyle.Fill,AutoScroll=true,WrapContents=false};
                var buttons=new FlowLayoutPanel{Dock=DockStyle.Fill};
                var save=new Button{Text="添加一条",AutoSize=true};var fresh=new Button{Text="新增事项",AutoSize=true};var remove=new Button{Text="删除此遗留事项",AutoSize=true,Enabled=false};var reminder=new Button{Text="设置提醒",AutoSize=true,Enabled=false};var files=new Button{Text="添加图片…",AutoSize=true};var gallery=new Button{Text="查看图片",AutoSize=true};var clearImages=new Button{Text="移除已有图片",AutoSize=true};var recover=new Button{Text="另存为新事项",AutoSize=true,Visible=false};
                buttons.Controls.AddRange(new Control[]{save,fresh,reminder,files,gallery,remove,clearImages,recover});
                var feedback=new Label{Text="选择一条可编辑；拖动归属和顺序请返回悬浮窗。",Dock=DockStyle.Fill};
                layout.Controls.Add(list);layout.Controls.Add(mode);layout.Controls.Add(input);layout.Controls.Add(priorityRow);layout.Controls.Add(previews);layout.Controls.Add(buttons);layout.Controls.Add(feedback);dialog.Controls.Add(layout);
                var pictures=new List<PastedImage>();string editingId=null,originalText="",originalHtml="",draftId=Guid.NewGuid().ToString(),lastCached="";int originalPriority=defaultPriority;bool writing=false,loading=false,removeExistingImages=false;
                Action disposePreviews=delegate{ClearImageThumbnails(previews);};
                Action renderPreviews=null;renderPreviews=delegate{
                    disposePreviews();
                    if(!removeExistingImages){var existingImages=new List<GalleryImage>();CollectImages(existingImages,originalHtml,"遗留事项图片");foreach(var image in existingImages)AddImageThumbnail(previews,image,dialog);}
                    foreach(var picture in pictures.ToList()){
                        var image=new GalleryImage{Bytes=picture.Bytes,Caption="待保存的遗留事项图片"};
                        AddImageThumbnail(previews,image,dialog,delegate{if(!writing){pictures.Remove(picture);renderPreviews();feedback.Text="内容尚未保存；自动保存只缓存到 .cache，点击保存后才会正式提交。";}});
                    }
                    int existing=removeExistingImages?0:Regex.Matches(originalHtml,@"<img\b",RegexOptions.IgnoreCase).Count;
                    previews.AccessibleName="遗留事项图片缩略图，点击查看大图";clearImages.Enabled=!writing && existing>0;
                };
                Action render=delegate{loading=true;list.Items.Clear();for(int i=0;i<shared.Items.Count;i++){shared.Items[i].Number=i+1;list.Items.Add(shared.Items[i]);}loading=false;};render();
                Func<bool> dirty=delegate{return input.Text!=originalText || priority.SelectedIndex!=originalPriority || pictures.Count>0 || removeExistingImages;};
                Func<string> draftKey=delegate{return editingId??"new";};
                Func<string> draftSnapshot=delegate{return json.Serialize(new{text=input.Text,priority=priority.SelectedIndex,pictures=pictures.Count,remove=removeExistingImages,draft=draftId});};
                Action cacheOutstanding=delegate{if(!dirty())return;string current=draftSnapshot();if(current==lastCached)return;WriteDraftCache("outstanding",id,draftKey(),new OutstandingDraftCache{Text=input.Text,DraftId=draftId,Priority=priority.SelectedIndex,RemoveExistingImages=removeExistingImages,Pictures=new List<PastedImage>(pictures)});lastCached=current;feedback.Text="草稿已自动缓存到 .cache，内容尚未保存；点击保存后才会正式提交。";};
                Func<bool> mayDiscard=delegate{if(!dirty())return true;if(autoSaveEnabled){try{cacheOutstanding();return true;}catch{}}return MessageBox.Show(dialog,"当前输入尚未保存，是否放弃？","遗留事项",MessageBoxButtons.YesNo,MessageBoxIcon.Question)==DialogResult.Yes;};
                Action<PendingItem> edit=delegate(PendingItem item){loading=true;editingId=item==null?null:item.Id;draftId=Guid.NewGuid().ToString();originalHtml=item==null?"":item.Html;originalPriority=item==null?defaultPriority:item.Priority;originalText=Plain(Regex.Replace(originalHtml,@"<img\b[^>]*>","",RegexOptions.IgnoreCase));input.Text=originalText;priority.SelectedIndex=originalPriority;pictures.Clear();removeExistingImages=false;recover.Visible=false;save.Text=item==null?"添加一条":"保存修改";remove.Enabled=item!=null;reminder.Enabled=item!=null;mode.Text=item==null?"遗留事项内容 · 新增（支持 Ctrl+V 粘贴图片）":"遗留事项内容 · 编辑第 "+item.Number+" 条（支持 Ctrl+V 粘贴图片）";var cached=ReadDraftCache<OutstandingDraftCache>("outstanding",id,draftKey());if(cached!=null){input.Text=cached.Text??"";draftId=String.IsNullOrWhiteSpace(cached.DraftId)?draftId:cached.DraftId;priority.SelectedIndex=Math.Max(0,Math.Min(9,cached.Priority));pictures=cached.Pictures??new List<PastedImage>();removeExistingImages=cached.RemoveExistingImages;lastCached=draftSnapshot();feedback.Text="已恢复 .cache 中的草稿，内容尚未保存；点击保存后才会正式提交。";}else lastCached="";renderPreviews();loading=false;};
                list.SelectedIndexChanged+=delegate{if(loading || writing)return;var selected=list.SelectedItem as PendingItem;if(selected==null || selected.Id==editingId)return;if(!mayDiscard()){loading=true;list.SelectedItem=shared.Items.FirstOrDefault(item=>item.Id==editingId);loading=false;return;}edit(selected);};
                if(!String.IsNullOrEmpty(selectedItemId)){var selected=shared.Items.FirstOrDefault(item=>item.Id==selectedItemId);if(selected!=null)list.SelectedItem=selected;}
                fresh.Click+=delegate{if(writing || !mayDiscard())return;loading=true;list.ClearSelected();loading=false;edit(null);input.Focus();};
                recover.Click+=delegate{if(writing)return;editingId=null;draftId=Guid.NewGuid().ToString();originalHtml="";originalText="";originalPriority=defaultPriority;priority.SelectedIndex=defaultPriority;removeExistingImages=false;save.Text="添加一条";mode.Text="遗留事项内容 · 另存为新事项（输入和待保存图片已保留）";recover.Visible=false;loading=true;list.ClearSelected();loading=false;renderPreviews();};
                clearImages.Click+=delegate{removeExistingImages=true;renderPreviews();feedback.Text="内容尚未保存；自动保存只缓存到 .cache，点击保存后才会正式提交。";};
                files.Click+=delegate{if(writing)return;using(var picker=new OpenFileDialog{Filter="图片|*.png;*.jpg;*.jpeg;*.bmp;*.gif|所有文件|*.*",Multiselect=true}){if(picker.ShowDialog(dialog)!=DialogResult.OK)return;foreach(string path in picker.FileNames)try{using(var image=Image.FromFile(path))using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}}catch{feedback.Text="部分文件无法读取，请选择 PNG、JPG、BMP 或 GIF 图片。";}renderPreviews();}};
                gallery.Click+=async delegate{await ShowImageGallery(id,editingId==null?null:originalHtml,dialog);};
                reminder.Click+=async delegate{if(writing||editingId==null)return;if(dirty()){feedback.Text="请先保存当前内容，再设置提醒。";return;}string selectedId=editingId;reminder.Enabled=false;try{if(await ShowReminderEditor(id,selectedId,dialog)){shared=ReadShared(await ReadHistory(id));render();var selected=shared.Items.FirstOrDefault(item=>item.Id==selectedId);if(selected!=null){list.SelectedItem=selected;edit(selected);}feedback.Text="提醒已保存，并已同步到网页模式。";}}catch(Exception e){feedback.Text="提醒未保存："+e.Message;}finally{if(!dialog.IsDisposed)reminder.Enabled=editingId!=null;}};
                Func<bool,Task> write=async delegate(bool deleting){
                    if(writing)return;if(deleting && editingId==null)return;
                    if(!deleting && String.IsNullOrWhiteSpace(input.Text) && pictures.Count==0 && (removeExistingImages || !Regex.IsMatch(originalHtml,@"<img\b",RegexOptions.IgnoreCase))){feedback.Text="请输入内容或添加图片。";return;}
                    writing=true;buttons.Enabled=false;list.Enabled=false;input.ReadOnly=true;priority.Enabled=false;
                    string savedDraftKey=draftKey();try{
                        string uploaded=deleting?"":await UploadOutstandingPictures(id,pictures);
                        var current=ReadShared(await ReadHistory(id));var existing=current.Items.FirstOrDefault(item=>item.Id==(editingId??draftId));
                        if(editingId!=null && existing==null)throw new Exception("这条事项已被移动或移除；输入已保留，请重新读取后添加。");
                        if(existing!=null && existing.Html!=originalHtml)throw new Exception("这条事项已在其他窗口更新。请保留输入并重新打开，避免覆盖修改。");
                        if(deleting)current.Items.Remove(existing);
                        else{
                            string body=input.Text==originalText?originalHtml:"<p>"+WebUtility.HtmlEncode(input.Text.Trim()).Replace("\r\n","<br>").Replace("\n","<br>")+"</p>"+String.Join("",Regex.Matches(originalHtml,@"<img\b[^>]*>",RegexOptions.IgnoreCase).Cast<Match>().Select(match=>match.Value));
                            if(removeExistingImages)body=Regex.Replace(body,@"<img\b[^>]*>","",RegexOptions.IgnoreCase);
                            if(existing==null)current.Items.Add(new PendingItem{Id=draftId,Html=body+uploaded,Priority=priority.SelectedIndex});else{existing.Html=body+uploaded;existing.Priority=priority.SelectedIndex;}
                        }
                        await WriteShared(id,current);DeleteDraftCache("outstanding",id,savedDraftKey);shared=current;render();edit(null);feedback.Text=deleting?"遗留事项已删除，可按 Ctrl+Z 撤销。":"已正式保存。图片随遗留事项保存，所有日期共享。";if(deleting)await RefreshUndo();
                    }catch(Exception e){feedback.Text=e.Message;recover.Visible=editingId!=null;}
                    finally{writing=false;buttons.Enabled=true;list.Enabled=true;input.ReadOnly=false;priority.Enabled=true;}
                };
                save.Click+=async delegate{await write(false);};remove.Click+=async delegate{if(editingId==null || MessageBox.Show(dialog,"确定删除当前遗留事项？删除后可按 Ctrl+Z 撤销。","删除遗留事项",MessageBoxButtons.YesNo,MessageBoxIcon.Warning)!=DialogResult.Yes)return;await write(true);};
                input.TextChanged+=delegate{if(!loading && !writing && dirty())feedback.Text="内容尚未保存；自动保存只缓存到 .cache，点击保存后才会正式提交。";};
                priority.SelectedIndexChanged+=delegate{if(!loading && !writing && dirty())feedback.Text="内容尚未保存；自动保存只缓存到 .cache，点击保存后才会正式提交。";};
                dialog.KeyDown+=delegate(object sender,KeyEventArgs e){
                    if(e.Control && e.KeyCode==Keys.V && !writing && Clipboard.ContainsImage()){e.SuppressKeyPress=true;try{using(var image=Clipboard.GetImage())using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}renderPreviews();feedback.Text="已粘贴图片，点击缩略图查看大图；使用缩略图右上角 × 可移除。";}catch{feedback.Text="剪贴板读取失败，请重试。";}}
                    if(e.Control && e.KeyCode==Keys.Enter){e.SuppressKeyPress=true;save.PerformClick();}
                };
                var autoTimer=new Timer{Interval=autoSaveSeconds*1000};autoTimer.Tick+=delegate{if(autoSaveEnabled && dirty())try{cacheOutstanding();}catch(Exception e){feedback.Text="草稿缓存失败："+e.Message+"；内容仍在当前窗口中。";}};autoTimer.Start();
                Exception verificationError=null;
                if(verify)dialog.Shown+=async delegate{
                    try{
                        int count=shared.Items.Count;input.Text="编辑器图片验收";priority.SelectedIndex=4;
                        using(var bitmap=new Bitmap(42,28))using(var stream=new MemoryStream()){using(var canvas=Graphics.FromImage(bitmap))canvas.Clear(Color.Teal);bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}
                        renderPreviews();await write(false);
                        if(shared.Items.Count!=count+1 || shared.Items.Last().Priority!=4 || Regex.Matches(shared.Items.Last().Html,@"<img\b").Count!=1)throw new Exception("Outstanding editor image create or priority failed: "+feedback.Text);
                        list.SelectedIndex=list.Items.Count-1;input.Text="编辑器修改验收";priority.SelectedIndex=2;
                        using(var bitmap=new Bitmap(32,20))using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}
                        await write(false);
                        if(shared.Items.Count!=count+1 || shared.Items.Last().Priority!=2 || !shared.Items.Last().Html.Contains("编辑器修改验收") || Regex.Matches(shared.Items.Last().Html,@"<img\b").Count!=2)throw new Exception("Outstanding editor image or priority update failed: "+feedback.Text);
                        list.SelectedIndex=list.Items.Count-1;
                        var editorImages=new List<GalleryImage>();CollectImages(editorImages,shared.Items.Last().Html,"遗留事项缩略图验收");
                        if(editorImages.Count!=2 || previews.Controls.OfType<Panel>().Count()!=2)throw new Exception("Saved outstanding images did not create thumbnails");
                        await ShowImagePreview(editorImages[0],dialog,true);
                        using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-outstanding-editor-test.png"));}
                    }catch(Exception e){verificationError=e;}finally{dialog.Close();}
                };
                dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing || (!verify && !mayDiscard()))e.Cancel=true;};
                try{dialog.ShowDialog(owner);if(verificationError!=null)throw verificationError;}finally{autoTimer.Stop();autoTimer.Dispose();disposePreviews();}
            }
            await LoadTasks();
        }catch(Exception e){if(verify)throw;Error(e);}finally{editingOutstanding=false;if(!nested){SetBusy(false);timer.Start();}}
    }

    sealed class GalleryImage {public string Source,Caption;public byte[] Bytes;}
    string AttachmentPath(string source){
        string value=WebUtility.HtmlDecode(source??"");Uri address;
        if(!value.StartsWith("/api/",StringComparison.Ordinal)){
            if(!Uri.TryCreate(value,UriKind.Absolute,out address))return null;
            var origin=new Uri(url);if(address.Scheme!=origin.Scheme || address.Host!=origin.Host || address.Port!=origin.Port)return null;value=address.AbsolutePath;
        }
        var match=Regex.Match(value,@"^/api/v[12]/tasks/([0-9]+)/attachments/([0-9]+)(?:\?.*)?$");
        return match.Success?"/api/v2/tasks/"+match.Groups[1].Value+"/attachments/"+match.Groups[2].Value:null;
    }
    void CollectImages(List<GalleryImage> images,string html,string caption){
        foreach(Match match in Regex.Matches(html??"","<img\\b[^>]*?\\bsrc\\s*=\\s*[\"']([^\"']+)[\"'][^>]*>",RegexOptions.IgnoreCase)){
            string source=AttachmentPath(match.Groups[1].Value);if(source!=null && !images.Any(image=>image.Source==source))images.Add(new GalleryImage{Source=source,Caption=caption});
        }
    }
    async Task<List<GalleryImage>> GatherImages(long taskId,string onlyHtml,Form gallery,string scopeLabel=null){
        var images=new List<GalleryImage>();
        if(onlyHtml!=null){CollectImages(images,onlyHtml,TaskTitle(taskId)+" · "+(scopeLabel??"遗留事项"));return images;}
        var queue=new Queue<long>();var seen=new HashSet<long>();queue.Enqueue(taskId);
        while(queue.Count>0 && !gallery.IsDisposed){
            long id=queue.Dequeue();if(!seen.Add(id))continue;
            var task=await Api("GET","/tasks/"+id,null);string title=(string)task["title"];
            CollectImages(images,Convert.ToString(task["description"]),title+" · 描述");
            var history=await ReadHistory(id);var shared=ReadShared(history);int index=0;
            foreach(var item in shared.Items)CollectImages(images,item.Html,title+" · 遗留事项 "+(++index));
            foreach(var note in DailyHistory(history))CollectImages(images,ProgressDisplayBody((string)note["comment"]),title+" · "+DayOf(note)+" 进展");
            foreach(var note in history.Where(note=>DayOf(note)=="" && !((string)note["comment"]).Contains(SharedHeading)))CollectImages(images,(string)note["comment"],title+" · 备注");
            var relations=task["related_tasks"] as Dictionary<string,object>;if(relations!=null && relations.ContainsKey("subtask"))foreach(Dictionary<string,object> child in (IEnumerable)relations["subtask"])if(Convert.ToInt64(child["project_id"])==Convert.ToInt64(task["project_id"]))queue.Enqueue(Convert.ToInt64(child["id"]));
        }
        return images;
    }
    async Task<byte[]> DownloadImage(string path){
        string safe=AttachmentPath(path);if(safe==null)throw new Exception("不支持的图片地址。");
        // Refresh independently before streaming binary content. Never send tokens to external URLs.
        var parts=safe.Split('/');await Api("GET","/tasks/"+parts[4],null);
        using(var request=new HttpRequestMessage(HttpMethod.Get,url+safe)){
            request.Headers.Authorization=new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer",token);
            using(var response=await http.SendAsync(request)){
                if(!response.IsSuccessStatusCode)throw new Exception("图片无法读取，请关闭窗口后重试。");
                return await response.Content.ReadAsByteArrayAsync();
            }
        }
    }
    async Task ShowSelectedImages(){
        if(busy || closing)return;long id=SelectedTaskId();if(id==0){status.Text="请先选中任务或遗留事项。";return;}
        var leaf=tasks.SelectedNode.Tag as OutstandingLeaf;await ShowImageGallery(id,leaf==null?null:leaf.Html,this);
    }
    async Task ShowImageGallery(long id,string onlyHtml,Form owner,string scopeLabel=null){
        var gallery=DpiDialog(new Form{Text="查看图片 · "+TaskTitle(id),Size=new Size(800,650),MinimumSize=new Size(400,300),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false});
        var heading=new Label{Dock=DockStyle.Top,Height=42,Padding=new Padding(12,10,12,0),Text="正在收集图片…"};
        var flow=new FlowLayoutPanel{Dock=DockStyle.Fill,AutoScroll=true,FlowDirection=FlowDirection.TopDown,WrapContents=false,Padding=new Padding(10)};
        gallery.Controls.Add(flow);gallery.Controls.Add(heading);var bitmaps=new List<Image>();
        gallery.FormClosed+=delegate{foreach(var image in bitmaps)image.Dispose();};
        gallery.Show(owner);
        try{
            var images=await GatherImages(id,onlyHtml,gallery,scopeLabel);if(gallery.IsDisposed)return;
            heading.Text=images.Count==0?"没有图片。可在遗留事项、任务描述或每日进展中添加。":images.Count+" 张图片 · "+(onlyHtml==null?"当前任务及所有下级任务":(scopeLabel??"当前遗留事项"));
            int loaded=0;
            foreach(var item in images){
                if(gallery.IsDisposed)return;
                var card=new Panel{Width=Math.Max(340,flow.ClientSize.Width-38),Height=330,Margin=new Padding(0,0,0,18)};
                var label=new Label{Text=(++loaded)+". "+item.Caption,Dock=DockStyle.Top,Height=34,AutoEllipsis=true};
                var box=new PictureBox{Dock=DockStyle.Fill,SizeMode=PictureBoxSizeMode.Zoom,BackColor=Color.White};card.Controls.Add(box);card.Controls.Add(label);flow.Controls.Add(card);
                try{
                    byte[] bytes=await DownloadImage(item.Source);if(gallery.IsDisposed)return;
                    using(var stream=new MemoryStream(bytes))using(var image=Image.FromStream(stream)){
                        double scale=Math.Min(1.0,Math.Min(1400.0/image.Width,1000.0/image.Height));var preview=new Bitmap(image,new Size(Math.Max(1,(int)(image.Width*scale)),Math.Max(1,(int)(image.Height*scale))));bitmaps.Add(preview);box.Image=preview;
                        card.Height=Math.Max(170,Math.Min(560,(int)((card.Width-10)*(double)image.Height/image.Width)+34));
                    }
                }catch{if(!gallery.IsDisposed)label.Text=item.Caption+" · 图片加载失败，关闭后重试。";}
            }
            if(!gallery.IsDisposed)flow.SizeChanged+=delegate{foreach(Control card in flow.Controls)card.Width=Math.Max(300,flow.ClientSize.Width-38);};
            if(selfTest && !gallery.IsDisposed){using(var bitmap=new Bitmap(gallery.Width,gallery.Height)){gallery.DrawToBitmap(bitmap,new Rectangle(Point.Empty,gallery.Size));bitmap.Save(Path.Combine(data,"floating-gallery-test.png"));}if(images.Count>0 && bitmaps.Count!=images.Count)throw new Exception("Gallery did not load every image");gallery.Close();}
        }catch(Exception e){if(selfTest){gallery.Close();throw;}if(!gallery.IsDisposed)heading.Text="图片读取失败："+e.Message;}
    }

    async Task TestInlinePriorityEdit(long targetId,long otherId,int priority,bool useSimpleMode) {
        var otherBefore=await Api("GET","/tasks/"+otherId,null);
        if(useSimpleMode){SetSimpleMode(true);await RefreshSimpleOutstanding();}
        var target=tasks.Nodes.Find(targetId.ToString(),true).First();
        tasks.SelectedNode=tasks.Nodes.Find(otherId.ToString(),true).First();target.EnsureVisible();
        SendSimpleMessage(tasks.Handle,0x114,new IntPtr(1),IntPtr.Zero);SendSimpleMessage(tasks.Handle,0x114,new IntPtr(1),IntPtr.Zero);tasks.Refresh();Point viewport=tasks.NativeScrollPosition();
        bool saved=false;Form opened=null;
        using(var driver=new Timer{Interval=25}) {
            driver.Tick+=delegate {
                opened=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="优先级 · "+TaskTitle(targetId));
                if(opened==null || saved)return;
                var layout=opened.Controls.OfType<TableLayoutPanel>().Single();
                layout.Controls.OfType<ComboBox>().Single().SelectedIndex=priority;
                saved=true;driver.Stop();layout.Controls.OfType<Button>().Single(button=>button.Text=="保存优先级").PerformClick();
            };
            driver.Start();
            var link=tasks.PriorityLinkBounds(target);
            if(link.IsEmpty)throw new Exception("Visible task has no priority link");
            int coordinates=(link.Top+link.Height/2)<<16 | (link.Left+link.Width/2)&0xffff;
            SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(coordinates));
            SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(coordinates));
            for(int attempt=0;attempt<400 && (!saved || busy || (opened!=null && !opened.IsDisposed));attempt++)await Task.Delay(25);
            if(!saved || busy || (opened!=null && !opened.IsDisposed))throw new Exception("Inline priority editor failed to save and close");
        }
        var updated=await Api("GET","/tasks/"+targetId,null);
        var otherAfter=await Api("GET","/tasks/"+otherId,null);
        if(PriorityNumber(updated)!=priority || PriorityNumber(otherBefore)!=PriorityNumber(otherAfter))throw new Exception("Priority click edited the wrong task or was not persisted");
        if(!tasks.Nodes.Find(targetId.ToString(),true).First().Text.Contains("[P"+priority+"]"))throw new Exception("Saved priority did not refresh its task label");
        if(tasks.NativeScrollPosition()!=viewport)throw new Exception("Priority edit changed the task tree scroll position");
        if(useSimpleMode)SetSimpleMode(false);
    }
    async Task TestInteractions(){
        var project=projects.SelectedItem as Project;var created=new List<long>();bool originalShowCompleted=showCompleted.Checked;
        {
            for(int number=0;number<=9;number++)if(PriorityNumber(new Dictionary<string,object>{{"priority",10-number}})!=number)throw new Exception("Priority range mapping failed");
            if(PriorityNumber(new Dictionary<string,object>{{"priority",0}})!=9)throw new Exception("Default priority is not nine");
            var ids=new List<long>();foreach(string title in new[]{"拖动验收 A","拖动验收 B","拖动验收 C"}){var item=await Api("POST","/projects/"+project.Id+"/tasks",new{title=title});long value=Convert.ToInt64(item["id"]);ids.Add(value);created.Add(value);}
            long a=ids[0],b=ids[1],c=ids[2];long child=await CreateSubtask(a,project.Id,"可拖动子任务"),sibling=await CreateSubtask(b,project.Id,"另一个子任务");created.Add(child);created.Add(sibling);
            await LoadTasks();
            foreach(long rootId in ids)await Api("POST","/tasks/"+rootId+"/move",new{parent_id=0,before_task_id=0,project_view_id=taskViewId});await LoadTasks();
            Func<long,TreeNode> node=delegate(long id){return tasks.Nodes.Find(id.ToString(),true).First();};
            if(!node(child).Text.StartsWith("1.1. "))throw new Exception("Nested task numbering failed");
            var childMove=MakeDropPlan(node(child),node(b),0);if(childMove==null || childMove.ParentId!=b)throw new Exception("Child drop target was not resolved");await ExecuteDrop(childMove);
            if(node(child).Parent!=node(b) || ParentOf(child)!=b)throw new Exception("Task reparent did not survive reload");
            if(MakeDropPlan(node(b),node(child),0)!=null)throw new Exception("Cycle drop was allowed");
            var reorder=MakeDropPlan(node(c),node(a),-1);if(reorder==null || reorder.BeforeId!=a)throw new Exception("Sibling drop position failed");await ExecuteDrop(reorder);
            if((long)tasks.Nodes[0].Tag!=c || !tasks.Nodes[0].Text.StartsWith("1. "))throw new Exception("Root ordering did not survive reload");
            await ExecuteDrop(MakeDropPlan(node(child),node(sibling),-1));
            if((long)node(b).Nodes[0].Tag!=child || !node(child).Text.StartsWith("3.1. "))throw new Exception("Child manual ordering failed");
            await Api("PATCH","/tasks/"+a,new{priority=2});await Api("PATCH","/tasks/"+b,new{priority=10});rendering=true;prioritySort.Checked=true;rendering=false;await LoadTasks();
            if((long)tasks.Nodes[0].Tag!=b || (long)tasks.Nodes[1].Tag!=a || (long)tasks.Nodes[2].Tag!=c || !node(b).Text.Contains("[P0]"))throw new Exception("Lower displayed priority did not sort first");
            if(node(child).Parent!=node(b))throw new Exception("Priority sorting detached children");
            var preference=ReadObject(File.ReadAllText(Path.Combine(data,"floating-order.json")));if(!Convert.ToBoolean(preference["priority"]))throw new Exception("Sort preference not persisted");
            await Api("PATCH","/tasks/"+child,new{title="可拖动子任务 "+new string('长',120)});await LoadTasks();
            await TestInlinePriorityEdit(child,b,3,false);await TestInlinePriorityEdit(child,b,9,true);
            await ExecuteDrop(MakeDropPlan(node(c),node(a),-1));if(prioritySort.Checked)throw new Exception("Manual drag did not restore manual ordering");
            SetSimpleMode(true);await LoadTasks();
            var compactMove=MakeDropPlan(node(child),node(a),0);if(compactMove==null || compactMove.ParentId!=a)throw new Exception("Compact task drop target was not resolved");await ExecuteDrop(compactMove);
            if(node(child).Parent!=node(a) || MakeDropPlan(node(a),node(child),0)!=null)throw new Exception("Compact task reparent or cycle protection failed");
            compactMove=MakeDropPlan(node(child),node(sibling),-1);if(compactMove==null || compactMove.ParentId!=b || compactMove.BeforeId!=sibling)throw new Exception("Compact sibling drop target was not resolved");await ExecuteDrop(compactMove);
            if(node(child).Parent!=node(b) || node(b).Nodes[0]!=node(child))throw new Exception("Compact child ordering did not survive reload");
            SetSimpleMode(false);
            rendering=true;showCompleted.Checked=true;rendering=false;await LoadTasks();
            var pictures=new List<PastedImage>();using(var bitmap=new Bitmap(80,50)){using(var canvas=Graphics.FromImage(bitmap))canvas.Clear(Color.SteelBlue);using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}}
            string imageHtml=await UploadOutstandingPictures(a,pictures);var source=new SharedList();source.Items.Add(new PendingItem{Id="drag-image",Html="带图片的遗留事项"+imageHtml,Done=true,Priority=3});source.Items.Add(new PendingItem{Id="drag-text",Html="其他事项"});
            var target=new SharedList();target.Items.Add(new PendingItem{Id="existing",Html="已存在的遗留事项"});
            Func<long,string,TreeNode> leaf=delegate(long id,string itemId){return node(id).Nodes.Cast<TreeNode>().Single(item=>item.Tag is OutstandingLeaf && ((OutstandingLeaf)item.Tag).Id==itemId);};
            SharedList moved=null;
            foreach(bool compact in new[]{false,true}) {
                SetSimpleMode(compact);await WriteShared(a,source);await WriteShared(b,target);await LoadTasks();
                var sourceLeaf=leaf(a,"drag-image");tasks.SelectedNode=sourceLeaf;
                if(SelectedTaskId()!=a || MakeDropPlan(node(a),leaf(b,"existing"),0)!=null || MakeDropPlan(sourceLeaf,sourceLeaf,0)!=null)throw new Exception("Direct outstanding owner or invalid drop protection failed");
                var append=MakeDropPlan(sourceLeaf,node(b),0);
                if(append==null || append.TaskId!=a || append.TargetId!=b || append.ItemId!="drag-image" || append.BeforeItemId!="")throw new Exception("Direct outstanding task append target was not resolved");
                await ExecuteDrop(append);
                moved=ReadShared(await ReadHistory(b));var remaining=ReadShared(await ReadHistory(a));
                if(moved.Items.Count!=2 || moved.Items[0].Id!="existing" || moved.Items[1].Id!="drag-image" || !moved.Items[1].Done || moved.Items[1].Priority!=3 || remaining.Items.Count!=1 || remaining.Items[0].Id!="drag-text")throw new Exception("Outstanding move lost contents, completion, priority or append order");
                if(moved.Items[1].Html.Contains("/tasks/"+a+"/attachments/"))throw new Exception("Moved image still depends on source task");
                if(leaf(b,"drag-image").Parent!=node(b) || !node(b).IsExpanded)throw new Exception("Moved outstanding item was not directly visible under its task");
                var before=MakeDropPlan(leaf(b,"drag-image"),leaf(b,"existing"),-1);
                if(before==null || before.TargetId!=b || before.BeforeItemId!="existing")throw new Exception("Direct outstanding before target was not resolved");
                await ExecuteDrop(before);moved=ReadShared(await ReadHistory(b));if(moved.Items[0].Id!="drag-image")throw new Exception("Outstanding before reorder not saved");
                var after=MakeDropPlan(leaf(b,"drag-image"),leaf(b,"existing"),1);
                if(after==null || after.TargetId!=b || after.BeforeItemId!="")throw new Exception("Direct outstanding after target was not resolved");
                await ExecuteDrop(after);moved=ReadShared(await ReadHistory(b));if(moved.Items[0].Id!="existing" || moved.Items[1].Id!="drag-image")throw new Exception("Outstanding after reorder not saved");
            }
            SetSimpleMode(false);
            await Api("DELETE","/tasks/"+a,null);created.Remove(a);await LoadTasks();
            var images=new List<GalleryImage>();CollectImages(images,moved.Items[1].Html,"test");if(images.Count!=1)throw new Exception("Gallery did not discover moved image");
            byte[] bytes=await DownloadImage(images[0].Source);using(var stream=new MemoryStream(bytes))using(var bitmap=Image.FromStream(stream))if(bitmap.Width!=80)throw new Exception("Image copy was corrupted after source deletion");
            if(AttachmentPath("https://example.com/api/v1/tasks/1/attachments/1")!=null || AttachmentPath("//example.com/api/v1/tasks/1/attachments/1")!=null)throw new Exception("External gallery URL was accepted");
            foreach(bool compact in new[]{false,true}) {
                SetSimpleMode(compact);await LoadTasks();
                var pictureLeaf=leaf(b,"drag-image");node(b).Expand();pictureLeaf.EnsureVisible();tasks.SelectedNode=pictureLeaf;
                if(SelectedTaskId()!=b || tasks.SimpleImageBounds(pictureLeaf).IsEmpty)throw new Exception("Direct outstanding image link or owner is unavailable in this layout");
                await ShowSelectedImages();await ShowImageGallery(b,null,this);
                await EditOutstanding(b,false,true);if(editingOutstanding)throw new Exception("Outstanding editor did not resume auto-save");
            }
            SetSimpleMode(false);
            foreach(var item in moved.Items)if(String.IsNullOrEmpty(OutstandingText(item.Html)))throw new Exception("Image-only outstanding title missing");
            tasks.Nodes[0].Expand();using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-interactions-test.png"));}
            File.WriteAllText(Path.Combine(data,"floating-interactions-test.txt"),"PASS: drop targets, task parent/order persisted, cycle blocked, hierarchical numbering, task and outstanding priorities 0-9 + default 9, persisted sort setting, inline priority links in full/simple mode, manual drag restores manual order, shared full/simple task and direct outstanding drop targets, before/after reorder + cross-task append preserving outstanding completion and priority, copied image survives deleting source, image links + scoped/task galleries + image editor in both layouts, external URLs rejected.");
        }
        {
            foreach(long id in created.AsEnumerable().Reverse())try{await Api("DELETE","/tasks/"+id,null);}catch{}
            rendering=true;prioritySort.Checked=false;showCompleted.Checked=originalShowCompleted;rendering=false;await LoadTasks();
        }
    }
}
