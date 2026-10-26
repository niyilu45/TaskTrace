param([string]$OutputDirectory = '.local-build/floating-outstanding-image-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Isolated native rows and an in-memory HTTP handler; no installed data or network.
$source = @"
using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;
internal sealed partial class FloatingWindow {
    internal FloatingWindow(string isolatedDirectory,HttpMessageHandler handler){root=isolatedDirectory;data=isolatedDirectory;url="http://127.0.0.1:3456";http=new HttpClient(handler);token="isolated";Opacity=0;ShowInTaskbar=false;Size=new Size(800,500);Controls.Add(tasks);InitializeSimpleOutstanding();simpleDetailsTesting=true;}
    static void Require(bool value,string message){if(!value)throw new Exception(message);}
    static string Picture(Color color){using(var bitmap=new Bitmap(8,8)){using(var graphics=Graphics.FromImage(bitmap))graphics.Clear(color);using(var stream=new MemoryStream()){bitmap.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return "data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());}}}
    List<GalleryImage> Images(string body,string note=null){var images=new List<GalleryImage>();CollectImages(images,body,"正文");CollectImages(images,note,"备注");return images;}
    void Row(TreeNode node,int count,string message){Require(node.Text.Contains("遗留事项正文")&&!node.Text.Contains("仅在编辑窗可见的备注"),message+": note text leaked into the floating row");Require(count==0?!Regex.IsMatch(node.Text,@"\d+ 张图片"):node.Text.Contains(count+" 张图片"),message+": wrong picture count in row: "+node.Text.Trim());Require(tasks.SimpleImageAvailable(node)==(count>0),message+": picture link availability disagreed with count");var state=(OutstandingLeaf)node.Tag;Require(OutstandingImageCount(state)==count && Images(OutstandingImageHtml(state)).Count==count,message+": clicked gallery payload disagreed with the row count");node.EnsureVisible();tasks.Refresh();Require(tasks.SimpleImageBounds(node).IsEmpty==(count==0),message+": picture link bounds were missing/stale");}
    internal async Task OutstandingImageCases(OutstandingImageHandler handler){
        string first=Picture(Color.Red),second=Picture(Color.Blue);
        var rootNode=new TreeNode("任务"){Tag=42L};tasks.Nodes.Add(rootNode);rootNode.Expand();
        var item=new PendingItem{Id="notes",Html="<p>遗留事项正文</p>",NoteHtml="<p>仅在编辑窗可见的备注</p><img src='"+first+"'><img src='"+second+"'>",Priority=7};
        foreach(bool compact in new[]{false,true})foreach(bool flat in new[]{false,true}){
            simpleMode=compact;singleLine.Checked=flat;rootNode.Nodes.Clear();var row=CreateOutstandingNode(42,item,0);rootNode.Nodes.Add(row);rootNode.Expand();await Task.Delay(10);Row(row,2,"Note-only images simple="+compact+" single="+flat);Require(Images(item.Html,item.NoteHtml).Count==2,"Note-only gallery did not contain both pictures");
        }
        Console.WriteLine("PASS note-only image count/link in full, simple, tree and single-line rows; note text remains hidden.");
        string body="<p>遗留事项正文</p><img src='/api/v1/tasks/42/attachments/1'>",notes="<p>仅在编辑窗可见的备注</p><img src='/api/v2/tasks/42/attachments/2?preview=1&amp;quality=2'><img src='"+first+"'>";
        item.Html=body;item.NoteHtml=notes;var mixed=new SharedList();mixed.Items.Add(item);ApplySimpleOutstanding(rootNode,mixed);Row(rootNode.Nodes[0],3,"Mixed body/note images");Require(Images(body,notes).Count==3,"Mixed gallery lost a body or note picture");
        string repeated="<img src='/api/v2/tasks/42/attachments/1?preview=1'><img src='http://127.0.0.1:3456/api/v1/tasks/42/attachments/1?download=1'><img src='blob:preview' data-tasktrace-src='/api/v2/tasks/42/attachments/1'><img data-src='/api/v1/tasks/42/attachments/1'><img src='"+first+"'>";
        Require(Images(body+"<img src='"+first+"'>",repeated).Count==2,"Duplicate attachment/inline pictures were counted separately across body and note");
        item.NoteHtml=repeated;ApplySimpleOutstanding(rootNode,mixed);Row(rootNode.Nodes[0],2,"Normalized repeated images");
        var displayItem=new PendingItem{Number=1,Priority=7,Html=body,NoteHtml="<img src='https://foreign.invalid/api/v2/tasks/42/attachments/8'><img src='http://127.0.0.1:3456/api/v2/tasks/42/attachments/1'>"};var displayImages=Images(displayItem.Html,displayItem.NoteHtml);string displayText=displayItem.DisplayText(url);Require(displayImages.Count==1 && displayText.Contains("1 张图片") && !displayText.Contains("2 张图片") && !displayText.Contains("3 张图片"),"Outstanding editor list label counted foreign API URLs or repeated same-origin pictures differently from the gallery");
        Console.WriteLine("PASS body+note combined count and gallery, v1/v2/query/same-origin and retained/lazy-source deduplication.");
        string hidden="<!-- <img src='/api/v1/tasks/42/attachments/91'> --><script>var x=\"<img src='/api/v1/tasks/42/attachments/92'>\";</script><style>/* <img src='/api/v1/tasks/42/attachments/93'> */</style><template><img src='/api/v1/tasks/42/attachments/94'></template><textarea><img src='/api/v1/tasks/42/attachments/95'></textarea><section hidden data-tasktrace-review-change='outstanding'><p>old</p><img src='/api/v1/tasks/42/attachments/96'></section>";
        string invalid="<img><img alt='missing'><img src='https://external.invalid/api/v2/tasks/42/attachments/97'><img src='/api/v2/tasks/42/attachments/not-an-id'><img src='data:image/png;base64,!!!'><img src='file:///private.png'>";
        Require(Images(hidden+invalid).Count==0,"Comment, inactive markup, historical snapshot or invalid/external sources appeared as pictures");
        Require(Images(hidden+"<p>当前</p><img src='/api/v1/tasks/42/attachments/3'>"+invalid).Count==1,"Filtering inactive markup also removed a current visible picture");
        string nested="<div data-tasktrace-review-change='outstanding'><div>旧</div><img src='/api/v1/tasks/42/attachments/99'></div><img src='/api/v1/tasks/42/attachments/3'>";var nestedImages=Images(nested);Require(nestedImages.Count==1 && nestedImages[0].Source=="/api/v2/tasks/42/attachments/3","Nested DIV inside a hidden snapshot ended suppression early or swallowed the following visible picture");
        Console.WriteLine("PASS comments, script/style/template/textarea, nested hidden historical snapshots and missing/invalid/foreign image sources excluded.");
        item.Html="<p>遗留事项正文</p>";item.NoteHtml="<p>仅在编辑窗可见的备注</p><img src='"+first+"'><img src='"+second+"'>";ApplySimpleOutstanding(rootNode,mixed);Row(rootNode.Nodes[0],2,"Before note deletion");
        var changed=new SharedList();changed.Items.Add(item.Copy());changed.Items[0].NoteHtml="<p>仅在编辑窗可见的备注</p><img src='"+second+"'>";Require(ApplySimpleOutstanding(rootNode,changed),"Deleting an image was not detected as a row update");Row(rootNode.Nodes[0],1,"After deleting first note image");var leaf=(OutstandingLeaf)rootNode.Nodes[0].Tag;var currentImages=Images(leaf.Html,leaf.NoteHtml);Require(currentImages.Count==1 && currentImages[0].Bytes.SequenceEqual(Convert.FromBase64String(second.Substring(second.IndexOf(',')+1))),"Deleted note image remained in refreshed row gallery");
        changed.Items[0].NoteHtml="<p>仅在编辑窗可见的备注</p>";Require(ApplySimpleOutstanding(rootNode,changed),"Removing the last image was not detected");Row(rootNode.Nodes[0],0,"After deleting last note image");leaf=(OutstandingLeaf)rootNode.Nodes[0].Tag;Require(Images(leaf.Html,leaf.NoteHtml).Count==0,"Last deleted picture remained in row gallery");
        var cached=new OutstandingLeaf{Html=body,NoteHtml=notes};Require(OutstandingImageCount(cached)==3,"Initial image summary cache missed pictures");cached.NoteHtml="<p>删除了备注图片</p>";Require(OutstandingImageCount(cached)==1,"In-place note update retained a stale cached image count");cached.Html="<p>遗留事项正文</p>";Require(OutstandingImageCount(cached)==0,"In-place body update retained the last cached image link");
        Require(OutstandingText(body,notes,url).Contains("3 张图片") && !OutstandingText(body,notes,url).Contains("仅在编辑窗可见的备注"),"Shared text summary disagreed with the parsed gallery or exposed note text");
        Console.WriteLine("PASS deleting one/last note image refreshes row count, gallery contents and picture-link visibility, including in-place cached leaf updates.");
        handler.CurrentNote="<p>当前备注</p><img src='/api/v2/tasks/42/attachments/4'>";handler.HistoricalImage="/api/v1/tasks/42/attachments/99";
        using(var gallery=new Form()){var all=await GatherImages(42,null,gallery);Require(all.Count==2 && all.Any(image=>image.Source=="/api/v2/tasks/42/attachments/4") && all.Any(image=>image.Source=="/api/v2/tasks/42/attachments/5"),"Task gallery lost a normal visible comment image or resurrected a deleted picture from older shared records/operation snapshots");var only=await GatherImages(42,handler.CurrentNote,gallery);Require(only.Count==1 && only[0].Source=="/api/v2/tasks/42/attachments/4","Scoped outstanding gallery disagreed with the current task gallery");handler.CurrentNote="<p>所有图片已删除</p>";all=await GatherImages(42,null,gallery);Require(all.Count==1 && all[0].Source=="/api/v2/tasks/42/attachments/5","Task gallery retained a deleted note image or lost the ordinary visible comment image");}
        Console.WriteLine("PASS current shared-list and ordinary visible comment images in task/scoped gallery; deleted images in older shared lists and operation snapshots do not reappear.");
    }
    internal sealed class OutstandingImageHandler:HttpMessageHandler {
        internal string CurrentNote,HistoricalImage;
        protected override Task<HttpResponseMessage> SendAsync(HttpRequestMessage request,CancellationToken cancellation){
            object value;string path=request.RequestUri.AbsolutePath;
            if(path.EndsWith("/comments")){
                string heading="<h3 "+SharedTypeMarker+">"+SharedHeading+"</h3><ul><li data-id='current'>遗留事项正文<aside data-tasktrace-outstanding-note='true' hidden>";
                value=new{items=new object[]{new{id=30,comment=heading+CurrentNote+"</aside></li></ul>"},new{id=20,comment=heading+"<img src='"+HistoricalImage+"'></aside></li></ul>"},new{id=15,comment="<p>普通可见评论图片</p><img src='/api/v1/tasks/42/attachments/5'>"},new{id=10,comment="<p>遗留事项操作记录</p><section data-tasktrace-review-change='outstanding' hidden><img src='"+HistoricalImage+"'></section>"}},total_pages=1};
            }else if(path.EndsWith("/tasks/42"))value=new{id=42,title="隔离任务",description="",project_id=1,related_tasks=new Dictionary<string,object>()};
            else throw new Exception("Unexpected request: "+request.RequestUri);
            var serializer=new System.Web.Script.Serialization.JavaScriptSerializer();return Task.FromResult(new HttpResponseMessage(HttpStatusCode.OK){Content=new StringContent(serializer.Serialize(value),Encoding.UTF8,"application/json")});
        }
    }
}
static class FloatingOutstandingImageTests {
    [STAThread]static int Main(string[] args){Console.OutputEncoding=Encoding.UTF8;Application.EnableVisualStyles();int code=0;var handler=new FloatingWindow.OutstandingImageHandler();using(var host=new FloatingWindow(args[0],handler)){host.Shown+=async delegate{try{await host.OutstandingImageCases(handler);}catch(Exception error){code=1;Console.Error.WriteLine(error);}finally{host.Close();}};Application.Run(host);}return code;}
}
"@
$harness=Join-Path $testRoot 'FloatingOutstandingImageTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingOutstandingImageTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingOutstandingImageTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Outstanding image test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -ArgumentList ('"'+$testRoot+'"') -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(30000)){$process.Kill();throw ('Outstanding image tests timed out: '+$testRoot)}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw ('Outstanding image tests failed: '+$testRoot)}
if($process.ExitCode){throw ('Outstanding image tests failed: '+$process.ExitCode+'; '+$testRoot)}
Write-Output ('Results: '+$testRoot)
