// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class TaskTreeView {
    internal Func<TreeNode,bool> SimpleImageAvailable;
    internal Action<TreeNode> SimpleImageClicked;
    bool simpleImageLinks;
    TreeNode pressedImageNode;
    bool swallowImageUp;

    internal void SetSimpleImageLinks(bool enabled) {
        // Retain the existing entry point; images are now available in both layouts.
        enabled=true;
        if(simpleImageLinks==enabled && DrawMode==TreeViewDrawMode.OwnerDrawText)return;
        simpleImageLinks=enabled;pressedImageNode=null;swallowImageUp=false;ResetPriorityLinkPress();
        if(Capture)Capture=false;
        Cursor=Cursors.Default;
        DrawMode=TreeViewDrawMode.OwnerDrawText;
        Invalidate();
    }
    internal Rectangle SimpleImageBounds(TreeNode node) {
        if(!simpleImageLinks || node==null || node.TreeView!=this || SimpleImageAvailable==null || !SimpleImageAvailable(node))return Rectangle.Empty;
        var bounds=node.Bounds;
        if(bounds.Height<=0)return Rectangle.Empty;
        Font font=DisplayFont(node);
        int width=TextRenderer.MeasureText("图片",font,Size.Empty,TextFormatFlags.NoPadding).Width;
        var numbered=TaskPriorityMatch(node);
        int left=bounds.Left+2+(numbered.Success?PriorityTextAdvance(numbered.Groups["prefix"].Value,font):0);
        return new Rectangle(left,bounds.Top,width+4,bounds.Height);
    }
    internal void ReserveSimpleImageSpace(TreeNode node) {
        if(!simpleImageLinks || SimpleImageAvailable==null || !SimpleImageAvailable(node))return;
        Font font=DisplayFont(node);
        int linkWidth=TextRenderer.MeasureText("图片",font,Size.Empty,TextFormatFlags.NoPadding).Width+12;
        int spaceWidth=Math.Max(1,TextRenderer.MeasureText("x x",font,Size.Empty,TextFormatFlags.NoPadding).Width-TextRenderer.MeasureText("xx",font,Size.Empty,TextFormatFlags.NoPadding).Width);
        // Native hit testing measures Node.Text, whereas owner drawing places a link before it.
        // Trailing spaces reserve that width without changing the outstanding item's HTML/title.
        node.Text=node.Text.TrimEnd(' ')+new string(' ',(linkWidth+spaceWidth-1)/spaceWidth+2);
    }
    TreeNode ImageNodeAt(Point point) {
        // GetNodeAt can return null to the right of the native text bounds.
        // Looking up the visible row also keeps a short image-only label clickable.
        for(var node=TopNode;node!=null;node=node.NextVisibleNode) {
            if(node.Bounds.Top>point.Y)break;
            if(SimpleImageBounds(node).Contains(point))return node;
        }
        return null;
    }
    protected override void OnDrawNode(DrawTreeNodeEventArgs e) {
        if(DrawPriorityLink(e)){base.OnDrawNode(e);return;}
        var link=SimpleImageBounds(e.Node);
        if(link.IsEmpty){e.DrawDefault=true;base.OnDrawNode(e);return;}
        Font font=DisplayFont(e.Node);
        bool selected=(e.State&TreeNodeStates.Selected)!=0;
        Color background=selected?SystemColors.Highlight:BackColor;
        Color foreground=selected?SystemColors.HighlightText:e.Node.ForeColor;
        if(foreground.IsEmpty)foreground=ForeColor;
        int textLeft=link.Right+6;
        string titleText=e.Node.Text.TrimEnd(' ');
        int textWidth=TextRenderer.MeasureText(titleText,font,Size.Empty,TextFormatFlags.NoPadding).Width;
        var bounds=new Rectangle(e.Bounds.Left,e.Bounds.Top,Math.Max(0,Math.Min(ClientSize.Width-e.Bounds.Left,textLeft-e.Bounds.Left+textWidth+4)),e.Bounds.Height);
        using(var brush=new SolidBrush(background))e.Graphics.FillRectangle(brush,bounds);
        using(var underline=new Font(font,font.Style|FontStyle.Underline)) {
            TextRenderer.DrawText(e.Graphics,"图片",underline,link,selected?SystemColors.HighlightText:Color.FromArgb(36,94,210),TextFormatFlags.NoPadding|TextFormatFlags.VerticalCenter|TextFormatFlags.SingleLine);
        }
        var title=new Rectangle(textLeft,e.Bounds.Top,Math.Max(0,ClientSize.Width-textLeft-2),e.Bounds.Height);
        TextRenderer.DrawText(e.Graphics,titleText,font,title,foreground,TextFormatFlags.NoPadding|TextFormatFlags.VerticalCenter|TextFormatFlags.SingleLine|TextFormatFlags.EndEllipsis);
        if(selected && Focused)ControlPaint.DrawFocusRectangle(e.Graphics,bounds,foreground,background);
        base.OnDrawNode(e);
    }
    protected override void OnMouseMove(MouseEventArgs e) {
        base.OnMouseMove(e);
        Cursor=PriorityNodeAt(e.Location)!=null || (simpleImageLinks && ImageNodeAt(e.Location)!=null)?Cursors.Hand:Cursors.Default;
    }
    protected override void OnMouseLeave(EventArgs e) {Cursor=Cursors.Default;base.OnMouseLeave(e);}
    internal bool HandleSimpleImageMessage(ref Message message) {
        if(HandlePriorityLinkMessage(ref message))return true;
        const int LeftDown=0x201,LeftUp=0x202,LeftDoubleClick=0x203,MouseMove=0x200,CaptureChanged=0x215;
        if(message.Msg==CaptureChanged){pressedImageNode=null;swallowImageUp=false;return false;}
        if(!simpleImageLinks || !Enabled)return false;
        if(message.Msg!=LeftDown && message.Msg!=LeftUp && message.Msg!=LeftDoubleClick && message.Msg!=MouseMove)return false;
        long coordinates=message.LParam.ToInt64();
        var point=new Point(unchecked((short)(coordinates&0xffff)),unchecked((short)((coordinates>>16)&0xffff)));
        if(message.Msg==LeftDown || message.Msg==LeftDoubleClick) {
            var node=ImageNodeAt(point);if(node==null)return false;
            Focus();pressedImageNode=message.Msg==LeftDown?node:null;swallowImageUp=true;Capture=true;
            return true;
        }
        if(message.Msg==MouseMove && swallowImageUp)return true;
        if(message.Msg==LeftUp && swallowImageUp) {
            var node=pressedImageNode;pressedImageNode=null;swallowImageUp=false;Capture=false;
            if(node!=null && ImageNodeAt(point)==node && SimpleImageClicked!=null)SimpleImageClicked(node);
            return true;
        }
        return false;
    }
}

internal sealed partial class FloatingWindow {
    [DllImport("user32.dll",EntryPoint="SendMessage")] static extern IntPtr SendSimpleMessage(IntPtr window,uint message,IntPtr wParam,IntPtr lParam);
    int simpleOutstandingVersion;
    bool simpleDetailsTesting;
    readonly HashSet<long> simpleCollapsedDuringRead=new HashSet<long>();

    void InitializeSimpleOutstanding() {
        tasks.SetSimpleImageLinks(true);
        tasks.SimpleImageAvailable=delegate(TreeNode node) {
            var leaf=node.Tag as OutstandingLeaf;
            if(leaf==null || node.Parent==null || !(node.Parent.Tag is long))return false;
            var images=new List<GalleryImage>();CollectImages(images,leaf.Html,"");return images.Count>0;
        };
        tasks.SimpleImageClicked=async delegate(TreeNode node) {
            var leaf=node.Tag as OutstandingLeaf;
            if(busy || closing || dragging || leaf==null || node.TreeView!=tasks)return;
            ShowSimpleModeRestore();hoverTimer.Stop();progressTip.Hide(tasks);
            try {await ShowImageGallery(leaf.TaskId,leaf.Html,this);}catch(Exception error){Error(error);}
        };
        tasks.AfterCollapse+=delegate(object sender,TreeViewEventArgs e){if(!rendering && !singleLine.Checked && e.Node.Tag is long)simpleCollapsedDuringRead.Add((long)e.Node.Tag);};
        tasks.AfterExpand+=delegate(object sender,TreeViewEventArgs e){if(!rendering && !singleLine.Checked && e.Node.Tag is long)simpleCollapsedDuringRead.Remove((long)e.Node.Tag);};
    }
    void InvalidateSimpleOutstanding() {simpleOutstandingVersion++;}
    Task RefreshSimpleOutstanding() {
        return RefreshSimpleOutstandingWithReader(async delegate(long id){return ReadShared(await ReadHistory(id));});
    }
    static IEnumerable<TreeNode> SimpleTaskNodes(TreeNodeCollection collection) {
        foreach(TreeNode node in collection) {
            if(!(node.Tag is long))continue;
            yield return node;
            foreach(var child in SimpleTaskNodes(node.Nodes))yield return child;
        }
    }
    async Task RefreshSimpleOutstandingWithReader(Func<long,Task<SharedList>> read) {
        if(closing || IsDisposed)return;
        int version=++simpleOutstandingVersion;
        var nodes=SimpleTaskNodes(tasks.Nodes).ToList();
        tasks.SetSimpleImageLinks(true);
        bool changed=false;Exception failed=null;
        using(var gate=new SemaphoreSlim(4,4)) {
            await Task.WhenAll(nodes.Select(async delegate(TreeNode node) {
                await gate.WaitAsync();
                try {
                    if(!SimpleOutstandingCurrent(version,node))return;
                    var shared=await read((long)node.Tag);
                    while(dragging && SimpleOutstandingCurrent(version,node))await Task.Delay(100);
                    if(!SimpleOutstandingCurrent(version,node))return;
                    if(ApplySimpleOutstanding(node,shared))changed=true;
                } catch(Exception error){if(SimpleOutstandingCurrent(version,node) && failed==null)failed=error;}
                finally {gate.Release();}
            }));
        }
        if(version==simpleOutstandingVersion && !closing && !IsDisposed) {
            if(changed){tasks.Invalidate();UpdateSimpleModeState();}
            if(failed!=null)throw new Exception("遗留事项读取失败，请刷新重试。",failed);
        }
    }
    bool SimpleOutstandingCurrent(int version,TreeNode node) {
        return version==simpleOutstandingVersion && !closing && !IsDisposed && node.TreeView==tasks;
    }
    static bool SimpleOutstandingMatches(TreeNode node,SharedList shared) {
        var leaves=node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is OutstandingLeaf).Select(child=>(OutstandingLeaf)child.Tag).ToList();
        if(leaves.Count!=shared.Items.Count)return false;
        for(int index=0;index<leaves.Count;index++) {
            var leaf=leaves[index];var item=shared.Items[index];
            if(leaf.TaskId!=(long)node.Tag || leaf.Id!=item.Id || leaf.Html!=item.Html || leaf.Done!=item.Done || leaf.CompletedAt!=item.CompletedAt || leaf.Priority!=item.Priority)return false;
        }
        return true;
    }
    sealed class OutstandingTreePosition {
        public TreeNode Node;
        public long TaskId;
        public string ItemId;
    }
    static OutstandingTreePosition CaptureOutstandingPosition(TreeNode node) {
        if(node==null)return null;
        var position=new OutstandingTreePosition{Node=node};
        var leaf=node.Tag as OutstandingLeaf;
        if(leaf!=null){position.TaskId=leaf.TaskId;position.ItemId=leaf.Id;}
        else if(node.Tag is long)position.TaskId=(long)node.Tag;
        return position;
    }
    static IEnumerable<TreeNode> OutstandingDescendants(TreeNodeCollection nodes) {
        foreach(TreeNode node in nodes){yield return node;foreach(var child in OutstandingDescendants(node.Nodes))yield return child;}
    }
    TreeNode ResolveOutstandingPosition(OutstandingTreePosition position) {
        if(position==null)return null;
        if(position.Node.TreeView==tasks)return position.Node;
        var nodes=OutstandingDescendants(tasks.Nodes).ToList();
        var task=nodes.FirstOrDefault(node=>node.Tag is long && (long)node.Tag==position.TaskId);
        if(position.ItemId!=null) {
            var candidates=nodes.Where(node=>node.Tag is OutstandingLeaf && ((OutstandingLeaf)node.Tag).Id==position.ItemId).ToList();
            var exact=candidates.FirstOrDefault(node=>((OutstandingLeaf)node.Tag).TaskId==position.TaskId);
            if(exact!=null)return exact;
            if(candidates.Count==1)return candidates[0];
        }
        return task;
    }
    void RestoreOutstandingPositions(OutstandingTreePosition selection,OutstandingTreePosition top) {
        var selected=ResolveOutstandingPosition(selection);
        if(selection!=null && tasks.SelectedNode!=selected)tasks.SelectedNode=selected;
        var first=ResolveOutstandingPosition(top);
        if(first!=null) {
            for(var ancestor=first.Parent;ancestor!=null;ancestor=ancestor.Parent)if(!ancestor.IsExpanded)first=ancestor;
            if(tasks.TopNode!=first)tasks.TopNode=first;
        }
    }
    bool ApplySimpleOutstanding(TreeNode node,SharedList shared) {
        if(SimpleOutstandingMatches(node,shared))return false;
        long id=(long)node.Tag;
        var selection=CaptureOutstandingPosition(tasks.SelectedNode);var top=CaptureOutstandingPosition(tasks.TopNode);
        bool wasRendering=rendering;rendering=true;tasks.BeginUpdate();
        try {
            // Clear native expansion before removing the last child; no placeholders are needed.
            if(shared.Items.Count==0 && !node.Nodes.Cast<TreeNode>().Any(child=>child.Tag is long))node.Collapse();
            foreach(var child in node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is OutstandingLeaf).ToArray())node.Nodes.Remove(child);
            for(int index=0;index<shared.Items.Count;index++) {
                var item=shared.Items[index];
                int number=item.Number>0?item.Number:index+1;
                var leaf=new TreeNode(number+". [P"+item.Priority+"] "+OutstandingText(item.Html)){
                    Tag=new OutstandingLeaf{TaskId=id,Id=item.Id,Html=item.Html,Done=item.Done,CompletedAt=item.CompletedAt,Priority=item.Priority},Checked=item.Done,
                    ForeColor=item.Done && grayCompleted?Color.FromArgb(100,110,125):ForeColor,
                    ToolTipText=(item.Done?"已完成 · ":"未完成 · ")+OutstandingText(item.Html)+" · 优先级 "+item.Priority};
                node.Nodes.Add(leaf);tasks.ReserveSimpleImageSpace(leaf);
            }
            tasks.SyncCompletionState(node);
            if(node.Nodes.Count==0)node.Collapse();
            else if(singleLine.Checked || (!simpleCollapsedDuringRead.Contains(id) && (search.Text.Trim().Length>0 || !collapsedTasks.Contains(id))))node.Expand();
            RestoreOutstandingPositions(selection,top);
        } finally {tasks.EndUpdate();rendering=wasRendering;}
        return true;
    }
    void ApplyBackgroundOutstanding(Dictionary<long,SharedList> lists) {
        if(closing || IsDisposed || lists==null)return;
        var updates=new List<KeyValuePair<TreeNode,SharedList>>();
        foreach(var node in SimpleTaskNodes(tasks.Nodes)) {
            SharedList shared;if(!lists.TryGetValue((long)node.Tag,out shared) || shared==null)continue;
            if(!SimpleOutstandingMatches(node,shared))updates.Add(new KeyValuePair<TreeNode,SharedList>(node,shared));
        }
        if(updates.Count==0)return;
        var selection=CaptureOutstandingPosition(tasks.SelectedNode);var top=CaptureOutstandingPosition(tasks.TopNode);
        bool wasRendering=rendering;rendering=true;tasks.BeginUpdate();
        try {
            foreach(var update in updates)ApplySimpleOutstanding(update.Key,update.Value);
            RestoreOutstandingPositions(selection,top);
        } finally {tasks.EndUpdate();rendering=wasRendering;}
        tasks.Invalidate();UpdateSimpleModeState();
    }

    async Task TestSimpleOutstandingDetails() {
        var originalNodes=tasks.Nodes.Cast<TreeNode>().ToArray();var originalSelected=tasks.SelectedNode;
        bool originalMode=simpleMode,originalBusy=busy,originalRendering=rendering,originalTesting=simpleDetailsTesting;
        var originalSize=Size;string originalSearch=search.Text;
        var originalCollapsed=collapsedTasks.ToArray();var originalReadCollapsed=simpleCollapsedDuringRead.ToArray();var clicked=tasks.SimpleImageClicked;
        simpleDetailsTesting=true;InvalidateSimpleOutstanding();
        try {
            SetBusy(false);rendering=true;SetSimpleMode(false);search.Clear();collapsedTasks.Clear();simpleCollapsedDuringRead.Clear();tasks.Nodes.Clear();
            var parent=new TreeNode("1. 事项"){Tag=900001L,Name="900001"};
            var child=new TreeNode("1.1. 子任务"){Tag=900002L,Name="900002"};
            var empty=new TreeNode("2. 无遗留事项"){Tag=900003L,Name="900003"};
            parent.Nodes.Add(child);tasks.Nodes.Add(parent);tasks.Nodes.Add(empty);rendering=false;
            var shared=new SharedList();
            shared.Items.Add(new PendingItem{Id="image",Html="核对图片<p><img src=\"/api/v1/tasks/900001/attachments/1\"></p>"});
            shared.Items.Add(new PendingItem{Id="plain",Html="联系负责人"});
            var changedShared=new SharedList();changedShared.Items.Add(new PendingItem{Id="plain",Html="联系负责人：已更新"});changedShared.Items.Add(shared.Items[0]);changedShared.Items.Add(new PendingItem{Id="new",Html="新增事项"});
            Func<long,Task<SharedList>> read=delegate(long id){return Task.FromResult(id==900001L?shared:new SharedList());};
            await RefreshSimpleOutstandingWithReader(read);
            if(parent.Nodes.Count!=3 || parent.Nodes[0]!=child || empty.Nodes.Count!=0 || child.Nodes.Count!=0)throw new Exception("Shared tree must use direct leaves and real child tasks without placeholders in full mode");
            var image=parent.Nodes[1];var plain=parent.Nodes[2];
            if(!image.Text.StartsWith("1. ") || !plain.Text.StartsWith("2. "))throw new Exception("Direct outstanding numbering failed");
            tasks.SyncCompletionStates();
            if(parent.StateImageIndex!=1 || child.StateImageIndex!=1 || image.StateImageIndex!=1 || plain.StateImageIndex!=1)throw new Exception("Task and outstanding completion boxes are not visible");
            tasks.SimpleImageClicked=delegate(TreeNode selected){if(selected!=image)throw new Exception("Image link selected a different leaf");};
            foreach(bool simple in new[]{false,true,false}) {
                parent.Expand();tasks.SelectedNode=image;var sameNodes=OutstandingDescendants(tasks.Nodes).ToArray();int previousVersion=simpleOutstandingVersion;
                SetSimpleMode(simple);if(simple)Size=new Size(360,350);
                if(simpleOutstandingVersion!=previousVersion || !sameNodes.SequenceEqual(OutstandingDescendants(tasks.Nodes)) || tasks.SelectedNode!=image || !parent.IsExpanded)throw new Exception("Layout switch changed data, selection, expansion, or cancelled the shared read generation");
                image.EnsureVisible();tasks.Refresh();var stableTop=tasks.TopNode;int invalidations=0;
                InvalidateEventHandler invalidated=delegate{invalidations++;};tasks.Invalidated+=invalidated;
                try {
                    tasks.SetSimpleImageLinks(true);
                    if(ApplySimpleOutstanding(parent,shared))throw new Exception("Identical shared items reported a mutation");
                    await RefreshSimpleOutstandingWithReader(read);
                    ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{900001L,shared},{900002L,new SharedList()},{900003L,new SharedList()}});
                    if(invalidations!=0 || parent.Nodes[1]!=image || parent.Nodes[2]!=plain || tasks.SelectedNode!=image || tasks.TopNode!=stableTop)throw new Exception("Unchanged full/simple refresh invalidated or recreated the shared tree");
                } finally {tasks.Invalidated-=invalidated;}
                Rectangle imageLink=tasks.SimpleImageBounds(image);
                if(imageLink.IsEmpty || !tasks.SimpleImageBounds(plain).IsEmpty || !tasks.SimpleImageBounds(child).IsEmpty)throw new Exception("Both layouts must expose image links only on image leaves");
                if(imageLink.Left<=image.Bounds.Left+2)throw new Exception("Outstanding image link must be placed after its sequence number");
                Rectangle taskCheck=tasks.CompletionBounds(child),leafSpace=tasks.CompletionBounds(image);
                if(taskCheck.IsEmpty || leafSpace.IsEmpty || taskCheck.Left!=leafSpace.Left || taskCheck.Width!=leafSpace.Width)throw new Exception("Task completion box and outstanding row are misaligned");
                var originalCompletion=tasks.CompletionClicked;int completions=0;TreeNode completionNode=null;
                tasks.CompletionClicked=delegate(TreeNode selected){completions++;completionNode=selected;};
                try {
                    int checkPoint=((taskCheck.Top+taskCheck.Height/2)<<16)|((taskCheck.Left+taskCheck.Width/2)&0xffff);
                    SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(checkPoint));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(checkPoint));
                    int leafPoint=((leafSpace.Top+leafSpace.Height/2)<<16)|((leafSpace.Left+leafSpace.Width/2)&0xffff);
                    SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(leafPoint));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(leafPoint));
                    tasks.SelectedNode=child;SendSimpleMessage(tasks.Handle,0x100,new IntPtr((int)Keys.Space),IntPtr.Zero);
                    if(completions!=3 || completionNode!=child)throw new Exception("Completion mouse/keyboard input missed a task or outstanding row");
                } finally {tasks.CompletionClicked=originalCompletion;}
                int clicks=0,checks=0,drags=0,doubleClicks=0;
                TreeViewCancelEventHandler check=delegate{checks++;};ItemDragEventHandler drag=delegate{drags++;};TreeNodeMouseClickEventHandler doubleClick=delegate{doubleClicks++;};
                tasks.BeforeCheck+=check;tasks.ItemDrag+=drag;tasks.NodeMouseDoubleClick+=doubleClick;
                tasks.SimpleImageClicked=delegate(TreeNode selected){if(selected!=image)throw new Exception("Image link selected the wrong leaf");clicks++;};
                try {
                    int coordinates=((imageLink.Top+imageLink.Height/2)<<16)|((imageLink.Left+imageLink.Width/2)&0xffff);
                    SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(coordinates));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(coordinates));
                    SendSimpleMessage(tasks.Handle,0x203,new IntPtr(1),new IntPtr(coordinates));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(coordinates));
                    if(clicks!=1 || checks!=0 || drags!=0 || doubleClicks!=0)throw new Exception("Shared image link leaked into checkbox, drag, or progress/editor actions");
                    var priorityLink=tasks.PriorityLinkBounds(image);
                    var point=new Point(priorityLink.Right+8,imageLink.Top+imageLink.Height/2);
                    var down=Message.Create(tasks.Handle,0x201,new IntPtr(1),new IntPtr((point.Y<<16)|(point.X&0xffff)));
                    if(tasks.HandleSimpleImageMessage(ref down))throw new Exception("Image leaf text lost normal drag handling");
                    string remainingTitle=Regex.Replace(image.Text.TrimEnd(' '),@"^[0-9]+(?:\.[0-9]+)*\.\s+","");
                    int titleEnd=imageLink.Right+6+TextRenderer.MeasureText(remainingTitle,tasks.Font,Size.Empty,TextFormatFlags.NoPadding).Width;
                    var hit=tasks.HitTest(new Point(titleEnd-3,imageLink.Top+imageLink.Height/2));
                    if(titleEnd>image.Bounds.Right || hit.Node!=image || (hit.Location&TreeViewHitTestLocations.Label)==0)throw new Exception("Shared image title extends beyond its native drag/select target");
                    var move=MakeDropPlan(image,empty,0);var order=MakeDropPlan(image,plain,1);
                    if(move==null || move.TargetId!=900003L || move.ItemId!="image" || order==null || order.TargetId!=900001L || order.BeforeItemId!="")throw new Exception("Shared leaf move/reorder targets differ between layouts");
                } finally {tasks.BeforeCheck-=check;tasks.ItemDrag-=drag;tasks.NodeMouseDoubleClick-=doubleClick;}
            }
            bool failed=false;var preserved=OutstandingDescendants(tasks.Nodes).ToArray();
            try {await RefreshSimpleOutstandingWithReader(delegate(long taskId){var error=new TaskCompletionSource<SharedList>();error.SetException(new Exception("Expected read failure"));return error.Task;});}catch(Exception){failed=true;}
            if(!failed || !preserved.SequenceEqual(OutstandingDescendants(tasks.Nodes)))throw new Exception("Failed reads must retain cached direct leaves in either layout");
            foreach(bool startSimple in new[]{false,true}) {
                SetSimpleMode(startSimple);parent.Expand();tasks.SelectedNode=parent;
                var pending=new TaskCompletionSource<SharedList>();
                Task inFlight=RefreshSimpleOutstandingWithReader(delegate(long id){return id==900001L?pending.Task:Task.FromResult(new SharedList());});
                int version=simpleOutstandingVersion;parent.Collapse();var waitingNodes=OutstandingDescendants(tasks.Nodes).ToArray();
                SetSimpleMode(!startSimple);
                if(simpleOutstandingVersion!=version || !waitingNodes.SequenceEqual(OutstandingDescendants(tasks.Nodes)))throw new Exception("Switching layout cancelled or rebuilt an in-flight shared tree");
                var result=startSimple?shared:changedShared;pending.SetResult(result);await inFlight;
                var actual=parent.Nodes.Cast<TreeNode>().Where(node=>node.Tag is OutstandingLeaf).Select(node=>(OutstandingLeaf)node.Tag).ToArray();
                if(parent.IsExpanded || actual.Length!=result.Items.Count || !actual.Select(item=>item.Id+"|"+item.Html).SequenceEqual(result.Items.Select(item=>item.Id+"|"+item.Html)))throw new Exception("Pending shared read did not survive a layout switch or reopened a manually collapsed task");
            }
            foreach(bool simple in new[]{false,true}) {
                SetSimpleMode(simple);parent.Expand();
                ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{900001L,changedShared},{900003L,shared}});
                var values=parent.Nodes.Cast<TreeNode>().Where(node=>node.Tag is OutstandingLeaf).Select(node=>(OutstandingLeaf)node.Tag).ToArray();
                if(values.Length!=3 || values[0].Id!="plain" || values[0].Html!="联系负责人：已更新" || values[1].Id!="image" || values[2].Id!="new")throw new Exception("Shared background changes lost additions, edits, or ordering");
                ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{900001L,new SharedList()},{900003L,new SharedList()}});
                if(parent.Nodes.Count!=1 || parent.Nodes[0]!=child || empty.Nodes.Count!=0 || empty.IsExpanded)throw new Exception("Removing outstanding items must leave only real child tasks and no empty expansion");
                await RefreshSimpleOutstandingWithReader(read);
            }
            var staleResult=new TaskCompletionSource<SharedList>();
            Task stale=RefreshSimpleOutstandingWithReader(delegate(long id){return id==900001L?staleResult.Task:Task.FromResult(new SharedList());});
            await RefreshSimpleOutstandingWithReader(read);preserved=OutstandingDescendants(tasks.Nodes).ToArray();
            staleResult.SetResult(changedShared);await stale;
            if(!preserved.SequenceEqual(OutstandingDescendants(tasks.Nodes)))throw new Exception("An older shared read replaced the newer generation");
            parent.Expand();tasks.SelectedNode=parent;SetSimpleMode(true);Size=new Size(360,350);
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-simple-details-test.png"));}
            var detachedResult=new TaskCompletionSource<SharedList>();
            stale=RefreshSimpleOutstandingWithReader(delegate(long id){return id==900001L?detachedResult.Task:Task.FromResult(new SharedList());});
            var fresh=new TreeNode("重新载入的任务"){Tag=900001L};parent.Remove();tasks.Nodes.Add(fresh);detachedResult.SetResult(changedShared);await stale;
            if(fresh.Nodes.Count!=0)throw new Exception("Detached shared read leaked into a replacement task node");
            File.WriteAllText(Path.Combine(data,"floating-simple-details-test.txt"),"PASS: aligned task and outstanding completion boxes with working mouse/keyboard callbacks in both layouts; one direct task/outstanding tree in both layouts; stable nodes and read generation across switches; shared image links and click isolation; native title drag/select targets; matching drop plans; unchanged refresh zero invalidation; cached data retained on read errors; in-flight reads survive layout changes and respect manual collapse; shared additions/edits/deletions/order; empty tasks have no placeholders; newer generation and detached-node guards.");
        } finally {
            InvalidateSimpleOutstanding();tasks.SimpleImageClicked=clicked;rendering=true;SetSimpleMode(false);tasks.Nodes.Clear();tasks.Nodes.AddRange(originalNodes);
            collapsedTasks.Clear();collapsedTasks.UnionWith(originalCollapsed);simpleCollapsedDuringRead.Clear();simpleCollapsedDuringRead.UnionWith(originalReadCollapsed);search.Text=originalSearch;
            if(originalMode)SetSimpleMode(true);Size=originalSize;
            if(originalSelected!=null && originalSelected.TreeView==tasks)tasks.SelectedNode=originalSelected;
            rendering=originalRendering;simpleDetailsTesting=originalTesting;SetBusy(originalBusy);
        }
    }
}
