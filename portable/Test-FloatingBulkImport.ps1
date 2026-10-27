param([string]$OutputDirectory='.local-build/bulk-import-tests')
$ErrorActionPreference='Stop'
$repoRoot=Split-Path $PSScriptRoot -Parent
$testRoot=[IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$allowed=[IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\')+'\'
if(!$testRoot.StartsWith($allowed,[StringComparison]::OrdinalIgnoreCase)){throw 'Test output must remain inside .local-build.'}
$testRoot=Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# No live program, clipboard, database or network is used. Requests stay in a fake handler.
$source=@"
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Reflection;
using System.Runtime.Serialization;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    internal static async Task TestAtomicSharedTaskCreation() {
        var window=(FloatingWindow)FormatterServices.GetUninitializedObject(typeof(FloatingWindow));var handler=new SharedCreationHandler();
        using(var client=new HttpClient(handler)){
            Action<string,object> field=(name,value)=>typeof(FloatingWindow).GetField(name,BindingFlags.Instance|BindingFlags.NonPublic).SetValue(window,value);
            field("http",client);field("url","http://127.0.0.1:1");field("token","isolated");field("json",new JavaScriptSerializer());field("defaultPriority",7);
            if(await window.CreateSubtask(10,5,"成员新增子任务",0,3)!=101 || handler.Created!=1 || handler.Relations!=0)throw new Exception("Single subtask was not atomically created under its shared parent");
            handler.Deny=true;
            try{await window.CreateSubtask(10,5,"不允许创建的子任务");throw new Exception("Read-only parent accepted a child");}catch(Exception error){if(!error.Message.Contains("共享父任务没有写权限") || !error.Message.Contains("成员只具有读权限"))throw;}
            if(handler.Created!=1 || handler.Relations!=0)throw new Exception("Read-only child failure produced an orphan or a second relation request");
            await window.CreateSubtask(10,5,"旧版本等待关联的条目",222,3);
            if(handler.Relations!=1 || handler.Created!=1)throw new Exception("Legacy pending child retry duplicated its task");
            await window.SavePersonalPriority(10,0);await window.UpdateOutstandingState(10,"outstanding",null,2);
            if(handler.PriorityWrites!=2)throw new Exception("Read-only personal task/outstanding priority did not use its dedicated route");
        }
    }
    internal static async Task TestBulkOutstanding() {
        var window=(FloatingWindow)FormatterServices.GetUninitializedObject(typeof(FloatingWindow));
        var handler=new BulkHandler();
        using(var client=new HttpClient(handler)){
            Action<string,object> field=(name,value)=>typeof(FloatingWindow).GetField(name,BindingFlags.Instance|BindingFlags.NonPublic).SetValue(window,value);
            field("outstandingWriteGates",new Dictionary<long,SemaphoreSlim>());field("http",client);field("url","http://127.0.0.1:1");field("token","isolated");field("json",new JavaScriptSerializer());field("defaultPriority",7);
            var lines=new List<FloatingBulkLine>{new FloatingBulkLine("A < B"),new FloatingBulkLine("第二个遗留事项")};
            handler.Deny=true;
            try{await window.ImportBulkOutstanding(42,lines,3,n=>{});throw new Exception("Denied save accepted");}catch(Exception error){if(!error.Message.Contains("写权限"))throw;}
            if(lines.Any(line=>line.Saved))throw new Exception("Failed save marked success");
            handler.Deny=false;await window.ImportBulkOutstanding(42,lines,3,n=>{});
            int writes=handler.Writes;
            var items=window.ReadShared(await window.ReadHistory(42)).Items;
            if(items.Count!=3 || items[0].NoteHtml!="<p>成员原备注</p>" || items[1].Priority!=3 || items[1].Html!="<p>A &lt; B</p>" || items.Any(item=>item.Done||item.Completion!=0))throw new Exception("Batch overwrite, HTML injection, state or priority mismatch");
            await window.ImportBulkOutstanding(42,lines,3,n=>{});
            if(handler.Writes!=writes)throw new Exception("Confirmed retry wrote duplicate outstanding activity");
        }
    }
}
internal sealed class SharedCreationHandler:HttpMessageHandler {
    internal bool Deny;internal int Created,Relations,PriorityWrites;
    readonly JavaScriptSerializer json=new JavaScriptSerializer();
    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken token){
        string path=request.RequestUri.AbsolutePath;object response;
        if(request.Method==HttpMethod.Get && path=="/api/v2/tasks/10")response=new{id=10,title="来自其他成员的父任务",related_tasks=new Dictionary<string,object>()};
        else if(request.Method==HttpMethod.Post && path=="/api/v2/projects/5/tasks"){
            var payload=json.Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync());if(Convert.ToInt64(payload["parent_task_id"])!=10)throw new Exception("Atomic parent context missing");
            if(Deny)return new HttpResponseMessage(HttpStatusCode.Forbidden){Content=new StringContent("{\"detail\":\"共享父任务没有写权限\",\"errors\":[{\"message\":\"成员只具有读权限\"}]}")};
            Created++;response=new{id=101};
        }else if(request.Method==HttpMethod.Post && path=="/api/v2/tasks/10/relations"){Relations++;response=new{};}
        else if(request.Method==HttpMethod.Put && path=="/api/v2/tasktrace/tasks/10/priority"){
            var payload=json.Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync());int priority=Convert.ToInt32(payload["priority"]);object key;
            if(payload.TryGetValue("outstanding_id",out key)){if(Convert.ToString(key)!="outstanding"||priority!=2)throw new Exception("Outstanding personal priority lost its item identity");}
            else if(priority!=0)throw new Exception("Task personal priority used the reversed storage scale");
            PriorityWrites++;response=new{task_id=10,priority=priority,stored_priority=10-priority};
        }else throw new Exception("Unexpected creation request: "+request.Method+" "+path);
        return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(json.Serialize(response))};
    }
}
internal sealed class BulkHandler:HttpMessageHandler {
    internal bool Deny;internal int Writes;
    string html="<h3 data-tasktrace-comment-type=\"outstanding\">TaskTrace 遗留事项清单</h3><ul><li data-id=\"existing\" data-priority=\"7\">成员原事项<aside data-tasktrace-outstanding-note=\"true\" hidden><p>成员原备注</p></aside></li></ul>";
    protected override async Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken token){
        if(request.RequestUri.Host!="127.0.0.1" || !request.RequestUri.AbsolutePath.StartsWith("/api/v2/tasks/42/comments"))throw new Exception("Unexpected destination");
        var json=new JavaScriptSerializer();
        if(request.Method!=HttpMethod.Get){
            if(Deny)return new HttpResponseMessage(HttpStatusCode.Forbidden){Content=new StringContent("{}")};
            Writes++;html=(string)json.Deserialize<Dictionary<string,object>>(await request.Content.ReadAsStringAsync())["comment"];
            return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent("{\"id\":9}")};
        }
        return new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(json.Serialize(new{items=new[]{new{id=9,comment=html}},total_pages=1}))};
    }
}
internal static class BulkTests {
    static void Check(bool value,string text){if(!value)throw new Exception(text);}
    static void Capture(Form form,string name){Application.DoEvents();using(var bitmap=new Bitmap(form.Width,form.Height)){form.DrawToBitmap(bitmap,new Rectangle(Point.Empty,form.Size));bitmap.Save(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,name+".png"));}}
    static void Display(Form form){form.ShowInTaskbar=false;form.StartPosition=FormStartPosition.Manual;form.Location=new Point(-30000,-30000);form.Show();Application.DoEvents();}
    [STAThread] static int Main(){
        int result=0;Application.EnableVisualStyles();
        using(var host=new Form{Opacity=0,ShowInTaskbar=false}){
            host.Shown+=async delegate{
                try {
                    var parsed=FloatingBulkText.Split("\r\n---\n，。！？\n1. 任务一 \n（二）任务二\n• 任务三\n- [x] 任务四\n①任务五\n1.2. 任务六\n  3.14 压力值  \n2026.10.15 发布\nC:\\work\\任务\n\u200B  任务七　\n重复\n重复\n1.\n___\n===\n  2 任务八");
                    Check(parsed.SequenceEqual(new[]{"任务一","任务二","任务三","任务四","任务五","任务六","3.14 压力值","2026.10.15 发布","C:\\work\\任务","任务七","重复","重复","任务八"}),"Parser lost content, kept separators or failed numbering cleanup: "+String.Join("|",parsed));
                    Check(FloatingBulkText.Split("１２、全角\n三：中文\n(a)英文\n无序号原文").SequenceEqual(new[]{"全角","中文","英文","无序号原文"}),"Alternative numbering failed");
                    Console.WriteLine("PASS blank/punctuation/separator lines, numeric/Chinese/fullwidth/circled/bullet numbering, edge whitespace, decimals, dates, paths and duplicate preservation.");

                    int creates=0,attempts=0,relations=0;bool fail=true;var payloads=new List<string>();var json=new JavaScriptSerializer();
                    Func<string,string,object,Task<Dictionary<string,object>>> api=(method,path,body)=>{
                        Check(method=="POST","Unexpected task operation");
                        var value=json.Deserialize<Dictionary<string,object>>(json.Serialize(body));
                        if(path.EndsWith("/tasks")){
                            Check(path=="/projects/5/tasks" && Convert.ToInt32(value["priority"])==7 && Convert.ToInt64(value["parent_task_id"])==10,"Wrong project, API priority or missing atomic parent");
                            attempts++;if(fail&&attempts==2)throw new Exception("共享父任务无写权限，未创建子任务");
                            creates++;payloads.Add((string)value["title"]);return Task.FromResult(new Dictionary<string,object>{{"id",100+creates}});
                        }
                        Check(path=="/tasks/10/relations","Wrong parent");
                        relations++;
                        return Task.FromResult(new Dictionary<string,object>());
                    };
                    using(var font=new Font("Microsoft YaHei UI",9F))
                    using(var dialog=new FloatingBulkImportDialog("任务","项目验收 / 父任务",3,250,font,(lines,progress)=>FloatingWindow.ImportBulkTasks(lines,5,10,3,api,progress))){
                        dialog.FitToWorkArea(new Rectangle(0,0,1920,1040));Check(dialog.Width>=1200 && dialog.Height>=800,"Bulk import default remains too small");Display(dialog);dialog.Source.Text="1. 第一项任务\n---\n2. 第二项任务\n3. 暂不导入";
                        Check(dialog.Source.Lines.Length==4 && dialog.Preview.Rows.Count==3 && creates==0,"Paste wrote before confirmation");
                        dialog.Preview.Rows[0].Cells[1].Value="修改后的第一项任务";dialog.Preview.Rows.RemoveAt(2);
                        Capture(dialog,"bulk-normal");
                        Check(!await dialog.Import() && creates==1 && dialog.HasSaved,"Partial atomic import not retained");
                        Check(dialog.Preview.Rows[0].ReadOnly && ((FloatingBulkLine)dialog.Preview.Rows[1].Tag).TaskId==0,"Denied atomic create left an orphan task or unlocked saved row");
                        Capture(dialog,"bulk-partial");
                        fail=false;Check(await dialog.Import() && creates==2 && relations==0,"Atomic retry duplicated a task or sent a redundant relation request");
                        Check(payloads.SequenceEqual(new[]{"修改后的第一项任务","第二项任务"}),"Preview edits not saved");
                    }
                    var legacy=new FloatingBulkLine("之前已创建、等待关联的子任务"){TaskId=88};
                    await FloatingWindow.ImportBulkTasks(new List<FloatingBulkLine>{legacy},5,10,3,api,n=>{});
                    Check(legacy.Saved&&relations==1&&creates==2,"Legacy pending association did not retry without creating another task");
                    int saves=0;
                    using(var dialog=new FloatingBulkImportDialog("遗留事项","协作验收任务",7,250,new Font("Microsoft YaHei UI",14F),async (lines,progress)=>{saves++;await Task.Yield();foreach(var line in lines)line.Saved=true;})){
                        Display(dialog);dialog.Size=new Size(1200,900);dialog.Source.Text="1、准备技术评审材料\n2、确认协作成员反馈\n3、安排下一次联调";
                        Capture(dialog,"bulk-large-text");
                        dialog.Source.Text="---\n！！！";Check(!await dialog.Import() && saves==0,"Invalid-only text imported");
                        dialog.Source.Text=new string('长',251);Check(!await dialog.Import() && saves==0,"Overlong task silently truncated");
                        dialog.FitToWorkArea(new Rectangle(0,0,800,600));Check(dialog.Width<=800 && dialog.Height<=600 && dialog.MinimumSize.Width<=dialog.Width && dialog.MinimumSize.Height<=dialog.Height,"Dialog escapes small screen");dialog.Source.Text="保留条目";dialog.Close();Check(saves==0,"Cancel saved data");
                    }
                    using(var dialog=new FloatingBulkImportDialog("任务","项目",7,250,SystemFonts.MessageBoxFont,async (lines,progress)=>{saves++;await Task.Delay(10);foreach(var line in lines)line.Saved=true;})){
                        Display(dialog);dialog.Source.Text="并发确认";var pending=dialog.Import();Check(!await dialog.Import(),"Double confirmation accepted");dialog.Close();Check(!dialog.IsDisposed,"Closed while saving");Check(await pending && saves==1,"One save not completed");
                    }
                    await FloatingWindow.TestAtomicSharedTaskCreation();
                    await FloatingWindow.TestBulkOutstanding();
                    Console.WriteLine("PASS preview edits/deletion, explicit confirmation, cancel, size validation, duplicate-click guard, atomic subtask creation/denied retry and legacy relation recovery, shared-list preservation, write-permission retry and stable-id no-op.");
                }catch(Exception error){Console.Error.WriteLine(error);result=1;}finally{host.Close();}
            };
            Application.Run(host);
        }
        return result;
    }
}
"@
$harness=Join-Path $testRoot 'BulkTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'BulkTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:BulkTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Bulk import compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){ $process.Kill();throw 'Bulk import tests timed out.'}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr;throw 'Bulk import tests failed.'}
if($process.ExitCode){throw ('Bulk import tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)
