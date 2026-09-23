$TaskTraceSourceFallbackVersion = 'v0.1.0-beta.14'
$TaskTraceSourceVersionFile = Join-Path $PSScriptRoot 'LATEST-RELEASE.txt'
if (Test-Path -LiteralPath $TaskTraceSourceVersionFile -PathType Leaf) {
    $bundledReleaseVersion = ([IO.File]::ReadAllText($TaskTraceSourceVersionFile)).Trim()
    if ($bundledReleaseVersion -match '^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') {
        $TaskTraceSourceFallbackVersion = $bundledReleaseVersion
    }
}

function ConvertTo-TaskTraceVersionParts([string]$Value) {
    $text = $Value.Trim().TrimStart('v', 'V')
    $metadata = $text.IndexOf('+')
    if ($metadata -ge 0) { $text = $text.Substring(0, $metadata) }
    $prerelease = @()
    $dash = $text.IndexOf('-')
    if ($dash -ge 0) {
        $prerelease = @($text.Substring($dash + 1).Split('.'))
        $text = $text.Substring(0, $dash)
    }
    $numbers = @(0, 0, 0)
    $segments = @($text.Split('.'))
    for ($index = 0; $index -lt [Math]::Min(3, $segments.Count); $index++) {
        $parsed = 0
        if ([int]::TryParse($segments[$index], [ref]$parsed)) { $numbers[$index] = $parsed }
    }
    return [pscustomobject]@{ Numbers = $numbers; Prerelease = $prerelease }
}

function Compare-TaskTraceVersion([string]$Left, [string]$Right) {
    $leftParts = ConvertTo-TaskTraceVersionParts $Left
    $rightParts = ConvertTo-TaskTraceVersionParts $Right
    for ($index = 0; $index -lt 3; $index++) {
        if ($leftParts.Numbers[$index] -ne $rightParts.Numbers[$index]) {
            return [Math]::Sign($leftParts.Numbers[$index] - $rightParts.Numbers[$index])
        }
    }
    $leftPre = @($leftParts.Prerelease)
    $rightPre = @($rightParts.Prerelease)
    if ($leftPre.Count -eq 0 -and $rightPre.Count -eq 0) { return 0 }
    if ($leftPre.Count -eq 0) { return 1 }
    if ($rightPre.Count -eq 0) { return -1 }
    for ($index = 0; $index -lt [Math]::Max($leftPre.Count, $rightPre.Count); $index++) {
        if ($index -ge $leftPre.Count) { return -1 }
        if ($index -ge $rightPre.Count) { return 1 }
        $leftNumber = 0L
        $rightNumber = 0L
        $leftNumeric = [long]::TryParse($leftPre[$index], [ref]$leftNumber)
        $rightNumeric = [long]::TryParse($rightPre[$index], [ref]$rightNumber)
        if ($leftNumeric -and $rightNumeric -and $leftNumber -ne $rightNumber) { return [Math]::Sign($leftNumber - $rightNumber) }
        if ($leftNumeric -ne $rightNumeric) { return $(if ($leftNumeric) { -1 } else { 1 }) }
        $comparison = [string]::Compare($leftPre[$index], $rightPre[$index], [StringComparison]::OrdinalIgnoreCase)
        if ($comparison -ne 0) { return [Math]::Sign($comparison) }
    }
    return 0
}

function Select-TaskTraceLatestReleaseVersion([object[]]$Releases, [string]$FallbackVersion = $TaskTraceSourceFallbackVersion) {
    $latest = $FallbackVersion
    foreach ($release in @($Releases)) {
        if ($null -eq $release -or $release.draft) { continue }
        $tag = ([string]$release.tag_name).Trim()
        if ($tag -notmatch '^v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$') { continue }
        if ((Compare-TaskTraceVersion $tag $latest) -gt 0) { $latest = $tag }
    }
    return $latest
}

function Select-TaskTraceLatestReleaseVersionFromFeed([string]$Content, [string]$FallbackVersion = $TaskTraceSourceFallbackVersion) {
    $latest = $FallbackVersion
    foreach ($match in [regex]::Matches($Content, '(?i)/releases/tag/(?<tag>v\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)')) {
        $tag = [Uri]::UnescapeDataString($match.Groups['tag'].Value)
        if ((Compare-TaskTraceVersion $tag $latest) -gt 0) { $latest = $tag }
    }
    return $latest
}

function New-TaskTraceSourceBuildVersion([string]$ReleaseVersion, [DateTime]$BuiltAtUtc = [DateTime]::UtcNow, [string]$Commit = '') {
    $base = $ReleaseVersion.Trim()
    if (!$base.StartsWith('v', [StringComparison]::OrdinalIgnoreCase)) { $base = 'v' + $base }
    $suffix = '.source.' + $BuiltAtUtc.ToUniversalTime().ToString('yyyyMMddHHmmss')
    if ($Commit -match '^[0-9a-fA-F]{7,40}$') { $suffix += '.g' + $Commit.Substring(0, [Math]::Min(12, $Commit.Length)).ToLowerInvariant() }
    return $base + $suffix
}

function Resolve-TaskTraceSourceBuildVersion([string]$ExplicitVersion, [string]$Repository, [string]$Proxy = '') {
    if (![string]::IsNullOrWhiteSpace($ExplicitVersion)) { return $ExplicitVersion.Trim() }
    $releaseVersion = $TaskTraceSourceFallbackVersion
    $releaseLookupSucceeded = $false
    try {
        $request = @{
            Uri = ('https://api.github.com/repos/' + $Repository + '/releases?per_page=30')
            Headers = @{ 'User-Agent' = 'TaskTrace-Source-Builder/1.0'; 'Accept' = 'application/vnd.github+json' }
            TimeoutSec = 20
            ErrorAction = 'Stop'
        }
        if (![string]::IsNullOrWhiteSpace($Proxy) -and $Proxy -ne '__TASKTRACE_DIRECT__') {
            $request.Proxy = $Proxy
            $request.ProxyUseDefaultCredentials = $true
        }
        $releaseVersion = Select-TaskTraceLatestReleaseVersion @(Invoke-RestMethod @request) $releaseVersion
        $releaseLookupSucceeded = $true
    } catch {
        Write-InstallLog ('GitHub Release API lookup failed; trying the public Release feed: ' + $_.Exception.Message)
    }
    if (!$releaseLookupSucceeded) {
        try {
            $feedRequest = @{
                Uri = ('https://github.com/' + $Repository + '/releases.atom')
                Headers = @{ 'User-Agent' = 'TaskTrace-Source-Builder/1.0' }
                TimeoutSec = 20
                UseBasicParsing = $true
                ErrorAction = 'Stop'
            }
            if (![string]::IsNullOrWhiteSpace($Proxy) -and $Proxy -ne '__TASKTRACE_DIRECT__') {
                $feedRequest.Proxy = $Proxy
                $feedRequest.ProxyUseDefaultCredentials = $true
            }
            $feed = Invoke-WebRequest @feedRequest
            $releaseVersion = Select-TaskTraceLatestReleaseVersionFromFeed ([string]$feed.Content) $releaseVersion
            $releaseLookupSucceeded = $true
        } catch {
            Write-InstallLog ('GitHub Release feed lookup failed; using bundled Release baseline ' + $releaseVersion + ': ' + $_.Exception.Message)
        }
    }
    $commit = ''
    try {
        if ((Test-Path -LiteralPath (Join-Path $root '.git')) -and (Get-Command git -ErrorAction SilentlyContinue)) {
            $commit = ([string](& git -C $root rev-parse HEAD 2>$null)).Trim()
        }
    } catch { }
    return New-TaskTraceSourceBuildVersion $releaseVersion ([DateTime]::UtcNow) $commit
}
