// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    readonly Button priorityFilterButton = new Button {Text="按优先级",AutoSize=true,AccessibleName="按优先级筛选"};
    readonly HashSet<int> visiblePriorities = new HashSet<int>(Enumerable.Range(0,10));
    readonly Timer priorityFilterReloadTimer = new Timer {Interval=150};
    readonly List<ToolStripDropDown> priorityFilterMenus = new List<ToolStripDropDown>();
    bool priorityFilterInitialized, priorityFilterReloadPending, priorityFilterReloading;
    long priorityFilterRevision;

    bool PriorityFilterActive { get { return visiblePriorities.Count != 10; } }
    string PriorityFilterEmptyMessage { get { return visiblePriorities.Count == 0 ? "尚未选择任何优先级，请在“按优先级”菜单中选择或全选。" : "没有符合所选优先级的事项，可在“按优先级”菜单中全选。"; } }
    bool MatchesPriority(Dictionary<string,object> task) { return task != null && visiblePriorities.Contains(PriorityNumber(task)); }
    bool MatchesPriority(PendingItem item) { return item != null && visiblePriorities.Contains(Math.Max(0,Math.Min(9,item.Priority))); }
    SharedList FilterOutstandingPriorities(SharedList source) {
        if(source==null || !PriorityFilterActive)return source;
        var filtered=new SharedList {CommentId=source.CommentId};
        filtered.Items.AddRange(source.Items.Where(MatchesPriority));return filtered;
    }
    string PriorityFilterPath { get { return Path.Combine(data,"floating-priority-filter.json"); } }

    void InitializePriorityFilter() {
        if(priorityFilterInitialized)return;
        priorityFilterInitialized=true;
        LoadPriorityFilterPreference();
        var menu=CreatePriorityFilterDropDown();
        priorityFilterButton.Click+=delegate {if(!closing && !IsDisposed)menu.Show(priorityFilterButton,new Point(0,priorityFilterButton.Height));};
        prioritySort.CheckedChanged+=delegate {UpdatePriorityFilterControls();};
        priorityFilterReloadTimer.Tick+=async delegate {await ApplyPendingPriorityFilter();};
        Disposed+=delegate {
            priorityFilterReloadTimer.Stop();priorityFilterReloadTimer.Dispose();
            foreach(var item in priorityFilterMenus.ToArray())item.Dispose();
        };
        UpdatePriorityFilterControls();
    }

    ToolStripMenuItem CreatePriorityFilterMenuItem() {
        var item=new ToolStripMenuItem("按优先级筛选");
        item.DropDown=CreatePriorityFilterDropDown();
        return item;
    }

    ContextMenuStrip CreatePriorityFilterDropDown() {
        var menu=new ContextMenuStrip {ShowCheckMargin=true,ShowImageMargin=false};
        menu.Items.Add(new ToolStripMenuItem("可多选，关闭菜单后刷新") {Enabled=false});
        var all=new ToolStripMenuItem("全选") {Name="priority-filter-all"};
        var none=new ToolStripMenuItem("全不选") {Name="priority-filter-none"};
        all.Click+=delegate {ChangePrioritySelection(Enumerable.Range(0,10));};
        none.Click+=delegate {ChangePrioritySelection(new int[0]);};
        menu.Items.Add(all);menu.Items.Add(none);menu.Items.Add(new ToolStripSeparator());
        for(int number=0;number<=9;number++) {
            int priority=number;
            var choice=new ToolStripMenuItem(PriorityChoiceText(number)) {CheckOnClick=true,Tag=priority,Checked=visiblePriorities.Contains(priority)};
            choice.Click+=delegate {
                var selection=new HashSet<int>(visiblePriorities);
                if(choice.Checked)selection.Add(priority);else selection.Remove(priority);
                ChangePrioritySelection(selection);
            };
            menu.Items.Add(choice);
        }
        menu.Items.Add(new ToolStripSeparator());
        var sort=new ToolStripMenuItem("按优先级排序") {Name="priority-filter-sort",CheckOnClick=true,Checked=prioritySort.Checked};
        sort.Click+=delegate {
            bool previousRendering=rendering;rendering=true;
            try {prioritySort.Checked=sort.Checked;SaveSortPreference();}
            finally {rendering=previousRendering;}
            UpdatePriorityFilterControls();QueuePriorityFilterReload();
        };
        menu.Items.Add(sort);
        // Keep mouse and keyboard multi-selection in one menu interaction.
        menu.Closing+=delegate(object sender,ToolStripDropDownClosingEventArgs e) {if(e.CloseReason==ToolStripDropDownCloseReason.ItemClicked)e.Cancel=true;};
        menu.Opening+=delegate {UpdatePriorityFilterControls();};
        menu.Closed+=delegate {if(priorityFilterReloadPending && !closing && !IsDisposed)priorityFilterReloadTimer.Start();};
        menu.Disposed+=delegate {priorityFilterMenus.Remove(menu);};
        priorityFilterMenus.Add(menu);
        return menu;
    }

    void UpdatePriorityFilterControls() {
        if(closing || IsDisposed)return;
        priorityFilterButton.Text=visiblePriorities.Count==10?"按优先级":"按优先级 ("+visiblePriorities.Count+"/10)";
        progressTip.SetToolTip(priorityFilterButton,visiblePriorities.Count==0?PriorityFilterEmptyMessage:"选择要显示的优先级，可多选；父任务会为匹配的子任务或遗留事项保留。");
        foreach(var menu in priorityFilterMenus.ToArray()) {
            if(menu.IsDisposed)continue;
            foreach(ToolStripItem item in menu.Items) {
                var choice=item as ToolStripMenuItem;if(choice==null)continue;
                if(choice.Tag is int)choice.Checked=visiblePriorities.Contains((int)choice.Tag);
                else if(choice.Name=="priority-filter-sort")choice.Checked=prioritySort.Checked;
                else if(choice.Name=="priority-filter-all")choice.Enabled=visiblePriorities.Count!=10;
                else if(choice.Name=="priority-filter-none")choice.Enabled=visiblePriorities.Count!=0;
            }
        }
    }

    void ChangePrioritySelection(IEnumerable<int> values) {
        var selection=new HashSet<int>(values);
        if(selection.Any(value=>value<0 || value>9))throw new ArgumentOutOfRangeException("values");
        if(visiblePriorities.SetEquals(selection))return;
        visiblePriorities.Clear();visiblePriorities.UnionWith(selection);
        SavePriorityFilterPreference();UpdatePriorityFilterControls();QueuePriorityFilterReload();
    }

    void QueuePriorityFilterReload() {
        priorityFilterRevision++;priorityFilterReloadPending=true;page=1;
        if(!closing && !IsDisposed)priorityFilterReloadTimer.Start();
    }

    async Task ApplyPendingPriorityFilter() {
        if(closing || IsDisposed){priorityFilterReloadTimer.Stop();return;}
        if(priorityFilterReloading || busy || rendering || dragging || priorityFilterMenus.Any(menu=>!menu.IsDisposed && menu.Visible))return;
        if(!priorityFilterReloadPending){priorityFilterReloadTimer.Stop();return;}
        priorityFilterReloadTimer.Stop();priorityFilterReloadPending=false;priorityFilterReloading=true;
        long revision=priorityFilterRevision;
        try {await Reload();}
        finally {
            priorityFilterReloading=false;
            if(revision!=priorityFilterRevision)priorityFilterReloadPending=true;
            if(priorityFilterReloadPending && !closing && !IsDisposed)priorityFilterReloadTimer.Start();
        }
    }

    bool TryParsePriorityFilter(string text,out HashSet<int> selection) {
        selection=null;
        try {
            var prefs=json.DeserializeObject(text) as Dictionary<string,object>;
            object version,values;
            if(prefs==null || !prefs.TryGetValue("version",out version) || !(version is int) || (int)version!=1 || !prefs.TryGetValue("priorities",out values))return false;
            var list=values as IEnumerable;if(list==null || values is string || values is IDictionary)return false;
            var result=new HashSet<int>();
            foreach(object value in list) {
                if(!(value is int) || (int)value<0 || (int)value>9)return false;
                result.Add((int)value);
            }
            selection=result;return true;
        } catch {return false;}
    }

    void LoadPriorityFilterPreference() {
        var selected=new HashSet<int>(Enumerable.Range(0,10));
        try {HashSet<int> saved;if(TryParsePriorityFilter(File.ReadAllText(PriorityFilterPath),out saved))selected=saved;} catch { }
        visiblePriorities.Clear();visiblePriorities.UnionWith(selected);UpdatePriorityFilterControls();
    }

    void SavePriorityFilterPreference() {
        try {File.WriteAllText(PriorityFilterPath,json.Serialize(new {version=1,priorities=visiblePriorities.OrderBy(value=>value).ToArray()}));}
        catch {if(!closing && !IsDisposed)status.Text="筛选已应用，但设置未能保存；下次启动需重新选择。";}
    }

    async Task TestPriorityFilter() {
        if(!selfTest)throw new InvalidOperationException("Priority filter tests require the isolated self-test instance.");
        var original=new HashSet<int>(visiblePriorities);
        string savedFilter=File.Exists(PriorityFilterPath)?File.ReadAllText(PriorityFilterPath):null;
        string sortPath=Path.Combine(data,"floating-order.json");string savedSort=File.Exists(sortPath)?File.ReadAllText(sortPath):null;
        string originalSearch=search.Text;bool originalSort=prioritySort.Checked,originalCompleted=showCompleted.Checked;
        var created=new List<long>();var project=projects.SelectedItem as Project;Exception failure=null;
        try {
            HashSet<int> parsed;
            foreach(string invalid in new[]{"{}","null","{","{\"version\":2,\"priorities\":[1]}","{\"version\":1,\"priorities\":[-1]}","{\"version\":1,\"priorities\":[10]}","{\"version\":1,\"priorities\":[1.5]}","{\"version\":1,\"priorities\":[\"1\"]}","{\"version\":1,\"priorities\":[true]}","{\"version\":1,\"priorities\":null}"})if(TryParsePriorityFilter(invalid,out parsed))throw new Exception("Invalid priority filter preference was accepted");
            if(!TryParsePriorityFilter("{\"version\":1,\"priorities\":[]}",out parsed) || parsed.Count!=0)throw new Exception("Empty priority filter was mistaken for missing settings");
            SetBusy(true);
            using(var first=CreatePriorityFilterMenuItem())using(var second=CreatePriorityFilterMenuItem()) {
                ChangePrioritySelection(Enumerable.Range(0,10));
                for(int number=0;number<=9;number++)if(!MatchesPriority(new Dictionary<string,object>{{"priority",10-number}}))throw new Exception("Default selection omits a priority");
                if(!MatchesPriority(new Dictionary<string,object>{{"priority",0}}))throw new Exception("Legacy unspecified priority nine is not selected");
                first.DropDownItems["priority-filter-none"].PerformClick();
                var chooseZero=first.DropDownItems.Cast<ToolStripItem>().OfType<ToolStripMenuItem>().Single(item=>item.Tag is int && (int)item.Tag==0);
                var chooseNine=first.DropDownItems.Cast<ToolStripItem>().OfType<ToolStripMenuItem>().Single(item=>item.Tag is int && (int)item.Tag==9);
                chooseZero.PerformClick();chooseNine.PerformClick();chooseZero.PerformClick();chooseZero.PerformClick();
                await ApplyPendingPriorityFilter();
                if(!priorityFilterReloadPending || !visiblePriorities.SetEquals(new[]{0,9}))throw new Exception("Rapid selections were dropped while loading");
                var otherNine=second.DropDownItems.Cast<ToolStripItem>().OfType<ToolStripMenuItem>().Single(item=>item.Tag is int && (int)item.Tag==9);
                if(!otherNine.Checked)throw new Exception("Independent priority menus do not share selection state");
                visiblePriorities.Clear();LoadPriorityFilterPreference();
                if(!visiblePriorities.SetEquals(new[]{0,9}))throw new Exception("Priority filter did not persist");
                ChangePrioritySelection(new int[0]);visiblePriorities.Add(1);LoadPriorityFilterPreference();
                if(visiblePriorities.Count!=0 || !PriorityFilterEmptyMessage.Contains("尚未选择"))throw new Exception("Select-none persistence or empty explanation failed");
                File.WriteAllText(PriorityFilterPath,"{\"version\":1,\"priorities\":[20]}");LoadPriorityFilterPreference();
                if(visiblePriorities.Count!=10)throw new Exception("Corrupt priority preference did not fall back to all");
                bool expectedSort=!prioritySort.Checked;first.DropDownItems["priority-filter-sort"].PerformClick();
                if(prioritySort.Checked!=expectedSort || ((ToolStripMenuItem)second.DropDownItems["priority-filter-sort"]).Checked!=expectedSort)throw new Exception("Sort menu does not reflect shared sorting state");
                bool previousRendering=rendering;rendering=true;prioritySort.Checked=false;rendering=previousRendering;
                if(((ToolStripMenuItem)first.DropDownItems["priority-filter-sort"]).Checked)throw new Exception("Manual order restoration left stale menu checks");
            }
            rendering=true;search.Clear();showCompleted.Checked=false;rendering=false;
            long parent,child,hidden,outstandingOwner;
            using(BeginUndoGroup()) {
                parent=Convert.ToInt64((await Api("POST","/projects/"+project.Id+"/tasks",new{title="优先级筛选父任务",priority=1}))["id"]);created.Add(parent);
                child=await CreateSubtask(parent,project.Id,"优先级筛选命中子任务");created.Add(child);await Api("PATCH","/tasks/"+child,new{priority=10});
                hidden=Convert.ToInt64((await Api("POST","/projects/"+project.Id+"/tasks",new{title="优先级筛选隐藏任务",priority=5}))["id"]);created.Add(hidden);
                outstandingOwner=Convert.ToInt64((await Api("POST","/projects/"+project.Id+"/tasks",new{title="遗留事项命中优先级的父任务",priority=1}))["id"]);created.Add(outstandingOwner);
                var shared=new SharedList();shared.Items.Add(new PendingItem{Id="priority-match",Html="命中筛选的遗留事项",Priority=0});shared.Items.Add(new PendingItem{Id="priority-hidden",Html="未命中筛选的遗留事项",Priority=4});await WriteShared(outstandingOwner,shared);
            }
            ChangePrioritySelection(new[]{0});SetBusy(false);await ApplyPendingPriorityFilter();
            var parents=tasks.Nodes.Find(parent.ToString(),true);var children=tasks.Nodes.Find(child.ToString(),true);var outstandingOwners=tasks.Nodes.Find(outstandingOwner.ToString(),true);
            if(parents.Length!=1 || children.Length!=1 || children[0].Parent!=parents[0] || tasks.Nodes.Find(hidden.ToString(),true).Length!=0)throw new Exception("Priority filter failed to retain unmatched ancestors or hide unmatched tasks");
            if(outstandingOwners.Length!=1 || outstandingOwners[0].Nodes.Cast<TreeNode>().Count(node=>node.Tag is OutstandingLeaf)!=1 || ((OutstandingLeaf)outstandingOwners[0].Nodes.Cast<TreeNode>().Single(node=>node.Tag is OutstandingLeaf).Tag).Priority!=0)throw new Exception("Matching outstanding priority did not retain its task or hide nonmatching outstanding items");
            ChangePrioritySelection(new int[0]);await ApplyPendingPriorityFilter();
            if(tasks.Nodes.Count!=0)throw new Exception("No-priority selection still displays tasks");
            ChangePrioritySelection(Enumerable.Range(0,10));await ApplyPendingPriorityFilter();
            if(tasks.Nodes.Find(hidden.ToString(),true).Length!=1)throw new Exception("Select-all did not restore hidden tasks");
            File.WriteAllText(Path.Combine(data,"floating-priority-filter-test.txt"),"PASS: priorities 0-9 plus legacy unspecified priority9; multi-select/all/none; independent menus synchronized; settings persist including none and reject malformed/out-of-range types; sorting and manual-order menu state; rapid changes retained while busy; unmatched ancestors retained; matching outstanding retains its task; nonmatching outstanding hidden; unmatched tasks hidden; select-all restores tasks.");
        } catch(Exception e) {failure=e;}
        {
            SetBusy(true);priorityFilterReloadTimer.Stop();priorityFilterReloadPending=false;
            using(BeginUndoGroup())foreach(long id in created.AsEnumerable().Reverse())try{await Api("DELETE","/tasks/"+id,null);}catch { }
            rendering=true;visiblePriorities.Clear();visiblePriorities.UnionWith(original);prioritySort.Checked=originalSort;showCompleted.Checked=originalCompleted;search.Text=originalSearch;rendering=false;
            if(savedFilter==null){if(File.Exists(PriorityFilterPath))File.Delete(PriorityFilterPath);}else File.WriteAllText(PriorityFilterPath,savedFilter);
            if(savedSort==null){if(File.Exists(sortPath))File.Delete(sortPath);}else File.WriteAllText(sortPath,savedSort);
            UpdatePriorityFilterControls();SetBusy(false);await Reload();
        }
        if(failure!=null)throw new Exception("Priority filter self-test failed",failure);
    }
}
