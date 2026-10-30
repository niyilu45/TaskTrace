param([string]$OutputDirectory = '.local-build/floating-quick-memo-storage-tests')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
$expectedRoot = [IO.Path]::GetFullPath((Join-Path $repoRoot '.local-build')).TrimEnd('\') + '\'
if (!$testRoot.StartsWith($expectedRoot,[StringComparison]::OrdinalIgnoreCase)) { throw 'Test output must remain inside .local-build.' }
$testRoot = Join-Path $testRoot ([Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $testRoot -Force | Out-Null
# Real file replacement is exercised only inside this isolated test directory.
# No installed application, account, database, network or clipboard is accessed.
$source = @"
using System;
using System.IO;
using System.Linq;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
static class FloatingQuickMemoStorageTests {
    static readonly UTF8Encoding Utf8=new UTF8Encoding(false,true);
    static void Check(bool value,string message){if(!value)throw new Exception(message);}
    static void Throws<T>(Action action,string message) where T:Exception {
        try {action();}catch(T){return;}catch(Exception error){throw new Exception(message+": wrong error type",error);}
        throw new Exception(message+": no error");
    }
    static void NoTemps(string directory){Check(Directory.GetFiles(directory,"*.tmp",SearchOption.AllDirectories).Length==0,"A failed or stale save left a temporary file");}
    static int Main(string[] args) {
        try {
            string root=Path.GetFullPath(args[0]);
            var store=new FloatingQuickMemoStore(Path.Combine(root,"data"));
            Check(store.FilePath==Path.Combine(root,"data","quick-memo","memo.html"),"Memo escaped the active personal-data directory");
            Check(store.LatestVersion==0 && store.Read()=="" && !Directory.Exists(Path.Combine(root,"data")),"Reading a missing memo created runtime files");
            Check(store.Save("",store.ReserveWrite()) && !File.Exists(store.FilePath),"An unchanged empty memo created a file");
            Throws<ArgumentOutOfRangeException>(()=>store.Save("invalid",0),"An unreserved version was accepted");
            string picture="<img src=\"data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jV1kAAAAASUVORK5CYII=\">";
            string original="<p>中文快捷备忘 \uD83D\uDCDD</p><p>第二行<br>缩进与换行</p>"+picture;
            Check(store.Save(original,store.ReserveWrite()),"Initial memo save failed");
            Check(store.Read()==original && !File.Exists(store.FilePath+".bak"),"UTF-8 text or embedded image was changed");
            byte[] firstBytes=File.ReadAllBytes(store.FilePath);
            Check(firstBytes.Take(3).SequenceEqual(new byte[]{0x3c,0x70,0x3e}),"Memo was not written as BOM-free UTF-8 HTML");
            var reopened=new FloatingQuickMemoStore(Path.Combine(root,"data"));
            Check(reopened.Read()==original && reopened.LatestVersion==0,"A new process could not read the persisted memo");
            string large="<p>超过 JSON 上限的图片和文字</p><p>"+new string('x',9*1024*1024)+"</p>"+picture;
            Check(store.Save(large,store.ReserveWrite()) && store.Read()==large && new FileInfo(store.FilePath).Length>8*1024*1024,"A memo larger than 8 MB was truncated or rejected");
            Check(File.ReadAllText(store.FilePath+".bak",Utf8)==original,"Atomic replacement did not retain the previous memo");
            var fixedTime=new DateTime(2003,2,1,4,5,6,DateTimeKind.Utc);
            File.SetLastWriteTimeUtc(store.FilePath,fixedTime);
            byte[] backupBefore=File.ReadAllBytes(store.FilePath+".bak");
            Check(store.Save(large,store.ReserveWrite()),"Unchanged memo was not accepted");
            Check(File.GetLastWriteTimeUtc(store.FilePath)==fixedTime && File.ReadAllBytes(store.FilePath+".bak").SequenceEqual(backupBefore),"Unchanged content rewrote the memo or its backup");
            Console.WriteLine("PASS missing file; strict UTF-8 Chinese/emoji and inline image; independent reopen; >8 MB HTML; previous-version backup; no-op preserves bytes/timestamp.");

            Check(store.Save(original,store.ReserveWrite()),"Preparing locked-file test failed");
            backupBefore=File.ReadAllBytes(store.FilePath+".bak");
            using(var locked=new FileStream(store.FilePath,FileMode.Open,FileAccess.Read,FileShare.Read)) {
                Throws<IOException>(()=>store.Save("<p>must not replace a locked memo</p>",store.ReserveWrite()),"A locked destination was replaced");
            }
            Check(store.Read()==original && File.ReadAllBytes(store.FilePath+".bak").SequenceEqual(backupBefore),"A failed replacement damaged the current or previous memo");
            NoTemps(root);
            Check(store.Save("<p>retry after unlock</p>",store.ReserveWrite()),"Saving did not recover after a temporary lock");
            using(var locked=new FileStream(store.FilePath,FileMode.Open,FileAccess.Read,FileShare.None)) {
                Throws<IOException>(()=>new FloatingQuickMemoStore(Path.Combine(root,"data")).Read(),"Unreadable existing content was treated as blank");
                Throws<IOException>(()=>store.Save("must not erase unreadable content",store.ReserveWrite()),"Saving erased unreadable content");
            }
            Check(store.Read()=="<p>retry after unlock</p>","A read failure changed the memo");
            Console.WriteLine("PASS real replacement failure preserves main/backup and clears temporary files; unlock retry; exclusive read failure never becomes an empty overwrite.");

            long stale=store.ReserveWrite(),latest=store.ReserveWrite();
            Check(store.Save("<p>newest reserved content</p>",latest),"Newest reserved write failed");
            Check(!store.Save("<p>obsolete content</p>",stale) && store.Read()=="<p>newest reserved content</p>","Out-of-order stale content overwrote a newer save");
            var start=new ManualResetEventSlim(false);
            var workers=Enumerable.Range(0,64).Select(index=>Task.Run(delegate {
                start.Wait();long version=store.ReserveWrite();return store.Save("<p>concurrent-"+version+"</p>",version);
            })).ToArray();
            start.Set();Task.WaitAll(workers);start.Dispose();
            Check(store.Read()=="<p>concurrent-"+store.LatestVersion+"</p>","Concurrent saves did not retain the greatest reserved version");
            NoTemps(root);
            Console.WriteLine("PASS out-of-order requests skip stale content; 64 concurrent reservations/saves retain the newest version; no leftover temporary files.");

            string damagedDirectory=Path.Combine(root,"damaged");
            var damaged=new FloatingQuickMemoStore(damagedDirectory);Directory.CreateDirectory(Path.GetDirectoryName(damaged.FilePath));
            byte[] bad={0x3c,0x70,0x3e,0xc3,0x28,0x3c,0x2f,0x70,0x3e};File.WriteAllBytes(damaged.FilePath,bad);
            Throws<DecoderFallbackException>(()=>damaged.Read(),"Malformed UTF-8 was accepted");
            Throws<DecoderFallbackException>(()=>damaged.Save("blank fallback",damaged.ReserveWrite()),"Saving overwrote malformed UTF-8 after a load error");
            Check(File.ReadAllBytes(damaged.FilePath).SequenceEqual(bad),"Malformed source bytes were modified");
            Throws<DecoderFallbackException>(()=>new FloatingQuickMemoStore(damagedDirectory).Read(),"A reopened store silently accepted malformed UTF-8");
            File.WriteAllBytes(damaged.FilePath,new byte[]{0xff,0xfe,0x3c,0x00,0x70,0x00});
            Throws<DecoderFallbackException>(()=>damaged.Read(),"UTF-16 was silently accepted as UTF-8");
            File.WriteAllText(damaged.FilePath,original,new UTF8Encoding(true,true));Check(damaged.Read()==original,"A valid UTF-8 BOM was not handled");
            string invalidSurrogate="<p>"+'\uD800'+"</p>";
            Throws<EncoderFallbackException>(()=>damaged.Save(invalidSurrogate,damaged.ReserveWrite()),"An invalid UTF-16 editor string was written as corrupted UTF-8");
            Check(damaged.Read()==original,"An encoding failure destroyed valid content");
            NoTemps(root);
            Console.WriteLine("PASS malformed UTF-8 and UTF-16 reject load/save without overwriting; valid UTF-8 BOM accepted; invalid surrogate rejected before replacing valid content.");
            return 0;
        }catch(Exception error){Console.Error.WriteLine(error);return 1;}
    }
}
"@
$harness=Join-Path $testRoot 'FloatingQuickMemoStorageTests.cs'
[IO.File]::WriteAllText($harness,$source,[Text.UTF8Encoding]::new($false))
$compiler=Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
$binary=Join-Path $testRoot 'FloatingQuickMemoStorageTests.exe'
& $compiler /nologo /target:exe /platform:x64 /optimize+ ('/out:'+$binary) (Join-Path $PSScriptRoot 'FloatingQuickMemoStorage.cs') $harness
if($LASTEXITCODE -ne 0){throw 'Quick memo storage test compilation failed.'}
$stdout=Join-Path $testRoot 'results.txt';$stderr=Join-Path $testRoot 'errors.txt'
$process=Start-Process -FilePath $binary -ArgumentList ('"'+$testRoot+'"') -WindowStyle Hidden -PassThru -RedirectStandardOutput $stdout -RedirectStandardError $stderr
if(!$process.WaitForExit(60000)){$process.Kill();throw 'Quick memo storage tests timed out.'}
$process.WaitForExit();$process.Refresh()
Get-Content -LiteralPath $stdout
if((Get-Item -LiteralPath $stderr).Length -gt 0){Get-Content -LiteralPath $stderr;throw 'Quick memo storage tests failed.'}
if($process.ExitCode){throw ('Quick memo storage tests failed: '+$process.ExitCode)}
Write-Output ('Results: '+$testRoot)