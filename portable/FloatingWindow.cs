// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Net;
using System.Net.Http;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;

internal sealed class FloatingWindow : Form {
    readonly JavaScriptSerializer json = new JavaScriptSerializer { MaxJsonLength = 8388608 };
    readonly HttpClient http = new HttpClient(new HttpClientHandler { UseProxy = false });
    readonly string root, url, data;
    string token, refresh;
    readonly ComboBox projects = new ComboBox { Dock = DockStyle.Fill, DropDownStyle = ComboBoxStyle.DropDownList, DisplayMember = "Title" };
    readonly TextBox entry = new TextBox { Dock = DockStyle.Fill, AccessibleName = "新增事项" };
    readonly TextBox search = new TextBox { Dock = DockStyle.Fill, AccessibleName = "搜索事项" };
    readonly TreeView tasks = new TreeView { Dock = DockStyle.Fill, CheckBoxes = true, HideSelection = false, ShowLines = true, ShowRootLines = true, ShowPlusMinus = true, ShowNodeToolTips = true, Indent = 20, ItemHeight = 28, AccessibleName = "任务与子任务" };
    readonly HashSet<long> collapsedTasks = new HashSet<long>();
    readonly Label status = new Label { Dock = DockStyle.Fill, AutoEllipsis = true, TextAlign = ContentAlignment.MiddleLeft };
    readonly Button previous = new Button { Text = "上一页", AutoSize = true };
    readonly Button next = new Button { Text = "下一页", AutoSize = true };
    readonly CheckBox showCompleted = new CheckBox { Text = "显示已完成", AutoSize = true, Dock = DockStyle.Fill };
    readonly CheckBox pin = new CheckBox { Text = "置顶", Checked = true, AutoSize = true, Padding = new Padding(0, 6, 0, 0) };
    readonly Button fold = new Button { Text = "收起", AutoSize = true };
    readonly TableLayoutPanel content = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1, RowCount = 6, Padding = new Padding(12, 0, 12, 10) };
    readonly Timer timer = new Timer { Interval = 30000 };
    readonly NotifyIcon tray = new NotifyIcon { Text = "TaskTrace · 悬浮事项", Visible = false };
    bool busy, rendering, collapsed, closing, projectsDirty = true;
    bool autoSaveEnabled = true;
    int autoSaveSeconds = 30;
    int page = 1, total, expandedHeight = 560;
    readonly bool selfTest;
    bool allowExit;
    sealed class Project { public long Id; public string Title { get; set; } }
    [DllImport("user32.dll", CharSet = CharSet.Unicode)] static extern IntPtr SendMessage(IntPtr window, uint message, IntPtr wParam, string text);
    static void Hint(TextBox input, string text) { input.HandleCreated += delegate { SendMessage(input.Handle, 0x1501, new IntPtr(1), text); }; }
    static readonly Color Blue = Color.FromArgb(36, 94, 210);

    [STAThread] static int Main(string[] args) {
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        try {
            if (args.Length < 2) throw new Exception("请通过 TaskTrace.exe 启动 TaskTrace。");
            Uri address;
            if (!Uri.TryCreate(args[0], UriKind.Absolute, out address) || address.Scheme != "http" || address.Host != "127.0.0.1") throw new Exception("悬浮窗仅支持本机工作区。");
            using (var window = new FloatingWindow(args[0], args[1], args.Length > 2 ? args[2] : Path.Combine(args[1], "data"), args.Length > 3 && args[3] == "--self-test", args.Length > 3 && args[3] == "--open-browser")) Application.Run(window);
            return Environment.ExitCode;
        } catch (Exception e) { MessageBox.Show(e.Message, "TaskTrace", MessageBoxButtons.OK, MessageBoxIcon.Error); return 1; }
    }
    FloatingWindow(string address, string directory, string dataDirectory, bool test, bool openBrowser) {
        url = address; root = directory; data = Path.GetFullPath(dataDirectory); selfTest = test;
        var session = ReadObject(File.ReadAllText(Path.Combine(data, "local-session.json")));
        token = (string)session["token"]; refresh = (string)session["refresh_token"];
        http.Timeout = TimeSpan.FromSeconds(10);
        Icon = Icon.ExtractAssociatedIcon(Application.ExecutablePath);
        tray.Icon = Icon; tray.Visible = true;
        Text = "TaskTrace · 悬浮事项"; Font = new Font("Microsoft YaHei UI", 9F);
        BackColor = Color.FromArgb(247, 249, 252); ForeColor = Color.FromArgb(31, 41, 55);
        Size = new Size(400, 560); MinimumSize = new Size(350, 300); TopMost = true; StartPosition = FormStartPosition.Manual;
        var area = Screen.PrimaryScreen.WorkingArea; Location = new Point(area.Right - Width - 24, area.Top + 60);
        LoadBounds(); LoadAutoSaveSettings(); LoadTreePreferences(); status.Click += delegate { ShowErrorDetails(); }; KeyPreview = true;
        var toolbar = new FlowLayoutPanel { Dock = DockStyle.Top, Height = 46, Padding = new Padding(9, 6, 0, 0), WrapContents = false };
        var full = new Button { Text = "完整界面", AutoSize = true };
        var reload = new Button { Text = "刷新", AutoSize = true };
        var settingsButton = new Button { Text = "设置", AutoSize = true };
        settingsButton.Click += delegate { ShowAutoSaveSettings(); };
        toolbar.Controls.AddRange(new Control[] { full, reload, pin, fold, settingsButton });
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 34));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 38));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 38));
        content.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 34));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 32));
        var projectRow = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 1 };
        projectRow.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        projectRow.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 118));
        projectRow.Controls.Add(projects, 0, 0); projectRow.Controls.Add(showCompleted, 1, 0);
        content.Controls.Add(projectRow, 0, 0);
        showCompleted.CheckedChanged += async delegate { if(!rendering) { page = 1; SaveBounds(); await Reload(); } };
        Hint(entry, "输入事项，按回车新增"); Hint(search, "搜索当前项目");
        var addRow = Row(entry, "新增", async delegate { await AddTask(); });
        content.Controls.Add(addRow, 0, 1);
        content.Controls.Add(Row(search, "搜索", async delegate { page = 1; await Reload(); }), 0, 2);
        tasks.BorderStyle = BorderStyle.FixedSingle;
        content.Controls.Add(tasks, 0, 3);
        var paging = new FlowLayoutPanel { Dock = DockStyle.Fill, WrapContents = false };
        var progressButton = new Button { Text = "记录进展", AutoSize = true };
        progressButton.Click += delegate { ShowProgress(); };
        var childrenButton = new Button { Text = "子任务", AutoSize = true };
        childrenButton.Click += async delegate { await ShowSubtasks(); };
        paging.Controls.AddRange(new Control[] { previous, next, progressButton, childrenButton }); content.Controls.Add(paging, 0, 4);
        content.Controls.Add(status, 0, 5); Controls.Add(content); Controls.Add(toolbar);
        full.Click += async delegate { await OpenFull(); };
        reload.Click += async delegate { projectsDirty = true; await Reload(); };
        pin.CheckedChanged += delegate { TopMost = pin.Checked; };
        fold.Click += delegate { ToggleFold(); };
        previous.Click += async delegate { if (page > 1) { page--; await Reload(); } };
        next.Click += async delegate { if (page * 50 < total) { page++; await Reload(); } };
        projects.SelectedIndexChanged += async delegate { if (!rendering) { page = 1; await Reload(); } };
        entry.KeyDown += async delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; await AddTask(); } };
        search.KeyDown += async delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; page = 1; await Reload(); } };
        KeyDown += async delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.F5) { e.Handled = true; await Reload(); } };
        tasks.BeforeCheck += delegate(object sender, TreeViewCancelEventArgs e) {
            if(rendering) return;
            e.Cancel = true;
            if(!busy) {
                long id = Convert.ToInt64(e.Node.Tag);
                bool done = !e.Node.Checked;
                BeginInvoke(new Action(async delegate { await Complete(id, done); }));
            }
        };
        tasks.NodeMouseDoubleClick += delegate(object sender, TreeNodeMouseClickEventArgs e) {
            if(e.Node.Nodes.Count == 0) { tasks.SelectedNode = e.Node; ShowProgress(); }
        };
        tasks.AfterCollapse += delegate(object sender, TreeViewEventArgs e) {
            if(!rendering && search.Text.Trim().Length == 0) { collapsedTasks.Add(Convert.ToInt64(e.Node.Tag)); SaveTreePreferences(); }
        };
        tasks.AfterExpand += delegate(object sender, TreeViewEventArgs e) {
            if(!rendering && search.Text.Trim().Length == 0) { collapsedTasks.Remove(Convert.ToInt64(e.Node.Tag)); SaveTreePreferences(); }
        };
        // Minimize keeps the application visible on the Windows taskbar.
        tray.DoubleClick += delegate { RestoreWindow(); };
        var menu = new ContextMenuStrip();
        menu.Items.Add("显示悬浮窗", null, delegate { RestoreWindow(); });
        menu.Items.Add("完整界面", null, async delegate { await OpenFull(); });
        menu.Items.Add("退出 TaskTrace", null, delegate { allowExit = true; Close(); }); tray.ContextMenuStrip = menu;
        timer.Tick += async delegate { if(Visible && !collapsed && !busy) { projectsDirty = true; await Reload(); } };
        Shown += async delegate { await Reload(); timer.Start(); if(selfTest) await TestFlow(); else if(openBrowser) await OpenFull(); };
        FormClosing += delegate(object sender, FormClosingEventArgs e) {
            if(e.CloseReason == CloseReason.UserClosing && !allowExit) {
                e.Cancel = true; SaveBounds(); ShowInTaskbar = true;
                WindowState = FormWindowState.Minimized;
                return;
            }
            closing = true; timer.Stop(); SaveBounds(); tray.Visible = false;
        };
    }
    TableLayoutPanel Row(TextBox input, string title, EventHandler action) {
        var row = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 1 };
        row.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); row.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 68));
        var button = new Button { Text = title, Dock = DockStyle.Fill, BackColor = title == "新增" ? Blue : SystemColors.Control, ForeColor = title == "新增" ? Color.White : ForeColor, FlatStyle = FlatStyle.Flat };
        button.Click += action; row.Controls.Add(input, 0, 0); row.Controls.Add(button, 1, 0); return row;
    }
    Dictionary<string, object> ReadObject(string value) { return json.Deserialize<Dictionary<string, object>>(value); }
    async Task<Dictionary<string, object>> Api(string method, string path, object body, bool retry = true) {
        using(var request = new HttpRequestMessage(new HttpMethod(method), url + "/api/v2" + path)) {
            request.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", token);
            if(body != null) request.Content = new StringContent(json.Serialize(body), Encoding.UTF8, method == "PATCH" ? "application/merge-patch+json" : "application/json");
            using(var response = await http.SendAsync(request)) {
                if(response.StatusCode == HttpStatusCode.Unauthorized && retry) {
                    using(var renewed = await http.PostAsync(url + "/api/v2/oauth/token", new StringContent(json.Serialize(new { grant_type = "refresh_token", refresh_token = refresh }), Encoding.UTF8, "application/json"))) {
                        if(!renewed.IsSuccessStatusCode) throw new Exception("会话已失效，请退出并重新启动悬浮窗。");
                        var pair = ReadObject(await renewed.Content.ReadAsStringAsync()); token = (string)pair["access_token"]; refresh = (string)pair["refresh_token"];
                    }
                    return await Api(method, path, body, false);
                }
                if(!response.IsSuccessStatusCode) throw new Exception("操作未保存（" + (int)response.StatusCode + "），请刷新后重试。");
                return ReadObject(await response.Content.ReadAsStringAsync());
            }
        }
    }
    void SetBusy(bool value) { busy = value; if(!closing) content.Enabled = !value; }
    async Task Reload() {
        if(busy || closing) return; SetBusy(true);
        try { status.ForeColor = ForeColor; status.Text = "正在同步…"; await LoadTasks(); }
        catch(Exception e) { Error(e); }
        finally { SetBusy(false); }
    }
    async Task LoadTasks() {
        if(projectsDirty) {
            var previousProject = projects.SelectedItem as Project;
            rendering = true;
            try {
                projects.Items.Clear();
                int p = 1;
                while(true) {
                    var list = await Api("GET", "/projects?per_page=100&page=" + p, null);
                    foreach(Dictionary<string, object> item in (IEnumerable)list["items"]) projects.Items.Add(new Project { Id = Convert.ToInt64(item["id"]), Title = (string)item["title"] });
                    if(projects.Items.Count >= Convert.ToInt32(list["total"])) break; p++;
                }
                if(projects.Items.Count > 0) {
                    projects.SelectedIndex = 0;
                    if(previousProject != null) foreach(Project candidate in projects.Items) if(candidate.Id == previousProject.Id) { projects.SelectedItem = candidate; break; }
                }
                projectsDirty = false;
            } finally { rendering = false; }
        }
        var project = projects.SelectedItem as Project;
        if(project == null) { status.Text = "请先在完整界面建立项目。"; return; }
        var all = new Dictionary<long, Dictionary<string, object>>();
        var ordered = new List<long>();
        for(int fetchPage = 1; ; fetchPage++) {
            var result = await Api("GET", "/projects/" + project.Id + "/tasks?per_page=100&page=" + fetchPage + "&sort_by=id&order_by=desc", null);
            int count = 0;
            foreach(Dictionary<string, object> item in (IEnumerable)result["items"]) {
                long id = Convert.ToInt64(item["id"]); count++;
                if(!all.ContainsKey(id)) ordered.Add(id);
                all[id] = item;
            }
            if(count == 0 || fetchPage >= Convert.ToInt32(result["total_pages"])) break;
        }
        var parents = new Dictionary<long, long>();
        foreach(long id in ordered) {
            object relationsValue, parentValue;
            if(!all[id].TryGetValue("related_tasks", out relationsValue)) continue;
            var relations = relationsValue as Dictionary<string, object>;
            if(relations == null || !relations.TryGetValue("parenttask", out parentValue) || parentValue == null) continue;
            foreach(Dictionary<string, object> parent in (IEnumerable)parentValue) {
                long parentId = Convert.ToInt64(parent["id"]);
                if(parentId != id && all.ContainsKey(parentId) && (!parents.ContainsKey(id) || parentId < parents[id])) parents[id] = parentId;
            }
        }
        // Break malformed cycles so every task still has a reachable root.
        foreach(long id in ordered) {
            var seen = new HashSet<long>(); long cursor = id;
            while(parents.ContainsKey(cursor)) {
                if(!seen.Add(cursor)) { parents.Remove(cursor); break; }
                cursor = parents[cursor];
            }
        }
        var included = new HashSet<long>(); var matches = new HashSet<long>();
        string query = search.Text.Trim();
        foreach(long id in ordered) {
            if(!showCompleted.Checked && Convert.ToBoolean(all[id]["done"])) continue;
            if(query.Length > 0 && ((string)all[id]["title"]).IndexOf(query, StringComparison.OrdinalIgnoreCase) < 0) continue;
            matches.Add(id); long cursor = id;
            while(included.Add(cursor) && parents.ContainsKey(cursor)) cursor = parents[cursor];
        }
        long selectedId = tasks.SelectedNode == null ? 0 : Convert.ToInt64(tasks.SelectedNode.Tag);
        rendering = true; tasks.BeginUpdate();
        try {
            tasks.Nodes.Clear(); var nodes = new Dictionary<long, TreeNode>(); var roots = new List<TreeNode>();
            foreach(long id in ordered) {
                if(!included.Contains(id)) continue;
                bool done = Convert.ToBoolean(all[id]["done"]);
                nodes[id] = new TreeNode((string)all[id]["title"]) {
                    Name = id.ToString(), Tag = id, Checked = done,
                    ForeColor = done ? Color.FromArgb(100, 110, 125) : ForeColor,
                    ToolTipText = (done ? "已完成 · " : "未完成 · ") + (string)all[id]["title"] + (matches.Contains(id) ? "" : "（为显示匹配子任务保留的父任务）")
                };
            }
            foreach(long id in ordered) {
                if(!nodes.ContainsKey(id)) continue;
                if(parents.ContainsKey(id) && nodes.ContainsKey(parents[id])) nodes[parents[id]].Nodes.Add(nodes[id]);
                else roots.Add(nodes[id]);
            }
            total = roots.Count; page = Math.Max(1, Math.Min(page, Math.Max(1, (total + 49) / 50)));
            for(int index = (page - 1) * 50; index < Math.Min(page * 50, roots.Count); index++) tasks.Nodes.Add(roots[index]);
            tasks.ExpandAll();
            if(query.Length == 0) foreach(var pair in nodes) if(collapsedTasks.Contains(pair.Key)) pair.Value.Collapse();
            if(nodes.ContainsKey(selectedId) && nodes[selectedId].TreeView == tasks) tasks.SelectedNode = nodes[selectedId];
            previous.Enabled = page > 1; next.Enabled = page * 50 < total;
            status.ForeColor = ForeColor;
            status.Text = matches.Count == 0 ? "没有匹配事项，可清空搜索或显示已完成。" : matches.Count + " 项 · " + total + " 个任务组 · 第 " + page + " 页";
        } finally { tasks.EndUpdate(); rendering = false; }
    }
    void LoadTreePreferences() {
        try { var values = json.Deserialize<long[]>(File.ReadAllText(Path.Combine(data, "floating-tree.json"))); foreach(long id in values) collapsedTasks.Add(id); } catch { }
    }
    void SaveTreePreferences() {
        try { File.WriteAllText(Path.Combine(data, "floating-tree.json"), json.Serialize(new List<long>(collapsedTasks))); } catch { }
    }

    async Task AddTask() {
        if(busy || closing || string.IsNullOrWhiteSpace(entry.Text)) return;
        var project = projects.SelectedItem as Project; if(project == null) return;
        SetBusy(true);
        try { await Api("POST", "/projects/" + project.Id + "/tasks", new { title = entry.Text.Trim() }); entry.Clear(); search.Clear(); page = 1; await LoadTasks(); entry.Focus(); }
        catch(Exception e) { Error(e); } finally { SetBusy(false); }
    }
    sealed class PastedImage { public byte[] Bytes; public long Id; }
    async Task<long> SaveProgress(long id, DateTime day, string progress, string nextStep, List<PastedImage> pictures = null, long commentId = 0) {
        string note = "<h3>每日进展 · " + day.ToString("yyyy-MM-dd") + "</h3><p>" + WebUtility.HtmlEncode(progress.Trim()).Replace("\r\n", "<br>").Replace("\n", "<br>") + "</p>";
        if(!string.IsNullOrWhiteSpace(nextStep)) note += "<p><strong>遗留问题 / 下一步</strong></p><p>" + WebUtility.HtmlEncode(nextStep.Trim()).Replace("\r\n", "<br>").Replace("\n", "<br>") + "</p>";
        if(pictures != null) foreach(var picture in pictures) {
            if(picture.Id == 0) {
                // Refresh before streaming a file so the multipart body never needs replaying.
                await Api("GET", "/tasks/" + id, null);
                using(var body = new MultipartFormDataContent()) {
                    var image = new ByteArrayContent(picture.Bytes); image.Headers.ContentType = new System.Net.Http.Headers.MediaTypeHeaderValue("image/png");
                    body.Add(image, "files", "clipboard-" + DateTime.Now.Ticks + ".png");
                    using(var request = new HttpRequestMessage(HttpMethod.Post, url + "/api/v2/tasks/" + id + "/attachments")) {
                        request.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", token); request.Content = body;
                        using(var response = await http.SendAsync(request)) {
                            if(!response.IsSuccessStatusCode) throw new Exception("图片上传失败，请重试。");
                            var result = ReadObject(await response.Content.ReadAsStringAsync());
                            foreach(Dictionary<string, object> attached in (IEnumerable)result["success"]) { picture.Id = Convert.ToInt64(attached["id"]); break; }
                            if(picture.Id == 0) throw new Exception("图片上传失败，请重试。");
                        }
                    }
                }
            }
            note += "<p><img src=\"/api/v1/tasks/" + id + "/attachments/" + picture.Id + "\" alt=\"进展图片\"></p>";
        }
        var resultNote = await Api(commentId == 0 ? "POST" : "PUT", "/tasks/" + id + "/comments" + (commentId == 0 ? "" : "/" + commentId), new { comment = note });
        return Convert.ToInt64(resultNote["id"]);
    }
    void ShowProgress() {
        if(busy || closing) return;
        if(tasks.SelectedNode == null) { status.Text = "请先选中要记录进展的事项。"; return; }
        long id = Convert.ToInt64(tasks.SelectedNode.Tag);
        using(var dialog = new Form { Text = "每日进展 · " + tasks.SelectedNode.Text, Size = new Size(430, 450), MinimumSize = new Size(380, 420), StartPosition = FormStartPosition.CenterParent, Font = Font, TopMost = TopMost, ShowInTaskbar = false }) {
            var layout = new TableLayoutPanel { Dock = DockStyle.Fill, Padding = new Padding(14), ColumnCount = 1, RowCount = 7 };
            var day = new DateTimePicker { Format = DateTimePickerFormat.Custom, CustomFormat = "yyyy-MM-dd", Dock = DockStyle.Fill, Value = DateTime.Today };
            var progress = new TextBox { Multiline = true, AcceptsReturn = true, ScrollBars = ScrollBars.Vertical, Dock = DockStyle.Fill, AccessibleName = "今日进展" };
            var nextStep = new TextBox { Multiline = true, AcceptsReturn = true, ScrollBars = ScrollBars.Vertical, Dock = DockStyle.Fill, AccessibleName = "遗留问题 / 下一步" };
            var save = new Button { Text = "保存进展 (Ctrl+Enter)", Dock = DockStyle.Fill };
            var feedback = new Label { Text = "保存后追加到事项历史，不覆盖已有内容。", Dock = DockStyle.Fill };
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 32)); layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 26));
            layout.RowStyles.Add(new RowStyle(SizeType.Percent, 60)); layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 26));
            layout.RowStyles.Add(new RowStyle(SizeType.Percent, 40)); layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 36)); layout.RowStyles.Add(new RowStyle(SizeType.Absolute, 36));
            layout.Controls.Add(day); layout.Controls.Add(new Label { Text = "今日进展", Dock = DockStyle.Fill }); layout.Controls.Add(progress);
            layout.Controls.Add(new Label { Text = "遗留问题 / 下一步（选填）", Dock = DockStyle.Fill }); layout.Controls.Add(nextStep); layout.Controls.Add(save); layout.Controls.Add(feedback);
            dialog.Controls.Add(layout);
            var pictures = new List<PastedImage>();
            bool submitting = false, saved = false;
            long commentId = 0;
            string lastSaved = "";
            Func<string> snapshot = delegate { return json.Serialize(new { date = day.Value.Date, text = progress.Text, next = nextStep.Text, images = pictures.Count }); };
            Func<bool, Task> write = async delegate(bool finish) {
                if(submitting || (string.IsNullOrWhiteSpace(progress.Text) && pictures.Count == 0 && commentId == 0)) return;
                string current = snapshot();
                if(current == lastSaved) { if(finish) { saved = true; dialog.Close(); } return; }
                submitting = true; save.Enabled = false; progress.ReadOnly = true; nextStep.ReadOnly = true; day.Enabled = false;
                try {
                    commentId = await SaveProgress(id, day.Value, progress.Text, nextStep.Text, pictures, commentId);
                    lastSaved = current;
                    if(finish) { saved = true; dialog.Close(); status.Text = "今日进展已保存。"; }
                    else feedback.Text = "已自动保存 " + DateTime.Now.ToString("HH:mm:ss") + "，继续编辑更新本条记录。";
                }
                catch(Exception) { feedback.Text = "保存失败，内容已保留，将在下次检查时重试。"; }
                finally { submitting = false; if(!dialog.IsDisposed) { save.Enabled = true; progress.ReadOnly = false; nextStep.ReadOnly = false; day.Enabled = true; } }
            };
            save.Click += async delegate { await write(true); };
            var autoTimer = new Timer { Interval = autoSaveSeconds * 1000 };
            autoTimer.Tick += async delegate { if(autoSaveEnabled) await write(false); };
            autoTimer.Start();            dialog.KeyPreview = true;
            feedback.Text = "支持 Ctrl+V 粘贴截图，可连续粘贴多张。";
            dialog.KeyDown += delegate(object sender, KeyEventArgs e) {
                if(e.Control && e.KeyCode == Keys.V && !submitting) {
                    try {
                        if(Clipboard.ContainsImage()) {
                            e.SuppressKeyPress = true;
                            using(var image = Clipboard.GetImage()) using(var stream = new MemoryStream()) {
                                image.Save(stream, System.Drawing.Imaging.ImageFormat.Png); pictures.Add(new PastedImage { Bytes = stream.ToArray() });
                            }
                            feedback.Text = "已粘贴 " + pictures.Count + " 张图片，点击保存后上传。";
                        }
                    } catch { feedback.Text = "剪贴板暂不可用，请重新复制图片后再试。"; }
                }
                if(e.Control && e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; save.PerformClick(); } };
            dialog.FormClosing += delegate(object sender, FormClosingEventArgs e) {
                if(saved || snapshot() == lastSaved) return;
                if(submitting) { e.Cancel = true; return; }
                if((progress.Text.Length > 0 || nextStep.Text.Length > 0 || pictures.Count > 0) && MessageBox.Show(dialog, "进展尚未保存，确定放弃？", "每日进展", MessageBoxButtons.YesNo) != DialogResult.Yes) e.Cancel = true;
            };
            SetBusy(true); timer.Stop();
            try { dialog.ShowDialog(this); } finally { autoTimer.Stop(); autoTimer.Dispose(); SetBusy(false); timer.Start(); }
        }
    }
    async Task<long> CreateSubtask(long parentId, long projectId, string title, long existingId = 0) {
        long childId = existingId;
        if(childId == 0) { var child = await Api("POST", "/projects/" + projectId + "/tasks", new { title = title }); childId = Convert.ToInt64(child["id"]); }
        await Api("POST", "/tasks/" + parentId + "/relations", new { other_task_id = childId, relation_kind = "subtask" });
        return childId;
    }
    async Task ShowSubtasks() {
        if(busy || closing) return;
        if(tasks.SelectedNode == null) { status.Text = "请先选中一个父事项。"; return; }
        long parentId = Convert.ToInt64(tasks.SelectedNode.Tag);
        SetBusy(true); timer.Stop();
        try {
            var parent = await Api("GET", "/tasks/" + parentId, null);
            long projectId = Convert.ToInt64(parent["project_id"]);
            using(var dialog = new Form { Text = "子任务 · " + (string)parent["title"], Size = new Size(450, 430), MinimumSize = new Size(380, 320), Font = Font, TopMost = TopMost, StartPosition = FormStartPosition.CenterParent }) {
                var list = new ListView { Dock = DockStyle.Fill, View = View.Details, CheckBoxes = true, FullRowSelect = true, HeaderStyle = ColumnHeaderStyle.None };
                list.Columns.Add("子任务", 390);
                var title = new TextBox { Dock = DockStyle.Fill };
                var feedback = new Label { Dock = DockStyle.Bottom, Height = 46, Text = "子任务独立完成；在主列表中选择它可记录每日进展。" };
                var row = new TableLayoutPanel { Dock = DockStyle.Top, Height = 38, ColumnCount = 2 };
                row.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); row.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 95));
                var add = new Button { Text = "添加子任务", Dock = DockStyle.Fill }; row.Controls.Add(title); row.Controls.Add(add);
                dialog.Controls.Add(list); dialog.Controls.Add(row); dialog.Controls.Add(feedback);
                bool loading = false, writing = false; long pendingId = 0;
                Func<Task> reload = async delegate {
                    loading = true;
                    try {
                        var current = await Api("GET", "/tasks/" + parentId, null);
                        list.Items.Clear();
                        var relations = current["related_tasks"] as Dictionary<string, object>;
                        if(relations != null && relations.ContainsKey("subtask")) foreach(Dictionary<string, object> child in (IEnumerable)relations["subtask"]) list.Items.Add(new ListViewItem((string)child["title"]) { Tag = Convert.ToInt64(child["id"]), Checked = Convert.ToBoolean(child["done"]) });
                    } finally { loading = false; }
                };
                add.Click += async delegate {
                    if(writing || string.IsNullOrWhiteSpace(title.Text)) return;
                    writing = true; add.Enabled = false; title.Enabled = false;
                    try {
                        if(pendingId == 0) { var child = await Api("POST", "/projects/" + projectId + "/tasks", new { title = title.Text.Trim() }); pendingId = Convert.ToInt64(child["id"]); }
                        await CreateSubtask(parentId, projectId, title.Text, pendingId);
                        pendingId = 0; title.Clear(); feedback.Text = "子任务已添加。可独立勾选完成。"; add.Text = "添加子任务"; await reload();
                    } catch { feedback.Text = pendingId == 0 ? "创建失败，请重试。" : "事项已创建，关联失败；点击重试，不会重复创建。"; add.Text = pendingId == 0 ? "添加子任务" : "重试关联"; }
                    finally { writing = false; add.Enabled = true; title.Enabled = pendingId == 0; }
                };
                title.KeyDown += delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; add.PerformClick(); } };
                list.ItemCheck += delegate(object sender, ItemCheckEventArgs e) {
                    if(loading) return;
                    bool done = e.NewValue == CheckState.Checked; long id = Convert.ToInt64(list.Items[e.Index].Tag); e.NewValue = e.CurrentValue;
                    if(writing) return;
                    writing = true;
                    dialog.BeginInvoke(new Action(async delegate {
                        try { await Api("PATCH", "/tasks/" + id, new { done = done }); await reload(); }
                        catch { feedback.Text = "状态未保存，请重试。"; }
                        finally { writing = false; }
                    }));
                };
                dialog.FormClosing += delegate(object sender, FormClosingEventArgs e) { if(writing) e.Cancel = true; };
                await reload(); dialog.ShowDialog(this);
            }
            await LoadTasks();
        } catch(Exception e) { Error(e); }
        finally { SetBusy(false); timer.Start(); }
    }
    async Task Complete(long id, bool done = true) {
        if(busy || closing) return; SetBusy(true);
        try { await Api("PATCH", "/tasks/" + id, new { done = done }); await LoadTasks(); }
        catch(Exception e) { Error(e); } finally { SetBusy(false); }
    }
    async Task OpenFull() {
        if(busy || closing) return; SetBusy(true);
        string step = "准备浏览器工作区";
        try {
            string browserSession = await PrepareBrowserSession();
            step = "打开系统默认浏览器，请检查 Windows 默认浏览器设置";
            Process.Start(new ProcessStartInfo(url + "/#tasktrace-local=" + Uri.EscapeDataString(browserSession)) { UseShellExecute = true });
        } catch(Exception e) { Error(new Exception(step + "失败。悬浮窗仍可继续使用。", e)); ShowErrorDetails(); } finally { SetBusy(false); }
    }
    async Task<string> PrepareBrowserSession() {
            // Each browser launch gets its own refresh family, independent of this window.
            string file = Path.Combine(data, "browser-session.json");
            var info = new ProcessStartInfo(Path.Combine(root, "TaskTrace-server.exe"), "--config \"" + Path.Combine(data, "local-config.yml") + "\" tasktrace-local-session --output \"" + file + "\" --user-id " + File.ReadAllText(Path.Combine(data, "local-user-id.txt")).Trim()) { UseShellExecute = false, CreateNoWindow = true, RedirectStandardOutput = true, RedirectStandardError = true };
            using(var process = Process.Start(info)) {
                var output = process.StandardOutput.ReadToEndAsync(); var error = process.StandardError.ReadToEndAsync();
                await Task.Run(delegate { process.WaitForExit(); }); await Task.WhenAll(output, error);
                if(process.ExitCode != 0) throw new Exception("无法打开工作区，请重启后重试。");
            }
            return File.ReadAllText(file);
    }
    string lastError = "";
    void ShowErrorDetails() {
        if(String.IsNullOrEmpty(lastError)) return;
        using(var dialog = new Form { Text = "TaskTrace · 错误详情（可复制）", Width = 740, Height = 480, StartPosition = FormStartPosition.CenterParent }) {
            dialog.Controls.Add(new TextBox { Multiline = true, ReadOnly = true, ScrollBars = ScrollBars.Both, WordWrap = false, Dock = DockStyle.Fill, Text = lastError });
            dialog.ShowDialog(this);
        }
    }
    void Error(Exception e) {
        if(closing) return;
        lastError = System.Text.RegularExpressions.Regex.Replace(e.ToString(), "#tasktrace-local=\\S+", "#tasktrace-local=[redacted]");
        for(Exception inner = e; inner != null; inner = inner.InnerException) {
            var windowsError = inner as System.ComponentModel.Win32Exception;
            if(windowsError != null) lastError += "\r\nWindows error code: " + windowsError.NativeErrorCode;
        }
        lastError = "Windows: " + Environment.OSVersion + "\r\n" + lastError;
        try {
            string file = Path.Combine(data, "TaskTrace-window-error.log");
            File.WriteAllText(file, DateTime.Now.ToString("o") + "\r\n" + lastError);
            lastError += "\r\n\r\n错误日志：" + file;
        } catch { }
        status.ForeColor = Color.FromArgb(170, 35, 35);
        status.Text = "操作失败，点击此处查看完整错误详情。";
    }
    void RestoreWindow() { Show(); WindowState = FormWindowState.Normal; Activate(); }
    void ToggleFold() { if(!collapsed) { expandedHeight = Height; content.Visible = false; MinimumSize = new Size(350, 85); Height = 85; collapsed = true; fold.Text = "展开"; } else { collapsed = false; content.Visible = true; MinimumSize = new Size(350, 300); Height = expandedHeight; fold.Text = "收起"; } }
    void LoadAutoSaveSettings() {
        try {
            var settings = ReadObject(File.ReadAllText(Path.Combine(data, "autosave.json")));
            autoSaveEnabled = Convert.ToBoolean(settings["enabled"]);
            autoSaveSeconds = Math.Max(5, Math.Min(3600, Convert.ToInt32(settings["seconds"])));
        } catch { }
    }
    void ShowAutoSaveSettings() {
        using(var settings = new Form { Text = "设置", Size = new Size(430, 345), FormBorderStyle = FormBorderStyle.FixedDialog, MaximizeBox = false, MinimizeBox = false, StartPosition = FormStartPosition.CenterParent, Font = Font, TopMost = TopMost }) {
            var enabled = new CheckBox { Text = "启用每日进展自动保存", Checked = autoSaveEnabled, Location = new Point(18, 18), AutoSize = true };
            var label = new Label { Text = "检查间隔（秒）", Location = new Point(18, 55), AutoSize = true };
            var seconds = new NumericUpDown { Minimum = 5, Maximum = 3600, Value = autoSaveSeconds, Location = new Point(155, 52), Width = 100 };
            var help = new Label { Text = "仅内容变化时保存；编辑期间更新同一条进展。\n设置保存在本机目录，网页设置需在网页中配置。", Location = new Point(18, 92), Size = new Size(335, 48) };
            var apply = new Button { Text = "保存设置", Location = new Point(235, 150), Width = 100 };
            apply.Click += delegate {
                try {
                    File.WriteAllText(Path.Combine(data, "autosave.json"), json.Serialize(new { enabled = enabled.Checked, seconds = (int)seconds.Value }));
                    autoSaveEnabled = enabled.Checked; autoSaveSeconds = (int)seconds.Value; settings.Close();
                } catch { MessageBox.Show(settings, "设置保存失败，请检查目录写入权限。"); }
            };
                        var dataLabel = new Label { Text = "当前数据目录：" + data, Location = new Point(18, 192), Size = new Size(380, 48), AutoEllipsis = true };
            var chooseData = new Button { Text = "配置数据目录（重启生效）", Location = new Point(18, 248), Size = new Size(250, 32) };
            chooseData.Click += delegate {
                try { Process.Start(new ProcessStartInfo("powershell.exe", "-NoProfile -STA -ExecutionPolicy Bypass -File \"" + Path.Combine(root, "Configure-TaskTrace.ps1") + "\"") { UseShellExecute = false, CreateNoWindow = true }); }
                catch(Exception e) { MessageBox.Show(settings, e.Message); }
            };
            settings.Controls.AddRange(new Control[] { enabled, label, seconds, help, apply, dataLabel, chooseData }); settings.ShowDialog(this);
        }
    }
    void LoadBounds() {
        try {
            var saved = ReadObject(File.ReadAllText(Path.Combine(data, "floating-window.json")));
            if(saved.ContainsKey("showCompleted")) showCompleted.Checked = Convert.ToBoolean(saved["showCompleted"]);
            var bounds = new Rectangle(Convert.ToInt32(saved["x"]), Convert.ToInt32(saved["y"]), Math.Max(350, Convert.ToInt32(saved["width"])), Math.Max(300, Convert.ToInt32(saved["height"])));
            var area = Screen.FromRectangle(bounds).WorkingArea;
            Size = new Size(Math.Min(bounds.Width, area.Width), Math.Min(bounds.Height, area.Height));
            Location = new Point(Math.Max(area.Left, Math.Min(bounds.X, area.Right - Width)), Math.Max(area.Top, Math.Min(bounds.Y, area.Bottom - Height)));
        } catch { }
    }
    void SaveBounds() {
        try { var b = WindowState == FormWindowState.Normal ? Bounds : RestoreBounds; File.WriteAllText(Path.Combine(data, "floating-window.json"), json.Serialize(new { x = b.X, y = b.Y, width = b.Width, height = collapsed ? expandedHeight : b.Height, showCompleted = showCompleted.Checked })); } catch { }
    }
    async Task TestFlow() {
        try {
            Error(new Exception("Diagnostic test", new System.ComponentModel.Win32Exception(1155, "No default browser #tasktrace-local=TEST_PRIVATE_SESSION")));
            string diagnostic = File.ReadAllText(Path.Combine(data, "TaskTrace-window-error.log"));
            if(!diagnostic.Contains("Windows error code: 1155") || diagnostic.Contains("TEST_PRIVATE_SESSION") || !lastError.Contains("错误日志")) throw new Exception("Error diagnostics incomplete or leaked session");
            await Reload();
            if(projects.Items.Count == 0 || !TopMost) throw new Exception("Workspace or TopMost missing");
            entry.Text = "悬浮窗验收 " + DateTime.Now.Ticks; string createdTitle = entry.Text; await AddTask();
            if(tasks.Nodes.Count == 0 || tasks.Nodes[0].Text != createdTitle) throw new Exception("Task creation failed");
            long id = Convert.ToInt64(tasks.Nodes[0].Tag);
            await Api("PATCH", "/tasks/" + id, new { description = "保留已有进展" });
            await SaveProgress(id, DateTime.Today, "已完成接口联调 <检查>", "明天补充图片");
            var testPictures = new List<PastedImage>();
            using(var bitmap = new Bitmap(24, 24)) using(var stream = new MemoryStream()) { bitmap.Save(stream, System.Drawing.Imaging.ImageFormat.Png); testPictures.Add(new PastedImage { Bytes = stream.ToArray() }); testPictures.Add(new PastedImage { Bytes = stream.ToArray() }); }
            long dailyId = await SaveProgress(id, DateTime.Today, "追加今日进展", "", testPictures);
            long updatedDailyId = await SaveProgress(id, DateTime.Today, "追加今日进展，自动保存更新", "", testPictures, dailyId);
            if(updatedDailyId != dailyId) throw new Exception("Auto-save duplicated the progress record");
            if(testPictures[0].Id <= 0 || testPictures[1].Id <= 0 || testPictures[0].Id == testPictures[1].Id) throw new Exception("Pasted image upload failed");
            var history = await Api("GET", "/tasks/" + id + "/comments?order_by=desc", null);
            if(Convert.ToInt32(history["total"]) != 2 || !json.Serialize(history).Contains("每日进展")) throw new Exception("Daily progress history failed");
            long childId = await CreateSubtask(id, Convert.ToInt64((await Api("GET", "/tasks/" + id, null))["project_id"]), "子任务验收");
            var parentWithChild = await Api("GET", "/tasks/" + id, null);
            if(!json.Serialize(parentWithChild["related_tasks"]).Contains("子任务验收")) throw new Exception("Subtask relationship missing");
            await Api("PATCH", "/tasks/" + childId, new { done = true });
            if(Convert.ToBoolean((await Api("GET", "/tasks/" + id, null))["done"])) throw new Exception("Child completion incorrectly completed parent");
            await Api("PATCH", "/tasks/" + childId, new { done = false });
            long grandchildId = await CreateSubtask(childId, Convert.ToInt64(parentWithChild["project_id"]), "下级子任务验收");
            await LoadTasks();
            if(tasks.Nodes.Count != 1 || tasks.Nodes[0].Nodes.Count != 1 || tasks.Nodes[0].Nodes[0].Nodes.Count != 1) throw new Exception("Task hierarchy missing");
            if(tasks.Nodes[0].Nodes[0].Level != 1 || tasks.Nodes[0].Nodes[0].Nodes[0].Level != 2) throw new Exception("Subtask indentation missing");
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-tree-test.png")); }
            tasks.Nodes[0].Collapse(); collapsedTasks.Clear(); LoadTreePreferences(); await LoadTasks();
            if(tasks.Nodes[0].IsExpanded || !collapsedTasks.Contains(id)) throw new Exception("Collapsed state not retained");
            search.Text = "下级子任务验收"; await LoadTasks();
            if(tasks.Nodes.Count != 1 || !tasks.Nodes[0].IsExpanded || tasks.Nodes[0].Nodes[0].Nodes.Count != 1) throw new Exception("Search lost hierarchy");
            search.Clear(); await LoadTasks(); tasks.Nodes[0].Expand();
            if(collapsedTasks.Contains(id)) throw new Exception("Expand state not retained");
            await Api("PATCH", "/tasks/" + id, new { done = true }); await LoadTasks();
            if(tasks.Nodes.Count != 1 || !tasks.Nodes[0].Checked || tasks.Nodes[0].Nodes.Count != 1) throw new Exception("Completed ancestor lost pending children");
            await Api("PATCH", "/tasks/" + id, new { done = false });
            await Api("DELETE", "/tasks/" + grandchildId, null);
            await Api("DELETE", "/tasks/" + childId, null);
            await Complete(id);
            var saved = await Api("GET", "/tasks/" + id, null);
            if(!Convert.ToBoolean(saved["done"])) throw new Exception("Completion was not persisted");
            if((string)saved["description"] != "保留已有进展") throw new Exception("Completion changed the description");
            if(tasks.Nodes.Count != 0) throw new Exception("Completed task was not hidden");
            rendering = true; showCompleted.Checked = true; rendering = false; await Reload();
            if(tasks.Nodes.Count != 1 || !tasks.Nodes[0].Checked) throw new Exception("Completed task not visible or not checked");
            search.Text = createdTitle; await Reload();
            if(tasks.Nodes.Count != 1) throw new Exception("Completed search failed");
            await Complete(id, false);
            saved = await Api("GET", "/tasks/" + id, null);
            if(Convert.ToBoolean(saved["done"]) || tasks.Nodes[0].Checked) throw new Exception("Reopen failed");
            await Complete(id);
            SaveBounds();
            var settings = ReadObject(File.ReadAllText(Path.Combine(data, "floating-window.json")));
            if(!Convert.ToBoolean(settings["showCompleted"])) throw new Exception("Filter preference not saved");
            rendering = true; showCompleted.Checked = false; rendering = false; search.Clear(); await Reload();
            if(tasks.Nodes.Count != 0) throw new Exception("Hide completed failed");
            var project = projects.SelectedItem as Project;
            var ids = new List<long>();
            {
                for(int i = 0; i < 51; i++) { var task = await Api("POST", "/projects/" + project.Id + "/tasks", new { title = "分页验收事项 " + i }); ids.Add(Convert.ToInt64(task["id"])); }
                page = 1; await Reload(); if(tasks.Nodes.Count != 50 || !next.Enabled) throw new Exception("Pagination first page failed");
                page = 2; await Reload(); if(tasks.Nodes.Count < 1 || !previous.Enabled) throw new Exception("Pagination second page failed");
                search.Text = "分页验收事项 50"; page = 1; await Reload(); if(tasks.Nodes.Count != 1) throw new Exception("Search failed");
            }
            { foreach(long testId in ids) await Api("DELETE", "/tasks/" + testId, null); search.Clear(); page = 1; }
            string browserSession = await PrepareBrowserSession();
            var browserPair = ReadObject(browserSession);
            using(var request = new HttpRequestMessage(HttpMethod.Get, url + "/api/v2/projects")) {
                request.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", (string)browserPair["token"]);
                using(var response = await http.SendAsync(request)) if(!response.IsSuccessStatusCode) throw new Exception("Browser session failed");
            }
            token = "expired"; await Reload();
            if(status.ForeColor != ForeColor) throw new Exception("Session refresh failed");
            ToggleFold(); if(Height != 85) throw new Exception("Collapse failed"); ToggleFold();
            pin.Checked = false; if(TopMost) throw new Exception("Unpin failed"); pin.Checked = true;
            Close();
            if(closing || IsDisposed || WindowState != FormWindowState.Minimized || !ShowInTaskbar || !Visible) throw new Exception("Close must minimize to taskbar");
            await Api("GET", "/projects", null);
            RestoreWindow();
            rendering = true; showCompleted.Checked = true; rendering = false; await Reload();
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-test.png")); }
            File.WriteAllText(Path.Combine(data, "floating-test.txt"), "PASS: full error diagnostics, Windows error code, session redaction, hierarchy, nested indentation, collapse/expand retention, search ancestors, completed parent context, show/hide completed, reopen, completed search, saved filter preference, create, complete preserving description, 51-task pagination, search, independent browser session, refresh, pin, collapse, restore; TopMost=" + TopMost);
        } catch(Exception e) { File.WriteAllText(Path.Combine(data, "floating-test.txt"), "FAIL: " + e); Environment.ExitCode = 1; }
        finally { allowExit = true; Close(); }
    }
    protected override void Dispose(bool disposing) { if(disposing) { timer.Dispose(); tray.Dispose(); http.Dispose(); } base.Dispose(disposing); }
}
