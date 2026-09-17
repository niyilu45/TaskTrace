// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.Collections.Generic;
using System.Drawing;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class FloatingWindow {
    int taskLoadVersion;

    string TaskViewContext() {
        var selected=projects.SelectedItem as Project;
        return json.Serialize(new object[]{selected==null?0:selected.Id,search.Text,page,showCompleted.Checked,
            prioritySort.Checked,visiblePriorities.OrderBy(value=>value).ToArray()});
    }
    bool TaskLoadCurrent(int version,string context,bool background) {
        return version==taskLoadVersion && !closing && !IsDisposed && context==TaskViewContext() &&
            (!background || (Visible && !collapsed && !busy && !rendering && !dragging && !AutoRefreshInteractionActive()));
    }
    async Task<List<Project>> FetchProjects() {
        var result=new List<Project>();
        for(int p=1;;p++) {
            var list=await Api("GET","/projects?per_page=100&page="+p,null);
            int count=0;
            foreach(Dictionary<string,object> item in (IEnumerable)list["items"]) {
                result.Add(new Project{Id=Convert.ToInt64(item["id"]),Title=(string)item["title"]});count++;
            }
            if(count==0 || result.Count>=Convert.ToInt32(list["total"]))return result;
        }
    }
    static string RefreshNodeKey(TreeNode node) {
        if(node==null)return "";
        if(node.Tag is long)return "task:"+node.Tag;
        var leaf=node.Tag as OutstandingLeaf;
        if(leaf!=null)return "leaf:"+leaf.TaskId+":"+leaf.Id;
        return "";
    }
    TreeNode FindRefreshNode(string key) {
        if(key=="")return null;
        var pending=new Queue<TreeNode>(tasks.Nodes.Cast<TreeNode>());
        while(pending.Count>0) {
            var node=pending.Dequeue();if(RefreshNodeKey(node)==key)return node;
            foreach(TreeNode child in node.Nodes)pending.Enqueue(child);
        }
        return null;
    }
    object[] TaskTreeShape(IEnumerable<TreeNode> nodes) {
        return nodes.Where(node=>node.Tag is long).Select(node=>(object)new object[]{node.Tag,node.Text,node.Checked,
            node.ToolTipText,TaskTreeShape(node.Nodes.Cast<TreeNode>())}).ToArray();
    }
    async Task<bool> LoadTasks(bool background=false) {
        int version=++taskLoadVersion;
        string context=TaskViewContext();
        var oldProject=projects.SelectedItem as Project;
        long requestedProject=oldProject==null?preferredProjectId:oldProject.Id;
        var projectList=(projectsDirty || background)?await FetchProjects():projects.Items.Cast<Project>().ToList();
        var project=projectList.FirstOrDefault(item=>item.Id==requestedProject)??projectList.FirstOrDefault();
        var all=new Dictionary<long,Dictionary<string,object>>();
        var ordered=new List<long>();
        if(project!=null)for(int fetchPage=1;;fetchPage++) {
            var result=await Api("GET","/projects/"+project.Id+"/tasks?per_page=100&page="+fetchPage+"&sort_by=id&order_by=desc",null);
            int count=0;
            foreach(Dictionary<string,object> item in (IEnumerable)result["items"]) {
                long id=Convert.ToInt64(item["id"]);count++;
                if(!all.ContainsKey(id))ordered.Add(id);all[id]=item;
            }
            if(count==0 || fetchPage>=Convert.ToInt32(result["total_pages"]))break;
        }
        var order=project==null?new TaskOrderState():await FetchTaskOrder(project.Id,all);
        if(!TaskLoadCurrent(version,context,background))return false;
        Func<long,double> position=delegate(long id){double value;return order.Positions.TryGetValue(id,out value)&&value>0?value:Double.MaxValue;};
        ordered=prioritySort.Checked?ordered.OrderBy(id=>PriorityNumber(all[id])).ThenBy(position).ThenBy(id=>id).ToList():ordered.OrderBy(position).ThenBy(id=>id).ToList();
        var parents=new Dictionary<long,long>();
        foreach(long id in ordered) {
            object relationsValue,parentValue;
            if(!all[id].TryGetValue("related_tasks",out relationsValue))continue;
            var relations=relationsValue as Dictionary<string,object>;
            if(relations==null || !relations.TryGetValue("parenttask",out parentValue) || parentValue==null)continue;
            foreach(Dictionary<string,object> parent in (IEnumerable)parentValue) {
                long parentId=Convert.ToInt64(parent["id"]);
                if(parentId!=id && all.ContainsKey(parentId) && (!parents.ContainsKey(id) || parentId<parents[id]))parents[id]=parentId;
            }
        }
        foreach(long id in ordered) {
            var seen=new HashSet<long>();long cursor=id;
            while(parents.ContainsKey(cursor)) {
                if(!seen.Add(cursor)){parents.Remove(cursor);break;}cursor=parents[cursor];
            }
        }
        var included=new HashSet<long>();var matches=new HashSet<long>();string query=search.Text.Trim();
        foreach(long id in ordered) {
            if(!showCompleted.Checked && Convert.ToBoolean(all[id]["done"]))continue;
            if(!MatchesPriority(all[id]))continue;
            if(query.Length>0 && ((string)all[id]["title"]).IndexOf(query,StringComparison.OrdinalIgnoreCase)<0)continue;
            matches.Add(id);long cursor=id;
            while(included.Add(cursor) && parents.ContainsKey(cursor))cursor=parents[cursor];
        }
        // Prepare off-screen. No live controls change until all reads finish and the request is still current.
        var nodes=new Dictionary<long,TreeNode>();var roots=new List<TreeNode>();
        foreach(long id in ordered) {
            if(!included.Contains(id))continue;bool done=Convert.ToBoolean(all[id]["done"]);
            nodes[id]=new TreeNode((string)all[id]["title"]){Name=id.ToString(),Tag=id,Checked=done,
                ForeColor=done?Color.FromArgb(100,110,125):ForeColor,
                ToolTipText=(done?"已完成 · ":"未完成 · ")+(string)all[id]["title"]+(matches.Contains(id)?"":"（为显示匹配子任务保留的父任务）")};
        }
        foreach(long id in ordered) {
            if(!nodes.ContainsKey(id))continue;
            if(parents.ContainsKey(id) && nodes.ContainsKey(parents[id]))nodes[parents[id]].Nodes.Add(nodes[id]);else roots.Add(nodes[id]);
        }
        NumberTasks(roots,all);
        foreach(var node in roots)tasks.SyncCompletionState(node);
        page=1;
        var visibleRoots=roots.ToArray();
        var sharedLists=new Dictionary<long,SharedList>();
        var needed=new HashSet<long>();
        foreach(var node in nodes.Values) {
            var ancestor=node;while(ancestor.Parent!=null)ancestor=ancestor.Parent;
            if(visibleRoots.Contains(ancestor))needed.Add((long)node.Tag);
        }
        using(var gate=new SemaphoreSlim(4,4)) {
            await Task.WhenAll(needed.Select(async delegate(long id){
                await gate.WaitAsync();try{sharedLists[id]=ReadShared(await ReadHistory(id));}finally{gate.Release();}
            }));
        }
        if(!TaskLoadCurrent(version,context,background))return false;
        bool projectChanged=projects.Items.Count!=projectList.Count || !projects.Items.Cast<Project>().Zip(projectList,(a,b)=>a.Id==b.Id && a.Title==b.Title).All(equal=>equal) ||
            (oldProject==null?0:oldProject.Id)!=(project==null?0:project.Id);
        bool treeChanged=!background || json.Serialize(TaskTreeShape(tasks.Nodes.Cast<TreeNode>()))!=json.Serialize(TaskTreeShape(visibleRoots));
        var expansion=SimpleTaskNodes(tasks.Nodes).ToDictionary(node=>(long)node.Tag,node=>node.IsExpanded);
        string selectedKey=RefreshNodeKey(tasks.SelectedNode),topKey=RefreshNodeKey(tasks.TopNode);
        long selectedId=SelectedTaskId();
        taskCache=all;taskParents=parents;taskOrder=order.Positions;taskViewId=order.ViewId;
        if(projectChanged) {
            rendering=true;projects.BeginUpdate();
            try {
                projects.Items.Clear();projects.Items.AddRange(projectList.Cast<object>().ToArray());
                if(project!=null)projects.SelectedItem=project;
            }finally{projects.EndUpdate();rendering=false;}
        }
        projectsDirty=false;
        if(treeChanged) {
            InvalidateSimpleOutstanding();hoverTimer.Stop();hoverNode=null;progressTip.Hide(tasks);
            rendering=true;tasks.BeginUpdate();
            try {
                tasks.Nodes.Clear();tasks.Nodes.AddRange(visibleRoots);tasks.ExpandAll();
                foreach(var pair in nodes) {
                    bool expanded;
                    if(background && expansion.TryGetValue(pair.Key,out expanded)) {if(!expanded)pair.Value.Collapse();}
                    else if(query.Length==0 && collapsedTasks.Contains(pair.Key))pair.Value.Collapse();
                }
                if(nodes.ContainsKey(selectedId) && nodes[selectedId].TreeView==tasks)tasks.SelectedNode=nodes[selectedId];
                var selected=FindRefreshNode(selectedKey);if(selected!=null)tasks.SelectedNode=selected;
            }finally{tasks.EndUpdate();rendering=false;}
        }
        int previousTotal=total;total=roots.Count;
        string nextStatus=project==null?"请先在完整界面建立项目。":matches.Count==0?(PriorityFilterActive?PriorityFilterEmptyMessage:"没有匹配事项，可清空搜索或显示已完成。"):
            matches.Count+" 项 · "+total+" 个任务组";
        if(!background || treeChanged || projectChanged || previousTotal!=total || status.Text!=nextStatus) {
            if(!background || treeChanged || projectChanged)status.ForeColor=ForeColor;
            if(status.Text!=nextStatus)status.Text=nextStatus;
            if(!background || treeChanged || projectChanged)UpdateSimpleModeState();
        }
        InvalidateSimpleOutstanding();
        ApplyBackgroundOutstanding(sharedLists);
        if(background) {
            rendering=true;
            try {foreach(var node in SimpleTaskNodes(tasks.Nodes)) {
                bool expanded;
                if(expansion.TryGetValue((long)node.Tag,out expanded) && node.Nodes.Count>0 && node.IsExpanded!=expanded) {
                    if(expanded)node.Expand();else node.Collapse();
                }
            }}finally{rendering=false;}
        }
        var restoredSelection=FindRefreshNode(selectedKey);if(restoredSelection!=null && tasks.SelectedNode!=restoredSelection)tasks.SelectedNode=restoredSelection;
        var restoredTop=FindRefreshNode(topKey);if(restoredTop!=null && tasks.TopNode!=restoredTop)tasks.TopNode=restoredTop;
        return version==taskLoadVersion;
    }
}
