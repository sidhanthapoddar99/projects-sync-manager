# Projects Sync Manager (PSM) — Windows Installer & Runner
# Downloads the latest binary once, caches it in TEMP, and runs it.
#
# Usage:
#   irm https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.ps1 | iex
#   # Or with arguments:
#   $env:PSM_ARGS="-d C:\projects"; irm https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.ps1 | iex

$ErrorActionPreference = "Stop"

$repo = "sidhanthapoddar99/projects-sync-manager"
$cacheDir = Join-Path $env:TEMP "psm-cache"
$binaryName = "psm-windows-amd64.exe"
$binaryPath = Join-Path $cacheDir $binaryName
$versionFile = Join-Path $cacheDir "version"

# --- Get latest release version ---
function Get-LatestVersion {
    try {
        $response = Invoke-WebRequest -Uri "https://github.com/$repo/releases/latest" -MaximumRedirection 0 -ErrorAction SilentlyContinue
    } catch {
        $redirect = $_.Exception.Response.Headers.Location
        if ($redirect) {
            return ($redirect -split "/tag/")[-1]
        }
    }
    # Fallback: use API
    try {
        $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest"
        return $release.tag_name
    } catch {
        return $null
    }
}

# --- Main ---
Write-Host "PSM - Detecting windows/amd64"

$latest = Get-LatestVersion

if (-not $latest) {
    Write-Host "Warning: Could not fetch latest version from GitHub" -ForegroundColor Yellow
    if (Test-Path $binaryPath) {
        Write-Host "Using cached binary"
    } else {
        Write-Host "Error: No cached binary and cannot reach GitHub" -ForegroundColor Red
        exit 1
    }
} elseif ((Test-Path $binaryPath) -and (Test-Path $versionFile) -and ((Get-Content $versionFile) -eq $latest)) {
    Write-Host "Already up to date ($latest)"
} else {
    $url = "https://github.com/$repo/releases/download/$latest/$binaryName"
    Write-Host "Downloading PSM $latest..."

    if (-not (Test-Path $cacheDir)) {
        New-Item -ItemType Directory -Path $cacheDir | Out-Null
    }

    Invoke-WebRequest -Uri $url -OutFile $binaryPath
    Set-Content -Path $versionFile -Value $latest
    Write-Host "Cached at $binaryPath"
}

Write-Host "---"

# Run with arguments
$psmArgs = @()
if ($env:PSM_ARGS) {
    $psmArgs = $env:PSM_ARGS -split " "
}
$psmArgs += $args

& $binaryPath @psmArgs
