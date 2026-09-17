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
internal sealed class TaskTreeView : TreeView {
    internal TreeNode DropNode;
    internal int DropZone;
    internal bool Dropping;
    protected override void WndProc(ref Message message) {
        base.WndProc(ref message);
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
    void InitializeInteractions() {
        var priority = new Button {Text="优先级",AutoSize=true};
        var pictures = new Button {Text="查看图片",AutoSize=true};
        priority.Click += async delegate {await ShowPriority();};
        pictures.Click += async delegate {await ShowSelectedImages();};
        toolbar.Controls.AddRange(new Control[]{priority,pictures,prioritySort}); toolbar.Height=108;
        try { var prefs=ReadObject(File.ReadAllText(Path.Combine(data,"floating-order.json")));prioritySort.Checked=Convert.ToBoolean(prefs["priority"]); } catch { }
        prioritySort.CheckedChanged += async delegate {SaveSortPreference();if(!rendering) await Reload();};
        var menu=new ContextMenuStrip();
        var setPriority=menu.Items.Add("设置优先级…",null,async delegate{await ShowPriority();});
        menu.Items.Add("查看图片…",null,async delegate{await ShowSelectedImages();});
        menu.Items.Add("管理遗留事项…",null,delegate {long id=SelectedTaskId();if(id>0)ShowOutstanding(id);});
        var toRoot=menu.Items.Add("移为顶层任务",null,async delegate{long id=SelectedTaskId();if(id>0)await ExecuteDrop(new DropPlan{TaskId=id,ParentId=0,BeforeId=0,Message="移为顶层任务"});});
        menu.Opening+=delegate(object sender,System.ComponentModel.CancelEventArgs e){e.Cancel=busy;setPriority.Enabled=tasks.SelectedNode!=null && tasks.SelectedNode.Tag is long;toRoot.Enabled=setPriority.Enabled && taskParents.ContainsKey(SelectedTaskId());};
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
            int zone=DropZone(source,target,point.Y);var plan=MakeDropPlan(source,target,zone);
            e.Effect=plan==null?DragDropEffects.None:DragDropEffects.Move;
            tasks.Dropping=plan!=null;tasks.DropNode=target;tasks.DropZone=zone;tasks.Invalidate();
            string message=plan==null?"此处不能放置（不能形成循环或超过 5 级）":plan.Message;
            status.Text=message;
            if(target!=dropHover){dropHover=target;dropHoverSince=DateTime.UtcNow;progressTip.Hide(tasks);progressTip.Show(message,tasks,Math.Min(point.X+12,Math.Max(6,tasks.Width-240)),Math.Max(0,point.Y-28),1500);}
            if(target!=null && !target.IsExpanded && (DateTime.UtcNow-dropHoverSince).TotalMilliseconds>700)target.Expand();
            if((DateTime.UtcNow-scrollSince).TotalMilliseconds>180){if(point.Y<24 && tasks.TopNode!=null && tasks.TopNode.PrevVisibleNode!=null)tasks.TopNode=tasks.TopNode.PrevVisibleNode;else if(point.Y>tasks.Height-24 && target!=null && target.NextVisibleNode!=null)target.NextVisibleNode.EnsureVisible();scrollSince=DateTime.UtcNow;}
        };
        tasks.DragLeave+=delegate{ClearDropMark();};
        tasks.DragDrop+=async delegate(object sender,DragEventArgs e){
            var source=e.Data.GetData(typeof(TreeNode)) as TreeNode;var point=tasks.PointToClient(new Point(e.X,e.Y));var target=tasks.GetNodeAt(point);
            var plan=MakeDropPlan(source,target,DropZone(source,target,point.Y));ClearDropMark();if(plan!=null)await ExecuteDrop(plan);
        };
    }
    void SaveSortPreference(){try{File.WriteAllText(Path.Combine(data,"floating-order.json"),json.Serialize(new{priority=prioritySort.Checked}));}catch{}}
    void ClearDropMark(){tasks.Dropping=false;tasks.DropNode=null;dropHover=null;tasks.Invalidate();progressTip.Hide(tasks);}
    long SelectedTaskId(){var node=tasks.SelectedNode;if(node==null)return 0;if(node.Tag is long)return (long)node.Tag;var leaf=node.Tag as OutstandingLeaf;if(leaf!=null)return leaf.TaskId;var branch=node.Tag as OutstandingBranch;return branch==null?0:branch.TaskId;}
    static int DropZone(TreeNode source,TreeNode target,int y){
        if(target==null)return 0;
        if(source!=null && source.Tag is OutstandingLeaf)return target.Tag is OutstandingLeaf?(y<target.Bounds.Top+target.Bounds.Height/2?-1:1):0;
        int edge=Math.Max(5,target.Bounds.Height/4);return y<target.Bounds.Top+edge?-1:y>=target.Bounds.Bottom-edge?1:0;
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
        long destination=0;string beforeItem="";var targetItem=target.Tag as OutstandingLeaf;var targetBranch=target.Tag as OutstandingBranch;
        if(target.Tag is long)destination=(long)target.Tag;else if(targetBranch!=null)destination=targetBranch.TaskId;else if(targetItem!=null){destination=targetItem.TaskId;if(zone<0)beforeItem=targetItem.Id;else{var leaves=target.Parent.Nodes.Cast<TreeNode>().Select(node=>node.Tag as OutstandingLeaf).Where(leaf=>leaf!=null && !(leaf.TaskId==item.TaskId && leaf.Id==item.Id)).ToList();int index=leaves.FindIndex(leaf=>leaf.Id==targetItem.Id);if(index>=0 && index+1<leaves.Count)beforeItem=leaves[index+1].Id;}}
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
            long selected=plan.ItemId==null?plan.TaskId:plan.TargetId;var matches=tasks.Nodes.Find(selected.ToString(),true);if(matches.Length>0){tasks.SelectedNode=matches[0];matches[0].EnsureVisible();if(plan.ItemId!=null){var branch=matches[0].Nodes.Cast<TreeNode>().FirstOrDefault(node=>node.Tag is OutstandingBranch);if(branch!=null)branch.Expand();}}
            status.Text="归属和顺序已保存。";
        }catch(Exception e){Error(e);}finally{SetBusy(false);timer.Start();}
    }
    async Task ReadTaskOrder(long projectId,Dictionary<long,Dictionary<string,object>> all){
        var views=await Api("GET","/projects/"+projectId+"/views",null);
        var options=((IEnumerable)views["items"]).Cast<Dictionary<string,object>>().ToList();
        var view=options.FirstOrDefault(item=>Convert.ToString(item["view_kind"])=="list")??options.FirstOrDefault();taskViewId=view==null?0:Convert.ToInt64(view["id"]);
        taskOrder.Clear();foreach(var pair in all){object index;if(pair.Value.TryGetValue("index",out index))taskOrder[pair.Key]=Convert.ToDouble(index)*65536.0;}if(taskViewId==0)return;
        for(int p=1;;p++){
            var positions=await Api("GET","/projects/"+projectId+"/views/"+taskViewId+"/tasktrace-positions?per_page=100&page="+p,null);
            foreach(Dictionary<string,object> item in (IEnumerable)positions["items"])taskOrder[Convert.ToInt64(item["task_id"])]=Convert.ToDouble(item["position"]);
            if(p>=Convert.ToInt32(positions["total_pages"]))break;
        }
    }
    List<long> SortTaskIds(List<long> ids,Dictionary<long,Dictionary<string,object>> all){
        if(prioritySort.Checked)return ids.OrderBy(id=>PriorityNumber(all[id])).ThenBy(id=>PositionOf(id)).ThenBy(id=>id).ToList();
        return ids.OrderBy(id=>PositionOf(id)).ThenBy(id=>id).ToList();
    }
    void NumberTasks(List<TreeNode> roots,Dictionary<long,Dictionary<string,object>> all){for(int index=0;index<roots.Count;index++)NumberTask(roots[index],(index+1).ToString(),all);}
    void NumberTask(TreeNode node,string number,Dictionary<long,Dictionary<string,object>> all){
        long id=(long)node.Tag;int priority=PriorityNumber(all[id]);node.Text=number+". "+"[P"+priority+"] "+(string)all[id]["title"];
        int index=0;foreach(TreeNode child in node.Nodes)if(child.Tag is long)NumberTask(child,number+"."+(++index),all);
    }
    async Task ShowPriority(){
        if(busy || closing)return;long id=SelectedTaskId();if(id==0 || !(tasks.SelectedNode.Tag is long)){status.Text="请先选中一个任务。";return;}
        SetBusy(true);timer.Stop();
        try {
            var current=await Api("GET","/tasks/"+id,null);
            using(var dialog=new Form{Text="优先级 · "+(string)current["title"],Size=new Size(350,210),MinimumSize=new Size(350,210),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false}){
                var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=3};
                layout.RowStyles.Add(new RowStyle(SizeType.Absolute,44));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,36));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
                var choice=new ComboBox{Dock=DockStyle.Fill,DropDownStyle=ComboBoxStyle.DropDownList};for(int value=0;value<=9;value++)choice.Items.Add(value+(value==0?" · 最高":value==9?" · 最低（默认）":""));choice.SelectedIndex=PriorityNumber(current);
                var save=new Button{Text="保存优先级",AutoSize=true};bool writing=false;
                layout.Controls.Add(new Label{Text="0 最高，9 最低；默认 9。按优先级排列时，\r\n同级任务排序，下级任务保留在父任务下。",Dock=DockStyle.Fill});layout.Controls.Add(choice);layout.Controls.Add(save);dialog.Controls.Add(layout);
                save.Click+=async delegate{if(writing)return;writing=true;save.Enabled=false;choice.Enabled=false;try{await Api("PATCH","/tasks/"+id,new{priority=10-choice.SelectedIndex});writing=false;dialog.Close();}catch(Exception e){MessageBox.Show(dialog,e.Message,"优先级未保存");}finally{writing=false;if(!dialog.IsDisposed){save.Enabled=true;choice.Enabled=true;}}};
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
    async void ShowOutstanding(long id,bool nested=false){ await EditOutstanding(id,nested,false); }
    async Task EditOutstanding(long id,bool nested,bool verify){
        if((busy && !nested)||closing)return;var owner=Form.ActiveForm??this;SetBusy(true);timer.Stop();editingOutstanding=true;
        try {
            var shared=ReadShared(await ReadHistory(id));
            using(var dialog=new Form{Text="遗留事项 · 所有日期共享",Size=new Size(580,570),MinimumSize=new Size(500,480),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false,KeyPreview=true}){
                var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(12),ColumnCount=1,RowCount=6};
                layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,30));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,70));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,68));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,68));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,44));
                var list=new ListBox{Dock=DockStyle.Fill,HorizontalScrollbar=true,AccessibleName="遗留事项清单"};
                var mode=new Label{Text="新增遗留事项（支持 Ctrl+V 粘贴图片）",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft};
                var input=new TextBox{Dock=DockStyle.Fill,Multiline=true,ScrollBars=ScrollBars.Vertical,AccessibleName="遗留事项内容"};
                var previews=new FlowLayoutPanel{Dock=DockStyle.Fill,AutoScroll=true,WrapContents=false};
                var buttons=new FlowLayoutPanel{Dock=DockStyle.Fill};
                var save=new Button{Text="添加一条",AutoSize=true};var fresh=new Button{Text="新增事项",AutoSize=true};var remove=new Button{Text="移除选中",AutoSize=true};var files=new Button{Text="添加图片…",AutoSize=true};var gallery=new Button{Text="查看图片",AutoSize=true};var clearImages=new Button{Text="移除已有图片",AutoSize=true};var recover=new Button{Text="另存为新事项",AutoSize=true,Visible=false};
                buttons.Controls.AddRange(new Control[]{save,fresh,files,gallery,remove,clearImages,recover});
                var feedback=new Label{Text="选择一条可编辑；拖动归属和顺序请返回悬浮窗。",Dock=DockStyle.Fill};
                layout.Controls.Add(list);layout.Controls.Add(mode);layout.Controls.Add(input);layout.Controls.Add(previews);layout.Controls.Add(buttons);layout.Controls.Add(feedback);dialog.Controls.Add(layout);
                var pictures=new List<PastedImage>();string editingId=null,originalText="",originalHtml="",draftId=Guid.NewGuid().ToString();bool writing=false,loading=false,removeExistingImages=false;
                Action disposePreviews=delegate{foreach(Control control in previews.Controls.Cast<Control>().ToArray()){var box=control as PictureBox;if(box!=null && box.Image!=null)box.Image.Dispose();control.Dispose();}previews.Controls.Clear();};
                Action renderPreviews=null;renderPreviews=delegate{
                    disposePreviews();foreach(var picture in pictures.ToList()){
                        using(var stream=new MemoryStream(picture.Bytes))using(var image=Image.FromStream(stream)){
                            var box=new PictureBox{Image=new Bitmap(image),SizeMode=PictureBoxSizeMode.Zoom,Width=62,Height=56,Cursor=Cursors.Hand,AccessibleName="点击移除此待保存图片"};
                            box.Click+=delegate{if(!writing){pictures.Remove(picture);renderPreviews();}};previews.Controls.Add(box);
                        }
                    }
                    int existing=removeExistingImages?0:Regex.Matches(originalHtml,@"<img\b",RegexOptions.IgnoreCase).Count;
                    if(existing>0)previews.Controls.Add(new Label{AutoSize=true,Text="已保存 "+existing+" 张图片\r\n点击“查看图片”浏览",Padding=new Padding(3,10,0,0)});
                    clearImages.Enabled=!writing && existing>0;
                };
                Action render=delegate{loading=true;list.Items.Clear();for(int i=0;i<shared.Items.Count;i++){shared.Items[i].Number=i+1;list.Items.Add(shared.Items[i]);}loading=false;};render();
                Func<bool> dirty=delegate{return input.Text!=originalText || pictures.Count>0 || removeExistingImages;};
                Func<bool> mayDiscard=delegate{return !dirty() || MessageBox.Show(dialog,"当前输入尚未保存，是否放弃？","遗留事项",MessageBoxButtons.YesNo,MessageBoxIcon.Question)==DialogResult.Yes;};
                Action<PendingItem> edit=delegate(PendingItem item){editingId=item==null?null:item.Id;draftId=Guid.NewGuid().ToString();originalHtml=item==null?"":item.Html;originalText=Plain(Regex.Replace(originalHtml,@"<img\b[^>]*>","",RegexOptions.IgnoreCase));input.Text=originalText;pictures.Clear();removeExistingImages=false;recover.Visible=false;save.Text=item==null?"添加一条":"保存修改";mode.Text=item==null?"新增遗留事项（支持 Ctrl+V 粘贴图片）":"编辑第 "+item.Number+" 条（支持 Ctrl+V 粘贴图片）";renderPreviews();};
                list.SelectedIndexChanged+=delegate{if(loading || writing)return;var selected=list.SelectedItem as PendingItem;if(selected==null || selected.Id==editingId)return;if(!mayDiscard()){loading=true;list.SelectedItem=shared.Items.FirstOrDefault(item=>item.Id==editingId);loading=false;return;}edit(selected);};
                fresh.Click+=delegate{if(writing || !mayDiscard())return;loading=true;list.ClearSelected();loading=false;edit(null);input.Focus();};
                recover.Click+=delegate{if(writing)return;editingId=null;draftId=Guid.NewGuid().ToString();originalHtml="";originalText="";removeExistingImages=false;save.Text="添加一条";mode.Text="另存为新事项：输入和待保存图片已保留";recover.Visible=false;loading=true;list.ClearSelected();loading=false;renderPreviews();};
                clearImages.Click+=delegate{removeExistingImages=true;renderPreviews();};
                files.Click+=delegate{if(writing)return;using(var picker=new OpenFileDialog{Filter="图片|*.png;*.jpg;*.jpeg;*.bmp;*.gif|所有文件|*.*",Multiselect=true}){if(picker.ShowDialog(dialog)!=DialogResult.OK)return;foreach(string path in picker.FileNames)try{using(var image=Image.FromFile(path))using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}}catch{feedback.Text="部分文件无法读取，请选择 PNG、JPG、BMP 或 GIF 图片。";}renderPreviews();}};
                gallery.Click+=async delegate{await ShowImageGallery(id,editingId==null?null:originalHtml,dialog);};
                Func<bool,Task> write=async delegate(bool deleting){
                    if(writing)return;if(deleting && editingId==null)return;
                    if(!deleting && String.IsNullOrWhiteSpace(input.Text) && pictures.Count==0 && (removeExistingImages || !Regex.IsMatch(originalHtml,@"<img\b",RegexOptions.IgnoreCase))){feedback.Text="请输入内容或添加图片。";return;}
                    writing=true;buttons.Enabled=false;list.Enabled=false;input.ReadOnly=true;
                    try{
                        string uploaded=deleting?"":await UploadOutstandingPictures(id,pictures);
                        var current=ReadShared(await ReadHistory(id));var existing=current.Items.FirstOrDefault(item=>item.Id==(editingId??draftId));
                        if(editingId!=null && existing==null)throw new Exception("这条事项已被移动或移除；输入已保留，请重新读取后添加。");
                        if(existing!=null && existing.Html!=originalHtml)throw new Exception("这条事项已在其他窗口更新。请保留输入并重新打开，避免覆盖修改。");
                        if(deleting)current.Items.Remove(existing);
                        else{
                            string body=input.Text==originalText?originalHtml:"<p>"+WebUtility.HtmlEncode(input.Text.Trim()).Replace("\r\n","<br>").Replace("\n","<br>")+"</p>"+String.Join("",Regex.Matches(originalHtml,@"<img\b[^>]*>",RegexOptions.IgnoreCase).Cast<Match>().Select(match=>match.Value));
                            if(removeExistingImages)body=Regex.Replace(body,@"<img\b[^>]*>","",RegexOptions.IgnoreCase);
                            if(existing==null)current.Items.Add(new PendingItem{Id=draftId,Html=body+uploaded});else existing.Html=body+uploaded;
                        }
                        await WriteShared(id,current);shared=current;render();edit(null);feedback.Text="已保存。图片随遗留事项保存，所有日期共享。";
                    }catch(Exception e){feedback.Text=e.Message;recover.Visible=editingId!=null;}
                    finally{writing=false;buttons.Enabled=true;list.Enabled=true;input.ReadOnly=false;}
                };
                save.Click+=async delegate{await write(false);};remove.Click+=async delegate{await write(true);};
                dialog.KeyDown+=delegate(object sender,KeyEventArgs e){
                    if(e.Control && e.KeyCode==Keys.V && !writing && Clipboard.ContainsImage()){e.SuppressKeyPress=true;try{using(var image=Clipboard.GetImage())using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}renderPreviews();feedback.Text="已粘贴图片，点击保存；点击缩略图可移除待保存图片。";}catch{feedback.Text="剪贴板读取失败，请重试。";}}
                    if(e.Control && e.KeyCode==Keys.Enter){e.SuppressKeyPress=true;save.PerformClick();}
                };
                Exception verificationError=null;
                if(verify)dialog.Shown+=async delegate{
                    try{
                        int count=shared.Items.Count;input.Text="编辑器图片验收";
                        using(var bitmap=new Bitmap(42,28))using(var stream=new MemoryStream()){using(var canvas=Graphics.FromImage(bitmap))canvas.Clear(Color.Teal);bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}
                        renderPreviews();await write(false);
                        if(shared.Items.Count!=count+1 || Regex.Matches(shared.Items.Last().Html,@"<img\b").Count!=1)throw new Exception("Outstanding editor image create failed: "+feedback.Text);
                        list.SelectedIndex=list.Items.Count-1;input.Text="编辑器修改验收";
                        using(var bitmap=new Bitmap(32,20))using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}
                        await write(false);
                        if(shared.Items.Count!=count+1 || !shared.Items.Last().Html.Contains("编辑器修改验收") || Regex.Matches(shared.Items.Last().Html,@"<img\b").Count!=2)throw new Exception("Outstanding editor image update failed: "+feedback.Text);
                        list.SelectedIndex=list.Items.Count-1;
                        using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-outstanding-editor-test.png"));}
                    }catch(Exception e){verificationError=e;}finally{dialog.Close();}
                };
                dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing || (!verify && !mayDiscard()))e.Cancel=true;};
                try{dialog.ShowDialog(owner);if(verificationError!=null)throw verificationError;}finally{disposePreviews();}
            }
            await LoadTasks();
        }catch(Exception e){if(verify)throw;Error(e);}finally{editingOutstanding=false;if(!nested){SetBusy(false);timer.Start();}}
    }

    sealed class GalleryImage {public string Source,Caption;}
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
    async Task<List<GalleryImage>> GatherImages(long taskId,string onlyHtml,Form gallery){
        var images=new List<GalleryImage>();
        if(onlyHtml!=null){CollectImages(images,onlyHtml,TaskTitle(taskId)+" · 遗留事项");return images;}
        var queue=new Queue<long>();var seen=new HashSet<long>();queue.Enqueue(taskId);
        while(queue.Count>0 && !gallery.IsDisposed){
            long id=queue.Dequeue();if(!seen.Add(id))continue;
            var task=await Api("GET","/tasks/"+id,null);string title=(string)task["title"];
            CollectImages(images,Convert.ToString(task["description"]),title+" · 描述");
            var history=await ReadHistory(id);var shared=ReadShared(history);int index=0;
            foreach(var item in shared.Items)CollectImages(images,item.Html,title+" · 遗留事项 "+(++index));
            foreach(var note in DailyHistory(history))CollectImages(images,ProgressBody((string)note["comment"]),title+" · "+DayOf(note)+" 进展");
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
    async Task ShowImageGallery(long id,string onlyHtml,Form owner){
        var gallery=new Form{Text="查看图片 · "+TaskTitle(id),Size=new Size(800,650),MinimumSize=new Size(400,300),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false};
        var heading=new Label{Dock=DockStyle.Top,Height=42,Padding=new Padding(12,10,12,0),Text="正在收集图片…"};
        var flow=new FlowLayoutPanel{Dock=DockStyle.Fill,AutoScroll=true,FlowDirection=FlowDirection.TopDown,WrapContents=false,Padding=new Padding(10)};
        gallery.Controls.Add(flow);gallery.Controls.Add(heading);var bitmaps=new List<Image>();
        gallery.FormClosed+=delegate{foreach(var image in bitmaps)image.Dispose();};
        gallery.Show(owner);
        try{
            var images=await GatherImages(id,onlyHtml,gallery);if(gallery.IsDisposed)return;
            heading.Text=images.Count==0?"没有图片。可在遗留事项、任务描述或每日进展中添加。":images.Count+" 张图片 · "+(onlyHtml==null?"当前任务及所有下级任务":"当前遗留事项");
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

    async Task TestInteractions(){
        var project=projects.SelectedItem as Project;var created=new List<long>();
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
            await ExecuteDrop(MakeDropPlan(node(c),node(a),-1));if(prioritySort.Checked)throw new Exception("Manual drag did not restore manual ordering");
            var pictures=new List<PastedImage>();using(var bitmap=new Bitmap(80,50)){using(var canvas=Graphics.FromImage(bitmap))canvas.Clear(Color.SteelBlue);using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage{Bytes=stream.ToArray()});}}
            string imageHtml=await UploadOutstandingPictures(a,pictures);var source=new SharedList();source.Items.Add(new PendingItem{Id="drag-image",Html="带图片的遗留事项"+imageHtml});source.Items.Add(new PendingItem{Id="drag-text",Html="其他事项"});await WriteShared(a,source);
            var target=new SharedList();target.Items.Add(new PendingItem{Id="existing",Html="已存在的遗留事项"});await WriteShared(b,target);
            await ExecuteDrop(new DropPlan{TaskId=a,TargetId=b,ItemId="drag-image",BeforeItemId="existing"});
            var moved=ReadShared(await ReadHistory(b));var remaining=ReadShared(await ReadHistory(a));
            if(moved.Items.Count!=2 || moved.Items[0].Id!="drag-image" || remaining.Items.Count!=1 || remaining.Items[0].Id!="drag-text")throw new Exception("Outstanding move lost contents or order");
            if(moved.Items[0].Html.Contains("/tasks/"+a+"/attachments/"))throw new Exception("Moved image still depends on source task");
            await ExecuteDrop(new DropPlan{TaskId=b,TargetId=b,ItemId="existing",BeforeItemId="drag-image"});moved=ReadShared(await ReadHistory(b));if(moved.Items[0].Id!="existing")throw new Exception("Outstanding reorder not saved");
            await Api("DELETE","/tasks/"+a,null);created.Remove(a);await LoadTasks();
            var images=new List<GalleryImage>();CollectImages(images,moved.Items[1].Html,"test");if(images.Count!=1)throw new Exception("Gallery did not discover moved image");
            byte[] bytes=await DownloadImage(images[0].Source);using(var stream=new MemoryStream(bytes))using(var bitmap=Image.FromStream(stream))if(bitmap.Width!=80)throw new Exception("Image copy was corrupted after source deletion");
            if(AttachmentPath("https://example.com/api/v1/tasks/1/attachments/1")!=null || AttachmentPath("//example.com/api/v1/tasks/1/attachments/1")!=null)throw new Exception("External gallery URL was accepted");
            await ShowImageGallery(b,null,this);
            await EditOutstanding(b,false,true);if(editingOutstanding)throw new Exception("Outstanding editor did not resume auto-save");
            foreach(var item in moved.Items)if(String.IsNullOrEmpty(OutstandingText(item.Html)))throw new Exception("Image-only outstanding title missing");
            tasks.Nodes[0].Expand();using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-interactions-test.png"));}
            File.WriteAllText(Path.Combine(data,"floating-interactions-test.txt"),"PASS: drop targets, task parent/order persisted, cycle blocked, hierarchical numbering, priorities 0-9 + default 9, persisted sort setting, manual drag restores manual order, outstanding reorder + cross-task transfer, copied image survives deleting source, gallery downloads, external URLs rejected.");
        }
        {
            foreach(long id in created.AsEnumerable().Reverse())try{await Api("DELETE","/tasks/"+id,null);}catch{}
            rendering=true;prioritySort.Checked=false;rendering=false;await LoadTasks();
        }
    }
}
