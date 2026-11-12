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
        public FloatingReminderRepeatRule RepeatRule;
    }
    sealed class ReminderState {
        public Dictionary<string,string> Dismissed {get;set;}
        public Dictionary<string,string> SnoozedUntil {get;set;}
        public ReminderState(){Dismissed=new Dictionary<string,string>();SnoozedUntil=new Dictionary<string,string>();}
    }
    sealed class ReminderChoice {
        public Dictionary<string,object> Value;
        public DateTimeOffset Due;
        public override string ToString(){var rule=FloatingReminderRepeatRule.Parse(RepeatValue(Value));if(rule!=null)return rule.ToString();return Due.LocalDateTime.ToString("yyyy-MM-dd HH:mm")+(Convert.ToString(Value.ContainsKey("relative_to")?Value["relative_to"]:"").Length>0?" · 网页相对提醒":"");}
    }

    readonly Timer reminderTimer=new Timer{Interval=15000};
    readonly Dictionary<string,ReminderTarget> reminderTargets=new Dictionary<string,ReminderTarget>();
    readonly System.Threading.SemaphoreSlim reminderMutationGate=new System.Threading.SemaphoreSlim(1,1);
    ReminderState reminderState;
    Form reminderPopup;
    bool dismissedReminderCleanupRunning;

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
        if(shared!=null)foreach(var item in shared.Items){DateTimeOffset due;if(TryReminderTime(item.ReminderAt,out due))owned.Add(ReminderIdentity(due,item.RepeatRule));}
        return TaskReminderValues(task).Count(value=>{DateTimeOffset due;return TryReminderTime(value["reminder"],out due)&&!owned.Contains(ReminderIdentity(due,RepeatValue(value)));});
    }
    static string ReminderStamp(DateTimeOffset value){return value.ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ss.fffZ",CultureInfo.InvariantCulture);}
    static string ReminderMomentKey(DateTimeOffset value){return value.ToUniversalTime().ToString("yyyyMMddHHmmssfff",CultureInfo.InvariantCulture);}
    static bool SameReminderTime(Dictionary<string,object> value,DateTimeOffset target) {
        object raw;DateTimeOffset parsed;return value!=null&&value.TryGetValue("reminder",out raw)&&TryReminderTime(raw,out parsed)&&Math.Abs((parsed.ToUniversalTime()-target.ToUniversalTime()).TotalSeconds)<1;
    }
    static bool SameReminderTime(string value,DateTimeOffset target) {
        DateTimeOffset parsed;return TryReminderTime(value,out parsed)&&Math.Abs((parsed.ToUniversalTime()-target.ToUniversalTime()).TotalSeconds)<1;
    }
    static bool SameReminderValue(string left,string right) {
        if(String.IsNullOrWhiteSpace(left)||String.IsNullOrWhiteSpace(right))return String.IsNullOrWhiteSpace(left)&&String.IsNullOrWhiteSpace(right);
        DateTimeOffset target;return TryReminderTime(right,out target)&&SameReminderTime(left,target);
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
        reminderTimer.Tick+=async delegate {CheckDueReminders();await RefreshReminderSnapshot();};
        Shown+=async delegate {reminderTimer.Start();await RefreshReminderSnapshot();};
    }
    void DisposeReminders(){reminderTimer.Stop();reminderTimer.Dispose();if(reminderPopup!=null&&!reminderPopup.IsDisposed)reminderPopup.Close();}
    void SaveReminderState(){try{File.WriteAllText(ReminderStatePath,json.Serialize(reminderState));}catch { }}

    void UpdateReminderTargets(Dictionary<long,Dictionary<string,object>> all,Dictionary<long,SharedList> sharedLists) {
        var next=new Dictionary<string,ReminderTarget>();var owned=new Dictionary<long,HashSet<string>>();repeatingReminderTargets.Clear();
        if(sharedLists!=null)foreach(var pair in sharedLists)foreach(var item in pair.Value.Items) {
            DateTimeOffset due;if(!TryReminderTime(item.ReminderAt,out due))continue;
            HashSet<string> values;if(!owned.TryGetValue(pair.Key,out values)){values=new HashSet<string>();owned[pair.Key]=values;}values.Add(ReminderIdentity(due,item.RepeatRule));
            AddReminderDefinition(next,"outstanding:"+pair.Key+":"+item.Id,pair.Key,item.Id,OutstandingText(item.Html,item.NoteHtml,url),"遗留事项",due,item.RepeatRule);
        }
        if(all!=null)foreach(var pair in all)foreach(var value in TaskReminderValues(pair.Value)) {
            DateTimeOffset due;if(!TryReminderTime(value["reminder"],out due))continue;string rule=RepeatValue(value);
            HashSet<string> values;if(owned.TryGetValue(pair.Key,out values)&&values.Contains(ReminderIdentity(due,rule)))continue;
            AddReminderDefinition(next,"task:"+pair.Key,pair.Key,null,Convert.ToString(pair.Value["title"]),"任务",due,rule);
        }
        reminderTargets.Clear();foreach(var pair in next)reminderTargets[pair.Key]=pair.Value;
        CleanupDismissedReminderTargets();
        CheckDueReminders();
    }
    async void CleanupDismissedReminderTargets() {
        if(dismissedReminderCleanupRunning||closing||reminderState==null)return;
        var pending=reminderTargets.Values.Where(target=>target.RepeatRule==null&&reminderState.Dismissed.ContainsKey(target.Key)).ToList();
        if(pending.Count==0)return;
        dismissedReminderCleanupRunning=true;bool changed=false;
        try {
            foreach(var target in pending) {
                try {
                    await DeleteReminderTarget(target);
                    reminderTargets.Remove(target.Key);InvalidateReminderSnapshot(target.TaskId);
                    reminderState.Dismissed.Remove(target.Key);reminderState.SnoozedUntil.Remove(target.Key);changed=true;
                } catch(Exception error) {
                    if(!closing&&!IsDisposed){status.ForeColor=Color.Firebrick;status.Text="提醒已关闭，删除同步将在后台重试："+error.Message;}
                }
            }
            if(changed){SaveReminderState();try{await LoadTasks();}catch { }}
        } finally {dismissedReminderCleanupRunning=false;}
    }
    void CheckDueReminders() {
        if(closing || IsDisposed || reminderPopup!=null || reminderState==null)return;
        DateTimeOffset now=DateTimeOffset.Now;
        ExpandRepeatingReminderTargets(now);
        foreach(var target in reminderTargets.Values.OrderBy(value=>value.Due)) {
            if(reminderState.Dismissed.ContainsKey(target.Key))continue;
            DateTimeOffset reminderTime=target.Due;
            string snooze;
            DateTimeOffset snoozed;
            if(reminderState.SnoozedUntil.TryGetValue(target.Key,out snooze) && TryReminderTime(snooze,out snoozed)) {
                reminderTime=snoozed;
            } else if(reminderTime<now.AddDays(-30))continue;
            if(reminderTime>now)continue;
            ShowReminderPopup(target,reminderTime);
            break;
        }
    }

    void ShowReminderPopup(ReminderTarget target,DateTimeOffset reminderTime) {
        const int defaultMinutes=15;
        const int maxHours=168;
        const string dateFormat="yyyy-MM-dd HH:mm";
        var dialog=DpiDialog(new Form {
            Text="TaskTrace 提醒", ClientSize=new Size(440,248), MinimumSize=new Size(410,270), Font=Font,
            TopMost=true, StartPosition=FormStartPosition.CenterScreen, ShowInTaskbar=false
        });
        reminderPopup=dialog;
        var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(18),ColumnCount=1,RowCount=6};
        layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
        foreach(int height in new[]{26,34,28,27,42})layout.RowStyles.Add(new RowStyle(SizeType.Absolute,height));

        var message=new Label {
            Text=target.Kind+"提醒\r\n"+target.Title, Dock=DockStyle.Fill,
            AutoEllipsis=true, TextAlign=ContentAlignment.MiddleLeft
        };
        var time=new Label {Text="本次提醒时间 "+reminderTime.LocalDateTime.ToString(dateFormat),Dock=DockStyle.Fill};
        var hours=new NumericUpDown {Minimum=0,Maximum=maxHours,Value=0,Width=70,AccessibleName="延迟小时数"};
        var minutes=new NumericUpDown {Minimum=0,Maximum=59,Value=defaultMinutes,Width=70,AccessibleName="延迟分钟数"};
        var delayRow=new FlowLayoutPanel {Dock=DockStyle.Fill,WrapContents=false,FlowDirection=FlowDirection.LeftToRight};
        delayRow.Controls.Add(new Label {Text="延迟时长",AutoSize=true,Margin=new Padding(0,7,8,0)});
        delayRow.Controls.Add(hours);
        delayRow.Controls.Add(new Label {Text="小时",AutoSize=true,Margin=new Padding(2,7,8,0)});
        delayRow.Controls.Add(minutes);
        delayRow.Controls.Add(new Label {Text="分钟",AutoSize=true,Margin=new Padding(2,7,0,0)});
        var preview=new Label {Dock=DockStyle.Fill};
        var hint=new Label {
            Text=target.RepeatRule==null?"关闭会删除本次提醒。":"关闭仅结束本次提醒，后续日期继续。",
            Dock=DockStyle.Fill, ForeColor=Color.DimGray
        };
        var actions=new FlowLayoutPanel {Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};
        var close=new Button {Text="关闭",Width=88,Height=30,CausesValidation=false};
        var delay=new Button {Text="延迟",Width=88,Height=30};
        actions.Controls.Add(close);
        actions.Controls.Add(delay);
        layout.Controls.Add(message);
        layout.Controls.Add(time);
        layout.Controls.Add(delayRow);
        layout.Controls.Add(preview);
        layout.Controls.Add(hint);
        layout.Controls.Add(actions);
        dialog.Controls.Add(layout);

        Func<DateTimeOffset?> nextReminder=delegate {
            int hourCount, minuteCount;
            if(!Int32.TryParse(hours.Text,out hourCount) || hourCount<0 || hourCount>maxHours ||
                !Int32.TryParse(minutes.Text,out minuteCount) || minuteCount<0 || minuteCount>59)return null;
            int delayMinutes=hourCount*60+minuteCount;
            if(delayMinutes==0 || delayMinutes>maxHours*60)return null;
            return DateTimeOffset.UtcNow.AddMinutes(delayMinutes);
        };
        Action updatePreview=delegate {
            var next=nextReminder();
            delay.Enabled=next.HasValue;
            preview.ForeColor=next.HasValue?ForeColor:Color.Firebrick;
            preview.Text=next.HasValue?"预计再次提醒 "+next.Value.LocalDateTime.ToString(dateFormat):"请输入 1 分钟至 168 小时的延迟时长。";
        };
        var previewTimer=new Timer {Interval=1000};
        hours.TextChanged+=delegate {updatePreview();};
        minutes.TextChanged+=delegate {updatePreview();};
        previewTimer.Tick+=delegate {updatePreview();};

        bool handled=false;
        Action<DateTimeOffset> snooze=delegate(DateTimeOffset next) {
            reminderState.SnoozedUntil[target.Key]=ReminderStamp(next);
            SaveReminderState();
        };
        close.Click+=delegate {
            handled=true;
            // Keep the original occurrence for dismissal and deletion after a delay.
            reminderState.Dismissed[target.Key]=ReminderStamp(target.Due);
            reminderState.SnoozedUntil.Remove(target.Key);
            SaveReminderState();
            dialog.Close();
            if(!closing && !IsDisposed)BeginInvoke(new Action(CleanupDismissedReminderTargets));
        };
        delay.Click+=delegate {
            var next=nextReminder();
            if(!next.HasValue)return;
            handled=true;
            snooze(next.Value);
            dialog.Close();
        };
        dialog.FormClosing+=delegate {if(!handled)snooze(DateTimeOffset.UtcNow.AddMinutes(defaultMinutes));};
        dialog.FormClosed+=delegate {
            previewTimer.Dispose();
            reminderPopup=null;
            if(!closing && !IsDisposed)BeginInvoke(new Action(CheckDueReminders));
        };
        updatePreview();
        dialog.Show();
        previewTimer.Start();
        dialog.Activate();
    }

    async Task PatchTaskReminders(long taskId,List<Dictionary<string,object>> values) {
        Exception failure=null;
        for(int attempt=0;attempt<3;attempt++) {
            try {await Api("PATCH","/tasks/"+taskId,new Dictionary<string,object>{{"reminders",values}});InvalidateReminderSnapshot(taskId);return;}
            catch(Exception error) {failure=error;}
            if(attempt<2)await Task.Delay(150*(attempt+1));
        }
        throw failure??new Exception("提醒保存失败，请重试。");
    }

    async Task SaveOutstandingReminderCore(long taskId,string itemId,string reminderAt,string repeatRule=null) {
        var task=await Api("GET","/tasks/"+taskId,null);
        var shared=ReadShared(await ReadHistory(taskId));
        var outstanding=shared.Items.FirstOrDefault(item=>item.Id==itemId);
        if(outstanding==null)throw new Exception("这条遗留事项已被移动或删除，请刷新后重试。");
        string oldValue=outstanding.ReminderAt,oldRule=outstanding.RepeatRule;
        if(!String.IsNullOrWhiteSpace(repeatRule)&&FloatingReminderRepeatRule.Parse(repeatRule)==null)throw new Exception("周期提醒格式无效。");
        DateTimeOffset oldDue=DateTimeOffset.MinValue,newDue=DateTimeOffset.MinValue;
        bool hadOld=TryReminderTime(oldValue,out oldDue),hasNew=TryReminderTime(reminderAt,out newDue);
        string desiredValue=hasNew?ReminderStamp(newDue):null;
        outstanding.ReminderAt=desiredValue;outstanding.RepeatRule=repeatRule;
        Exception writeFailure=null;
        for(int attempt=0;attempt<3;attempt++) {
            writeFailure=null;
            try {await WriteShared(taskId,shared);}
            catch(Exception error) {writeFailure=error;}
            if(writeFailure==null)break;
            // A request can reach the server even when its response is interrupted. Re-read before
            // retrying so a successful POST is not duplicated and unrelated list edits are retained.
            shared=ReadShared(await ReadHistory(taskId));
            outstanding=shared.Items.FirstOrDefault(item=>item.Id==itemId);
            if(outstanding==null)throw new Exception("这条遗留事项已被移动或删除，请刷新后重试。");
            if(SameReminderValue(outstanding.ReminderAt,desiredValue)&&(outstanding.RepeatRule??"")==(repeatRule??"")){writeFailure=null;break;}
            outstanding.ReminderAt=desiredValue;outstanding.RepeatRule=repeatRule;
            if(attempt<2)await Task.Delay(150*(attempt+1));
        }
        if(writeFailure!=null)throw writeFailure;
        Exception syncFailure=null;
        try {
            var taskValues=TaskReminderValues(task);
            if(hadOld&&!shared.Items.Any(item=>item.Id!=itemId&&SameReminderTime(item.ReminderAt,oldDue))) {
                int match=taskValues.FindIndex(value=>SameReminderTime(value,oldDue)&&RepeatValue(value)==(oldRule??""));
                if(match>=0)taskValues.RemoveAt(match);
            }
            // Outstanding reminders are now edited on their own web row; do not create a second task reminder.
            if(taskValues.Count!=TaskReminderValues(task).Count)await PatchTaskReminders(taskId,taskValues);
        } catch(Exception error) {syncFailure=error;}
        if(syncFailure!=null) {
            // Restore only the field changed by this operation, using the newest shared list so
            // unrelated outstanding edits made while the request was in flight are preserved.
            SharedList rollback=null;
            try {
                rollback=ReadShared(await ReadHistory(taskId));
                var current=rollback.Items.FirstOrDefault(item=>item.Id==itemId);
                if(current!=null) {
                    if(!SameReminderValue(current.ReminderAt,desiredValue)||(current.RepeatRule??"")!=(repeatRule??""))throw syncFailure;
                    current.ReminderAt=oldValue;current.RepeatRule=oldRule;
                    await WriteShared(taskId,rollback);
                }
            } catch { }
            throw syncFailure;
        }
    }

    async Task SaveOutstandingReminder(long taskId,string itemId,string reminderAt,string repeatRule=null) {
        await reminderMutationGate.WaitAsync();
        try {await SaveOutstandingReminderCore(taskId,itemId,reminderAt,repeatRule);}
        finally {reminderMutationGate.Release();}
    }

    async Task DeleteReminderTarget(ReminderTarget target) {
        if(target.RepeatRule!=null)return;
        await reminderMutationGate.WaitAsync();
        try {
            if(!String.IsNullOrWhiteSpace(target.ItemId)) {
                var shared=ReadShared(await ReadHistory(target.TaskId));
                var item=shared.Items.FirstOrDefault(value=>value.Id==target.ItemId);
                // A newer reminder replaced the one shown in this popup. Do not delete it.
                if(item==null||!String.IsNullOrWhiteSpace(item.RepeatRule)||!SameReminderTime(item.ReminderAt,target.Due))return;
                await SaveOutstandingReminderCore(target.TaskId,target.ItemId,null);
                return;
            }
            var task=await Api("GET","/tasks/"+target.TaskId,null);
            var values=TaskReminderValues(task);
            int index=values.FindIndex(value=>SameReminderTime(value,target.Due)&&String.IsNullOrWhiteSpace(RepeatValue(value)));
            if(index<0)return;
            values.RemoveAt(index);
            await PatchTaskReminders(target.TaskId,values);
        } finally {reminderMutationGate.Release();}
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
        else {shared=ReadShared(await ReadHistory(taskId));outstanding=shared.Items.FirstOrDefault(item=>item.Id==itemId);if(outstanding==null)throw new Exception("这条遗留事项已被移动或删除，请刷新后重试。");title=OutstandingText(outstanding.Html,outstanding.NoteHtml,url);DateTimeOffset existing;if(TryReminderTime(outstanding.ReminderAt,out existing)){var reminder=AbsoluteReminder(existing.LocalDateTime);reminder["repeat_rule"]=outstanding.RepeatRule??"";values.Add(reminder);}}
        bool saved=false;
        using(var dialog=DpiDialog(new Form{Text="设置提醒 · "+title,Size=new Size(670,640),MinimumSize=new Size(580,590),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=8};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,42));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,24));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.AutoSize));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,30));
            var explanation=new Label{Text=itemId==null?"选中一条可修改；直接保存当前设置。添加多个提醒时，可先加入列表再新增。":"设置单次或每周提醒，点击保存后与网页同步。",Dock=DockStyle.Fill,AutoEllipsis=true};
            var list=new ListBox{Dock=DockStyle.Fill,IntegralHeight=false,HorizontalScrollbar=true,AccessibleName="已设置的提醒"};
            var fieldLabel=new Label{Text="提醒日期与时间",Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft};
            var fields=new TableLayoutPanel{Dock=DockStyle.Fill,ColumnCount=2,RowCount=1,Margin=Padding.Empty};fields.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,62));fields.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,38));
            var date=new DateTimePicker{Dock=DockStyle.Fill,Format=DateTimePickerFormat.Long,AccessibleName="提醒日期或周期开始日期"};
            var time=new DateTimePicker{Dock=DockStyle.Fill,Format=DateTimePickerFormat.Custom,CustomFormat="HH:mm",ShowUpDown=true,AccessibleName="提醒时间或每周默认时间"};
            fields.Controls.Add(date,0,0);fields.Controls.Add(time,1,0);
            var repeatFields=new FloatingReminderRepeatFields();
            var editActions=new FlowLayoutPanel{Dock=DockStyle.Fill,AutoSize=true,AutoSizeMode=AutoSizeMode.GrowAndShrink,WrapContents=false};var add=new Button{Text=itemId==null?"添加提醒":"设置提醒",AutoSize=true};var remove=new Button{Text="移除选中",AutoSize=true,Enabled=false};var fresh=new Button{Text="新增一条",AutoSize=true,Visible=itemId==null};editActions.Controls.Add(add);editActions.Controls.Add(remove);editActions.Controls.Add(fresh);
            var commitActions=new FlowLayoutPanel{Dock=DockStyle.Fill,AutoSize=true,AutoSizeMode=AutoSizeMode.GrowAndShrink,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};var commit=new Button{Text="保存",Width=92,Height=32,AutoSize=true};var cancel=new Button{Text="取消",Width=92,Height=32,AutoSize=true};commitActions.Controls.Add(commit);commitActions.Controls.Add(cancel);
            var feedback=new Label{Text="尚未修改。",Dock=DockStyle.Fill,ForeColor=Color.DimGray};
            layout.Controls.Add(explanation);layout.Controls.Add(list);layout.Controls.Add(fieldLabel);layout.Controls.Add(fields);layout.Controls.Add(repeatFields);layout.Controls.Add(editActions);layout.Controls.Add(commitActions);layout.Controls.Add(feedback);dialog.Controls.Add(layout);

            bool writing=false,listChanged=false,addingNew=false,fillingEditor=false;
            string loadedKey="";
            Func<string> editorKey=()=>date.Value.ToString("yyyy-MM-dd",CultureInfo.InvariantCulture)+"|"+time.Value.ToString("HH:mm",CultureInfo.InvariantCulture)+"|"+repeatFields.EditKey();
            Func<bool> editorChanged=()=>addingNew || editorKey()!=loadedKey;
            Action updateFeedback=delegate {
                if(!fillingEditor && !writing)feedback.Text=editorChanged() || listChanged?"有未保存的提醒设置，点击保存生效。":"尚未修改。";
            };
            Action<ReminderChoice> loadEditor=delegate(ReminderChoice choice) {
                fillingEditor=true;
                try {
                    var rule=choice==null?null:FloatingReminderRepeatRule.Parse(RepeatValue(choice.Value));
                    DateTime local=choice==null?DateTime.Now.AddHours(1):choice.Due.LocalDateTime;
                    date.Value=rule==null?local.Date:DateTime.ParseExact(rule.startDate,"yyyy-MM-dd",CultureInfo.InvariantCulture);
                    time.Value=rule==null?local:date.Value.Date.Add(DateTime.ParseExact(rule.time,"HH:mm",CultureInfo.InvariantCulture).TimeOfDay);
                    repeatFields.LoadRule(rule,date.Value.Date.Add(time.Value.TimeOfDay));
                    loadedKey=editorKey();addingNew=false;
                } finally {fillingEditor=false;}
            };
            Action<Dictionary<string,object>> refreshList=delegate(Dictionary<string,object> selection) {
                list.BeginUpdate();
                try {
                    list.Items.Clear();
                    foreach(var value in values) {
                        object raw;DateTimeOffset due;
                        if(!value.TryGetValue("reminder",out raw) || !TryReminderTime(raw,out due))continue;
                        int index=list.Items.Add(new ReminderChoice{Value=value,Due=due});
                        if(Object.ReferenceEquals(value,selection))list.SelectedIndex=index;
                    }
                } finally {list.EndUpdate();}
                remove.Enabled=list.SelectedIndex>=0;
            };
            list.SelectedIndexChanged+=delegate {
                var choice=list.SelectedItem as ReminderChoice;remove.Enabled=choice!=null;
                if(choice!=null)loadEditor(choice);
                updateFeedback();
            };
            date.ValueChanged+=delegate {updateFeedback();};
            time.ValueChanged+=delegate {repeatFields.SetDefaultTime(time.Value);updateFeedback();};
            repeatFields.Weekly.CheckedChanged+=delegate {
                fieldLabel.Text=repeatFields.Weekly.Checked?"周期开始日期（包含当天）与默认时间":"提醒日期与时间";
                updateFeedback();
            };
            repeatFields.End.ValueChanged+=delegate {updateFeedback();};
            foreach(var day in repeatFields.Days)day.CheckedChanged+=delegate {updateFeedback();};
            foreach(var clock in repeatFields.DayTimes)clock.ValueChanged+=delegate {updateFeedback();};

            Func<bool> stageEditor=delegate {
                try {
                    var rule=repeatFields.ReadRule(date.Value,time.Value);
                    var selected=rule==null?date.Value.Date.AddHours(time.Value.Hour).AddMinutes(time.Value.Minute):rule.FirstDate();
                    var replacement=AbsoluteReminder(selected);
                    replacement["repeat_rule"]=rule==null?"":rule.Serialize();
                    var choice=list.SelectedItem as ReminderChoice;
                    int index=choice==null?-1:values.IndexOf(choice.Value);
                    if(itemId!=null){values.Clear();values.Add(replacement);}
                    else if(index>=0)values[index]=replacement;
                    else values.Add(replacement);
                    listChanged=true;
                    refreshList(replacement);
                    updateFeedback();
                    return true;
                } catch(Exception error){feedback.Text=error.Message;return false;}
            };
            fresh.Click+=delegate {
                list.ClearSelected();loadEditor(null);addingNew=true;updateFeedback();
            };
            add.Click+=delegate {stageEditor();};
            remove.Click+=delegate {
                var choice=list.SelectedItem as ReminderChoice;
                if(choice==null || !values.Remove(choice.Value))return;
                listChanged=true;refreshList(null);loadEditor(null);updateFeedback();
            };
            cancel.Click+=delegate {dialog.Close();};
            commit.Click+=async delegate {
                if(writing)return;
                // Save applies the current form too; staging is only needed when working on multiple entries.
                if((editorChanged() || (!listChanged && values.Count==0)) && !stageEditor())return;
                if(!listChanged){dialog.Close();return;}
                writing=true;commit.Enabled=editActions.Enabled=fields.Enabled=repeatFields.Enabled=list.Enabled=cancel.Enabled=false;
                feedback.Text="正在同步提醒…";
                try {
                    if(itemId==null) {
                        await reminderMutationGate.WaitAsync();
                        try {await PatchTaskReminders(taskId,values);}finally{reminderMutationGate.Release();}
                    } else {
                        DateTimeOffset newDue;
                        string reminderAt=values.Count>0 && TryReminderTime(values[0]["reminder"],out newDue)?ReminderStamp(newDue):null;
                        await SaveOutstandingReminder(taskId,itemId,reminderAt,values.Count>0?RepeatValue(values[0]):null);
                    }
                    InvalidateReminderSnapshot(taskId);saved=true;writing=false;dialog.Close();
                } catch(Exception error){feedback.Text="提醒未保存："+error.Message;}
                finally {
                    writing=false;
                    if(!dialog.IsDisposed){commit.Enabled=editActions.Enabled=fields.Enabled=repeatFields.Enabled=list.Enabled=cancel.Enabled=true;}
                }
            };
            loadEditor(null);refreshList(values.FirstOrDefault());
            dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing)e.Cancel=true;};
            dialog.ShowDialog(owner??this);
        }
        return saved;
    }
}
