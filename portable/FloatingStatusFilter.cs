// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    static readonly string[] displayStates={"to-do","doing","done","hold"};
    readonly HashSet<string> visibleTaskStatuses=new HashSet<string>(new[]{"to-do","doing","hold"});
    readonly Button displayStatusButton=new Button {Text="显示选项",AccessibleName="显示选项",Dock=DockStyle.Fill};
    readonly ContextMenuStrip displayStatusMenu=new ContextMenuStrip();
    readonly Dictionary<long,string> recentlyCompletedTaskStates=new Dictionary<long,string>();
    string[] SelectedDisplayStatuses() {return displayStates.Where(MatchesDisplayState).ToArray();}
    bool MatchesDisplayState(string state) {return state=="done"?showCompleted.Checked:visibleTaskStatuses.Contains(state);}
    void SetDisplayStatuses(IEnumerable<string> states) {
        var values=new HashSet<string>(states);bool previous=rendering;rendering=true;
        try {visibleTaskStatuses.Clear();foreach(string state in displayStates)if(state!="done"&&values.Contains(state))visibleTaskStatuses.Add(state);showCompleted.Checked=values.Contains("done");}
        finally {rendering=previous;}
        RefreshDisplayStatusMenu();
    }
    void RefreshDisplayStatusMenu() {
        int count=SelectedDisplayStatuses().Length;
        foreach(ToolStripMenuItem item in displayStatusMenu.Items.OfType<ToolStripMenuItem>()) {
            string state=Convert.ToString(item.Tag);
            item.CheckState=state=="all"?(count==4?CheckState.Checked:count==0?CheckState.Unchecked:CheckState.Indeterminate):
                MatchesDisplayState(state)?CheckState.Checked:CheckState.Unchecked;
        }
    }
    async Task ChangeDisplayStatus(string state) {
        if(busy||closing)return;
        var selected=SelectedDisplayStatuses().ToList();
        if(state=="all")selected=selected.Count==4?new List<string>():displayStates.ToList();
        else if(!selected.Remove(state))selected.Add(state);
        SetDisplayStatuses(selected);page=1;SaveBounds();await Reload();
    }
    void InitializeDisplayStatusFilter() {
        foreach(string value in new[]{"all","to-do","doing","done","hold"}) {
            string state=value;
            var item=new ToolStripMenuItem(state=="all"?"全选":TaskStatusText(state)){Tag=state};
            item.Click+=async delegate {await ChangeDisplayStatus(state);};displayStatusMenu.Items.Add(item);
            if(state=="all")displayStatusMenu.Items.Add(new ToolStripSeparator());
        }
        displayStatusMenu.Opening+=delegate(object sender,System.ComponentModel.CancelEventArgs e){e.Cancel=busy||closing;RefreshDisplayStatusMenu();};
        displayStatusButton.Click+=delegate {displayStatusMenu.Show(displayStatusButton,0,displayStatusButton.Height);};
        RefreshDisplayStatusMenu();
        FormClosed+=delegate {displayStatusMenu.Dispose();};
    }
    string SelectedNodeStatus(TreeNode node) {
        var leaf=node.Tag as OutstandingLeaf;if(leaf!=null)return leaf.Done?"done":"to-do";
        Dictionary<string,object> task;
        return node.Tag is long&&taskCache.TryGetValue((long)node.Tag,out task)?TaskStatusValue(task):node.Checked?"done":"to-do";
    }
    void RefreshStateMenu(ToolStripMenuItem menu) {
        var selection=SelectedActionNodes();
        menu.Enabled=selection.Count>0;menu.Text=selection.Count>1?"批量设置状态":"设置状态";
        foreach(ToolStripMenuItem item in menu.DropDownItems.OfType<ToolStripMenuItem>()) {
            string state=Convert.ToString(item.Tag);
            item.Enabled=selection.Count>0&&((state=="to-do"||state=="done")||selection.All(node=>node.Tag is long));
            item.Checked=selection.Count>0&&selection.All(node=>SelectedNodeStatus(node)==state);
            item.ToolTipText=item.Enabled?"":"遗留事项支持待办和已完成状态。";
        }
    }
}
