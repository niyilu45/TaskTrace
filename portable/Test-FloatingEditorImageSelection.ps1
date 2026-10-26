param([string]$OutputDirectory = '.local-build/floating-editor-image-selection-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot, [StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($testRoot)
# Exercise real MSHTML focus/selection with only in-memory clipboard payloads.
$source = @"
using System;
using System.Drawing;
using System.IO;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;
using System.Windows.Forms;
static class FloatingEditorImageSelectionTests {
    static void Require(bool ok,string message){if(!ok)throw new Exception(message);}
    static object Eval(WebBrowser browser,string code){return browser.Document.InvokeScript("eval",new object[]{code});}
    static string Picture(Color color){using(var image=new Bitmap(300,190)){using(var g=Graphics.FromImage(image))g.Clear(color);using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);return "data:image/png;base64,"+Convert.ToBase64String(stream.ToArray());}}}
    static string Fixture(string[] pictures){var html=new StringBuilder();for(int n=0;n<pictures.Length;n++){for(int i=0;i<18;i++)html.Append("<p>备注行 "+n+" / "+i+"：换行、缩进与图片位置保持不变。</p>");html.Append("<p><img id='image-"+n+"' src='"+pictures[n]+"' data-tasktrace-src='/api/v1/tasks/12/attachments/"+(n+1)+"'></p>");}for(int i=0;i<12;i++)html.Append("<p>末尾备注 "+i+"</p>");return html.ToString();}
    static int Scroll(WebBrowser browser){return Convert.ToInt32(Eval(browser,"Math.round(window.pageYOffset||document.documentElement.scrollTop||document.body.scrollTop||0)"));}
    static void SetScroll(WebBrowser browser,int value){Eval(browser,"window.scrollTo(0,"+value+")");}
    static void Mouse(WebBrowser browser,int index,string kind){Eval(browser,"(function(){var image=document.getElementById('image-"+index+"'),b=image.getBoundingClientRect(),e=document.createEvent('MouseEvents');e.initMouseEvent('"+kind+"',true,true,window,"+(kind=="dblclick"?2:1)+",0,0,Math.round(b.left+10),Math.round(b.top+10),false,false,false,false,0,null);image.dispatchEvent(e);})()");}
    static void Selection(WebBrowser browser,int index,string stage){
        string actual=Convert.ToString(Eval(browser,"(function(){var s=window.getSelection();if(!s||!s.rangeCount)return 'no range';var images=s.getRangeAt(0).cloneContents().querySelectorAll('img');return images.length===1?images[0].id:'images='+images.length;})()"));
        Require(actual=="image-"+index,stage+" DOM selection points to "+actual+", expected image-"+index);
        string legacy=Convert.ToString(Eval(browser,"(function(){if(!document.selection||document.selection.type!=='Control')return 'not control';var r=document.selection.createRange();return r.length===1?r.item(0).id:'controls='+r.length;})()"));
        Require(legacy=="not control"||legacy=="image-"+index,stage+" native control selection points to another image: "+legacy);
    }
    static async Task WaitEditor(WebBrowser browser){for(int i=0;i<100 && (browser.Document==null||browser.Document.GetElementById("progress-editor")==null);i++)await Task.Delay(30);Require(browser.Document!=null&&browser.Document.GetElementById("progress-editor")!=null,"Editor did not load");await Task.Delay(50);}
    static async Task Cases(Form host) {
        var pictures=new[]{Picture(Color.Red),Picture(Color.Green),Picture(Color.Blue)};
        string copied="",source="",retained="";int copies=0,opened=0;Form preview=null;ProgressEditorImageEventArgs activeImage=null;
        using(var blur=new TextBox{Dock=DockStyle.Bottom,Height=22})using(var editor=new ProgressHtmlEditor(()=>"",(html,text)=>{copied=html;copies++;return true;}){Dock=DockStyle.Fill}) {
            host.Controls.Add(editor);host.Controls.Add(blur);var browser=(ProgressEditorBrowser)editor.Controls[0];await WaitEditor(browser);
            editor.ImageDoubleClicked+=delegate(object sender,ProgressEditorImageEventArgs e){source=e.Source;retained=e.RetainedSource;activeImage=e;opened++;preview=new Form{Opacity=0,ShowInTaskbar=false,Width=250,Height=180};preview.Controls.Add(new TextBox{Dock=DockStyle.Fill});preview.FormClosed+=delegate{e.RestoreEditorView(true);};preview.Show();preview.Activate();preview.Controls[0].Focus();};
            foreach(bool readOnly in new[]{false,true}) {
                editor.Html=Fixture(pictures);editor.SetReadOnly(readOnly);await Task.Delay(60);int baseline=editor.ChangeVersion;string original=editor.Html;
                foreach(int index in new[]{1,2}) {
                    editor.FocusEditor();Mouse(browser,0,"mousedown");Mouse(browser,0,"mouseup");await Task.Delay(30);blur.Focus();
                    Eval(browser,"document.getElementById('image-"+index+"').scrollIntoView(true);window.scrollBy(0,-24)");await Task.Delay(25);int before=Scroll(browser);Require(before>400,"Fixture did not scroll below the first image");
                    Mouse(browser,index,"mousedown");await Task.Delay(25);Mouse(browser,index,"mouseup");await Task.Delay(25);
                    Console.WriteLine("Click readOnly="+readOnly+" image="+index+" before="+before+" after="+Scroll(browser));
                    Require(Math.Abs(Scroll(browser)-before)<=2,"Clicking image "+index+" changed editor scroll from "+before+" to "+Scroll(browser));
                    Selection(browser,index,"After clicking image "+index);
                    int previousCopies=copies;Require(browser.ShortcutForTest(Keys.Control|Keys.C),"Copy shortcut was not handled");Require(copies==previousCopies+1 && copied.Contains(pictures[index])&&Regex.Matches(copied,"<img\\b",RegexOptions.IgnoreCase).Count==1,"Clicking image "+index+" selected/copied another image");
                    int previousOpened=opened;Mouse(browser,index,"mousedown");Mouse(browser,index,"mouseup");Mouse(browser,index,"dblclick");for(int t=0;t<30&&opened==previousOpened;t++)await Task.Delay(10);
                    Require(opened==previousOpened+1 && source==pictures[index] && retained.EndsWith("/"+(index+1)),"Double-click opened another image or duplicate previews");await Task.Delay(50);
                    Require(Math.Abs(Scroll(browser)-before)<=2,"Opening preview reset editor scroll");
                    preview.Close();preview.Dispose();preview=null;host.Activate();await Task.Delay(80);
                    Require(Math.Abs(Scroll(browser)-before)<=2,"Closing preview reset editor scroll");
                    Selection(browser,index,"After closing preview "+index);
                    browser.ShortcutForTest(Keys.Control|Keys.C);Require(copied.Contains(pictures[index])&&Regex.Matches(copied,"<img\\b",RegexOptions.IgnoreCase).Count==1,"Preview focus changed the selected image");
                    Require(editor.Html==original && editor.ChangeVersion==baseline,"Image selection/preview dirtied or rewrote the editor");
                }
                Console.WriteLine("PASS "+(readOnly?"read-only":"editable")+" second/third image exact selection, copy, preview identity, focus, stable scroll and clean draft.");
            }
            editor.SetReadOnly(false);editor.Html=Fixture(pictures);await Task.Delay(40);
            foreach(string interaction in new[]{"wheel","other-image","typed-content","reloaded-content"}) {
                Eval(browser,"document.getElementById('image-1').scrollIntoView(true);window.scrollBy(0,-24)");Mouse(browser,1,"mousedown");Mouse(browser,1,"mouseup");int previousOpened=opened;Mouse(browser,1,"dblclick");for(int t=0;t<30&&opened==previousOpened;t++)await Task.Delay(10);
                Require(preview!=null&&activeImage!=null,"Navigation fixture did not open a preview");
                if(interaction=="wheel")Eval(browser,"var wheel=document.createEvent('Event');wheel.initEvent('mousewheel',true,true);document.getElementById('progress-editor').dispatchEvent(wheel);window.scrollBy(0,160)");
                else if(interaction=="other-image"){Eval(browser,"document.getElementById('image-2').scrollIntoView(true);window.scrollBy(0,-24)");Mouse(browser,2,"mousedown");Mouse(browser,2,"mouseup");}
                else if(interaction=="typed-content"){Eval(browser,"var e=document.createEvent('Event');e.initEvent('keydown',true,true);Object.defineProperty(e,'keyCode',{value:32});document.getElementById('progress-editor').dispatchEvent(e)");editor.SimulateUserHtmlForTest(editor.Html+"<p>用户新输入</p>");SetScroll(browser,180);}
                else {editor.Html=Fixture(pictures)+"<p>重新加载的内容</p>";SetScroll(browser,200);}
                await Task.Delay(30);int userScroll=Scroll(browser);preview.Close();preview.Dispose();preview=null;host.Activate();await Task.Delay(80);
                Require(Math.Abs(Scroll(browser)-userScroll)<=2,"Closing an old preview overwrote newer user navigation: "+interaction);
                if(interaction=="other-image"){browser.ShortcutForTest(Keys.Control|Keys.C);Require(copied.Contains(pictures[2])&&!copied.Contains(pictures[1]),"Closing an old preview selected its image instead of the newly clicked image");}
                Console.WriteLine("PASS preview close preserves newer user interaction: "+interaction+" at scroll "+userScroll+".");
            }
            host.Controls.Remove(editor);host.Controls.Remove(blur);
        }
    }
    [STAThread]static int Main(){Console.OutputEncoding=Encoding.UTF8;int code=0;Application.EnableVisualStyles();using(var host=new Form{Opacity=0,ShowInTaskbar=false,Width=620,Height=360}){host.Shown+=async delegate{try{await Cases(host);}catch(Exception error){code=1;Console.Error.WriteLine(error);}finally{host.Close();}};Application.Run(host);}return code;}
}
"@
$harness=Join-Path $testRoot 'FloatingEditorImageSelectionTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($true))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingEditorImageSelectionTests.exe'
$sources=@(Get-ChildItem -LiteralPath $PSScriptRoot -Filter 'Floating*.cs' | ForEach-Object FullName)
& $compiler /nologo /target:exe /platform:x64 /optimize+ /main:FloatingEditorImageSelectionTests /reference:System.Windows.Forms.dll /reference:System.Drawing.dll /reference:System.Net.Http.dll /reference:System.Web.Extensions.dll ('/out:'+$binary) @sources $harness
if($LASTEXITCODE -ne 0){throw 'Image selection test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(30000)){ $process.Kill();throw ('Image selection tests timed out: '+$testRoot) }
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout -Encoding UTF8
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr -Encoding UTF8;throw ('Image selection tests failed: '+$testRoot)}
if($process.ExitCode){throw ('Image selection tests failed: '+$process.ExitCode+'; '+$testRoot)}
Write-Output ('Results: '+$testRoot)
