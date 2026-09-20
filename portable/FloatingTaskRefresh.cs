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
        return json.Serialize(new object[]{selected==null?0:selected.Id,search.Text,page,showCompleted.Checked,singleLine.Checked,
            prioritySort.Checked,visiblePriorities.OrderBy(value=>value).ToArray(),grayCompleted,strikeCompleted,completedHideDelayMinutes});
    }
    static string OutstandingCompletionKey(long taskId,string itemId) {return taskId+":"+itemId;}
    void RememberTaskCompletion(long id,bool done) {
        if(done && completedHideDelayMinutes>0)recentlyCompletedTasks[id]=DateTime.UtcNow;
        else recentlyCompletedTasks.Remove(id);
    }
    void RememberOutstandingCompletion(long taskId,string itemId,bool done) {
        string key=OutstandingCompletionKey(taskId,itemId);
        if(done && completedHideDelayMinutes>0)recentlyCompletedOutstanding[key]=DateTime.UtcNow;
        else recentlyCompletedOutstanding.Remove(key);
    }
    bool KeepRecentlyCompleted(DateTime completed,ref DateTime nextRefreshUtc) {
        if(completedHideDelayMinutes<=0)return false;
        DateTime deadline=completed.AddMinutes(completedHideDelayMinutes);
        if(deadline<=DateTime.UtcNow)return false;
        if(deadline<nextRefreshUtc)nextRefreshUtc=deadline;
        return true;
    }
    bool HideCompletedTask(long id,bool done,ref DateTime nextRefreshUtc) {
        if(!done){recentlyCompletedTasks.Remove(id);return false;}
        if(showCompleted.Checked)return false;
        DateTime completed;
        if(recentlyCompletedTasks.TryGetValue(id,out completed) && KeepRecentlyCompleted(completed,ref nextRefreshUtc))return false;
        recentlyCompletedTasks.Remove(id);return true;
    }
    bool HideCompletedOutstanding(long taskId,PendingItem item,ref DateTime nextRefreshUtc) {
        string key=OutstandingCompletionKey(taskId,item.Id);
        if(!item.Done){recentlyCompletedOutstanding.Remove(key);return false;}
        if(showCompleted.Checked)return false;
        DateTime completed;
        if(recentlyCompletedOutstanding.TryGetValue(key,out completed) && KeepRecentlyCompleted(completed,ref nextRefreshUtc))return false;
        recentlyCompletedOutstanding.Remove(key);return true;
    }
    SharedList FilterCompletedOutstanding(long taskId,SharedList source,ref DateTime nextRefreshUtc) {
        if(source==null)return null;
        var filtered=new SharedList {CommentId=source.CommentId};
        for(int index=0;index<source.Items.Count;index++) {var item=source.Items[index];if(item.Number<=0)item.Number=index+1;if(!HideCompletedOutstanding(taskId,item,ref nextRefreshUtc))filtered.Items.Add(item);}
        return filtered;
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
        return nodes.Where(node=>node.Tag is long || (singleLine.Checked && node.Tag is OutstandingLeaf)).Select(node=>{
            var leaf=node.Tag as OutstandingLeaf;
            var task=node as TaskNode;
            return leaf==null?(object)new object[]{"task",node.Tag,node.Text,node.Checked,node.ToolTipText,task==null?0:task.ReminderCount,TaskTreeShape(node.Nodes.Cast<TreeNode>())}:
                (object)new object[]{"outstanding",leaf.TaskId,leaf.Id,node.Text,leaf.Done,leaf.Priority,leaf.Html,leaf.ReminderAt};
        }).ToArray();
    }
    static void CollectFlatTaskNodes(TreeNode node,List<TreeNode> flat) {
        var children=node.Nodes.Cast<TreeNode>().Where(child=>child.Tag is long).ToList();
        flat.Add(node);foreach(var child in children)CollectFlatTaskNodes(child,flat);
    }
    List<TreeNode> FlattenTaskNodes(List<TreeNode> roots,Dictionary<long,Dictionary<string,object>> all,Dictionary<long,long> parents,Dictionary<long,SharedList> sharedLists) {
        var flat=new List<TreeNode>();foreach(var root in roots)CollectFlatTaskNodes(root,flat);
        var hasTaskChildren=flat.ToDictionary(node=>(long)node.Tag,node=>node.Nodes.Cast<TreeNode>().Any(child=>child.Tag is long));
        foreach(var node in flat)if(node.Parent!=null)node.Remove();
        var visible=new List<TreeNode>();
        foreach(var node in flat) {
            long id=(long)node.Tag;SharedList shared;sharedLists.TryGetValue(id,out shared);
            bool hasPending=shared!=null && shared.Items.Any(item=>!item.Done);
            if(!hasTaskChildren[id] && !hasPending)visible.Add(node);
            if(shared==null)continue;
            var titles=new List<string>();long cursor=id;
            while(all.ContainsKey(cursor)) {titles.Add((string)all[cursor]["title"]);if(!parents.ContainsKey(cursor))break;cursor=parents[cursor];}
            string path=TaskTreeView.SingleLineSeparator+String.Join(TaskTreeView.SingleLineSeparator,titles);
            for(int index=0;index<shared.Items.Count;index++)visible.Add(CreateOutstandingNode(id,shared.Items[index],index,path));
        }
        return visible;
    }
    async Task ReadSharedLists(IEnumerable<long> ids,Dictionary<long,SharedList> destination) {
        using(var gate=new SemaphoreSlim(4,4)) {
            await Task.WhenAll(ids.Distinct().Select(async delegate(long id){
                await gate.WaitAsync();try{destination[id]=ReadShared(await ReadHistory(id));}finally{gate.Release();}
            }));
        }
    }
    async Task<bool> LoadTasks(bool background=false) {
        DateTime nextCompletionRefreshUtc=DateTime.MaxValue;
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
        var sharedLists=new Dictionary<long,SharedList>();var reminderSharedLists=new Dictionary<long,SharedList>();var candidates=new List<long>();
        foreach(long id in ordered) {
            if(HideCompletedTask(id,Convert.ToBoolean(all[id]["done"]),ref nextCompletionRefreshUtc))continue;
            if(query.Length>0 && ((string)all[id]["title"]).IndexOf(query,StringComparison.OrdinalIgnoreCase)<0)continue;
            candidates.Add(id);
        }
        if(PriorityFilterActive && visiblePriorities.Count>0){await ReadSharedLists(candidates,sharedLists);foreach(var pair in sharedLists)reminderSharedLists[pair.Key]=pair.Value;}
        if(!showCompleted.Checked)foreach(long id in sharedLists.Keys.ToArray())sharedLists[id]=FilterCompletedOutstanding(id,sharedLists[id],ref nextCompletionRefreshUtc);
        if(!TaskLoadCurrent(version,context,background))return false;
        foreach(long id in candidates) {
            SharedList shared;bool outstandingMatch=sharedLists.TryGetValue(id,out shared) && shared.Items.Any(MatchesPriority);
            if(!MatchesPriority(all[id]) && !outstandingMatch)continue;
            matches.Add(id);long cursor=id;
            while(included.Add(cursor) && parents.ContainsKey(cursor))cursor=parents[cursor];
        }
        // Prepare off-screen. No live controls change until all reads finish and the request is still current.
        var nodes=new Dictionary<long,TreeNode>();var roots=new List<TreeNode>();
        foreach(long id in ordered) {
            if(!included.Contains(id))continue;bool done=Convert.ToBoolean(all[id]["done"]);
            string taskStatus=TaskStatusValue(all[id]);
            nodes[id]=new TaskNode((string)all[id]["title"]){Name=id.ToString(),Tag=id,Checked=done,ReminderCount=TaskReminderTimes(all[id]).Count,
                ForeColor=done && grayCompleted?Color.FromArgb(100,110,125):ForeColor,
                ToolTipText=TaskStatusText(taskStatus)+" · "+(string)all[id]["title"]+(matches.Contains(id)?"":"（为显示匹配子任务或遗留事项保留的父任务）")};
        }
        foreach(long id in ordered) {
            if(!nodes.ContainsKey(id))continue;
            if(parents.ContainsKey(id) && nodes.ContainsKey(parents[id]))nodes[parents[id]].Nodes.Add(nodes[id]);else roots.Add(nodes[id]);
        }
        NumberTasks(roots,all);
        int groupCount=roots.Count;
        await ReadSharedLists(nodes.Keys.Where(id=>!sharedLists.ContainsKey(id)),sharedLists);
        if(!TaskLoadCurrent(version,context,background))return false;
        foreach(var pair in sharedLists)if(!reminderSharedLists.ContainsKey(pair.Key))reminderSharedLists[pair.Key]=pair.Value;
        UpdateReminderTargets(all,reminderSharedLists);
        foreach(var pair in nodes) {SharedList owned;reminderSharedLists.TryGetValue(pair.Key,out owned);var taskNode=pair.Value as TaskNode;if(taskNode!=null)taskNode.ReminderCount=DirectTaskReminderCount(all[pair.Key],owned);}
        if(!showCompleted.Checked)foreach(long id in sharedLists.Keys.ToArray())sharedLists[id]=FilterCompletedOutstanding(id,sharedLists[id],ref nextCompletionRefreshUtc);
        if(PriorityFilterActive)sharedLists=sharedLists.ToDictionary(pair=>pair.Key,pair=>FilterOutstandingPriorities(pair.Value));
        if(!TaskLoadCurrent(version,context,background))return false;
        if(singleLine.Checked)roots=FlattenTaskNodes(roots,all,parents,sharedLists);
        foreach(var node in roots)tasks.SyncCompletionState(node);
        page=1;
        var visibleRoots=roots.ToArray();
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
                    if(singleLine.Checked)pair.Value.Expand();
                    else if(background && expansion.TryGetValue(pair.Key,out expanded)) {if(!expanded)pair.Value.Collapse();}
                    else if(query.Length==0 && collapsedTasks.Contains(pair.Key))pair.Value.Collapse();
                }
                if(nodes.ContainsKey(selectedId) && nodes[selectedId].TreeView==tasks)tasks.SelectedNode=nodes[selectedId];
                var selected=FindRefreshNode(selectedKey);if(selected!=null)tasks.SelectedNode=selected;
            }finally{tasks.EndUpdate();rendering=false;}
        }
        int previousTotal=total;total=groupCount;
        string nextStatus=project==null?"请先在完整界面建立项目。":matches.Count==0?(PriorityFilterActive?PriorityFilterEmptyMessage:"没有匹配事项，可清空搜索或显示已完成。"):
            matches.Count+" 项 · "+total+" 个任务组";
        if(!background || treeChanged || projectChanged || previousTotal!=total || status.Text!=nextStatus) {
            if(!background || treeChanged || projectChanged)status.ForeColor=ForeColor;
            if(status.Text!=nextStatus)status.Text=nextStatus;
            if(!background || treeChanged || projectChanged)UpdateSimpleModeState();
        }
        InvalidateSimpleOutstanding();
        if(!singleLine.Checked)ApplyBackgroundOutstanding(sharedLists);
        tasks.RefreshWrappedLayout();
        if(background) {
            rendering=true;
            try {
                if(singleLine.Checked)tasks.ExpandAll();
                else foreach(var node in SimpleTaskNodes(tasks.Nodes)) {
                    bool expanded;
                    if(expansion.TryGetValue((long)node.Tag,out expanded) && node.Nodes.Count>0 && node.IsExpanded!=expanded) {
                        if(expanded)node.Expand();else node.Collapse();
                    }
                }
            }finally{rendering=false;}
        }
        var restoredSelection=FindRefreshNode(selectedKey);if(restoredSelection!=null && tasks.SelectedNode!=restoredSelection)tasks.SelectedNode=restoredSelection;
        var restoredTop=FindRefreshNode(topKey);if(restoredTop!=null && tasks.TopNode!=restoredTop)tasks.TopNode=restoredTop;
        completedHideRefreshAfterUtc=nextCompletionRefreshUtc;
        return version==taskLoadVersion;
    }
}
