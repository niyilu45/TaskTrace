// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Diagnostics;
using System.IO;
using System.Windows.Forms;

internal static class TaskTraceUpdater {
    [STAThread] static int Main(string[] args) {
        Application.EnableVisualStyles();
        Application.SetCompatibleTextRenderingDefault(false);
        string installRoot = args.Length >= 2 ? Path.GetFullPath(args[1]) : "";
        bool selfTest = args.Length == 3 && args[2] == "--self-test";
        try {
            if(args.Length < 2 || args.Length > 3) throw new ArgumentException("更新参数不完整。");
            string archive = Path.GetFullPath(args[0]);
            if(!File.Exists(archive)) throw new FileNotFoundException("找不到已下载的更新包。", archive);
            if(!File.Exists(Path.Combine(installRoot, "TaskTrace.exe"))) throw new InvalidOperationException("安装目录中没有 TaskTrace.exe，更新已取消。");
            using(var window = new UpdateProgressWindow(archive, installRoot, selfTest)) Application.Run(window);
            return UpdateProgressWindow.LastExitCode;
        } catch(Exception e) {
            if(!selfTest) {
                MessageBox.Show("自动更新未完成，原有程序和数据没有变化。\r\n\r\n" + e.Message, "TaskTrace · 更新失败", MessageBoxButtons.OK, MessageBoxIcon.Error);
                RestartTaskTrace(installRoot);
            }
            return 1;
        }
    }

    internal static void RestartTaskTrace(string installRoot) {
        try {
            string launcher=Path.Combine(installRoot,"TaskTrace.exe");
            if(File.Exists(launcher))Process.Start(new ProcessStartInfo(launcher){WorkingDirectory=installRoot,UseShellExecute=true});
        } catch {}
    }
}
