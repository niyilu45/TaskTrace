// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Net.Http;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;

internal sealed partial class FloatingWindow {
    readonly AsyncLocal<FloatingSaveOperation> saveOperationContext=new AsyncLocal<FloatingSaveOperation>();
    TimeSpan floatingSaveTimeout=TimeSpan.FromSeconds(45);
    sealed class FloatingSaveOperation : IDisposable {
        readonly FloatingWindow owner;
        readonly FloatingSaveOperation previous;
        readonly CancellationTokenSource cancellation;
        internal readonly CancellationToken Token;
        internal FloatingSaveOperation(FloatingWindow owner) {
            this.owner=owner;previous=owner.saveOperationContext.Value;
            cancellation=previous==null?new CancellationTokenSource():CancellationTokenSource.CreateLinkedTokenSource(previous.Token);
            Token=cancellation.Token;cancellation.CancelAfter(owner.floatingSaveTimeout);
            owner.saveOperationContext.Value=this;
        }
        internal void Cancel(){cancellation.Cancel();}
        public void Dispose(){owner.saveOperationContext.Value=previous;cancellation.Dispose();}
    }
    FloatingSaveOperation BeginFloatingSave(){return new FloatingSaveOperation(this);}
    CancellationToken SaveCancellation {get {var operation=saveOperationContext.Value;return operation==null?CancellationToken.None:operation.Token;}}

    // A local content write may need longer than a background read. The entire
    // editor save still has one deadline, including lock/auth/attachment waits.
    async Task<HttpResponseMessage> SendLocalRequest(HttpClient client,HttpRequestMessage request,TimeSpan? timeout=null) {
        var operationToken=SaveCancellation;operationToken.ThrowIfCancellationRequested();
        using(var limit=CancellationTokenSource.CreateLinkedTokenSource(operationToken)) {
            limit.CancelAfter(timeout??TimeSpan.FromSeconds(request.Method==HttpMethod.Get?10:30));
            try {return await client.SendAsync(request,limit.Token);}
            catch(OperationCanceledException) {
                if(operationToken.IsCancellationRequested)throw;
                throw new TimeoutException("等待本机服务响应超时。请检查程序或共享目录连接后重试。");
            }
        }
    }
    sealed class ReminderMutationGate {
        internal readonly SemaphoreSlim Semaphore=new SemaphoreSlim(1,1);
        internal int Users;
    }
    readonly Dictionary<long,ReminderMutationGate> reminderMutationGates=new Dictionary<long,ReminderMutationGate>();
    sealed class ReminderMutationLease : IDisposable {
        FloatingWindow owner;readonly long taskId;readonly ReminderMutationGate gate;
        internal ReminderMutationLease(FloatingWindow owner,long taskId,ReminderMutationGate gate){this.owner=owner;this.taskId=taskId;this.gate=gate;}
        public void Dispose(){if(owner==null)return;owner.ReleaseReminderMutation(taskId,gate,true);owner=null;}
    }
    void ReleaseReminderMutation(long taskId,ReminderMutationGate gate,bool acquired) {
        if(acquired)gate.Semaphore.Release();
        if(--gate.Users==0){reminderMutationGates.Remove(taskId);gate.Semaphore.Dispose();}
    }
    async Task<IDisposable> AcquireReminderMutation(long taskId) {
        ReminderMutationGate gate;
        if(!reminderMutationGates.TryGetValue(taskId,out gate))reminderMutationGates.Add(taskId,gate=new ReminderMutationGate());
        gate.Users++;
        try {
            if(!await gate.Semaphore.WaitAsync(TimeSpan.FromSeconds(15),SaveCancellation))
                throw new TimeoutException("这项任务的另一项提醒操作仍在等待响应，请稍后重试。");
            return new ReminderMutationLease(this,taskId,gate);
        } catch {ReleaseReminderMutation(taskId,gate,false);throw;}
    }
    static string FloatingRefreshFailure(Exception error) {
        return error is OperationCanceledException || error is TimeoutException
            ?"显示读取超时或已取消，不需要再次保存。":error.Message;
    }
    static string FloatingSaveFailure(Exception error) {
        if(error is OperationCanceledException || error is TimeoutException)
            return "等待保存结果超时或已取消，输入已保留。服务端可能已收到请求，请核对后重试。"+(error is TimeoutException?" "+error.Message:"");
        return error.Message;
    }
}
