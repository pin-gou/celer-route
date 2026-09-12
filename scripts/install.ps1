# celer-route single-binary installer for Windows.
#
# Downloads the celer-route-http.exe gateway binary for windows/amd64 from the
# GitHub release assets, verifies its SHA256 checksum, and installs it to the
# prefix directory (default %USERPROFILE%\.local\bin).
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File install.ps1
#   powershell -ExecutionPolicy Bypass -File install.ps1 -Version v1.2.3

param(
    [string]$Version = "latest",
    [string]$Prefix = "$env:USERPROFILE\.local\bin"
)

$ErrorActionPreference = "Stop"

$Repo = "pin-gou/celer-route"
$BinName = "celer-route-http.exe"
$Arch = "amd64"

[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# Resolve the latest transports release if no explicit version was given
if ($Version -eq "latest") {
    $refs = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/git/matching-refs/tags/transports/v" -Headers @{ "User-Agent" = "celer-route-installer" }
    $versions = @($refs | ForEach-Object { $_.ref -replace '^refs/tags/transports/v', '' } | Where-Object { $_ -match '^[0-9]+\.[0-9]+\.[0-9]+$' })
    if ($versions.Count -eq 0) {
        Write-Host "could not resolve the latest version from GitHub" -ForegroundColor Red
        exit 1
    }
    $sorted = $versions | Sort-Object { [version]$_ }
    $Version = "v$($sorted[-1])"
    Write-Host "latest version: $Version"
}
else {
    $Version = $Version.TrimStart('v')
    $Version = "v$Version"
}

$Tag = "transports/$Version"
$Asset = "celer-route-http-windows-$Arch.exe"
$Url = "https://github.com/$Repo/releases/download/$Tag/$Asset"

Write-Host "Downloading $Url..."
$Tmp = Join-Path $env:TEMP "celer-route-install-$([guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $Tmp | Out-Null

try {
    Invoke-WebRequest -Uri $Url -OutFile (Join-Path $Tmp $Asset) -UseBasicParsing
    Invoke-WebRequest -Uri "$Url.sha256" -OutFile (Join-Path $Tmp "$Asset.sha256") -UseBasicParsing

    # Verify SHA256 checksum
    $expected = (Get-Content (Join-Path $Tmp "$Asset.sha256") -Raw).Trim().Split(' ')[0]
    $actual = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $Tmp $Asset)).Hash.ToLowerInvariant()
    if ($actual -ne $expected.ToLowerInvariant()) {
        Write-Host "SHA256 checksum verification failed" -ForegroundColor Red
        Write-Host "  expected: $expected"
        Write-Host "  actual:   $actual"
        exit 1
    }
    Write-Host "SHA256 checksum verified"

    New-Item -ItemType Directory -Path $Prefix -Force | Out-Null
    $dest = Join-Path $Prefix $BinName
    Move-Item -Force -Path (Join-Path $Tmp $Asset) -Destination $dest

    Write-Host ""
    Write-Host "installed celer-route-http $Version to $dest" -ForegroundColor Green
    Write-Host ""
    Write-Host "Run it:"
    Write-Host "  & '$dest' -host 0.0.0.0 -port 8080"
    Write-Host ""
    Write-Host "Add $Prefix to your PATH to call celer-route-http from anywhere."
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}