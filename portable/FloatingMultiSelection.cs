// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed partial class TaskTreeSurface {
    // Stable identities survive refreshes which replace the native TreeNode instances.
    readonly HashSet<string> selectedItems=new HashSet<string>(StringComparer.Ordinal);
    string selectionAnchor;
    bool selecting;
    internal Func<bool> ModelUpdating;
    internal bool HasModel {get{return model!=null;}}
    static string SelectionKey(TreeNode node) {
        if(node==null)return "";
        if(node.Tag is long)return "task:"+node.Tag;
        var leaf=node.Tag as FloatingWindow.OutstandingLeaf;
        return leaf==null?"":"leaf:"+leaf.TaskId+":"+leaf.Id;
    }
    internal bool IsSelected(TreeNode node){return selectedItems.Contains(SelectionKey(node));}
    internal List<TreeNode> SelectedItems {
        get {
            if(model==null)return new List<TreeNode>();
            var visible=VisibleNodes(model.Nodes).ToList();ReconcileSelection(visible);
            return visible.Where(IsSelected).ToList();
        }
    }
    void ReconcileSelection(List<TreeNode> visible) {
        if(ModelUpdating!=null&&ModelUpdating())return;
        var allowed=new HashSet<string>(visible.Select(SelectionKey),StringComparer.Ordinal);
        bool changed=selectedItems.RemoveWhere(key=>!allowed.Contains(key))>0;
        if(selectionAnchor!=null&&!allowed.Contains(selectionAnchor))selectionAnchor=null;
        if(model.SelectedNode==null||!allowed.Contains(SelectionKey(model.SelectedNode))) {
            var caret=visible.FirstOrDefault(IsSelected);if(caret!=null)SetSelectionCaret(caret);
        }
        if(changed)Invalidate();
    }
    void ModelSelectionChanged() {
        if(!selecting && (ModelUpdating==null||!ModelUpdating()) && Real(model.SelectedNode)) {
            string key=SelectionKey(model.SelectedNode);
            if(!selectedItems.Contains(key)){selectedItems.Clear();selectedItems.Add(key);selectionAnchor=key;}
        }
        Invalidate();
    }
    void SetSelectionCaret(TreeNode node) {
        selecting=true;try{model.SelectedNode=node;}finally{selecting=false;}
    }
    internal void ClearSelection(){selectedItems.Clear();selectionAnchor=null;if(model!=null)SetSelectionCaret(null);Invalidate();}
    internal void SelectItem(TreeNode node,Keys modifiers,MouseButtons button) {
        if(model==null||!Real(node))return;
        var visible=VisibleNodes(model.Nodes).ToList();ReconcileSelection(visible);
        if(!visible.Contains(node))return;
        string key=SelectionKey(node);bool control=(modifiers&Keys.Control)!=0,shift=(modifiers&Keys.Shift)!=0;
        if(button==MouseButtons.Right) {
            if(!selectedItems.Contains(key)){selectedItems.Clear();selectedItems.Add(key);selectionAnchor=key;}
        } else if(shift) {
            int anchor=visible.FindIndex(item=>SelectionKey(item)==selectionAnchor),target=visible.IndexOf(node);
            if(anchor<0){anchor=target;selectionAnchor=key;}
            if(!control)selectedItems.Clear();
            for(int index=Math.Min(anchor,target);index<=Math.Max(anchor,target);index++)selectedItems.Add(SelectionKey(visible[index]));
        } else if(control) {
            if(!selectedItems.Remove(key))selectedItems.Add(key);
            selectionAnchor=key;
        } else {selectedItems.Clear();selectedItems.Add(key);selectionAnchor=key;}
        SetSelectionCaret(node);Invalidate();
    }
    internal void SelectAllItems() {
        if(model==null)return;
        // The rendered tree is already filtered. Never consult the unfiltered task cache.
        var visible=VisibleNodes(model.Nodes).ToList();selectedItems.Clear();
        foreach(var node in visible)selectedItems.Add(SelectionKey(node));
        if(visible.Count>0){var caret=visible.Contains(model.SelectedNode)?model.SelectedNode:visible[0];SetSelectionCaret(caret);selectionAnchor=SelectionKey(caret);}
        else selectionAnchor=null;
        Invalidate();
    }
}

internal sealed partial class FloatingWindow {
    List<TreeNode> SelectedActionNodes() {
        if(taskSurface.HasModel)return taskSurface.SelectedItems;
        var node=tasks.SelectedNode;
        return node!=null&&(node.Tag is long||node.Tag is OutstandingLeaf)?new List<TreeNode>{node}:new List<TreeNode>();
    }
    sealed class BatchEditResult {
        public int Saved,Unchanged;
        public readonly List<string> Failures=new List<string>();
    }
    // Read one current outstanding list per owner, then change only the selected IDs.
    // A failed owner/task does not discard successful writes for other selected items.
    async Task<BatchEditResult> SaveSelectedStates(List<TreeNode> selected,bool? done,int? priority) {
        var result=new BatchEditResult();
        using(BeginUndoGroup()) {
            foreach(var node in selected.Where(item=>item.Tag is long)) {
                long id=(long)node.Tag;
                try {
                    var current=await Api("GET","/tasks/"+id,null);
                    bool wasDone=Convert.ToBoolean(current["done"]);
                    var patch=new Dictionary<string,object>();
                    if(done.HasValue&&wasDone!=done.Value)patch["done"]=done.Value;
                    if(priority.HasValue&&PriorityNumber(current)!=priority.Value)patch["priority"]=ApiPriority(priority.Value);
                    if(patch.Count==0){result.Unchanged++;continue;}
                    await Api("PATCH","/tasks/"+id,patch);
                    if(patch.ContainsKey("done"))RememberTaskCompletion(id,done.Value);
                    result.Saved++;
                }catch(Exception error){result.Failures.Add(node.Text+"："+error.Message);}
            }
            foreach(var owner in selected.Where(node=>node.Tag is OutstandingLeaf).GroupBy(node=>((OutstandingLeaf)node.Tag).TaskId)) {
                var changed=new List<PendingItem>();
                var pending=owner.ToList();
                try {
                    var shared=ReadShared(await ReadHistory(owner.Key));
                    pending.Clear();
                    foreach(var node in owner) {
                        var leaf=(OutstandingLeaf)node.Tag;var item=shared.Items.FirstOrDefault(value=>value.Id==leaf.Id);
                        if(item==null){result.Failures.Add(node.Text+"：已被移动或移除，请刷新后重试。");continue;}
                        bool changeDone=done.HasValue&&item.Done!=done.Value,changePriority=priority.HasValue&&item.Priority!=priority.Value;
                        if(!changeDone&&!changePriority){result.Unchanged++;continue;}
                        if(changeDone){item.Done=done.Value;item.CompletedAt=done.Value?DateTimeOffset.UtcNow.ToString("o"):null;}
                        if(changePriority)item.Priority=priority.Value;
                        changed.Add(item);pending.Add(node);
                    }
                    if(changed.Count==0)continue;
                    await WriteShared(owner.Key,shared);
                    if(done.HasValue)foreach(var item in changed)RememberOutstandingCompletion(owner.Key,item.Id,done.Value);
                    result.Saved+=changed.Count;
                }catch(Exception error){
                    foreach(var node in pending)result.Failures.Add(node.Text+"："+error.Message);
                }
            }
        }
        return result;
    }
    async Task ApplySelectedStates(bool? done,int? priority) {
        if(busy||closing)return;var selected=SelectedActionNodes();if(selected.Count==0)return;
        SetBusy(true);timer.Stop();
        try {
            var result=await SaveSelectedStates(selected,done,priority);
            if(result.Saved>0)await LoadTasks();
            string message="已保存 "+result.Saved+" 项"+(result.Unchanged>0?"，另有 "+result.Unchanged+" 项无需修改":"")+"。";
            status.Text=message;
            if(result.Failures.Count>0)MessageBox.Show(this,message+"\r\n以下条目未保存：\r\n"+String.Join("\r\n",result.Failures),"批量修改结果",MessageBoxButtons.OK,MessageBoxIcon.Warning);
        }catch(Exception error){Error(error);}finally{SetBusy(false);timer.Start();}
    }
    ToolStripMenuItem CreateBatchPriorityMenu() {
        var menu=new ToolStripMenuItem("批量设置优先级");
        for(int value=0;value<=9;value++){int priority=value;menu.DropDownItems.Add(PriorityChoiceText(value),null,async delegate{await ApplySelectedStates(null,priority);});}
        return menu;
    }
    ToolStripMenuItem CreateCompletionMenu() {
        var menu=new ToolStripMenuItem("设置完成状态");
        menu.DropDownItems.Add("标记完成",null,async delegate{await ApplySelectedStates(true,null);});
        menu.DropDownItems.Add("恢复未完成",null,async delegate{await ApplySelectedStates(false,null);});
        return menu;
    }
}
