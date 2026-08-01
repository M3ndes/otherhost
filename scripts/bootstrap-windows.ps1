[CmdletBinding()]
param(
    [ValidateSet('Check', 'Apply')]
    [string]$Mode = 'Check',
    [string]$ConfigPath = (Join-Path (Split-Path -Parent $PSScriptRoot) 'devbox.local.conf')
)

$ErrorActionPreference = 'Stop'

function Write-Ok([string]$Message) { Write-Host "[ok] $Message" -ForegroundColor Green }
function Write-Warn([string]$Message) { Write-Warning $Message }
function Fail([string]$Message) { throw $Message }

function Read-DevboxConfig([string]$Path) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        Fail "Configuration not found: $Path"
    }

    $values = @{}
    foreach ($line in [System.IO.File]::ReadAllLines($Path)) {
        $trimmed = $line.Trim()
        if (-not $trimmed -or $trimmed.StartsWith('#')) { continue }
        $separator = $trimmed.IndexOf('=')
        if ($separator -lt 1) { continue }
        $key = $trimmed.Substring(0, $separator).Trim()
        $value = $trimmed.Substring($separator + 1).Trim()
        $values[$key] = $value
    }
    return $values
}

function ConvertTo-ByteCount([string]$Value) {
    if ($Value -notmatch '^([1-9][0-9]*)(KB|MB|GB|TB)$') {
        Fail "Unsupported memory value: $Value"
    }
    $number = [int64]$Matches[1]
    $power = @{ KB = 1; MB = 2; GB = 3; TB = 4 }[$Matches[2]]
    return $number * [math]::Pow(1024, $power)
}

function Set-IniValue([System.Collections.ArrayList]$Lines, [string]$Section, [string]$Key, [string]$Value) {
    $sectionStart = -1
    $sectionEnd = $Lines.Count
    for ($index = 0; $index -lt $Lines.Count; $index++) {
        if ($Lines[$index] -match '^\s*\[([^]]+)\]\s*$') {
            if ($Matches[1] -ieq $Section) {
                $sectionStart = $index
                continue
            }
            if ($sectionStart -ge 0) {
                $sectionEnd = $index
                break
            }
        }
    }

    if ($sectionStart -lt 0) {
        if ($Lines.Count -gt 0 -and $Lines[$Lines.Count - 1] -ne '') { [void]$Lines.Add('') }
        [void]$Lines.Add("[$Section]")
        [void]$Lines.Add("$Key=$Value")
        return
    }

    for ($index = $sectionStart + 1; $index -lt $sectionEnd; $index++) {
        if ($Lines[$index] -match '^\s*([^#;][^=]*)\s*=') {
            if ($Matches[1].Trim() -ieq $Key) {
                $Lines[$index] = "$Key=$Value"
                return
            }
        }
    }
    $Lines.Insert($sectionStart + 1, "$Key=$Value")
}

$config = Read-DevboxConfig $ConfigPath
$required = @('ssh_port', 'wsl_distribution', 'wsl_memory', 'wsl_processors', 'wsl_swap', 'wsl_networking_mode')
foreach ($key in $required) {
    if (-not $config.ContainsKey($key) -or -not $config[$key]) { Fail "Missing configuration: $key" }
}

if (-not ($config.ssh_port -as [int]) -or [int]$config.ssh_port -gt 65535) { Fail 'ssh_port must be between 1 and 65535' }
if (-not ($config.wsl_processors -as [int]) -or [int]$config.wsl_processors -lt 1) { Fail 'wsl_processors must be a positive integer' }
if ($config.wsl_networking_mode -ne 'mirrored') { Fail 'only mirrored WSL networking is supported by this release' }

$os = Get-CimInstance Win32_OperatingSystem
$computer = Get-CimInstance Win32_ComputerSystem
$build = [int]$os.BuildNumber
$totalMemory = [int64]$computer.TotalPhysicalMemory
$wslMemory = [int64](ConvertTo-ByteCount $config.wsl_memory)
if ($wslMemory -gt ($totalMemory - 8GB)) {
    Fail "wsl_memory=$($config.wsl_memory) leaves less than 8 GB for Windows"
}
Write-Ok ("Windows build {0}; {1:N1} GB RAM; {2} logical processors" -f $build, ($totalMemory / 1GB), $computer.NumberOfLogicalProcessors)
if ([int]$config.wsl_processors -gt [int]$computer.NumberOfLogicalProcessors) {
    Fail "wsl_processors=$($config.wsl_processors) exceeds the desktop's logical processor count"
}

if ($config.wsl_networking_mode -eq 'mirrored' -and $build -lt 22621) {
    Fail 'mirrored networking requires Windows 11 22H2 (build 22621) or newer'
}

if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) { Fail 'WSL is not installed' }
$wslVersionOutput = @(& wsl.exe --version 2>&1)
$wslVersion = ($wslVersionOutput -join "`n").Replace([string][char]0, '').Trim()
Write-Ok "WSL is installed`n$wslVersion"
$distributionOutput = @(& wsl.exe --list --quiet)
$distributions = @(
    $distributionOutput |
        ForEach-Object { $_.Trim([char]0).Trim() } |
        Where-Object { $_ }
)
if ($config.wsl_distribution -notin $distributions) {
    Fail "WSL distribution '$($config.wsl_distribution)' is not installed"
}
Write-Ok "WSL distribution exists: $($config.wsl_distribution)"

if (Get-Command docker.exe -ErrorAction SilentlyContinue) {
    $previousErrorActionPreference = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & docker.exe info *> $null
    $dockerExitCode = $LASTEXITCODE
    $ErrorActionPreference = $previousErrorActionPreference
    if ($dockerExitCode -eq 0) {
        Write-Ok 'Docker Desktop engine is running'
    } else {
        Write-Warn 'Docker CLI exists, but Docker Desktop engine is not ready'
    }
} else {
    Write-Warn 'Docker Desktop was not found; install it and enable WSL integration for the configured distribution'
}

$wslConfigPath = Join-Path $HOME '.wslconfig'
if ($Mode -eq 'Check') {
    if (Test-Path -LiteralPath $wslConfigPath) {
        Write-Ok ".wslconfig exists: $wslConfigPath"
    } else {
        Write-Warn ".wslconfig is missing: $wslConfigPath"
    }
    Write-Host "`nRun again with -Mode Apply to merge the configured resource policy and firewall rule."
    exit 0
}

$lines = [System.Collections.ArrayList]::new()
if (Test-Path -LiteralPath $wslConfigPath) {
    $backup = "$wslConfigPath.$(Get-Date -Format 'yyyyMMdd-HHmmss').bak"
    Copy-Item -LiteralPath $wslConfigPath -Destination $backup
    Write-Ok "backed up existing configuration: $backup"
    foreach ($line in [System.IO.File]::ReadAllLines($wslConfigPath)) { [void]$lines.Add($line) }
}

Set-IniValue $lines 'wsl2' 'memory' $config.wsl_memory
Set-IniValue $lines 'wsl2' 'processors' $config.wsl_processors
Set-IniValue $lines 'wsl2' 'swap' $config.wsl_swap
Set-IniValue $lines 'wsl2' 'networkingMode' $config.wsl_networking_mode
Set-IniValue $lines 'experimental' 'autoMemoryReclaim' 'gradual'
Set-IniValue $lines 'experimental' 'sparseVhd' 'true'

$encoding = [System.Text.UTF8Encoding]::new($false)
[System.IO.File]::WriteAllLines($wslConfigPath, [string[]]$lines, $encoding)
Write-Ok "updated $wslConfigPath"

if ($config.wsl_networking_mode -eq 'mirrored') {
    $vmCreatorId = '{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}'
    $ruleName = 'devbox-bridge-ssh'
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (Get-Command Get-NetFirewallHyperVRule -ErrorAction SilentlyContinue) {
        $existingRule = Get-NetFirewallHyperVRule -Name $ruleName -ErrorAction SilentlyContinue
        if ($existingRule) {
            $currentPorts = @($existingRule.LocalPorts) -join ','
            if ($currentPorts -eq [string]$config.ssh_port) {
                Write-Ok "Hyper-V firewall rule already exists: $ruleName"
            } elseif ($isAdmin -and (Get-Command Set-NetFirewallHyperVRule -ErrorAction SilentlyContinue)) {
                Set-NetFirewallHyperVRule -Name $ruleName -LocalPorts ([string]$config.ssh_port) | Out-Null
                Write-Ok "updated Hyper-V firewall rule to TCP port $($config.ssh_port)"
            } else {
                Write-Warn "Hyper-V firewall rule uses ports $currentPorts; rerun as Administrator to update it"
            }
        } elseif ($isAdmin) {
            New-NetFirewallHyperVRule -Name $ruleName -DisplayName 'Devbox Bridge SSH' -Direction Inbound -VMCreatorId $vmCreatorId -Protocol TCP -LocalPorts ([int]$config.ssh_port) | Out-Null
            Write-Ok "opened Hyper-V firewall TCP port $($config.ssh_port) for WSL"
        } else {
            Write-Warn "rerun PowerShell as Administrator to open Hyper-V firewall TCP port $($config.ssh_port)"
        }
    } else {
        Write-Warn 'Hyper-V firewall cmdlets are unavailable; verify LAN access to the SSH port manually'
    }
}

Write-Host "`nRestarting WSL so resource and networking changes take effect..."
& wsl.exe --shutdown
if ($LASTEXITCODE -ne 0) { Fail 'wsl --shutdown failed' }
Write-Ok 'WSL stopped; start Docker Desktop and the configured WSL distribution again'
