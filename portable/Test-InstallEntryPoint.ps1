param([string]$SourceArchive = '')
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$testRoot = Join-Path $repoRoot ('.local-build\install-entry-tests\' + [Guid]::NewGuid().ToString('N'))
$wrapperPath = Join-Path $repoRoot 'dist\Install-TaskTrace.cmd'
$cases = @([pscustomobject]@{Name='checkout'; Text=[Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes($wrapperPath))})
if ($SourceArchive) {
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead([IO.Path]::GetFullPath($SourceArchive))
    try {
        $entry = $zip.GetEntry('dist/Install-TaskTrace.cmd')
        if ($null -eq $entry) { throw 'Source archive has no installer entry.' }
        $reader = [IO.StreamReader]::new($entry.Open(), [Text.Encoding]::UTF8, $false)
        try { $cases += [pscustomobject]@{Name='source-zip'; Text=$reader.ReadToEnd()} } finally { $reader.Dispose() }
    } finally { $zip.Dispose() }
}
$stub = @'
param([switch]$Interactive,[string]$InstallDirectory,[int]$TestExitCode)
[IO.File]::WriteAllText((Join-Path $PSScriptRoot 'engine-called.txt'), $InstallDirectory)
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
Write-Output 'ENGINE_REACHED'
exit $TestExitCode
'@
foreach ($case in $cases) {
    $ending = $case.Name
    $text = $case.Text
    if (($text.Length -gt 0 -and $text[0] -eq [char]0xfeff) -or $text -match '(?<!\r)\n|\r(?!\n)') { throw ('CMD requires CRLF without BOM: ' + $ending) }
    $fixture = Join-Path $testRoot ($ending + ' source [copy] 中文')
    [void][IO.Directory]::CreateDirectory($fixture)
    [IO.File]::WriteAllText((Join-Path $fixture 'Install-TaskTrace.cmd'),$text,[Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText((Join-Path $fixture 'Install-TaskTrace-Engine.ps1'),$stub,[Text.UTF8Encoding]::new($true))
    foreach ($code in @(0,1,1223)) {
        $marker = Join-Path $fixture 'engine-called.txt'
        if (Test-Path -LiteralPath $marker) { Remove-Item -LiteralPath $marker }
        $target = Join-Path $fixture 'target [install] with spaces'
        $start = [Diagnostics.ProcessStartInfo]::new()
        $start.FileName = $env:ComSpec
        $start.Arguments = '/d /c ""' + (Join-Path $fixture 'Install-TaskTrace.cmd') + '" -InstallDirectory "' + $target + '" -TestExitCode ' + $code + '"'
        $start.UseShellExecute = $false
        $start.CreateNoWindow = $true
        $start.RedirectStandardOutput = $true
        $start.RedirectStandardError = $true
        $start.RedirectStandardInput = $true
        $start.StandardOutputEncoding = [Text.UTF8Encoding]::new($false)
        $start.StandardErrorEncoding = [Text.UTF8Encoding]::new($false)
        $process = [Diagnostics.Process]::Start($start)
        try {
            $stdout = $process.StandardOutput.ReadToEndAsync()
            $stderr = $process.StandardError.ReadToEndAsync()
            if ($code -ne 1223) { $process.StandardInput.WriteLine('x'); $process.StandardInput.Close() }
            if (!$process.WaitForExit(15000)) { $process.Kill(); throw ('Installer entry did not exit: ' + $ending + '/' + $code) }
            $output = $stdout.Result
            $errors = $stderr.Result
            [IO.File]::WriteAllText((Join-Path $fixture ($code.ToString()+'.log')), $output + $errors, [Text.UTF8Encoding]::new($false))
            if ($process.ExitCode -ne $code -or $errors -or !$output.Contains('ENGINE_REACHED')) { throw ('Entry failed: ' + $ending + '/' + $code + ', exit=' + $process.ExitCode + "`n" + $output + $errors) }
            if ([IO.File]::ReadAllText($marker) -ne $target) { throw 'Install directory argument was damaged.' }
            if ($code -eq 0 -and !$output.Contains('安装完成，可以关闭本窗口。')) { throw 'Missing completion message.' }
            if ($code -ne 0 -and $output.Contains('安装完成，可以关闭本窗口。')) { throw 'Failed/cancelled install reported success.' }
            if ($code -eq 1223 -and $output -match 'Press any key|Build failed|Installation completed|按任意键|安装完成') { throw 'Cancellation reached completion output or pause.' }
            Write-Output ('PASS: ' + $ending + ' entry, exit ' + $code + ', quoted install path and no malformed commands.')
        } finally {
            if (!$process.HasExited) { $process.Kill() }
            $process.Dispose()
        }
    }
}
Write-Output ('Results: ' + $testRoot)
