// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.IO;
using System.Text;
using System.Threading;

internal sealed class FloatingQuickMemoStore {
    static readonly UTF8Encoding StrictUtf8 = new UTF8Encoding(false,true);
    readonly object writeGate = new object();
    long latestVersion;

    internal string FilePath { get; private set; }
    internal long LatestVersion { get { return Interlocked.Read(ref latestVersion); } }

    internal FloatingQuickMemoStore(string dataDirectory) {
        if(String.IsNullOrWhiteSpace(dataDirectory))throw new ArgumentException("A data directory is required.","dataDirectory");
        FilePath=Path.Combine(Path.GetFullPath(dataDirectory),"quick-memo","memo.html");
    }

    internal string Read() {
        lock(writeGate)return ReadCurrent();
    }

    string ReadCurrent() {
        try {
            // BOM detection is disabled so UTF-16 or malformed UTF-8 cannot become a blank draft.
            using(var input=new FileStream(FilePath,FileMode.Open,FileAccess.Read,FileShare.Read|FileShare.Delete))
            using(var reader=new StreamReader(input,StrictUtf8,false)) {
                string html=reader.ReadToEnd();
                return html.Length>0 && html[0]=='\uFEFF'?html.Substring(1):html;
            }
        } catch(FileNotFoundException) {return "";}
        catch(DirectoryNotFoundException) {return "";}
    }

    internal long ReserveWrite() {return Interlocked.Increment(ref latestVersion);}

    internal bool Save(string html,long version) {
        if(version<=0 || version>LatestVersion)throw new ArgumentOutOfRangeException("version","Reserve a write version before saving.");
        lock(writeGate) {
            if(version<LatestVersion)return false;
            html=html??"";
            // Re-read strictly before replacing: a failed load must never overwrite unreadable data.
            if(String.Equals(ReadCurrent(),html,StringComparison.Ordinal))return true;
            byte[] bytes=StrictUtf8.GetBytes(html);
            string directory=Path.GetDirectoryName(FilePath);
            Directory.CreateDirectory(directory);
            string temporary=FilePath+"."+Guid.NewGuid().ToString("N")+".tmp";
            try {
                using(var output=new FileStream(temporary,FileMode.CreateNew,FileAccess.Write,FileShare.None,81920,FileOptions.WriteThrough)) {
                    output.Write(bytes,0,bytes.Length);
                    output.Flush(true);
                }
                if(version<LatestVersion)return false;
                if(File.Exists(FilePath))File.Replace(temporary,FilePath,FilePath+".bak");
                else File.Move(temporary,FilePath);
                return true;
            } finally {
                try {if(File.Exists(temporary))File.Delete(temporary);}catch(IOException){}catch(UnauthorizedAccessException){}
            }
        }
    }
}