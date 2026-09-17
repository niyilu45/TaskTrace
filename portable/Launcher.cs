// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Diagnostics;
using System.IO;
using System.Windows.Forms;

internal static class Launcher {
    [STAThread] static int Main(string[] args) {
        try {
            string root = AppDomain.CurrentDomain.BaseDirectory;
            string script = Path.Combine(root, "Launch-TaskTrace.ps1");
            if(!File.Exists(script)) throw new FileNotFoundException("请完整解压程序包，缺少 Launch-TaskTrace.ps1。", script);
            string powershell = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System), @"WindowsPowerShell\v1.0\powershell.exe");
            string options = args.Length == 1 && args[0] == "--self-test" ? " -Floating -FloatingSelfTest" : " -Floating -OpenBrowser";
            var start = new ProcessStartInfo(powershell, "-NoLogo -NoProfile -STA -ExecutionPolicy Bypass -File \"" + script + "\"" + options) {
                UseShellExecute = false, CreateNoWindow = true, WorkingDirectory = root
            };
            using(var process = Process.Start(start)) {
                if(process == null) throw new Exception("Windows 未能启动 TaskTrace。");
                process.WaitForExit();
                return process.ExitCode;
            }
        } catch(Exception e) {
            MessageBox.Show(e.ToString(), "TaskTrace · 启动失败", MessageBoxButtons.OK, MessageBoxIcon.Error);
            return 1;
        }
    }
}
