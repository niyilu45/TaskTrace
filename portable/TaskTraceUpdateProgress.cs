// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using System.Windows.Forms;

internal sealed class UpdateProgressWindow : Form {
    readonly string archive; readonly string installRoot; readonly bool selfTest;
    readonly Label phase=new Label(); readonly Label detail=new Label(); readonly ProgressBar progress=new ProgressBar(); readonly Button cancel=new Button();
    readonly CancellationTokenSource cancellation=new CancellationTokenSource(); bool finished;
    internal static int LastExitCode=1;

    internal UpdateProgressWindow(string archive,string installRoot,bool selfTest) {
        this.archive=archive;this.installRoot=installRoot;this.selfTest=selfTest;
        Text="TaskTrace · 安装更新";Size=new System.Drawing.Size(560,230);MinimumSize=new System.Drawing.Size(480,220);StartPosition=FormStartPosition.CenterScreen;ShowInTaskbar=true;
        var layout=new TableLayoutPanel{Dock=DockStyle.Fill,Padding=new Padding(18),ColumnCount=1,RowCount=4};
        layout.RowStyles.Add(new RowStyle(SizeType.Absolute,40));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,38));layout.RowStyles.Add(new RowStyle(SizeType.Absolute,34));layout.RowStyles.Add(new RowStyle(SizeType.Percent,100));
        phase.Dock=DockStyle.Fill;phase.Text="正在准备安全更新…";phase.Font=new System.Drawing.Font(Font,System.Drawing.FontStyle.Bold);phase.TextAlign=System.Drawing.ContentAlignment.MiddleLeft;
        detail.Dock=DockStyle.Fill;detail.Text="新版本会先解压和校验，替换失败或取消时会自动恢复原版本。";detail.TextAlign=System.Drawing.ContentAlignment.MiddleLeft;
        progress.Dock=DockStyle.Fill;progress.Style=ProgressBarStyle.Marquee;
        var actions=new FlowLayoutPanel{Dock=DockStyle.Fill,FlowDirection=FlowDirection.RightToLeft,WrapContents=false};cancel.Text="取消更新";cancel.AutoSize=true;cancel.Click+=delegate{RequestCancel();};actions.Controls.Add(cancel);
        layout.Controls.Add(phase);layout.Controls.Add(detail);layout.Controls.Add(progress);layout.Controls.Add(actions);Controls.Add(layout);CancelButton=cancel;
        Shown+=async delegate{await RunUpdate();};
    }

    async Task RunUpdate() {
        UpdateOutcome outcome;
        try {
            var reporter=new Progress<UpdateProgress>(value=>SetProgress(value));
            outcome=await Task.Run(()=>ApplyUpdate(archive,installRoot,cancellation.Token,reporter));
        } catch(Exception e) { outcome=new UpdateOutcome(false,false,"原版本恢复失败，请不要删除当前目录中的程序文件。\r\n\r\n"+e.Message); }
        finished=true;LastExitCode=outcome.Success?0:(outcome.Cancelled?2:1);
        if(outcome.Success) {
            SetProgress(new UpdateProgress("更新完成","正在重新启动 TaskTrace…",100,false));
            await Task.Delay(450);Close();
            if(!selfTest)TaskTraceUpdater.RestartTaskTrace(installRoot);
            return;
        }
        Close();
        if(!selfTest) {
            MessageBox.Show(outcome.Message,outcome.Cancelled?"TaskTrace · 已取消更新":"TaskTrace · 更新失败",MessageBoxButtons.OK,outcome.Cancelled?MessageBoxIcon.Information:MessageBoxIcon.Error);
            TaskTraceUpdater.RestartTaskTrace(installRoot);
        }
    }

    void SetProgress(UpdateProgress value) {
        if(IsDisposed)return;phase.Text=value.Phase;detail.Text=value.Detail;progress.Style=value.Indeterminate?ProgressBarStyle.Marquee:ProgressBarStyle.Continuous;if(!value.Indeterminate)progress.Value=Math.Max(0,Math.Min(100,value.Percent));
    }
    void RequestCancel(){if(finished||cancellation.IsCancellationRequested)return;cancellation.Cancel();cancel.Enabled=false;cancel.Text="正在取消…";phase.Text="正在取消并恢复原版本…";detail.Text="请不要强制关闭，恢复完成后会自动重新启动原程序。";progress.Style=ProgressBarStyle.Marquee;}
    protected override void OnFormClosing(FormClosingEventArgs e){if(!finished&&e.CloseReason==CloseReason.UserClosing){e.Cancel=true;RequestCancel();return;}base.OnFormClosing(e);}
    protected override void Dispose(bool disposing){if(disposing)cancellation.Dispose();base.Dispose(disposing);}

    static UpdateOutcome ApplyUpdate(string archive,string installRoot,CancellationToken token,IProgress<UpdateProgress> reporter) {
        string work=Path.Combine(Path.GetTempPath(),"TaskTrace-update-"+Guid.NewGuid().ToString("N"));
        string stage=Path.Combine(work,"stage"),backup=Path.Combine(work,"backup");
        var applied=new List<AppliedFile>();
        bool preserveWork=false;
        try {
            reporter.Report(new UpdateProgress("等待 TaskTrace 安全退出…","关闭完成后才会开始替换文件。",3,true));
            WaitForTaskTraceToExit(token);
            token.ThrowIfCancellationRequested();
            Directory.CreateDirectory(stage);Directory.CreateDirectory(backup);
            reporter.Report(new UpdateProgress("正在解压更新包…","当前程序尚未修改，可以安全取消。",10,true));
            ZipFile.ExtractToDirectory(archive,stage);
            string stagedLauncher=Directory.GetFiles(stage,"TaskTrace.exe",SearchOption.AllDirectories).FirstOrDefault();
            if(stagedLauncher==null)throw new InvalidDataException("更新包不是有效的 TaskTrace 免安装包。");
            string sourceRoot=Path.GetDirectoryName(stagedLauncher);
            string[] required={"TaskTrace.exe","TaskTrace-server.exe","TaskTrace-floating.exe","TaskTrace-updater.exe","Launch-TaskTrace.ps1","VERSION.txt"};
            foreach(string name in required)if(!File.Exists(Path.Combine(sourceRoot,name)))throw new InvalidDataException("更新包缺少必要文件："+name);
            var files=Directory.GetFiles(sourceRoot,"*",SearchOption.TopDirectoryOnly).Where(path=>!string.Equals(Path.GetFileName(path),"tasktrace-settings.json",StringComparison.OrdinalIgnoreCase)).ToArray();
            token.ThrowIfCancellationRequested();
            reporter.Report(new UpdateProgress("正在备份当前版本…","发生错误或取消时会从此备份自动恢复。",25,false));
            for(int i=0;i<files.Length;i++) {
                token.ThrowIfCancellationRequested();string name=Path.GetFileName(files[i]);string destination=Path.Combine(installRoot,name);bool existed=File.Exists(destination);
                if(existed)File.Copy(destination,Path.Combine(backup,name),true);
                applied.Add(new AppliedFile(destination,Path.Combine(backup,name),existed));
                reporter.Report(new UpdateProgress("正在备份当前版本…",name,25+(int)(20L*(i+1)/Math.Max(1,files.Length)),false));
            }
            token.ThrowIfCancellationRequested();
            reporter.Report(new UpdateProgress("正在安全替换程序文件…","如果现在取消，已替换的文件会全部恢复。",48,false));
            string cancelAt=Environment.GetEnvironmentVariable("TASKTRACE_UPDATER_TEST_CANCEL_AT")??"";
            for(int i=0;i<files.Length;i++) {
                token.ThrowIfCancellationRequested();string source=files[i];string name=Path.GetFileName(source);string destination=Path.Combine(installRoot,name);string pending=destination+".tasktrace-new-"+Guid.NewGuid().ToString("N")+".tmp";
                try {
                    File.Copy(source,pending,true);
                    if(File.Exists(destination))File.Replace(pending,destination,null,true);else File.Move(pending,destination);
                } finally {TryDelete(pending);}
                reporter.Report(new UpdateProgress("正在安全替换程序文件…",name,48+(int)(48L*(i+1)/Math.Max(1,files.Length)),false));
                if(cancelAt=="apply"&&i==0)throw new OperationCanceledException();
                token.ThrowIfCancellationRequested();
            }
            reporter.Report(new UpdateProgress("正在完成更新…","新版本文件已全部安装。",98,false));
            return new UpdateOutcome(true,false,"");
        } catch(OperationCanceledException) {
            Rollback(applied,reporter);
            return new UpdateOutcome(false,true,"更新已取消，原版本已完整恢复并将重新启动。");
        } catch(Exception e) {
            try {Rollback(applied,reporter);} catch(Exception rollbackError){preserveWork=true;throw new AggregateException("原版本恢复失败，备份保留在："+backup,e,rollbackError);}
            return new UpdateOutcome(false,false,"自动更新未完成，原版本已恢复并将重新启动。\r\n\r\n"+e.Message);
        } finally {if(!preserveWork)try{Directory.Delete(work,true);}catch{}}
    }

    static void Rollback(List<AppliedFile> files,IProgress<UpdateProgress> reporter) {
        if(files.Count==0)return;reporter.Report(new UpdateProgress("正在恢复原版本…","恢复期间请不要关闭更新窗口。",50,true));var errors=new List<Exception>();
        foreach(AppliedFile file in files.AsEnumerable().Reverse())try{
            if(file.Existed) {
                if(!File.Exists(file.Backup))throw new FileNotFoundException("备份文件不存在。",file.Backup);
                string pending=file.Destination+".tasktrace-restore-"+Guid.NewGuid().ToString("N")+".tmp";
                try{File.Copy(file.Backup,pending,true);if(File.Exists(file.Destination))File.Replace(pending,file.Destination,null,true);else File.Move(pending,file.Destination);}finally{TryDelete(pending);}
            } else TryDelete(file.Destination);
        }catch(Exception e){errors.Add(e);}
        if(errors.Count>0)throw new AggregateException("无法完整恢复原版本。",errors);
    }

    static void WaitForTaskTraceToExit(CancellationToken token) {
        DateTime deadline=DateTime.UtcNow.AddSeconds(60);string[] names={"TaskTrace","TaskTrace-server","TaskTrace-floating"};
        while(DateTime.UtcNow<deadline){token.ThrowIfCancellationRequested();bool running=false;foreach(string name in names)foreach(Process process in Process.GetProcessesByName(name))try{if(process.Id!=Process.GetCurrentProcess().Id&&!process.HasExited)running=true;}finally{process.Dispose();}if(!running)return;Thread.Sleep(250);}
        throw new TimeoutException("TaskTrace 仍在运行。请从系统托盘退出后重新检查更新。");
    }
    static void TryDelete(string path){try{if(File.Exists(path))File.Delete(path);}catch{}}

    sealed class AppliedFile{internal readonly string Destination,Backup;internal readonly bool Existed;internal AppliedFile(string destination,string backup,bool existed){Destination=destination;Backup=backup;Existed=existed;}}
    sealed class UpdateOutcome{internal readonly bool Success,Cancelled;internal readonly string Message;internal UpdateOutcome(bool success,bool cancelled,string message){Success=success;Cancelled=cancelled;Message=message;}}
    sealed class UpdateProgress{internal readonly string Phase,Detail;internal readonly int Percent;internal readonly bool Indeterminate;internal UpdateProgress(string phase,string detail,int percent,bool indeterminate){Phase=phase;Detail=detail;Percent=percent;Indeterminate=indeterminate;}}
}
