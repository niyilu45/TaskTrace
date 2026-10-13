param([string]$OutputDirectory = '.local-build/floating-task-name-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Exercise the real editor and its PATCH helper with an isolated in-memory server.
# No installed application, account, database, network or clipboard is accessed.
$source = @"
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;
static class FloatingTaskNameTests {
    static void Check(bool value,string message){if(!value)throw new Exception(message);}
    [STAThread] static int Main() {
        int code=0;Application.EnableVisualStyles();
        using(var host=new Form{ClientSize=new Size(592,116),Opacity=0,ShowInTaskbar=false,Font=new Font("Microsoft YaHei UI",9F),Text="任务名编辑验收"}) {
            string stored="协作任务原名",feedback="";int reads=0,writes=0;bool denied=false;TaskCompletionSource<bool> pending=null;
            Func<string,string,object,Task<Dictionary<string,object>>> api=async delegate(string method,string path,object body) {
                Check(path=="/tasks/42","Rename targeted the wrong task or created a comment");
                if(method=="GET"){reads++;if(pending!=null)await pending.Task;return new Dictionary<string,object>{{"title",stored}};}
                Check(method=="PATCH","Rename must use a partial update");writes++;
                if(denied)throw new Exception("当前不具备写权限");
                var payload=new JavaScriptSerializer().Deserialize<Dictionary<string,object>>(new JavaScriptSerializer().Serialize(body));
                Check(payload.Count==1 && payload.ContainsKey("title"),"Rename overwrote unrelated task data");stored=(string)payload["title"];
                return new Dictionary<string,object>{{"title",stored}};
            };
            using(var editor=new TaskNameEditor(stored,(original,title)=>FloatingWindow.WriteTaskName(42,original,title,api),message=>feedback=message)) {
                editor.Dock=DockStyle.Top;editor.Height=58;host.Controls.Add(editor);
                host.Shown+=async delegate {
                    try {
                        Check(editor.Input.Text==stored && !editor.Dirty && !editor.SaveButton.Enabled,"Opening a task must show its name without dirty state");
                        Check(await editor.Save() && reads==0 && writes==0,"Unchanged name issued a request");
                        editor.Input.Text="  "+stored+"  ";Check(!editor.Dirty,"Unchanged trimmed name was dirty");editor.Discard();
                        editor.Input.Text=" ";Check(!await editor.Save() && reads==0 && feedback.Contains("不能为空"),"Empty title was accepted");editor.Discard();
                        editor.Input.Text="协作任务新名";Check(editor.Dirty && await editor.Save(),"Rename failed");
                        Check(stored=="协作任务新名" && !editor.Dirty && writes==1,"Successful rename retained unsaved state");
                        Check(await editor.Save() && writes==1,"Saving twice duplicated an update");
                        editor.Input.Text="丢弃的修改";editor.Discard();Check(!editor.Dirty && editor.Input.Text==stored,"Discard kept dirty name");
                        using(var reopened=new TaskNameEditor(stored,(original,title)=>FloatingWindow.WriteTaskName(42,original,title,api),message=>feedback=message)){Check(!reopened.Dirty,"Reopening after discard was dirty");}
                        denied=true;editor.Input.Text="权限恢复后保存";Check(!await editor.Save() && editor.Dirty && feedback.Contains("写权限"),"Permission failure lost the edit");
                        denied=false;Check(await editor.Save() && !editor.Dirty && stored=="权限恢复后保存","Retry after permission change failed");
                        editor.Input.Text="本机改名";stored="协作成员已改名";int before=writes;
                        Check(!await editor.Save() && writes==before && editor.Dirty && feedback.Contains("协作成员"),"Concurrent rename was overwritten");
                        editor.Input.Text=stored;Check(await editor.Save() && !editor.Dirty && writes==before,"Already-applied remote name caused a duplicate update");
                        editor.CanSave=()=>false;editor.Input.Text="下一次修改";before=reads;
                        Check(!await editor.Save() && reads==before && editor.Dirty,"Concurrent progress save was not respected");editor.CanSave=()=>true;
                        pending=new TaskCompletionSource<bool>();var save=editor.Save();Check(editor.Saving && editor.Input.ReadOnly && !editor.SaveButton.Enabled && !await editor.Save(),"Pending save allowed duplicate edits");
                        pending.SetResult(true);Check(await save && !editor.Dirty,"Pending save did not clear dirty state");pending=null;
                        foreach(int width in new[]{592,432}) {
                            host.ClientSize=new Size(width,116);host.PerformLayout();editor.PerformLayout();
                            Check(editor.Input.Width>100 && editor.Input.Height>=20 && editor.SaveButton.Right<=editor.Width-14,"Task name controls overflow the narrow dialog");
                            using(var bitmap=new Bitmap(host.Width,host.Height)){host.DrawToBitmap(bitmap,new Rectangle(Point.Empty,host.Size));bitmap.Save(Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"task-name-"+width+".png"));}
                        }
                        Console.WriteLine("PASS show name; blank/no-op; partial rename; save/discard/reopen; permission retry; concurrent rename; duplicate save prevention; narrow/wide layout.");
                    } catch(Exception error){code=1;Console.Error.WriteLine(error);}finally{host.Close();}
                };
                Application.Run(host);
            }
        }
        return code;
    }
}
"@
$harness=Join-Path $testRoot 'FloatingTaskNameTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingTaskNameTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingTaskNameTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Task name test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(45000)){ $process.Kill();throw 'Task name tests timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr;throw 'Task name tests failed.'}
if($process.ExitCode){throw ('Task name tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)