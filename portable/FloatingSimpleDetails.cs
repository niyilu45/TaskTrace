// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
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
        Font font=node.NodeFont??Font;
        int width=TextRenderer.MeasureText("图片",font,Size.Empty,TextFormatFlags.NoPadding).Width;
        return new Rectangle(bounds.Left+2,bounds.Top,width+4,bounds.Height);
    }
    internal void ReserveSimpleImageSpace(TreeNode node) {
        if(!simpleImageLinks || SimpleImageAvailable==null || !SimpleImageAvailable(node))return;
        Font font=node.NodeFont??Font;
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
        Font font=e.Node.NodeFont??Font;
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
    bool simpleDetailsActive,simpleDetailsTesting;
    readonly HashSet<long> simpleCollapsedDuringRead=new HashSet<long>();

    void InitializeSimpleOutstanding() {
        tasks.SimpleImageAvailable=delegate(TreeNode node) {
            var leaf=node.Tag as OutstandingLeaf;
            if(!simpleMode || leaf==null || node.Parent==null || !(node.Parent.Tag is long))return false;
            var images=new List<GalleryImage>();CollectImages(images,leaf.Html,"");return images.Count>0;
        };
        tasks.SimpleImageClicked=async delegate(TreeNode node) {
            var leaf=node.Tag as OutstandingLeaf;
            if(busy || closing || !simpleMode || leaf==null || node.TreeView!=tasks)return;
            ShowSimpleModeRestore();hoverTimer.Stop();progressTip.Hide(tasks);
            try {await ShowImageGallery(leaf.TaskId,leaf.Html,this);}catch(Exception error){Error(error);}
        };
        tasks.AfterCollapse+=delegate(object sender,TreeViewEventArgs e){if(!rendering && simpleMode && e.Node.Tag is long)simpleCollapsedDuringRead.Add((long)e.Node.Tag);};
        tasks.AfterExpand+=delegate(object sender,TreeViewEventArgs e){if(!rendering && e.Node.Tag is long)simpleCollapsedDuringRead.Remove((long)e.Node.Tag);};
    }
    void InvalidateSimpleOutstanding() {simpleOutstandingVersion++;}
    async void RefreshSimpleOutstandingInBackground() {
        if(simpleDetailsTesting)return;
        if(rendering){InvalidateSimpleOutstanding();tasks.SetSimpleImageLinks(simpleMode);return;}
        try {await RefreshSimpleOutstanding();}catch(Exception error){if(!closing && !IsDisposed)Error(error);}
    }
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
    void RestoreNormalOutstanding(List<TreeNode> nodes) {
        if(!simpleDetailsActive)return;
        if(nodes.Count==0){simpleDetailsActive=false;simpleCollapsedDuringRead.Clear();return;}
        bool wasRendering=rendering;rendering=true;tasks.BeginUpdate();
        try {
            foreach(var node in nodes) {
                bool expanded=node.IsExpanded;
                foreach(var leaf in node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is OutstandingLeaf).ToArray())node.Nodes.Remove(leaf);
                if(!node.Nodes.Cast<TreeNode>().Any(child=>child.Tag is OutstandingBranch)) {
                    var branch=new TreeNode("遗留事项（展开查看，双击管理）"){Tag=new OutstandingBranch{TaskId=(long)node.Tag}};
                    branch.Nodes.Add(new TreeNode("读取中…"));node.Nodes.Add(branch);
                }
                if(expanded)node.Expand();
            }
            simpleDetailsActive=false;simpleCollapsedDuringRead.Clear();
        } finally {tasks.EndUpdate();rendering=wasRendering;}
    }
    async Task RefreshSimpleOutstandingWithReader(Func<long,Task<SharedList>> read) {
        int version=++simpleOutstandingVersion;
        if(closing || IsDisposed)return;
        var nodes=SimpleTaskNodes(tasks.Nodes).ToList();
        tasks.SetSimpleImageLinks(simpleMode);
        if(!simpleMode){RestoreNormalOutstanding(nodes);return;}
        simpleDetailsActive=true;
        var branches=nodes.SelectMany(node=>node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is OutstandingBranch)).ToList();
        bool changed=branches.Count>0;
        if(changed) {
            bool wasRendering=rendering;rendering=true;tasks.BeginUpdate();
            try {foreach(var branch in branches)branch.Remove();}
            finally {tasks.EndUpdate();rendering=wasRendering;}
        }
        Exception failed=null;
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
        return version==simpleOutstandingVersion && simpleMode && !closing && !IsDisposed && node.TreeView==tasks;
    }
    static bool OutstandingLeavesMatch(IEnumerable<TreeNode> nodes,long taskId,SharedList shared) {
        var leaves=nodes.Select(node=>node.Tag as OutstandingLeaf).ToList();
        if(leaves.Count!=shared.Items.Count)return false;
        for(int index=0;index<leaves.Count;index++) {
            var leaf=leaves[index];var item=shared.Items[index];
            if(leaf==null || leaf.TaskId!=taskId || leaf.Id!=item.Id || leaf.Html!=item.Html)return false;
        }
        return true;
    }
    static bool SimpleOutstandingMatches(TreeNode node,SharedList shared) {
        if(node.Nodes.Cast<TreeNode>().Any(child=>child.Tag is OutstandingBranch))return false;
        return OutstandingLeavesMatch(node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is OutstandingLeaf),(long)node.Tag,shared);
    }
    static bool NormalOutstandingMatches(TreeNode node,SharedList shared) {
        var branch=(OutstandingBranch)node.Tag;
        if(shared.Items.Count==0 && node.Nodes.Count==1) {
            var placeholder=node.Nodes[0].Tag as OutstandingBranch;
            return placeholder!=null && placeholder.Loaded && placeholder.TaskId==branch.TaskId;
        }
        return OutstandingLeavesMatch(node.Nodes.Cast<TreeNode>(),branch.TaskId,shared);
    }
    sealed class OutstandingTreePosition {
        public TreeNode Node;
        public long TaskId;
        public string ItemId;
        public bool Branch,Placeholder;
    }
    static OutstandingTreePosition CaptureOutstandingPosition(TreeNode node) {
        if(node==null)return null;
        var position=new OutstandingTreePosition{Node=node};
        var leaf=node.Tag as OutstandingLeaf;var branch=node.Tag as OutstandingBranch;
        if(leaf!=null){position.TaskId=leaf.TaskId;position.ItemId=leaf.Id;}
        else if(branch!=null){position.TaskId=branch.TaskId;position.Branch=true;position.Placeholder=node.Parent!=null && node.Parent.Tag is OutstandingBranch;}
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
        if(position.Branch && task!=null) {
            var branch=task.Nodes.Cast<TreeNode>().FirstOrDefault(node=>node.Tag is OutstandingBranch);
            if(branch!=null)return position.Placeholder?branch.Nodes.Cast<TreeNode>().FirstOrDefault(node=>node.Tag is OutstandingBranch)??branch:branch;
        }
        return task;
    }
    void RestoreOutstandingPositions(OutstandingTreePosition selection,OutstandingTreePosition top) {
        var selected=ResolveOutstandingPosition(selection);
        if(selection!=null && tasks.SelectedNode!=selected)tasks.SelectedNode=selected;
        var first=ResolveOutstandingPosition(top);
        if(first!=null) {
            // Do not open a branch that the user deliberately kept collapsed.
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
            foreach(var child in node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is OutstandingLeaf || child.Tag is OutstandingBranch).ToArray())node.Nodes.Remove(child);
            for(int index=0;index<shared.Items.Count;index++) {
                var item=shared.Items[index];
                var leaf=new TreeNode((index+1)+". "+OutstandingText(item.Html)){Tag=new OutstandingLeaf{TaskId=id,Id=item.Id,Html=item.Html}};
                node.Nodes.Add(leaf);tasks.ReserveSimpleImageSpace(leaf);
            }
            if(!simpleCollapsedDuringRead.Contains(id) && (search.Text.Trim().Length>0 || !collapsedTasks.Contains(id)))node.Expand();
            RestoreOutstandingPositions(selection,top);
        } finally {tasks.EndUpdate();rendering=wasRendering;}
        return true;
    }
    void ApplyNormalOutstanding(TreeNode node,SharedList shared) {
        var branch=(OutstandingBranch)node.Tag;bool expanded=node.IsExpanded;
        node.Nodes.Clear();
        for(int index=0;index<shared.Items.Count;index++) {
            var item=shared.Items[index];
            node.Nodes.Add(new TreeNode((index+1)+". "+OutstandingText(item.Html)){Tag=new OutstandingLeaf{TaskId=branch.TaskId,Id=item.Id,Html=item.Html}});
        }
        if(shared.Items.Count==0)node.Nodes.Add(new TreeNode("暂无遗留事项，双击此处添加"){Tag=new OutstandingBranch{TaskId=branch.TaskId,Loaded=true}});
        if(expanded)node.Expand();
    }
    void ApplyBackgroundOutstanding(Dictionary<long,SharedList> lists) {
        if(closing || IsDisposed || lists==null)return;
        var updates=new List<KeyValuePair<TreeNode,SharedList>>();
        foreach(var node in SimpleTaskNodes(tasks.Nodes)) {
            SharedList shared;if(!lists.TryGetValue((long)node.Tag,out shared) || shared==null)continue;
            if(simpleMode) {
                if(!SimpleOutstandingMatches(node,shared))updates.Add(new KeyValuePair<TreeNode,SharedList>(node,shared));
            } else foreach(var branch in node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is OutstandingBranch && ((OutstandingBranch)child.Tag).Loaded)) {
                if(!NormalOutstandingMatches(branch,shared))updates.Add(new KeyValuePair<TreeNode,SharedList>(branch,shared));
            }
        }
        if(updates.Count==0)return;
        var selection=CaptureOutstandingPosition(tasks.SelectedNode);var top=CaptureOutstandingPosition(tasks.TopNode);
        bool wasRendering=rendering;rendering=true;tasks.BeginUpdate();
        try {
            foreach(var update in updates) {
                if(simpleMode)ApplySimpleOutstanding(update.Key,update.Value);
                else ApplyNormalOutstanding(update.Key,update.Value);
            }
            RestoreOutstandingPositions(selection,top);
        } finally {tasks.EndUpdate();rendering=wasRendering;}
        tasks.Invalidate();UpdateSimpleModeState();
    }

    async Task TestSimpleOutstandingDetails() {
        var originalNodes=tasks.Nodes.Cast<TreeNode>().ToArray();bool originalMode=simpleMode;
        var originalSize=Size;bool originalBusy=busy;string originalSearch=search.Text;
        var originalCollapsed=collapsedTasks.ToArray();var clicked=tasks.SimpleImageClicked;
        simpleDetailsTesting=true;InvalidateSimpleOutstanding();
        try {
            SetBusy(false);rendering=true;search.Clear();collapsedTasks.Clear();tasks.Nodes.Clear();
            var parent=new TreeNode("1. 简洁事项"){Tag=900001L,Name="900001"};
            var child=new TreeNode("1.1. 子任务"){Tag=900002L,Name="900002"};
            var empty=new TreeNode("2. 无遗留事项"){Tag=900003L,Name="900003"};
            parent.Nodes.Add(child);tasks.Nodes.Add(parent);tasks.Nodes.Add(empty);
            var oldBranch=new TreeNode("遗留事项"){Tag=new OutstandingBranch{TaskId=900001L}};oldBranch.Nodes.Add("读取中…");parent.Nodes.Add(oldBranch);
            SetSimpleMode(true);Size=new Size(360,330);rendering=false;
            var shared=new SharedList();
            shared.Items.Add(new PendingItem{Id="image",Html="核对图片<p><img src=\"/api/v1/tasks/900001/attachments/1\"></p>"});
            shared.Items.Add(new PendingItem{Id="plain",Html="联系负责人"});
            Func<long,Task<SharedList>> read=delegate(long id){return Task.FromResult(id==900001L?shared:new SharedList());};
            await RefreshSimpleOutstandingWithReader(read);
            if(parent.Nodes.Count!=3 || parent.Nodes[0]!=child || empty.Nodes.Count!=0 || child.Nodes.Count!=0)throw new Exception("Simple outstanding leaves must be direct, preserve child tasks, and omit empty layers");
            var image=parent.Nodes[1];var plain=parent.Nodes[2];
            if(!(image.Tag is OutstandingLeaf) || !image.Text.StartsWith("1. ") || !plain.Text.StartsWith("2. ") || parent.Nodes.Cast<TreeNode>().Any(node=>node.Tag is OutstandingBranch))throw new Exception("Simple outstanding structure or numbering failed");
            parent.Expand();image.EnsureVisible();tasks.Refresh();
            tasks.SelectedNode=image;var stableTop=tasks.TopNode;int invalidations=0;
            InvalidateEventHandler invalidated=delegate{invalidations++;};tasks.Invalidated+=invalidated;
            try {
                if(ApplySimpleOutstanding(parent,shared))throw new Exception("Unchanged simple outstanding data reported a mutation");
                tasks.SetSimpleImageLinks(true);await RefreshSimpleOutstandingWithReader(read);
                ApplyBackgroundOutstanding(new Dictionary<long,SharedList>{{900001L,shared},{900002L,new SharedList()},{900003L,new SharedList()}});
                if(invalidations!=0 || parent.Nodes[1]!=image || parent.Nodes[2]!=plain || !parent.IsExpanded || tasks.SelectedNode!=image || tasks.TopNode!=stableTop)throw new Exception("Unchanged simple refresh invalidated, recreated, expanded, selected, or scrolled the tree");
            } finally {tasks.Invalidated-=invalidated;}
            parent.Collapse();var collapsedSelection=tasks.SelectedNode;
            if(ApplySimpleOutstanding(parent,shared) || parent.IsExpanded || tasks.SelectedNode!=collapsedSelection)throw new Exception("Unchanged outstanding data reopened a collapsed task");
            parent.Expand();image.EnsureVisible();tasks.Refresh();
            Rectangle imageLink=tasks.SimpleImageBounds(image);
            if(imageLink.IsEmpty || !tasks.SimpleImageBounds(plain).IsEmpty || !tasks.SimpleImageBounds(child).IsEmpty)throw new Exception("Image links must only appear on leaves with pictures");
            int clicks=0,checks=0,drags=0,doubleClicks=0;
            TreeViewCancelEventHandler check=delegate{checks++;};ItemDragEventHandler drag=delegate{drags++;};TreeNodeMouseClickEventHandler doubleClick=delegate{doubleClicks++;};
            tasks.BeforeCheck+=check;tasks.ItemDrag+=drag;tasks.NodeMouseDoubleClick+=doubleClick;
            tasks.SimpleImageClicked=delegate(TreeNode selected){if(selected!=image)throw new Exception("Image link selected the wrong leaf");clicks++;};
            try {
                int coordinates=(imageLink.Top+imageLink.Height/2)<<16 | (imageLink.Left+imageLink.Width/2)&0xffff;
                SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(coordinates));
                SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(coordinates));
                SendSimpleMessage(tasks.Handle,0x203,new IntPtr(1),new IntPtr(coordinates));
                SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(coordinates));
                if(clicks!=1 || checks!=0 || drags!=0 || doubleClicks!=0)throw new Exception("Image click leaked into checkbox, dragging, or editor actions");
                var textPoint=new Point(imageLink.Right+8,imageLink.Top+imageLink.Height/2);
                var normalDown=Message.Create(tasks.Handle,0x201,new IntPtr(1),new IntPtr((textPoint.Y<<16)|(textPoint.X&0xffff)));
                if(tasks.HandleSimpleImageMessage(ref normalDown))throw new Exception("Image leaf text must retain ordinary drag handling");
                int titleEnd=imageLink.Right+6+TextRenderer.MeasureText(image.Text.TrimEnd(' '),tasks.Font,Size.Empty,TextFormatFlags.NoPadding).Width;
                if(titleEnd>image.Bounds.Right)throw new Exception("Image label tail exceeds native bounds: titleEnd="+titleEnd+", bounds="+image.Bounds+", link="+imageLink+", text="+image.Text.Length+"/"+image.Text.TrimEnd(' ').Length);
                var tailHit=tasks.HitTest(new Point(titleEnd-3,imageLink.Top+imageLink.Height/2));
                if(tailHit.Node!=image || (tailHit.Location&TreeViewHitTestLocations.Label)==0)throw new Exception("Image label tail is outside the native label hit target");
                var move=MakeDropPlan(image,empty,0);var order=MakeDropPlan(image,plain,1);
                if(move==null || move.TargetId!=900003L || move.ItemId!="image" || order==null || order.TargetId!=900001L || order.BeforeItemId!="")throw new Exception("Direct outstanding leaves lost move/reorder targets");
                using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-simple-details-test.png"));}
            } finally {tasks.BeforeCheck-=check;tasks.ItemDrag-=drag;tasks.NodeMouseDoubleClick-=doubleClick;tasks.SimpleImageClicked=clicked;}
            var pending=new TaskCompletionSource<SharedList>();
            Task stale=RefreshSimpleOutstandingWithReader(delegate(long id){return id==900001L?pending.Task:Task.FromResult(new SharedList());});
            SetSimpleMode(false);await RefreshSimpleOutstandingWithReader(read);pending.SetResult(shared);await stale;
            if(parent.Nodes.Cast<TreeNode>().Any(node=>node.Tag is OutstandingLeaf) || !parent.Nodes.Cast<TreeNode>().Any(node=>node.Tag is OutstandingBranch) || tasks.DrawMode!=TreeViewDrawMode.OwnerDrawText || !tasks.SimpleImageBounds(image).IsEmpty)throw new Exception("Late simple results replaced normal outstanding branches");
            var normal=parent.Nodes.Cast<TreeNode>().First(node=>node.Tag is OutstandingBranch);
            ((OutstandingBranch)normal.Tag).Loaded=true;
            var unloaded=child.Nodes.Cast<TreeNode>().First(node=>node.Tag is OutstandingBranch);var unloadedChild=unloaded.Nodes[0];
            var backgroundLists=new Dictionary<long,SharedList>{{900001L,shared},{900002L,shared}};
            ApplyBackgroundOutstanding(backgroundLists);normal.Expand();
            var normalImage=normal.Nodes[0];var normalPlain=normal.Nodes[1];tasks.SelectedNode=normalImage;stableTop=tasks.TopNode;invalidations=0;
            tasks.Invalidated+=invalidated;
            try {
                tasks.SetSimpleImageLinks(false);ApplyBackgroundOutstanding(backgroundLists);
                if(invalidations!=0 || normal.Nodes[0]!=normalImage || normal.Nodes[1]!=normalPlain || !normal.IsExpanded || tasks.SelectedNode!=normalImage || tasks.TopNode!=stableTop || unloaded.Nodes[0]!=unloadedChild || ((OutstandingBranch)unloaded.Tag).Loaded)throw new Exception("Unchanged normal background refresh mutated loaded or unloaded branches");
            } finally {tasks.Invalidated-=invalidated;}
            var changedShared=new SharedList();changedShared.Items.Add(new PendingItem{Id="plain",Html="联系负责人：已更新"});changedShared.Items.Add(shared.Items[0]);
            backgroundLists[900001L]=changedShared;ApplyBackgroundOutstanding(backgroundLists);
            if(!normal.IsExpanded || ((OutstandingLeaf)normal.Nodes[0].Tag).Html!="联系负责人：已更新" || ((OutstandingLeaf)normal.Nodes[1].Tag).Id!="image" || tasks.SelectedNode!=normal.Nodes[1] || tasks.TopNode!=stableTop)throw new Exception("Changed normal background refresh lost contents, order, expansion, selection, or scroll");
            normal.Collapse();backgroundLists[900001L]=new SharedList();ApplyBackgroundOutstanding(backgroundLists);
            if(normal.IsExpanded || normal.Nodes.Count!=1 || !(normal.Nodes[0].Tag is OutstandingBranch))throw new Exception("Empty normal background refresh reopened the branch or lost its placeholder");
            var emptyPlaceholder=normal.Nodes[0];ApplyBackgroundOutstanding(backgroundLists);
            if(normal.Nodes[0]!=emptyPlaceholder)throw new Exception("Unchanged empty normal branch was recreated");
            SetSimpleMode(true);
            var replaced=new TaskCompletionSource<SharedList>();
            stale=RefreshSimpleOutstandingWithReader(delegate(long id){return replaced.Task;});
            var fresh=new TreeNode("新树"){Tag=900001L};tasks.Nodes.Clear();tasks.Nodes.Add(fresh);replaced.SetResult(shared);await stale;
            if(fresh.Nodes.Count!=0)throw new Exception("Detached task results leaked into a reloaded tree");
            File.WriteAllText(Path.Combine(data,"floating-simple-details-test.txt"),"PASS: direct numbered outstanding leaves; no empty layers; child tasks preserved; image-only links; single scoped click without check/drag/editor; move/reorder targets; normal branch restoration; stale mode/tree results discarded; identical simple/loaded normal refresh keeps references, selection, scroll, expansion and avoids invalidation; changed/empty loaded branches update without opening; unloaded branches untouched.");
        } finally {
            InvalidateSimpleOutstanding();tasks.SimpleImageClicked=clicked;rendering=true;
            SetSimpleMode(false);tasks.Nodes.Clear();tasks.Nodes.AddRange(originalNodes);simpleDetailsActive=false;
            collapsedTasks.Clear();foreach(long id in originalCollapsed)collapsedTasks.Add(id);simpleCollapsedDuringRead.Clear();search.Text=originalSearch;
            if(originalMode)SetSimpleMode(true);Size=originalSize;rendering=false;simpleDetailsTesting=false;SetBusy(originalBusy);
        }
        await RefreshSimpleOutstanding();
    }
}
