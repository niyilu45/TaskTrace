$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = Join-Path $repoRoot ('.local-build/weekly-reminder-tests/' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
$source = @'

using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;
internal sealed class RepeatReminderApi : HttpMessageHandler {
 internal Dictionary<string,object> TaskData=new Dictionary<string,object>{{"id",1},{"title","\u5468\u671f\u63d0\u9192\u6d4b\u8bd5\u4efb\u52a1"},{"comment_count",0},{"reminders",new object[0]}};
 readonly List<Dictionary<string,object>> comments=new List<Dictionary<string,object>>();
 internal int TaskWrites, OutstandingWrites;
 static HttpResponseMessage Reply(object result){
  return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(new JavaScriptSerializer().Serialize(result))};
 }
 protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancel){
  string path=request.RequestUri.AbsolutePath;
  string method=request.Method.Method;
  if(path=="/api/v2/tasks/1"&&method=="PATCH"){
   var patch=new JavaScriptSerializer().Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync());
   TaskData["reminders"]=patch["reminders"];
   TaskWrites++;
   return Reply(TaskData);
  }
  if(path=="/api/v2/tasks/1/comments"&&method=="GET")return Reply(new {items=comments,total_pages=1});
  if((path=="/api/v2/tasks/1/comments"&&method=="POST")||(path=="/api/v2/tasks/1/comments/1"&&method=="PUT")){
   var comment=new JavaScriptSerializer().Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync());
   comment["id"]=1;
   if(method=="POST")comments.Add(comment);
   else comments[0]=comment;
   TaskData["comment_count"]=comments.Count;
   OutstandingWrites++;
   return Reply(comment);
  }
  if(path=="/api/v2/tasks"&&method=="GET")return Reply(new {items=new[]{TaskData,new Dictionary<string,object>{{"id",2},{"title","Other project"},{"project_id",2},{"comment_count",0},{"reminders",TaskData["reminders"]}}},total_pages=1});
  if(path=="/api/v2/tasks/1"&&method=="GET")return Reply(TaskData);
  throw new Exception("Unexpected API request "+method+" "+path);
 }
}
internal sealed partial class FloatingWindow {
 internal FloatingWindow(string directory,RepeatReminderApi api){
  root=directory;data=directory;url="http://127.0.0.1:1";selfTest=true;
  http.Dispose();http=new HttpClient(api);token="test";reminderState=new ReminderState();undoRecording=false;
  StartPosition=FormStartPosition.Manual;Location=new Point(-30000,-30000);ShowInTaskbar=false;Opacity=0;
 }
 static void Check(bool ok,string name){if(!ok)throw new Exception(name);}
 static IEnumerable<Control> ControlsDeep(Control control){foreach(Control child in control.Controls){yield return child;foreach(var nested in ControlsDeep(child))yield return nested;}}
 static void CaptureReminder(Form form,string name){form.PerformLayout();Application.DoEvents();using(var bmp=new Bitmap(form.Width,form.Height)){form.DrawToBitmap(bmp,new Rectangle(Point.Empty,bmp.Size));bmp.Save(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,name+".png"));}}
 static Button ReminderButton(Form form,string caption){
  return ControlsDeep(form).OfType<Button>().Single(button=>button.Text==caption);
 }
 static void ClickReminderButton(Form form,string caption){ReminderButton(form,caption).PerformClick();}
 static void CheckUnclipped(Control control){
  var bounds=control.RectangleToScreen(control.ClientRectangle);
  Check(control.Visible&&bounds.Width>0&&bounds.Height>0,"reminder control is hidden: "+control.AccessibleName);
  for(var parent=control.Parent;parent!=null;parent=parent.Parent)
   Check(parent.RectangleToScreen(parent.ClientRectangle).Contains(bounds),"reminder control is clipped: "+(control.AccessibleName??control.Text));
 }
 async Task<bool> DriveReminderDialog(string itemId,Action<Form> operate){
  Exception error=null;bool operated=false;Form active=null;
  using(var timer=new System.Windows.Forms.Timer{Interval=100}){
   var timeout=DateTime.UtcNow.AddSeconds(3);
   timer.Tick+=delegate{
    active=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text.StartsWith("\u8bbe\u7f6e\u63d0\u9192"));
    if(active==null)return;
    if(operated){
     if(DateTime.UtcNow<timeout)return;
     error=new Exception("Reminder dialog did not close. "+String.Join(" | ",ControlsDeep(active).OfType<Label>().Select(label=>label.Text)));
     active.Close();
     return;
    }
    operated=true;
    active.Location=new Point(-30000,-30000);
    try{operate(active);}
    catch(Exception failure){error=failure;active.Close();}
   };
   timer.Start();
   bool saved=await ShowReminderEditor(1,itemId,this);
   timer.Stop();
   if(error!=null)throw error;
   Check(operated,"reminder editor did not open");
   return saved;
  }
 }
 static DateTimePicker[] MainReminderPickers(Form form,FloatingReminderRepeatFields fields){
  return ControlsDeep(form).OfType<DateTimePicker>().Where(picker=>picker!=fields.End&&!fields.DayTimes.Contains(picker)).ToArray();
 }
 void TestSnoozedPopup(string itemId){
  reminderTargets.Clear();repeatingReminderTargets.Clear();reminderState=new ReminderState();
  DateTimeOffset original=DateTimeOffset.UtcNow.AddHours(-2);
  string kind=itemId==null?"\u4efb\u52a1":"\u9057\u7559\u4e8b\u9879";
  string prefix=itemId==null?"task:1":"outstanding:1:"+itemId;
  AddReminderDefinition(reminderTargets,prefix,1,itemId,"Snooze test",kind,original,null);
  var target=reminderTargets.Values.Single();
  const string format="yyyy-MM-dd HH:mm";
  const string timePrefix="\u672c\u6b21\u63d0\u9192\u65f6\u95f4 ";
  const string previewPrefix="\u9884\u8ba1\u518d\u6b21\u63d0\u9192 ";
  CheckDueReminders();
  var dialog=reminderPopup;
  Check(dialog!=null,"original reminder did not open");
  dialog.Opacity=0;
  dialog.Size=dialog.MinimumSize;
  dialog.PerformLayout();
  var labels=ControlsDeep(dialog).OfType<Label>().ToArray();
  var currentTime=labels.Single(label=>label.Text.StartsWith(timePrefix));
  var preview=labels.Single(label=>label.Text.StartsWith(previewPrefix));
  var inputs=ControlsDeep(dialog).OfType<NumericUpDown>().ToArray();
  var hours=inputs[0];var minutes=inputs[1];
  var delay=ReminderButton(dialog,"\u5ef6\u8fdf");
  Check(currentTime.Text==timePrefix+original.LocalDateTime.ToString(format),"first reminder time is missing");
  Check(hours.Value==0&&minutes.Value==15,"default snooze is not 15 minutes");
  foreach(var control in new Control[]{currentTime,preview,hours,minutes,delay})CheckUnclipped(control);
  minutes.Value=0;
  Check(!delay.Enabled,"zero duration can be saved");
  hours.Value=168;minutes.Value=1;
  Check(!delay.Enabled,"duration beyond seven days can be saved");
  minutes.Value=0;
  Check(delay.Enabled,"exactly seven days is not allowed");
  hours.Text="25";minutes.Text="7";
  DateTimeOffset before=DateTimeOffset.UtcNow;
  DateTimeOffset expected=before.AddHours(25).AddMinutes(7);
  Check(delay.Enabled&&(preview.Text==previewPrefix+expected.LocalDateTime.ToString(format)||
   preview.Text==previewPrefix+expected.AddMinutes(-1).LocalDateTime.ToString(format)),"typed hours and minutes did not update the next time");
  delay.PerformClick();
  Check(reminderPopup==null,"snoozing did not close the popup");
  reminderState=json.Deserialize<ReminderState>(File.ReadAllText(ReminderStatePath));
  DateTimeOffset saved;
  Check(TryReminderTime(reminderState.SnoozedUntil[target.Key],out saved)&&
   saved>=expected.AddSeconds(-1)&&saved<=DateTimeOffset.UtcNow.AddHours(25).AddMinutes(7),"saved snooze does not match the duration");
  CheckDueReminders();
  Check(reminderPopup==null,"snoozed reminder opened before its next time");

  DateTimeOffset next=DateTimeOffset.UtcNow.AddMinutes(-3);
  reminderState.SnoozedUntil[target.Key]=ReminderStamp(next);
  SaveReminderState();
  reminderState=json.Deserialize<ReminderState>(File.ReadAllText(ReminderStatePath));
  CheckDueReminders();
  dialog=reminderPopup;
  Check(dialog!=null,"due snoozed reminder did not reopen");
  dialog.Opacity=0;
  currentTime=ControlsDeep(dialog).OfType<Label>().Single(label=>label.Text.StartsWith(timePrefix));
  Check(currentTime.Text==timePrefix+next.LocalDateTime.ToString(format),"reopened reminder displayed the original time");
  ClickReminderButton(dialog,"\u5ef6\u8fdf");
  reminderTargets.Clear();reminderState=new ReminderState();
 }
 static readonly DateTime RepeatStart=new DateTime(2026,10,12);
 static readonly DateTime RepeatEnd=new DateTime(2026,10,23);
 static void FillWeeklyReminder(Form form){
  var fields=ControlsDeep(form).OfType<FloatingReminderRepeatFields>().Single();
  var pickers=MainReminderPickers(form,fields);
  pickers[0].Value=RepeatStart;pickers[1].Value=RepeatStart.AddHours(9).AddMinutes(30);
  fields.Weekly.Checked=true;
  for(int day=0;day<7;day++)fields.Days[day].Checked=day==1||day==3||day==5;
  fields.End.Value=RepeatEnd;
  Check(fields.DayTimes[1].Enabled&&!fields.DayTimes[2].Enabled,"only selected weekdays can be edited");
  fields.DayTimes[1].Value=RepeatStart.AddHours(8).AddMinutes(15);
  fields.DayTimes[5].Value=RepeatStart.AddHours(17).AddMinutes(45);
 }
 static void CheckWeeklyReminder(Form form,string monday,string wednesday,string friday){
  var list=ControlsDeep(form).OfType<ListBox>().Single();
  Check(list.Items.Count==1&&list.SelectedIndex==0,"saved reminder was not selected on reopening");
  var fields=ControlsDeep(form).OfType<FloatingReminderRepeatFields>().Single();
  var pickers=MainReminderPickers(form,fields);
  Check(fields.Weekly.Checked&&Enumerable.Range(0,7).Where(day=>fields.Days[day].Checked).SequenceEqual(new[]{1,3,5}),"saved weekdays missing");
  Check(pickers[0].Value.Date==RepeatStart&&fields.End.Value.Date==RepeatEnd,"saved date range missing");
  Check(fields.DayTimes[1].Value.ToString("HH:mm")==monday&&fields.DayTimes[3].Value.ToString("HH:mm")==wednesday&&fields.DayTimes[5].Value.ToString("HH:mm")==friday,"saved weekday times missing");
  var summary=list.Items[0].ToString();
  Check(summary.Contains("\u5468\u4e00")&&summary.Contains(monday)&&summary.Contains(friday),"reminder list summary missing weekday times");
 }
 async Task<FloatingReminderRepeatRule> SavedWeeklyReminder(RepeatReminderApi api,string itemId){
  if(itemId==null)return FloatingReminderRepeatRule.Parse(RepeatValue(TaskReminderValues(api.TaskData).SingleOrDefault()));
  var item=ReadShared(await ReadHistory(1)).Items.Single(row=>row.Id==itemId);
  return FloatingReminderRepeatRule.Parse(item.RepeatRule);
 }
 async Task TestReminderEditor(RepeatReminderApi api,string itemId){
  string kind=itemId==null?"task":"outstanding";
  Func<int> writes=()=>itemId==null?api.TaskWrites:api.OutstandingWrites;
  int before=writes();
  Check(await DriveReminderDialog(itemId,form=>{
   FillWeeklyReminder(form);
   if(itemId==null){
    var normal=form.Size;
    form.Size=form.MinimumSize;
    CaptureReminder(form,"weekly-editor-minimum");
    var fields=ControlsDeep(form).OfType<FloatingReminderRepeatFields>().Single();
    foreach(var clock in fields.DayTimes)CheckUnclipped(clock);
    CheckUnclipped(fields.End);
    CheckUnclipped(ReminderButton(form,"\u4fdd\u5b58"));
    form.Size=normal;
    CaptureReminder(form,"weekly-editor");
    form.Font=new Font(SystemFonts.MessageBoxFont.FontFamily,12F);
    form.Size=new Size(800,760);
    CaptureReminder(form,"weekly-editor-large-text");
   }
   ClickReminderButton(form,"\u4fdd\u5b58");
  }),kind+" direct save did not close after saving");
  var saved=await SavedWeeklyReminder(api,itemId);
  Check(writes()==before+1&&saved!=null&&saved.time=="09:30"&&saved.times.Count==2&&saved.times["1"]=="08:15"&&saved.times["5"]=="17:45",kind+" direct save did not persist the rule");
  Check(!await DriveReminderDialog(itemId,form=>{
   CheckWeeklyReminder(form,"08:15","09:30","17:45");
   ClickReminderButton(form,"\u4fdd\u5b58");
  })&&writes()==before+1,kind+" saving untouched reminder unexpectedly wrote data");
  Check(!await DriveReminderDialog(itemId,form=>{
   var fields=ControlsDeep(form).OfType<FloatingReminderRepeatFields>().Single();
   fields.DayTimes[1].Value=RepeatStart.AddHours(6);
   ClickReminderButton(form,"\u53d6\u6d88");
  })&&writes()==before+1,kind+" cancel wrote changes");
  Check(await DriveReminderDialog(itemId,form=>{
   CheckWeeklyReminder(form,"08:15","09:30","17:45");
   var fields=ControlsDeep(form).OfType<FloatingReminderRepeatFields>().Single();
   MainReminderPickers(form,fields)[1].Value=RepeatStart.AddHours(10);
   Check(fields.DayTimes[1].Value.ToString("HH:mm")=="08:15"&&fields.DayTimes[3].Value.ToString("HH:mm")=="10:00","changing the default time overwrote weekday overrides");
   fields.DayTimes[5].Value=RepeatStart.AddHours(18);
   ClickReminderButton(form,"\u4fdd\u5b58");
  }),kind+" direct update did not save");
  saved=await SavedWeeklyReminder(api,itemId);
  Check(writes()==before+2&&saved!=null&&saved.time=="10:00"&&saved.times["5"]=="18:00",kind+" direct update did not persist changes");
  Check(await DriveReminderDialog(itemId,form=>{
   CheckWeeklyReminder(form,"08:15","10:00","18:00");
   ClickReminderButton(form,"\u79fb\u9664\u9009\u4e2d");
   ClickReminderButton(form,"\u4fdd\u5b58");
  }),kind+" removal did not save");
  Check(writes()==before+3&&(await SavedWeeklyReminder(api,itemId))==null,kind+" save after removal recreated the rule");
  Check(!await DriveReminderDialog(itemId,form=>{
   Check(ControlsDeep(form).OfType<ListBox>().Single().Items.Count==0,"removed reminder still listed");
   ClickReminderButton(form,"\u53d6\u6d88");
  }),kind+" unexpected save after removal");
  Console.WriteLine("PASS "+kind+" native weekly editor direct save, reopen, unchanged save, cancel, update and removal.");
 }
 internal async Task TestRepeating(RepeatReminderApi api){
  var rule=new FloatingReminderRepeatRule{weekdays=new[]{1,3,5},startDate="2026-10-12",endDate="2026-10-23",time="09:30"};rule.Validate();
  DateTime due;Check(rule.OnDate(new DateTime(2026,10,12),out due)&&due.Hour==9&&due.Minute==30,"inclusive start");
  Check(rule.OnDate(new DateTime(2026,10,23),out due),"inclusive end");
  Check(!rule.OnDate(new DateTime(2026,10,24),out due)&&!rule.OnDate(new DateTime(2026,10,13),out due),"range and weekday");
  Check(FloatingReminderRepeatRule.Parse(rule.Serialize()).ToString().Contains("\u5468\u4e00\u3001\u5468\u4e09\u3001\u5468\u4e94"),"roundtrip");
  Check(FloatingReminderRepeatRule.Parse("{\"weekdays\":[]}") == null,"invalid rule");
  Check(FloatingReminderRepeatRule.Parse("{\"weekdays\":[1],\"startDate\":\"2026-10-13\",\"endDate\":\"2026-10-13\",\"time\":\"09:00\"}")==null,"no occurrence");
  var individual=FloatingReminderRepeatRule.Parse("{\"weekdays\":[1,3,5],\"startDate\":\"2026-10-12\",\"endDate\":\"2026-10-23\",\"time\":\"09:30\",\"times\":{\"1\":\"08:15\",\"5\":\"17:45\"}}");
  Check(individual!=null&&individual.FirstDate()==RepeatStart.AddHours(8).AddMinutes(15),"first date uses weekday override");
  Check(individual.OnDate(RepeatStart.AddDays(2),out due)&&due.ToString("HH:mm")=="09:30","unconfigured weekday uses default time");
  Check(FloatingReminderRepeatRule.Parse(individual.Serialize()).OnDate(RepeatEnd,out due)&&due.ToString("HH:mm")=="17:45","last weekday override survives roundtrip");
  Check(!rule.Serialize().Contains("\"times\""),"legacy shared time serialization changed");
  DateTimeOffset now=new DateTimeOffset(new DateTime(2026,10,14,10,0,0));
  AddReminderDefinition(reminderTargets,"task:1",1,null,"task","\u4efb\u52a1",new DateTimeOffset(rule.FirstDate()),rule.Serialize());
  ExpandRepeatingReminderTargets(now);Check(reminderTargets.Count==1,"exactly today's occurrence");
  var today=reminderTargets.Values.Single();reminderState.Dismissed[today.Key]=ReminderStamp(today.Due);SaveReminderState();
  ExpandRepeatingReminderTargets(now);Check(reminderState.Dismissed.ContainsKey(reminderTargets.Values.Single().Key),"refresh repeated dismissed occurrence");
  await DeleteReminderTarget(today);Check(repeatingReminderTargets.Count==1,"dismiss deleted schedule");
  ExpandRepeatingReminderTargets(now.AddDays(2));Check(reminderTargets.Count==1&&!reminderState.Dismissed.ContainsKey(reminderTargets.Values.Single().Key),"next occurrence");
  var friday=reminderTargets.Values.Single();reminderState.SnoozedUntil[friday.Key]=ReminderStamp(now.AddDays(3));
  ExpandRepeatingReminderTargets(now.AddDays(3));Check(reminderTargets.ContainsKey(friday.Key),"snooze across midnight");
  reminderState=json.Deserialize<ReminderState>(File.ReadAllText(ReminderStatePath));Check(reminderState.Dismissed.ContainsKey(today.Key),"dismiss survives restart");
  ExpandRepeatingReminderTargets(now.AddDays(12));Check(reminderTargets.Count==0,"stops after date range");
  reminderTargets.Clear();repeatingReminderTargets.Clear();
  AddReminderDefinition(reminderTargets,"outstanding:1:test",1,"test","outstanding","weekday clock",new DateTimeOffset(individual.FirstDate()),individual.Serialize());
  ExpandRepeatingReminderTargets(new DateTimeOffset(RepeatStart.AddHours(10)));
  Check(reminderTargets.Values.Single().Due.LocalDateTime==RepeatStart.AddHours(8).AddMinutes(15),"scheduled Monday uses its override");
  ExpandRepeatingReminderTargets(new DateTimeOffset(RepeatEnd.AddHours(18)));
  Check(reminderTargets.Values.Single().Due.LocalDateTime==RepeatEnd.AddHours(17).AddMinutes(45),"scheduled Friday uses its override");
  reminderTargets.Clear();repeatingReminderTargets.Clear();
  Check(AbsoluteReminder(DateTime.Now).ContainsKey("reminder"),"single compatibility");
  Console.WriteLine("PASS calendar boundaries, multi weekdays, one-time compatibility, duplicate suppression, dismissal, restart, next date and snooze.");

  var legacy=AbsoluteReminder(rule.FirstDate());
  legacy["repeat_rule"]=rule.Serialize();api.TaskData["reminders"]=new[]{legacy};
  Check(!await DriveReminderDialog(null,form=>{
   CheckWeeklyReminder(form,"09:30","09:30","09:30");
   ClickReminderButton(form,"\u53d6\u6d88");
  })&&api.TaskWrites==0,"legacy rule could not be reopened without writes");
  api.TaskData["reminders"]=new object[0];
  await TestReminderEditor(api,null);
  var shared=new SharedList();shared.Items.Add(new PendingItem{Id="test-outstanding",Html="Outstanding test item"});
  await WriteShared(1,shared);
  await TestReminderEditor(api,"test-outstanding");
  Check(TaskReminderValues(api.TaskData).Count==0,"outstanding reminder created a duplicate task reminder");
  var future=new FloatingReminderRepeatRule{weekdays=new[]{0,1,2,3,4,5,6},startDate=DateTime.Today.AddDays(7).ToString("yyyy-MM-dd"),endDate=DateTime.Today.AddDays(21).ToString("yyyy-MM-dd"),time="09:30"};
  var futureValue=AbsoluteReminder(future.FirstDate());futureValue["repeat_rule"]=future.Serialize();api.TaskData["reminders"]=new[]{futureValue};
  await RefreshReminderSnapshot();
  Check(repeatingReminderTargets.Count==2,"reminders from another project were omitted");
  Check(reminderSharedCache.Count==2,"snapshot did not cache per-task outstanding lists");
  var deadline=nextReminderReadUtc;await RefreshReminderSnapshot();Check(nextReminderReadUtc==deadline,"unnecessary full refresh");
  var single=AbsoluteReminder(future.FirstDate());api.TaskData["reminders"]=new[]{futureValue,single};
  await DeleteReminderTarget(new ReminderTarget{TaskId=1,Due=new DateTimeOffset(future.FirstDate())});
  Check(TaskReminderValues(api.TaskData).Count==1&&!String.IsNullOrEmpty(RepeatValue(TaskReminderValues(api.TaskData)[0])),"closing single reminder deleted weekly rule at same time");
  Console.WriteLine("PASS reminders independent of current project and cached polling.");

  TestSnoozedPopup(null);
  TestSnoozedPopup("test-outstanding");
  Console.WriteLine("PASS task and outstanding popup hours, minutes, live preview, saved delay, and the time shown when the reminder reopens.");

 }
}
internal static class RepeatReminderTests {
 [STAThread] static int Main(){
  Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);int result=0;var api=new RepeatReminderApi();
  using(var app=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory,api)){
   app.Shown+=async delegate{try{await app.TestRepeating(api);}catch(Exception e){Console.Error.WriteLine(e);result=1;}finally{app.Close();}};
   Application.Run(app);
  }
  return result;
 }
}

'@
$harness=Join-Path $testRoot 'RepeatReminderTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'RepeatReminderTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:RepeatReminderTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll /reference:System.Web.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Weekly reminder test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(45000)){ $process.Kill();throw 'Weekly reminder test timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr;throw 'Weekly reminder test failed'}
Write-Output ('Results: '+$stdout)
