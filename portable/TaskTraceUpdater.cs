// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Diagnostics;
using System.IO;
using System.IO.Compression;
using System.Linq;
using System.Threading;
using System.Windows.Forms;

internal static class TaskTraceUpdater {
    [STAThread] static int Main(string[] args) {
        Application.EnableVisualStyles();
        try {
            if(args.Length < 2 || args.Length > 3) throw new ArgumentException("更新参数不完整。");
            bool selfTest = args.Length == 3 && args[2] == "--self-test";
            string archive = Path.GetFullPath(args[0]);
            string installRoot = Path.GetFullPath(args[1]);
            if(!File.Exists(archive)) throw new FileNotFoundException("找不到已下载的更新包。", archive);
            if(!File.Exists(Path.Combine(installRoot, "TaskTrace.exe"))) throw new InvalidOperationException("安装目录中没有 TaskTrace.exe，更新已取消。");
            WaitForTaskTraceToExit();
            string work = Path.Combine(Path.GetTempPath(), "TaskTrace-update-" + Guid.NewGuid().ToString("N"));
            string stage = Path.Combine(work, "stage");
            string backup = Path.Combine(work, "backup");
            Directory.CreateDirectory(stage); Directory.CreateDirectory(backup);
            try {
                ZipFile.ExtractToDirectory(archive, stage);
                string stagedLauncher = Directory.GetFiles(stage, "TaskTrace.exe", SearchOption.AllDirectories).FirstOrDefault();
                if(stagedLauncher == null) throw new InvalidDataException("更新包不是有效的 TaskTrace 免安装包。");
                string sourceRoot = Path.GetDirectoryName(stagedLauncher);
                var copied = Directory.GetFiles(sourceRoot, "*", SearchOption.TopDirectoryOnly);
                foreach(string source in copied) {
                    string name = Path.GetFileName(source);
                    if(string.Equals(name, "tasktrace-settings.json", StringComparison.OrdinalIgnoreCase)) continue;
                    string destination = Path.Combine(installRoot, name);
                    if(File.Exists(destination)) File.Copy(destination, Path.Combine(backup, name), true);
                }
                try {
                    foreach(string source in copied) {
                        string name = Path.GetFileName(source);
                        if(string.Equals(name, "tasktrace-settings.json", StringComparison.OrdinalIgnoreCase)) continue;
                        File.Copy(source, Path.Combine(installRoot, name), true);
                    }
                } catch {
                    foreach(string saved in Directory.GetFiles(backup)) File.Copy(saved, Path.Combine(installRoot, Path.GetFileName(saved)), true);
                    throw;
                }
            } finally { try { Directory.Delete(work, true); } catch {} }
            if(!selfTest) Process.Start(new ProcessStartInfo(Path.Combine(installRoot, "TaskTrace.exe")) { WorkingDirectory = installRoot, UseShellExecute = true });
            return 0;
        } catch(Exception e) {
            MessageBox.Show("自动更新未完成，原有数据不受影响。\r\n\r\n" + e.Message, "TaskTrace · 更新失败", MessageBoxButtons.OK, MessageBoxIcon.Error);
            return 1;
        }
    }

    static void WaitForTaskTraceToExit() {
        DateTime deadline = DateTime.UtcNow.AddSeconds(60);
        string[] names = {"TaskTrace", "TaskTrace-server", "TaskTrace-floating"};
        while(DateTime.UtcNow < deadline) {
            bool running = false;
            foreach(string name in names) foreach(Process process in Process.GetProcessesByName(name)) {
                try { if(process.Id != Process.GetCurrentProcess().Id && !process.HasExited) running = true; }
                finally { process.Dispose(); }
            }
            if(!running) return;
            Thread.Sleep(250);
        }
        throw new TimeoutException("TaskTrace 仍在运行。请从系统托盘退出后重新检查更新。");
    }
}
