// SPDX-License-Identifier: AGPL-3.0-or-later
using System;
using System.Collections;
using System.ComponentModel;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Net;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;
using Microsoft.Win32.SafeHandles;

internal static class Launcher {
    internal sealed class LaunchPlan {
        public string DataRoot { get; set; }
        public string TeamRoot { get; set; }
        public string ConfigPath { get; set; }
        public string Url { get; set; }
        public string LockHandle { get; set; }
    }

    const string InstanceName = @"Local\TaskTrace.Desktop.SingleInstance";
    static readonly JavaScriptSerializer json = new JavaScriptSerializer();

    [STAThread] static int Main(string[] args) {
        bool selfTest = args.Length == 1 && args[0] == "--self-test";
        string root = AppDomain.CurrentDomain.BaseDirectory;
        LaunchPlan plan = null;
        string failureTitle = "启动失败";
        try {
            int result;
            using(var instance = selfTest ? null : new Mutex(false, InstanceName)) {
                bool locked = instance != null && TakeLock(instance);
                try {
                    if(!selfTest && (!locked || AnotherInstanceRunning())) {
                        MessageBox.Show("TaskTrace 已有程序正在运行（可能是旧版本）。\r\n请先从系统托盘退出，再启动此版本。", "TaskTrace · 已有程序运行", MessageBoxButtons.OK, MessageBoxIcon.Warning);
                        return 2;
                    }
                    using(var job = new ProcessJob()) {
                        int setup = PrepareWorkspace(root, selfTest, job, out plan);
                        if(setup != 0) return setup; // The bootstrap already displayed its diagnostic.
                        using(var workspaceLock = new SafeFileHandle(new IntPtr(long.Parse(plan.LockHandle, CultureInfo.InvariantCulture)), true)) {
                            if(workspaceLock.IsInvalid) throw new IOException("无法保留数据目录的使用锁。");
                            try { result = RunApplication(root, plan, selfTest, job); }
                            finally { job.Dispose(); }
                        }
                    }
                } finally { if(locked) instance.ReleaseMutex(); }
            }
            if(result == 10) {
                failureTitle = "更新失败";
                StartUpdate(root, plan.DataRoot);
                return 0;
            }
            return result;
        } catch(Exception error) {
            string details = error.ToString();
            try {
                string log = Path.Combine(plan == null ? root : plan.DataRoot, "TaskTrace-startup-error.log");
                File.WriteAllText(log, details, Encoding.UTF8);
                details += "\r\n\r\n错误日志 " + log;
            } catch(IOException) { } catch(UnauthorizedAccessException) { }
            if(!selfTest) MessageBox.Show(details, "TaskTrace · " + failureTitle, MessageBoxButtons.OK, MessageBoxIcon.Error);
            return 1;
        }
    }

    static bool TakeLock(Mutex mutex) {
        try { return mutex.WaitOne(0); }
        catch(AbandonedMutexException) { return true; }
    }

    static bool AnotherInstanceRunning() {
        int session;
        using(var current = Process.GetCurrentProcess()) session = current.SessionId;
        foreach(string name in new[] {"TaskTrace-server", "TaskTrace-server.exe~", "TaskTrace-floating"}) {
            foreach(var running in Process.GetProcessesByName(name)) {
                using(running) {
                    try { if(running.SessionId == session && !running.HasExited) return true; }
                    catch(InvalidOperationException) { }
                }
            }
        }
        return false;
    }

    static string Quote(string value) {
        return "\"" + value + (value.EndsWith("\\", StringComparison.Ordinal) ? "\\" : "") + "\"";
    }

    static ProcessStartInfo Hidden(string file, string arguments, string directory) {
        return new ProcessStartInfo(file, arguments) { UseShellExecute=false, CreateNoWindow=true, WorkingDirectory=directory };
    }

    static int PrepareWorkspace(string root, bool selfTest, ProcessJob job, out LaunchPlan plan) {
        plan = null;
        string script = Path.Combine(root, "Launch-TaskTrace.ps1");
        if(!File.Exists(script)) throw new FileNotFoundException("请完整解压程序包，缺少 Launch-TaskTrace.ps1。", script);
        string powershell = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.System), @"WindowsPowerShell\v1.0\powershell.exe");
        int parent;
        using(var current = Process.GetCurrentProcess()) parent = current.Id;
        var start = Hidden(powershell, "-NoLogo -NoProfile -STA -ExecutionPolicy Bypass -File " + Quote(script) +
            " -Floating -PrepareForLauncher " + parent.ToString(CultureInfo.InvariantCulture) +
            (selfTest ? " -FloatingSelfTest" : ""), root);
        start.RedirectStandardOutput = true;
        start.StandardOutputEncoding = Encoding.UTF8;
        using(var process = job.Start(start)) {
            string response = process.StandardOutput.ReadToEnd();
            process.WaitForExit();
            if(process.ExitCode != 0) return process.ExitCode;
            plan = json.Deserialize<LaunchPlan>(response);
            if(plan == null || !Path.IsPathRooted(plan.ConfigPath) || String.IsNullOrEmpty(plan.LockHandle))
                throw new IOException("启动脚本没有返回有效的工作区信息。");
        }
        return 0;
    }

    static ProcessStartInfo AppChild(string root, LaunchPlan plan, string file, string args) {
        var start = Hidden(Path.Combine(root, file), args, root);
        var names = new string[start.EnvironmentVariables.Count];
        start.EnvironmentVariables.Keys.CopyTo(names, 0);
        foreach(string name in names) if(name.StartsWith("VIKUNJA_", StringComparison.OrdinalIgnoreCase)) start.EnvironmentVariables.Remove(name);
        start.EnvironmentVariables["TASKTRACE_PACKAGE_ROOT"] = root;
        start.EnvironmentVariables["TASKTRACE_DATA_ROOT"] = plan.DataRoot;
        start.EnvironmentVariables["TASKTRACE_TEAM_ROOT"] = plan.TeamRoot;
        return start;
    }

    static int RunApplication(string root, LaunchPlan plan, bool selfTest, ProcessJob job) {
        var start = AppChild(root, plan, "TaskTrace-server.exe", "--config " + Quote(plan.ConfigPath));
        start.RedirectStandardOutput = start.RedirectStandardError = true;
        using(var output = File.Create(Path.Combine(plan.DataRoot, "server.log")))
        using(var errors = File.Create(Path.Combine(plan.DataRoot, "server-error.log")))
        using(var server = job.Start(start)) {
            var outputCopy = server.StandardOutput.BaseStream.CopyToAsync(output);
            var errorCopy = server.StandardError.BaseStream.CopyToAsync(errors);
            try {
                WaitForServer(server, plan);
                string args = Quote(plan.Url) + " " + Quote(Path.Combine(root, ".")) + " " + Quote(Path.Combine(plan.DataRoot, ".")) + (selfTest ? " --self-test" : "");
                using(var window = job.Start(AppChild(root, plan, "TaskTrace-floating.exe", args))) {
                    window.WaitForExit();
                    if(window.ExitCode != 0 && window.ExitCode != 10) throw new InvalidOperationException("悬浮窗异常退出。退出码 " + window.ExitCode);
                    return window.ExitCode;
                }
            } finally {
                job.Dispose();
                server.WaitForExit();
                try { Task.WaitAll(outputCopy, errorCopy); } catch(AggregateException) { }
            }
        }
    }

    static void WaitForServer(Process server, LaunchPlan plan) {
        using(var http = new StartupClient { Proxy = null }) {
            var timeout = Stopwatch.StartNew();
            while(timeout.Elapsed < TimeSpan.FromSeconds(60)) {
                if(server.HasExited) throw new InvalidOperationException("后台服务启动失败。请查看 " + Path.Combine(plan.DataRoot, "server-error.log"));
                try {
                    string info = http.DownloadString(plan.Url + "/api/v1/info");
                    var values = json.DeserializeObject(info) as IDictionary;
                    if(values != null && !String.IsNullOrEmpty(values["version"] as string)) return;
                } catch(WebException) { } catch(ArgumentException) { }
                Thread.Sleep(500);
            }
        }
        throw new TimeoutException("后台服务在 60 秒内未就绪。请查看 " + Path.Combine(plan.DataRoot, "server.log"));
    }

    sealed class StartupClient : WebClient {
        protected override WebRequest GetWebRequest(Uri address) {
            var request = base.GetWebRequest(address);
            request.Timeout = 1000;
            return request;
        }
    }

    static void StartUpdate(string root, string dataRoot) {
        string readyFile = Path.Combine(dataRoot, "update-ready.json");
        var ready = json.DeserializeObject(File.ReadAllText(readyFile)) as IDictionary;
        string temporary = Path.GetTempPath();
        string updater = Path.Combine(temporary, "TaskTrace-updater-" + Guid.NewGuid().ToString("N") + ".exe");
        File.Copy(Path.Combine(root, "TaskTrace-updater.exe"), updater);
        using(var process = Process.Start(Hidden(updater, Quote((string)ready["archive"]) + " " + Quote((string)ready["install_root"]), temporary))) {
            if(process == null) throw new IOException("无法启动更新程序。");
        }
        try { File.Delete(readyFile); } catch(IOException) { } catch(UnauthorizedAccessException) { }
    }
}

// A single native supervisor owns the lifetime of bootstrap, server and UI.
internal sealed class ProcessJob : IDisposable {
    [StructLayout(LayoutKind.Sequential)] struct Basic {
        public long ProcessTime, JobTime;
        public uint Flags;
        public UIntPtr MinWorkingSet, MaxWorkingSet;
        public uint ActiveProcesses;
        public UIntPtr Affinity;
        public uint Priority, Scheduling;
    }
    [StructLayout(LayoutKind.Sequential)] struct Io {
        public ulong ReadOps, WriteOps, OtherOps, ReadBytes, WriteBytes, OtherBytes;
    }
    [StructLayout(LayoutKind.Sequential)] struct Extended {
        public Basic Basic;
        public Io Io;
        public UIntPtr ProcessMemory, JobMemory, PeakProcessMemory, PeakJobMemory;
    }
    [DllImport("kernel32.dll", SetLastError=true)]
    static extern SafeFileHandle CreateJobObject(IntPtr attributes, IntPtr name);
    [DllImport("kernel32.dll", SetLastError=true)]
    static extern bool SetInformationJobObject(SafeFileHandle job, int kind, ref Extended info, uint length);
    [DllImport("kernel32.dll", SetLastError=true)]
    static extern bool AssignProcessToJobObject(SafeFileHandle job, IntPtr process);

    readonly SafeFileHandle handle;
    internal ProcessJob() {
        handle = CreateJobObject(IntPtr.Zero, IntPtr.Zero);
        if(handle.IsInvalid) throw new Win32Exception();
        var info = new Extended();
        info.Basic.Flags = 0x2000; // JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
        if(!SetInformationJobObject(handle, 9, ref info, (uint)Marshal.SizeOf(info))) {
            int error = Marshal.GetLastWin32Error();
            handle.Dispose();
            throw new Win32Exception(error);
        }
    }

    internal Process Start(ProcessStartInfo start) {
        var process = Process.Start(start);
        if(process == null) throw new IOException("Windows 未能启动 " + Path.GetFileName(start.FileName));
        try {
            if(!AssignProcessToJobObject(handle, process.Handle)) {
                int error = Marshal.GetLastWin32Error();
                if(!process.HasExited) throw new Win32Exception(error);
            }
            return process;
        } catch {
            try { if(!process.HasExited) process.Kill(); } catch(InvalidOperationException) { }
            process.Dispose();
            throw;
        }
    }

    public void Dispose() { handle.Dispose(); }
}
