$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
$buildScript = Join-Path $PSScriptRoot 'Build-Local.ps1'
$tokens = $null
$parseErrors = $null
$ast = [Management.Automation.Language.Parser]::ParseFile($buildScript, [ref]$tokens, [ref]$parseErrors)
if ($parseErrors.Count -gt 0) { throw ($parseErrors | Out-String) }
$assertExit = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq 'Assert-Exit' }, $true)
. ([ScriptBlock]::Create($assertExit.Extent.Text))
# Compile the actual install/release commands, not a separate test source list.
# The isolated output also exercises paths containing spaces and brackets.
$packageRoot = Join-Path $repoRoot ('.local-build\native-install-tests\' + [Guid]::NewGuid().ToString('N') + '\program [copy]')
[void][IO.Directory]::CreateDirectory($packageRoot)
$source = [IO.File]::ReadAllText($buildScript)
$start = $source.IndexOf('    $compiler = Join-Path')
$end = $source.IndexOf('    Copy-Item -LiteralPath', $start)
if ($start -lt 0 -or $end -le $start) { throw 'Cannot find production native build commands.' }
& ([ScriptBlock]::Create($source.Substring($start, $end - $start)))
foreach ($name in @('TaskTrace.exe', 'TaskTrace-floating.exe', 'TaskTrace-updater.exe')) {
    if (!(Test-Path -LiteralPath (Join-Path $packageRoot $name) -PathType Leaf)) { throw ('Missing binary: ' + $name) }
}
$assembly = [Reflection.Assembly]::LoadFile((Join-Path $packageRoot 'TaskTrace-floating.exe'))
foreach ($name in @('FloatingWindow', 'ProgressHistoryBrowserBridge', 'FloatingClipboard')) {
    if ($null -eq $assembly.GetType($name)) { throw ('Missing compiled feature: ' + $name) }
}
$export = $assembly.GetType('FloatingWindow').GetMethod('ExportMarkdown', [Reflection.BindingFlags]'Instance,NonPublic')
if ($null -eq $export) { throw 'Markdown export was not compiled into the floating window.' }
Write-Output 'PASS: production native build includes Markdown export, history browser, clipboard, launcher and updater.'
Write-Output ('Results: ' + $packageRoot)
