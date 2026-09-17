// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text.RegularExpressions;
using System.Windows.Forms;

internal sealed partial class TaskTreeView {
    internal Action<TreeNode> PriorityClicked;
    static readonly Regex PriorityPrefix=new Regex(@"^(?<prefix>[0-9]+(?:\.[0-9]+)*\.\s+)(?<priority>\[P[0-9]\])(?=\s|$)");
    TreeNode pressedPriorityNode;
    bool swallowPriorityUp;

    static Match TaskPriorityMatch(TreeNode node) {
        return node!=null && node.Tag is long?PriorityPrefix.Match(node.Text):Match.Empty;
    }
    static int PriorityTextAdvance(string text,Font font) {
        // Appending a glyph measures trailing spaces without MeasureText's minimum-width shortcut.
        return TextRenderer.MeasureText(text+"x",font,Size.Empty,TextFormatFlags.NoPadding).Width-TextRenderer.MeasureText("x",font,Size.Empty,TextFormatFlags.NoPadding).Width;
    }
    internal Rectangle PriorityLinkBounds(TreeNode node) {
        if(node==null || node.TreeView!=this)return Rectangle.Empty;
        var match=TaskPriorityMatch(node);var bounds=node.Bounds;
        if(!match.Success || bounds.Height<=0)return Rectangle.Empty;
        Font font=node.NodeFont??Font;
        int left=bounds.Left+2+PriorityTextAdvance(match.Groups["prefix"].Value,font);
        int width=PriorityTextAdvance(match.Groups["priority"].Value,font);
        return new Rectangle(left,bounds.Top,width,bounds.Height);
    }
    TreeNode PriorityNodeAt(Point point) {
        if(!ClientRectangle.Contains(point))return null;
        for(var node=TopNode;node!=null;node=node.NextVisibleNode) {
            if(node.Bounds.Top>point.Y)break;
            if(PriorityLinkBounds(node).Contains(point))return node;
        }
        return null;
    }
    internal bool DrawPriorityLink(DrawTreeNodeEventArgs e) {
        var link=PriorityLinkBounds(e.Node);if(link.IsEmpty)return false;
        var match=TaskPriorityMatch(e.Node);Font font=e.Node.NodeFont??Font;
        bool selected=(e.State&TreeNodeStates.Selected)!=0;
        Color background=selected?SystemColors.Highlight:BackColor;
        Color foreground=selected?SystemColors.HighlightText:e.Node.ForeColor;
        if(foreground.IsEmpty)foreground=ForeColor;
        if(!Enabled)foreground=SystemColors.GrayText;
        var bounds=e.Node.Bounds;
        using(var brush=new SolidBrush(background))e.Graphics.FillRectangle(brush,bounds);
        var text=new Rectangle(bounds.Left+2,bounds.Top,Math.Max(0,bounds.Width-2),bounds.Height);
        TextRenderer.DrawText(e.Graphics,e.Node.Text,font,text,foreground,TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix|TextFormatFlags.VerticalCenter|TextFormatFlags.SingleLine);
        // Repaint only the marker; the native text/title and its hit-test width stay unchanged.
        using(var brush=new SolidBrush(background))e.Graphics.FillRectangle(brush,link);
        using(var underline=new Font(font,font.Style|FontStyle.Underline)) {
            Color color=!Enabled?SystemColors.GrayText:selected?SystemColors.HighlightText:Color.FromArgb(36,94,210);
            TextRenderer.DrawText(e.Graphics,match.Groups["priority"].Value,underline,link,color,TextFormatFlags.NoPadding|TextFormatFlags.VerticalCenter|TextFormatFlags.SingleLine);
        }
        if(selected && Focused)ControlPaint.DrawFocusRectangle(e.Graphics,bounds,foreground,background);
        e.DrawDefault=false;return true;
    }
    void ResetPriorityLinkPress() {pressedPriorityNode=null;swallowPriorityUp=false;}
    internal bool HandlePriorityLinkMessage(ref Message message) {
        const int LeftDown=0x201,LeftUp=0x202,LeftDoubleClick=0x203,MouseMove=0x200,CaptureChanged=0x215;
        if(message.Msg==CaptureChanged){ResetPriorityLinkPress();return false;}
        if(!Enabled)return false;
        if(message.Msg!=LeftDown && message.Msg!=LeftUp && message.Msg!=LeftDoubleClick && message.Msg!=MouseMove)return false;
        long coordinates=message.LParam.ToInt64();
        var point=new Point(unchecked((short)(coordinates&0xffff)),unchecked((short)((coordinates>>16)&0xffff)));
        if(message.Msg==LeftDown || message.Msg==LeftDoubleClick) {
            var node=PriorityNodeAt(point);if(node==null)return false;
            Focus();pressedPriorityNode=message.Msg==LeftDown?node:null;swallowPriorityUp=true;Capture=true;
            return true;
        }
        if(message.Msg==MouseMove && swallowPriorityUp)return true;
        if(message.Msg==LeftUp && swallowPriorityUp) {
            var node=pressedPriorityNode;ResetPriorityLinkPress();Capture=false;
            if(node!=null && PriorityNodeAt(point)==node && PriorityClicked!=null)PriorityClicked(node);
            return true;
        }
        return false;
    }
}

internal sealed partial class FloatingWindow {
    void TestPriorityLinks() {
        var originalNodes=tasks.Nodes.Cast<TreeNode>().ToArray();var originalCallback=tasks.PriorityClicked;var originalDoubleClick=tasks.NodeDoubleClicked;
        bool originalMode=simpleMode,originalBusy=busy,originalRendering=rendering,originalTesting=simpleDetailsTesting;
        var originalSize=Size;var originalSelected=tasks.SelectedNode;
        simpleDetailsTesting=true;InvalidateSimpleOutstanding();rendering=true;SetBusy(false);
        int clicks=0,checks=0,drags=0,doubleClicks=0,taskDoubleClicks=0,expands=0;TreeNode clicked=null;
        TreeViewCancelEventHandler check=delegate{checks++;};ItemDragEventHandler drag=delegate{drags++;};
        TreeNodeMouseClickEventHandler doubleClick=delegate{doubleClicks++;};TreeViewEventHandler expand=delegate{expands++;};
        try {
            SetSimpleMode(false);Size=new Size(470,700);tasks.Nodes.Clear();
            for(int priority=0;priority<=9;priority++)tasks.Nodes.Add(new TreeNode((priority+1)+". [P"+priority+"] 优先级验收事项"){Tag=910000L+priority});
            var nested=new TreeNode("1.1. [P4] 子任务正文 [P2] 不是链接"){Tag=910010L};tasks.Nodes[0].Nodes.Add(nested);
            var leaf=new TreeNode("1. [P9] 遗留正文不是优先级"){Tag=new OutstandingLeaf{TaskId=910000L,Id="plain",Html="[P9]"}};tasks.Nodes[0].Nodes.Add(leaf);
            var unnumbered=new TreeNode("正文中的 [P5] 不是行首优先级"){Tag=910011L};tasks.Nodes.Add(unnumbered);tasks.ExpandAll();
            tasks.PriorityClicked=delegate(TreeNode node){clicked=node;clicks++;};
            tasks.NodeDoubleClicked=delegate(TreeNode node){taskDoubleClicks++;clicked=node;};
            tasks.BeforeCheck+=check;tasks.ItemDrag+=drag;tasks.NodeMouseDoubleClick+=doubleClick;tasks.AfterExpand+=expand;tasks.AfterCollapse+=expand;
            foreach(bool simple in new[]{false,true}) {
                SetSimpleMode(simple);if(simple)Size=new Size(360,570);tasks.SetSimpleImageLinks(true);
                if(tasks.DrawMode!=TreeViewDrawMode.OwnerDrawText)throw new Exception("Priority links need owner drawing in both window modes");
                foreach(var node in tasks.Nodes.Cast<TreeNode>().Where(item=>item!=unnumbered).Concat(new[]{nested})) {
                    node.EnsureVisible();tasks.Refresh();var bounds=tasks.PriorityLinkBounds(node);
                    if(bounds.IsEmpty || !tasks.ClientRectangle.Contains(new Point(bounds.Left+bounds.Width/2,bounds.Top+bounds.Height/2)))throw new Exception("Priority marker is not visible/hit-testable");
                    int before=clicks,beforeExpand=expands,beforeTaskDouble=taskDoubleClicks;clicked=null;
                    int coordinates=((bounds.Top+bounds.Height/2)<<16)|((bounds.Left+bounds.Width/2)&0xffff);
                    SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(coordinates));
                    SendSimpleMessage(tasks.Handle,0x200,new IntPtr(1),new IntPtr(coordinates+2));
                    SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(coordinates));
                    SendSimpleMessage(tasks.Handle,0x203,new IntPtr(1),new IntPtr(coordinates));
                    SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(coordinates));
                    if(clicks!=before+1 || clicked!=node || checks!=0 || drags!=0 || doubleClicks!=0 || taskDoubleClicks!=beforeTaskDouble || expands!=beforeExpand)throw new Exception("Priority click/double-click escaped into another tree action");
                    var titlePoint=new Point(bounds.Right+10,bounds.Top+bounds.Height/2);
                    var titleHit=tasks.HitTest(titlePoint);
                    var down=Message.Create(tasks.Handle,0x201,new IntPtr(1),new IntPtr((titlePoint.Y<<16)|(titlePoint.X&0xffff)));
                    if(titleHit.Node!=node || (titleHit.Location&TreeViewHitTestLocations.Label)==0 || tasks.HandleSimpleImageMessage(ref down))throw new Exception("Task title lost its native selection/drag target");
                }
                if(!tasks.PriorityLinkBounds(leaf).IsEmpty || !tasks.PriorityLinkBounds(unnumbered).IsEmpty)throw new Exception("A marker in ordinary title text was made clickable");
                var owner=tasks.Nodes[0];owner.Expand();owner.EnsureVisible();tasks.Refresh();
                var ownerPriority=tasks.PriorityLinkBounds(owner);var ownerTitle=new Point(ownerPriority.Right+12,owner.Bounds.Top+owner.Bounds.Height/2);
                int ownerCoordinates=(ownerTitle.Y<<16)|(ownerTitle.X&0xffff),ownerBeforeDouble=taskDoubleClicks;
                SendSimpleMessage(tasks.Handle,0x203,new IntPtr(1),new IntPtr(ownerCoordinates));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(ownerCoordinates));
                if(!owner.IsExpanded || taskDoubleClicks!=ownerBeforeDouble+1 || clicked!=owner)throw new Exception("Double-click expanded task changed its expansion state");
                owner.Collapse();tasks.Refresh();ownerBeforeDouble=taskDoubleClicks;
                SendSimpleMessage(tasks.Handle,0x203,new IntPtr(1),new IntPtr(ownerCoordinates));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(ownerCoordinates));
                if(owner.IsExpanded || taskDoubleClicks!=ownerBeforeDouble+1 || clicked!=owner)throw new Exception("Double-click collapsed task changed its expansion state");
                owner.Expand();
                nested.EnsureVisible();tasks.Refresh();var nestedBounds=tasks.PriorityLinkBounds(nested);
                int titleMarker=nested.Text.LastIndexOf("[P2]",StringComparison.Ordinal);
                int bodyX=nested.Bounds.Left+2+TextRenderer.MeasureText(nested.Text.Substring(0,titleMarker)+"x",tasks.Font,Size.Empty,TextFormatFlags.NoPadding).Width-TextRenderer.MeasureText("x",tasks.Font,Size.Empty,TextFormatFlags.NoPadding).Width+2;
                var bodyMessage=Message.Create(tasks.Handle,0x201,new IntPtr(1),new IntPtr(((nestedBounds.Top+nestedBounds.Height/2)<<16)|(bodyX&0xffff)));
                if(tasks.HandleSimpleImageMessage(ref bodyMessage))throw new Exception("A priority marker inside the task title triggered the link");
            }
            // Native horizontal scrolling changes Node.Bounds; the link must follow those live coordinates.
            var longNode=tasks.Nodes[0];longNode.Text="1. [P0] "+new string('长',120);longNode.EnsureVisible();Size=new Size(230,420);tasks.Refresh();
            var beforeScroll=tasks.PriorityLinkBounds(longNode);
            SendSimpleMessage(tasks.Handle,0x114,new IntPtr(1),IntPtr.Zero);SendSimpleMessage(tasks.Handle,0x114,new IntPtr(1),IntPtr.Zero);tasks.Refresh();
            var afterScroll=tasks.PriorityLinkBounds(longNode);
            if(afterScroll.Left>=beforeScroll.Left || afterScroll.Right<=0)throw new Exception("Priority link did not follow horizontal scrolling: before="+beforeScroll+", after="+afterScroll+", client="+tasks.ClientSize+", node="+longNode.Bounds);
            int visibleX=Math.Max(1,afterScroll.Left)+Math.Min(afterScroll.Width,afterScroll.Right-Math.Max(1,afterScroll.Left))/2;
            int scrollPoint=((afterScroll.Top+afterScroll.Height/2)<<16)|(visibleX&0xffff);int previousClicks=clicks;
            SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(scrollPoint));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(scrollPoint));
            if(clicks!=previousClicks+1 || clicked!=longNode)throw new Exception("Scrolled priority marker selected the wrong task");
            int cancelBefore=clicks;
            SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(scrollPoint));
            SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(((afterScroll.Top+afterScroll.Height/2)<<16)|((afterScroll.Right+12)&0xffff)));
            if(clicks!=cancelBefore)throw new Exception("Releasing outside a priority link should cancel it");
            SendSimpleMessage(tasks.Handle,0x114,new IntPtr(6),IntPtr.Zero);longNode.Text="1. [P0] 点击优先级标记即可修改";Size=new Size(390,570);longNode.EnsureVisible();tasks.SelectedNode=null;tasks.Refresh();
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-priority-links-test.png"));}
            File.WriteAllText(Path.Combine(data,"floating-priority-links-test.txt"),"PASS: priorities 0-9 and nested tasks in full/simple mode; title markers and outstanding leaves excluded; task double-click invokes its action without changing expanded/collapsed state; priority single/double-click isolated from checks, drag and expansion; native title hit target retained; narrow horizontal-scroll targeting; cancelled outside release.");
        } finally {
            tasks.BeforeCheck-=check;tasks.ItemDrag-=drag;tasks.NodeMouseDoubleClick-=doubleClick;tasks.AfterExpand-=expand;tasks.AfterCollapse-=expand;
            InvalidateSimpleOutstanding();tasks.PriorityClicked=originalCallback;tasks.NodeDoubleClicked=originalDoubleClick;SetSimpleMode(false);tasks.Nodes.Clear();tasks.Nodes.AddRange(originalNodes);
            if(originalMode)SetSimpleMode(true);tasks.SetSimpleImageLinks(true);Size=originalSize;
            if(originalSelected!=null && originalSelected.TreeView==tasks)tasks.SelectedNode=originalSelected;
            rendering=originalRendering;simpleDetailsTesting=originalTesting;SetBusy(originalBusy);
        }
    }
}
