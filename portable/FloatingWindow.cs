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
using System.Text.RegularExpressions;
using System.Linq;
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
    bool allowExit, simpleMode;
    FlowLayoutPanel toolbar;
    Rectangle fullBounds;
    Size simpleSize = new Size(300, 380);
    readonly Button restoreSimple = new Button { Text = "回到完整悬浮窗", AutoSize = true, Visible = false };
    readonly ToolTip progressTip = new ToolTip { AutoPopDelay = 20000, InitialDelay = 300, ReshowDelay = 200 };
    readonly Timer hoverTimer = new Timer { Interval = 400 };
    TreeNode hoverNode;
    sealed class OutstandingBranch { public long TaskId; public bool Loaded; }
    sealed class OutstandingLeaf { public long TaskId; public string Id, Html; }
    sealed class PendingItem { public string Id, Html; public override string ToString() { return Plain(Html); } }
    sealed class SharedList { public long CommentId; public List<PendingItem> Items = new List<PendingItem>(); }
    const string SharedHeading = "TaskTrace 遗留事项清单";

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
        ShowInTaskbar = false; Text = "TaskTrace · 悬浮事项"; Font = new Font("Microsoft YaHei UI", 9F);
        BackColor = Color.FromArgb(247, 249, 252); ForeColor = Color.FromArgb(31, 41, 55);
        Size = new Size(400, 560); MinimumSize = new Size(350, 300); TopMost = true; StartPosition = FormStartPosition.Manual;
        var area = Screen.PrimaryScreen.WorkingArea; Location = new Point(area.Right - Width - 24, area.Top + 60);
        LoadBounds(); LoadAutoSaveSettings(); LoadTreePreferences(); status.Click += delegate { ShowErrorDetails(); }; KeyPreview = true;
        toolbar = new FlowLayoutPanel { Dock = DockStyle.Top, Height = 46, Padding = new Padding(9, 6, 0, 0), WrapContents = false };
        var full = new Button { Text = "完整界面", AutoSize = true };
        var reload = new Button { Text = "刷新", AutoSize = true };
        var settingsButton = new Button { Text = "设置", AutoSize = true };
        settingsButton.Click += delegate { ShowAutoSaveSettings(); };
        toolbar.Height = 76; toolbar.WrapContents = true;
        var simple = new Button { Text = "简洁模式", AutoSize = true };
        simple.Click += delegate { SetSimpleMode(true); };
        toolbar.Controls.AddRange(new Control[] { full, reload, pin, fold, settingsButton, simple });
        restoreSimple.Click += delegate { SetSimpleMode(false); };
        Controls.Add(restoreSimple);
        Resize += delegate { restoreSimple.Location = new Point(Math.Max(6, ClientSize.Width - restoreSimple.Width - 10), 8); };
        Deactivate += delegate { if(simpleMode) restoreSimple.Visible = false; };
        tasks.NodeMouseClick += delegate { if(simpleMode) { restoreSimple.Visible = true; restoreSimple.BringToFront(); } };
        tasks.MouseDown += delegate(object sender, MouseEventArgs e) { if(simpleMode && e.Button == MouseButtons.Left && tasks.GetNodeAt(e.Location) == null) { ReleaseCapture(); SendMessage(Handle, 0xA1, new IntPtr(2), null); } };
        tasks.ShowNodeToolTips = false;
        tasks.MouseMove += delegate(object sender, MouseEventArgs e) { var node = tasks.GetNodeAt(e.Location); if(node != hoverNode) { hoverTimer.Stop(); progressTip.Hide(tasks); hoverNode = node; if(node != null && node.Tag is long) hoverTimer.Start(); } };
        tasks.MouseLeave += delegate { hoverTimer.Stop(); hoverNode = null; progressTip.Hide(tasks); };
        hoverTimer.Tick += async delegate {
            hoverTimer.Stop(); var node = hoverNode;
            if(node == null || !(node.Tag is long)) return;
            try {
                var history = DailyHistory(await ReadHistory((long)node.Tag));
                var latest = history.FirstOrDefault();
                string text = latest == null ? "暂无每日进展" : DayOf(latest) + "：" + String.Join("\r\n",history.Where(note=>DayOf(note)==DayOf(latest)).OrderBy(note=>Convert.ToInt64(note["id"])).Select(note=>Plain(ProgressBody((string)note["comment"]))));
                if(text.Length > 1500) text = text.Substring(0, 1500) + "…";
                if(!closing && node == hoverNode && node.TreeView == tasks) progressTip.Show(text, tasks, tasks.PointToClient(Cursor.Position).X + 12, tasks.PointToClient(Cursor.Position).Y + 18, 20000);
            } catch { if(!closing && node == hoverNode) progressTip.Show("进展读取失败，请重新悬停重试。", tasks, 20, 20, 5000); }
        };
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
            if(!(e.Node.Tag is long)) return;
            if(!busy) {
                long id = Convert.ToInt64(e.Node.Tag);
                bool done = !e.Node.Checked;
                BeginInvoke(new Action(async delegate { await Complete(id, done); }));
            }
        };
        tasks.NodeMouseDoubleClick += delegate(object sender, TreeNodeMouseClickEventArgs e) {
            if(e.Node.Tag is OutstandingBranch || e.Node.Tag is OutstandingLeaf) { var branch = e.Node.Tag as OutstandingBranch; var leaf = e.Node.Tag as OutstandingLeaf; ShowOutstanding(branch != null ? branch.TaskId : leaf.TaskId); }
            else if(e.Node.Tag is long) { tasks.SelectedNode = e.Node; ShowProgress(); }
        };
        tasks.AfterCollapse += delegate(object sender, TreeViewEventArgs e) {
            if(!rendering && e.Node.Tag is long && search.Text.Trim().Length == 0) { collapsedTasks.Add(Convert.ToInt64(e.Node.Tag)); SaveTreePreferences(); }
        };
        tasks.BeforeExpand += async delegate(object sender, TreeViewCancelEventArgs e) {
            var branch = e.Node.Tag as OutstandingBranch;
            if(branch == null || branch.Loaded || rendering) return;
            e.Cancel = true;
            try {
                var shared = ReadShared(await ReadHistory(branch.TaskId));
                if(e.Node.TreeView != tasks) return;
                e.Node.Nodes.Clear();
                foreach(var item in shared.Items) e.Node.Nodes.Add(new TreeNode(Plain(item.Html)) { Tag = new OutstandingLeaf { TaskId = branch.TaskId, Id = item.Id, Html = item.Html } });
                if(shared.Items.Count == 0) e.Node.Nodes.Add(new TreeNode("暂无遗留事项，双击此处添加") { Tag = new OutstandingBranch { TaskId = branch.TaskId, Loaded = true } });
                branch.Loaded = true; e.Node.Expand();
            } catch { if(!closing) { Error(new Exception("遗留事项读取失败，请重新展开重试。")); } }
        };
        tasks.AfterExpand += delegate(object sender, TreeViewEventArgs e) {
            if(!rendering && e.Node.Tag is long && search.Text.Trim().Length == 0) { collapsedTasks.Remove(Convert.ToInt64(e.Node.Tag)); SaveTreePreferences(); }
        };
        // Both close and minimize keep the application available only in the notification area.
        Resize += delegate { if(!closing && WindowState == FormWindowState.Minimized) HideToTray(); };
        tray.DoubleClick += delegate { RestoreWindow(); };
        var menu = new ContextMenuStrip();
        menu.Items.Add("显示悬浮窗", null, delegate { RestoreWindow(); });
        menu.Items.Add("完整界面", null, async delegate { await OpenFull(); });
        menu.Items.Add("退出 TaskTrace", null, delegate { allowExit = true; Close(); }); tray.ContextMenuStrip = menu;
        timer.Tick += async delegate { if(Visible && !collapsed && !busy) { projectsDirty = true; await Reload(); } };
        Shown += async delegate { await Reload();
            if(!selfTest) try { var prefs = ReadObject(File.ReadAllText(Path.Combine(data, "simple-window.json"))); simpleSize = new Size(Math.Max(160, Convert.ToInt32(prefs["width"])), Math.Max(120, Convert.ToInt32(prefs["height"]))); if(Convert.ToBoolean(prefs["enabled"])) SetSimpleMode(true); } catch { }
            timer.Start(); if(selfTest) await TestFlow(); else if(openBrowser) await OpenFull(); };
        FormClosing += delegate(object sender, FormClosingEventArgs e) {
            if(e.CloseReason == CloseReason.UserClosing && !allowExit) {
                e.Cancel = true; HideToTray();
                return;
            }
            closing = true; timer.Stop(); SaveSimpleMode(); SaveBounds(); tray.Visible = false;
        };
    }
    [DllImport("user32.dll")] static extern bool ReleaseCapture();
    protected override void WndProc(ref Message message) {
        base.WndProc(ref message);
        if(simpleMode && message.Msg == 0x84) {
            var point = PointToClient(Cursor.Position);
            bool left = point.X < 7, right = point.X >= ClientSize.Width - 7, top = point.Y < 7, bottom = point.Y >= ClientSize.Height - 7;
            if(top) message.Result = new IntPtr(left ? 13 : right ? 14 : 12);
            else if(bottom) message.Result = new IntPtr(left ? 16 : right ? 17 : 15);
            else if(left || right) message.Result = new IntPtr(left ? 10 : 11);
        }
    }
    void SetSimpleMode(bool enabled) {
        if(enabled == simpleMode) return;
        if(enabled) {
            if(collapsed) ToggleFold(); fullBounds = Bounds; simpleMode = true;
            content.Controls.Remove(tasks); content.Visible = false; toolbar.Visible = false;
            FormBorderStyle = FormBorderStyle.None; MinimumSize = new Size(160,120); Padding = new Padding(7);
            Controls.Add(tasks); tasks.Dock = DockStyle.Fill; Size = simpleSize;
            restoreSimple.Visible = false;
        } else {
            simpleSize = Size; simpleMode = false; Controls.Remove(tasks); Padding = Padding.Empty;
            FormBorderStyle = FormBorderStyle.Sizable; MinimumSize = new Size(350,300);
            content.Controls.Add(tasks, 0, 3); content.Visible = true; toolbar.Visible = true; restoreSimple.Visible = false; Bounds = fullBounds;
        }
        SaveSimpleMode();
    }
    void SaveSimpleMode() {
        try { if(simpleMode && WindowState == FormWindowState.Normal) simpleSize = Size; File.WriteAllText(Path.Combine(data,"simple-window.json"), json.Serialize(new {enabled=simpleMode,width=simpleSize.Width,height=simpleSize.Height})); } catch { }
    }
    static string Plain(string html) {
        string value = Regex.Replace(html ?? "", @"<br\s*/?>|</p>|</div>|</li>", "\r\n", RegexOptions.IgnoreCase);
        return WebUtility.HtmlDecode(Regex.Replace(value, "<[^>]+>", "")).Trim();
    }
    async Task<List<Dictionary<string,object>>> ReadHistory(long id) {
        var notes = new List<Dictionary<string,object>>();
        for(int pageIndex=1; ;pageIndex++) {
            var result = await Api("GET", "/tasks/"+id+"/comments?per_page=100&page="+pageIndex+"&order_by=desc", null);
            int count=0; foreach(Dictionary<string,object> note in (IEnumerable)result["items"]) { notes.Add(note); count++; }
            if(count==0 || pageIndex>=Convert.ToInt32(result["total_pages"])) break;
        }
        return notes;
    }
    static string DayOf(Dictionary<string,object> note) {
        var match = Regex.Match((string)note["comment"], @"<h3[^>]*>每日进展\s*[·:：]\s*(\d{4}-\d{2}-\d{2})</h3>");
        return match.Success ? match.Groups[1].Value : "";
    }
    static List<long> MergedIds(string html) {
        var match = Regex.Match(html, "data-tasktrace-merged=\"([0-9,]*)\"");
        var result = new List<long>(); long id;
        foreach(string value in match.Groups[1].Value.Split(',')) if(long.TryParse(value,out id) && id>0) result.Add(id);
        return result;
    }
    static List<Dictionary<string,object>> DailyHistory(List<Dictionary<string,object>> notes) {
        var hidden = new HashSet<long>(notes.SelectMany(note => MergedIds((string)note["comment"])));
        return notes.Where(note => DayOf(note)!="" && !hidden.Contains(Convert.ToInt64(note["id"]))).OrderByDescending(note => DayOf(note)).ThenByDescending(note => Convert.ToInt64(note["id"])).ToList();
    }
    static string ProgressBody(string html) {
        html = Regex.Replace(html, @"<h3[^>]*>.*?</h3>", "", RegexOptions.Singleline);
        return Regex.Replace(html, @"<p>\s*<strong>遗留问题 / 下一步</strong>\s*</p>\s*<p>.*?</p>", "", RegexOptions.Singleline);
    }
    static SharedList ReadShared(List<Dictionary<string,object>> notes) {
        var list = new SharedList();
        var record = notes.OrderByDescending(note => Convert.ToInt64(note["id"])).FirstOrDefault(note => ((string)note["comment"]).Contains("<h3>"+SharedHeading+"</h3>"));
        if(record != null) {
            list.CommentId = Convert.ToInt64(record["id"]);
            foreach(Match match in Regex.Matches((string)record["comment"], "<li data-id=\"([^\"]+)\">(.*?)</li>", RegexOptions.Singleline)) list.Items.Add(new PendingItem {Id=match.Groups[1].Value,Html=match.Groups[2].Value});
        } else {
            var latest = DailyHistory(notes).FirstOrDefault();
            if(latest != null) {
                var match = Regex.Match((string)latest["comment"], @"<p>\s*<strong>遗留问题 / 下一步</strong>\s*</p>\s*<p>(.*?)</p>", RegexOptions.Singleline);
                int index=0;
                foreach(string value in Regex.Split(match.Groups[1].Value, @"<br\s*/?>")) if(value.Trim()!="") list.Items.Add(new PendingItem {Id="legacy-"+latest["id"]+"-"+(index++),Html=value});
            }
        }
        return list;
    }
    async Task WriteShared(long id, SharedList list) {
        string html = "<h3>"+SharedHeading+"</h3><ul>"+String.Join("",list.Items.Select(item => "<li data-id=\""+item.Id+"\">"+item.Html+"</li>"))+"</ul>";
        var saved = await Api(list.CommentId==0 ? "POST" : "PUT", "/tasks/"+id+"/comments"+(list.CommentId==0 ? "" : "/"+list.CommentId), new {comment=html});
        list.CommentId = Convert.ToInt64(saved["id"]);
    }
    async void ShowOutstanding(long id, bool nested = false) {
        if(busy && !nested) return;
        var owner = Form.ActiveForm ?? this;
        SetBusy(true); timer.Stop();
        try {
            var shared = ReadShared(await ReadHistory(id));
            using(var dialog = new Form {Text="遗留事项 · 所有日期共享",Size=new Size(480,380),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false}) {
                var list = new ListBox {Dock=DockStyle.Fill};
                var input = new TextBox {Dock=DockStyle.Top,AccessibleName="新增遗留事项"};
                var buttons = new FlowLayoutPanel {Dock=DockStyle.Bottom,Height=40};
                var add = new Button {Text="添加一条",AutoSize=true}; var remove = new Button {Text="移除选中",AutoSize=true};
                var feedback = new Label {Dock=DockStyle.Bottom,Height=32,Text="逐条添加，切换进展日期不会改变此清单。"};
                Action render = delegate {list.Items.Clear();foreach(var item in shared.Items) list.Items.Add(item);};render();
                bool writing=false;
                Func<bool,Task> save = async delegate(bool adding) {
                    if(writing || (adding && String.IsNullOrWhiteSpace(input.Text)) || (!adding && list.SelectedItem==null)) return;
                    var selected = list.SelectedItem as PendingItem; writing=true; buttons.Enabled=false;input.Enabled=false;list.Enabled=false;
                    try { var current = ReadShared(await ReadHistory(id)); if(adding) current.Items.Add(new PendingItem {Id=Guid.NewGuid().ToString(),Html=WebUtility.HtmlEncode(input.Text.Trim())}); else current.Items.RemoveAll(item=>item.Id==selected.Id); await WriteShared(id,current); shared=current;render();if(adding)input.Clear();feedback.Text="已保存。"; }
                    catch {feedback.Text="保存失败，输入已保留，请重试。";}
                    finally {writing=false;buttons.Enabled=true;input.Enabled=true;list.Enabled=true;}
                };
                add.Click+=async delegate {await save(true);};remove.Click+=async delegate {await save(false);};
                input.KeyDown+=async delegate(object sender,KeyEventArgs e){if(e.KeyCode==Keys.Enter){e.SuppressKeyPress=true;await save(true);}};
                buttons.Controls.AddRange(new Control[]{add,remove});dialog.Controls.Add(list);dialog.Controls.Add(input);dialog.Controls.Add(feedback);dialog.Controls.Add(buttons);
                dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(writing)e.Cancel=true;};dialog.ShowDialog(owner);
            }
            await LoadTasks();
        } catch(Exception e){Error(e);}finally {if(!nested){SetBusy(false);timer.Start();}}
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
        long selectedId = tasks.SelectedNode == null || !(tasks.SelectedNode.Tag is long) ? 0 : (long)tasks.SelectedNode.Tag;
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
            if(!selfTest) foreach(var pair in nodes) {
                if(!parents.ContainsKey(pair.Key)) continue;
                var branch = new TreeNode("遗留事项（展开查看，双击管理）") { Tag = new OutstandingBranch { TaskId = pair.Key } };
                branch.Nodes.Add(new TreeNode("读取中…")); pair.Value.Nodes.Add(branch);
            }
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
    async Task<long> SaveProgress(long id, DateTime day, string progress, string nextStep, List<PastedImage> pictures = null, long commentId = 0, string bodyOverride = null, List<long> mergedIds = null) {
        string note = "<h3>每日进展 · " + day.ToString("yyyy-MM-dd") + "</h3><p>" + WebUtility.HtmlEncode(progress.Trim()).Replace("\r\n", "<br>").Replace("\n", "<br>") + "</p>";
        if(bodyOverride != null) note = "<h3 data-tasktrace-merged=\"" + String.Join(",", mergedIds ?? new List<long>()) + "\">每日进展 · " + day.ToString("yyyy-MM-dd") + "</h3>" + bodyOverride;
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
    sealed class ProgressDraft { public string Text; public List<PastedImage> Pictures; }
    async void ShowProgress() {
        if(busy || closing) return;
        if(tasks.SelectedNode == null || !(tasks.SelectedNode.Tag is long)) { status.Text="请先选中事项。";return; }
        long id=(long)tasks.SelectedNode.Tag; string taskTitle=tasks.SelectedNode.Text;
        SetBusy(true); timer.Stop();
        try {
            var history = await ReadHistory(id);
            using(var dialog=new Form {Text="每日进展 · "+taskTitle,Size=new Size(480,500),MinimumSize=new Size(380,420),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false}) {
                var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=6};
                layout.RowStyles.Add(new RowStyle(SizeType.Absolute,32));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,30));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,36));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,36));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,52));
                var day=new DateTimePicker {Format=DateTimePickerFormat.Custom,CustomFormat="yyyy-MM-dd",Value=DateTime.Today,Dock=DockStyle.Fill};
                var progress=new TextBox {Multiline=true,AcceptsReturn=true,ScrollBars=ScrollBars.Vertical,Dock=DockStyle.Fill,AccessibleName="今日进展"};
                var sharedButton=new Button {Text="遗留事项 · 所有日期共享",Dock=DockStyle.Fill};
                sharedButton.Click+=delegate {ShowOutstanding(id,true);};
                var save=new Button {Text="保存当天进展 (Ctrl+Enter)",Dock=DockStyle.Fill};
                var feedback=new Label {Dock=DockStyle.Fill};
                layout.Controls.Add(day);layout.Controls.Add(new Label {Text="同一天的记录合并编辑，历史图片保留",Dock=DockStyle.Fill});layout.Controls.Add(progress);layout.Controls.Add(sharedButton);layout.Controls.Add(save);layout.Controls.Add(feedback);dialog.Controls.Add(layout);
                var drafts=new Dictionary<string,ProgressDraft>();var pictures=new List<PastedImage>();
                string selectedDay="",originalBody="",originalText="",lastSaved="";long commentId=0;var mergedIds=new List<long>();bool submitting=false;
                Func<string> snapshot=delegate {return json.Serialize(new {date=selectedDay,text=progress.Text,images=pictures.Count});};
                Action stash=delegate {if(selectedDay!="" && snapshot()!=lastSaved)drafts[selectedDay]=new ProgressDraft {Text=progress.Text,Pictures=new List<PastedImage>(pictures)};};
                Action loadDay=delegate {
                    selectedDay=day.Value.ToString("yyyy-MM-dd");
                    var records=DailyHistory(history).Where(note=>DayOf(note)==selectedDay).OrderBy(note=>Convert.ToInt64(note["id"])).ToList();
                    commentId=records.Count==0?0:records.Max(note=>Convert.ToInt64(note["id"]));
                    mergedIds=records.Select(note=>Convert.ToInt64(note["id"])).Concat(records.SelectMany(note=>MergedIds((string)note["comment"]))).Where(value=>value!=commentId).Distinct().ToList();
                    originalBody=String.Join("",records.Select(note=>ProgressBody((string)note["comment"])));originalText=Plain(Regex.Replace(originalBody,"<img[^>]*>","",RegexOptions.IgnoreCase));
                    progress.Text=originalText;pictures=new List<PastedImage>();lastSaved=snapshot();
                    if(drafts.ContainsKey(selectedDay)){progress.Text=drafts[selectedDay].Text;pictures=drafts[selectedDay].Pictures;}
                    feedback.Text=records.Count==0?"此日期尚无进展。支持 Ctrl+V 粘贴图片。":"已载入当天合并内容；保存更新当天记录，原图片保留。";
                };
                loadDay();day.ValueChanged+=delegate {if(!submitting){stash();loadDay();}};
                Func<bool,Task> write=async delegate(bool finish) {
                    if(submitting || (String.IsNullOrWhiteSpace(progress.Text) && pictures.Count==0 && commentId==0))return;
                    if(snapshot()==lastSaved && mergedIds.Count==0){if(finish)feedback.Text="没有需要保存的修改。";return;}
                    submitting=true;day.Enabled=false;save.Enabled=false;sharedButton.Enabled=false;progress.ReadOnly=true;
                    try {
                        var sharedBeforeSave=ReadShared(await ReadHistory(id));if(sharedBeforeSave.CommentId==0)await WriteShared(id,sharedBeforeSave);
                        string images=String.Join("",Regex.Matches(originalBody,"<img[^>]*>",RegexOptions.IgnoreCase).Cast<Match>().Select(match=>match.Value));
                        string body=progress.Text==originalText?originalBody:"<p>"+WebUtility.HtmlEncode(progress.Text.Trim()).Replace("\r\n","<br>").Replace("\n","<br>")+"</p>"+images;
                        commentId=await SaveProgress(id,day.Value,progress.Text,"",pictures,commentId,body,mergedIds);
                        // Preserve uploaded images without adding them again on the next edit.
                        foreach(var picture in pictures) body+="<p><img src=\"/api/v1/tasks/"+id+"/attachments/"+picture.Id+"\"></p>";
                        originalBody=body;originalText=progress.Text;pictures.Clear();lastSaved=snapshot();drafts.Remove(selectedDay);
                        history=await ReadHistory(id);feedback.Text=finish?"当天进展已保存，可以继续编辑。":"当天进展已自动保存。";
                    } catch {feedback.Text="保存失败，内容已保留，请重试。";}
                    finally{submitting=false;day.Enabled=true;save.Enabled=true;sharedButton.Enabled=true;progress.ReadOnly=false;}
                };
                save.Click+=async delegate {await write(true);};
                var autoTimer=new Timer {Interval=autoSaveSeconds*1000};autoTimer.Tick+=async delegate {if(autoSaveEnabled && snapshot()!=lastSaved)await write(false);};autoTimer.Start();
                dialog.KeyPreview=true;dialog.KeyDown+=delegate(object sender,KeyEventArgs e){
                    if(e.Control && e.KeyCode==Keys.V && !submitting && Clipboard.ContainsImage()) {e.SuppressKeyPress=true;try{using(var image=Clipboard.GetImage())using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage {Bytes=stream.ToArray()});}feedback.Text="已粘贴 "+pictures.Count+" 张图片。";}catch{feedback.Text="剪贴板读取失败，请重试。";}}
                    if(e.Control && e.KeyCode==Keys.Enter){e.SuppressKeyPress=true;save.PerformClick();}
                };
                dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(submitting){e.Cancel=true;return;}stash();if(drafts.Count>0 && MessageBox.Show(dialog,"还有日期的进展未保存，确定放弃这些修改？","每日进展",MessageBoxButtons.YesNo)!=DialogResult.Yes)e.Cancel=true;};
                try{dialog.ShowDialog(this);}finally{autoTimer.Stop();autoTimer.Dispose();}
            }
        }catch(Exception e){Error(e);}finally{SetBusy(false);timer.Start();}
    }
    const string TaskDepthMessage = "任务最多支持 5 级（顶层任务为第 1 级），无法继续添加子任务。";
    async Task<int> TaskHierarchySpan(long id, int level = 1) {
        if(level >= 5) return 1;
        var task = await Api("GET", "/tasks/" + id, null);
        var relations = task["related_tasks"] as Dictionary<string,object>;
        int span = 1;
        if(relations != null && relations.ContainsKey("parenttask")) foreach(Dictionary<string,object> parent in (IEnumerable)relations["parenttask"]) {
            span = Math.Max(span, 1 + await TaskHierarchySpan(Convert.ToInt64(parent["id"]), level + 1));
            if(span >= 6-level) break;
        }
        return span;
    }
    async Task<long> CreateSubtask(long parentId, long projectId, string title, long existingId = 0) {
        if(await TaskHierarchySpan(parentId) >= 5) throw new Exception(TaskDepthMessage);
        long childId = existingId;
        if(childId == 0) { var child = await Api("POST", "/projects/" + projectId + "/tasks", new { title = title }); childId = Convert.ToInt64(child["id"]); }
        await Api("POST", "/tasks/" + parentId + "/relations", new { other_task_id = childId, relation_kind = "subtask" });
        return childId;
    }
    async Task ShowSubtasks() {
        if(busy || closing) return;
        if(tasks.SelectedNode == null || !(tasks.SelectedNode.Tag is long)) { status.Text = "请先选中一个父事项。"; return; }
        long parentId = Convert.ToInt64(tasks.SelectedNode.Tag);
        SetBusy(true); timer.Stop();
        try {
            var parent = await Api("GET", "/tasks/" + parentId, null);
            long projectId = Convert.ToInt64(parent["project_id"]);
            using(var dialog = new Form { Text = "子任务 · " + (string)parent["title"], Size = new Size(450, 430), MinimumSize = new Size(380, 320), Font = Font, TopMost = TopMost, StartPosition = FormStartPosition.CenterParent, ShowInTaskbar = false }) {
                var list = new ListView { Dock = DockStyle.Fill, View = View.Details, CheckBoxes = true, FullRowSelect = true, HeaderStyle = ColumnHeaderStyle.None };
                list.Columns.Add("子任务", 390);
                var title = new TextBox { Dock = DockStyle.Fill };
                var feedback = new Label { Dock = DockStyle.Bottom, Height = 46, Text = "子任务独立完成；在主列表中选择它可记录每日进展。" };
                var row = new TableLayoutPanel { Dock = DockStyle.Top, Height = 38, ColumnCount = 2 };
                row.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); row.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 95));
                var add = new Button { Text = "添加子任务", Dock = DockStyle.Fill }; row.Controls.Add(title); row.Controls.Add(add);
                var renameTitle = new TextBox { Dock = DockStyle.Fill, MaxLength = 250, AccessibleName = "修改子任务名称" };
                var rename = new Button { Text = "保存名称", Dock = DockStyle.Fill, Enabled = false };
                var renameRow = new TableLayoutPanel { Dock = DockStyle.Top, Height = 38, ColumnCount = 2 };
                renameRow.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); renameRow.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 95));
                renameRow.Controls.Add(renameTitle); renameRow.Controls.Add(rename);
                dialog.Controls.Add(list); dialog.Controls.Add(renameRow); dialog.Controls.Add(row); dialog.Controls.Add(feedback);
                bool loading = false, writing = false; long pendingId = 0; bool depthLimit = false;
                Func<Task> reload = async delegate {
                    loading = true;
                    try {
                        depthLimit = await TaskHierarchySpan(parentId) >= 5;
                        add.Enabled = !depthLimit; title.Enabled = !depthLimit && pendingId == 0;
                        if(depthLimit) feedback.Text = TaskDepthMessage;
                        var current = await Api("GET", "/tasks/" + parentId, null);
                        list.Items.Clear();
                        var relations = current["related_tasks"] as Dictionary<string, object>;
                        if(relations != null && relations.ContainsKey("subtask")) foreach(Dictionary<string, object> child in (IEnumerable)relations["subtask"]) list.Items.Add(new ListViewItem((string)child["title"]) { Tag = Convert.ToInt64(child["id"]), Checked = Convert.ToBoolean(child["done"]) });
                    } finally { loading = false; }
                };
                list.SelectedIndexChanged += delegate {
                    if(writing) return;
                    rename.Enabled = list.SelectedItems.Count > 0;
                    renameTitle.Text = rename.Enabled ? list.SelectedItems[0].Text : "";
                };
                rename.Click += async delegate {
                    if(writing || list.SelectedItems.Count == 0 || String.IsNullOrWhiteSpace(renameTitle.Text)) return;
                    long childId = Convert.ToInt64(list.SelectedItems[0].Tag);
                    writing = true; rename.Enabled = false; renameTitle.Enabled = false; list.Enabled = false;
                    try {
                        await Api("PATCH", "/tasks/" + childId, new { title = renameTitle.Text.Trim() });
                        await reload(); renameTitle.Clear(); feedback.Text = "子任务名称已保存。";
                    } catch { feedback.Text = "名称保存失败，输入已保留，请重试。"; }
                    finally { writing = false; renameTitle.Enabled = true; list.Enabled = true; rename.Enabled = list.SelectedItems.Count > 0; }
                };
                renameTitle.KeyDown += delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; rename.PerformClick(); } };
                add.Click += async delegate {
                    if(writing || depthLimit || string.IsNullOrWhiteSpace(title.Text)) return;
                    writing = true; add.Enabled = false; title.Enabled = false;
                    try {
                        if(await TaskHierarchySpan(parentId) >= 5) { depthLimit = true; throw new Exception(TaskDepthMessage); }
                        if(pendingId == 0) { var child = await Api("POST", "/projects/" + projectId + "/tasks", new { title = title.Text.Trim() }); pendingId = Convert.ToInt64(child["id"]); }
                        await CreateSubtask(parentId, projectId, title.Text, pendingId);
                        pendingId = 0; title.Clear(); feedback.Text = "子任务已添加。可独立勾选完成。"; add.Text = "添加子任务"; await reload();
                    } catch { feedback.Text = depthLimit ? TaskDepthMessage : pendingId == 0 ? "创建失败，请重试。" : "事项已创建，关联失败；点击重试，不会重复创建。"; add.Text = pendingId == 0 ? "添加子任务" : "重试关联"; }
                    finally { writing = false; add.Enabled = !depthLimit; title.Enabled = !depthLimit && pendingId == 0; }
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
        using(var dialog = new Form { Text = "TaskTrace · 错误详情（可复制）", Width = 740, Height = 480, StartPosition = FormStartPosition.CenterParent, ShowInTaskbar = false }) {
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
    void HideToTray() {
        SaveSimpleMode(); SaveBounds(); hoverTimer.Stop(); progressTip.Hide(tasks);
        restoreSimple.Visible = false; Hide();
    }
    void RestoreWindow() { WindowState = FormWindowState.Normal; Show(); Activate(); }
    void ToggleFold() { if(!collapsed) { expandedHeight = Height; content.Visible = false; MinimumSize = new Size(350, 85); Height = 85; collapsed = true; fold.Text = "展开"; } else { collapsed = false; content.Visible = true; MinimumSize = new Size(350, 300); Height = expandedHeight; fold.Text = "收起"; } }
    void LoadAutoSaveSettings() {
        try {
            var settings = ReadObject(File.ReadAllText(Path.Combine(data, "autosave.json")));
            autoSaveEnabled = Convert.ToBoolean(settings["enabled"]);
            autoSaveSeconds = Math.Max(5, Math.Min(3600, Convert.ToInt32(settings["seconds"])));
        } catch { }
    }
    void ShowAutoSaveSettings() {
        using(var settings = new Form { Text = "设置", Size = new Size(430, 345), FormBorderStyle = FormBorderStyle.FixedDialog, MaximizeBox = false, MinimizeBox = false, StartPosition = FormStartPosition.CenterParent, Font = Font, TopMost = TopMost, ShowInTaskbar = false }) {
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
        try { var b = simpleMode ? fullBounds : (WindowState == FormWindowState.Normal ? Bounds : RestoreBounds); File.WriteAllText(Path.Combine(data, "floating-window.json"), json.Serialize(new { x = b.X, y = b.Y, width = b.Width, height = collapsed ? expandedHeight : b.Height, showCompleted = showCompleted.Checked })); } catch { }
    }
    async Task TestFlow() {
        try {
            Error(new Exception("Diagnostic test", new System.ComponentModel.Win32Exception(1155, "No default browser #tasktrace-local=TEST_PRIVATE_SESSION")));
            string diagnostic = File.ReadAllText(Path.Combine(data, "TaskTrace-window-error.log"));
            if(!diagnostic.Contains("Windows error code: 1155") || diagnostic.Contains("TEST_PRIVATE_SESSION") || !lastError.Contains("错误日志")) throw new Exception("Error diagnostics incomplete or leaked session");
            await Reload();
            if(projects.Items.Count == 0 || !TopMost || ShowInTaskbar || !tray.Visible) throw new Exception("Workspace, TopMost or tray-only startup failed");
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
            await Api("PATCH", "/tasks/" + childId, new { title = "子任务验收改名" });
            var renamedChild = await Api("GET", "/tasks/" + childId, null);
            if((string)renamedChild["title"] != "子任务验收改名") throw new Exception("Subtask rename not persisted");
            var parentWithChild = await Api("GET", "/tasks/" + id, null);
            if(!json.Serialize(parentWithChild["related_tasks"]).Contains("子任务验收")) throw new Exception("Subtask relationship missing");
            await Api("PATCH", "/tasks/" + childId, new { done = true });
            if(Convert.ToBoolean((await Api("GET", "/tasks/" + id, null))["done"])) throw new Exception("Child completion incorrectly completed parent");
            await Api("PATCH", "/tasks/" + childId, new { done = false });
            var sharedTest = new SharedList(); sharedTest.Items.Add(new PendingItem {Id="test-one",Html="跨日期待办一"}); sharedTest.Items.Add(new PendingItem {Id="test-two",Html="跨日期待办二"});
            await WriteShared(childId,sharedTest);
            long oldDay=await SaveProgress(childId,DateTime.Today.AddDays(-2),"第一条", "");
            long sameDay=await SaveProgress(childId,DateTime.Today.AddDays(-2),"第二条", "");
            await SaveProgress(childId,DateTime.Today.AddDays(-2),"合并编辑", "",null,sameDay,"<p>合并编辑</p>",new List<long>{oldDay});
            var sharedHistory=await ReadHistory(childId);
            if(ReadShared(sharedHistory).Items.Count!=2 || DailyHistory(sharedHistory).Count!=1 || Plain(ProgressBody((string)DailyHistory(sharedHistory)[0]["comment"]))!="合并编辑")throw new Exception("Shared outstanding or merged history failed");
            sharedTest.Items.RemoveAt(0);await WriteShared(childId,sharedTest);
            if(ReadShared(await ReadHistory(childId)).Items.Count!=1)throw new Exception("Individual outstanding removal failed");
            var beforeSimple=Bounds;SetSimpleMode(true);Size=new Size(230,220);
            if(!simpleMode || content.Visible || toolbar.Visible || tasks.Parent!=this || FormBorderStyle!=FormBorderStyle.None || restoreSimple.Visible)throw new Exception("Simple mode layout failed");
            SetSimpleMode(false);if(Bounds!=beforeSimple || tasks.Parent!=content || !toolbar.Visible)throw new Exception("Restore full floating window failed");
            long grandchildId = await CreateSubtask(childId, Convert.ToInt64(parentWithChild["project_id"]), "下级子任务验收");
            await LoadTasks();
            if(tasks.Nodes.Count != 1 || tasks.Nodes[0].Nodes.Count != 1 || tasks.Nodes[0].Nodes[0].Nodes.Count != 1) throw new Exception("Task hierarchy missing");
            if(tasks.Nodes[0].Nodes[0].Level != 1 || tasks.Nodes[0].Nodes[0].Nodes[0].Level != 2) throw new Exception("Subtask indentation missing");
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-tree-test.png")); }
            var outstandingTest = new TreeNode("遗留事项") {Tag = new OutstandingBranch {TaskId=childId}};
            outstandingTest.Nodes.Add(new TreeNode("读取中…")); tasks.Nodes[0].Nodes[0].Nodes.Add(outstandingTest); outstandingTest.Expand();
            for(int attempt=0;attempt<100 && !((OutstandingBranch)outstandingTest.Tag).Loaded;attempt++) await Task.Delay(50);
            if(!outstandingTest.IsExpanded || outstandingTest.Nodes.Count!=1 || outstandingTest.Nodes[0].Text!="跨日期待办二")throw new Exception("Outstanding dropdown failed");
            SetSimpleMode(true);Size=new Size(330,260);
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-simple-test.png")); }
            restoreSimple.Visible=true;restoreSimple.BringToFront();
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-simple-selected-test.png")); }
            restoreSimple.PerformClick();if(simpleMode)throw new Exception("Simple mode restore button failed");
            outstandingTest.Remove();
            tasks.Nodes[0].Collapse(); collapsedTasks.Clear(); LoadTreePreferences(); await LoadTasks();
            if(tasks.Nodes[0].IsExpanded || !collapsedTasks.Contains(id)) throw new Exception("Collapsed state not retained");
            search.Text = "下级子任务验收"; await LoadTasks();
            if(tasks.Nodes.Count != 1 || !tasks.Nodes[0].IsExpanded || tasks.Nodes[0].Nodes[0].Nodes.Count != 1) throw new Exception("Search lost hierarchy");
            search.Clear(); await LoadTasks(); tasks.Nodes[0].Expand();
            if(collapsedTasks.Contains(id)) throw new Exception("Expand state not retained");
            await Api("PATCH", "/tasks/" + id, new { done = true }); await LoadTasks();
            if(tasks.Nodes.Count != 1 || !tasks.Nodes[0].Checked || tasks.Nodes[0].Nodes.Count != 1) throw new Exception("Completed ancestor lost pending children");
            await Api("PATCH", "/tasks/" + id, new { done = false });
            long levelFour = await CreateSubtask(grandchildId, Convert.ToInt64(parentWithChild["project_id"]), "第四级验收");
            long levelFive = await CreateSubtask(levelFour, Convert.ToInt64(parentWithChild["project_id"]), "第五级验收");
            if(await TaskHierarchySpan(levelFive) != 5) throw new Exception("Five-level depth calculation failed");
            int tasksBeforeRejection = Convert.ToInt32((await Api("GET", "/tasks", null))["total"]);
            bool sixthBlocked = false;
            try { await CreateSubtask(levelFive, Convert.ToInt64(parentWithChild["project_id"]), "不应创建的第六级"); }
            catch(Exception e) { if(e.Message == TaskDepthMessage) sixthBlocked = true; else throw; }
            if(!sixthBlocked) throw new Exception("Sixth level was not blocked before creation");
            var rejectedTasks = await Api("GET", "/tasks", null);
            if(Convert.ToInt32(rejectedTasks["total"]) != tasksBeforeRejection) throw new Exception("Depth rejection left an orphan task");
            await Api("DELETE", "/tasks/" + levelFive, null);
            await Api("DELETE", "/tasks/" + levelFour, null);
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
            if(closing || IsDisposed || ShowInTaskbar || Visible || !tray.Visible) throw new Exception("Close must hide to tray without a taskbar entry");
            await Api("GET", "/projects", null);
            RestoreWindow();
            if(!Visible || WindowState != FormWindowState.Normal || ShowInTaskbar) throw new Exception("Tray restore failed");
            WindowState = FormWindowState.Minimized;
            if(Visible || ShowInTaskbar || !tray.Visible) throw new Exception("Minimize must hide to tray");
            RestoreWindow(); SetSimpleMode(true); Close();
            if(Visible || ShowInTaskbar || !tray.Visible || closing) throw new Exception("Simple window must hide to tray");
            RestoreWindow();
            if(!simpleMode || !Visible || ShowInTaskbar) throw new Exception("Simple window tray restore failed");
            SetSimpleMode(false);
            rendering = true; showCompleted.Checked = true; rendering = false; await Reload();
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-test.png")); }
            File.WriteAllText(Path.Combine(data, "floating-test.txt"), "PASS: five-level task limit, rejected sixth level without orphan, tray-only startup, close/minimize to tray, full/simple tray restore, simple mode, resizing, restore button, outstanding dropdown, shared list, same-day merge, full error diagnostics, Windows error code, session redaction, hierarchy, nested indentation, collapse/expand retention, search ancestors, completed parent context, show/hide completed, reopen, completed search, saved filter preference, create, complete preserving description, 51-task pagination, search, independent browser session, refresh, pin, collapse, restore; TopMost=" + TopMost);
        } catch(Exception e) { File.WriteAllText(Path.Combine(data, "floating-test.txt"), "FAIL: " + e); Environment.ExitCode = 1; }
        finally { allowExit = true; Close(); }
    }
    protected override void Dispose(bool disposing) { if(disposing) { timer.Dispose(); hoverTimer.Dispose(); progressTip.Dispose(); tray.Dispose(); http.Dispose(); } base.Dispose(disposing); }
}
