// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Text.RegularExpressions;
using System.Windows.Forms;

internal sealed class TaskTreeSurface : Panel {
    sealed class Row {
        internal TreeNode Node;
        internal Rectangle Bounds,Expand,Check,Prefix,Image,Reminder,Priority,Title,Ancestors;
        internal string PrefixText="",PriorityText="",TitleText="",AncestorText="";
        internal Font Font;
        internal int AnchorY;
        internal bool HasChildren,Done,Wrap,ImageAvailable,ReminderAvailable;
        internal string SourceText="",CurrentText="";
        internal int Level;
        internal Color SourceColor;
    }
    static readonly Regex Parts=new Regex(@"^(?<prefix>[0-9]+(?:\.[0-9]+)*\.\s+)(?<priority>\[P[0-9]\])(?:\s+)?(?<title>.*)$");
    readonly List<Row> rows=new List<Row>();
    TaskTreeView model;
    bool rebuildQueued;
    TreeNode hoverNode,pressedNode,dragNode,dropNode;
    Point pressedAt;
    int dropZone;
    bool dragging;
    bool correctingScroll;
    int verticalWheelRemainder,horizontalWheelRemainder;
    Size layoutClientSize;
    int layoutIndent,layoutItemHeight;
    bool layoutFlat,layoutWrap;
    TreeNode layoutSelection;
    internal long LayoutBuildCount,LayoutReuseCount;

    internal Action<TreeNode> CompletionClicked,PriorityClicked,ImageClicked,ReminderClicked,NodeDoubleClicked;
    internal Action<TreeNode,Point> HoverChanged;
    internal Func<TreeNode,bool> BeginItemDrag;
    internal Action EndItemDrag;
    internal Func<TreeNode,TreeNode,int,bool> CanDrop;
    internal Action<TreeNode,TreeNode,int> DropRequested;
    internal Func<bool> BlankDragEnabled;
    internal Action<Point> BlankDragRequested;

    internal TaskTreeSurface() {
        AutoScroll=true;DoubleBuffered=true;TabStop=true;BackColor=Color.White;BorderStyle=BorderStyle.FixedSingle;AllowDrop=true;
        SetStyle(ControlStyles.AllPaintingInWmPaint|ControlStyles.OptimizedDoubleBuffer|ControlStyles.ResizeRedraw|ControlStyles.UserPaint,true);
    }
    internal void Bind(TaskTreeView value) {
        model=value;if(model==null)return;
        Font=model.Font;ForeColor=model.ForeColor;BackColor=model.BackColor;
        model.ViewChanged=QueueRebuild;
        model.AfterSelect+=delegate {Invalidate();};
        model.AfterExpand+=delegate {QueueRebuild();};
        model.AfterCollapse+=delegate {QueueRebuild();};
        Rebuild(false);
    }
    internal void QueueRebuild() {
        if(rebuildQueued || IsDisposed)return;rebuildQueued=true;
        if(!IsHandleCreated){rebuildQueued=false;return;}
        BeginInvoke(new Action(delegate {rebuildQueued=false;if(!IsDisposed)Rebuild(true);}));
    }
    protected override void OnResize(EventArgs e) {base.OnResize(e);QueueRebuild();}
    protected override void OnFontChanged(EventArgs e) {base.OnFontChanged(e);QueueRebuild();}
    int VisibleHorizontalMaximum(int verticalOffset) {
        int viewportBottom=verticalOffset+Math.Max(1,ClientSize.Height),right=ClientSize.Width;
        foreach(var row in rows)if(row.Bounds.Bottom>verticalOffset && row.Bounds.Top<viewportBottom)right=Math.Max(right,row.Bounds.Right);
        return Math.Max(0,right-ClientSize.Width);
    }
    int RowHorizontalOffset(Row row) {
        int requested=-AutoScrollPosition.X,maximum=Math.Max(0,row.Bounds.Right-ClientSize.Width);
        return -Math.Min(requested,maximum);
    }
    Point RowPaintOffset(Row row) {return new Point(RowHorizontalOffset(row),AutoScrollPosition.Y);}
    void ClampHorizontalScrollToVisibleRows() {
        if(correctingScroll || rows.Count==0)return;
        int x=-AutoScrollPosition.X,y=-AutoScrollPosition.Y,maxX=VisibleHorizontalMaximum(y);
        if(x<=maxX)return;
        correctingScroll=true;
        try {AutoScrollPosition=new Point(maxX,y);} finally {correctingScroll=false;}
    }
    void RepaintViewport() {Invalidate(ClientRectangle);Update();}
    protected override void OnScroll(ScrollEventArgs e) {
        base.OnScroll(e);ClampHorizontalScrollToVisibleRows();Invalidate(ClientRectangle);
    }
    protected override void WndProc(ref Message message) {
        const int HorizontalScroll=0x114,VerticalScroll=0x115,MouseWheel=0x20A,MouseHorizontalWheel=0x20E;
        bool scrolled=message.Msg==HorizontalScroll||message.Msg==VerticalScroll||message.Msg==MouseWheel||message.Msg==MouseHorizontalWheel;
        base.WndProc(ref message);
        if(scrolled&&!IsDisposed){ClampHorizontalScrollToVisibleRows();RepaintViewport();}
    }
    void SetScrollOffset(int x,int y) {
        int maxX=Math.Max(0,AutoScrollMinSize.Width-ClientSize.Width),maxY=Math.Max(0,AutoScrollMinSize.Height-ClientSize.Height);
        y=Math.Max(0,Math.Min(maxY,y));maxX=Math.Min(maxX,VisibleHorizontalMaximum(y));
        x=Math.Max(0,Math.Min(maxX,x));
        if(x==-AutoScrollPosition.X && y==-AutoScrollPosition.Y)return;
        AutoScrollPosition=new Point(x,y);RepaintViewport();
    }
    void ScrollByWheel(int delta,bool horizontal) {
        int remainder=(horizontal?horizontalWheelRemainder:verticalWheelRemainder)+delta;
        int notches=remainder/SystemInformation.MouseWheelScrollDelta;if(notches==0){if(horizontal)horizontalWheelRemainder=remainder;else verticalWheelRemainder=remainder;return;}
        remainder-=notches*SystemInformation.MouseWheelScrollDelta;if(horizontal)horizontalWheelRemainder=remainder;else verticalWheelRemainder=remainder;
        int lines=SystemInformation.MouseWheelScrollLines;int units=lines<0?3:Math.Max(1,lines);int step=Math.Max(18,model==null?28:model.ItemHeight)*units;
        int x=-AutoScrollPosition.X,y=-AutoScrollPosition.Y;if(horizontal)x-=notches*step;else y-=notches*step;SetScrollOffset(x,y);
    }
    protected override void OnMouseWheel(MouseEventArgs e) {ScrollByWheel(e.Delta,(ModifierKeys&Keys.Shift)==Keys.Shift||(!VScroll&&HScroll));}
    static bool Real(TreeNode node) {return node!=null && (node.Tag is long || node.Tag is FloatingWindow.OutstandingLeaf);}
    static IEnumerable<TreeNode> VisibleNodes(TreeNodeCollection nodes) {
        foreach(TreeNode node in nodes)if(Real(node)) {
            yield return node;
            if(node.IsExpanded)foreach(var child in VisibleNodes(node.Nodes))yield return child;
        }
    }
    bool HasImage(TreeNode node) {return model!=null && model.ImageAvailableFor(node);}
    static bool HasReminder(TreeNode node) {return FloatingWindow.NodeHasReminder(node);}
    static int Advance(string text,Font font) {return TextRenderer.MeasureText((text??"")+"x",font,Size.Empty,TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix).Width-TextRenderer.MeasureText("x",font,Size.Empty,TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix).Width;}
    int BaseHeight(Font font) {return Math.Max(model==null?28:model.ItemHeight,TextRenderer.MeasureText("Ag中",font,Size.Empty,TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix).Height+8);}
    bool ReuseLayout(List<TreeNode> visible,bool preserveScroll) {
        if(layoutClientSize!=ClientSize || layoutIndent!=model.Indent || layoutItemHeight!=model.ItemHeight ||
            layoutFlat!=model.SingleLinePaths || layoutWrap!=model.WrapNodeText || rows.Count!=visible.Count)return false;
        bool repaint=layoutSelection!=model.SelectedNode;
        for(int index=0;index<visible.Count;index++) {
            var node=visible[index];var row=rows[index];var leaf=node.Tag as FloatingWindow.OutstandingLeaf;
            if(row.Node!=node || row.SourceText!=node.Text || row.Level!=node.Level || row.CurrentText!=model.CurrentTaskText(node) ||
                !row.Font.Equals(model.DisplayFont(node)) || row.HasChildren!=node.Nodes.Cast<TreeNode>().Any(Real) ||
                row.Done!=(leaf==null?node.Checked:leaf.Done) || row.ImageAvailable!=HasImage(node) || row.ReminderAvailable!=HasReminder(node))return false;
            if(row.SourceColor!=node.ForeColor){row.SourceColor=node.ForeColor;repaint=true;}
        }
        if(!preserveScroll && AutoScrollPosition!=Point.Empty){AutoScrollPosition=Point.Empty;repaint=true;}
        layoutSelection=model.SelectedNode;LayoutReuseCount++;
        if(repaint)Invalidate();
        return true;
    }
    internal void Rebuild(bool preserveScroll) {
        if(model==null || IsDisposed)return;
        var visible=VisibleNodes(model.Nodes).ToList();
        // Selection, focus and unchanged background reads invalidate the native model too.
        // Reuse the measured rows unless content, geometry or display options actually changed.
        if(ReuseLayout(visible,preserveScroll))return;
        LayoutBuildCount++;
        Point scroll=preserveScroll?new Point(-AutoScrollPosition.X,-AutoScrollPosition.Y):Point.Empty;
        Size measuredClientSize=ClientSize;
        rows.Clear();int y=2,maxRight=Math.Max(0,measuredClientSize.Width-2);bool flat=model.SingleLinePaths;
        int viewport=Math.Max(80,measuredClientSize.Width-(flat?4:SystemInformation.VerticalScrollBarWidth+6));
        foreach(var node in visible) {
            Font font=model.DisplayFont(node);string current=model.CurrentTaskText(node),ancestors=model.AncestorTaskText(node);var match=Parts.Match(current);
            string prefix=match.Success?match.Groups["prefix"].Value:"";string priority=match.Success?match.Groups["priority"].Value:"";string title=match.Success?match.Groups["title"].Value:current;
            int level=flat?0:node.Level;int left=4+level*Math.Max(18,model.Indent);int expandLeft=left;int checkLeft=left+14;int textLeft=checkLeft+20;
            bool image=HasImage(node),reminder=HasReminder(node);int prefixWidth=Advance(prefix,font);int imageWidth=image?Advance("图片",font)+8:0;int reminderWidth=reminder?Advance("提醒",font)+8:0;int priorityWidth=Advance(priority,font);int headerWidth=prefixWidth+(image?6+imageWidth:0)+(reminder?6+reminderWidth:0)+(priority.Length>0?6+priorityWidth:0)+(title.Length>0?6:0);
            int inlineTitleLeft=textLeft+headerWidth;int baseHeight=BaseHeight(font);int titleWidth;
            bool wrap=!flat && model.WrapNodeText;int fullTitleWidth=Math.Max(40,viewport-textLeft-6);int inlineTitleWidth=Math.Max(40,viewport-inlineTitleLeft);
            bool stacked=wrap&&title.Length>0&&Advance(title,font)+4>inlineTitleWidth&&inlineTitleWidth<Math.Max(80,fullTitleWidth*2/3);
            int titleLeft=stacked?textLeft:inlineTitleLeft;
            if(wrap)titleWidth=stacked?fullTitleWidth:inlineTitleWidth;
            else titleWidth=Math.Max(1,Advance(title,font)+4);
            int textHeight=TextRenderer.MeasureText(title.Length==0?" ":title,font,new Size(titleWidth,Int32.MaxValue),TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix|(wrap?TextFormatFlags.WordBreak|TextFormatFlags.TextBoxControl:TextFormatFlags.SingleLine)).Height;
            int height=wrap?Math.Max(baseHeight,(stacked?baseHeight:0)+textHeight+(stacked?2:8)):baseHeight;
            var row=new Row{Node=node,SourceText=node.Text,CurrentText=current,Level=node.Level,SourceColor=node.ForeColor,ImageAvailable=image,ReminderAvailable=reminder,Font=font,PrefixText=prefix,PriorityText=priority,TitleText=title,AncestorText=ancestors,HasChildren=node.Nodes.Cast<TreeNode>().Any(Real),Done=node.Tag is FloatingWindow.OutstandingLeaf?((FloatingWindow.OutstandingLeaf)node.Tag).Done:node.Checked,Wrap=wrap};
            row.Bounds=new Rectangle(0,y,Math.Max(viewport,titleLeft+titleWidth+6),height);row.AnchorY=y+baseHeight/2;
            row.Expand=new Rectangle(expandLeft,y+(baseHeight-12)/2,12,12);row.Check=new Rectangle(checkLeft,y+(baseHeight-16)/2,16,16);
            int lineHeight=TextRenderer.MeasureText("Ag中",font,Size.Empty,TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix).Height+2;int textTop=y+4;int headerHeight=Math.Max(1,Math.Min(height-8,lineHeight));
            int cursor=textLeft;row.Prefix=new Rectangle(cursor,textTop,prefixWidth,headerHeight);cursor+=prefixWidth;
            if(image){cursor+=6;row.Image=new Rectangle(cursor,textTop,imageWidth,headerHeight);cursor+=imageWidth;}
            if(reminder){cursor+=6;row.Reminder=new Rectangle(cursor,textTop,reminderWidth,headerHeight);cursor+=reminderWidth;}
            if(priority.Length>0){cursor+=6;row.Priority=new Rectangle(cursor,textTop,priorityWidth,headerHeight);cursor+=priorityWidth;}
            if(title.Length>0&&!stacked)cursor+=6;
            int titleTop=stacked?y+baseHeight-2:textTop;
            row.Title=new Rectangle(stacked?titleLeft:cursor,titleTop,titleWidth,Math.Max(1,height-(titleTop-y)-(stacked?0:4)));
            int titlePixels=Advance(title,font)+4;int ancestorWidth=Advance(ancestors,font)+4;
            if(flat && ancestors.Length>0)row.Ancestors=new Rectangle(row.Title.Left+titlePixels,textTop,ancestorWidth,headerHeight);
            int right=flat?(row.Ancestors.IsEmpty?row.Title.Left+titlePixels:row.Ancestors.Right)+8:viewport;
            row.Bounds.Width=Math.Max(viewport,right);maxRight=Math.Max(maxRight,right);rows.Add(row);y+=height;
        }
        AutoScrollMinSize=new Size(flat?maxRight:0,y+2);
        AutoScrollPosition=scroll;
        layoutClientSize=measuredClientSize;layoutIndent=model.Indent;layoutItemHeight=model.ItemHeight;layoutFlat=model.SingleLinePaths;layoutWrap=model.WrapNodeText;layoutSelection=model.SelectedNode;
        Invalidate();
    }
    static Color NodeColor(TreeNode node,Color fallback) {return node.ForeColor.IsEmpty?fallback:node.ForeColor;}
    static Rectangle OffsetRectangle(Rectangle value,Point offset) {value.Offset(offset);return value;}
    protected override void OnPaint(PaintEventArgs e) {
        base.OnPaint(e);if(model==null)return;
        using(var linePen=new Pen(Color.FromArgb(185,192,201)))foreach(var row in rows) {
            var offset=RowPaintOffset(row);
            var bounds=OffsetRectangle(row.Bounds,offset);
            if(bounds.Bottom<0 || bounds.Top>ClientSize.Height)continue;
            var expand=OffsetRectangle(row.Expand,offset);var check=OffsetRectangle(row.Check,offset);var prefix=OffsetRectangle(row.Prefix,offset);
            var image=OffsetRectangle(row.Image,offset);var reminder=OffsetRectangle(row.Reminder,offset);var priority=OffsetRectangle(row.Priority,offset);var title=OffsetRectangle(row.Title,offset);var ancestors=OffsetRectangle(row.Ancestors,offset);
            bool selected=model.SelectedNode==row.Node;Color background=selected?SystemColors.Highlight:BackColor;Color foreground=selected?SystemColors.HighlightText:NodeColor(row.Node,ForeColor);
            using(var backgroundBrush=new SolidBrush(background))e.Graphics.FillRectangle(backgroundBrush,new Rectangle(0,bounds.Top,ClientSize.Width,bounds.Height));
            if(!model.SingleLinePaths) {
                for(int level=0;level<row.Node.Level;level++){int x=offset.X+10+level*Math.Max(18,model.Indent);e.Graphics.DrawLine(linePen,x,bounds.Top,x,bounds.Bottom);}
                if(row.Node.Level>0){int x=offset.X+10+(row.Node.Level-1)*Math.Max(18,model.Indent);e.Graphics.DrawLine(linePen,x,offset.Y+row.AnchorY,expand.Left,offset.Y+row.AnchorY);}
            }
            if(row.HasChildren && !model.SingleLinePaths){e.Graphics.FillRectangle(Brushes.White,expand);e.Graphics.DrawRectangle(Pens.Gray,expand);e.Graphics.DrawLine(Pens.DimGray,expand.Left+3,expand.Top+6,expand.Right-3,expand.Top+6);if(!row.Node.IsExpanded)e.Graphics.DrawLine(Pens.DimGray,expand.Left+6,expand.Top+3,expand.Left+6,expand.Bottom-3);}
            CheckBoxRenderer.DrawCheckBox(e.Graphics,new Point(check.Left,check.Top),row.Done?System.Windows.Forms.VisualStyles.CheckBoxState.CheckedNormal:System.Windows.Forms.VisualStyles.CheckBoxState.UncheckedNormal);
            var single=TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix|TextFormatFlags.VerticalCenter|TextFormatFlags.SingleLine|TextFormatFlags.EndEllipsis;
            if(row.PrefixText.Length>0)TextRenderer.DrawText(e.Graphics,row.PrefixText,row.Font,prefix,foreground,single);
            if(!row.Image.IsEmpty)using(var underline=new Font(row.Font,row.Font.Style|FontStyle.Underline))TextRenderer.DrawText(e.Graphics,"图片",underline,image,selected?SystemColors.HighlightText:Color.FromArgb(36,94,210),single);
            if(!row.Reminder.IsEmpty)using(var underline=new Font(row.Font,row.Font.Style|FontStyle.Underline))TextRenderer.DrawText(e.Graphics,"提醒",underline,reminder,selected?SystemColors.HighlightText:Color.FromArgb(196,92,28),single);
            if(!row.Priority.IsEmpty)using(var underline=new Font(row.Font,row.Font.Style|FontStyle.Underline))TextRenderer.DrawText(e.Graphics,row.PriorityText,underline,priority,selected?SystemColors.HighlightText:Color.FromArgb(36,94,210),single);
            var titleFlags=TextFormatFlags.NoPadding|TextFormatFlags.NoPrefix|TextFormatFlags.EndEllipsis|(row.Wrap?TextFormatFlags.WordBreak|TextFormatFlags.TextBoxControl:TextFormatFlags.VerticalCenter|TextFormatFlags.SingleLine);
            TextRenderer.DrawText(e.Graphics,row.TitleText,row.Font,title,foreground,titleFlags);
            if(!row.Ancestors.IsEmpty)TextRenderer.DrawText(e.Graphics,row.AncestorText,row.Font,ancestors,selected?Color.FromArgb(215,225,236):Color.FromArgb(156,163,175),single);
            if(selected && Focused)ControlPaint.DrawFocusRectangle(e.Graphics,bounds,foreground,background);
            if(row.Node==dropNode){int y=dropZone<0?bounds.Top:dropZone>0?bounds.Bottom-2:bounds.Top+bounds.Height/2;using(var pen=new Pen(Color.FromArgb(36,94,210),2))e.Graphics.DrawLine(pen,Math.Max(2,bounds.Left+2),y,Math.Max(2,bounds.Right-4),y);}
        }
    }
    Point ContentPoint(Row row,Point client) {return new Point(client.X-RowHorizontalOffset(row),client.Y-AutoScrollPosition.Y);}
    Row RowAt(Point client) {int y=client.Y-AutoScrollPosition.Y;return rows.FirstOrDefault(row=>y>=row.Bounds.Top&&y<row.Bounds.Bottom);}
    enum Part {None,Expand,Check,Priority,Image,Reminder,Title}
    static Part HitPart(Row row,Point point) {if(row==null)return Part.None;if(row.Expand.Contains(point)&&row.HasChildren)return Part.Expand;if(row.Check.Contains(point))return Part.Check;if(row.Priority.Contains(point))return Part.Priority;if(row.Image.Contains(point))return Part.Image;if(row.Reminder.Contains(point))return Part.Reminder;return row.Bounds.Contains(point)?Part.Title:Part.None;}
    protected override void OnMouseDown(MouseEventArgs e) {
        base.OnMouseDown(e);Focus();var row=RowAt(e.Location);Point point=row==null?Point.Empty:ContentPoint(row,e.Location);
        if(row==null){if(e.Button==MouseButtons.Left&&BlankDragEnabled!=null&&BlankDragEnabled()&&BlankDragRequested!=null)BlankDragRequested(e.Location);return;}
        model.SelectedNode=row.Node;pressedNode=row.Node;pressedAt=e.Location;Part part=HitPart(row,point);
        if(e.Button!=MouseButtons.Left)return;
        if(part==Part.Expand){if(row.Node.IsExpanded)row.Node.Collapse();else row.Node.Expand();Rebuild(true);pressedNode=null;}
        else if(part==Part.Check){if(CompletionClicked!=null)CompletionClicked(row.Node);pressedNode=null;}
        else if(part==Part.Priority){if(PriorityClicked!=null)PriorityClicked(row.Node);pressedNode=null;}
        else if(part==Part.Image){if(ImageClicked!=null)ImageClicked(row.Node);pressedNode=null;}
        else if(part==Part.Reminder){if(ReminderClicked!=null)ReminderClicked(row.Node);pressedNode=null;}
        else dragNode=row.Node;
        Invalidate();
    }
    protected override void OnMouseUp(MouseEventArgs e) {base.OnMouseUp(e);pressedNode=null;dragNode=null;dragging=false;}
    protected override void OnMouseDoubleClick(MouseEventArgs e) {base.OnMouseDoubleClick(e);var row=RowAt(e.Location);if(e.Button==MouseButtons.Left&&row!=null&&HitPart(row,ContentPoint(row,e.Location))==Part.Title&&NodeDoubleClicked!=null)NodeDoubleClicked(row.Node);}
    protected override void OnMouseMove(MouseEventArgs e) {
        base.OnMouseMove(e);var row=RowAt(e.Location);TreeNode node=row==null?null:row.Node;
        if(node!=hoverNode){hoverNode=node;if(HoverChanged!=null)HoverChanged(node,e.Location);}
        if(!dragging&&e.Button==MouseButtons.Left&&dragNode!=null&&(Math.Abs(e.X-pressedAt.X)>=SystemInformation.DragSize.Width/2||Math.Abs(e.Y-pressedAt.Y)>=SystemInformation.DragSize.Height/2)){
            if(BeginItemDrag==null||BeginItemDrag(dragNode)){dragging=true;try{DoDragDrop(dragNode,DragDropEffects.Move);}finally{dragging=false;dragNode=null;if(EndItemDrag!=null)EndItemDrag();}}
        }
    }
    protected override void OnMouseLeave(EventArgs e) {base.OnMouseLeave(e);hoverNode=null;if(HoverChanged!=null)HoverChanged(null,Point.Empty);}
    protected override bool IsInputKey(Keys keyData) {if((keyData&Keys.KeyCode)==Keys.Up||(keyData&Keys.KeyCode)==Keys.Down||(keyData&Keys.KeyCode)==Keys.Left||(keyData&Keys.KeyCode)==Keys.Right)return true;return base.IsInputKey(keyData);}
    protected override void OnKeyDown(KeyEventArgs e) {
        var current=rows.FindIndex(row=>row.Node==model.SelectedNode);
        if(e.KeyCode==Keys.Up||e.KeyCode==Keys.Down){int next=Math.Max(0,Math.Min(rows.Count-1,current+(e.KeyCode==Keys.Up?-1:1)));if(rows.Count>0){model.SelectedNode=rows[next].Node;EnsureVisible(rows[next]);}e.Handled=true;}
        else if(e.KeyCode==Keys.Space&&model.SelectedNode!=null){if(CompletionClicked!=null)CompletionClicked(model.SelectedNode);e.Handled=true;}
        else if(e.KeyCode==Keys.Enter&&model.SelectedNode!=null){if(NodeDoubleClicked!=null)NodeDoubleClicked(model.SelectedNode);e.Handled=true;}
        else if(e.KeyCode==Keys.Left&&model.SelectedNode!=null){if(model.SelectedNode.IsExpanded)model.SelectedNode.Collapse();else if(model.SelectedNode.Parent!=null)model.SelectedNode=model.SelectedNode.Parent;Rebuild(true);e.Handled=true;}
        else if(e.KeyCode==Keys.Right&&model.SelectedNode!=null){if(model.SelectedNode.Nodes.Cast<TreeNode>().Any(Real))model.SelectedNode.Expand();Rebuild(true);e.Handled=true;}
        base.OnKeyDown(e);
    }
    void EnsureVisible(Row row) {int top=-AutoScrollPosition.Y,bottom=top+ClientSize.Height;if(row.Bounds.Top<top)AutoScrollPosition=new Point(-AutoScrollPosition.X,row.Bounds.Top);else if(row.Bounds.Bottom>bottom)AutoScrollPosition=new Point(-AutoScrollPosition.X,row.Bounds.Bottom-ClientSize.Height);Invalidate();}
    protected override void OnDragEnter(DragEventArgs e) {base.OnDragEnter(e);e.Effect=e.Data.GetDataPresent(typeof(TreeNode))?DragDropEffects.Move:DragDropEffects.None;}
    protected override void OnDragOver(DragEventArgs e) {
        base.OnDragOver(e);var source=e.Data.GetData(typeof(TreeNode)) as TreeNode;var row=RowAt(PointToClient(new Point(e.X,e.Y)));var target=row==null?null:row.Node;int zone=0;
        if(row!=null){int y=ContentPoint(row,PointToClient(new Point(e.X,e.Y))).Y;int edge=Math.Max(7,(int)Math.Round(row.Bounds.Height*.3));zone=y<row.Bounds.Top+edge?-1:y>=row.Bounds.Bottom-edge?1:0;}
        bool allowed=source!=null&&target!=null&&CanDrop!=null&&CanDrop(source,target,zone);e.Effect=allowed?DragDropEffects.Move:DragDropEffects.None;dropNode=allowed?target:null;dropZone=zone;Invalidate();
    }
    protected override void OnDragLeave(EventArgs e) {base.OnDragLeave(e);ClearDropMark();}
    protected override void OnDragDrop(DragEventArgs e) {base.OnDragDrop(e);var source=e.Data.GetData(typeof(TreeNode)) as TreeNode;var target=dropNode;int zone=dropZone;ClearDropMark();if(source!=null&&target!=null&&DropRequested!=null)DropRequested(source,target,zone);}
    internal void ClearDropMark(){dropNode=null;dropZone=0;Invalidate();}
    Rectangle ClientBounds(Row row,Rectangle value){var offset=RowPaintOffset(row);return new Rectangle(value.X+offset.X,value.Y+offset.Y,value.Width,value.Height);}
    internal Rectangle NodeBounds(TreeNode node){var row=rows.FirstOrDefault(item=>item.Node==node);return row==null?Rectangle.Empty:ClientBounds(row,row.Bounds);}
    internal Rectangle CheckBounds(TreeNode node){var row=rows.FirstOrDefault(item=>item.Node==node);return row==null?Rectangle.Empty:ClientBounds(row,row.Check);}
    internal Rectangle PrefixBounds(TreeNode node){var row=rows.FirstOrDefault(item=>item.Node==node);return row==null?Rectangle.Empty:ClientBounds(row,row.Prefix);}
    internal Rectangle ImageBounds(TreeNode node){var row=rows.FirstOrDefault(item=>item.Node==node);return row==null?Rectangle.Empty:ClientBounds(row,row.Image);}
    internal Rectangle ReminderBounds(TreeNode node){var row=rows.FirstOrDefault(item=>item.Node==node);return row==null?Rectangle.Empty:ClientBounds(row,row.Reminder);}
    internal Rectangle PriorityBounds(TreeNode node){var row=rows.FirstOrDefault(item=>item.Node==node);return row==null?Rectangle.Empty:ClientBounds(row,row.Priority);}
    internal Rectangle TitleBounds(TreeNode node){var row=rows.FirstOrDefault(item=>item.Node==node);return row==null?Rectangle.Empty:ClientBounds(row,row.Title);}
    internal Size ContentExtent {get{return AutoScrollMinSize;}}
    internal Point ScrollOffset {get{return new Point(-AutoScrollPosition.X,-AutoScrollPosition.Y);}}
    internal void TestWheel(int delta,bool horizontal){ScrollByWheel(delta,horizontal);}
    internal void TestScrollTo(int x,int y){SetScrollOffset(x,y);}
    internal TreeNode NodeAt(Point point){var row=RowAt(point);return row==null?null:row.Node;}
    internal void TestClick(Rectangle bounds,int clicks){var point=new Point(bounds.Left+bounds.Width/2,bounds.Top+bounds.Height/2);OnMouseDown(new MouseEventArgs(MouseButtons.Left,clicks,point.X,point.Y,0));if(clicks>1)OnMouseDoubleClick(new MouseEventArgs(MouseButtons.Left,clicks,point.X,point.Y,0));OnMouseUp(new MouseEventArgs(MouseButtons.Left,clicks,point.X,point.Y,0));}
    internal void TestKey(Keys key){OnKeyDown(new KeyEventArgs(key));}
}

internal sealed partial class TaskTreeView {
    internal Action ViewChanged;
    internal bool ImageAvailableFor(TreeNode node) {return simpleImageLinks&&SimpleImageAvailable!=null&&SimpleImageAvailable(node);}
    protected override void OnInvalidated(InvalidateEventArgs e) {base.OnInvalidated(e);if(ViewChanged!=null)ViewChanged();}
}

internal sealed partial class FloatingWindow {
    readonly TaskTreeSurface taskSurface=new TaskTreeSurface{Dock=DockStyle.Fill,AccessibleName="任务与子任务"};
    Control hoverSurface;
    void InitializeTaskSurface() {
        taskSurface.Bind(tasks);taskSurface.ContextMenuStrip=tasks.ContextMenuStrip;
        taskSurface.MouseDown+=delegate{ShowSimpleModeRestore();};taskSurface.MouseClick+=delegate{ShowSimpleModeRestore();};
        taskSurface.CompletionClicked=delegate(TreeNode node){if(tasks.CompletionClicked!=null)tasks.CompletionClicked(node);};
        taskSurface.PriorityClicked=delegate(TreeNode node){if(tasks.PriorityClicked!=null)tasks.PriorityClicked(node);};
        taskSurface.ImageClicked=delegate(TreeNode node){if(tasks.SimpleImageClicked!=null)tasks.SimpleImageClicked(node);};
        taskSurface.ReminderClicked=async delegate(TreeNode node){await ShowReminderForNode(node,this);};
        taskSurface.NodeDoubleClicked=delegate(TreeNode node){if(tasks.NodeDoubleClicked!=null)tasks.NodeDoubleClicked(node);};
        taskSurface.HoverChanged=delegate(TreeNode node,Point point){hoverTimer.Stop();progressTip.Hide(taskSurface);hoverNode=node;hoverSurface=taskSurface;if(!dragging&&node!=null&&node.Tag is long)hoverTimer.Start();};
        taskSurface.BlankDragEnabled=delegate{return simpleMode&&!closing&&!dragging;};taskSurface.BlankDragRequested=delegate(Point point){StartSimpleWindowDrag(taskSurface,point);};
        taskSurface.BeginItemDrag=delegate(TreeNode node){if(busy||node==null)return false;dragging=true;timer.Stop();hoverTimer.Stop();progressTip.Hide(taskSurface);return true;};
        taskSurface.EndItemDrag=delegate{dragging=false;ClearDropMark();timer.Start();};
        taskSurface.CanDrop=delegate(TreeNode source,TreeNode target,int zone){if(source!=null&&source.Tag is long&&target!=null&&target.Tag is OutstandingLeaf)target=target.Parent;return MakeDropPlan(source,target,zone)!=null;};
        taskSurface.DropRequested=async delegate(TreeNode source,TreeNode target,int zone){if(source!=null&&source.Tag is long&&target!=null&&target.Tag is OutstandingLeaf)target=target.Parent;var plan=MakeDropPlan(source,target,zone);if(plan!=null)await ExecuteDrop(plan);};
    }
    void TestTaskSurfaceLayout() {
        var originalNodes=tasks.Nodes.Cast<TreeNode>().ToArray();var originalSize=Size;var originalSelection=tasks.SelectedNode;
        bool originalMode=simpleMode,originalFlat=tasks.SingleLinePaths,originalWrap=tasks.WrapNodeText,originalRendering=rendering;
        rendering=true;
        try {
            SetSimpleMode(false);Size=new Size(420,560);tasks.Nodes.Clear();
            var shortNode=new TaskNode("1. [P7] 短事项"){Tag=920001L};shortNode.CurrentTextLength=shortNode.Text.Length;
            var longNode=new TaskNode("2. [P4] 这是一条很长的任务文字，用于验证窗口变窄时只增加当前条目的行高，字体字号保持不变，其他短事项仍然保持正常行高，而且完整模式和简洁模式采用完全相同的排版逻辑。"){Tag=920002L};longNode.CurrentTextLength=longNode.Text.Length;
            string imageLeafText="1. [P3] 带图片的遗留事项文字很长，用于验证最长条目宽度计算时包含图片入口本身占用的空间，而且滚动到最右侧时正文不会被图片入口挤掉";
            string plainLeafText="2. [P3] 带图片的遗留事项文字很长，用于验证最长条目宽度计算时包含图片入口本身占用的空间，而且滚动到最右侧时正文不会被图片入口挤掉";
            var imageLeaf=new TreeNode(imageLeafText){Tag=new OutstandingLeaf{TaskId=920001L,Id="surface-image",Html="带图片<img src='/api/v2/tasks/920001/attachments/1'>",ReminderAt=DateTimeOffset.UtcNow.AddHours(1).ToString("o"),Priority=3,CurrentTextLength=imageLeafText.Length}};
            var plainLeaf=new TreeNode(plainLeafText){Tag=new OutstandingLeaf{TaskId=920001L,Id="surface-plain",Html="无图片",Priority=3,CurrentTextLength=plainLeafText.Length}};
            shortNode.Nodes.Add(imageLeaf);shortNode.Nodes.Add(plainLeaf);tasks.Nodes.Add(shortNode);tasks.Nodes.Add(longNode);shortNode.Expand();tasks.SetSimpleImageLinks(true);
            tasks.SingleLinePaths=false;tasks.SetWrappedText(true);taskSurface.Rebuild(false);taskSurface.Update();
            var shortBounds=taskSurface.NodeBounds(shortNode);var longBounds=taskSurface.NodeBounds(longNode);
            if(shortBounds.IsEmpty || longBounds.Height<=shortBounds.Height)throw new Exception("Long text did not increase only its own row height");
            var wrappedImageBounds=taskSurface.NodeBounds(imageLeaf);var wrappedPlainBounds=taskSurface.NodeBounds(plainLeaf);
            if(wrappedImageBounds.Bottom!=wrappedPlainBounds.Top)throw new Exception("Wrapped rows contain an unexpected blank line");
            if(Math.Abs(tasks.DisplayFont(shortNode).SizeInPoints-tasks.DisplayFont(longNode).SizeInPoints)>.01f)throw new Exception("Wrapped text changed the task font size");
            var prefix=taskSurface.PrefixBounds(imageLeaf);var image=taskSurface.ImageBounds(imageLeaf);var reminder=taskSurface.ReminderBounds(imageLeaf);var priority=taskSurface.PriorityBounds(imageLeaf);var title=taskSurface.TitleBounds(imageLeaf);
            if(prefix.IsEmpty || image.IsEmpty || reminder.IsEmpty || priority.IsEmpty || title.IsEmpty || prefix.Right>=image.Left || image.Right>=reminder.Left || reminder.Right>=priority.Left || priority.Right>=title.Left)throw new Exception("Sequence, image, reminder, priority, and title spacing overlap");
            if(priority.Top!=title.Top || reminder.Top!=priority.Top || image.Top!=reminder.Top || prefix.Top!=image.Top)throw new Exception("Reminder or priority text is vertically misaligned");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-variable-rows-full-test.png"));}
            SetSimpleMode(true);Size=new Size(260,430);taskSurface.Rebuild(false);taskSurface.Update();
            if(taskSurface.NodeBounds(longNode).Height<=taskSurface.NodeBounds(shortNode).Height || Math.Abs(tasks.DisplayFont(shortNode).SizeInPoints-tasks.DisplayFont(longNode).SizeInPoints)>.01f)throw new Exception("Simple mode did not retain the shared variable-row renderer and font size");
            var simpleImageTitle=taskSurface.TitleBounds(imageLeaf);var simpleImagePriority=taskSurface.PriorityBounds(imageLeaf);var simpleImagePrefix=taskSurface.PrefixBounds(imageLeaf);
            if(simpleImageTitle.Top<=simpleImagePriority.Top || simpleImageTitle.Left!=simpleImagePrefix.Left)throw new Exception("Narrow wrapped text did not use the full content width below its markers");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-variable-rows-simple-test.png"));}
            for(int index=0;index<24;index++) {
                string text=(index+3)+". [P"+(index%10)+"] "+(index%2==0?"滚动测试短事项":"用于验证简洁模式单行显示横向与纵向滚动可以同时稳定工作的长事项 "+index+" / 上级任务 / 项目总表");
                var node=new TaskNode(text){Tag=921000L+index};node.CurrentTextLength=text.Length;tasks.Nodes.Add(node);
            }
            var shortTailNodes=new List<TreeNode>();
            for(int index=0;index<16;index++) {
                string text=(index+27)+". [P7] 底部短事项 "+index;
                var node=new TaskNode(text){Tag=922000L+index};node.CurrentTextLength=text.Length;tasks.Nodes.Add(node);shortTailNodes.Add(node);
            }
            SetSimpleMode(true);Size=new Size(360,360);tasks.SingleLinePaths=true;tasks.SetWrappedText(false);
            longNode.Text="2. [P4] 很长的最底层任务文字 / 1. [P7] 同样很长的父任务文字 / 0. [P9] 更长的祖先任务文字";longNode.CurrentTextLength="2. [P4] 很长的最底层任务文字".Length;
            taskSurface.Rebuild(false);taskSurface.Update();
            if(taskSurface.ContentExtent.Width<=taskSurface.ClientSize.Width)throw new Exception("Single-line mode did not calculate a horizontal range from the longest item");
            if(taskSurface.ContentExtent.Height<=taskSurface.ClientSize.Height)throw new Exception("Single-line mode did not calculate a vertical range from all visible items");
            var imageRow=taskSurface.NodeBounds(imageLeaf);var plainRow=taskSurface.NodeBounds(plainLeaf);
            if(imageRow.Width<=plainRow.Width || taskSurface.ContentExtent.Width<imageRow.Right)throw new Exception("The image button was omitted from longest-item width calculation");
            taskSurface.AutoScrollPosition=Point.Empty;taskSurface.Update();
            var shortBefore=taskSurface.PriorityBounds(shortNode);var longBefore=taskSurface.PriorityBounds(longNode);
            int invalidations=0;InvalidateEventHandler invalidated=delegate{invalidations++;};taskSurface.Invalidated+=invalidated;
            taskSurface.TestWheel(-SystemInformation.MouseWheelScrollDelta,false);taskSurface.Update();var verticalOffset=taskSurface.ScrollOffset;
            if(verticalOffset.Y<=0 || verticalOffset.X!=0)throw new Exception("Mouse wheel did not scroll the single-line list vertically");
            var shortAfterVertical=taskSurface.PriorityBounds(shortNode);var longAfterVertical=taskSurface.PriorityBounds(longNode);
            taskSurface.TestWheel(-SystemInformation.MouseWheelScrollDelta,true);taskSurface.Update();var mixedOffset=taskSurface.ScrollOffset;
            taskSurface.Invalidated-=invalidated;
            if(mixedOffset.X<=0 || mixedOffset.Y!=verticalOffset.Y)throw new Exception("Horizontal wheel scrolling changed the vertical position or did not move horizontally");
            var shortAfter=taskSurface.PriorityBounds(shortNode);var longAfter=taskSurface.PriorityBounds(longNode);
            int shortShift=shortAfterVertical.Left-shortAfter.Left,longShift=longAfterVertical.Left-longAfter.Left;
            if(shortShift!=0 || longShift<=0 || shortAfter.Top!=shortAfterVertical.Top || longAfter.Top!=longAfterVertical.Top)throw new Exception("Horizontal scrolling blanked a short row or failed to scroll a long row");
            if(taskSurface.NodeBounds(shortNode).Height!=taskSurface.NodeBounds(longNode).Height)throw new Exception("Single-line rows with the same font have inconsistent heights");
            if(shortBefore.Top-shortAfterVertical.Top!=verticalOffset.Y || longBefore.Top-longAfterVertical.Top!=verticalOffset.Y)throw new Exception("Vertical scrolling produced inconsistent row positions");
            if(invalidations==0)throw new Exception("Scrolling did not invalidate the custom task surface");
            var preserved=taskSurface.ScrollOffset;taskSurface.Rebuild(true);taskSurface.Update();
            if(taskSurface.ScrollOffset!=preserved)throw new Exception("Refreshing the compact list did not preserve both scroll positions");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-single-line-scroll-test.png"));}
            int globalHorizontal=Math.Max(0,taskSurface.ContentExtent.Width-taskSurface.ClientSize.Width),bottom=Math.Max(0,taskSurface.ContentExtent.Height-taskSurface.ClientSize.Height);
            taskSurface.TestScrollTo(globalHorizontal,bottom);taskSurface.Update();
            if(taskSurface.ScrollOffset.X!=0)throw new Exception("Vertical scrolling into shorter rows retained an empty horizontal viewport");
            Rectangle tailTitle=Rectangle.Intersect(taskSurface.ClientRectangle,taskSurface.TitleBounds(shortTailNodes.Last()));
            if(tailTitle.IsEmpty)throw new Exception("The final visible short row has no title bounds after mixed-axis scrolling");
            using(var bitmap=new Bitmap(taskSurface.Width,taskSurface.Height)) {
                taskSurface.DrawToBitmap(bitmap,new Rectangle(Point.Empty,bitmap.Size));int ink=0;
                for(int y=tailTitle.Top;y<tailTitle.Bottom;y++)for(int x=tailTitle.Left;x<tailTitle.Right;x++){Color pixel=bitmap.GetPixel(x,y);if(pixel.R<190&&pixel.G<190&&pixel.B<190)ink++;}
                if(ink<3)throw new Exception("Scrolled short-row text was not painted until selection");
                bitmap.Save(Path.Combine(data,"floating-scroll-visible-rows-test.png"));
            }
            File.WriteAllText(Path.Combine(data,"floating-task-surface-test.txt"),"PASS: full/simple modes share one renderer; narrow wrapped titles use the full content width below their markers; wrapped rows are contiguous; compact single-line rows keep a consistent height; horizontal scrolling advances long rows without blanking shorter rows; every native and wheel scroll immediately repaints the custom surface; mixed-axis positions and refresh preservation are stable; shorter visible rows clamp stale horizontal offsets and retain painted text.");
        } finally {
            if(simpleMode)SetSimpleMode(false);tasks.Nodes.Clear();tasks.Nodes.AddRange(originalNodes);tasks.SingleLinePaths=originalFlat;tasks.SetWrappedText(originalWrap);Size=originalSize;taskSurface.Rebuild(false);
            if(originalMode)SetSimpleMode(true);if(originalSelection!=null&&originalSelection.TreeView==tasks)tasks.SelectedNode=originalSelection;rendering=originalRendering;
        }
    }
}
