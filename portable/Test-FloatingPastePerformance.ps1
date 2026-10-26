param([string]$OutputDirectory = '.local-build/floating-paste-performance-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# IDataObject fixtures avoid the system clipboard and installed application data.
$source = @"
using System;
using System.Diagnostics;
using System.Drawing;
using System.Drawing.Imaging;
using System.IO;
using System.Runtime.InteropServices;
using System.Reflection;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
sealed class PasteTrackingData : IDataObject {
    readonly IDataObject source;internal int BitmapReads;internal bool RejectBitmap;
    internal PasteTrackingData(IDataObject value){source=value;}
    public object GetData(string format,bool autoConvert){if(format==DataFormats.Bitmap){BitmapReads++;if(RejectBitmap)throw new Exception("Complete HTML unnecessarily requested bitmap encoding");}return source.GetData(format,autoConvert);}
    public object GetData(string format){return GetData(format,true);}public object GetData(Type format){return source.GetData(format);}
    public bool GetDataPresent(string format,bool autoConvert){return source.GetDataPresent(format,autoConvert);}public bool GetDataPresent(string format){return source.GetDataPresent(format);}public bool GetDataPresent(Type format){return source.GetDataPresent(format);}
    public string[] GetFormats(bool autoConvert){return source.GetFormats(autoConvert);}public string[] GetFormats(){return source.GetFormats();}
    public void SetData(string format,bool autoConvert,object value){source.SetData(format,autoConvert,value);}public void SetData(string format,object value){source.SetData(format,value);}public void SetData(Type format,object value){source.SetData(format,value);}public void SetData(object value){source.SetData(value);}
}
static class FloatingPastePerformanceTests {
    static void Require(bool value,string message){if(!value)throw new Exception(message);}
    static Bitmap NoiseBitmap(){var image=new Bitmap(2000,1200);var bounds=new Rectangle(0,0,image.Width,image.Height);var bits=image.LockBits(bounds,ImageLockMode.WriteOnly,PixelFormat.Format32bppArgb);try{var bytes=new byte[Math.Abs(bits.Stride)*image.Height];new Random(17).NextBytes(bytes);for(int i=3;i<bytes.Length;i+=4)bytes[i]=255;Marshal.Copy(bytes,0,bits.Scan0,bytes.Length);}finally{image.UnlockBits(bits);}return image;}
    static void ValidateBitmapHtml(string html,Color expected){Require(html.StartsWith("<img src=\"data:image/png;base64,"),"Bitmap fallback did not produce a PNG");int start=html.IndexOf(',')+1;using(var stream=new MemoryStream(Convert.FromBase64String(html.Substring(start,html.IndexOf('\"',start)-start))))using(var image=new Bitmap(stream)){Require(image.Width==2000 && image.Height==1200 && image.GetPixel(10,10).ToArgb()==expected.ToArgb(),"Deferred bitmap conversion used changed or disposed clipboard content");}}
    static byte[] DeclaredHtml(string fragment,Encoding encoding) {
        string format="Version:1.0\r\nStartHTML:{0:D10}\r\nEndHTML:{1:D10}\r\nStartFragment:{2:D10}\r\nEndFragment:{3:D10}\r\n";
        string prefix="<html><head><!--"+new string('x',5000)+"--><meta charset='"+encoding.WebName+"'></head><body>outside<!--StartFragment-->",suffix="<!--EndFragment-->outside</body></html>";
        int start=encoding.GetByteCount(String.Format(format,0,0,0,0)),from=start+encoding.GetByteCount(prefix),to=from+encoding.GetByteCount(fragment);
        return encoding.GetBytes(String.Format(format,start,to+encoding.GetByteCount(suffix),from,to)+prefix+fragment+suffix+"\0");
    }
    static void LateCharsetCases(){string fragment="<p>设备检查<br>下一行<img src='/api/v1/tasks/1/attachments/2'></p>";var bytes=DeclaredHtml(fragment,Encoding.GetEncoding(936));Require(FloatingClipboard.PrepareHtml(bytes,null,"设备检查\n下一行",1252)==fragment,"Long-header GBK declaration lost to non-Chinese system ANSI encoding");Require(FloatingClipboard.PrepareHtml(bytes,null,null,1252)==fragment,"HTML-only long-header GBK declaration lost its original encoding priority");Console.WriteLine("PASS charset declarations beyond 4 KB retain GBK byte-offset fragments, with and without Unicode text on a non-Chinese ANSI system.");}
    static object Eval(WebBrowser browser,string code){return browser.Document.InvokeScript("eval",new object[]{code});}
    static async Task EditorCases(Form host,byte[] png) {
        using(var editor=new ProgressHtmlEditor(()=>""){Dock=DockStyle.Fill}) {
            host.Controls.Add(editor);var browser=(WebBrowser)editor.Controls[0];for(int i=0;i<100&&(browser.Document==null||browser.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(25);
            Require(browser.Document!=null&&browser.Document.GetElementById("progress-editor")!=null,"Performance editor not ready");string fragment="<p>before<img src='data:image/png;base64,"+Convert.ToBase64String(png)+"'>after</p>";editor.Html=fragment;await Task.Delay(40);
            var timer=Stopwatch.StartNew();int key=Convert.ToInt32(Eval(browser,"(function(){var state;for(var i=0;i<20;i++)state=captureTaskTraceHistory();return state.key.length})()"));timer.Stop();
            Require(key<4096 && Convert.ToInt32(Eval(browser,"Object.keys(taskTraceHistory.media).length"))==1,"History snapshots duplicated inline media payloads");Console.WriteLine("PASS actual 8 MB editor snapshots reuse one media entry; 20_captures_ms="+timer.ElapsedMilliseconds+", snapshot_key_chars="+key);
            var notifying=typeof(ProgressHtmlEditor).GetField("notifying",BindingFlags.Instance|BindingFlags.NonPublic);var observed=typeof(ProgressHtmlEditor).GetField("observedHtml",BindingFlags.Instance|BindingFlags.NonPublic);Require(notifying!=null&&observed!=null,"Editor cache fields unavailable");string previous=(string)observed.GetValue(editor);observed.SetValue(editor,"<p>cached sentinel without images</p>");notifying.SetValue(editor,true);
            try{timer.Restart();for(int i=0;i<100;i++)Require(editor.ImageCount==1,"ImageCount read/counted serialized cached HTML instead of current DOM images");timer.Stop();Console.WriteLine("PASS 100 ImageCount reads use DOM without serializing large HTML; elapsed_ms="+timer.ElapsedMilliseconds);}finally{notifying.SetValue(editor,false);observed.SetValue(editor,previous);}
            timer.Restart();for(int i=0;i<3;i++)Require(editor.Html.Length>png.Length,"Large editor HTML fixture unexpectedly empty");timer.Stop();Console.WriteLine("INFO three full HTML reads for comparison; elapsed_ms="+timer.ElapsedMilliseconds);
        }
    }
    [STAThread] static int Main(){try{
        LateCharsetCases();byte[] png;Color original;using(var bitmap=NoiseBitmap()){
            original=bitmap.GetPixel(10,10);png=FloatingClipboard.PngBytes(bitmap);
            string fragment="<p>中文😀before<img src='data:image/png;base64,"+Convert.ToBase64String(png)+"'>after</p>";
            var copied=FloatingClipboard.CopyData(fragment,"中文😀beforeafter");copied.SetData(DataFormats.Bitmap,false,bitmap);
            var timer=Stopwatch.StartNew();var snapshot=FloatingClipboard.SnapshotData(copied);timer.Stop();
            try{Require(!snapshot.GetDataPresent("PNG",false),"UI snapshot encoded an unused bitmap into PNG");Require(snapshot.GetDataPresent(DataFormats.Bitmap,false),"UI snapshot did not retain bitmap fallback");Require(!Object.ReferenceEquals(snapshot.GetData(DataFormats.Bitmap,false),bitmap),"UI snapshot retained source-owned bitmap");var tracked=new PasteTrackingData(snapshot){RejectBitmap=true};string html=Task.Run(()=>FloatingClipboard.ReadData(tracked)).GetAwaiter().GetResult();Require(html==fragment && tracked.BitmapReads==0,"Complete mixed HTML was changed or unnecessarily encoded fallback bitmap");Console.WriteLine("PASS complete mixed HTML keeps Chinese/emoji/image order and skips bitmap encoding; snapshot_ms="+timer.ElapsedMilliseconds+", png_bytes="+png.Length);}
            finally{FloatingClipboard.DisposeCopyData(snapshot);var owned=copied.GetData(DataFormats.Html,false) as IDisposable;if(owned!=null)owned.Dispose();}
            var source=new DataObject();source.SetData(DataFormats.Bitmap,false,bitmap);snapshot=FloatingClipboard.SnapshotData(source);Require(!snapshot.GetDataPresent("PNG",false),"Bitmap-only snapshot encoded PNG on UI thread");bitmap.SetPixel(10,10,Color.Red);bitmap.Dispose();
            try{string html=Task.Run(()=>FloatingClipboard.ReadData(snapshot)).GetAwaiter().GetResult();ValidateBitmapHtml(html,original);Console.WriteLine("PASS bitmap-only paste defers conversion and survives source mutation/disposal.");}finally{FloatingClipboard.DisposeCopyData(snapshot);}
        }
        foreach(bool streamFormat in new[]{true,false}){
            var source=new DataObject();byte[] supplied=(byte[])png.Clone();MemoryStream sourceStream=null;if(streamFormat){sourceStream=new MemoryStream(supplied,false);source.SetData("PNG",false,sourceStream);}else source.SetData("PNG",false,supplied);
            var tracked=new PasteTrackingData(source){RejectBitmap=true};var snapshot=FloatingClipboard.SnapshotData(tracked);try{Require(tracked.BitmapReads==0 && snapshot.GetDataPresent("PNG",false),"PNG snapshot needlessly requested bitmap conversion");supplied[0]=0;if(sourceStream!=null)sourceStream.Dispose();var actual=FloatingClipboard.ImageBytes(snapshot);Require(actual.Length==png.Length && actual[0]==png[0] && actual[actual.Length-1]==png[png.Length-1],"PNG snapshot retained mutable source bytes");}finally{FloatingClipboard.DisposeCopyData(snapshot);}
        }
        Console.WriteLine("PASS PNG stream/byte-array snapshots keep original encoded bytes and independent ownership.");
        Exception failure=null;Application.EnableVisualStyles();using(var host=new Form{ShowInTaskbar=false,Opacity=0,Width=600,Height=400}){host.Shown+=async delegate{try{await EditorCases(host,png);}catch(Exception error){failure=error;}finally{host.Close();}};Application.Run(host);}if(failure!=null)throw failure;
        Console.WriteLine("All clipboard performance/ownership regressions passed without accessing the system clipboard.");return 0;
    }catch(Exception error){Console.Error.WriteLine(error);return 1;}}
}
"@
$harness=Join-Path $testRoot 'FloatingPastePerformanceTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingPastePerformanceTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName) + @($harness)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingPastePerformanceTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources
if($LASTEXITCODE -ne 0){throw 'Clipboard performance test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(45000)){$process.Kill();throw 'Clipboard performance tests timed out.'}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw 'Clipboard performance tests failed.'}
if($process.ExitCode){throw ('Clipboard performance tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)
