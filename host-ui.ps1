[CmdletBinding()]
param(
    [ValidatePattern('^[A-Za-z0-9._ -]+$')]
    [string]$Distro = 'Ubuntu',
    [switch]$NoOpen
)

$ErrorActionPreference = 'Stop'
$RepositoryUrl = 'https://github.com/M3ndes/otherhost'
$LocalDashboard = Join-Path $PSScriptRoot 'build\otherhost-ui.exe'
$InstalledDashboard = Join-Path $env:LOCALAPPDATA 'otherhost\bin\otherhost-ui.exe'

function Write-Step([string]$Message) { Write-Host "==> $Message" -ForegroundColor Cyan }
function Write-Ok([string]$Message) { Write-Host "[ok] $Message" -ForegroundColor Green }

function Start-Dashboard([string]$Executable) {
    $arguments = @('--mode', 'host', '--repository', $PSScriptRoot, '--distribution', $Distro)
    if ($NoOpen) { $arguments += '--no-open' }
    & $Executable @arguments
    return $LASTEXITCODE
}

if (Test-Path -LiteralPath $LocalDashboard -PathType Leaf) {
    exit (Start-Dashboard -Executable $LocalDashboard)
}

$goCommand = Get-Command go.exe -ErrorAction SilentlyContinue
if (-not $goCommand) { $goCommand = Get-Command go -ErrorAction SilentlyContinue }
if ($goCommand) {
    Write-Step 'Building the host dashboard from this checkout'
    Push-Location $PSScriptRoot
    try {
        $arguments = @('run', './cmd/otherhost-ui', '--mode', 'host', '--repository', $PSScriptRoot, '--distribution', $Distro)
        if ($NoOpen) { $arguments += '--no-open' }
        & $goCommand.Source @arguments
        exit $LASTEXITCODE
    } finally {
        Pop-Location
    }
}

Write-Step 'Installing the checksum-verified release dashboard'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$headers = @{ 'User-Agent' = 'otherhost-host-ui' }
$release = Invoke-RestMethod -UseBasicParsing -Headers $headers -Uri 'https://api.github.com/repos/M3ndes/otherhost/releases/latest'

$architecture = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
$assetName = "otherhost-ui-windows-$architecture.exe"
$dashboardAsset = @($release.assets | Where-Object { $_.name -eq $assetName })
$checksumAsset = @($release.assets | Where-Object { $_.name -eq 'checksums.txt' })
if ($dashboardAsset.Count -ne 1 -or $checksumAsset.Count -ne 1) {
    throw "The latest Otherhost release does not contain $assetName and checksums.txt"
}

$temporaryDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ('otherhost-ui-' + [guid]::NewGuid().ToString('N'))
[void][System.IO.Directory]::CreateDirectory($temporaryDirectory)
try {
    $temporaryDashboard = Join-Path $temporaryDirectory $assetName
    $temporaryChecksums = Join-Path $temporaryDirectory 'checksums.txt'
    Invoke-WebRequest -UseBasicParsing -Headers $headers -Uri $dashboardAsset[0].browser_download_url -OutFile $temporaryDashboard
    Invoke-WebRequest -UseBasicParsing -Headers $headers -Uri $checksumAsset[0].browser_download_url -OutFile $temporaryChecksums

    $expectedHash = ''
    foreach ($line in [System.IO.File]::ReadAllLines($temporaryChecksums)) {
        if ($line -match '^([A-Fa-f0-9]{64})\s+\*?(.+)$' -and $Matches[2] -eq $assetName) {
            $expectedHash = $Matches[1].ToUpperInvariant()
            break
        }
    }
    if (-not $expectedHash) { throw "Release checksum is missing for $assetName" }
    $actualHash = (Get-FileHash -LiteralPath $temporaryDashboard -Algorithm SHA256).Hash.ToUpperInvariant()
    if ($actualHash -ne $expectedHash) { throw "Checksum verification failed for $assetName" }

    $installDirectory = Split-Path -Parent $InstalledDashboard
    [void][System.IO.Directory]::CreateDirectory($installDirectory)
    Move-Item -LiteralPath $temporaryDashboard -Destination $InstalledDashboard -Force
    Write-Ok "installed Otherhost host dashboard $($release.tag_name)"
} finally {
    Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force -ErrorAction SilentlyContinue
}

exit (Start-Dashboard -Executable $InstalledDashboard)
