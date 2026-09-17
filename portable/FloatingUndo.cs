// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    readonly Button undoButton = new Button { Text = "撤销", AutoSize = true, Enabled = false, AccessibleName = "撤销最近操作" };
    ToolStripItem undoMenu;
    long undoId;
    string undoLabel = "", undoGroup;
    bool undoRecording = true, undoing;
    sealed class UndoScope : IDisposable {
        readonly FloatingWindow owner;
        readonly string previous;
        public UndoScope(FloatingWindow window, string group) { owner=window; previous=window.undoGroup; if(previous==null)window.undoGroup=group ?? Guid.NewGuid().ToString(); }
        public void Dispose() { owner.undoGroup=previous; }
    }
    IDisposable BeginUndoGroup(string group = null) { return new UndoScope(this,group); }
    void InitializeUndo(ContextMenuStrip menu) {
        toolbar.Controls.Add(undoButton);
        undoButton.Click += async delegate { await PerformUndo(); };
        menu.Items.Add(new ToolStripSeparator());
        undoMenu=menu.Items.Add("撤销最近操作 (Ctrl+Z)",null,async delegate {await PerformUndo();});
        undoMenu.Enabled=false;
        KeyDown+=async delegate(object sender,KeyEventArgs e) {
            if(e.Control && !e.Shift && !e.Alt && e.KeyCode==Keys.Z && !EditingText()) {
                e.Handled=true;e.SuppressKeyPress=true;await PerformUndo();
            }
        };
    }
    bool EditingText() {
        Control active=ActiveControl;
        while(active is ContainerControl && ((ContainerControl)active).ActiveControl!=null)active=((ContainerControl)active).ActiveControl;
        return active is TextBoxBase || active is ComboBox;
    }
    void UpdateUndoControls() {
        if(closing || IsDisposed)return;
        bool available=undoId>0 && !busy && !undoing;
        undoButton.Enabled=available;
        string text=undoId>0 ? "撤销："+undoLabel+" (Ctrl+Z)" : "暂无可撤销操作";
        progressTip.SetToolTip(undoButton,text);
        if(undoMenu!=null){undoMenu.Enabled=available;undoMenu.Text=text;}
    }
    async Task RefreshUndo() {
        long previousId=undoId;string previousLabel=undoLabel;
        try {
            var state=await Api("GET","/tasktrace/undo",null);
            undoId=Convert.ToInt64(state["id"]);undoLabel=Convert.ToString(state["label"]);
        } catch { undoId=0;undoLabel=""; }
        if(previousId!=undoId || previousLabel!=undoLabel)UpdateUndoControls();
    }
    async Task PerformUndo() {
        if(busy || closing || undoing)return;
        if(undoId==0){await RefreshUndo();return;}
        long expected=undoId;string label=undoLabel;
        undoing=true;SetBusy(true);
        try {
            await Api("POST","/tasktrace/undo",new{id=expected});
            projectsDirty=true;await LoadTasks();await RefreshUndo();
            status.ForeColor=ForeColor;status.Text="已撤销："+label;
        } catch(Exception e) {
            Error(e);
            if(simpleMode && !selfTest) MessageBox.Show(this,e.Message,"无法撤销",MessageBoxButtons.OK,MessageBoxIcon.Information);
        }
        await RefreshUndo();undoing=false;SetBusy(false);
    }
    async Task UndoForTest() {
        await RefreshUndo();if(undoId==0)throw new Exception("Undo journal is empty");
        await Api("POST","/tasktrace/undo",new{id=undoId});await RefreshUndo();
    }
    async Task TestUndo() {
        var project=projects.SelectedItem as Project;
        var cleanup=new List<long>();
        undoRecording=false;
        long a=Convert.ToInt64((await Api("POST","/projects/"+project.Id+"/tasks",new{title="撤销验收 A"}))["id"]);cleanup.Add(a);
        long b=Convert.ToInt64((await Api("POST","/projects/"+project.Id+"/tasks",new{title="撤销验收 B"}))["id"]);cleanup.Add(b);
        await LoadTasks();
        var picture=new List<PastedImage>();
        using(var bitmap=new Bitmap(32,24))using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);picture.Add(new PastedImage{Bytes=stream.ToArray()});}
        string imageHtml=await UploadOutstandingPictures(a,picture);
        var shared=new SharedList();shared.Items.Add(new PendingItem{Id="undo-image",Html="保留原图片"+imageHtml});await WriteShared(a,shared);
        var target=new SharedList();target.Items.Add(new PendingItem{Id="keep",Html="已有事项"});await WriteShared(b,target);
        undoRecording=true;
        await Api("PATCH","/tasks/"+a,new{priority=10});await RefreshUndo();long stale=undoId;
        await Api("PATCH","/tasks/"+a,new{title="改名后"});
        bool rejected=false;try{await Api("POST","/tasktrace/undo",new{id=stale});}catch{rejected=true;}
        if(!rejected || (string)(await Api("GET","/tasks/"+a,null))["title"]!="改名后")throw new Exception("Stale undo changed current state");
        await UndoForTest();if((string)(await Api("GET","/tasks/"+a,null))["title"]!="撤销验收 A")throw new Exception("Rename undo failed");
        // A concurrent, unrecorded edit to another field must survive undo.
        undoRecording=false;await Api("PATCH","/tasks/"+a,new{description="另一窗口的新内容"});undoRecording=true;
        await UndoForTest();var item=await Api("GET","/tasks/"+a,null);
        if(PriorityNumber(item)!=9 || (string)item["description"]!="另一窗口的新内容")throw new Exception("Priority undo lost unrelated content");
        await Api("PATCH","/tasks/"+a,new{done=true});await UndoForTest();if(Convert.ToBoolean((await Api("GET","/tasks/"+a,null))["done"]))throw new Exception("Completion undo failed");
        await Api("PATCH","/tasks/"+a,new{priority=10});
        undoRecording=false;await Api("PATCH","/tasks/"+a,new{priority=5});undoRecording=true;
        await RefreshUndo();long conflictId=undoId;rejected=false;try{await Api("POST","/tasktrace/undo",new{id=conflictId});}catch{rejected=true;}
        await RefreshUndo();if(!rejected || undoId!=conflictId || Convert.ToInt32((await Api("GET","/tasks/"+a,null))["priority"])!=5)throw new Exception("Conflict undo overwrote newer data");
        undoRecording=false;await Api("PATCH","/tasks/"+a,new{priority=10});undoRecording=true;await UndoForTest();
        long child=await CreateSubtask(a,project.Id,"成组撤销的子任务");
        await UndoForTest();rejected=false;try{await Api("GET","/tasks/"+child,null);}catch{rejected=true;}if(!rejected)throw new Exception("Grouped creation left an orphan task");
        await Api("DELETE","/tasks/"+a,null);await UndoForTest();if((string)(await Api("GET","/tasks/"+a,null))["title"]!="撤销验收 A")throw new Exception("Deleted task undo failed");
        long note=await SaveProgress(a,DateTime.Today,"原进展","");
        await SaveProgress(a,DateTime.Today,"改后的进展","",null,note);await UndoForTest();
        if(!DailyHistory(await ReadHistory(a)).Any(row=>((string)row["comment"]).Contains("原进展")))throw new Exception("Progress edit undo failed");
        await UndoForTest();if(DailyHistory(await ReadHistory(a)).Any(row=>Convert.ToInt64(row["id"])==note))throw new Exception("Progress create undo failed");
        await LoadTasks();long oldParent=ParentOf(b);
        await Api("POST","/tasks/"+b+"/move",new{parent_id=a,before_task_id=0,project_view_id=taskViewId});await UndoForTest();await LoadTasks();if(ParentOf(b)!=oldParent)throw new Exception("Move undo failed");
        await Api("POST","/tasks/"+a+"/outstanding/move",new{target_task_id=b,item_id="undo-image",before_item_id="keep"});await UndoForTest();
        shared=ReadShared(await ReadHistory(a));target=ReadShared(await ReadHistory(b));
        if(shared.Items.Count!=1 || shared.Items[0].Id!="undo-image" || target.Items.Count!=1 || target.Items[0].Id!="keep")throw new Exception("Outstanding move undo lost items");
        if((await DownloadImage("/api/v1/tasks/"+a+"/attachments/"+picture[0].Id)).Length==0)throw new Exception("Undo removed original image");
        await Api("PATCH","/tasks/"+a,new{title="按钮撤销"});await RefreshUndo();SetBusy(false);
        if(!undoButton.Enabled)throw new Exception("Undo button is not enabled");
        await PerformUndo();if((string)(await Api("GET","/tasks/"+a,null))["title"]!="撤销验收 A")throw new Exception("Undo button action failed");
        entry.Focus();if(!EditingText())throw new Exception("Text Ctrl+Z was not isolated");tasks.Focus();if(EditingText())throw new Exception("Tree Ctrl+Z was not enabled");
        undoRecording=false;foreach(long id in cleanup)await Api("DELETE","/tasks/"+id,null);undoRecording=true;await LoadTasks();await RefreshUndo();
        File.WriteAllText(Path.Combine(data,"floating-undo-test.txt"),"PASS: native toolbar undo, stale request rejection, unrelated field preservation, conflict safety and retry, grouped child creation, task delete/restore, progress create/edit undo, task move undo, outstanding image transfer undo, original image retained, input Ctrl+Z isolation.");
    }
}
