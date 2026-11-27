// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Diagnostics;
using System.IO;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

// Every queued UI operation must recheck its target after the message loop resumes.
internal static class FloatingUi {
    internal static bool Post(Control target,Action action) {
        if(target==null || target.IsDisposed || target.Disposing || !target.IsHandleCreated)return false;
        try {
            target.BeginInvoke(new Action(delegate {
                if(target.IsDisposed || target.Disposing || !target.IsHandleCreated)return;
                try {action();}catch(Exception error){FloatingRuntimeErrors.Report(error,"延迟界面操作");}
            }));
            return true;
        } catch(ObjectDisposedException){return false;}
        catch(InvalidOperationException){if(target.IsDisposed || target.Disposing || !target.IsHandleCreated)return false;throw;}
    }
}

// A last-resort report does not save drafts, close windows, or retry failed writes.
internal sealed class FloatingRuntimeErrors : IDisposable {
    static FloatingRuntimeErrors current;
    readonly object gate=new object();
    readonly string path;
    readonly Action<Exception> notify;
    bool reporting;
    internal FloatingRuntimeErrors(string directory,Action<Exception> notify) {
        path=Path.Combine(directory,"TaskTrace-crash.log");this.notify=notify;current=this;
        Application.ThreadException+=UiException;
        AppDomain.CurrentDomain.UnhandledException+=FatalException;
        TaskScheduler.UnobservedTaskException+=UnobservedException;
    }
    internal static void Report(Exception error,string operation) {
        var handler=current;
        if(handler!=null)handler.Record(error,operation,true);
        else Trace.TraceError("TaskTrace {0}: {1}",operation,error);
    }
    void UiException(object sender,ThreadExceptionEventArgs args){Record(args.Exception,"界面事件",true);}
    void FatalException(object sender,UnhandledExceptionEventArgs args){Record(args.ExceptionObject as Exception,"进程异常（不可恢复）",false);}
    void UnobservedException(object sender,UnobservedTaskExceptionEventArgs args){Record(args.Exception,"后台任务",false);args.SetObserved();}
    internal void Record(Exception error,string operation,bool show) {
        lock(gate) {
            if(reporting)return;reporting=true;
            try {
                try {
                    Directory.CreateDirectory(Path.GetDirectoryName(path));
                    if(File.Exists(path) && new FileInfo(path).Length>2*1024*1024) {File.Copy(path,path+".previous",true);File.Delete(path);}
                    string detail=Regex.Replace(Convert.ToString(error),"#tasktrace-local=\\S+","#tasktrace-local=[redacted]");
                    detail=Regex.Replace(detail,@"(?i)(bearer\s+|[""']?(?:refresh_token|token)[""']?\s*[:=]\s*[""']?)[^\s,""'}]+","$1[redacted]");
                    File.AppendAllText(path,DateTimeOffset.Now.ToString("o")+" · "+operation+Environment.NewLine+detail+Environment.NewLine,Encoding.UTF8);
                } catch { /* Reporting must also work when the data disk is unavailable. */ }
                if(show && notify!=null)try {notify(error);}catch { }
            } finally {reporting=false;}
        }
    }
    public void Dispose() {
        Application.ThreadException-=UiException;
        AppDomain.CurrentDomain.UnhandledException-=FatalException;
        TaskScheduler.UnobservedTaskException-=UnobservedException;
        if(Object.ReferenceEquals(current,this))current=null;
    }
}
