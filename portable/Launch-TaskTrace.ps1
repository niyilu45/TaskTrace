param([switch]$NoBrowser, [int]$RunSeconds = 0, [switch]$Floating, [switch]$FloatingSelfTest, [switch]$OpenBrowser)
$ErrorActionPreference = 'Stop'
$packageRoot = $PSScriptRoot
$dataRoot = $null
$server = $null
$sessionLock = $null
$processJob = $null
$exitCode = 0
$stage = 'Read settings and data directory'
try {
    $settingsFile = Join-Path $packageRoot 'tasktrace-settings.json'
    if (!(Test-Path -LiteralPath $settingsFile)) {
        [IO.File]::WriteAllText($settingsFile, '{"dataDirectory":"data"}', [Text.UTF8Encoding]::new($false))
    }
    $settings = [IO.File]::ReadAllText($settingsFile) | ConvertFrom-Json
    if ($settings.dataDirectory -isnot [string] -or [string]::IsNullOrWhiteSpace($settings.dataDirectory)) { throw 'Invalid dataDirectory in tasktrace-settings.json.' }
    $configuredPath = [Environment]::ExpandEnvironmentVariables($settings.dataDirectory.Trim())
    if (![IO.Path]::IsPathRooted($configuredPath)) { $configuredPath = Join-Path $packageRoot $configuredPath }
    $dataRoot = [IO.Path]::GetFullPath($configuredPath)
    New-Item -ItemType Directory -Path $dataRoot -Force | Out-Null
    try {
        $sessionLock = [IO.File]::Open((Join-Path $dataRoot 'session.lock'), 'OpenOrCreate', 'ReadWrite', 'None')
    } catch {
        throw 'TaskTrace is already running from this folder. Use its existing floating window or tray icon to open the full interface.'
    }
    $binary = Join-Path $packageRoot 'TaskTrace-server.exe'
    if (!(Test-Path -LiteralPath $binary)) { throw 'TaskTrace-server.exe is missing. Extract the complete package first.' }
    # Isolate this local launch from inherited Vikunja server settings.
    Get-ChildItem Env: | Where-Object { $_.Name -like 'VIKUNJA_*' } | ForEach-Object { Remove-Item -LiteralPath ('Env:' + $_.Name) }
    $secretFile = Join-Path $dataRoot 'secret.txt'
    if (!(Test-Path -LiteralPath $secretFile)) {
        $bytes = New-Object byte[] 32
        $rng = [Security.Cryptography.RandomNumberGenerator]::Create()
        try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
        [IO.File]::WriteAllText($secretFile, ([BitConverter]::ToString($bytes).Replace('-', '').ToLowerInvariant()))
    }
    $secret = [IO.File]::ReadAllText($secretFile).Trim()
    if ($secret -notmatch '^[a-f0-9]{64}$') { throw 'Invalid data/secret.txt. Restore this file from your backup.' }
    $port = 0
    foreach ($candidate in 3456..3476) {
        $listener = [Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback, $candidate)
        try { $listener.Start(); $port = $candidate; break } catch {} finally { $listener.Stop() }
    }
    if ($port -eq 0) { throw 'No free local port between 3456 and 3476.' }
    $url = 'http://127.0.0.1:' + $port
    $rootYaml = ConvertTo-Json -InputObject ($dataRoot.Replace('\', '/')) -Compress
    $dbYaml = ConvertTo-Json -InputObject ((Join-Path $dataRoot 'tasktrace.db').Replace('\', '/')) -Compress
    $filesYaml = ConvertTo-Json -InputObject ((Join-Path $dataRoot 'files').Replace('\', '/')) -Compress
    $config = @"
service:
  interface: "127.0.0.1:$port"
  publicurl: "$url/"
  rootpath: $rootYaml
  secret: "$secret"
  timezone: "Asia/Shanghai"
  enableregistration: false
database:
  type: "sqlite"
  path: $dbYaml
files:
  basepath: $filesYaml
mailer:
  enabled: false
log:
  enabled: true
  path: "logs"
"@
    $configFile = Join-Path $dataRoot 'local-config.yml'
    [IO.File]::WriteAllText($configFile, $config, [Text.UTF8Encoding]::new($false))
    $sessionFile = Join-Path $dataRoot 'local-session.json'
    $ownerFile = Join-Path $dataRoot 'local-user-id.txt'
    $sessionArgs = @('--config', $configFile, 'tasktrace-local-session', '--output', $sessionFile)
    if (Test-Path -LiteralPath $ownerFile) {
        $sessionArgs += @('--user-id', ([IO.File]::ReadAllText($ownerFile).Trim()))
    }
    $stage = 'Prepare local workspace'
    & $binary @sessionArgs *> (Join-Path $dataRoot 'workspace-setup.log')
    if ($LASTEXITCODE -ne 0) { throw 'Cannot prepare local workspace. See data/workspace-setup.log.' }
    $localSession = [IO.File]::ReadAllText($sessionFile) | ConvertFrom-Json
    [IO.File]::WriteAllText($ownerFile, [string]$localSession.user_id)
    $launchUrl = $url + '/#tasktrace-local=' + [Uri]::EscapeDataString(([IO.File]::ReadAllText($sessionFile)))

    # A Windows job stops our hidden server even if the launch window is closed.
    Add-Type -TypeDefinition @'
using System;
using System.Runtime.InteropServices;
public sealed class TaskTraceProcessJob : IDisposable {
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
    [DllImport("kernel32.dll", CharSet=CharSet.Unicode, SetLastError=true)]
    static extern IntPtr CreateJobObject(IntPtr attributes, string name);
    [DllImport("kernel32.dll", SetLastError=true)]
    static extern bool SetInformationJobObject(IntPtr job, int kind, ref Extended info, uint length);
    [DllImport("kernel32.dll", SetLastError=true)]
    static extern bool AssignProcessToJobObject(IntPtr job, IntPtr process);
    [DllImport("kernel32.dll")] static extern bool CloseHandle(IntPtr handle);
    IntPtr handle;
    public TaskTraceProcessJob() {
        handle = CreateJobObject(IntPtr.Zero, null);
        if(handle == IntPtr.Zero) throw new System.ComponentModel.Win32Exception();
        var info = new Extended();
        info.Basic.Flags = 0x2000;
        if(!SetInformationJobObject(handle, 9, ref info, (uint)Marshal.SizeOf(info))) {
            int error = Marshal.GetLastWin32Error(); Dispose();
            throw new System.ComponentModel.Win32Exception(error);
        }
    }
    public void Attach(IntPtr process) {
        if(!AssignProcessToJobObject(handle, process)) throw new System.ComponentModel.Win32Exception();
    }
    public void Dispose() {
        if(handle != IntPtr.Zero) { CloseHandle(handle); handle = IntPtr.Zero; }
    }
}
'@
    $stage = 'Create Windows process job'
    $processJob = New-Object TaskTraceProcessJob

    $stage = 'Start local server'
    $server = Start-Process -FilePath $binary -ArgumentList @('--config', ('"' + $configFile + '"')) -WorkingDirectory $packageRoot -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $dataRoot 'server.log') -RedirectStandardError (Join-Path $dataRoot 'server-error.log')
    $processJob.Attach($server.Handle)
    Write-Host 'Starting TaskTrace...'
    $ready = $false
    $http = New-Object Net.WebClient
    $http.Proxy = $null
    try {
        for ($attempt = 0; $attempt -lt 120; $attempt++) {
            if ($server.HasExited) { throw 'Local server exited. See data/server-error.log and data/server.log.' }
            try {
                $response = $http.DownloadString($url + '/api/v1/info') | ConvertFrom-Json
                if ($response.version) { $ready = $true; break }
            } catch {}
            Start-Sleep -Milliseconds 500
        }
    } finally { $http.Dispose() }
    if (!$ready) { throw 'Local server did not become ready within 60 seconds. See data/server.log.' }
    Write-Host ('TaskTrace is ready: ' + $url)
    Write-Host ('Your database and images are saved in: ' + $dataRoot)
    if ($Floating) {
        $floatingArgs = @(('"' + $url + '"'), ('"' + $packageRoot + '"'), ('"' + ($dataRoot.TrimEnd('\') + '\.') + '"'))
        if ($FloatingSelfTest) { $floatingArgs += '--self-test' }
        elseif ($OpenBrowser -and !$NoBrowser) { $floatingArgs += '--open-browser' }
    $stage = 'Start floating window'
        $window = Start-Process -FilePath (Join-Path $packageRoot 'TaskTrace-floating.exe') -ArgumentList $floatingArgs -PassThru
        $processJob.Attach($window.Handle)
        $window.WaitForExit()
        if ($window.ExitCode -ne 0) { throw 'Floating window exited with an error.' }
    }
    elseif (!$NoBrowser) { Start-Process $launchUrl }
    if ($Floating) {}
    elseif ($RunSeconds -gt 0) { Start-Sleep -Seconds $RunSeconds }
    else { [void](Read-Host 'Keep this window open while using TaskTrace. Press Enter to stop') }
} catch {
    $failure = $_
    $details = 'Step: ' + $stage + "`r`nWindows: " + [Environment]::OSVersion + "`r`n" + $failure.Exception.ToString()
    $inner = $failure.Exception
    while ($null -ne $inner) {
        if ($inner -is [ComponentModel.Win32Exception]) { $details += "`r`nWindows error code: " + $inner.NativeErrorCode }
        $inner = $inner.InnerException
    }
    $details += "`r`n" + $failure.ScriptStackTrace
    $details = $details -replace '#tasktrace-local=\S+', '#tasktrace-local=[redacted]'
    foreach ($folder in @($dataRoot, $packageRoot, [IO.Path]::GetTempPath())) {
        if (!$folder) { continue }
        try {
            $log = Join-Path $folder 'TaskTrace-startup-error.log'
            [IO.File]::WriteAllText($log, $details, [Text.UTF8Encoding]::new($true))
            $details += "`r`n`r`nError log: " + $log
            break
        } catch {}
    }
    Write-Host $details -ForegroundColor Red
    $exitCode = 1
    if ($Floating -and !$FloatingSelfTest) {
        try {
            Add-Type -AssemblyName System.Windows.Forms
            $dialog = New-Object Windows.Forms.Form
            $dialog.Text = 'TaskTrace - 启动失败（可复制详情）'
            $dialog.Width = 760; $dialog.Height = 510; $dialog.StartPosition = 'CenterScreen'
            $box = New-Object Windows.Forms.TextBox
            $box.Multiline = $true; $box.ReadOnly = $true; $box.ScrollBars = 'Both'
            $box.WordWrap = $false; $box.Dock = 'Fill'; $box.Text = $details
            $dialog.Controls.Add($box)
            [void]$dialog.ShowDialog()
            $dialog.Dispose()
        } catch {
            try { $shell = New-Object -ComObject WScript.Shell; [void]$shell.Popup($details, 0, 'TaskTrace startup error', 16) } catch {}
        }
    }
    elseif ($RunSeconds -eq 0 -and !$FloatingSelfTest) { [void](Read-Host 'Press Enter to close') }
} finally {
    if ($null -ne $server -and !$server.HasExited) {
        Stop-Process -Id $server.Id -ErrorAction SilentlyContinue
        $server.WaitForExit()
    }
    if ($null -ne $processJob) { $processJob.Dispose() }
    if ($null -ne $sessionLock) { $sessionLock.Dispose() }
}
exit $exitCode
