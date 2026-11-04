
// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Drawing;
using System.Globalization;
using System.Linq;
using System.Web.Script.Serialization;
using System.Windows.Forms;
using System.Collections;
using System.Threading.Tasks;

internal sealed class FloatingReminderRepeatRule {
    public int[] weekdays {get;set;}
    public string startDate {get;set;}
    public string endDate {get;set;}
    public string time {get;set;}
    public Dictionary<string,string> times {get;set;}
    internal static readonly string[] DayNames={"周日","周一","周二","周三","周四","周五","周六"};
    internal static readonly int[] DisplayOrder={1,2,3,4,5,6,0};

    internal static FloatingReminderRepeatRule Parse(string raw) {
        try {var rule=new JavaScriptSerializer().Deserialize<FloatingReminderRepeatRule>(raw);rule.Validate();return rule;} catch {return null;}
    }

    internal void Validate() {
        DateTime start,end,clock;
        if(weekdays==null || weekdays.Length==0 || weekdays.Any(day=>day<0 || day>6))throw new Exception("请至少选择一个星期。");
        if(!DateTime.TryParseExact(startDate,"yyyy-MM-dd",CultureInfo.InvariantCulture,DateTimeStyles.None,out start) ||
            !DateTime.TryParseExact(endDate,"yyyy-MM-dd",CultureInfo.InvariantCulture,DateTimeStyles.None,out end))throw new Exception("请选择有效的开始和结束日期。");
        if(end<start)throw new Exception("结束日期不能早于开始日期。");
        if(!DateTime.TryParseExact(time,"HH:mm",CultureInfo.InvariantCulture,DateTimeStyles.None,out clock))throw new Exception("请选择有效的提醒时间。");
        weekdays=weekdays.Distinct().OrderBy(day=>day).ToArray();
        if(times!=null) {
            var normalized=new Dictionary<string,string>();
            foreach(var entry in times.OrderBy(entry=>entry.Key,StringComparer.Ordinal)) {
                int day;
                if(!Int32.TryParse(entry.Key,NumberStyles.None,CultureInfo.InvariantCulture,out day) ||
                    entry.Key!=DayKey(day) || !weekdays.Contains(day) ||
                    !DateTime.TryParseExact(entry.Value,"HH:mm",CultureInfo.InvariantCulture,DateTimeStyles.None,out clock))
                    throw new Exception("请为选中的星期设置有效的提醒时间。");
                if(entry.Value!=time)normalized.Add(entry.Key,entry.Value);
            }
            times=normalized.Count==0?null:normalized;
        }
        if(!Enumerable.Range(0,7).Select(offset=>start.AddDays(offset)).Any(date=>date<=end && weekdays.Contains((int)date.DayOfWeek)))
            throw new Exception("日期范围内没有所选星期，请调整日期或星期。");
    }

    static string DayKey(int day){return day.ToString(CultureInfo.InvariantCulture);}
    internal string TimeForDay(int day){string value;return times!=null && times.TryGetValue(DayKey(day),out value)?value:time;}

    internal DateTime FirstDate() {
        Validate();
        DateTime start=DateTime.ParseExact(startDate,"yyyy-MM-dd",CultureInfo.InvariantCulture);
        for(int i=0;i<7;i++){DateTime due;if(OnDate(start.AddDays(i),out due))return due;}
        throw new Exception("日期范围内没有所选星期。");
    }

    internal bool OnDate(DateTime date,out DateTime due) {
        due=DateTime.MinValue;
        string day=date.ToString("yyyy-MM-dd",CultureInfo.InvariantCulture);
        int weekday=(int)date.DayOfWeek;
        if(String.CompareOrdinal(day,startDate)<0 || String.CompareOrdinal(day,endDate)>0 || !weekdays.Contains(weekday))return false;
        return DateTime.TryParseExact(day+" "+TimeForDay(weekday),"yyyy-MM-dd HH:mm",CultureInfo.InvariantCulture,DateTimeStyles.None,out due);
    }

    internal string Serialize() {
        Validate();
        // Keep the old representation unchanged for shared-time rules and their dismissal keys.
        var value=new Dictionary<string,object>{{"weekdays",weekdays},{"startDate",startDate},{"endDate",endDate},{"time",time}};
        if(times!=null)value["times"]=times;
        return new JavaScriptSerializer().Serialize(value);
    }

    public override string ToString() {
        var selected=DisplayOrder.Where(day=>weekdays.Contains(day));
        string schedule=times==null
            ?String.Join("、",selected.Select(day=>DayNames[day]))+" "+time
            :String.Join("、",selected.Select(day=>DayNames[day]+" "+TimeForDay(day)));
        return "每周 "+schedule+"（"+startDate+" 至 "+endDate+"）";
    }
}

internal sealed class FloatingReminderRepeatFields : TableLayoutPanel {
    internal readonly CheckBox Weekly=new CheckBox{Text="按周重复",AutoSize=true,Name="reminder-weekly"};
    internal readonly DateTimePicker End=new DateTimePicker{Format=DateTimePickerFormat.Custom,CustomFormat="yyyy-MM-dd",Name="reminder-end",Width=150,AccessibleName="结束日期"};
    internal readonly CheckBox[] Days=new CheckBox[7];
    internal readonly DateTimePicker[] DayTimes=new DateTimePicker[7];
    readonly bool[] customTime=new bool[7];
    readonly FlowLayoutPanel weekdaysRow=new FlowLayoutPanel{AutoSize=true,Dock=DockStyle.Fill,WrapContents=true};
    readonly FlowLayoutPanel endRow=new FlowLayoutPanel{AutoSize=true,Dock=DockStyle.Fill,WrapContents=true};
    readonly Label hint=new Label{Text="各星期可单独调整时间；未调整的沿用上方默认时间。按本机时间提醒。",AutoSize=true,Dock=DockStyle.Fill,ForeColor=SystemColors.GrayText,MaximumSize=new Size(540,0)};
    TimeSpan defaultTime;
    bool loading;

    internal FloatingReminderRepeatFields() {
        Dock=DockStyle.Fill;AutoSize=true;ColumnCount=1;RowCount=4;Margin=Padding.Empty;
        for(int i=0;i<4;i++)RowStyles.Add(new RowStyle(SizeType.AutoSize));
        foreach(int day in FloatingReminderRepeatRule.DisplayOrder) {
            int weekday=day;
            string name=FloatingReminderRepeatRule.DayNames[day];
            var group=new FlowLayoutPanel{AutoSize=true,AutoSizeMode=AutoSizeMode.GrowAndShrink,WrapContents=false,Margin=new Padding(0,0,8,4)};
            Days[day]=new CheckBox{Text=name,AutoSize=true,Name="reminder-day-"+day,Margin=new Padding(0,5,3,0)};
            DayTimes[day]=new DateTimePicker{Format=DateTimePickerFormat.Custom,CustomFormat="HH:mm",ShowUpDown=true,Width=78,Name="reminder-time-"+day,AccessibleName=name+"提醒时间",Margin=Padding.Empty};
            Days[day].CheckedChanged+=delegate {DayTimes[weekday].Enabled=Weekly.Checked && Days[weekday].Checked;};
            DayTimes[day].ValueChanged+=delegate {if(!loading)customTime[weekday]=Clock(DayTimes[weekday].Value)!=defaultTime;};
            group.Controls.Add(Days[day]);group.Controls.Add(DayTimes[day]);weekdaysRow.Controls.Add(group);
        }
        endRow.Controls.Add(new Label{Text="结束日期（包含当天）",AutoSize=true,Margin=new Padding(0,6,10,0)});
        endRow.Controls.Add(End);
        Controls.Add(Weekly);Controls.Add(weekdaysRow);Controls.Add(endRow);Controls.Add(hint);
        Weekly.CheckedChanged+=delegate{UpdateWeeklyFields();};
        LoadRule(null,DateTime.Now);
    }

    static TimeSpan Clock(DateTime value){return new TimeSpan(value.Hour,value.Minute,0);}
    void UpdateWeeklyFields() {
        weekdaysRow.Visible=endRow.Visible=hint.Visible=Weekly.Checked;
        for(int day=0;day<7;day++)DayTimes[day].Enabled=Weekly.Checked && Days[day].Checked;
    }

    internal void SetDefaultTime(DateTime clock) {
        defaultTime=Clock(clock);
        bool wasLoading=loading;loading=true;
        try {for(int day=0;day<7;day++)if(!customTime[day])DayTimes[day].Value=DateTime.Today.Add(defaultTime);}
        finally {loading=wasLoading;}
    }

    internal void LoadRule(FloatingReminderRepeatRule rule,DateTime start) {
        loading=true;
        try {
            Weekly.Checked=rule!=null;
            defaultTime=rule==null?Clock(start):DateTime.ParseExact(rule.time,"HH:mm",CultureInfo.InvariantCulture).TimeOfDay;
            for(int day=0;day<7;day++) {
                Days[day].Checked=rule==null?day==(int)start.DayOfWeek:rule.weekdays.Contains(day);
                TimeSpan clock=rule==null?defaultTime:DateTime.ParseExact(rule.TimeForDay(day),"HH:mm",CultureInfo.InvariantCulture).TimeOfDay;
                customTime[day]=clock!=defaultTime;
                DayTimes[day].Value=DateTime.Today.Add(clock);
            }
            End.Value=rule==null?start.AddMonths(1):DateTime.ParseExact(rule.endDate,"yyyy-MM-dd",CultureInfo.InvariantCulture);
            UpdateWeeklyFields();
        } finally {loading=false;}
    }

    internal string EditKey() {
        if(!Weekly.Checked)return "";
        return End.Value.ToString("yyyy-MM-dd",CultureInfo.InvariantCulture)+"|"+
            String.Join(",",Enumerable.Range(0,7).Where(day=>Days[day].Checked).Select(day=>day+":"+DayTimes[day].Value.ToString("HH:mm",CultureInfo.InvariantCulture)));
    }

    internal FloatingReminderRepeatRule ReadRule(DateTime start,DateTime clock) {
        if(!Weekly.Checked)return null;
        var rule=new FloatingReminderRepeatRule{
            weekdays=Enumerable.Range(0,7).Where(day=>Days[day].Checked).ToArray(),
            startDate=start.ToString("yyyy-MM-dd",CultureInfo.InvariantCulture),
            endDate=End.Value.ToString("yyyy-MM-dd",CultureInfo.InvariantCulture),
            time=clock.ToString("HH:mm",CultureInfo.InvariantCulture),
            times=new Dictionary<string,string>(),
        };
        foreach(int day in rule.weekdays)if(customTime[day])rule.times[day.ToString(CultureInfo.InvariantCulture)]=DayTimes[day].Value.ToString("HH:mm",CultureInfo.InvariantCulture);
        rule.Validate();
        return rule;
    }
}
internal sealed partial class FloatingWindow {
    readonly Dictionary<long,SharedList> reminderSharedCache=new Dictionary<long,SharedList>();
    readonly Dictionary<long,string> reminderSharedVersions=new Dictionary<long,string>();
    DateTime nextReminderReadUtc=DateTime.MinValue;
    bool readingReminderSnapshot;
    int reminderSnapshotVersion;
    void InvalidateReminderSnapshot(long taskId) {
        reminderSharedCache.Remove(taskId);reminderSharedVersions.Remove(taskId);
        nextReminderReadUtc=DateTime.MinValue;reminderSnapshotVersion++;
    }
    async Task RefreshReminderSnapshot() {
        if(closing||IsDisposed||readingReminderSnapshot||DateTime.UtcNow<nextReminderReadUtc)return;
        readingReminderSnapshot=true;int version=reminderSnapshotVersion;
        try {
            var all=new Dictionary<long,Dictionary<string,object>>();
            for(int page=1;;page++) {
                var result=await Api("GET","/tasks?per_page=100&page="+page+"&sort_by=id&order_by=asc&expand=comment_count",null);
                if(closing||IsDisposed||version!=reminderSnapshotVersion)return;
                int count=0;foreach(Dictionary<string,object> task in (IEnumerable)result["items"]){all[Convert.ToInt64(task["id"])]=task;count++;}
                if(count==0||page>=Convert.ToInt32(result["total_pages"]))break;
            }
            var shared=new Dictionary<long,SharedList>();var versions=new Dictionary<long,string>();
            foreach(var pair in all) {
                if(closing||IsDisposed||version!=reminderSnapshotVersion)return;
                object updated,count;pair.Value.TryGetValue("updated",out updated);pair.Value.TryGetValue("comment_count",out count);
                string stamp=Convert.ToString(updated)+"|"+Convert.ToString(count),previous;SharedList cached;
                if(reminderSharedVersions.TryGetValue(pair.Key,out previous)&&previous==stamp&&reminderSharedCache.TryGetValue(pair.Key,out cached))shared[pair.Key]=cached;
                else {
                    var list=await ReadOutstandingList(pair.Key,count==null?-1:Convert.ToInt64(count),path=>Api("GET",path,null),()=>!closing&&version==reminderSnapshotVersion);
                    if(list==null)return;shared[pair.Key]=list;
                }
                versions[pair.Key]=stamp;
            }
            if(closing||IsDisposed||version!=reminderSnapshotVersion)return;
            reminderSharedCache.Clear();reminderSharedVersions.Clear();
            foreach(var pair in shared)reminderSharedCache[pair.Key]=pair.Value;
            foreach(var pair in versions)reminderSharedVersions[pair.Key]=pair.Value;
            nextReminderReadUtc=DateTime.UtcNow.AddMinutes(1);
            UpdateReminderTargets(all,shared);
        } catch {nextReminderReadUtc=DateTime.UtcNow.AddSeconds(15);}
        finally {readingReminderSnapshot=false;}
    }
    readonly List<ReminderTarget> repeatingReminderTargets=new List<ReminderTarget>();
    static string RepeatValue(Dictionary<string,object> value){object raw;return value!=null&&value.TryGetValue("repeat_rule",out raw)?Convert.ToString(raw):"";}
    static string ReminderIdentity(DateTimeOffset due,string rule){return ReminderMomentKey(due)+"|"+(rule??"");}
    void ExpandRepeatingReminderTargets(DateTimeOffset now) {
        foreach(string key in reminderTargets.Where(pair=>pair.Value.RepeatRule!=null).Select(pair=>pair.Key).ToArray())reminderTargets.Remove(key);
        foreach(var definition in repeatingReminderTargets) {
            var occurrences=new Dictionary<string,DateTimeOffset>();
            DateTime local;
            if(definition.RepeatRule.OnDate(now.LocalDateTime.Date,out local)) {
                var due=new DateTimeOffset(local);occurrences[definition.Key+":"+ReminderMomentKey(due)]=due;
            }
            // A snoozed occurrence remains valid across restart and the range end,
            // including a popup that was left open for several days before snoozing.
            foreach(string key in reminderState.SnoozedUntil.Keys) {
                if(!key.StartsWith(definition.Key+":",StringComparison.Ordinal))continue;
                DateTimeOffset due;
                if(!DateTimeOffset.TryParseExact(key.Substring(definition.Key.Length+1),"yyyyMMddHHmmssfff",CultureInfo.InvariantCulture,DateTimeStyles.AssumeUniversal,out due))continue;
                if(definition.RepeatRule.OnDate(due.LocalDateTime.Date,out local))occurrences[key]=due;
            }
            foreach(var occurrence in occurrences)
                reminderTargets[occurrence.Key]=new ReminderTarget{Key=occurrence.Key,Title=definition.Title,Kind=definition.Kind,TaskId=definition.TaskId,ItemId=definition.ItemId,Due=occurrence.Value,RepeatRule=definition.RepeatRule};
        }
    }
    void AddReminderDefinition(Dictionary<string,ReminderTarget> next,string prefix,long taskId,string itemId,string title,string kind,DateTimeOffset due,string rawRule) {
        var rule=FloatingReminderRepeatRule.Parse(rawRule);
        if(!String.IsNullOrWhiteSpace(rawRule)&&rule==null)return;
        string key=prefix+":"+ReminderMomentKey(due);
        if(rule!=null)key+=":weekly:"+Convert.ToBase64String(System.Text.Encoding.UTF8.GetBytes(rule.Serialize()));
        var target=new ReminderTarget{Key=key,Title=title,Kind=kind,TaskId=taskId,ItemId=itemId,Due=due,RepeatRule=rule};
        if(rule==null)next[key]=target;else repeatingReminderTargets.Add(target);
    }
}
