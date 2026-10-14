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
    readonly CheckBox showCompletion=new CheckBox{Text="显示完成度",AutoSize=true,AccessibleName="显示条目完成度"};
    readonly Button sortOptions=new Button{Text="排序方式",AutoSize=true,AccessibleName="设置多级排序"};
    List<string> completionSort=new List<string>{"priority:asc"};
    bool settingCompletionSort;
    static int ManualCompletion(object value) {double number;return value!=null && Double.TryParse(Convert.ToString(value),out number) && !Double.IsNaN(number) && !Double.IsInfinity(number)?(int)Math.Round(Math.Max(0,Math.Min(100,number))):0;}
    static int TaskManualCompletion(Dictionary<string,object> task) {object value;return task!=null && task.TryGetValue("percent_done",out value)?ManualCompletion(Convert.ToDouble(value)*100):0;}
    static int EffectiveCompletion(int manual,bool done) {return done?100:ManualCompletion(manual);}
    string CompletionStatus(string state,int manual,bool done) {return "["+state+(showCompletion.Checked?"-"+EffectiveCompletion(manual,done)+"%":"")+"] ";}
    void InitializeCompletion() {
        try {
            var prefs=ReadObject(File.ReadAllText(Path.Combine(data,"floating-order.json")));object value;
            if(prefs.TryGetValue("sort",out value))completionSort=((IEnumerable)value).Cast<object>().Select(Convert.ToString).Where(key=>new[]{"priority:asc","priority:desc","completion:asc","completion:desc"}.Contains(key)).GroupBy(key=>key.Split(':')[0]).Select(group=>group.First()).Take(2).ToList();
            else if(prefs.TryGetValue("priority",out value)&&!Convert.ToBoolean(value))completionSort.Clear();
            if(prefs.TryGetValue("showCompletion",out value))showCompletion.Checked=Convert.ToBoolean(value);
        }catch{}
        prioritySort.Checked=completionSort.Count>0;
        showCompletion.CheckedChanged+=delegate{SaveSortPreference();if(!rendering)ObserveCompletionRefresh(RefreshCompletionView());};
        sortOptions.Click+=delegate{ShowCompletionSort();};
        prioritySort.CheckedChanged+=async delegate{
            if(settingCompletionSort)return;
            completionSort=prioritySort.Checked?new List<string>{"priority:asc"}:new List<string>();
            SaveSortPreference();if(!rendering)await Reload();
        };
    }
    async Task RefreshCompletionView() {Point viewport=tasks.NativeScrollPosition();await LoadTasks();tasks.RestoreNativeScroll(viewport);}
    int CompareCompletionValues(bool doneA,int priorityA,int completionA,bool doneB,int priorityB,int completionB) {
        // Preserve the established completed-last grouping unless completion itself is a sort key.
        if(!completionSort.Any(key=>key.StartsWith("completion:"))){int done=doneA.CompareTo(doneB);if(done!=0)return done;}
        foreach(string key in completionSort){int result=key.StartsWith("priority:")?priorityA.CompareTo(priorityB):completionA.CompareTo(completionB);if(result!=0)return key.EndsWith(":desc")?-result:result;}
        return doneA.CompareTo(doneB);
    }
    int CompareCompletionNodes(TreeNode a,TreeNode b,Dictionary<long,Dictionary<string,object>> all) {
        var left=a.Tag as OutstandingLeaf;var right=b.Tag as OutstandingLeaf;
        var leftTask=left==null&&a.Tag is long&&all.ContainsKey((long)a.Tag)?all[(long)a.Tag]:null;
        var rightTask=right==null&&b.Tag is long&&all.ContainsKey((long)b.Tag)?all[(long)b.Tag]:null;
        return CompareCompletionValues(a.Checked,left==null?(leftTask==null?9:PriorityNumber(leftTask)):left.Priority,EffectiveCompletion(left==null?TaskManualCompletion(leftTask):left.Completion,a.Checked),b.Checked,right==null?(rightTask==null?9:PriorityNumber(rightTask)):right.Priority,EffectiveCompletion(right==null?TaskManualCompletion(rightTask):right.Completion,b.Checked));
    }
    List<TreeNode> OrderCompletionNodes(IEnumerable<TreeNode> nodes,Dictionary<long,Dictionary<string,object>> all) {return nodes.OrderBy(node=>node,Comparer<TreeNode>.Create((a,b)=>CompareCompletionNodes(a,b,all))).ToList();}
    void SortCompletionHierarchy(List<TreeNode> roots,Dictionary<long,Dictionary<string,object>> all) {
        var ordered=OrderCompletionNodes(roots,all);roots.Clear();roots.AddRange(ordered);
        foreach(var node in roots){var children=node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is long).ToList();SortCompletionHierarchy(children,all);node.Nodes.Clear();node.Nodes.AddRange(children.ToArray());}
    }
    void ShowCompletionSort() {
        using(var dialog=DpiDialog(new Form{Text="悬浮窗排序",ClientSize=new Size(430,258),MinimumSize=new Size(430,295),Font=Font,TopMost=TopMost,StartPosition=FormStartPosition.CenterParent})) {
            var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(14),ColumnCount=2,RowCount=4};layout.ColumnStyles.Add(new ColumnStyle(SizeType.Absolute,100));layout.ColumnStyles.Add(new ColumnStyle(SizeType.Percent,100));
            layout.RowStyles.Add(new RowStyle(SizeType.Absolute,46));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,46));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));
            string[] names={"手动顺序 / 不使用","优先级升序（0 → 9）","优先级降序（9 → 0）","完成度升序（0% → 100%）","完成度降序（100% → 0%）"};string[] keys={"","priority:asc","priority:desc","completion:asc","completion:desc"};
            var first=new ComboBox{Dock=DockStyle.Fill,DropDownStyle=ComboBoxStyle.DropDownList,AccessibleName="第一排序条件"};var second=new ComboBox{Dock=DockStyle.Fill,DropDownStyle=ComboBoxStyle.DropDownList,AccessibleName="第二排序条件"};first.Items.AddRange(names);second.Items.AddRange(names);
            first.SelectedIndex=Math.Max(0,Array.IndexOf(keys,completionSort.FirstOrDefault()??""));second.SelectedIndex=Math.Max(0,Array.IndexOf(keys,completionSort.Skip(1).FirstOrDefault()??""));
            layout.Controls.Add(new Label{Text="第一排序条件",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft},0,0);layout.Controls.Add(first,1,0);layout.Controls.Add(new Label{Text="第二排序条件",Dock=DockStyle.Fill,TextAlign=ContentAlignment.MiddleLeft},0,1);layout.Controls.Add(second,1,1);
            var hint=new Label{Text="先按第一条件排序，相同时再按第二条件。树状显示只调整同级顺序；完成度按最终显示值排序。",Dock=DockStyle.Fill};layout.Controls.Add(hint,0,2);layout.SetColumnSpan(hint,2);
            var save=new Button{Text="应用排序",Dock=DockStyle.Fill};layout.Controls.Add(save,1,3);dialog.Controls.Add(layout);
            save.Click+=delegate{
                var selected=new[]{keys[first.SelectedIndex],keys[second.SelectedIndex]}.Where(key=>key!="").ToList();
                if(selected.Select(key=>key.Split(':')[0]).Distinct().Count()!=selected.Count){hint.Text="两个排序条件请选择不同字段。";return;}
                completionSort=selected;settingCompletionSort=true;try{prioritySort.Checked=selected.Count>0;}finally{settingCompletionSort=false;}SaveSortPreference();dialog.DialogResult=DialogResult.OK;dialog.Close();
            };
            if(dialog.ShowDialog(this)==DialogResult.OK){var refresh=RefreshCompletionView();ObserveCompletionRefresh(refresh);}
        }
    }
    async void ObserveCompletionRefresh(Task refresh) {try{await refresh;}catch(Exception error){Error(error);}}
    ToolStripMenuItem CreateProgressMenu() {
        var menu=new ToolStripMenuItem("设置完成度");
        for(int value=0;value<=100;value+=10){int percent=value;var item=new ToolStripMenuItem(value+"%"){Tag=value};item.Click+=async delegate{await ApplySelectedProgress(percent);};menu.DropDownItems.Add(item);}
        menu.DropDownOpening+=delegate{var selection=SelectedActionNodes();menu.Text=selection.Count>1?"批量设置完成度":"设置完成度";foreach(ToolStripMenuItem item in menu.DropDownItems){item.Checked=selection.Count==1 && NodeManualCompletion(selection[0])==(int)item.Tag;item.ToolTipText="勾选完成时显示 100%；取消勾选恢复此填写值。";}};
        return menu;
    }
    int NodeManualCompletion(TreeNode node){var leaf=node.Tag as OutstandingLeaf;return leaf!=null?leaf.Completion:node.Tag is long&&taskCache.ContainsKey((long)node.Tag)?TaskManualCompletion(taskCache[(long)node.Tag]):0;}
    async Task ApplySelectedProgress(int percent) {
        if(busy||closing)return;var selected=SelectedActionNodes();if(selected.Count==0)return;
        SetBusy(true);timer.Stop();Point viewport=tasks.NativeScrollPosition();var failures=new List<string>();int saved=0;
        try {
            foreach(var node in selected.Where(node=>node.Tag is long))try{await Api("PATCH","/tasks/"+(long)node.Tag,new{percent_done=percent/100.0});saved++;}catch(Exception error){failures.Add(node.Text+"："+error.Message);}
            foreach(var group in selected.Where(node=>node.Tag is OutstandingLeaf).GroupBy(node=>((OutstandingLeaf)node.Tag).TaskId))try {
                var shared=ReadShared(await ReadHistory(group.Key));int count=0;
                foreach(var node in group){var leaf=(OutstandingLeaf)node.Tag;var item=shared.Items.FirstOrDefault(candidate=>candidate.Id==leaf.Id);if(item==null){failures.Add(node.Text+"：已被移动或移除，请刷新后重试。");continue;}item.Completion=percent;count++;}
                if(count>0){await WriteShared(group.Key,shared);saved+=count;}
            }catch(Exception error){failures.Add(error.Message);}
            if(saved>0)await LoadTasks();tasks.RestoreNativeScroll(viewport);status.Text="已保存 "+saved+" 项的填写完成度。勾选完成的条目仍显示 100%。";
            if(failures.Count>0)MessageBox.Show(this,String.Join("\r\n",failures),"部分完成度未保存",MessageBoxButtons.OK,MessageBoxIcon.Warning);
        }catch(Exception error){Error(error);}finally{SetBusy(false);timer.Start();}
    }
}
