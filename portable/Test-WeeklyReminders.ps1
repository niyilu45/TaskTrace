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
 protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancel){
  if(request.Method.Method=="PATCH"){
   var patch=new JavaScriptSerializer().Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync());
   TaskData["reminders"]=patch["reminders"];
  } else if(request.Method!=HttpMethod.Get)throw new Exception("Unexpected write");
  if(request.RequestUri.AbsolutePath.EndsWith("/tasks"))return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(new JavaScriptSerializer().Serialize(new {items=new[]{TaskData,new Dictionary<string,object>{{"id",2},{"title","Other project"},{"project_id",2},{"comment_count",0},{"reminders",TaskData["reminders"]}}},total_pages=1}))};
  return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(new JavaScriptSerializer().Serialize(TaskData))};
 }
}
internal sealed partial class FloatingWindow {
 internal FloatingWindow(string directory,RepeatReminderApi api){
  root=directory;data=directory;url="http://127.0.0.1:1";selfTest=true;
  http.Dispose();http=new HttpClient(api);token="test";reminderState=new ReminderState();
  StartPosition=FormStartPosition.Manual;Location=new Point(-30000,-30000);ShowInTaskbar=false;Opacity=0;
 }
 static void Check(bool ok,string name){if(!ok)throw new Exception(name);}
 static IEnumerable<Control> ControlsDeep(Control control){foreach(Control child in control.Controls){yield return child;foreach(var nested in ControlsDeep(child))yield return nested;}}
 static void CaptureReminder(Form form,string name){form.PerformLayout();Application.DoEvents();using(var bmp=new Bitmap(form.Width,form.Height)){form.DrawToBitmap(bmp,new Rectangle(Point.Empty,bmp.Size));bmp.Save(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,name+".png"));}}
 internal async Task TestRepeating(RepeatReminderApi api){
  var rule=new FloatingReminderRepeatRule{weekdays=new[]{1,3,5},startDate="2026-10-12",endDate="2026-10-23",time="09:30"};rule.Validate();
  DateTime due;Check(rule.OnDate(new DateTime(2026,10,12),out due)&&due.Hour==9&&due.Minute==30,"inclusive start");
  Check(rule.OnDate(new DateTime(2026,10,23),out due),"inclusive end");
  Check(!rule.OnDate(new DateTime(2026,10,24),out due)&&!rule.OnDate(new DateTime(2026,10,13),out due),"range and weekday");
  Check(FloatingReminderRepeatRule.Parse(rule.Serialize()).ToString().Contains("\u5468\u4e00\u3001\u5468\u4e09\u3001\u5468\u4e94"),"roundtrip");
  Check(FloatingReminderRepeatRule.Parse("{\"weekdays\":[]}") == null,"invalid rule");
  Check(FloatingReminderRepeatRule.Parse("{\"weekdays\":[1],\"startDate\":\"2026-10-13\",\"endDate\":\"2026-10-13\",\"time\":\"09:00\"}")==null,"no occurrence");
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
  Check(AbsoluteReminder(DateTime.Now).ContainsKey("reminder"),"single compatibility");
  Console.WriteLine("PASS calendar boundaries, multi weekdays, one-time compatibility, duplicate suppression, dismissal, restart, next date and snooze.");

  Exception uiError=null;bool operated=false;Form active=null;
  using(var timer=new System.Windows.Forms.Timer{Interval=100}){
   timer.Tick+=delegate{
    try{
     active=Application.OpenForms.Cast<Form>().FirstOrDefault(f=>f.Text.StartsWith("\u8bbe\u7f6e\u63d0\u9192"));
     if(active==null||operated)return;operated=true;active.Location=new Point(-30000,-30000);
     var fields=ControlsDeep(active).OfType<FloatingReminderRepeatFields>().Single();
     fields.Weekly.Checked=true;for(int i=0;i<7;i++)fields.Days[i].Checked=i==1||i==3||i==5;
     var pickers=ControlsDeep(active).OfType<DateTimePicker>().Where(p=>p!=fields.End).ToArray();
     pickers[0].Value=new DateTime(2026,10,12);pickers[1].Value=new DateTime(2026,10,12,9,30,0);fields.End.Value=new DateTime(2026,10,23);
     ControlsDeep(active).OfType<Button>().Single(b=>b.Text=="\u6dfb\u52a0\u63d0\u9192").PerformClick();
     Check(ControlsDeep(active).OfType<ListBox>().Single().Items[0].ToString().Contains("\u5468\u4e00\u3001\u5468\u4e09\u3001\u5468\u4e94"),"editor summary");
     CaptureReminder(active,"weekly-editor");
     active.Font=new Font(SystemFonts.MessageBoxFont.FontFamily,12F);active.Size=new Size(800,760);CaptureReminder(active,"weekly-editor-large-text");
     ControlsDeep(active).OfType<Button>().Single(b=>b.Text=="\u4fdd\u5b58").PerformClick();
    }catch(Exception e){uiError=e;if(active!=null)active.Close();}
   };
   timer.Start();
   bool saved=await ShowReminderEditor(1,null,this);timer.Stop();
   if(uiError!=null)throw uiError;
   Check(saved&&operated,"save must close dialog");
   Check(TaskReminderValues(api.TaskData).Count==1&&FloatingReminderRepeatRule.Parse(RepeatValue(TaskReminderValues(api.TaskData)[0]))!=null,"API did not receive rule");
  }
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
  Console.WriteLine("PASS actual native editor weekday selection, summary, save/close and API payload.");
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
