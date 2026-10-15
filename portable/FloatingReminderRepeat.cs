
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
    internal static readonly string[] DayNames={"\u5468\u65e5","\u5468\u4e00","\u5468\u4e8c","\u5468\u4e09","\u5468\u56db","\u5468\u4e94","\u5468\u516d"};
    internal static FloatingReminderRepeatRule Parse(string raw) {
        try {var rule=new JavaScriptSerializer().Deserialize<FloatingReminderRepeatRule>(raw);rule.Validate();return rule;} catch {return null;}
    }
    internal void Validate() {
        DateTime start,end,clock;
        if(weekdays==null||weekdays.Length==0||weekdays.Any(day=>day<0||day>6))throw new Exception("\u8bf7\u81f3\u5c11\u9009\u62e9\u4e00\u4e2a\u661f\u671f\u3002");
        if(!DateTime.TryParseExact(startDate,"yyyy-MM-dd",CultureInfo.InvariantCulture,DateTimeStyles.None,out start)||!DateTime.TryParseExact(endDate,"yyyy-MM-dd",CultureInfo.InvariantCulture,DateTimeStyles.None,out end))throw new Exception("\u8bf7\u9009\u62e9\u6709\u6548\u7684\u5f00\u59cb\u548c\u7ed3\u675f\u65e5\u671f\u3002");
        if(end<start)throw new Exception("\u7ed3\u675f\u65e5\u671f\u4e0d\u80fd\u65e9\u4e8e\u5f00\u59cb\u65e5\u671f\u3002");
        if(!DateTime.TryParseExact(time,"HH:mm",CultureInfo.InvariantCulture,DateTimeStyles.None,out clock))throw new Exception("\u8bf7\u9009\u62e9\u63d0\u9192\u65f6\u95f4\u3002");
        weekdays=weekdays.Distinct().OrderBy(day=>day).ToArray();
        if(!Enumerable.Range(0,7).Select(offset=>start.AddDays(offset)).Any(date=>date<=end&&weekdays.Contains((int)date.DayOfWeek)))throw new Exception("\u65e5\u671f\u8303\u56f4\u5185\u6ca1\u6709\u6240\u9009\u661f\u671f\uff0c\u8bf7\u8c03\u6574\u65e5\u671f\u6216\u661f\u671f\u3002");
    }
    internal DateTime FirstDate() {
        Validate();DateTime start=DateTime.ParseExact(startDate,"yyyy-MM-dd",CultureInfo.InvariantCulture);
        for(int i=0;i<7;i++){DateTime due;if(OnDate(start.AddDays(i),out due))return due;}
        throw new Exception("\u65e5\u671f\u8303\u56f4\u5185\u6ca1\u6709\u6240\u9009\u661f\u671f\u3002");
    }
    internal bool OnDate(DateTime date,out DateTime due) {
        due=DateTime.MinValue;string day=date.ToString("yyyy-MM-dd",CultureInfo.InvariantCulture);
        if(String.CompareOrdinal(day,startDate)<0||String.CompareOrdinal(day,endDate)>0||!weekdays.Contains((int)date.DayOfWeek))return false;
        return DateTime.TryParseExact(day+" "+time,"yyyy-MM-dd HH:mm",CultureInfo.InvariantCulture,DateTimeStyles.None,out due);
    }
    internal string Serialize(){Validate();return new JavaScriptSerializer().Serialize(this);}
    public override string ToString(){return "\u6bcf\u5468 "+String.Join("\u3001",new[]{1,2,3,4,5,6,0}.Where(day=>weekdays.Contains(day)).Select(day=>DayNames[day]))+" "+time+"\uff08"+startDate+" \u81f3 "+endDate+"\uff09";}
}
internal sealed class FloatingReminderRepeatFields : TableLayoutPanel {
    internal readonly CheckBox Weekly=new CheckBox{Text="\u6309\u5468\u91cd\u590d",AutoSize=true,Name="reminder-weekly"};
    internal readonly DateTimePicker End=new DateTimePicker{Format=DateTimePickerFormat.Custom,CustomFormat="yyyy-MM-dd",Name="reminder-end",Width=150};
    internal readonly CheckBox[] Days=new CheckBox[7];
    readonly FlowLayoutPanel weekdaysRow=new FlowLayoutPanel{AutoSize=true,Dock=DockStyle.Fill,WrapContents=true};
    readonly FlowLayoutPanel endRow=new FlowLayoutPanel{AutoSize=true,Dock=DockStyle.Fill,WrapContents=true};
    internal FloatingReminderRepeatFields() {
        Dock=DockStyle.Fill;AutoSize=true;ColumnCount=1;RowCount=4;Margin=Padding.Empty;
        for(int i=0;i<4;i++)RowStyles.Add(new RowStyle(SizeType.AutoSize));
        foreach(int day in new[]{1,2,3,4,5,6,0}) {Days[day]=new CheckBox{Text=FloatingReminderRepeatRule.DayNames[day],AutoSize=true,Name="reminder-day-"+day};weekdaysRow.Controls.Add(Days[day]);}
        endRow.Controls.Add(new Label{Text="\u7ed3\u675f\u65e5\u671f\uff08\u5305\u542b\u5f53\u5929\uff09",AutoSize=true,Margin=new Padding(0,6,10,0)});endRow.Controls.Add(End);
        Controls.Add(Weekly);Controls.Add(weekdaysRow);Controls.Add(endRow);
        Controls.Add(new Label{Text="\u5f00\u59cb\u65e5\u671f\u4f7f\u7528\u4e0a\u65b9\u63d0\u9192\u65e5\u671f\u3002\u6309\u672c\u673a\u65f6\u95f4\u63d0\u9192\uff0c\u5173\u95ed\u4e00\u6b21\u4e0d\u5f71\u54cd\u540e\u7eed\u65e5\u671f\u3002",AutoSize=true,Dock=DockStyle.Fill,ForeColor=SystemColors.GrayText,MaximumSize=new Size(580,0)});
        Weekly.CheckedChanged+=delegate{weekdaysRow.Enabled=endRow.Enabled=Weekly.Checked;};
        LoadRule(null,DateTime.Now);
    }
    internal void LoadRule(FloatingReminderRepeatRule rule,DateTime start) {
        Weekly.Checked=rule!=null;
        for(int i=0;i<7;i++)Days[i].Checked=rule==null?i==(int)start.DayOfWeek:rule.weekdays.Contains(i);
        End.Value=rule==null?start.AddMonths(1):DateTime.ParseExact(rule.endDate,"yyyy-MM-dd",CultureInfo.InvariantCulture);
        weekdaysRow.Enabled=endRow.Enabled=Weekly.Checked;
    }
    internal FloatingReminderRepeatRule ReadRule(DateTime start,DateTime clock) {
        if(!Weekly.Checked)return null;
        var rule=new FloatingReminderRepeatRule{weekdays=Enumerable.Range(0,7).Where(day=>Days[day].Checked).ToArray(),startDate=start.ToString("yyyy-MM-dd"),endDate=End.Value.ToString("yyyy-MM-dd"),time=clock.ToString("HH:mm")};
        rule.Validate();return rule;
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
