$ErrorActionPreference = 'Stop'

function Format-TaskTraceBytes([long]$Bytes) {
    if ($Bytes -ge 1GB) { return ('{0:N2} GB' -f ($Bytes / 1GB)) }
    if ($Bytes -ge 1MB) { return ('{0:N2} MB' -f ($Bytes / 1MB)) }
    if ($Bytes -ge 1KB) { return ('{0:N1} KB' -f ($Bytes / 1KB)) }
    return ($Bytes.ToString() + ' B')
}

function Set-TaskTraceDependencyProxy([string]$Proxy) {
    if ([string]::IsNullOrWhiteSpace($Proxy)) { return }
    if ($Proxy -eq '__TASKTRACE_DIRECT__') {
        Remove-Item Env:HTTP_PROXY,Env:HTTPS_PROXY,Env:http_proxy,Env:https_proxy -ErrorAction SilentlyContinue
        return
    }
    $env:HTTP_PROXY = $Proxy
    $env:HTTPS_PROXY = $Proxy
    $env:http_proxy = $Proxy
    $env:https_proxy = $Proxy
}

function Get-TaskTraceFrontendDependencies([string]$LockFile) {
    $dependencies = New-Object 'System.Collections.Generic.List[string]'
    $inPackages = $false
    foreach ($line in [IO.File]::ReadLines($LockFile)) {
        if ($line -eq 'packages:') { $inPackages = $true; continue }
        if ($inPackages -and $line -match '^\S') { break }
        if ($inPackages -and $line -match '^  (\S.*):\s*$') {
            $value = $matches[1].Trim()
            if (($value.StartsWith("'") -and $value.EndsWith("'")) -or ($value.StartsWith('"') -and $value.EndsWith('"'))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            if (!$dependencies.Contains($value)) { $dependencies.Add($value) }
        }
    }
    return $dependencies.ToArray()
}

function Get-TaskTraceGoDependencies([string]$GoModFile) {
    $required = New-Object 'System.Collections.Generic.List[object]'
    $replacements = @{}
    $inRequire = $false
    foreach ($rawLine in [IO.File]::ReadLines($GoModFile)) {
        $line = (($rawLine -replace '\s+//.*$', '')).Trim()
        if ($line -eq 'require (') { $inRequire = $true; continue }
        if ($inRequire -and $line -eq ')') { $inRequire = $false; continue }
        if ($inRequire -and $line -match '^(\S+)\s+(v\S+)$') {
            $required.Add([pscustomobject]@{ Path = $matches[1]; Version = $matches[2] })
            continue
        }
        if ($line -match '^require\s+(\S+)\s+(v\S+)$') {
            $required.Add([pscustomobject]@{ Path = $matches[1]; Version = $matches[2] })
            continue
        }
        if ($line -match '^replace\s+(\S+)(?:\s+v\S+)?\s+=>\s+(\S+)(?:\s+(v\S+))?$') {
            $replacements[$matches[1]] = [pscustomobject]@{ Path = $matches[2]; Version = $matches[3] }
        }
    }

    $result = New-Object 'System.Collections.Generic.List[object]'
    $seen = [Collections.Generic.HashSet[string]]::new([StringComparer]::Ordinal)
    foreach ($dependency in $required) {
        $actual = $dependency
        if ($replacements.ContainsKey($dependency.Path)) { $actual = $replacements[$dependency.Path] }
        if ([string]::IsNullOrWhiteSpace($actual.Version)) { continue }
        $id = $actual.Path + '@' + $actual.Version
        if ($seen.Add($id)) { $result.Add([pscustomobject]@{ Id = $id; Path = $actual.Path; Version = $actual.Version }) }
    }
    return $result.ToArray()
}

function Write-TaskTraceDependencyManifest([string]$RepoRoot, [string]$OutputFile) {
    $frontend = @(Get-TaskTraceFrontendDependencies (Join-Path $RepoRoot 'frontend\pnpm-lock.yaml'))
    $goModules = @(Get-TaskTraceGoDependencies (Join-Path $RepoRoot 'go.mod'))
    $lines = New-Object 'System.Collections.Generic.List[string]'
    $lines.Add('TaskTrace 构建依赖清单')
    $lines.Add('生成时间：' + [DateTime]::Now.ToString('yyyy-MM-dd HH:mm:ss'))
    $lines.Add('')
    $lines.Add('构建工具：')
    $lines.Add('  Windows 10/11 x64')
    $lines.Add('  Windows PowerShell 5.1 或 PowerShell 7')
    $lines.Add('  Node.js >= 24.0.0')
    $lines.Add('  pnpm >= 11.26.0')
    $lines.Add('  Go >= 1.27.0')
    $lines.Add('  Windows x64 GCC')
    $lines.Add('  GNU Binutils strip（通常随 GCC 提供）')
    $lines.Add('  .NET Framework 4.8 C# 编译器')
    $lines.Add('')
    $lines.Add('前端锁定依赖（' + $frontend.Count + '）：')
    foreach ($dependency in $frontend) { $lines.Add('  ' + $dependency) }
    $lines.Add('')
    $lines.Add('Go 模块（' + $goModules.Count + '）：')
    foreach ($module in $goModules) { $lines.Add('  ' + $module.Id) }
    [IO.File]::WriteAllLines($OutputFile, $lines, [Text.UTF8Encoding]::new($true))
    return [pscustomobject]@{ Frontend = $frontend; GoModules = $goModules }
}

function Install-TaskTraceFrontendDependencies([int]$LockedCount) {
    $started = @{}
    [long]$downloadedBytes = 0
    $downloadedPackages = 0
    $downloadStartedAt = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
	& pnpm install --frozen-lockfile --reporter ndjson 2>&1 | ForEach-Object {
        $line = [string]$_
        try { $event = $line | ConvertFrom-Json -ErrorAction Stop } catch { Write-Host $line; return }
        $name = [string]$event.name
        $status = [string]$event.status
        if ($name -eq 'pnpm:fetching-progress' -and $status -eq 'started') {
            $packageId = [string]$event.packageId
            $size = if ($null -ne $event.size) { [long]$event.size } else { 0L }
            $started[$packageId] = [pscustomobject]@{ Time = [long]$event.time; Size = $size }
            Write-Host ('前端正在下载：' + $packageId + $(if ($size -gt 0) { ' · ' + (Format-TaskTraceBytes $size) } else { '' })) -ForegroundColor Cyan
            return
        }
        if ($name -eq 'pnpm:progress' -and $status -eq 'fetched') {
            $packageId = [string]$event.packageId
            if ($started.ContainsKey($packageId)) {
                $entry = $started[$packageId]
                $seconds = [Math]::Max(0.001, (([long]$event.time - $entry.Time) / 1000.0))
                $downloadedPackages++
                $downloadedBytes += $entry.Size
                $totalSeconds = [Math]::Max(0.001, (([long]$event.time - $downloadStartedAt) / 1000.0))
                Write-Host ('前端下载完成 [' + $downloadedPackages + ']：' + $packageId + ' · ' + (Format-TaskTraceBytes $entry.Size) + ' · ' + (Format-TaskTraceBytes ([long]($entry.Size / $seconds))) + '/s；累计 ' + (Format-TaskTraceBytes $downloadedBytes) + '，平均 ' + (Format-TaskTraceBytes ([long]($downloadedBytes / $totalSeconds))) + '/s') -ForegroundColor Green
                $started.Remove($packageId)
            }
            return
        }
        $level = [string]$event.level
        $isWarning = $level -in @('warn', 'warning', 'error', 'fatal')
        if (!$isWarning -and $level -match '^\d+$') { $isWarning = [int]$level -ge 40 }
        if ($isWarning -or $name -eq 'pnpm:error') {
            $message = if (![string]::IsNullOrWhiteSpace([string]$event.message)) { [string]$event.message } elseif (![string]::IsNullOrWhiteSpace([string]$event.err.message)) { [string]$event.err.message } else { $line }
            Write-Host ('pnpm：' + $message) -ForegroundColor Yellow
        }
    }
    if ($LASTEXITCODE -ne 0) { throw ('Frontend dependency installation failed with exit code ' + $LASTEXITCODE) }
	if ($downloadedPackages -gt 0) { Write-Host ('前端依赖下载结束：本次下载 ' + $downloadedPackages + ' 项，共 ' + (Format-TaskTraceBytes $downloadedBytes) + '。') -ForegroundColor Green }
}

function ConvertTo-TaskTraceGoCachePart([string]$Value) {
    $builder = New-Object Text.StringBuilder
    foreach ($character in $Value.ToCharArray()) {
        if ([char]::IsUpper($character)) { [void]$builder.Append('!'); [void]$builder.Append([char]::ToLowerInvariant($character)) }
        else { [void]$builder.Append($character) }
    }
    return $builder.ToString()
}

function Get-TaskTraceGoZipPath([string]$CacheRoot, $Module) {
    $escapedPath = (ConvertTo-TaskTraceGoCachePart $Module.Path).Replace('/', [IO.Path]::DirectorySeparatorChar)
    $escapedVersion = ConvertTo-TaskTraceGoCachePart $Module.Version
    return Join-Path (Join-Path (Join-Path $CacheRoot 'cache\download') $escapedPath) ('@v\' + $escapedVersion + '.zip')
}

function Download-TaskTraceGoDependencies([object[]]$Modules) {
    $cacheRoot = ((& go env GOMODCACHE) | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or [string]::IsNullOrWhiteSpace($cacheRoot)) { throw 'Unable to locate the Go module cache.' }
    $missing = New-Object 'System.Collections.Generic.List[object]'
    foreach ($module in $Modules) {
        $zip = Get-TaskTraceGoZipPath $cacheRoot $module
        if (!(Test-Path -LiteralPath $zip -PathType Leaf)) { $missing.Add($module) }
    }
	if ($missing.Count -eq 0) { return }
	Write-Host ('Go 模块需要下载：' + $missing.Count + ' 项。')
    [long]$totalBytes = 0
    $overall = [Diagnostics.Stopwatch]::StartNew()
    for ($index = 0; $index -lt $missing.Count; $index++) {
        $module = $missing[$index]
        Write-Host ('Go 正在下载 [' + ($index + 1) + '/' + $missing.Count + ']：' + $module.Id) -ForegroundColor Cyan
        $timer = [Diagnostics.Stopwatch]::StartNew()
        $output = @()
        $exitCode = 1
        for ($attempt = 1; $attempt -le 3; $attempt++) {
            $output = @(& go mod download -json $module.Id 2>&1)
            $exitCode = $LASTEXITCODE
            if ($exitCode -eq 0) { break }
            if ($attempt -lt 3) {
                Write-Host ('Go 下载连接中断，' + (2 * $attempt) + ' 秒后重试（' + $attempt + '/3）：' + $module.Id) -ForegroundColor Yellow
                Start-Sleep -Seconds (2 * $attempt)
            }
        }
        $timer.Stop()
        if ($exitCode -ne 0) { throw ('Go module download failed: ' + $module.Id + [Environment]::NewLine + (($output | ForEach-Object { [string]$_ }) -join [Environment]::NewLine)) }
        $result = (($output | ForEach-Object { [string]$_ }) -join [Environment]::NewLine) | ConvertFrom-Json
        if (![string]::IsNullOrWhiteSpace([string]$result.Error)) { throw ('Go module download failed: ' + $module.Id + ' · ' + $result.Error) }
        $bytes = if (![string]::IsNullOrWhiteSpace([string]$result.Zip) -and (Test-Path -LiteralPath $result.Zip -PathType Leaf)) { (Get-Item -LiteralPath $result.Zip).Length } else { 0L }
        $totalBytes += $bytes
        $seconds = [Math]::Max(0.001, $timer.Elapsed.TotalSeconds)
        $averageSeconds = [Math]::Max(0.001, $overall.Elapsed.TotalSeconds)
        Write-Host ('Go 下载完成：' + $module.Id + ' · ' + (Format-TaskTraceBytes $bytes) + ' · ' + (Format-TaskTraceBytes ([long]($bytes / $seconds))) + '/s；累计 ' + (Format-TaskTraceBytes $totalBytes) + '，平均 ' + (Format-TaskTraceBytes ([long]($totalBytes / $averageSeconds))) + '/s') -ForegroundColor Green
    }
    $overall.Stop()
}
