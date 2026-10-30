param([string]$OutputDirectory = '.local-build/floating-history-details-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Exercise the real history readers and dialogs with isolated in-memory records.
# No installed application, account, database, network or clipboard is accessed.
$source = @'
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Reflection;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    internal FloatingWindow(string isolatedDirectory,bool testMode=true){root=isolatedDirectory;data=isolatedDirectory;url="http://127.0.0.1:1";selfTest=testMode;}
    static void Require(bool ok,string message){if(!ok)throw new Exception(message);}
    static Dictionary<string,object> HistoryNote(long id,string date,string html){return new Dictionary<string,object>{{"id",id},{"comment","<h3>每日进展: "+date+"</h3>"+html}};}
    static T FindControl<T>(Control parent) where T:Control {if(parent is T)return (T)parent;foreach(Control child in parent.Controls){var found=FindControl<T>(child);if(found!=null)return found;}return null;}
    static byte[] Picture(Color color,string text){using(var image=new Bitmap(260,75))using(var stream=new MemoryStream()){using(var graphics=Graphics.FromImage(image)){graphics.Clear(color);graphics.DrawString(text,SystemFonts.DefaultFont,Brushes.Black,12,28);}image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return stream.ToArray();}}
    async Task TestImagePreviewTaskbar(Form owner,byte[] bytes) {
        bool checkedWindow=false;Exception failure=null;Form preview=null;
        using(var driver=new Timer{Interval=40}) {
            driver.Tick+=async delegate {
                if(checkedWindow)return;
                preview=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="查看大图 · 任务栏恢复验收");
                if(preview==null)return;checkedWindow=true;driver.Stop();
                try {
                    Require(preview.ShowInTaskbar && preview.ShowIcon && preview.Icon!=null && preview.MinimizeBox,"Enlarged image preview is missing its taskbar entry, icon or minimize button");
                    Require(!preview.Modal && preview.Owner==null && owner.Enabled,"Enlarged image preview blocks another editor");
                    preview.WindowState=FormWindowState.Minimized;await Task.Delay(100);
                    Require(preview.WindowState==FormWindowState.Minimized && preview.ShowInTaskbar && owner.Enabled,"Minimized image cannot be found independently in the taskbar");
                    preview.WindowState=FormWindowState.Normal;preview.Activate();await Task.Delay(100);
                    Require(preview.WindowState==FormWindowState.Normal && preview.Visible && owner.Enabled,"Image preview cannot be restored independently after minimize");
                    var image=FindControl<ZoomableImagePreview>(preview);Require(image!=null,"Restored preview lost its image control");
                    image.SimulateWheelForTest(SystemInformation.MouseWheelScrollDelta);Require(image.Zoom>1f,"Restored image preview lost wheel zoom");
                }catch(Exception error){failure=error;}finally{preview.Close();}
            };
            driver.Start();var shown=ShowImagePreview(new GalleryImage{Bytes=bytes,Caption="任务栏恢复验收"},owner);
            if(await Task.WhenAny(shown,Task.Delay(5000))!=shown){if(preview!=null && !preview.IsDisposed)preview.Close();throw new Exception("Taskbar image preview test timed out");}
            await shown;
        }
        Require(checkedWindow,"Enlarged image preview did not open");if(failure!=null)throw failure;
        Console.WriteLine("PASS enlarged-image taskbar icon; independent minimize/restore; owner remains enabled; wheel zoom after restore.");
    }
    internal async Task TestHistoryDetails(Form owner) {
        using(var editor=new ProgressHtmlEditor{Dock=DockStyle.Fill}) {
            owner.Controls.Add(editor);editor.Html="<p>start</p>";
            var browser=FindControl<WebBrowser>(editor);
            for(int wait=0;wait<60 && (browser.Document==null || browser.Document.GetElementById("progress-editor")==null);wait++)await Task.Delay(40);
            string pasted=Convert.ToString(browser.Document.InvokeScript("cleanTaskTracePaste",new object[]{"<p style='font-size:40pt;color:white;background:black;margin-left:36pt;text-indent:12pt'><b>before</b><br>after</p><img src='data:image/png;base64,AQID'>"}));
            Require(!Regex.IsMatch(pasted,@"font-size|color|background|<b>",RegexOptions.IgnoreCase) && pasted.Contains("36pt") && pasted.Contains("12pt") && pasted.Contains("before") && pasted.Contains("<br") && pasted.Contains("data:image/png"),"Word paste lost text/image structure or retained its font theme");
            string saved=ProgressSnapshotHtml(pasted,42);Require(saved.Contains("36pt") && saved.Contains("12pt") && saved.Contains("<br"),"Saving removed pasted indentation or line breaks");owner.Controls.Remove(editor);
        }
        var bytes=new[]{Picture(Color.LightSkyBlue,"Image A"),Picture(Color.PaleGreen,"Image B")};
        string body="<p><strong>检查设备</strong>，第一张图片如下：</p><p><img src=\"/api/v1/tasks/42/attachments/1\"></p><p>处理完成后，第二张图片如下：<br>保留这一行换行。</p><p><img src=\"/api/v1/tasks/42/attachments/2\"></p><ul><li>确认结果</li></ul>";
        var reference=new ProgressReference{Id="r"+new string('a',32),TaskId=42,Date="2026-10-11",CommentIds=new List<long>{1},Html="<p>旧记录说明</p><img src=\"/api/v1/tasks/42/attachments/1\">"};
        var notes=new List<Dictionary<string,object>>{HistoryNote(1,"2026-10-11","<p>纯文字进展</p>"),HistoryNote(2,"2026-10-13",body+SerializeProgressReferences(new[]{reference})+FloatingTeamMarkerHtml("one","张三")),HistoryNote(3,"2026-10-13","<p>另一位成员确认完成。</p>"+FloatingTeamMarkerHtml("two","李四"))};
        var rows=ProgressHistoryRows(42,notes);Require(rows.Count==2 && rows[0].Date=="2026-10-13" && rows[0].ImageCount==2 && rows[1].ImageCount==0,"History grouping, date order or unique gallery count changed");
        Require(rows[0].Html.IndexOf("张三")<rows[0].Html.IndexOf("检查设备") && rows[0].Html.IndexOf("引用 2026-10-11")<rows[0].Html.IndexOf("李四"),"Author or reference contents moved to another member");
        var downloads=new List<string>();Func<string,Task<byte[]>> download=path=>{downloads.Add(path);return Task.FromResult(bytes[path.EndsWith("/2")?1:0]);};
        string document=await ProgressHistoryDocument(rows[0].Html,download);
        Require(downloads.Count==2 && downloads.All(path=>path.StartsWith("/api/v2/tasks/42/attachments/")),"Pictures were downloaded repeatedly or from an external source");
        Require(document.IndexOf("检查设备")<document.IndexOf(Convert.ToBase64String(bytes[0])) && document.IndexOf(Convert.ToBase64String(bytes[0]))<document.IndexOf("处理完成后") && document.IndexOf("处理完成后")<document.IndexOf(Convert.ToBase64String(bytes[1])) && document.Contains("<strong>检查设备</strong>") && document.Contains("<br>") && document.Contains("<ul><li>确认结果"),"Text, formatting or image order was lost");
        string blocked=await ProgressHistoryDocument("<script>alert(1)</script><img src=\"https://example.com/a.png\"><p onclick=\"run()\">safe</p><iframe src=\"file:///secret\"></iframe>",download);
        Require(!blocked.Contains("script>") && !blocked.Contains("onclick") && !blocked.Contains("example.com") && !blocked.Contains("iframe") && downloads.Count==2,"Untrusted progress loaded active or remote content");
        string failed=await ProgressHistoryDocument("<p>before</p><img src=\"/api/v1/tasks/42/attachments/1\"><p>after</p>",path=>{throw new IOException("offline");});
        Require(failed.Contains("before</p><span>（图片加载失败") && failed.Contains("</span><p>after"),"Image failure removed neighbouring text");
        File.WriteAllText(Path.Combine(data,"history-details.html"),document);
        // Exercise the production history dialog and its Complete information button with inline images.
        var inlineNotes=notes.Select(note=>new Dictionary<string,object>(note)).ToList();
        foreach(var note in inlineNotes){string html=(string)note["comment"];for(int i=0;i<2;i++)html=html.Replace("/api/v1/tasks/42/attachments/"+(i+1),"data:image/png;base64,"+Convert.ToBase64String(bytes[i]));note["comment"]=html;}
        using(var previewWindow=new FloatingWindow(data,false))await previewWindow.TestImagePreviewTaskbar(owner,bytes[0]);
        await ShowImagePreview(new GalleryImage{Bytes=bytes[0],Caption="small"},owner,true);
        using(var small=Image.FromFile(Path.Combine(data,"floating-image-preview-test.png")))Require(small.Width<650 && small.Height<480,"Small pictures still open in a large fixed window");
        using(var huge=new Bitmap(3000,2200))using(var stream=new MemoryStream()){
            huge.Save(stream,System.Drawing.Imaging.ImageFormat.Png);
            await ShowImagePreview(new GalleryImage{Bytes=stream.ToArray(),Caption="large"},owner,true);
            using(var rendered=Image.FromFile(Path.Combine(data,"floating-image-preview-test.png"))){var screen=Screen.FromControl(owner).WorkingArea;Require(rendered.Width<=screen.Width && rendered.Height<=screen.Height && rendered.Width>650,"Large pictures exceed the screen or fail to resize the preview");}
        }
        Exception failure=null;bool handled=false;
        using(var driver=new Timer{Interval=40}) {
            driver.Tick+=async delegate {
                if(handled)return;var history=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text=="所有进展 · 验收任务");if(history==null)return;handled=true;driver.Stop();
                try {
                    var table=FindControl<DataGridView>(history);Require(table!=null && (string)table.Rows[0].Cells[2].Value=="完整信息" && (string)table.Rows[1].Cells[1].Value=="","History actions are missing or empty days have image links");
                    var links=table.Columns[1] as DataGridViewLinkColumn;Require(links!=null && links.LinkBehavior==LinkBehavior.AlwaysUnderline && links.LinkColor.B>links.LinkColor.R,"Image count is not a blue underlined link");
                    foreach(int width in new[]{760,480}){history.Width=width;history.PerformLayout();table.ClearSelection();await Task.Delay(100);using(var bitmap=new Bitmap(history.Width,history.Height)){history.DrawToBitmap(bitmap,new Rectangle(Point.Empty,history.Size));bitmap.Save(Path.Combine(data,"history-list-"+width+".png"));}Require(table.Columns[0].Width>180,"Narrow history leaves no room for progress text");}
                    typeof(DataGridView).GetMethod("OnCellContentClick",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(table,new object[]{new DataGridViewCellEventArgs(1,0)});
                    Require(File.Exists(Path.Combine(data,"floating-gallery-test.png")),"Image count link did not open the existing image gallery");
                    typeof(DataGridView).GetMethod("OnCellContentClick",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(table,new object[]{new DataGridViewCellEventArgs(2,0)});
                    Form details=null;WebBrowser browser=null;
                    for(int wait=0;wait<80;wait++){await Task.Delay(50);details=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text.StartsWith("完整信息 ·"));if(details!=null){browser=FindControl<WebBrowser>(details);if(browser!=null && browser.Document!=null && browser.Document.Body!=null && browser.Document.Body.InnerText.Contains("检查设备"))break;}}
                    Require(details!=null && browser!=null && browser.Document!=null && browser.Document.Body.InnerText.Contains("李四"),"Complete information button did not open all collaborators' content");
                    Require(browser.Document.Images.Count==3 && browser.Document.GetElementsByTagName("strong").Count>=2 && browser.Document.GetElementsByTagName("ul").Count==1,"Rendered details lost pictures or rich formatting");
                    foreach(int width in new[]{760,480}){details.Width=width;details.PerformLayout();await Task.Delay(150);Require(browser.Document.Body.ScrollRectangle.Width<=browser.ClientSize.Width,"Narrow details overflow horizontally");using(var bitmap=new Bitmap(details.Width,details.Height)){details.DrawToBitmap(bitmap,new Rectangle(Point.Empty,details.Size));bitmap.Save(Path.Combine(data,"history-details-"+width+".png"));}}
                    Require(browser.Document.GetElementsByTagName("section").Count==1,"Other dates loaded before scrolling");
                    browser.Document.InvokeScript("eval",new object[]{"window.scrollTo(0,document.body.scrollHeight);historyWheel({wheelDelta:-120});historyWheel({wheelDelta:-120});"});
                    for(int wait=0;wait<60 && browser.Document.GetElementsByTagName("section").Count<2;wait++)await Task.Delay(40);
                    Require(browser.Document.GetElementsByTagName("section").Count==2,"Scrolling past the bottom did not load the older date once");
                    Require(browser.Document.GetElementById("history-2026-10-11")!=null,"Loaded the wrong adjacent date");
                    browser.Document.InvokeScript("eval",new object[]{"openHistoryImage({target:document.images[0]});"});
                    for(int wait=0;wait<60 && !File.Exists(Path.Combine(data,"floating-image-preview-test.png"));wait++)await Task.Delay(40);
                    Require(File.Exists(Path.Combine(data,"floating-image-preview-test.png")),"History image double-click did not open fitted preview");
                    details.Close();
                    typeof(DataGridView).GetMethod("OnCellContentClick",BindingFlags.Instance|BindingFlags.NonPublic).Invoke(table,new object[]{new DataGridViewCellEventArgs(2,1)});
                    for(int wait=0;wait<60;wait++){await Task.Delay(40);details=Application.OpenForms.Cast<Form>().FirstOrDefault(form=>form.Text.StartsWith("完整信息 ·"));if(details!=null){browser=FindControl<WebBrowser>(details);if(browser!=null&&browser.Document!=null&&browser.Document.GetElementById("history-2026-10-11")!=null)break;}}
                    Require(browser.Document.GetElementsByTagName("section").Count==1,"Selecting an older day preloaded another day");
                    browser.Document.InvokeScript("eval",new object[]{"historyWheel({wheelDelta:120});"});
                    for(int wait=0;wait<60 && browser.Document.GetElementsByTagName("section").Count<2;wait++)await Task.Delay(40);
                    Require(browser.Document.GetElementById("history-2026-10-13")!=null,"Top scroll did not load the newer day");
                    int offset=browser.Document.GetElementById("history-2026-10-11").OffsetRectangle.Top;
                    Require(browser.Document.Window.Position.Y>=0 && offset>0,"Prepending a day did not retain the old section");
                    details.Close();
                    Console.WriteLine("PASS rich text/image order; authors/references; unique image links; text-only entries; failed images; passive content; image-count gallery click; Complete information click; desktop/narrow layout; deferred adjacent days; repeated wheel guarded; image double-click preview.");
                }catch(Exception error){failure=error;}finally{foreach(var open in Application.OpenForms.Cast<Form>().Where(form=>form.Text.StartsWith("完整信息 ·")).ToList())open.Close();history.Close();}
            };
            driver.Start();await ShowProgressHistory(42,"验收任务",inlineNotes,owner);
        }
        if(failure!=null)throw failure;
    }
}
static class FloatingHistoryDetailsTests {
    [STAThread] static int Main(){Application.EnableVisualStyles();int code=0;using(var host=new Form{Opacity=0,ShowInTaskbar=false}){host.Shown+=async delegate{try{using(var window=new FloatingWindow(AppDomain.CurrentDomain.BaseDirectory))await window.TestHistoryDetails(host);}catch(Exception error){code=1;Console.Error.WriteLine(error);}finally{host.Close();}};Application.Run(host);}return code;}
}
'@
$harness=Join-Path $testRoot 'FloatingHistoryDetailsTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingHistoryDetailsTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingHistoryDetailsTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'History details test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(45000)){ $process.Kill();throw 'History details tests timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr;throw 'History details tests failed.'}
if($process.ExitCode){throw ('History details tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)