// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Net;
using System.Net.Http;
using System.Security.Cryptography;
using System.Text;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    const string UpdateArchiveName = "TaskTrace-local-windows-x64.zip";
    const string UpdateChecksumName = "SHA256SUMS.txt";
    readonly System.Windows.Forms.Timer updateTimer = new System.Windows.Forms.Timer { Interval = 2000 };
    bool updateBusy;
    DateTime lastAutomaticUpdateCheck = DateTime.MinValue;

    static HttpClient CreateUpdateHttpClient() {
        var proxy = WebRequest.DefaultWebProxy;
        if(proxy != null) proxy.Credentials = CredentialCache.DefaultCredentials;
        var handler = new HttpClientHandler { UseProxy = true, Proxy = proxy, UseDefaultCredentials = false };
        var client = new HttpClient(handler) { Timeout = TimeSpan.FromSeconds(45) };
        client.DefaultRequestHeaders.UserAgent.ParseAdd("TaskTrace-Windows-Updater/1.0");
        client.DefaultRequestHeaders.Accept.ParseAdd("application/vnd.github+json");
        return client;
    }

    void InitializeUpdates() {
        updateTimer.Tick += async delegate { await UpdateTimerTick(); };
        Shown += async delegate {
            if(selfTest) return;
            updateTimer.Start();
            await CheckForUpdates(false);
        };
        FormClosing += delegate { updateTimer.Stop(); };
    }

    async Task UpdateTimerTick() {
        if(updateBusy || closing || selfTest) return;
        string commandPath = Path.Combine(data, "update-command.json");
        if(File.Exists(commandPath)) {
            await ProcessUpdateCommand(commandPath);
            return;
        }
        int minutes = ReadUpdateIntervalMinutes();
        if(lastAutomaticUpdateCheck != DateTime.MinValue && DateTime.UtcNow - lastAutomaticUpdateCheck >= TimeSpan.FromMinutes(minutes)) {
            await CheckForUpdates(false);
        }
    }

    async Task ProcessUpdateCommand(string path) {
        Dictionary<string,object> command;
        try { command = ReadObject(File.ReadAllText(path, Encoding.UTF8)); }
        catch { TryDelete(path); return; }
        string action = command.ContainsKey("action") ? Convert.ToString(command["action"]) : "";
        TryDelete(path);
        if(action == "check") await CheckForUpdates(false);
        else if(action == "install") await DownloadAndStartUpdate(Convert.ToString(command.ContainsKey("version") ? command["version"] : ""));
    }

    async Task CheckForUpdates(bool interactive) {
        if(updateBusy) return;
        updateBusy = true;
        try {
            WriteUpdateState(new Dictionary<string,object> {
                {"current_version", CurrentPackageVersion()}, {"status", "checking"}, {"notify", false}, {"available", false}, {"error", ""}
            });
            Dictionary<string,object> latest = null;
            using(var client = CreateUpdateHttpClient()) {
                string response = await client.GetStringAsync("https://api.github.com/repos/niyilu45/TaskTrace/releases?per_page=30");
                var releases = json.DeserializeObject(response) as IEnumerable;
                if(releases != null) foreach(object item in releases) {
                    var release = item as Dictionary<string,object>;
                    if(release == null || Bool(release, "draft")) continue;
                    string tag = UpdateText(release, "tag_name");
                    if(CompareVersions(tag, CurrentPackageVersion()) <= 0) continue;
                    if(FindReleaseAsset(release, UpdateArchiveName) == "") continue;
                    if(latest == null || CompareVersions(tag, UpdateText(latest, "tag_name")) > 0) latest = release;
                }
            }
            var settings = ReadUpdateSettings();
            var state = new Dictionary<string,object>();
            state["current_version"] = CurrentPackageVersion();
            state["checked_at"] = DateTime.UtcNow.ToString("o", CultureInfo.InvariantCulture);
            state["status"] = "idle"; state["error"] = "";
            state["available"] = latest != null;
            if(latest != null) {
                string version = UpdateText(latest, "tag_name");
                state["latest_version"] = version;
                state["published_at"] = UpdateText(latest, "published_at");
                state["release_notes"] = UpdateText(latest, "body");
                state["release_url"] = UpdateText(latest, "html_url");
                state["asset_url"] = FindReleaseAsset(latest, UpdateArchiveName);
                state["checksum_url"] = FindReleaseAsset(latest, UpdateChecksumName);
                state["notify"] = version != UpdateText(settings, "ignored_version");
            } else {
                state["latest_version"] = ""; state["release_notes"] = ""; state["release_url"] = ""; state["asset_url"] = ""; state["checksum_url"] = ""; state["notify"] = false;
            }
            WriteUpdateState(state);
            lastAutomaticUpdateCheck = DateTime.UtcNow;
            if(interactive) await ShowInteractiveUpdateResult(state);
        } catch(Exception e) {
            WriteUpdateState(new Dictionary<string,object> {
                {"current_version", CurrentPackageVersion()}, {"checked_at", DateTime.UtcNow.ToString("o", CultureInfo.InvariantCulture)}, {"status", "error"}, {"available", false}, {"notify", false}, {"error", FriendlyUpdateError(e)}
            });
            lastAutomaticUpdateCheck = DateTime.UtcNow;
            if(interactive) MessageBox.Show(FriendlyUpdateError(e), "TaskTrace · 检查更新失败", MessageBoxButtons.OK, MessageBoxIcon.Warning);
        } finally { updateBusy = false; }
    }

    async Task ShowInteractiveUpdateResult(Dictionary<string,object> state) {
        if(!Bool(state, "available")) {
            MessageBox.Show("当前已是最新版本。\r\n\r\n当前版本：" + CurrentPackageVersion(), "TaskTrace · 检查更新", MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }
        string version = UpdateText(state, "latest_version");
        string published = DisplayReleaseDate(UpdateText(state, "published_at"));
        string notes = UpdateText(state, "release_notes");
        if(notes.Length > 1800) notes = notes.Substring(0, 1800) + "…";
        string message = "发现新版本 " + version + "\r\n发布日期：" + published + "\r\n\r\n更新内容：\r\n" + (notes == "" ? "本次发布未填写更新内容。" : notes) + "\r\n\r\n是否下载并更新？";
        if(MessageBox.Show(message, "TaskTrace · 发现新版本", MessageBoxButtons.YesNo, MessageBoxIcon.Information) != DialogResult.Yes) {
            IgnoreUpdateVersion(version);
            return;
        }
        if(MessageBox.Show("更新需要关闭正在运行的 TaskTrace。\r\n\r\n是否关闭程序并继续更新？", "TaskTrace · 准备更新", MessageBoxButtons.YesNo, MessageBoxIcon.Question) != DialogResult.Yes) return;
        updateBusy = false;
        await DownloadAndStartUpdate(version);
    }

    async Task DownloadAndStartUpdate(string version) {
        if(updateBusy) return;
        updateBusy = true;
        try {
            var state = ReadUpdateState();
            if(!Bool(state, "available") || UpdateText(state, "latest_version") != version || UpdateText(state, "asset_url") == "") throw new Exception("所选版本已经失效，请重新检查更新。");
            state["status"] = "downloading"; state["error"] = ""; WriteUpdateState(state);
            string folder = Path.Combine(data, "updates"); Directory.CreateDirectory(folder);
            string archive = Path.Combine(folder, "TaskTrace-" + SafeVersion(version) + ".zip");
            byte[] payload;
            using(var client = CreateUpdateHttpClient()) payload = await client.GetByteArrayAsync(UpdateText(state, "asset_url"));
            File.WriteAllBytes(archive + ".tmp", payload);
            if(File.Exists(archive)) File.Delete(archive);
            File.Move(archive + ".tmp", archive);
            string checksumUrl = UpdateText(state, "checksum_url");
            if(checksumUrl != "") {
                string sums;
                using(var client = CreateUpdateHttpClient()) sums = await client.GetStringAsync(checksumUrl);
                string expected = ParseExpectedChecksum(sums, UpdateArchiveName);
                if(expected == "") throw new Exception("发布包校验文件中没有找到 " + UpdateArchiveName + "。更新已取消。");
                string actual = Sha256(archive);
                if(!string.Equals(expected, actual, StringComparison.OrdinalIgnoreCase)) throw new Exception("下载文件校验失败，更新已取消。请稍后重试。");
            }
            File.WriteAllText(Path.Combine(data, "update-ready.json"), json.Serialize(new Dictionary<string,object> {{"version",version},{"archive",archive},{"install_root",root}}), new UTF8Encoding(false));
            state["status"] = "ready"; WriteUpdateState(state);
            Environment.ExitCode = 10; allowExit = true; Close();
        } catch(Exception e) {
            var state = ReadUpdateState(); state["status"] = "error"; state["error"] = FriendlyUpdateError(e); WriteUpdateState(state);
            MessageBox.Show(FriendlyUpdateError(e), "TaskTrace · 更新失败", MessageBoxButtons.OK, MessageBoxIcon.Warning);
        } finally { updateBusy = false; }
    }

    Dictionary<string,object> ReadUpdateSettings() {
        string path = Path.Combine(data, "update-settings.json");
        try { return ReadObject(File.ReadAllText(path, Encoding.UTF8)); }
        catch { return new Dictionary<string,object> {{"check_interval_minutes", 60}, {"ignored_version", ""}}; }
    }
    Dictionary<string,object> ReadUpdateState() {
        try { return ReadObject(File.ReadAllText(Path.Combine(data, "update-state.json"), Encoding.UTF8)); }
        catch { return new Dictionary<string,object> {{"current_version", CurrentPackageVersion()}, {"status", "idle"}}; }
    }
    int ReadUpdateIntervalMinutes() { int result; return int.TryParse(UpdateText(ReadUpdateSettings(), "check_interval_minutes"), out result) && result >= 5 && result <= 10080 ? result : 60; }
    void IgnoreUpdateVersion(string version) {
        var settings = ReadUpdateSettings(); settings["ignored_version"] = version; WriteJsonAtomic(Path.Combine(data, "update-settings.json"), settings);
        var state = ReadUpdateState(); state["notify"] = false; WriteUpdateState(state);
    }
    void WriteUpdateState(Dictionary<string,object> state) { WriteJsonAtomic(Path.Combine(data, "update-state.json"), state); }
    void WriteJsonAtomic(string path, object value) { string temporary=path+".tmp"; File.WriteAllText(temporary,json.Serialize(value),new UTF8Encoding(false)); if(File.Exists(path))File.Delete(path);File.Move(temporary,path); }
    string CurrentPackageVersion() { try { string v=File.ReadAllText(Path.Combine(root,"VERSION.txt")).Trim();return v.StartsWith("v")?v:"v"+v; } catch { return "v0.0.0"; } }
    static string FindReleaseAsset(Dictionary<string,object> release,string name) { var assets=release.ContainsKey("assets")?release["assets"] as IEnumerable:null;if(assets!=null)foreach(object item in assets){var asset=item as Dictionary<string,object>;if(asset!=null&&UpdateText(asset,"name")==name)return UpdateText(asset,"browser_download_url");}return ""; }
    static bool Bool(Dictionary<string,object> value,string key) { try{return value.ContainsKey(key)&&Convert.ToBoolean(value[key]);}catch{return false;} }
    static string UpdateText(Dictionary<string,object> value,string key) { return value!=null&&value.ContainsKey(key)&&value[key]!=null?Convert.ToString(value[key],CultureInfo.InvariantCulture):""; }
    static string DisplayReleaseDate(string value) { DateTime date;return DateTime.TryParse(value,CultureInfo.InvariantCulture,DateTimeStyles.AssumeUniversal,out date)?date.ToLocalTime().ToString("yyyy-MM-dd HH:mm"):"未知"; }
    static string SafeVersion(string value) { return new string(value.Where(c=>char.IsLetterOrDigit(c)||c=='.'||c=='-').ToArray()); }
    static void TryDelete(string path) { try{File.Delete(path);}catch{} }
    static string FriendlyUpdateError(Exception e) { if(e is HttpRequestException || e is WebException || e is TaskCanceledException)return "无法连接 GitHub Releases。请检查网络或系统代理设置后重试。\r\n\r\n"+e.Message;return e.Message; }
    static string ParseExpectedChecksum(string content,string fileName) { foreach(string line in content.Split(new[]{'\r','\n'},StringSplitOptions.RemoveEmptyEntries)){string trimmed=line.Trim();if(trimmed.EndsWith(fileName,StringComparison.OrdinalIgnoreCase)){string[] parts=trimmed.Split((char[])null,StringSplitOptions.RemoveEmptyEntries);if(parts.Length>0&&parts[0].Length==64)return parts[0];}}return ""; }
    static string Sha256(string path) { using(var stream=File.OpenRead(path))using(var hash=SHA256.Create())return BitConverter.ToString(hash.ComputeHash(stream)).Replace("-","").ToLowerInvariant(); }

    static int CompareVersions(string left,string right) {
        var a=ParseVersion(left);var b=ParseVersion(right);for(int i=0;i<3;i++){int c=a.Item1[i].CompareTo(b.Item1[i]);if(c!=0)return c;}
        string ap=a.Item2,bp=b.Item2;if(ap==bp)return 0;if(ap=="")return 1;if(bp=="")return -1;
        string[] aa=ap.Split('.'),bb=bp.Split('.');for(int i=0;i<Math.Max(aa.Length,bb.Length);i++){if(i>=aa.Length)return -1;if(i>=bb.Length)return 1;int an,bn;bool ai=int.TryParse(aa[i],out an),bi=int.TryParse(bb[i],out bn);int c;if(ai&&bi)c=an.CompareTo(bn);else if(ai!=bi)c=ai?-1:1;else c=string.Compare(aa[i],bb[i],StringComparison.OrdinalIgnoreCase);if(c!=0)return c;}return 0;
    }
    static Tuple<int[],string> ParseVersion(string value) { string text=(value??"").Trim().TrimStart('v','V');int plus=text.IndexOf('+');if(plus>=0)text=text.Substring(0,plus);string pre="";int dash=text.IndexOf('-');if(dash>=0){pre=text.Substring(dash+1);text=text.Substring(0,dash);}string[] parts=text.Split('.');int[] nums=new int[3];for(int i=0;i<3&&i<parts.Length;i++)int.TryParse(parts[i],out nums[i]);return Tuple.Create(nums,pre); }
}
