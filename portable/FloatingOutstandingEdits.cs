// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading;

internal sealed partial class FloatingWindow {
    readonly Dictionary<long,SemaphoreSlim> outstandingWriteGates=new Dictionary<long,SemaphoreSlim>();
    SemaphoreSlim OutstandingWriteGate(long taskId){SemaphoreSlim gate;if(!outstandingWriteGates.TryGetValue(taskId,out gate)){gate=new SemaphoreSlim(1,1);outstandingWriteGates.Add(taskId,gate);}return gate;}
    static bool SameOutstandingItem(PendingItem a,PendingItem b){return SameProgressEditorContent(a.Html,b.Html) && SameProgressEditorContent(a.NoteHtml,b.NoteHtml) && a.Priority==b.Priority && a.Completion==b.Completion && a.Done==b.Done && a.CompletedAt==b.CompletedAt && a.ReminderAt==b.ReminderAt && a.RepeatRule==b.RepeatRule;}
    static T MergeOutstandingField<T>(T original,T proposed,T latest){if(EqualityComparer<T>.Default.Equals(original,proposed))return latest;if(!EqualityComparer<T>.Default.Equals(original,latest) && !EqualityComparer<T>.Default.Equals(proposed,latest))throw new Exception("这条遗留事项的同一内容已在其他窗口修改，当前输入已保留，请重新读取后合并。");return proposed;}
    static string MergeOutstandingHtml(string original,string proposed,string latest){if(SameProgressEditorContent(original,proposed))return latest;if(!SameProgressEditorContent(original,latest) && !SameProgressEditorContent(proposed,latest))throw new Exception("这条遗留事项的正文或备注已在其他窗口修改，当前输入已保留，请重新读取后合并。");return proposed;}
    // Each editor submits only its delta; an older window must not replace another window's saved list.
    static List<PendingItem> MergeOutstandingEdits(List<PendingItem> original,List<PendingItem> proposed,List<PendingItem> latest) {
        var result=latest.Select(item=>item.Copy()).ToList();
        foreach(var before in original) {
            var desired=proposed.FirstOrDefault(item=>item.Id==before.Id);var current=result.FirstOrDefault(item=>item.Id==before.Id);
            if(desired==null){if(current!=null){if(!SameOutstandingItem(before,current))throw new Exception("待删除的遗留事项已在其他窗口修改，请重新读取后再删除。");result.Remove(current);}continue;}
            if(current==null){if(!SameOutstandingItem(before,desired))throw new Exception("这条遗留事项已被删除或移动，当前输入已保留。");continue;}
            current.Html=MergeOutstandingHtml(before.Html,desired.Html,current.Html);current.NoteHtml=MergeOutstandingHtml(before.NoteHtml,desired.NoteHtml,current.NoteHtml);
            current.Priority=MergeOutstandingField(before.Priority,desired.Priority,current.Priority);current.Completion=MergeOutstandingField(before.Completion,desired.Completion,current.Completion);current.Done=MergeOutstandingField(before.Done,desired.Done,current.Done);
            current.CompletedAt=MergeOutstandingField(before.CompletedAt,desired.CompletedAt,current.CompletedAt);current.ReminderAt=MergeOutstandingField(before.ReminderAt,desired.ReminderAt,current.ReminderAt);current.RepeatRule=MergeOutstandingField(before.RepeatRule,desired.RepeatRule,current.RepeatRule);
        }
        foreach(var added in proposed.Where(item=>!original.Any(before=>before.Id==item.Id))) {
            var current=result.FirstOrDefault(item=>item.Id==added.Id);
            if(current==null)result.Add(added.Copy());else if(!SameOutstandingItem(added,current))throw new Exception("同一遗留事项已在其他窗口保存，当前输入已保留，请重新读取后合并。");
        }
        return result;
    }
}
