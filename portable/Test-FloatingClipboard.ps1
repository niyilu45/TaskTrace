param([string]$OutputDirectory = '.local-build/floating-clipboard-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# In-memory clipboard payloads exercise the real editor without reading/replacing
# the user's clipboard, starting an installed app, or accessing personal data.
$source = @"
using System;
using System.Drawing;
using System.IO;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;
static class FloatingClipboardTests {
    static void Require(bool ok,string message){if(!ok)throw new Exception(message);}
    static byte[] CfHtml(string fragment,Encoding encoding,bool markers) {
        string header="Version:1.0\r\nStartHTML:{0:D10}\r\nEndHTML:{1:D10}\r\nStartFragment:{2:D10}\r\nEndFragment:{3:D10}\r\n";
        string prefix="<html><head><meta charset='"+encoding.WebName+"'></head><body>片段之前"+(markers?"<!--StartFragment-->":"");
        string suffix=(markers?"<!--EndFragment-->":"")+"片段之后</body></html>";
        int startHtml=encoding.GetByteCount(String.Format(header,0,0,0,0));
        int startFragment=startHtml+encoding.GetByteCount(prefix),endFragment=startFragment+encoding.GetByteCount(fragment);
        return encoding.GetBytes(String.Format(header,startHtml,endFragment+encoding.GetByteCount(suffix),startFragment,endFragment)+prefix+fragment+suffix+"\0");
    }
    static string Read(byte[] bytes,string text,int codePage){return FloatingClipboard.PrepareHtml(bytes,null,text,codePage);}
    static void DecodeCases() {
        string text="中文、标点‘引号’和 emoji 😀\n下一行",fragment="<p>中文、标点‘引号’和 emoji 😀<br>下一行</p>";
        foreach(bool markers in new[]{true,false})Require(Read(CfHtml(fragment,Encoding.UTF8,markers),text,936)==fragment,"CF_HTML byte offsets damaged Chinese/emoji or included unselected context");
        string chinese="设备检查\n第二行",legacy="<p style='margin-left:36pt'>设备检查<br>第二行</p>";
        var gb=Encoding.GetEncoding(936);byte[] raw=CfHtml(legacy,gb,true);
        Require(Encoding.UTF8.GetString(raw).Contains("\ufffd"),"Legacy encoding fixture does not reproduce the previous lossy decoder");
        Require(Read(raw,chinese,1252)==legacy,"Declared GBK charset did not recover the original Unicode text");
        Require(Read(raw,null,936)==legacy,"HTML-only legacy source could not be decoded");
        foreach(var encoding in new[]{Encoding.Unicode,Encoding.BigEndianUnicode})Require(Read(encoding.GetBytes("<html><!--StartFragment-->"+fragment+"<!--EndFragment--></html>\0"),text,1252)==fragment,"UTF-16 clipboard HTML was read as UTF-8");
        string corrupt=Encoding.GetEncoding(28591).GetString(Encoding.UTF8.GetBytes(fragment));
        Require(Read(CfHtml(corrupt,Encoding.UTF8,true),text,936)==fragment,"Reversible mojibake was not repaired against Unicode clipboard text");
        Require(FloatingClipboard.PrepareHtml(null,"<p>\ufffd\ufffd</p>","中文",936)=="<p>中文</p>","Broken HTML did not fall back to intact Unicode text");
        Require(Read(gb.GetBytes("<p>设备检查</p>"),"设备检查",1252)=="<p>设备检查</p>","Undeclared legacy charset left mojibake instead of using Unicode text");
        Require(FloatingClipboard.PrepareHtml(null,"Header\r\n<html><!-- StartFragment -->"+fragment+"<!-- EndFragment --></html>\0",text,936)==fragment,"Spaced fragment markers or clipboard NUL leaked into content");
        Require(FloatingClipboard.PrepareHtml(null,"<p>UTF-8 &amp; 中文</p>","UTF-8 & 中文",936)=="<p>UTF-8 &amp; 中文</p>","HTML entities or valid Unicode text changed");
        Require(FloatingClipboard.PrepareHtml(null,"<p>????</p>","中文测试",936)=="<p>中文测试</p>","Lossy question marks overrode intact Unicode text");
        Require(FloatingClipboard.PrepareHtml(null,"<p>真的??</p>","真的??",936)=="<p>真的??</p>","Intentional question marks were mistaken for mojibake");
        byte[] terminated=CfHtml(fragment,Encoding.UTF8,true),padded=new byte[terminated.Length+4];Array.Copy(terminated,padded,terminated.Length);padded[padded.Length-1]=255;
        Require(Read(padded,text,936)==fragment,"Bytes after the clipboard NUL corrupted a valid HTML payload");
        Require(FloatingClipboard.PrepareHtml(null,null,null,936)=="","Empty clipboard produced content");
        Require(FloatingClipboard.TextHtml("one two  three")=="<p>one two\u00a0 three</p>","Ordinary word spaces became nonbreaking and prevented wrapping");
        string lines=FloatingClipboard.TextHtml("  缩进\r\n\r\n\t下一行 <>&\r\n");
        Require(lines=="<p>\u00a0\u00a0缩进<br><br>&emsp;下一行 &lt;&gt;&amp;<br></p>","Plain text lost spaces/indentation/empty lines or was interpreted as markup");
        Console.WriteLine("PASS UTF-8/GBK/UTF-16, byte offsets, Unicode fallback, reversible mojibake, entities, fragment markers, blank lines and indentation.");
    }
    static async Task EditorCases(Form host) {
        string payload="";
        using(var editor=new ProgressHtmlEditor(()=>payload){Dock=DockStyle.Fill}) {
            host.Controls.Add(editor);var browser=(WebBrowser)editor.Controls[0];
            for(int i=0;i<100 && (browser.Document==null || browser.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(40);
            Require(browser.Document!=null && browser.Document.GetElementById("progress-editor")!=null,"Editor did not load");
            string picture;using(var image=new Bitmap(2,2))using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);picture="data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());}
            string original="<p style='font-size:40pt;color:white;background:black;margin-left:36pt;text-indent:12pt'><b>中文😀在图片前</b><br><img src='"+picture+"'><br>中文在图片后</p>";
            payload=Read(CfHtml(original,Encoding.UTF8,true),"中文😀在图片前\n中文在图片后",936);
            editor.Html="<p>已有文字</p>";
            browser.Document.InvokeScript("eval",new object[]{"var e=document.getElementById('progress-editor'),r=document.createRange(),s=window.getSelection();r.selectNodeContents(e);r.collapse(false);s.removeAllRanges();s.addRange(r);pasteTaskTrace({preventDefault:function(){window.pasteCancelled=true;}});"});
            await Task.Delay(80);
            string saved=editor.Html;
            Require(saved.IndexOf("已有文字")<saved.IndexOf("中文😀在图片前") && saved.IndexOf("中文😀在图片前")<saved.IndexOf("data:image/png") && saved.IndexOf("data:image/png")<saved.IndexOf("中文在图片后"),"Paste changed text/image order, damaged Unicode or overwrote existing text");
            Require(editor.ImageCount==1 && editor.ChangeVersion>0,"Paste duplicated/lost its image or did not update the draft");
            Require(!saved.Contains("font-size") && !saved.Contains("background") && !saved.Contains("color:") && !saved.Contains("<b>") && saved.Contains("36pt") && saved.Contains("12pt") && saved.Contains("<br"),"Paste retained source fonts/theme or lost structure");
            Require(Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled"})),"Native paste was not cancelled");
            editor.Html=saved;Require(editor.Html.Contains("中文😀在图片前") && editor.ImageCount==1,"Reopening saved HTML changed Unicode or lost images");
            payload=FloatingClipboard.TextHtml("中文\r\n\r\n下一行");editor.Html="";
            browser.Document.InvokeScript("eval",new object[]{"pasteTaskTrace({preventDefault:function(){}});"});
            Require(editor.Html.Contains("中文<br><br>下一行"),"Plain Unicode paste lost blank lines in the real editor");
            editor.SetReadOnly(true);string readOnly=editor.Html;
            browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled=false;pasteTaskTrace({preventDefault:function(){window.pasteCancelled=true;}});"});
            Require(editor.Html==readOnly && Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled"})),"Read-only editor allowed native paste");
            editor.SetReadOnly(false);payload="";
            browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled=false;pasteTaskTrace({preventDefault:function(){window.pasteCancelled=true;}});"});
            Require(editor.Html==readOnly && Convert.ToBoolean(browser.Document.InvokeScript("eval",new object[]{"window.pasteCancelled"})),"Empty/unavailable clipboard changed content or fell through to native paste");
        }
        Console.WriteLine("PASS actual editor Unicode/image paste, formatting cleanup, existing text, draft change, roundtrip, read-only and empty clipboard.");
    }
    [STAThread] static int Main(){Console.OutputEncoding=Encoding.UTF8;int code=0;Application.EnableVisualStyles();using(var host=new Form{Opacity=0,ShowInTaskbar=false}){host.Shown+=async delegate{try{DecodeCases();await EditorCases(host);}catch(Exception error){code=1;Console.Error.WriteLine(error);}finally{host.Close();}};Application.Run(host);}return code;}
}
"@
$harness=Join-Path $testRoot 'FloatingClipboardTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingClipboardTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingClipboardTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Clipboard test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(30000)){ $process.Kill();throw 'Clipboard tests timed out.' }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw 'Clipboard tests failed.'}
if($process.ExitCode){throw ('Clipboard tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)