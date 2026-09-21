// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.IO;
using System.Text.RegularExpressions;

internal sealed partial class FloatingWindow {
    string DraftCachePath(string kind,long taskId,string key) {
        if(kind!="progress" && kind!="outstanding")throw new ArgumentException("Invalid draft kind");
        string safe=Regex.Replace(key??"",@"[^A-Za-z0-9._-]","_");if(safe=="")safe="new";
        return Path.Combine(root,".cache","tasktrace-drafts",kind,taskId.ToString(),safe+".json");
    }
    void WriteDraftCache(string kind,long taskId,string key,object value) {
        string path=DraftCachePath(kind,taskId,key),directory=Path.GetDirectoryName(path),temporary=path+".tmp";
        Directory.CreateDirectory(directory);File.WriteAllText(temporary,json.Serialize(value));
        if(File.Exists(path))File.Delete(path);File.Move(temporary,path);
    }
    T ReadDraftCache<T>(string kind,long taskId,string key) where T:class {
        try {string path=DraftCachePath(kind,taskId,key);return File.Exists(path)?json.Deserialize<T>(File.ReadAllText(path)):null;}catch{return null;}
    }
    bool HasDraftCache(string kind,long taskId,string key) {return File.Exists(DraftCachePath(kind,taskId,key));}
    void DeleteDraftCache(string kind,long taskId,string key) {try{string path=DraftCachePath(kind,taskId,key);if(File.Exists(path))File.Delete(path);}catch{}}
    void DeleteDraftCaches(string kind,long taskId) {
        try {
            string directory=Path.GetDirectoryName(DraftCachePath(kind,taskId,"all"));
            if(Directory.Exists(directory))Directory.Delete(directory,true);
        } catch {}
    }
}
