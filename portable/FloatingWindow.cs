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

internal sealed partial class FloatingWindow : Form {
    readonly JavaScriptSerializer json = new JavaScriptSerializer { MaxJsonLength = 8388608 };
    readonly HttpClient http = new HttpClient(new HttpClientHandler { UseProxy = false });
    readonly string root, url, data;
    string token, refresh;
    readonly System.Threading.SemaphoreSlim refreshGate = new System.Threading.SemaphoreSlim(1,1);
    readonly ComboBox projects = new ComboBox { Dock = DockStyle.Fill, DropDownStyle = ComboBoxStyle.DropDownList, DisplayMember = "Title" };
    readonly TextBox entry = new TextBox { Dock = DockStyle.Fill, AccessibleName = "新事项名称" };
    readonly ComboBox entryPriority = new ComboBox { Dock = DockStyle.Fill, DropDownStyle = ComboBoxStyle.DropDownList, AccessibleName = "新事项优先级" };
    readonly TextBox search = new TextBox { Dock = DockStyle.Fill, AccessibleName = "查找事项" };
    readonly Button newTaskButton = new Button { Text = "新事项", AutoSize = false, Width = 66, AccessibleName = "添加新事项" };
    readonly FlowLayoutPanel bottomActions = new FlowLayoutPanel { Dock = DockStyle.Fill, AutoSize = true, AutoSizeMode = AutoSizeMode.GrowAndShrink, WrapContents = false, FlowDirection = FlowDirection.LeftToRight, Margin = Padding.Empty };
    readonly TaskTreeView tasks = new TaskTreeView { Dock = DockStyle.Fill, HideSelection = false, ShowLines = true, ShowRootLines = true, ShowPlusMinus = true, ShowNodeToolTips = true, Indent = 20, ItemHeight = 28, AccessibleName = "任务与子任务" };
    readonly HashSet<long> collapsedTasks = new HashSet<long>();
    readonly Label status = new Label { Dock = DockStyle.Fill, AutoEllipsis = true, TextAlign = ContentAlignment.MiddleLeft };
    readonly CheckBox showCompleted = new CheckBox { Text = "显示已完成", AutoSize = true, Dock = DockStyle.Fill };
    readonly CheckBox singleLine = new CheckBox { Text = "单行显示", AutoSize = true, Dock = DockStyle.Fill, AccessibleName = "任务单行显示" };
    readonly CheckBox pin = new CheckBox { Text = "置顶", Checked = true, AutoSize = true, Padding = new Padding(0, 6, 0, 0) };
    readonly TableLayoutPanel content = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 1, RowCount = 6, Padding = new Padding(12, 0, 12, 10) };
    readonly Timer timer = new Timer { Interval = 1000 };
    readonly NotifyIcon tray = new NotifyIcon { Text = "TaskTrace · 悬浮事项", Visible = false };
    bool busy, rendering, collapsed, closing, completionPending, projectsDirty = true;
    bool autoSaveEnabled = true;
    int autoSaveSeconds = 30;
    int defaultPriority = 7;
    int page = 1, total, expandedHeight = 560;
    readonly bool selfTest;
    bool allowExit, simpleMode;
    FlowLayoutPanel toolbar;
    TableLayoutPanel addRow, searchRow, projectRow;
    bool fullLayoutRefreshQueued;
    Rectangle fullBounds;
    Size simpleSize = new Size(300, 380);
    readonly Button restoreSimple = new Button { Text = "完整", AutoSize = true, Visible = false };
    readonly Button minimizeSimple = new Button { Text = "最小化", AutoSize = true, Visible = false };
    readonly ToolTip progressTip = new ToolTip { AutoPopDelay = 20000, InitialDelay = 300, ReshowDelay = 200 };
    readonly Timer hoverTimer = new Timer { Interval = 400 };
    TreeNode hoverNode;
    internal sealed class TaskNode : TreeNode { public int CurrentTextLength; public TaskNode(string text) : base(text) {} }
    internal sealed class OutstandingLeaf { public long TaskId; public string Id, Html; public bool Done; public int Priority=9; }
    sealed class PendingItem {
        public string Id, Html; public int Number; public bool Done; public int Priority=9;
        public override string ToString() { return Number + ". [P"+Priority+"] " + OutstandingText(Html) + (Done?"（已完成）":""); }
    }
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
        Size = new Size(400, 560); MinimumSize = new Size(350, 420); TopMost = true; StartPosition = FormStartPosition.Manual;
        var area = Screen.PrimaryScreen.WorkingArea; Location = new Point(area.Right - Width - 24, area.Top + 60);
        LoadBounds(); LoadAutoSaveSettings(); LoadTreePreferences(); status.Click += delegate { ShowErrorDetails(); }; KeyPreview = true;
        toolbar = new FlowLayoutPanel { Dock = DockStyle.Top, AutoSize = true, AutoSizeMode = AutoSizeMode.GrowAndShrink, Padding = new Padding(9, 6, 3, 0), WrapContents = true, FlowDirection = FlowDirection.LeftToRight };
        var full = new Button { Text = "完整界面", AutoSize = true };
        var settingsButton = new Button { Text = "设置", AutoSize = true };
        settingsButton.Click += delegate { ShowAutoSaveSettings(); };
        var simple = new Button { Text = "简洁模式", AutoSize = true };
        simple.Click += delegate { SetSimpleMode(true); };
        foreach(Control control in new Control[] { full, pin, settingsButton, simple }) control.Margin = new Padding(0, 0, 6, 6);
        newTaskButton.BackColor=Blue;newTaskButton.ForeColor=Color.White;newTaskButton.FlatStyle=FlatStyle.Flat;
        newTaskButton.Click+=delegate {ShowNewTaskEditor();};
        toolbar.Controls.AddRange(new Control[] { full, pin, settingsButton, simple });
        tasks.ShowNodeToolTips = false;
        tasks.MouseMove += delegate(object sender, MouseEventArgs e) { var node = tasks.GetNodeAt(e.Location); if(!dragging && node != hoverNode) { hoverTimer.Stop(); progressTip.Hide(tasks); hoverNode = node; if(node != null && node.Tag is long) hoverTimer.Start(); } };
        tasks.MouseLeave += delegate { hoverTimer.Stop(); hoverNode = null; progressTip.Hide(tasks); };
        hoverTimer.Tick += async delegate {
            hoverTimer.Stop(); var node = hoverNode;
            if(node == null || !(node.Tag is long)) return;
            try {
                var history = DailyHistory(await ReadHistory((long)node.Tag));
                var latest = history.FirstOrDefault();
                string text = latest == null ? "暂无每日进展" : DayOf(latest) + "：" + String.Join("\r\n",history.Where(note=>DayOf(note)==DayOf(latest)).OrderBy(note=>Convert.ToInt64(note["id"])).Select(note=>Plain(ProgressDisplayBody((string)note["comment"]))));
                if(text.Length > 1500) text = text.Substring(0, 1500) + "…";
                if(!closing && node == hoverNode && node.TreeView == tasks) progressTip.Show(text, tasks, tasks.PointToClient(Cursor.Position).X + 12, tasks.PointToClient(Cursor.Position).Y + 18, 20000);
            } catch { if(!closing && node == hoverNode) progressTip.Show("进展读取失败，请重新悬停重试。", tasks, 20, 20, 5000); }
        };
        content.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 34));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 0));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 58));
        content.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 34));
        content.RowStyles.Add(new RowStyle(SizeType.Absolute, 32));
        projectRow = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 3, RowCount = 1, Margin = Padding.Empty };
        projectRow.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100));
        projectRow.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 112));
        projectRow.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, 88));
        projectRow.Controls.Add(projects, 0, 0); projectRow.Controls.Add(showCompleted, 1, 0); projectRow.Controls.Add(singleLine, 2, 0);
        content.Controls.Add(projectRow, 0, 0);
        showCompleted.CheckedChanged += async delegate { if(!rendering) { page = 1; SaveBounds(); await Reload(); } };
        singleLine.CheckedChanged += async delegate { ApplyTaskTreeLayout(); if(!rendering) { SaveBounds(); await Reload(); } };
        Hint(entry, "输入事项，按回车新增"); Hint(search, "搜索当前项目");
        InitializePriorityChoice(entryPriority, defaultPriority);
        addRow = TaskCreationRow(entry, entryPriority, "新事项名称", "添加事项", async delegate { await AddTask(); });
        addRow.Visible=false;
        content.Controls.Add(addRow, 0, 1);
        searchRow=Row(search, "查找事项", "搜索", async delegate { page = 1; await Reload(); });
        content.Controls.Add(searchRow, 0, 2);
        tasks.BorderStyle = BorderStyle.FixedSingle; InitializeInteractions(); InitializeSimpleOutstanding(); ApplyTaskTreeLayout();
        content.Controls.Add(tasks, 0, 3);
        var progressButton = new Button { Text = "记录进展", AutoSize = false, Width = 72 };
        progressButton.Click += delegate { ShowProgress(); };
        var childrenButton = new Button { Text = "新任务", AutoSize = false, Width = 66 };
        childrenButton.Click += async delegate { await ShowSubtasks(); };
        fullAddOutstanding.AutoSize=false;fullAddOutstanding.Width=66;
        foreach(Control action in new Control[] {progressButton,newTaskButton,childrenButton,fullAddOutstanding}) {action.Height=27;action.Margin=new Padding(0,3,6,3);}
        fullAddOutstanding.Margin=new Padding(0,3,0,3);
        bottomActions.Controls.AddRange(new Control[] { progressButton, newTaskButton, childrenButton, fullAddOutstanding }); content.Controls.Add(bottomActions, 0, 4);
        content.Controls.Add(status, 0, 5); Controls.Add(content); Controls.Add(toolbar);
        full.Click += async delegate { await OpenFull(); };
        pin.CheckedChanged += delegate { TopMost = pin.Checked; };
        projects.SelectedIndexChanged += async delegate { if (!rendering) { page = 1; SaveBounds(); await Reload(); } };
        entry.KeyDown += async delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; await AddTask(); } };
        search.KeyDown += async delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.Enter) { e.SuppressKeyPress = true; page = 1; await Reload(); } };
        KeyDown += async delegate(object sender, KeyEventArgs e) { if(e.KeyCode == Keys.F5) { e.Handled = true; projectsDirty = true; await Reload(); } };
        tasks.CompletionClicked = delegate(TreeNode node) {
            if(rendering || busy || closing || completionPending || node==null || (!(node.Tag is long) && !(node.Tag is OutstandingLeaf)) || node.TreeView!=tasks)return;
            completionPending=true;
            BeginInvoke(new Action(async delegate {
                try {
                    var leaf=node.Tag as OutstandingLeaf;
                    if(leaf!=null)await CompleteOutstanding(leaf,!leaf.Done);else await Complete(Convert.ToInt64(node.Tag),!node.Checked);
                } finally {completionPending=false;}
            }));
        };
        tasks.NodeDoubleClicked = delegate(TreeNode node) {
            if(node.Tag is OutstandingLeaf) ShowOutstanding(((OutstandingLeaf)node.Tag).TaskId);
            else if(node.Tag is long) { tasks.SelectedNode = node; ShowProgress(); }
        };
        tasks.AfterCollapse += delegate(object sender, TreeViewEventArgs e) {
            if(!rendering && !singleLine.Checked && e.Node.Tag is long && search.Text.Trim().Length == 0) { collapsedTasks.Add(Convert.ToInt64(e.Node.Tag)); SaveTreePreferences(); }
        };
        tasks.AfterExpand += delegate(object sender, TreeViewEventArgs e) {
            if(!rendering && !singleLine.Checked && e.Node.Tag is long && search.Text.Trim().Length == 0) { collapsedTasks.Remove(Convert.ToInt64(e.Node.Tag)); SaveTreePreferences(); }
        };
        tasks.BeforeCollapse += delegate(object sender, TreeViewCancelEventArgs e) { if(singleLine.Checked && !rendering && e.Node.Tag is long)e.Cancel=true; };
        // Both close and minimize keep the application available only in the notification area.
        Resize += delegate { if(!closing && WindowState == FormWindowState.Minimized) HideToTray(); else QueueFullLayoutRefresh(); };
        tray.DoubleClick += delegate { RestoreWindow(); };
        var menu = new ContextMenuStrip();
        menu.Items.Add("显示悬浮窗", null, delegate { RestoreWindow(); });
        menu.Items.Add("完整界面", null, async delegate { await OpenFull(); });
        menu.Items.Add("检查更新", null, async delegate { await CheckForUpdates(true); });
        menu.Items.Add("退出 TaskTrace", null, delegate { allowExit = true; Close(); }); tray.ContextMenuStrip = menu;
        InitializeSimpleModeRecovery(menu);
        InitializeUpdates();
        InitializeAutoRefresh();
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
    static List<DateTime> ProgressDates(List<Dictionary<string,object>> notes) {
        var dates=new List<DateTime>();DateTime date;
        foreach(string value in DailyHistory(notes).Select(note=>DayOf(note)).Distinct())if(DateTime.TryParseExact(value,"yyyy-MM-dd",System.Globalization.CultureInfo.InvariantCulture,System.Globalization.DateTimeStyles.None,out date))dates.Add(date.Date);
        return dates;
    }
    static string ProgressBody(string html,long taskId=0) {
        return ProgressDisplayBody(SplitProgressReferences(html,taskId).Body);
    }
    static SharedList ReadShared(List<Dictionary<string,object>> notes) {
        var list = new SharedList();
        var record = notes.OrderByDescending(note => Convert.ToInt64(note["id"])).FirstOrDefault(note => ((string)note["comment"]).Contains("<h3>"+SharedHeading+"</h3>"));
        if(record != null) {
            list.CommentId = Convert.ToInt64(record["id"]);
            foreach(Match match in Regex.Matches((string)record["comment"], @"<li\b(?<attrs>[^>]*)>(?<body>.*?)</li>", RegexOptions.Singleline|RegexOptions.IgnoreCase)) {
                string attrs=match.Groups["attrs"].Value;
                var id=Regex.Match(attrs,"\\bdata-id\\s*=\\s*\"(?<value>[^\"]+)\"",RegexOptions.IgnoreCase);
                if(!id.Success)continue;
                var done=Regex.Match(attrs,"\\bdata-done\\s*=\\s*\"(?<value>true|false)\"",RegexOptions.IgnoreCase);
                var priority=Regex.Match(attrs,"\\bdata-priority\\s*=\\s*\"(?<value>[0-9])\"",RegexOptions.IgnoreCase);
                int value=9;if(priority.Success)Int32.TryParse(priority.Groups["value"].Value,out value);
                list.Items.Add(new PendingItem {Id=WebUtility.HtmlDecode(id.Groups["value"].Value),Html=match.Groups["body"].Value,Done=done.Success && String.Equals(done.Groups["value"].Value,"true",StringComparison.OrdinalIgnoreCase),Priority=Math.Max(0,Math.Min(9,value))});
            }
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
        string html = "<h3>"+SharedHeading+"</h3><ul>"+String.Join("",list.Items.Select(item => "<li data-id=\""+WebUtility.HtmlEncode(item.Id)+"\" data-done=\""+(item.Done?"true":"false")+"\" data-priority=\""+Math.Max(0,Math.Min(9,item.Priority))+"\">"+item.Html+"</li>"))+"</ul>";
        var saved = await Api(list.CommentId==0 ? "POST" : "PUT", "/tasks/"+id+"/comments"+(list.CommentId==0 ? "" : "/"+list.CommentId), new {comment=html});
        list.CommentId = Convert.ToInt64(saved["id"]);
    }
    TableLayoutPanel TextFieldRow(TextBox input, string description, Control action) {
        var row = new TableLayoutPanel { Dock = DockStyle.Fill, ColumnCount = 2, RowCount = 2 };
        row.ColumnStyles.Add(new ColumnStyle(SizeType.Percent, 100)); row.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute, action.Width>0?action.Width:68));
        row.RowStyles.Add(new RowStyle(SizeType.Absolute, 20));row.RowStyles.Add(new RowStyle(SizeType.Percent, 100));
        var label=new Label {Text=description,Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft,AutoEllipsis=true,AccessibleName=description};
        input.AccessibleDescription=description;
        row.Controls.Add(label,0,0);row.SetColumnSpan(label,2);row.Controls.Add(input,0,1);row.Controls.Add(action,1,1);
        return row;
    }
    TableLayoutPanel Row(TextBox input, string description, string title, EventHandler action) {
        bool primary=title=="新增" || title=="添加事项";
        var button = new Button { Text = title, Dock = DockStyle.Fill, BackColor = primary ? Blue : SystemColors.Control, ForeColor = primary ? Color.White : ForeColor, FlatStyle = FlatStyle.Flat };
        button.Click += action;return TextFieldRow(input,description,button);
    }
    static string PriorityChoiceText(int value) { return value + (value == 0 ? " · 最高" : value == 9 ? " · 最低" : ""); }
    static int ApiPriority(int value) { return 10-Math.Max(0,Math.Min(9,value)); }
    void InitializePriorityChoice(ComboBox choice,int selected) {
        choice.Items.Clear();for(int value=0;value<=9;value++)choice.Items.Add(PriorityChoiceText(value));
        choice.SelectedIndex=Math.Max(0,Math.Min(9,selected));
    }
    TableLayoutPanel TaskCreationRow(TextBox input,ComboBox priority,string description,string title,EventHandler action) {
        var button=new Button {Text=title,Dock=DockStyle.Fill,BackColor=Blue,ForeColor=Color.White,FlatStyle=FlatStyle.Flat};button.Click+=action;
        return TaskCreationRow(input,priority,description,button);
    }
    TableLayoutPanel TaskCreationRow(TextBox input,ComboBox priority,string description,Control action) {
        var row=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=3,RowCount=2};
        row.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));row.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,82));row.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,96));
        row.RowStyles.Add(new RowStyle(SizeType.Absolute,20));row.RowStyles.Add(new RowStyle(SizeType.Percent,100));
        var inputLabel=new Label {Text=description,Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft,AutoEllipsis=true,AccessibleName=description};
        var priorityLabel=new Label {Text="优先级",Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft,AccessibleName="优先级说明"};
        action.Dock=DockStyle.Fill;
        input.AccessibleDescription=description;priority.AccessibleDescription="0 最高，9 最低";
        row.Controls.Add(inputLabel,0,0);row.Controls.Add(priorityLabel,1,0);row.Controls.Add(input,0,1);row.Controls.Add(priority,1,1);row.Controls.Add(action,2,1);
        return row;
    }
    void ShowNewTaskEditor() {
        if(simpleMode || closing || IsDisposed)return;
        if(collapsed)ToggleFold();
        addRow.Visible=true;content.RowStyles[1].Height=58;RefreshFullLayout();
        if(IsHandleCreated)BeginInvoke(new Action(delegate {if(!closing && addRow.Visible)entry.Focus();}));
    }
    void HideNewTaskEditor() {
        if(addRow==null)return;
        entry.Clear();entryPriority.SelectedIndex=defaultPriority;addRow.Visible=false;content.RowStyles[1].Height=0;RefreshFullLayout();
    }
    void ApplyTaskTreeLayout() {
        bool flat=singleLine.Checked;
        tasks.SingleLinePaths=flat;
        tasks.ShowLines=!flat;
        tasks.ShowRootLines=true;
        tasks.ShowPlusMinus=!flat;
        tasks.Indent=flat?0:20;
        if(flat)tasks.ExpandAll();
        tasks.SyncCompletionStates();
        tasks.Invalidate();
    }
    void QueueFullLayoutRefresh() {
        if(simpleMode || closing || IsDisposed || fullLayoutRefreshQueued || !IsHandleCreated || WindowState!=FormWindowState.Normal)return;
        fullLayoutRefreshQueued=true;
        BeginInvoke(new Action(delegate {fullLayoutRefreshQueued=false;RefreshFullLayout();}));
    }
    void RefreshFullLayout() {
        if(simpleMode || closing || IsDisposed || toolbar==null || content==null || WindowState!=FormWindowState.Normal)return;
        toolbar.PerformLayout();content.PerformLayout();
        if(addRow!=null && addRow.Visible)addRow.PerformLayout();
        if(searchRow!=null)searchRow.PerformLayout();
        content.Invalidate(true);
    }
    void TestResponsiveFullLayout() {
        var original=ClientSize;bool editorWasVisible=addRow.Visible;
        try {
            int narrowWidth=Math.Max(334,MinimumSize.Width-16),wideWidth=Math.Max(640,original.Width),height=Math.Max(404,original.Height);
            ShowNewTaskEditor();
            ClientSize=new Size(narrowWidth,height);RefreshFullLayout();
            int narrowEntry=entry.Width,narrowSearch=search.Width,narrowProject=projects.Width;
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-new-task-compact-test.png"));}
            ClientSize=new Size(wideWidth,height);RefreshFullLayout();
            if(entry.Width<=narrowEntry || search.Width<=narrowSearch || projects.Width<=narrowProject)throw new Exception("Floating fields did not expand after widening the window");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-new-task-wide-test.png"));}
            ClientSize=new Size(narrowWidth,height);RefreshFullLayout();
            if(Math.Abs(entry.Width-narrowEntry)>2 || Math.Abs(search.Width-narrowSearch)>2 || Math.Abs(projects.Width-narrowProject)>2)throw new Exception("Floating fields did not shrink back after widening the window");
            HideNewTaskEditor();RefreshFullLayout();
            int verticalGap=searchRow.Top-projectRow.Bottom;bottomActions.PerformLayout();
            if(verticalGap>8 || projectRow.Height>34 || content.GetColumnWidths()[0]>content.ClientSize.Width-content.Padding.Horizontal+2)throw new Exception("Project and search layout retained excess space after resizing");
            if(bottomActions.Controls.Cast<Control>().Any(action=>!action.Visible || action.Top!=3 || action.Right+action.Margin.Right>bottomActions.ClientSize.Width))throw new Exception("Bottom actions wrapped or were clipped at the minimum window width");
            using(var bitmap=new Bitmap(Width,Height)){DrawToBitmap(bitmap,new Rectangle(Point.Empty,Size));bitmap.Save(Path.Combine(data,"floating-full-resize-return-test.png"));}
            ClientSize=new Size(wideWidth,height);RefreshFullLayout();
            ClientSize=new Size(narrowWidth,height);RefreshFullLayout();
            if(Math.Abs(search.Width-narrowSearch)>2 || Math.Abs(projects.Width-narrowProject)>2)throw new Exception("Repeated wide-to-narrow resizing left stale field widths");
        } finally {
            ClientSize=original;
            if(editorWasVisible)ShowNewTaskEditor();else HideNewTaskEditor();
            RefreshFullLayout();
        }
    }
    Dictionary<string, object> ReadObject(string value) { return json.Deserialize<Dictionary<string, object>>(value); }
    async Task<Dictionary<string, object>> Api(string method, string path, object body, bool retry = true) {
        using(var request = new HttpRequestMessage(new HttpMethod(method), url + "/api/v2" + path)) {
            string requestToken = token;
            request.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", requestToken);
            bool recordUndo = undoRecording && method != "GET" && path != "/tasktrace/undo" && path != "/tasktrace/team/sync";
            if(recordUndo) { request.Headers.Add("X-TaskTrace-Undo", "1"); if(undoGroup != null) request.Headers.Add("X-TaskTrace-Undo-Group", undoGroup); }
            if(body != null) request.Content = new StringContent(json.Serialize(body), Encoding.UTF8, method == "PATCH" ? "application/merge-patch+json" : "application/json");
            using(var response = await http.SendAsync(request)) {
                if(response.StatusCode == HttpStatusCode.Unauthorized && retry) {
                    await refreshGate.WaitAsync();
                    try {
                        if(token == requestToken) using(var renewed = await http.PostAsync(url + "/api/v2/oauth/token", new StringContent(json.Serialize(new { grant_type = "refresh_token", refresh_token = refresh }), Encoding.UTF8, "application/json"))) {
                            if(!renewed.IsSuccessStatusCode) throw new Exception("会话已失效，请退出并重新启动悬浮窗。");
                            var pair = ReadObject(await renewed.Content.ReadAsStringAsync()); token = (string)pair["access_token"]; refresh = (string)pair["refresh_token"];
                        }
                    } finally { refreshGate.Release(); }
                    return await Api(method, path, body, false);
                }
                string responseBody = await response.Content.ReadAsStringAsync();
                if(!response.IsSuccessStatusCode) {
                    string message = "操作未保存（" + (int)response.StatusCode + "），请刷新后重试。";
                    if(path == "/tasktrace/undo") try { var problem = ReadObject(responseBody); object detail; if(problem.TryGetValue("detail", out detail) || problem.TryGetValue("message", out detail)) message = Convert.ToString(detail); } catch { }
                    throw new Exception(message);
                }
                var result = ReadObject(responseBody);
                if(recordUndo) await RefreshUndo();
                return result;
            }
        }
    }
    void SetBusy(bool value) { busy = value; if(!closing) { content.Enabled = !value; tasks.Enabled = !value; toolbar.Enabled = !value; UpdateUndoControls(); UpdateSimpleModeState(); } }
    async Task Reload() {
        if(busy || closing) return; SetBusy(true);
        try { status.ForeColor = ForeColor; status.Text = "正在同步…"; await LoadTasks(); await RefreshUndo(); }
        catch(Exception e) { Error(e); }
        finally { SetBusy(false); }
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
        bool created=false;SetBusy(true);
        try { await Api("POST", "/projects/" + project.Id + "/tasks", new { title = entry.Text.Trim(), priority = ApiPriority(entryPriority.SelectedIndex) }); search.Clear(); page = 1; await LoadTasks(); HideNewTaskEditor(); created=true; }
        catch(Exception e) { Error(e); } finally { SetBusy(false); if(addRow.Visible)entry.Focus();else if(created)tasks.Focus(); }
    }
    sealed class PastedImage { public byte[] Bytes; public long Id; }
    async Task<long> SaveProgress(long id, DateTime day, string progress, string nextStep, List<PastedImage> pictures = null, long commentId = 0, string bodyOverride = null, List<long> mergedIds = null, List<ProgressReference> references = null) {
        if(references==null && commentId>0) {
            var existing=(await ReadHistory(id)).FirstOrDefault(row=>Convert.ToInt64(row["id"])==commentId);
            if(existing!=null)references=SplitProgressReferences((string)existing["comment"],id).References;
        }
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
        note += SerializeProgressReferences(references);
        var resultNote = await Api(commentId == 0 ? "POST" : "PUT", "/tasks/" + id + "/comments" + (commentId == 0 ? "" : "/" + commentId), new { comment = note });
        return Convert.ToInt64(resultNote["id"]);
    }
    sealed class ProgressDraft { public string Text; public List<PastedImage> Pictures; public List<ProgressReference> References; }
    async void ShowProgress() {
        if(busy || closing) return;
        if(tasks.SelectedNode == null || !(tasks.SelectedNode.Tag is long)) { status.Text="请先选中事项。";return; }
        await EditProgress((long)tasks.SelectedNode.Tag,tasks.SelectedNode.Text);
    }
    async Task EditProgress(long id,string taskTitle,bool verify=false) {
        SetBusy(true); timer.Stop();
        try {
            var history = await ReadHistory(id);
            using(var dialog=new Form {Text="每日进展 · "+taskTitle,Size=new Size(560,700),MinimumSize=new Size(420,580),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent,ShowInTaskbar=false}) {
                var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=1,RowCount=7};
                layout.RowStyles.Add(new RowStyle(SizeType.Absolute,32));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,40));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,52));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,36));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,36));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,52));
                var day=new ProgressDatePicker {Value=DateTime.Today,Dock=DockStyle.Fill};day.SetMarkedDates(ProgressDates(history));
                var progress=new TextBox {Multiline=true,AcceptsReturn=true,ScrollBars=ScrollBars.Vertical,Dock=DockStyle.Fill,AccessibleName="当天进展与更正",AccessibleDescription="当天进展与更正"};
                var sharedButton=new Button {Text="遗留事项 · 所有日期共享",Dock=DockStyle.Fill};
                sharedButton.Click+=delegate {ShowOutstanding(id,true);};
                var save=new Button {Text="保存当天进展 (Ctrl+Enter)",Dock=DockStyle.Fill};
                var feedback=new Label {Dock=DockStyle.Fill};
                var referenceGroup=new GroupBox {Text="引用历史进展",Dock=DockStyle.Fill,Padding=new Padding(8)};
                var referenceLayout=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=1,RowCount=3};
                referenceLayout.RowStyles.Add(new RowStyle(SizeType.Absolute,32));referenceLayout.RowStyles.Add(new RowStyle(SizeType.Percent,100));referenceLayout.RowStyles.Add(new RowStyle(SizeType.Absolute,34));
                var choiceRow=new TableLayoutPanel {Dock=DockStyle.Fill,ColumnCount=2,RowCount=1};choiceRow.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));choiceRow.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,170));
                var chooseReferences=new Button {Text="引用历史进展",Dock=DockStyle.Fill,AccessibleName="打开历史进展多选表格"};
                var toggleReferences=new LinkLabel {Text="",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleRight,Visible=false,AccessibleName="展开或收起已引用的历史进展"};choiceRow.Controls.Add(chooseReferences);choiceRow.Controls.Add(toggleReferences);
                var referenceList=new ListBox {Dock=DockStyle.Fill,HorizontalScrollbar=true,IntegralHeight=false,AccessibleName="已引用的历史进展"};
                var referenceActions=new FlowLayoutPanel {Dock=DockStyle.Fill,WrapContents=false};
                var previewReference=new Button {Text="查看快照",AutoSize=true,Enabled=false};var removeReference=new Button {Text="移除引用",AutoSize=true,Enabled=false};
                referenceActions.Controls.Add(previewReference);referenceActions.Controls.Add(removeReference);referenceLayout.Controls.Add(choiceRow);referenceLayout.Controls.Add(referenceList);referenceLayout.Controls.Add(referenceActions);referenceGroup.Controls.Add(referenceLayout);
                layout.Controls.Add(day);layout.Controls.Add(new Label {Text="当天进展与更正 · 可填写当日进展，引用旧记录不会改写原记录。",Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft,AccessibleName="当天进展与更正说明"});layout.Controls.Add(progress);layout.Controls.Add(referenceGroup);layout.Controls.Add(sharedButton);layout.Controls.Add(save);layout.Controls.Add(feedback);dialog.Controls.Add(layout);
                var drafts=new Dictionary<string,ProgressDraft>();var pictures=new List<PastedImage>();var references=new List<ProgressReference>();
                string selectedDay="",originalBody="",originalText="",lastSaved="";long commentId=0;var mergedIds=new List<long>();bool submitting=false,referenceExpanded=false;
                Func<string> snapshot=delegate {return json.Serialize(new {date=selectedDay,text=progress.Text,images=pictures.Count,references=SerializeProgressReferences(references)});};
                Action stash=delegate {if(selectedDay=="")return;if(snapshot()!=lastSaved)drafts[selectedDay]=new ProgressDraft {Text=progress.Text,Pictures=new List<PastedImage>(pictures),References=references.Select(item=>item.Copy()).ToList()};else drafts.Remove(selectedDay);};
                Action refreshReferences=delegate {
                    var selected=referenceList.SelectedItem as ProgressReference;referenceList.BeginUpdate();referenceList.Items.Clear();foreach(var reference in references)referenceList.Items.Add(reference);referenceList.EndUpdate();
                    if(selected!=null)referenceList.SelectedItem=references.FirstOrDefault(item=>item.Id==selected.Id);if(referenceList.SelectedIndex<0 && references.Count>0)referenceList.SelectedIndex=0;
                    int choices=DailyHistory(history).Select(note=>DayOf(note)).Distinct().Count(date=>String.CompareOrdinal(date,selectedDay)<0 && !references.Any(item=>item.Date==date));chooseReferences.Enabled=choices>0;previewReference.Enabled=removeReference.Enabled=referenceList.SelectedIndex>=0;
                    toggleReferences.Visible=references.Count>0;toggleReferences.Text=referenceExpanded?"收起引用的历史进展":"展开引用的历史进展（"+references.Count+"）";
                    referenceList.Visible=referenceActions.Visible=referenceExpanded && references.Count>0;layout.RowStyles[3].Height=referenceList.Visible?180:52;
                    referenceGroup.Text="引用历史进展";
                };
                Action loadDay=delegate {
                    selectedDay=day.Value.ToString("yyyy-MM-dd");
                    var records=DailyHistory(history).Where(note=>DayOf(note)==selectedDay).OrderBy(note=>Convert.ToInt64(note["id"])).ToList();
                    commentId=records.Count==0?0:records.Max(note=>Convert.ToInt64(note["id"]));
                    mergedIds=records.Select(note=>Convert.ToInt64(note["id"])).Concat(records.SelectMany(note=>MergedIds((string)note["comment"]))).Where(value=>value!=commentId).Distinct().ToList();
                    originalBody=String.Join("",records.Select(note=>ProgressBody((string)note["comment"],id)));originalText=Plain(Regex.Replace(originalBody,"<img[^>]*>","",RegexOptions.IgnoreCase));
                    references=records.SelectMany(note=>SplitProgressReferences((string)note["comment"],id).References).GroupBy(item=>item.Id).Select(group=>group.First().Copy()).ToList();
                    progress.Text=originalText;pictures=new List<PastedImage>();lastSaved=snapshot();
                    if(drafts.ContainsKey(selectedDay)){progress.Text=drafts[selectedDay].Text;pictures=new List<PastedImage>(drafts[selectedDay].Pictures);references=drafts[selectedDay].References.Select(item=>item.Copy()).ToList();}
                    refreshReferences();feedback.Text=records.Count==0?"此日期尚无进展。支持 Ctrl+V 粘贴图片。":"已载入当天合并内容；正文、原图片和引用分别保留。";
                };
                loadDay();day.ValueChanged+=delegate {if(!submitting){stash();loadDay();}};
                toggleReferences.LinkClicked+=delegate{referenceExpanded=!referenceExpanded;refreshReferences();};
                referenceList.SelectedIndexChanged+=delegate{previewReference.Enabled=removeReference.Enabled=referenceList.SelectedIndex>=0;};
                Func<bool,Task> selectReferences=async delegate(bool verifyPicker) {
                    if(submitting)return;string targetDay=selectedDay;
                    submitting=true;day.Enabled=false;save.Enabled=false;sharedButton.Enabled=false;referenceGroup.Enabled=false;feedback.Text="正在读取所选日期的最新进展…";
                    try {
                        var latest=await ReadHistory(id);
                        if(dialog.IsDisposed || selectedDay!=targetDay)return;
                        history=latest;var selected=SelectProgressReferences(id,targetDay,history,references,dialog,verifyPicker);int added=0;
                        foreach(string sourceDay in selected)if(!references.Any(item=>item.Date==sourceDay)){references.Add(ReferenceForDay(id,sourceDay,targetDay,history));added++;}
                        if(added>0){referenceExpanded=true;refreshReferences();referenceList.SelectedIndex=references.Count-1;feedback.Text="已添加 "+added+" 条引用。请在上方填写当天更正，保存后原记录不变。";}else {refreshReferences();feedback.Text="未添加引用。";}
                    } catch(Exception e){if(!dialog.IsDisposed)feedback.Text=e.Message;}
                    finally{if(!dialog.IsDisposed){submitting=false;day.Enabled=true;save.Enabled=true;sharedButton.Enabled=true;referenceGroup.Enabled=true;}}
                };
                chooseReferences.Click+=async delegate {await selectReferences(false);};
                removeReference.Click+=delegate{if(submitting || referenceList.SelectedIndex<0)return;references.RemoveAt(referenceList.SelectedIndex);refreshReferences();feedback.Text="引用已移除，保存后生效；原记录不变。";};
                previewReference.Click+=async delegate {var item=referenceList.SelectedItem as ProgressReference;if(item!=null)await ShowReferenceSnapshot(item,dialog);};
                Func<bool,Task> write=async delegate(bool finish) {
                    if(submitting || (String.IsNullOrWhiteSpace(progress.Text) && pictures.Count==0 && references.Count==0 && commentId==0))return;
                    if(snapshot()==lastSaved && mergedIds.Count==0){if(finish)feedback.Text="没有需要保存的修改。";return;}
                    submitting=true;day.Enabled=false;save.Enabled=false;sharedButton.Enabled=false;referenceGroup.Enabled=false;progress.ReadOnly=true;
                    try {
                        using(BeginUndoGroup()) {
                        var sharedBeforeSave=ReadShared(await ReadHistory(id));if(sharedBeforeSave.CommentId==0)await WriteShared(id,sharedBeforeSave);
                        string images=String.Join("",Regex.Matches(originalBody,"<img[^>]*>",RegexOptions.IgnoreCase).Cast<Match>().Select(match=>match.Value));
                        string body=progress.Text==originalText?originalBody:ProgressTextHtml(progress.Text)+images;
                        commentId=await SaveProgress(id,day.Value,progress.Text,"",pictures,commentId,body,mergedIds,references);
                        foreach(var picture in pictures) body+="<p><img src=\"/api/v1/tasks/"+id+"/attachments/"+picture.Id+"\"></p>";
                        originalBody=body;originalText=progress.Text;pictures.Clear();lastSaved=snapshot();drafts.Remove(selectedDay);
                        history=await ReadHistory(id);day.SetMarkedDates(ProgressDates(history));refreshReferences();feedback.Text=finish?"当天进展已保存，可以继续编辑。":"当天进展已自动保存。";
                        }
                    } catch {feedback.Text="保存失败，内容已保留，请重试。";}
                    finally{submitting=false;day.Enabled=true;save.Enabled=true;sharedButton.Enabled=true;referenceGroup.Enabled=true;progress.ReadOnly=false;}
                };
                save.Click+=async delegate {await write(true);};
                var autoTimer=new Timer {Interval=autoSaveSeconds*1000};autoTimer.Tick+=async delegate {if(autoSaveEnabled && !editingOutstanding && snapshot()!=lastSaved)await write(false);};autoTimer.Start();
                dialog.KeyPreview=true;dialog.KeyDown+=delegate(object sender,KeyEventArgs e){
                    if(e.Control && e.KeyCode==Keys.V && !submitting && progress.ContainsFocus && Clipboard.ContainsImage()) {e.SuppressKeyPress=true;try{using(var image=Clipboard.GetImage())using(var stream=new MemoryStream()){image.Save(stream,System.Drawing.Imaging.ImageFormat.Png);pictures.Add(new PastedImage {Bytes=stream.ToArray()});}feedback.Text="已粘贴 "+pictures.Count+" 张图片。";}catch{feedback.Text="剪贴板读取失败，请重试。";}}
                    if(e.Control && e.KeyCode==Keys.Enter){e.SuppressKeyPress=true;save.PerformClick();}
                };
                Exception verificationError=null;
                if(verify)dialog.Shown+=async delegate {
                    try {
                        autoTimer.Stop();
                        if(day.VisibleMarkedDatesForTest()<3)throw new Exception("Progress calendar did not mark existing dates");
                        using(var calendar=day.RenderCalendarForTest())calendar.Save(Path.Combine(data,"floating-progress-calendar-test.png"));
                        DateTime beforeOutside=day.Value;day.OpenCalendarForTest();if(!day.CalendarVisibleForTest())throw new Exception("Progress calendar did not open modelessly");day.SimulateCalendarOutsideClickForTest();if(day.CalendarVisibleForTest() || day.Value!=beforeOutside)throw new Exception("Clicking outside progress calendar did not cancel selection");
                        var choices=DailyHistory(history).Select(note=>DayOf(note)).Distinct().Where(date=>String.CompareOrdinal(date,selectedDay)<0 && !references.Any(item=>item.Date==date)).OrderByDescending(date=>date).ToList();
                        if(choices.Count<2 || choices.Any(date=>String.CompareOrdinal(date,selectedDay)>=0))throw new Exception("Reference table choices are not earlier than selected day");
                        string first=choices[0],second=choices[1];
                        var sourceRecord=DailyHistory(history).First(note=>DayOf(note)==first);string sourceOriginal=(string)sourceRecord["comment"];
                        await Api("PUT","/tasks/"+id+"/comments/"+sourceRecord["id"],new {comment=sourceOriginal+"<p>引用时的最新内容</p>"});
                        await selectReferences(true);if(references.Count!=2 || !references.Any(item=>item.Date==first && item.Html.Contains("引用时的最新内容")) || !references.Any(item=>item.Date==second))throw new Exception("Multi-select reference table did not use latest source history");
                        await Api("PUT","/tasks/"+id+"/comments/"+sourceRecord["id"],new {comment=sourceOriginal});
                        if(!references.Any(item=>item.Date==first && item.Html.Contains("引用时的最新内容")))throw new Exception("Reference snapshot changed when its source changed");
                        if(DailyHistory(history).Select(note=>DayOf(note)).Distinct().Where(date=>String.CompareOrdinal(date,selectedDay)<0 && !references.Any(item=>item.Date==date)).Contains(first))throw new Exception("Duplicate reference prevention failed");
                        progress.Text="本日更正：以新结论为准";string draftSnapshot=snapshot();day.Value=DateTime.Today.AddDays(1);if(references.Count!=0)throw new Exception("Reference draft leaked to other date");day.Value=DateTime.Today;if(snapshot()!=draftSnapshot)throw new Exception("Reference date draft did not restore");
                        await write(false);if(snapshot()!=lastSaved)throw new Exception("Reference automatic save failed: "+feedback.Text);
                        var saved=SplitProgressReferences((string)(await ReadHistory(id)).First(note=>Convert.ToInt64(note["id"])==commentId)["comment"],id);if(saved.References.Count!=2 || Plain(ProgressBody(saved.Body,id))!=progress.Text)throw new Exception("Reference save mixed snapshot text into body");
                        var citationHistory=await ReadHistory(id);var citations=ProgressCitationsForDay(id,first,citationHistory);if(!citations.Any(item=>item.Date==selectedDay && Plain(item.Html).Contains("本日更正")))throw new Exception("Referenced source did not expose citing progress");
                        day.Value=DateTime.Today.AddDays(-1);day.Value=DateTime.Today;if(references.Count!=2 || progress.Text!="本日更正：以新结论为准")throw new Exception("Saved references failed date roundtrip");
                        progress.Text="本日更正：再次确认";await write(true);if(references.Count!=2)throw new Exception("Ordinary edit lost references");
                        using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-progress-references-test.png"));}
                        dialog.Size=dialog.MinimumSize;dialog.PerformLayout();if(progress.Height<60 || referenceList.Height<35 || referenceActions.Bottom>referenceLayout.ClientSize.Height+2)throw new Exception("Reference controls overflow minimum dialog size");
                        using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-progress-references-small-test.png"));}
                        await ShowReferenceSnapshot(references[0],dialog,true);
                        referenceList.SelectedIndex=0;removeReference.PerformClick();await write(false);var retained=SplitProgressReferences((string)(await ReadHistory(id)).First(note=>Convert.ToInt64(note["id"])==commentId)["comment"],id);if(retained.References.Count!=1)throw new Exception("Removed reference was not auto saved");
                    } catch(Exception e){verificationError=e;}finally{drafts.Clear();dialog.Close();}
                };
                dialog.FormClosing+=delegate(object sender,FormClosingEventArgs e){if(submitting){e.Cancel=true;return;}if(verify)return;stash();if(drafts.Count>0 && MessageBox.Show(dialog,"还有日期的进展未保存，确定放弃这些修改？","每日进展",MessageBoxButtons.YesNo)!=DialogResult.Yes)e.Cancel=true;};
                try{dialog.ShowDialog(this);if(verificationError!=null)throw verificationError;}finally{autoTimer.Stop();autoTimer.Dispose();}
            }
        }catch(Exception e){if(verify)throw;Error(e);}finally{SetBusy(false);timer.Start();}
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
    async Task<long> CreateSubtask(long parentId, long projectId, string title, long existingId = 0, int priority = -1) {
        if(await TaskHierarchySpan(parentId) >= 5) throw new Exception(TaskDepthMessage);
        using(BeginUndoGroup()) {
        long childId = existingId;
        if(childId == 0) { int selectedPriority=priority<0?defaultPriority:priority;var child = await Api("POST", "/projects/" + projectId + "/tasks", new { title = title, priority = ApiPriority(selectedPriority) }); childId = Convert.ToInt64(child["id"]); }
        await Api("POST", "/tasks/" + parentId + "/relations", new { other_task_id = childId, relation_kind = "subtask" });
        return childId;
        }
    }
    async Task ShowSubtasks(bool verify=false) {
        if(busy || closing) return;
        if(tasks.SelectedNode == null || !(tasks.SelectedNode.Tag is long)) { status.Text = "请先选中一个父事项。"; return; }
        long parentId = Convert.ToInt64(tasks.SelectedNode.Tag);
        SetBusy(true); timer.Stop();
        try {
            var parent = await Api("GET", "/tasks/" + parentId, null);
            long projectId = Convert.ToInt64(parent["project_id"]);
            using(var dialog = new Form { Text = "子任务 · " + (string)parent["title"], Size = new Size(450, 470), MinimumSize = new Size(380, 360), Font = Font, TopMost = TopMost, StartPosition = FormStartPosition.CenterParent, ShowInTaskbar = false }) {
                var list = new ListView { Dock = DockStyle.Fill, View = View.Details, CheckBoxes = true, FullRowSelect = true, HeaderStyle = ColumnHeaderStyle.None };
                list.Columns.Add("子任务", 390);
                var title = new TextBox { Dock = DockStyle.Fill, MaxLength = 250, AccessibleName = "新子任务名称" };
                var priority = new ComboBox { Dock = DockStyle.Fill, DropDownStyle = ComboBoxStyle.DropDownList, AccessibleName = "新子任务优先级" };InitializePriorityChoice(priority,defaultPriority);
                var feedback = new Label { Dock = DockStyle.Bottom, Height = 46, Text = "子任务独立完成；在主列表中选择它可记录每日进展。" };
                var add = new Button { Text = "添加子任务", Dock = DockStyle.Fill, Width = 95 };
                var row = TaskCreationRow(title,priority,"新子任务名称",add);row.Dock=DockStyle.Top;row.Height=58;
                var renameTitle = new TextBox { Dock = DockStyle.Fill, MaxLength = 250, AccessibleName = "修改子任务名称" };
                var rename = new Button { Text = "保存名称", Dock = DockStyle.Fill, Enabled = false, Width = 95 };
                var renameRow = TextFieldRow(renameTitle,"修改选中子任务名称",rename);renameRow.Dock=DockStyle.Top;renameRow.Height=58;
                dialog.Controls.Add(list); dialog.Controls.Add(renameRow); dialog.Controls.Add(row); dialog.Controls.Add(feedback);
                bool loading = false, writing = false; long pendingId = 0; int pendingPriority=-1; string pendingUndoGroup = null; bool depthLimit = false;
                Func<Task> reload = async delegate {
                    loading = true;
                    try {
                        depthLimit = await TaskHierarchySpan(parentId) >= 5;
                        add.Enabled = !depthLimit; title.Enabled = !depthLimit && pendingId == 0;priority.Enabled=!depthLimit && pendingId==0;
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
                    writing = true; add.Enabled = false; title.Enabled = false;priority.Enabled=false;
                    try {
                        if(await TaskHierarchySpan(parentId) >= 5) { depthLimit = true; throw new Exception(TaskDepthMessage); }
                        if(pendingUndoGroup == null) pendingUndoGroup = Guid.NewGuid().ToString();
                        using(BeginUndoGroup(pendingUndoGroup)) {
                        if(pendingId == 0) { pendingPriority=priority.SelectedIndex;var child = await Api("POST", "/projects/" + projectId + "/tasks", new { title = title.Text.Trim(), priority = ApiPriority(pendingPriority) }); pendingId = Convert.ToInt64(child["id"]); }
                        await CreateSubtask(parentId, projectId, title.Text, pendingId,pendingPriority);
                        }
                        pendingId = 0;pendingPriority=-1; pendingUndoGroup = null; title.Clear();priority.SelectedIndex=defaultPriority; feedback.Text = "子任务已添加。可独立勾选完成。"; add.Text = "添加子任务"; await reload();
                    } catch { feedback.Text = depthLimit ? TaskDepthMessage : pendingId == 0 ? "创建失败，请重试。" : "事项已创建，关联失败；点击重试，不会重复创建。"; add.Text = pendingId == 0 ? "添加子任务" : "重试关联"; }
                    finally { writing = false; add.Enabled = !depthLimit; title.Enabled = !depthLimit && pendingId == 0;priority.Enabled=!depthLimit && pendingId==0; }
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
                Exception verificationError=null;
                if(verify)dialog.Shown+=delegate {
                    try {
                        if(priority.SelectedIndex!=defaultPriority || priority.AccessibleName!="新子任务优先级" || !row.Controls.OfType<Label>().Any(value=>value.Text=="优先级"))throw new Exception("Subtask priority selector missing or has the wrong default");
                        using(var bitmap=new Bitmap(dialog.Width,dialog.Height)){dialog.DrawToBitmap(bitmap,new Rectangle(Point.Empty,dialog.Size));bitmap.Save(Path.Combine(data,"floating-new-subtask-priority-test.png"));}
                    }catch(Exception e){verificationError=e;}finally{dialog.Close();}
                };
                await reload(); dialog.ShowDialog(this);if(verificationError!=null)throw verificationError;
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
    async Task CompleteOutstanding(OutstandingLeaf leaf,bool done) {
        if(busy || closing || leaf==null)return;SetBusy(true);timer.Stop();
        try {await UpdateOutstandingState(leaf.TaskId,leaf.Id,done,null);await LoadTasks();}
        catch(Exception e){Error(e);}finally{SetBusy(false);timer.Start();}
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
            var layout=new TableLayoutPanel {Dock=DockStyle.Fill,Padding=new Padding(12),ColumnCount=1,RowCount=2};
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,28));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
            layout.Controls.Add(new Label {Text="错误详情（可选择并复制）",Dock=DockStyle.Fill,TextAlign=ContentAlignment.BottomLeft});
            layout.Controls.Add(new TextBox { Multiline = true, ReadOnly = true, ScrollBars = ScrollBars.Both, WordWrap = false, Dock = DockStyle.Fill, Text = lastError, AccessibleName="错误详情", AccessibleDescription="错误详情（可选择并复制）" });
            dialog.Controls.Add(layout);
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
        status.Text = "操作失败，点击此处查看完整错误详情。"; UpdateSimpleModeState();
    }
    void HideToTray() {
        SaveSimpleMode(); SaveBounds(); hoverTimer.Stop(); progressTip.Hide(tasks);
        restoreSimple.Visible = false; Hide();
    }
    void RestoreWindow() { Show(); WindowState = FormWindowState.Normal; Activate(); }
    bool fittingFoldedToolbar;
    void FitFoldedToolbar() {
        if(!collapsed || simpleMode || simpleLayout || fittingFoldedToolbar || closing || toolbar==null || WindowState!=FormWindowState.Normal)return;
        fittingFoldedToolbar=true;
        try {
            int height=SizeFromClientSize(new Size(ClientSize.Width,toolbar.Height)).Height;
            MinimumSize=new Size(350,height);
            if(Height!=height)Height=height;
        }finally{fittingFoldedToolbar=false;}
    }
    void ToggleFold() {
        if(!collapsed) {
            expandedHeight=Height;collapsed=true;content.Visible=false;
            toolbar.PerformLayout();FitFoldedToolbar();
        }else {
            collapsed=false;content.Visible=true;MinimumSize=new Size(350,420);Height=expandedHeight;
        }
    }
    void LoadAutoSaveSettings() {
        try {
            var settings = ReadObject(File.ReadAllText(Path.Combine(data, "autosave.json")));
            autoSaveEnabled = Convert.ToBoolean(settings["enabled"]);
            autoSaveSeconds = Math.Max(5, Math.Min(3600, Convert.ToInt32(settings["seconds"])));
            if(settings.ContainsKey("default_priority"))defaultPriority=Math.Max(0,Math.Min(9,Convert.ToInt32(settings["default_priority"])));
        } catch { }
    }
    void ShowAutoSaveSettings(bool verify=false) {
        using(var settings = new Form { Text = "设置", ClientSize = new Size(470, 510), FormBorderStyle = FormBorderStyle.FixedDialog, MaximizeBox = false, MinimizeBox = false, StartPosition = FormStartPosition.CenterParent, Font = Font, TopMost = TopMost, ShowInTaskbar = false }) {
            var enabled = new CheckBox { Text = "启用每日进展自动保存", Checked = autoSaveEnabled, Location = new Point(18, 18), AutoSize = true };
            var label = new Label { Text = "检查间隔（秒）", Location = new Point(18, 55), AutoSize = true };
            var seconds = new NumericUpDown { Minimum = 5, Maximum = 3600, Value = autoSaveSeconds, Location = new Point(155, 52), Width = 100 };
            var help = new Label { Text = "仅内容变化时保存；编辑期间更新同一条进展。\n设置保存在本机目录，网页设置需在网页中配置。", Location = new Point(18, 92), Size = new Size(430, 48) };
            var priorityLabel = new Label { Text = "默认新增任务优先级（0 最高，9 最低）", Location = new Point(18, 150), AutoSize = true };
            var priority = new NumericUpDown { Minimum = 0, Maximum = 9, Value = defaultPriority, Location = new Point(320, 147), Width = 55, AccessibleName = "默认新增任务优先级" };
            var visibleLabel = new Label { Text = "悬浮窗展示哪些优先级（可多选）", Location = new Point(18, 190), AutoSize = true };
            var visibleChoices = new CheckedListBox { Location = new Point(18, 216), Size = new Size(286, 96), MultiColumn = true, ColumnWidth = 92, CheckOnClick = true, IntegralHeight = false, AccessibleName = "悬浮窗展示优先级" };
            for(int number=0;number<=9;number++)visibleChoices.Items.Add("P"+number+(number==0?"（最高）":number==9?"（最低）":""),visiblePriorities.Contains(number));
            var selectAll = new Button { Text = "全选", Location = new Point(320, 216), Size = new Size(110, 30) };
            var selectNone = new Button { Text = "全不选", Location = new Point(320, 254), Size = new Size(110, 30) };
            selectAll.Click += delegate {for(int index=0;index<visibleChoices.Items.Count;index++)visibleChoices.SetItemChecked(index,true);};
            selectNone.Click += delegate {for(int index=0;index<visibleChoices.Items.Count;index++)visibleChoices.SetItemChecked(index,false);};
            var apply = new Button { Text = "保存设置", Location = new Point(320, 326), Size = new Size(110, 32) };
            settings.AcceptButton=apply;
            apply.Click += delegate {
                try {
                    File.WriteAllText(Path.Combine(data, "autosave.json"), json.Serialize(new { enabled = enabled.Checked, seconds = (int)seconds.Value, default_priority = (int)priority.Value }));
                    autoSaveEnabled = enabled.Checked; autoSaveSeconds = (int)seconds.Value;defaultPriority=(int)priority.Value;entryPriority.SelectedIndex=defaultPriority;
                    ChangePrioritySelection(Enumerable.Range(0,10).Where(index=>visibleChoices.GetItemChecked(index)));
                    settings.Close();
                } catch { MessageBox.Show(settings, "设置保存失败，请检查目录写入权限。"); }
            };
            var dataLabel = new Label { Text = "当前数据目录：" + data, Location = new Point(18, 380), Size = new Size(430, 48), AutoEllipsis = true };
            var chooseData = new Button { Text = "配置数据目录（重启生效）", Location = new Point(18, 446), Size = new Size(250, 32) };
            chooseData.Click += delegate {
                try { Process.Start(new ProcessStartInfo("powershell.exe", "-NoProfile -STA -ExecutionPolicy Bypass -File \"" + Path.Combine(root, "Configure-TaskTrace.ps1") + "\"") { UseShellExecute = false, CreateNoWindow = true }); }
                catch(Exception e) { MessageBox.Show(settings, e.Message); }
            };
            settings.Controls.AddRange(new Control[] { enabled, label, seconds, help, priorityLabel, priority, visibleLabel, visibleChoices, selectAll, selectNone, apply, dataLabel, chooseData });
            Exception verificationError=null;
            if(verify)settings.Shown+=delegate {
                try {
                    var checkedPriorities=new HashSet<int>(Enumerable.Range(0,10).Where(index=>visibleChoices.GetItemChecked(index)));
                    if((int)priority.Value!=defaultPriority || priority.AccessibleName!="默认新增任务优先级" || !priorityLabel.Text.Contains("0 最高，9 最低"))throw new Exception("Default priority setting is missing or incorrect");
                    if(visibleChoices.Items.Count!=10 || visibleChoices.AccessibleName!="悬浮窗展示优先级" || !checkedPriorities.SetEquals(visiblePriorities) || visibleChoices.Bottom>settings.ClientSize.Height || chooseData.Bottom>settings.ClientSize.Height)throw new Exception("Visible priority settings are missing, stale, or clipped");
                    selectNone.PerformClick();if(Enumerable.Range(0,10).Any(index=>visibleChoices.GetItemChecked(index)))throw new Exception("Select-none did not clear visible priorities");
                    selectAll.PerformClick();if(Enumerable.Range(0,10).Any(index=>!visibleChoices.GetItemChecked(index)))throw new Exception("Select-all did not restore visible priorities");
                    for(int index=0;index<10;index++)visibleChoices.SetItemChecked(index,visiblePriorities.Contains(index));
                    using(var bitmap=new Bitmap(settings.Width,settings.Height)){settings.DrawToBitmap(bitmap,new Rectangle(Point.Empty,settings.Size));bitmap.Save(Path.Combine(data,"floating-settings-priority-test.png"));}
                }catch(Exception e){verificationError=e;}finally{settings.Close();}
            };
            settings.ShowDialog(this);if(verificationError!=null)throw verificationError;
        }
    }
    void LoadBounds() {
        try {
            var saved = ReadObject(File.ReadAllText(Path.Combine(data, "floating-window.json")));
            if(saved.ContainsKey("projectId")) preferredProjectId = Convert.ToInt64(saved["projectId"]);
            if(saved.ContainsKey("showCompleted")) showCompleted.Checked = Convert.ToBoolean(saved["showCompleted"]);
            if(saved.ContainsKey("singleLine")) singleLine.Checked = Convert.ToBoolean(saved["singleLine"]);
            var bounds = new Rectangle(Convert.ToInt32(saved["x"]), Convert.ToInt32(saved["y"]), Math.Max(350, Convert.ToInt32(saved["width"])), Math.Max(300, Convert.ToInt32(saved["height"])));
            var area = Screen.FromRectangle(bounds).WorkingArea;
            Size = new Size(Math.Min(bounds.Width, area.Width), Math.Min(bounds.Height, area.Height));
            Location = new Point(Math.Max(area.Left, Math.Min(bounds.X, area.Right - Width)), Math.Max(area.Top, Math.Min(bounds.Y, area.Bottom - Height)));
        } catch { }
    }
    void SaveBounds() {
        try { var selectedProject = projects.SelectedItem as Project; if(selectedProject != null) preferredProjectId = selectedProject.Id; var b = simpleMode ? fullBounds : (WindowState == FormWindowState.Normal ? Bounds : RestoreBounds); File.WriteAllText(Path.Combine(data, "floating-window.json"), json.Serialize(new { x = b.X, y = b.Y, width = b.Width, height = collapsed ? expandedHeight : b.Height, showCompleted = showCompleted.Checked, singleLine = singleLine.Checked, projectId = preferredProjectId })); } catch { }
    }
    async Task TestFlow() {
        try {
            string settingsPath=Path.Combine(data,"autosave.json");string savedSettings=File.Exists(settingsPath)?File.ReadAllText(settingsPath):null;
            bool savedAutoEnabled=autoSaveEnabled;int savedAutoSeconds=autoSaveSeconds,savedDefaultPriority=defaultPriority;
            try {
                File.WriteAllText(settingsPath,json.Serialize(new{enabled=false,seconds=17,default_priority=4}));LoadAutoSaveSettings();
                if(autoSaveEnabled || autoSaveSeconds!=17 || defaultPriority!=4)throw new Exception("Default priority setting did not load");
            } finally {
                if(savedSettings==null){if(File.Exists(settingsPath))File.Delete(settingsPath);}else File.WriteAllText(settingsPath,savedSettings);
                autoSaveEnabled=savedAutoEnabled;autoSaveSeconds=savedAutoSeconds;defaultPriority=savedDefaultPriority;entryPriority.SelectedIndex=defaultPriority;
            }
            Error(new Exception("Diagnostic test", new System.ComponentModel.Win32Exception(1155, "No default browser #tasktrace-local=TEST_PRIVATE_SESSION")));
            string diagnostic = File.ReadAllText(Path.Combine(data, "TaskTrace-window-error.log"));
            if(!diagnostic.Contains("Windows error code: 1155") || diagnostic.Contains("TEST_PRIVATE_SESSION") || !lastError.Contains("错误日志")) throw new Exception("Error diagnostics incomplete or leaked session");
            await Reload();
            if(projects.Items.Count == 0 || !TopMost || ShowInTaskbar || !tray.Visible) throw new Exception("Workspace, TopMost or tray-only startup failed");
            ShowAutoSaveSettings(true);
            if(newTaskButton.Parent!=bottomActions || !newTaskButton.Visible || addRow.Visible)throw new Exception("Bottom new-item button entry point missing");
            newTaskButton.PerformClick();
            if(!addRow.Visible || entry.AccessibleDescription!="新事项名称" || entryPriority.SelectedIndex!=defaultPriority || entryPriority.AccessibleDescription!="0 最高，9 最低" || search.AccessibleDescription!="查找事项" || !entry.Parent.Controls.OfType<Label>().Any(label=>label.Visible && label.Text=="新事项名称") || !entryPriority.Parent.Controls.OfType<Label>().Any(label=>label.Visible && label.Text=="优先级") || !search.Parent.Controls.OfType<Label>().Any(label=>label.Visible && label.Text=="查找事项")) throw new Exception("Labeled task fields or default priority missing");
            TestResponsiveFullLayout();ShowNewTaskEditor();
            entry.Text = "悬浮窗验收 " + DateTime.Now.Ticks; string createdTitle = entry.Text; await AddTask();
            if(addRow.Visible)throw new Exception("New task editor stayed open after creation");
            if(tasks.Nodes.Count == 0 || !tasks.Nodes[0].Text.EndsWith(createdTitle)) throw new Exception("Task creation failed");
            long id = Convert.ToInt64(tasks.Nodes[0].Tag);
            var createdTask=await Api("GET","/tasks/"+id,null);
            if(PriorityNumber(createdTask)!=defaultPriority)throw new Exception("New task did not use the configured default priority");
            if(TaskStatusValue(createdTask)!="to-do")throw new Exception("New task did not default to to-do");
            await Api("PATCH","/tasks/"+id,new{status="hold"});await LoadTasks();var heldTask=await Api("GET","/tasks/"+id,null);
            if(TaskStatusValue(heldTask)!="hold" || Convert.ToBoolean(heldTask["done"]) || !tasks.Nodes.Find(id.ToString(),true).First().Text.Contains("[暂停]"))throw new Exception("Hold status was not persisted or displayed");
            await Api("PATCH","/tasks/"+id,new{status="doing"});await LoadTasks();var doingTask=await Api("GET","/tasks/"+id,null);
            if(TaskStatusValue(doingTask)!="doing" || Convert.ToBoolean(doingTask["done"]))throw new Exception("Doing status was not persisted");
            tasks.SelectedNode=tasks.Nodes.Find(id.ToString(),true).First();await ShowSubtasks(true);
            tasks.Nodes[0].EnsureVisible();tasks.Refresh();var completionBounds=tasks.CompletionBounds(tasks.Nodes[0]);
            if(completionBounds.IsEmpty)throw new Exception("Task completion box is not visible");
            int completionPoint=((completionBounds.Top+completionBounds.Height/2)<<16)|((completionBounds.Left+completionBounds.Width/2)&0xffff);
            SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(completionPoint));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(completionPoint));
            bool clickCompleted=false;
            for(int attempt=0;attempt<40;attempt++){await Task.Delay(50);if(Convert.ToBoolean((await Api("GET","/tasks/"+id,null))["done"])){clickCompleted=true;break;}}
            if(!clickCompleted)throw new Exception("Task completion box click did not persist");
            var completedTask=await Api("GET","/tasks/"+id,null);if(TaskStatusValue(completedTask)!="done")throw new Exception("Completion did not sync task status");
            while(busy)await Task.Delay(20);await Complete(id,false);
            var reopenedTask=await Api("GET","/tasks/"+id,null);if(Convert.ToBoolean(reopenedTask["done"]) || TaskStatusValue(reopenedTask)!="to-do")throw new Exception("Task did not reopen as to-do after completion click test");
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
            if(PriorityNumber(await Api("GET","/tasks/"+childId,null))!=defaultPriority)throw new Exception("New subtask did not use the configured default priority");
            await Api("PATCH", "/tasks/" + childId, new { title = "子任务验收改名" });
            var renamedChild = await Api("GET", "/tasks/" + childId, null);
            if((string)renamedChild["title"] != "子任务验收改名") throw new Exception("Subtask rename not persisted");
            var parentWithChild = await Api("GET", "/tasks/" + id, null);
            if(!json.Serialize(parentWithChild["related_tasks"]).Contains("子任务验收")) throw new Exception("Subtask relationship missing");
            await Api("PATCH", "/tasks/" + childId, new { done = true });
            if(Convert.ToBoolean((await Api("GET", "/tasks/" + id, null))["done"])) throw new Exception("Child completion incorrectly completed parent");
            await Api("PATCH", "/tasks/" + childId, new { done = false });
            var sharedTest = new SharedList(); sharedTest.Items.Add(new PendingItem {Id="test-one",Html="跨日期待办一"}); sharedTest.Items.Add(new PendingItem {Id="test-two",Html="跨日期待办二",Done=true,Priority=2});
            await WriteShared(childId,sharedTest);
            long oldDay=await SaveProgress(childId,DateTime.Today.AddDays(-2),"第一条", "");
            long sameDay=await SaveProgress(childId,DateTime.Today.AddDays(-2),"第二条", "");
            await SaveProgress(childId,DateTime.Today.AddDays(-2),"合并编辑", "",null,sameDay,"<p>合并编辑</p>",new List<long>{oldDay});
            var sharedHistory=await ReadHistory(childId);
            var parsedShared=ReadShared(sharedHistory);
            if(parsedShared.Items.Count!=2 || parsedShared.Items[0].Done || parsedShared.Items[0].Priority!=9 || !parsedShared.Items[1].Done || parsedShared.Items[1].Priority!=2 || DailyHistory(sharedHistory).Count!=1 || Plain(ProgressBody((string)DailyHistory(sharedHistory)[0]["comment"]))!="合并编辑")throw new Exception("Shared outstanding status, priority or merged history failed");
            sharedTest.Items.RemoveAt(0);await WriteShared(childId,sharedTest);
            if(ReadShared(await ReadHistory(childId)).Items.Count!=1)throw new Exception("Individual outstanding removal failed");
            TestSimpleModeRecovery();
            var beforeSimple=Bounds;SetSimpleMode(true);Size=new Size(230,220);
            if(!simpleMode || content.Visible || toolbar.Visible || tasks.Parent!=this || FormBorderStyle!=FormBorderStyle.None)throw new Exception("Simple mode layout failed");
            SetSimpleMode(false);if(Bounds!=beforeSimple || tasks.Parent!=content || !toolbar.Visible)throw new Exception("Restore full floating window failed");
            long grandchildId = await CreateSubtask(childId, Convert.ToInt64(parentWithChild["project_id"]), "下级子任务验收");
            await LoadTasks();
            if(tasks.Nodes.Count != 1 || tasks.Nodes[0].Nodes.Cast<TreeNode>().Count(node=>node.Tag is long) != 1 || tasks.Nodes[0].Nodes[0].Nodes.Cast<TreeNode>().Count(node=>node.Tag is long) != 1) throw new Exception("Task hierarchy missing");
            if(tasks.Nodes[0].Nodes[0].Level != 1 || tasks.Nodes[0].Nodes[0].Nodes[0].Level != 2) throw new Exception("Subtask indentation missing");
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-tree-test.png")); }
            var sharedChild=tasks.Nodes.Find(childId.ToString(),true).Single();
            var sharedLeaf=sharedChild.Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf);
            var leafState=(OutstandingLeaf)sharedLeaf.Tag;
            if(sharedLeaf.Text!="1. [P2] 跨日期待办二" || !leafState.Done || leafState.Priority!=2 || sharedLeaf.StateImageIndex!=2 || !tasks.DisplayFont(sharedLeaf).Bold)throw new Exception("Full mode direct outstanding status, priority or bold font failed");
            if(singleLine.Parent!=projectRow || !singleLine.Visible || singleLine.AccessibleName!="任务单行显示")throw new Exception("Single-line checkbox is missing from full floating mode");
            rendering=true;singleLine.Checked=true;rendering=false;ApplyTaskTreeLayout();await LoadTasks();
            var flatGrandchild=tasks.Nodes.Find(grandchildId.ToString(),true).Single();
            string expectedAncestors=TaskTreeView.SingleLineSeparator+"子任务验收改名"+TaskTreeView.SingleLineSeparator+createdTitle;
            if(tasks.CurrentTaskText(flatGrandchild).EndsWith("下级子任务验收")==false || tasks.AncestorTaskText(flatGrandchild)!=expectedAncestors || flatGrandchild.Level!=0 || tasks.ShowLines || tasks.ShowPlusMinus || !flatGrandchild.IsVisible || tasks.Nodes.Count!=3)throw new Exception("Single-line task path layout failed: current="+tasks.CurrentTaskText(flatGrandchild)+" ancestors="+tasks.AncestorTaskText(flatGrandchild)+" expected="+expectedAncestors+" level="+flatGrandchild.Level+" rootCount="+tasks.Nodes.Count+" lines="+tasks.ShowLines+" roots="+tasks.ShowRootLines+" plus="+tasks.ShowPlusMinus+" visible="+flatGrandchild.IsVisible);
            var flatRoot=tasks.Nodes.Find(id.ToString(),true).Single();var flatChild=tasks.Nodes.Find(childId.ToString(),true).Single();
            var rootCheck=tasks.CompletionBounds(flatRoot);var childCheck=tasks.CompletionBounds(flatChild);var grandchildCheck=tasks.CompletionBounds(flatGrandchild);
            if(rootCheck.Left!=childCheck.Left || childCheck.Left!=grandchildCheck.Left || flatRoot.StateImageIndex!=0 || flatChild.StateImageIndex!=0 || flatGrandchild.StateImageIndex!=0)throw new Exception("Single-line task completion boxes are not aligned");
            SaveBounds();var singleLineSettings=ReadObject(File.ReadAllText(Path.Combine(data,"floating-window.json")));if(!Convert.ToBoolean(singleLineSettings["singleLine"]))throw new Exception("Single-line preference was not persisted");
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-single-line-full-test.png")); }
            int flatCompletionPoint=((grandchildCheck.Top+grandchildCheck.Height/2)<<16)|((grandchildCheck.Left+grandchildCheck.Width/2)&0xffff);
            SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(flatCompletionPoint));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(flatCompletionPoint));
            bool flatCompleted=false;for(int attempt=0;attempt<40;attempt++){await Task.Delay(50);if(Convert.ToBoolean((await Api("GET","/tasks/"+grandchildId,null))["done"])){flatCompleted=true;break;}}
            if(!flatCompleted)throw new Exception("Single-line custom completion box did not persist");while(busy)await Task.Delay(20);await Complete(grandchildId,false);
            sharedChild=tasks.Nodes.Find(childId.ToString(),true).Single();sharedLeaf=sharedChild.Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf);leafState=(OutstandingLeaf)sharedLeaf.Tag;
            sharedLeaf.EnsureVisible();tasks.Refresh();var outstandingCheck=tasks.CompletionBounds(sharedLeaf);if(outstandingCheck.IsEmpty)throw new Exception("Outstanding completion box is not visible");
            int outstandingPoint=((outstandingCheck.Top+outstandingCheck.Height/2)<<16)|((outstandingCheck.Left+outstandingCheck.Width/2)&0xffff);
            SendSimpleMessage(tasks.Handle,0x201,new IntPtr(1),new IntPtr(outstandingPoint));SendSimpleMessage(tasks.Handle,0x202,IntPtr.Zero,new IntPtr(outstandingPoint));
            bool outstandingReopened=false;for(int attempt=0;attempt<40;attempt++){await Task.Delay(50);var state=ReadShared(await ReadHistory(childId));if(state.Items.Count==1 && !state.Items[0].Done){outstandingReopened=true;break;}}
            if(!outstandingReopened)throw new Exception("Outstanding completion box click did not persist");while(busy)await Task.Delay(20);
            await UpdateOutstandingState(childId,"test-two",null,4);var updatedShared=ReadShared(await ReadHistory(childId));
            if(updatedShared.Items.Count!=1 || updatedShared.Items[0].Done || updatedShared.Items[0].Priority!=4)throw new Exception("Outstanding completion or priority update failed");
            await LoadTasks();sharedChild=tasks.Nodes.Find(childId.ToString(),true).Single();sharedLeaf=sharedChild.Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf);leafState=(OutstandingLeaf)sharedLeaf.Tag;
            if(sharedLeaf.Text!="1. [P4] 跨日期待办二" || leafState.Done || sharedLeaf.StateImageIndex!=1)throw new Exception("Outstanding completion or priority refresh failed");
            tasks.SelectedNode=sharedLeaf;int beforeModeLoad=taskLoadVersion;
            SetSimpleMode(true);Size=new Size(330,260);
            if(tasks.Nodes.Find(childId.ToString(),true).Single()!=sharedChild || sharedLeaf.Parent!=sharedChild || tasks.SelectedNode!=sharedLeaf || taskLoadVersion!=beforeModeLoad)throw new Exception("Mode switch changed shared task data or selection");
            flatGrandchild=tasks.Nodes.Find(grandchildId.ToString(),true).Single();
            if(!singleLine.Checked || !tasks.SingleLinePaths || tasks.AncestorTaskText(flatGrandchild)!=expectedAncestors || !flatGrandchild.IsVisible || !tasks.DisplayFont(sharedLeaf).Bold)throw new Exception("Simple mode did not retain single-line paths or bold outstanding items");
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-single-line-simple-test.png")); }
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-simple-test.png")); }
            ShowSimpleModeRestore();
            using(var bitmap = new Bitmap(Width, Height)) { DrawToBitmap(bitmap, new Rectangle(Point.Empty, Size)); bitmap.Save(Path.Combine(data, "floating-simple-selected-test.png")); }
            restoreSimple.PerformClick();if(simpleMode)throw new Exception("Simple mode restore button failed");
            if(sharedLeaf.Parent!=sharedChild || tasks.SelectedNode!=sharedLeaf || taskLoadVersion!=beforeModeLoad)throw new Exception("Full mode restore changed shared task data or selection");
            rendering=true;singleLine.Checked=false;rendering=false;ApplyTaskTreeLayout();SaveBounds();await LoadTasks();
            tasks.Nodes[0].Collapse(); collapsedTasks.Clear(); LoadTreePreferences(); await LoadTasks();
            if(tasks.Nodes[0].IsExpanded || !collapsedTasks.Contains(id)) throw new Exception("Collapsed state not retained");
            search.Text = "下级子任务验收"; await LoadTasks();
            if(tasks.Nodes.Count != 1 || !tasks.Nodes[0].IsExpanded || tasks.Nodes[0].Nodes[0].Nodes.Cast<TreeNode>().Count(node=>node.Tag is long) != 1) throw new Exception("Search lost hierarchy");
            search.Clear(); await LoadTasks(); tasks.Nodes[0].Expand();
            if(collapsedTasks.Contains(id)) throw new Exception("Expand state not retained");
            await Api("PATCH", "/tasks/" + id, new { done = true }); await LoadTasks();
            if(tasks.Nodes.Count != 1 || !tasks.Nodes[0].Checked || tasks.Nodes[0].Nodes.Cast<TreeNode>().Count(node=>node.Tag is long) != 1) throw new Exception("Completed ancestor lost pending children");
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
                page = 1; await Reload(); if(tasks.Nodes.Count != 51) throw new Exception("Task list still paginates instead of scrolling all task groups");
                search.Text = "分页验收事项 50"; page = 1; await Reload(); if(tasks.Nodes.Count != 1) throw new Exception("Search failed");
            }
            { foreach(long testId in ids) await Api("DELETE", "/tasks/" + testId, null); search.Clear(); page = 1; }
            string browserSession = await PrepareBrowserSession();
            var browserPair = ReadObject(browserSession);
            using(var request = new HttpRequestMessage(HttpMethod.Get, url + "/api/v2/projects")) {
                request.Headers.Authorization = new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer", (string)browserPair["token"]);
                using(var response = await http.SendAsync(request)) if(!response.IsSuccessStatusCode) throw new Exception("Browser session failed");
            }
            await TestInteractions();
            await TestPriorityFilter();
            TestPriorityLinks();
            await TestSimpleOutstandingDetails();
            TestSimpleOutstandingActions();
            await TestUndo();
            await TestProgressReferences();
            await TestAutoRefresh();
            token = "expired"; await Reload();
            if(status.ForeColor != ForeColor) throw new Exception("Session refresh failed");
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
            File.WriteAllText(Path.Combine(data, "floating-test.txt"), "PASS: bottom new-item button and labeled editor, responsive text fields after repeated width changes, all task groups scroll without pagination, double-click leaves expansion unchanged, shortcut-only undo and F5 refresh, persistent grouped undo, stale undo rejection, unrelated updates preserved, undo task/comment/delete/move/image, native text shortcut isolation, drag/drop reparent and order, outstanding move with image migration, priority sorting, image gallery, numbering, five-level task limit, rejected sixth level without orphan, tray-only startup, close/minimize to tray, full/simple tray restore, simple mode, resizing, restore button, shared direct outstanding tree across modes, bold outstanding items, persisted full/simple single-line task paths, layout-only mode switches, shared list, same-day merge, full error diagnostics, Windows error code, session redaction, hierarchy, nested indentation, collapse/expand retention, search ancestors, completed parent context, show/hide completed, reopen, completed search, saved filter preference, create, complete preserving description, search, independent browser session, pin, restore; TopMost=" + TopMost);
        } catch(Exception e) { File.WriteAllText(Path.Combine(data, "floating-test.txt"), "FAIL: " + e); Environment.ExitCode = 1; }
        finally { allowExit = true; Close(); }
    }
    protected override void Dispose(bool disposing) { if(disposing) { DisposeAutoRefresh(); timer.Dispose(); hoverTimer.Dispose(); progressTip.Dispose(); tray.Dispose(); http.Dispose(); } base.Dispose(disposing); }
}
