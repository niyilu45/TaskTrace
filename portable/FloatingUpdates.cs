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
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    const string UpdateArchiveName = "TaskTrace-local-windows-x64.zip";
    const string UpdateChecksumName = "SHA256SUMS.txt";
    const string UpdateReleasesUrl = "https://github.com/niyilu45/TaskTrace/releases";
    readonly System.Windows.Forms.Timer updateTimer = new System.Windows.Forms.Timer { Interval = 2000 };
    bool updateBusy;
    DateTime lastAutomaticUpdateCheck = DateTime.MinValue;
    readonly List<UnsavedUpdateEditor> unsavedUpdateEditors = new List<UnsavedUpdateEditor>();

    sealed class UnsavedUpdateEditor {
        internal string Label;
        internal Func<bool> IsDirty;
        internal Func<Task<bool>> Save;
        internal Action Discard;
    }
    sealed class UnsavedUpdateRegistration : IDisposable {
        FloatingWindow owner;UnsavedUpdateEditor editor;
        internal UnsavedUpdateRegistration(FloatingWindow owner,UnsavedUpdateEditor editor){this.owner=owner;this.editor=editor;}
        public void Dispose(){if(owner!=null){owner.unsavedUpdateEditors.Remove(editor);owner=null;editor=null;}}
    }
    IDisposable RegisterUnsavedUpdateEditor(string label,Func<bool> dirty,Func<Task<bool>> save,Action discard) {
        var editor=new UnsavedUpdateEditor{Label=label,IsDirty=dirty,Save=save,Discard=discard};unsavedUpdateEditors.Add(editor);return new UnsavedUpdateRegistration(this,editor);
    }

    static HttpClient CreateUpdateHttpClient() {
        // This executable is compiled with the in-box .NET Framework compiler. On
        // machines without the strong-crypto registry switch its process default
        // can still be TLS 1.0, which GitHub rejects before an HTTP response exists.
        // Keep this local to the updater traffic so both release checks and asset
        // downloads use the protocol accepted by GitHub.
        ServicePointManager.SecurityProtocol = SecurityProtocolType.Tls12;
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
            var newerReleases = new List<Dictionary<string,object>>();
            string currentVersion = CurrentPackageVersion();
            using(var client = CreateUpdateHttpClient()) {
                string response = await client.GetStringAsync("https://api.github.com/repos/niyilu45/TaskTrace/releases?per_page=30");
                var releases = json.DeserializeObject(response) as IEnumerable;
                if(releases != null) foreach(object item in releases) {
                    var release = item as Dictionary<string,object>;
                    if(release == null || Bool(release, "draft")) continue;
                    string tag = UpdateText(release, "tag_name");
                    if(CompareVersions(tag, currentVersion) <= 0) continue;
                    newerReleases.Add(release);
                    if(FindReleaseAsset(release, UpdateArchiveName) == "") continue;
                    if(latest == null || CompareVersions(tag, UpdateText(latest, "tag_name")) > 0) latest = release;
                }
            }
            var settings = ReadUpdateSettings();
            var state = new Dictionary<string,object>();
            state["current_version"] = currentVersion;
            state["checked_at"] = DateTime.UtcNow.ToString("o", CultureInfo.InvariantCulture);
            state["status"] = "idle"; state["error"] = "";
            state["available"] = latest != null;
            if(latest != null) {
                string version = UpdateText(latest, "tag_name");
                state["latest_version"] = version;
                state["published_at"] = UpdateText(latest, "published_at");
                state["release_notes"] = BuildVersionChanges(newerReleases, currentVersion, version);
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
                {"current_version", CurrentPackageVersion()}, {"checked_at", DateTime.UtcNow.ToString("o", CultureInfo.InvariantCulture)}, {"status", "error"}, {"available", false}, {"notify", false}, {"error", FriendlyUpdateError(e)}, {"release_url", UpdateReleasesUrl}
            });
            lastAutomaticUpdateCheck = DateTime.UtcNow;
            if(interactive) ShowUpdateCheckFailure(FriendlyUpdateError(e));
        } finally { updateBusy = false; }
    }

    static string BuildVersionChanges(List<Dictionary<string,object>> releases,string currentVersion,string latestVersion) {
        var included=releases.Where(release=>CompareVersions(UpdateText(release,"tag_name"),currentVersion)>0&&CompareVersions(UpdateText(release,"tag_name"),latestVersion)<=0).ToList();
        included.Sort(delegate(Dictionary<string,object> left,Dictionary<string,object> right){return CompareVersions(UpdateText(right,"tag_name"),UpdateText(left,"tag_name"));});
        var result=new StringBuilder();
        foreach(var release in included) {
            if(result.Length>0)result.AppendLine().AppendLine();
            string tag=UpdateText(release,"tag_name"),published=DisplayReleaseDate(UpdateText(release,"published_at")),body=UpdateText(release,"body").Trim();
            result.Append(tag).Append(" · ").Append(published).AppendLine();
            result.Append(body==""?"本版本未填写更新内容。":body);
        }
        return result.ToString();
    }

    void ShowUpdateCheckFailure(string error) {
        using(var dialog=DpiDialog(new Form {Text="TaskTrace · 检查更新失败",Size=new System.Drawing.Size(620,300),MinimumSize=new System.Drawing.Size(500,260),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=4};
            layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,26));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,44));
            var details=new TextBox {Dock=DockStyle.Fill,Multiline=true,ReadOnly=true,ScrollBars=ScrollBars.Vertical,BackColor=System.Drawing.SystemColors.Window,Text=error,AccessibleName="检查更新失败原因"};
            var hint=new Label {Dock=DockStyle.Fill,Text="自动检查失败时，可以从下面的 GitHub Releases 页面手动下载免安装包：",TextAlign=System.Drawing.ContentAlignment.MiddleLeft};
            var address=new TextBox {Dock=DockStyle.Fill,ReadOnly=true,BackColor=System.Drawing.SystemColors.Window,Text=UpdateReleasesUrl,AccessibleName="TaskTrace 手动下载网址"};
            var actions=new FlowLayoutPanel {Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};
            var close=new Button {Text="关闭",AutoSize=true,DialogResult=DialogResult.Cancel};var open=new Button {Text="打开下载页面",AutoSize=true};
            open.Click+=delegate{try{System.Diagnostics.Process.Start(new System.Diagnostics.ProcessStartInfo(UpdateReleasesUrl){UseShellExecute=true});}catch(Exception e){MessageBox.Show("无法打开浏览器，请复制下载网址。\r\n\r\n"+e.Message,"TaskTrace · 打开下载页面失败",MessageBoxButtons.OK,MessageBoxIcon.Warning);}};
            actions.Controls.Add(close);actions.Controls.Add(open);layout.Controls.Add(details);layout.Controls.Add(hint);layout.Controls.Add(address);layout.Controls.Add(actions);dialog.Controls.Add(layout);dialog.AcceptButton=open;dialog.CancelButton=close;dialog.ShowDialog(this);
        }
    }

    async Task ShowInteractiveUpdateResult(Dictionary<string,object> state) {
        if(!Bool(state, "available")) {
            MessageBox.Show("当前已是最新版本。\r\n\r\n当前版本：" + CurrentPackageVersion(), "TaskTrace · 检查更新", MessageBoxButtons.OK, MessageBoxIcon.Information);
            return;
        }
        string version = UpdateText(state, "latest_version");
        string published = DisplayReleaseDate(UpdateText(state, "published_at"));
        string notes = UpdateText(state, "release_notes");
        if(!ShowUpdateAvailableDialog(version,published,notes)) {
            IgnoreUpdateVersion(version);
            return;
        }
        updateBusy = false;
        await DownloadAndStartUpdate(version);
    }

    bool ShowUpdateAvailableDialog(string version,string published,string notes) {
        using(var dialog=DpiDialog(new Form {Text="TaskTrace · 发现新版本",Size=new System.Drawing.Size(640,330),MinimumSize=new System.Drawing.Size(520,300),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false})) {
            var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(16),ColumnCount=1,RowCount=5};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,40));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,58));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,0));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
            var heading=new Label{Dock=DockStyle.Fill,Text="发现新版本 "+version,Font=new System.Drawing.Font(Font,System.Drawing.FontStyle.Bold),TextAlign=System.Drawing.ContentAlignment.MiddleLeft};
            var range=new Label{Dock=DockStyle.Fill,Text="本地版本："+CurrentPackageVersion()+"\r\n最新版本："+version+"    发布日期："+published,TextAlign=System.Drawing.ContentAlignment.MiddleLeft};
            var toggle=new Button{Text="查看版本改动",AutoSize=true,Anchor=AnchorStyles.Left};
            var changes=new TextBox{Dock=DockStyle.Fill,Multiline=true,ReadOnly=true,ScrollBars=ScrollBars.Vertical,BackColor=System.Drawing.SystemColors.Window,Text=notes==""?"本次发布未填写更新内容。":notes,AccessibleName="版本改动"};
            var actions=new FlowLayoutPanel{Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};
            var install=new Button{Text="开始更新",AutoSize=true,DialogResult=DialogResult.Yes};var later=new Button{Text="暂不更新",AutoSize=true,DialogResult=DialogResult.No};
            bool expanded=false;toggle.Click+=delegate{expanded=!expanded;layout.RowStyles[3].Height=expanded?150:0;toggle.Text=expanded?"收起版本改动":"查看版本改动";dialog.Height=expanded?480:330;};
            actions.Controls.Add(install);actions.Controls.Add(later);layout.Controls.Add(heading);layout.Controls.Add(range);layout.Controls.Add(toggle);layout.Controls.Add(changes);layout.Controls.Add(actions);dialog.Controls.Add(layout);dialog.AcceptButton=install;dialog.CancelButton=later;
            return dialog.ShowDialog(this)==DialogResult.Yes;
        }
    }

    async Task<List<Action>> PrepareUnsavedContentForUpdate() {
        var dirty=new List<UnsavedUpdateEditor>();
        foreach(var editor in unsavedUpdateEditors.ToArray())try{if(editor.IsDirty())dirty.Add(editor);}catch{dirty.Add(editor);}
        if(dirty.Count==0)return new List<Action>();
        DialogResult choice=ShowUnsavedUpdateDialog(dirty.Select(editor=>editor.Label).Distinct().ToList());
        if(choice==DialogResult.Cancel)return null;
        if(choice==DialogResult.Ignore)return dirty.Where(editor=>editor.Discard!=null).Select(editor=>editor.Discard).ToList();
        for(int index=dirty.Count-1;index>=0;index--) {
            bool saved=false;try{saved=await dirty[index].Save();}catch{}
            if(!saved){MessageBox.Show("“"+dirty[index].Label+"”未能保存。更新已取消，编辑窗口和草稿均已保留，请检查内容后重试。","TaskTrace · 未保存内容",MessageBoxButtons.OK,MessageBoxIcon.Warning);return null;}
        }
        return new List<Action>();
    }

    DialogResult ShowUnsavedUpdateDialog(List<string> labels) {
        using(var dialog=DpiDialog(new Form{Text="TaskTrace · 更新前检查",Size=new System.Drawing.Size(590,310),MinimumSize=new System.Drawing.Size(500,280),Font=Font,Icon=Icon,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=true})) {
            var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(16),ColumnCount=1,RowCount=3};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,66));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,48));
            var heading=new Label{Dock=DockStyle.Fill,Text="检测到尚未保存的内容。请选择如何继续更新：",Font=new System.Drawing.Font(Font,System.Drawing.FontStyle.Bold),TextAlign=System.Drawing.ContentAlignment.MiddleLeft};
            var details=new TextBox{Dock=DockStyle.Fill,Multiline=true,ReadOnly=true,ScrollBars=ScrollBars.Vertical,BackColor=System.Drawing.SystemColors.Window,Text=String.Join("\r\n",labels.Select(label=>"• "+label)),AccessibleName="尚未保存的内容"};
            var actions=new FlowLayoutPanel{Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};
            var save=new Button{Text="保存后更新",AutoSize=true,DialogResult=DialogResult.Yes};var cancel=new Button{Text="取消更新",AutoSize=true,DialogResult=DialogResult.Cancel};var force=new Button{Text="强制更新",AutoSize=true,DialogResult=DialogResult.Ignore};
            actions.Controls.Add(save);actions.Controls.Add(cancel);actions.Controls.Add(force);layout.Controls.Add(heading);layout.Controls.Add(details);layout.Controls.Add(actions);dialog.Controls.Add(layout);dialog.AcceptButton=save;dialog.CancelButton=cancel;
            return dialog.ShowDialog(Form.ActiveForm??this);
        }
    }

    async Task DownloadAndStartUpdate(string version) {
        if(updateBusy) return;
        var discardAfterVerification=await PrepareUnsavedContentForUpdate();
        if(discardAfterVerification==null)return;
        if(MessageBox.Show(Form.ActiveForm??this,"更新需要关闭正在运行的 TaskTrace。\r\n\r\n是否关闭程序并继续更新？", "TaskTrace · 准备更新", MessageBoxButtons.YesNo, MessageBoxIcon.Question) != DialogResult.Yes) return;
        updateBusy = true;
        await Task.Yield();
        string temporaryArchive = "";
        try {
            var state = ReadUpdateState();
            if(!Bool(state, "available") || UpdateText(state, "latest_version") != version || UpdateText(state, "asset_url") == "") throw new Exception("所选版本已经失效，请重新检查更新。");
            state["status"] = "downloading"; state["error"] = ""; WriteUpdateState(state);
            string folder = Path.Combine(data, "updates"); Directory.CreateDirectory(folder);
            string archive = Path.Combine(folder, "TaskTrace-" + SafeVersion(version) + ".zip");
            temporaryArchive = archive + ".tmp";
            TryDelete(temporaryArchive);
            bool completed = false;
            using(var progress = new UpdateDownloadDialog(Font, TopMost, version)) {
                Exception failure = null;
                progress.Shown += async delegate {
                    try {
                        await DownloadUpdateArchive(state, temporaryArchive, progress);
                        progress.SetPhase("正在校验更新包…", "校验通过后才会关闭当前程序。", 100, false);
                        await VerifyUpdateArchive(state, temporaryArchive, progress.Token);
                        progress.Token.ThrowIfCancellationRequested();
                        if(File.Exists(archive)) File.Delete(archive);
                        File.Move(temporaryArchive, archive);
                        completed = true;
                    } catch(OperationCanceledException) {
                    } catch(Exception e) { failure = e; }
                    finally { progress.Finish(); }
                };
                progress.ShowDialog(this);
                if(failure != null) throw failure;
                if(!completed) {
                    TryDelete(temporaryArchive);
                    state["status"] = "idle"; state["error"] = ""; WriteUpdateState(state);
                    MessageBox.Show("更新已取消，当前程序和数据没有变化。", "TaskTrace · 已取消更新", MessageBoxButtons.OK, MessageBoxIcon.Information);
                    return;
                }
            }
            foreach(var discard in discardAfterVerification)try{discard();}catch{}
            File.WriteAllText(Path.Combine(data, "update-ready.json"), json.Serialize(new Dictionary<string,object> {{"version",version},{"archive",archive},{"install_root",root}}), new UTF8Encoding(false));
            state["status"] = "ready"; WriteUpdateState(state);
            Environment.ExitCode = 10; allowExit = true; Close();
        } catch(Exception e) {
            TryDelete(temporaryArchive);
            var state = ReadUpdateState(); state["status"] = "error"; state["error"] = FriendlyUpdateError(e); WriteUpdateState(state);
            MessageBox.Show(FriendlyUpdateError(e), "TaskTrace · 更新失败", MessageBoxButtons.OK, MessageBoxIcon.Warning);
        } finally { updateBusy = false; }
    }

    async Task DownloadUpdateArchive(Dictionary<string,object> state,string destination,UpdateDownloadDialog progress) {
        using(var client = CreateUpdateHttpClient()) {
            client.Timeout = TimeSpan.FromMinutes(30);
            using(var response = await client.GetAsync(UpdateText(state, "asset_url"), HttpCompletionOption.ResponseHeadersRead, progress.Token)) {
                response.EnsureSuccessStatusCode();
                long total = response.Content.Headers.ContentLength.HasValue ? response.Content.Headers.ContentLength.Value : -1;
                using(var input = await response.Content.ReadAsStreamAsync())
                using(var output = new FileStream(destination, FileMode.Create, FileAccess.Write, FileShare.None, 81920, true)) {
                    byte[] buffer = new byte[81920]; long received = 0; int read;
                    var watch = System.Diagnostics.Stopwatch.StartNew(); long lastUi = 0;
                    while((read = await input.ReadAsync(buffer, 0, buffer.Length, progress.Token)) > 0) {
                        await output.WriteAsync(buffer, 0, read, progress.Token); received += read;
                        if(watch.ElapsedMilliseconds - lastUi >= 150 || (total > 0 && received >= total)) {
                            lastUi = watch.ElapsedMilliseconds;
                            int percent = total > 0 ? (int)Math.Min(100, received * 100L / total) : 0;
                            double seconds = Math.Max(.001, watch.Elapsed.TotalSeconds);
                            string detail = FormatUpdateBytes(received) + (total > 0 ? " / " + FormatUpdateBytes(total) : "") + " · " + FormatUpdateBytes((long)(received / seconds)) + "/s";
                            progress.SetPhase("正在下载 " + UpdateArchiveName, detail, percent, total <= 0);
                        }
                    }
                    await output.FlushAsync(progress.Token);
                }
            }
        }
    }

    async Task VerifyUpdateArchive(Dictionary<string,object> state,string archive,CancellationToken token) {
        string checksumUrl = UpdateText(state, "checksum_url");
        if(checksumUrl == "") return;
        string sums;
        using(var client = CreateUpdateHttpClient()) {
            client.Timeout = TimeSpan.FromMinutes(5);
            using(var response = await client.GetAsync(checksumUrl, token)) {
                response.EnsureSuccessStatusCode();
                sums = await response.Content.ReadAsStringAsync();
            }
        }
        token.ThrowIfCancellationRequested();
        string expected = ParseExpectedChecksum(sums, UpdateArchiveName);
        if(expected == "") throw new Exception("发布包校验文件中没有找到 " + UpdateArchiveName + "。更新已取消。");
        string actual = Sha256(archive);
        token.ThrowIfCancellationRequested();
        if(!string.Equals(expected, actual, StringComparison.OrdinalIgnoreCase)) throw new Exception("下载文件校验失败，更新已取消。请稍后重试。");
    }

    static string FormatUpdateBytes(long value) {
        string[] units={"B","KB","MB","GB"};double size=Math.Max(0,value);int index=0;
        while(size>=1024&&index<units.Length-1){size/=1024;index++;}
        return size.ToString(index==0?"0":"0.0",CultureInfo.InvariantCulture)+" "+units[index];
    }

    sealed class UpdateDownloadDialog : Form {
        readonly Label phase = new Label(); readonly Label detail = new Label(); readonly ProgressBar bar = new ProgressBar(); readonly Button cancel = new Button();
        readonly CancellationTokenSource cancellation = new CancellationTokenSource(); bool finished;
        internal CancellationToken Token { get { return cancellation.Token; } }
        internal UpdateDownloadDialog(System.Drawing.Font font,bool topMost,string version) {
            Text="TaskTrace · 正在更新";Size=new System.Drawing.Size(540,220);MinimumSize=new System.Drawing.Size(460,210);StartPosition=FormStartPosition.CenterScreen;ShowInTaskbar=true;TopMost=topMost;Font=font;
            var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(18),ColumnCount=1,RowCount=4};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,34));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,34));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
            phase.Dock=DockStyle.Fill;phase.Text="准备下载 "+version+"…";phase.Font=new System.Drawing.Font(font,System.Drawing.FontStyle.Bold);phase.TextAlign=System.Drawing.ContentAlignment.MiddleLeft;
            detail.Dock=DockStyle.Fill;detail.Text="更新包会先下载到临时文件并完成校验，取消不会修改当前程序。";detail.TextAlign=System.Drawing.ContentAlignment.MiddleLeft;
            bar.Dock=DockStyle.Fill;bar.Style=ProgressBarStyle.Marquee;
            var actions=new FlowLayoutPanel{Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};cancel.Text="取消更新";cancel.AutoSize=true;cancel.Click+=delegate{RequestCancel();};actions.Controls.Add(cancel);
            layout.Controls.Add(phase);layout.Controls.Add(detail);layout.Controls.Add(bar);layout.Controls.Add(actions);Controls.Add(layout);CancelButton=cancel;
        }
        internal void SetPhase(string title,string description,int percent,bool indeterminate) {
            if(IsDisposed)return;phase.Text=title;detail.Text=description;bar.Style=indeterminate?ProgressBarStyle.Marquee:ProgressBarStyle.Continuous;if(!indeterminate)bar.Value=Math.Max(0,Math.Min(100,percent));
        }
        void RequestCancel(){if(finished||cancellation.IsCancellationRequested)return;cancellation.Cancel();cancel.Enabled=false;cancel.Text="正在取消…";phase.Text="正在取消更新…";detail.Text="正在清理临时文件，当前程序不会被修改。";bar.Style=ProgressBarStyle.Marquee;}
        internal void Finish(){finished=true;if(!IsDisposed)Close();}
        protected override void OnFormClosing(FormClosingEventArgs e){if(!finished&&e.CloseReason==CloseReason.UserClosing){e.Cancel=true;RequestCancel();return;}base.OnFormClosing(e);}
        protected override void Dispose(bool disposing){if(disposing)cancellation.Dispose();base.Dispose(disposing);}
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
    static string FriendlyUpdateError(Exception e) {
        if(!(e is HttpRequestException) && !(e is WebException) && !(e is TaskCanceledException))return e.Message;
        Uri github=new Uri("https://api.github.com");string route="直连";
        try { var proxy=WebRequest.DefaultWebProxy;var resolved=proxy==null?github:proxy.GetProxy(github);if(proxy!=null&&!proxy.IsBypassed(github)&&resolved!=github)route="Windows 系统代理 "+resolved.Scheme+"://"+resolved.Host+":"+resolved.Port; } catch {}
        var messages=new List<string>();for(Exception current=e;current!=null;current=current.InnerException)if(!String.IsNullOrWhiteSpace(current.Message)&&!messages.Contains(current.Message))messages.Add(current.Message);
        return "无法连接 GitHub Releases。\r\n已使用："+route+"（TLS 1.2）\r\n\r\n"+String.Join("\r\n",messages);
    }
    static string ParseExpectedChecksum(string content,string fileName) { foreach(string line in content.Split(new[]{'\r','\n'},StringSplitOptions.RemoveEmptyEntries)){string trimmed=line.Trim();if(trimmed.EndsWith(fileName,StringComparison.OrdinalIgnoreCase)){string[] parts=trimmed.Split((char[])null,StringSplitOptions.RemoveEmptyEntries);if(parts.Length>0&&parts[0].Length==64)return parts[0];}}return ""; }
    static string Sha256(string path) { using(var stream=File.OpenRead(path))using(var hash=SHA256.Create())return BitConverter.ToString(hash.ComputeHash(stream)).Replace("-","").ToLowerInvariant(); }

    static int CompareVersions(string left,string right) {
        var a=ParseVersion(left);var b=ParseVersion(right);for(int i=0;i<3;i++){int c=a.Item1[i].CompareTo(b.Item1[i]);if(c!=0)return c;}
        string ap=a.Item2,bp=b.Item2;if(ap==bp)return 0;if(ap=="")return 1;if(bp=="")return -1;
        string[] aa=ap.Split('.'),bb=bp.Split('.');for(int i=0;i<Math.Max(aa.Length,bb.Length);i++){if(i>=aa.Length)return -1;if(i>=bb.Length)return 1;int an,bn;bool ai=int.TryParse(aa[i],out an),bi=int.TryParse(bb[i],out bn);int c;if(ai&&bi)c=an.CompareTo(bn);else if(ai!=bi)c=ai?-1:1;else c=string.Compare(aa[i],bb[i],StringComparison.OrdinalIgnoreCase);if(c!=0)return c;}return 0;
    }
    static Tuple<int[],string> ParseVersion(string value) { string text=(value??"").Trim().TrimStart('v','V');int plus=text.IndexOf('+');if(plus>=0)text=text.Substring(0,plus);string pre="";int dash=text.IndexOf('-');if(dash>=0){pre=text.Substring(dash+1);text=text.Substring(0,dash);}string[] parts=text.Split('.');int[] nums=new int[3];for(int i=0;i<3&&i<parts.Length;i++)int.TryParse(parts[i],out nums[i]);return Tuple.Create(nums,pre); }
}
