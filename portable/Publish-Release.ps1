param(
    [string]$Version = 'v0.1.0-beta.10',
    [string]$Archive = 'Releases/TaskTrace-local-windows-x64.zip',
    [string]$Notes = 'Releases/RELEASE-NOTES.md'
)
$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path $PSScriptRoot -Parent
Push-Location $repoRoot
try {
    foreach ($path in @($Archive, $Notes)) {
        if (!(Test-Path -LiteralPath $path -PathType Leaf)) { throw "Missing release file: $path" }
    }
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $Archive).Hash.ToLowerInvariant()
    ($hash + '  ' + [IO.Path]::GetFileName($Archive)) | Set-Content -LiteralPath 'Releases/SHA256SUMS.txt' -Encoding ASCII
    & gh release view $Version *> $null
    if ($LASTEXITCODE -eq 0) {
        & gh release edit $Version --title ("TaskTrace " + $Version) --notes-file $Notes
    } else {
        & gh release create $Version --title ("TaskTrace " + $Version) --notes-file $Notes
    }
    if ($LASTEXITCODE -ne 0) { throw 'Failed to create or update the GitHub release.' }
    & gh release upload $Version $Archive 'Releases/SHA256SUMS.txt' --clobber
    if ($LASTEXITCODE -ne 0) { throw 'Failed to upload release assets.' }
} finally {
    Pop-Location
}
