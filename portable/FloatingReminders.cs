// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Drawing;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    sealed class ReminderTarget {
        public string Key,Title,Kind;
        public long TaskId;
        public string ItemId;
        public DateTimeOffset Due;
    }
    sealed class ReminderState {
        public Dictionary<string,string> Dismissed {get;set;}
        public Dictionary<string,string> SnoozedUntil {get;set;}
        public ReminderState(){Dismissed=new Dictionary<string,string>();SnoozedUntil=new Dictionary<string,string>();}
    }
    sealed class ReminderChoice {
        public Dictionary<string,object> Value;
        public DateTimeOffset Due;
        public override string ToString(){return Due.LocalDateTime.ToString("yyyy-MM-dd HH:mm")+(Convert.ToString(Value.ContainsKey("relative_to")?Value["relative_to"]:"").Length>0?" · 网页相对提醒":"");}
    }

    readonly Timer reminderTimer=new Timer{Interval=15000};
    readonly Dictionary<string,ReminderTarget> reminderTargets=new Dictionary<string,ReminderTarget>();
    ReminderState reminderState;
    Form reminderPopup;

    string ReminderStatePath {get{return Path.Combine(data,"floating-reminder-state.json");}}
    static bool TryReminderTime(object value,out DateTimeOffset due) {
        return DateTimeOffset.TryParse(Convert.ToString(value),CultureInfo.InvariantCulture,DateTimeStyles.RoundtripKind,out due);
    }
    static List<Dictionary<string,object>> TaskReminderValues(Dictionary<string,object> task) {
        var result=new List<Dictionary<string,object>>();object value;
        if(task==null || !task.TryGetValue("reminders",out value) || value==null)return result;
        var sequence=value as IEnumerable;if(sequence==null)return result;
        foreach(object entry in sequence) {
            var reminder=entry as Dictionary<string,object>;
            if(reminder!=null)result.Add(new Dictionary<string,object>(reminder));
        }
        return result;
    }
    static List<DateTimeOffset> TaskReminderTimes(Dictionary<string,object> task) {
        var result=new List<DateTimeOffset>();
        foreach(var reminder in TaskReminderValues(task)) {object value;DateTimeOffset due;if(reminder.TryGetValue("reminder",out value)&&TryReminderTime(value,out due))result.Add(due);}
        return result;
    }
    static int DirectTaskReminderCount(Dictionary<string,object> task,SharedList shared) {
        var owned=new HashSet<string>();
        if(shared!=null)foreach(var item in shared.Items){DateTimeOffset due;if(TryReminderTime(item.ReminderAt,out due))owned.Add(ReminderMomentKey(due));}
        return TaskReminderTimes(task).Count(due=>!owned.Contains(ReminderMomentKey(due)));
    }
    static string ReminderStamp(DateTimeOffset value){return value.ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ss.fffZ",CultureInfo.InvariantCulture);}
    static string ReminderMomentKey(DateTimeOffset value){return value.ToUniversalTime().ToString("yyyyMMddHHmmssfff",CultureInfo.InvariantCulture);}
    static bool SameReminderTime(Dictionary<string,object> value,DateTimeOffset target) {
        object raw;DateTimeOffset parsed;return value!=null&&value.TryGetValue("reminder",out raw)&&TryReminderTime(raw,out parsed)&&Math.Abs((parsed.ToUniversalTime()-target.ToUniversalTime()).TotalSeconds)<1;
    }
    static bool SameReminderTime(string value,DateTimeOffset target) {
        DateTimeOffset parsed;return TryReminderTime(value,out parsed)&&Math.Abs((parsed.ToUniversalTime()-target.ToUniversalTime()).TotalSeconds)<1;
    }
    static Dictionary<string,object> AbsoluteReminder(DateTime local) {
        return new Dictionary<string,object>{{"reminder",ReminderStamp(new DateTimeOffset(local))},{"relative_period",0},{"relative_to",""}};
    }
    internal static bool NodeHasReminder(TreeNode node) {
        var task=node==null?null:node as TaskNode;if(task!=null)return task.ReminderCount>0;
        var leaf=node==null?null:node.Tag as OutstandingLeaf;return leaf!=null&&!String.IsNullOrWhiteSpace(leaf.ReminderAt);
    }

    void InitializeReminders() {
        try {if(File.Exists(ReminderStatePath))reminderState=json.Deserialize<ReminderState>(File.ReadAllText(ReminderStatePath));}catch { }
        if(reminderState==null)reminderState=new ReminderState();
        if(reminderState.Dismissed==null)reminderState.Dismissed=new Dictionary<string,string>();
        if(reminderState.SnoozedUntil==null)reminderState.SnoozedUntil=new Dictionary<string,string>();
        reminderTimer.Tick+=delegate {CheckDueReminders();};
        Shown+=delegate {reminderTimer.Start();};
    }
    void DisposeReminders(){reminderTimer.Stop();reminderTimer.Dispose();if(reminderPopup!=null&&!reminderPopup.IsDisposed)reminderPopup.Close();}
    void SaveReminderState(){try{File.WriteAllText(ReminderStatePath,json.Serialize(reminderState));}catch { }}

    void UpdateReminderTargets(Dictionary<long,Dictionary<string,object>> all,Dictionary<long,SharedList> sharedLists) {
        var next=new Dictionary<string,ReminderTarget>();var owned=new Dictionary<long,HashSet<string>>();
        if(sharedLists!=null)foreach(var pair in sharedLists)foreach(var item in pair.Value.Items) {
            DateTimeOffset due;if(String.IsNullOrWhiteSpace(item.ReminderAt)||!TryReminderTime(item.ReminderAt,out due))continue;
            HashSet<string> values;if(!owned.TryGetValue(pair.Key,out values)){values=new HashSet<string>();owned[pair.Key]=values;}values.Add(ReminderMomentKey(due));
            string key="outstanding:"+pair.Key+":"+item.Id+":"+ReminderMomentKey(due);
            next[key]=new ReminderTarget{Key=key,TaskId=pair.Key,ItemId=item.Id,Kind="遗留事项",Title=OutstandingText(item.Html),Due=due};
        }
        if(all!=null)foreach(var pair in all)foreach(var due in TaskReminderTimes(pair.Value)) {
            HashSet<string> values;if(owned.TryGetValue(pair.Key,out values)&&values.Contains(ReminderMomentKey(due)))continue;
            string key="task:"+pair.Key+":"+ReminderMomentKey(due);
            next[key]=new ReminderTarget{Key=key,TaskId=pair.Key,Kind="任务",Title=Convert.ToString(pair.Value["title"]),Due=due};
        }
        reminderTargets.Clear();foreach(var pair in next)reminderTargets[pair.Key]=pair.Value;
        CheckDueReminders();
    }
    void CheckDueReminders() {
        if(closing||IsDisposed||reminderPopup!=null||reminderTargets.Count==0)return;
        DateTimeOffset now=DateTimeOffset.Now;ReminderTarget dueTarget=null;
        foreach(var target in reminderTargets.Values.OrderBy(value=>value.Due)) {
            if(reminderState.Dismissed.ContainsKey(target.Key))continue;
            string snooze;DateTimeOffset snoozed;
            if(reminderState.SnoozedUntil.TryGetValue(target.Key,out snooze)&&TryReminderTime(snooze,out snoozed)) {
                if(snoozed>now)continue;
            } else if(target.Due>now || target.Due<now.AddDays(-30))continue;
            dueTarget=target;break;
        }
        if(dueTarget!=null)ShowReminderPopup(dueTarget);
    }
    void ShowReminderPopup(ReminderTarget target) {
        var dialog=DpiDialog(new Form{Text="TaskTrace 提醒",Size=new Size(440,245),MinimumSize=new Size(390,225),Font=Font,TopMost=true,StartPosition=FormStartPosition.CenterScreen,ShowInTaskbar=false});reminderPopup=dialog;
        var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(18),ColumnCount=1,RowCount=4};
        layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,34));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,28));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,42));
        var message=new Label{Text=target.Kind+"提醒\r\n"+target.Title+"\r\n提醒时间："+target.Due.LocalDateTime.ToString("yyyy-MM-dd HH:mm"),Dock=DockStyle.Fill,AutoEllipsis=true,TextAlign=ContentAlignment.MiddleLeft};
        var minutes=new NumericUpDown{Minimum=1,Maximum=10080,Value=15,Dock=DockStyle.Left,Width=110};
        var delayRow=new FlowLayoutPanel{Dock=DockStyle.Fill,WrapContents=false,FlowDirection=FlowDirection.LeftToRight};delayRow.Controls.Add(new Label{Text="延迟分钟数",AutoSize=true,Margin=new Padding(0,7,8,0)});delayRow.Controls.Add(minutes);
        var hint=new Label{Text="关闭后不再显示本次提醒；延迟后会按上面的分钟数再次提醒。",Dock=DockStyle.Fill,ForeColor=Color.DimGray};
        var actions=new FlowLayoutPanel{Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};var close=new Button{Text="关闭",Width=88,Height=30};var delay=new Button{Text="延迟",Width=88,Height=30};actions.Controls.Add(close);actions.Controls.Add(delay);
        layout.Controls.Add(message);layout.Controls.Add(delayRow);layout.Controls.Add(hint);layout.Controls.Add(actions);dialog.Controls.Add(layout);
        bool handled=false;
        close.Click+=delegate {handled=true;reminderState.Dismissed[target.Key]=DateTimeOffset.UtcNow.ToString("o");reminderState.SnoozedUntil.Remove(target.Key);SaveReminderState();dialog.Close();};
        delay.Click+=delegate {handled=true;reminderState.SnoozedUntil[target.Key]=DateTimeOffset.UtcNow.AddMinutes((double)minutes.Value).ToString("o");SaveReminderState();dialog.Close();};
        dialog.FormClosing+=delegate {if(!handled){reminderState.SnoozedUntil[target.Key]=DateTimeOffset.UtcNow.AddMinutes(15).ToString("o");SaveReminderState();}};
        dialog.FormClosed+=delegate {reminderPopup=null;dialog.Dispose();BeginInvoke(new Action(CheckDueReminders));};dialog.Show();dialog.Activate();
    }

    async void ShowReminderForSelected() {
        var node=tasks.SelectedNode;if(node==null||(!(node.Tag is long)&&!(node.Tag is OutstandingLeaf))){status.Text="请先选中一个任务或遗留事项。";return;}
        await ShowReminderForNode(node,this);
    }
    async Task<bool> ShowReminderForNode(TreeNode node,Form owner) {
        try {
            if(node==null)return false;var leaf=node.Tag as OutstandingLeaf;long taskId=leaf==null?Convert.ToInt64(node.Tag):leaf.TaskId;
            bool changed=await ShowReminderEditor(taskId,leaf==null?null:leaf.Id,owner);
            if(changed){await LoadTasks();status.Text="提醒已同步到网页模式。";}
            return changed;
        } catch(Exception error){Error(error);return false;}
    }
    async Task<bool> ShowReminderEditor(long taskId,string itemId,Form owner) {
        var task=await Api("GET","/tasks/"+taskId,null);var values=new List<Dictionary<string,object>>();string title=Convert.ToString(task["title"]);SharedList shared=null;PendingItem outstanding=null;
        if(itemId==null)values=TaskReminderValues(task);
        else {shared=ReadShared(await ReadHistory(taskId));outstanding=shared.Items.FirstOrDefault(item=>item.Id==itemId);if(outstanding==null)throw new Exception("这条遗留事项已被移动或删除，请刷新后重试。");title=OutstandingText(outstanding.Html);DateTimeOffset existing;if(TryReminderTime(outstanding.ReminderAt,out existing))values.Add(AbsoluteReminder(existing.LocalDateTime));}
        bool saved=false;
        using(var dialog=DpiDialog(new Form{Text="设置提醒 · "+title,Size=new Size(480,390),MinimumSize=new Size(420,350),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=7};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,42));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,24));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,42));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,30));
            var explanation=new Label{Text=itemId==null?"与网页模式共用同一组提醒。可添加多个提醒；选中后可修改或移除。":"遗留事项使用一个提醒，并同步到所属任务的网页提醒。",Dock=DockStyle.Fill,AutoEllipsis=true};
            var list=new ListBox{Dock=DockStyle.Fill,IntegralHeight=false,AccessibleName="已设置的提醒"};
            var fieldLabel=new Label{Text="提醒日期与时间",Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft};
            var fields=new TableLayoutPanel{Dock=DockStyle.Fill,ColumnCount=2,RowCount=1,Margin=Padding.Empty};fields.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,62));fields.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,38));
            var date=new DateTimePicker{Dock=DockStyle.Fill,Format=DateTimePickerFormat.Long};var time=new DateTimePicker{Dock=DockStyle.Fill,Format=DateTimePickerFormat.Custom,CustomFormat="HH:mm",ShowUpDown=true};fields.Controls.Add(date,0,0);fields.Controls.Add(time,1,0);
            var editActions=new FlowLayoutPanel{Dock=DockStyle.Fill,WrapContents=false};var add=new Button{Text=itemId==null?"添加提醒":"设置提醒",AutoSize=true};var remove=new Button{Text="移除选中",AutoSize=true,Enabled=false};var fresh=new Button{Text="新增一条",AutoSize=true,Visible=itemId==null};editActions.Controls.Add(add);editActions.Controls.Add(remove);editActions.Controls.Add(fresh);
            var commitActions=new FlowLayoutPanel{Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};var commit=new Button{Text="保存",Width=92};var cancel=new Button{Text="取消",Width=92};commitActions.Controls.Add(commit);commitActions.Controls.Add(cancel);
            var feedback=new Label{Text="尚未修改。",Dock=DockStyle.Fill,ForeColor=Color.DimGray};
            layout.Controls.Add(explanation);layout.Controls.Add(list);layout.Controls.Add(fieldLabel);layout.Controls.Add(fields);layout.Controls.Add(editActions);layout.Controls.Add(commitActions);layout.Controls.Add(feedback);dialog.Controls.Add(layout);
            Action refreshList=delegate {list.BeginUpdate();list.Items.Clear();foreach(var value in values){object raw;DateTimeOffset due;if(value.TryGetValue("reminder",out raw)&&TryReminderTime(raw,out due))list.Items.Add(new ReminderChoice{Value=value,Due=due});}list.EndUpdate();remove.Enabled=list.SelectedIndex>=0;};
            DateTime initial=DateTime.Now.AddHours(1);initial=new DateTime(initial.Year,initial.Month,initial.Day,initial.Hour,initial.Minute,0);date.Value=initial.Date;time.Value=initial;
            list.SelectedIndexChanged+=delegate {remove.Enabled=list.SelectedIndex>=0;var choice=list.SelectedItem as ReminderChoice;if(choice!=null){date.Value=choice.Due.LocalDateTime.Date;time.Value=choice.Due.LocalDateTime;}};
            fresh.Click+=delegate {list.ClearSelected();DateTime next=DateTime.Now.AddHours(1);date.Value=next.Date;time.Value=next;feedback.Text="填写时间后点击添加提醒。";};
            add.Click+=delegate {DateTime selected=date.Value.Date.Add(time.Value.TimeOfDay);var replacement=AbsoluteReminder(selected);int index=list.SelectedIndex;if(itemId!=null){values.Clear();values.Add(replacement);}else if(index>=0&&index<values.Count)values[index]=replacement;else values.Add(replacement);refreshList();if(values.Count>0)list.SelectedIndex=Math.Max(0,Math.Min(index<0?values.Count-1:index,values.Count-1));feedback.Text="提醒时间已加入，点击保存后同步。";};
            remove.Click+=delegate {int index=list.SelectedIndex;if(index<0||index>=values.Count)return;values.RemoveAt(index);refreshList();feedback.Text="已移除，点击保存后同步。";};
            cancel.Click+=delegate {dialog.Close();};
            bool writing=false;
            commit.Click+=async delegate {
                if(writing)return;writing=true;commit.Enabled=false;editActions.Enabled=false;feedback.Text="正在同步提醒…";
                try {
                    if(itemId==null)await Api("PATCH","/tasks/"+taskId,new Dictionary<string,object>{{"reminders",values}});
                    else {
                        string oldValue=outstanding.ReminderAt;DateTimeOffset oldDue=DateTimeOffset.MinValue,newDue=DateTimeOffset.MinValue;bool hadOld=TryReminderTime(oldValue,out oldDue);bool hasNew=values.Count>0&&TryReminderTime(values[0]["reminder"],out newDue);
                        outstanding.ReminderAt=hasNew?ReminderStamp(newDue):null;await WriteShared(taskId,shared);
                        Exception syncFailure=null;
                        try {
                            var taskValues=TaskReminderValues(task);
                            if(hadOld&&!shared.Items.Any(item=>item.Id!=itemId&&SameReminderTime(item.ReminderAt,oldDue))) {var match=taskValues.FindIndex(value=>SameReminderTime(value,oldDue));if(match>=0)taskValues.RemoveAt(match);}
                            if(hasNew&&!taskValues.Any(value=>SameReminderTime(value,newDue)))taskValues.Add(AbsoluteReminder(newDue.LocalDateTime));
                            await Api("PATCH","/tasks/"+taskId,new Dictionary<string,object>{{"reminders",taskValues}});
                        } catch(Exception error){syncFailure=error;}
                        if(syncFailure!=null){outstanding.ReminderAt=oldValue;await WriteShared(taskId,shared);throw syncFailure;}
                    }
                    saved=true;dialog.Close();
                } catch(Exception error){feedback.Text="提醒未保存："+error.Message;}
                finally{writing=false;if(!dialog.IsDisposed){commit.Enabled=true;editActions.Enabled=true;}}
            };
            refreshList();if(list.Items.Count>0)list.SelectedIndex=0;
            dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing)e.Cancel=true;};dialog.ShowDialog(owner??this);
        }
        return saved;
    }
}
