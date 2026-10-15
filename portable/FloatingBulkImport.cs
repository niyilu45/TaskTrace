// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Linq;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed class FloatingBulkLine {
    internal readonly string Id=Guid.NewGuid().ToString();
    internal string Text;
    internal long TaskId;
    internal bool Saved;
    internal FloatingBulkLine(string text){Text=text;}
}
internal static class FloatingBulkText {
    static readonly Regex Prefix=new Regex(@"^(?:[-*+]\s+\[[ xX✓√]\]\s*|\[[ xX✓√]\]\s*|[-*+]\s+|[•·●○▪■□◆◇▶►→✓✔☐☑]\s*|[①-⑳]\s*|[（(](?:[0-9０-９]+|[一二三四五六七八九十百零〇]+|[a-zA-Z])[)）]\s*|(?:[0-9０-９]+(?:[.．][0-9０-９]+)*|[一二三四五六七八九十百零〇]+|[a-zA-Z])[、)）:：]\s*|(?:[0-9０-９]+(?:[.．][0-9０-９]+)*|[一二三四五六七八九十百零〇]+|[a-zA-Z])[.．](?![0-9０-９])\s*|[0-9０-９]{1,3}\s+)",RegexOptions.Compiled);
    internal static string Trim(string text){return Regex.Replace(text??"",@"^[\s\u200B\uFEFF\u2060]+|[\s\u200B\uFEFF\u2060]+$","");}
    internal static bool Meaningful(string text){return Regex.IsMatch(text??"", @"[\p{L}\p{N}]");}
    internal static List<string> Split(string text) {
        var result=new List<string>();
        foreach(string raw in Regex.Split(text??"",@"\r\n|[\r\n\u2028\u2029]")){
            string value=Trim(raw);
            for(int i=0;i<8;i++){if(Regex.IsMatch(value,@"^[A-Za-z]:[\\/]"))break;string next=Trim(Prefix.Replace(value,"",1));if(next==value)break;value=next;}
            if(Meaningful(value))result.Add(value);
        }
        return result;
    }
}
internal sealed class FloatingBulkImportDialog : Form {
    internal readonly TextBox Source=new TextBox {Name="bulk-source",Multiline=true,MaxLength=0,AcceptsReturn=true,ScrollBars=ScrollBars.Vertical,Dock=DockStyle.Fill,AccessibleName="粘贴待导入文字"};
    internal readonly DataGridView Preview=new DataGridView {Name="bulk-preview",Dock=DockStyle.Fill,AllowUserToAddRows=false,AllowUserToDeleteRows=true,AllowUserToResizeRows=false,RowHeadersVisible=false,ColumnHeadersHeightSizeMode=DataGridViewColumnHeadersHeightSizeMode.AutoSize,AutoGenerateColumns=false,BackgroundColor=SystemColors.Window,BorderStyle=BorderStyle.FixedSingle,SelectionMode=DataGridViewSelectionMode.FullRowSelect,MultiSelect=true,AutoSizeRowsMode=DataGridViewAutoSizeRowsMode.AllCells};
    internal readonly Button Confirm=new Button {Name="bulk-confirm",Text="确认导入",AutoSize=true};
    readonly Button cancel=new Button {Text="取消",AutoSize=true,Name="bulk-cancel"};
    readonly Label feedback=new Label {Name="bulk-feedback",AutoSize=true,Dock=DockStyle.Fill,Text="粘贴文字后自动生成预览。"};
    readonly Func<IList<FloatingBulkLine>,Action<int>,Task> save;
    readonly int maxLength;
    bool writing;
    internal bool HasSaved {get{return Preview.Rows.Cast<DataGridViewRow>().Any(row=>((FloatingBulkLine)row.Tag).Saved);}}
    internal FloatingBulkImportDialog(string noun,string destination,int priority,int limit,Font font,Func<IList<FloatingBulkLine>,Action<int>,Task> saveAction) {
        save=saveAction;maxLength=limit;
        Text="批量导入"+noun;Font=font;AutoScaleMode=AutoScaleMode.Font;StartPosition=FormStartPosition.CenterParent;
        Size=new Size(660,610);MinimumSize=new Size(500,480);ShowInTaskbar=true;MinimizeBox=true;
        var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=7};
        layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent,35));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        layout.RowStyles.Add(new RowStyle(SizeType.Percent,65));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));
        var description=new Label {Text="导入到："+destination+"  ·  优先级 P"+priority,Dock=DockStyle.Fill,AutoSize=true,Padding=new Padding(0,0,0,8)};
        var inputLabel=new Label {Text="粘贴文字（每行一条）",AutoSize=true,Dock=DockStyle.Fill,Padding=new Padding(0,0,0,4)};
        var previewLabel=new Label {Text="导入预览 · 可直接修改；选中整行后按 Delete 移除",AutoSize=true,Dock=DockStyle.Fill,Padding=new Padding(0,10,0,4)};
        Preview.AccessibleName="待导入"+noun+"列表";
        Preview.Columns.Add(new DataGridViewTextBoxColumn {Name="number",HeaderText="序号",Width=52,ReadOnly=true,SortMode=DataGridViewColumnSortMode.NotSortable});
        Preview.Columns.Add(new DataGridViewTextBoxColumn {Name="text",HeaderText=noun+"名称 / 内容",AutoSizeMode=DataGridViewAutoSizeColumnMode.Fill,SortMode=DataGridViewColumnSortMode.NotSortable,DefaultCellStyle=new DataGridViewCellStyle{WrapMode=DataGridViewTriState.True}});
        Preview.Columns.Add(new DataGridViewTextBoxColumn {Name="state",HeaderText="状态",Width=76,ReadOnly=true,SortMode=DataGridViewColumnSortMode.NotSortable});
        var actions=new FlowLayoutPanel {Dock=DockStyle.Fill,AutoSize=true,FlowDirection=FlowDirection.RightToLeft,Padding=new Padding(0,6,0,0)};
        actions.Controls.Add(cancel);actions.Controls.Add(Confirm);
        layout.Controls.Add(description);layout.Controls.Add(inputLabel);layout.Controls.Add(Source);layout.Controls.Add(previewLabel);layout.Controls.Add(Preview);layout.Controls.Add(feedback);layout.Controls.Add(actions);Controls.Add(layout);
        Source.TextChanged+=delegate{
            if(writing || !Source.Enabled)return;
            string normalized=Source.Text.Replace("\r\n","\n").Replace("\r","\n").Replace("\u2028","\n").Replace("\u2029","\n").Replace("\n","\r\n");
            if(normalized!=Source.Text){int caret=Source.SelectionStart;bool atEnd=caret==Source.TextLength;Source.Text=normalized;Source.SelectionStart=atEnd?Source.TextLength:Math.Min(caret,Source.TextLength);return;}
            Preview.Rows.Clear();var lines=FloatingBulkText.Split(Source.Text);
            foreach(string line in lines){int index=Preview.Rows.Add(Preview.Rows.Count+1,line,"待导入");Preview.Rows[index].Tag=new FloatingBulkLine(line);}
            int raw=Regex.Split(Source.Text,@"\r\n|[\r\n\u2028\u2029]").Length;
            feedback.Text=lines.Count==0?"没有有效条目，请粘贴包含文字或数字的内容。":"已识别 "+lines.Count+" 条；已过滤 "+Math.Max(0,raw-lines.Count)+" 行。可修改后确认导入。";
            Confirm.Enabled=lines.Count>0;
        };
        Preview.UserDeletingRow+=delegate(object sender,DataGridViewRowCancelEventArgs e){var item=e.Row.Tag as FloatingBulkLine;if(writing || (item!=null&&(item.Saved||item.TaskId>0)))e.Cancel=true;};
        Preview.RowsRemoved+=delegate{for(int i=0;i<Preview.Rows.Count;i++)Preview.Rows[i].Cells[0].Value=i+1;if(!writing && Source.Enabled)feedback.Text="当前预览 "+Preview.Rows.Count+" 条；可修改后确认导入。";};
        Preview.CellBeginEdit+=delegate(object sender,DataGridViewCellCancelEventArgs e){var item=(FloatingBulkLine)Preview.Rows[e.RowIndex].Tag;if(writing||item.Saved||item.TaskId>0)e.Cancel=true;};
        Confirm.Click+=async delegate{await Import();};
        cancel.Click+=delegate{Close();};
        FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing)e.Cancel=true;};
        CancelButton=cancel;Confirm.Enabled=false;
    }
    internal async Task<bool> Import() {
        if(writing)return false;
        Preview.EndEdit();
        var pending=new List<FloatingBulkLine>();
        foreach(DataGridViewRow row in Preview.Rows){
            var item=(FloatingBulkLine)row.Tag;if(item.Saved)continue;
            item.Text=FloatingBulkText.Trim(Convert.ToString(row.Cells[1].Value));row.Cells[1].Value=item.Text;
            if(!FloatingBulkText.Meaningful(item.Text)){row.Cells[2].Value="已忽略";continue;}
            if(item.Text.Length>maxLength){feedback.Text="第 "+(row.Index+1)+" 条超过 "+maxLength+" 个字符，请缩短后导入。";return false;}
            pending.Add(item);
        }
        if(pending.Count==0){feedback.Text="没有待导入的有效条目。";return false;}
        writing=true;Confirm.Enabled=false;cancel.Enabled=false;Source.Enabled=false;Preview.ReadOnly=true;
        try{
            await save(pending,delegate(int done){feedback.Text="正在导入 "+done+" / "+pending.Count+" 条…";RefreshRows();});
            RefreshRows();
            if(pending.Any(item=>!item.Saved))throw new Exception("部分条目未保存，请检查后重试。");
            writing=false;DialogResult=DialogResult.OK;Close();return true;
        }catch(Exception error){
            RefreshRows();feedback.Text="已导入 "+Preview.Rows.Cast<DataGridViewRow>().Count(row=>((FloatingBulkLine)row.Tag).Saved)+" 条；"+error.Message+" 未完成的内容已保留。";
            cancel.Text="关闭";return false;
        }finally{writing=false;if(!IsDisposed){Confirm.Enabled=true;cancel.Enabled=true;Preview.ReadOnly=false;RefreshRows();}}
    }
    void RefreshRows(){foreach(DataGridViewRow row in Preview.Rows){var item=(FloatingBulkLine)row.Tag;if(item.Saved){row.Cells[2].Value="已导入";row.ReadOnly=true;row.DefaultCellStyle.ForeColor=SystemColors.GrayText;}else if(item.TaskId>0)row.Cells[2].Value="待关联";}}
}
internal sealed partial class FloatingWindow {
    Button AddBulkButton(TableLayoutPanel row,EventHandler action) {
        var button=new Button {Text="批量导入…",Dock=DockStyle.Fill,Margin=Padding.Empty,AccessibleName="批量导入任务"};
        row.RowStyles[0].Height=28;row.Controls.Add(button,2,0);button.Click+=action;return button;
    }
    internal static async Task ImportBulkTasks(IList<FloatingBulkLine> lines,long projectId,long parentId,int priority,Func<string,string,object,Task<Dictionary<string,object>>> api,Action<int> progress) {
        int count=0;
        foreach(var line in lines){
            if(line.Saved){progress(++count);continue;}
            if(line.TaskId==0){var created=await api("POST","/projects/"+projectId+"/tasks",new{title=line.Text,priority=ApiPriority(priority)});line.TaskId=Convert.ToInt64(created["id"]);}
            if(parentId>0)await api("POST","/tasks/"+parentId+"/relations",new{other_task_id=line.TaskId,relation_kind="subtask"});
            line.Saved=true;progress(++count);
        }
    }
    async Task<bool> ShowBulkTasks(Form owner,long projectId,long parentId,int priority,string destination) {
        string group=Guid.NewGuid().ToString();
        using(var dialog=new FloatingBulkImportDialog("任务",destination,priority,250,Font,async delegate(IList<FloatingBulkLine> lines,Action<int> progress){
            if(parentId>0 && await TaskHierarchySpan(parentId)>=5)throw new Exception(TaskDepthMessage);
            using(BeginUndoGroup(group))await ImportBulkTasks(lines,projectId,parentId,priority,(method,path,body)=>Api(method,path,body),progress);
        })){
            dialog.Icon=Icon;dialog.TopMost=owner.TopMost;DpiDialog(dialog);dialog.ShowDialog(owner);
            if(dialog.HasSaved)await RefreshUndo();return dialog.HasSaved;
        }
    }
    async Task ShowBulkRootTasks() {
        var project=projects.SelectedItem as Project;
        if(busy||closing||project==null)return;
        SetBusy(true);timer.Stop();
        try{if(await ShowBulkTasks(this,project.Id,0,entryPriority.SelectedIndex,project.Title)){search.Clear();page=1;await LoadTasks();}}
        catch(Exception error){Error(error);}
        finally{SetBusy(false);timer.Start();}
    }
    async Task ImportBulkOutstanding(long taskId,IList<FloatingBulkLine> lines,int priority,Action<int> progress) {
        var current=ReadShared(await ReadHistory(taskId));
        int added=0;
        foreach(var line in lines)if(!current.Items.Any(item=>item.Id==line.Id)){current.Items.Add(new PendingItem{Id=line.Id,Html=OutstandingTextHtml(line.Text),Priority=priority});added++;}
        if(added>0)await WriteShared(taskId,current);
        int count=0;foreach(var line in lines){line.Saved=true;progress(++count);}
    }
}
