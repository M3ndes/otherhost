[CmdletBinding()]
param(
    [ValidatePattern('^[A-Za-z0-9._ -]+$')]
    [string]$Distro = 'Ubuntu',
    [ValidatePattern('^[A-Za-z0-9-]*$')]
    [string]$GitHubUser = '',
    [ValidatePattern('^(|SHA256:[A-Za-z0-9+/]+)$')]
    [string]$GitHubKeyFingerprint = '',
    [switch]$Check,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'
$RepositoryUrl = 'https://github.com/M3ndes/devbox-bridge.git'
$LinuxRepository = '~/src/devbox-bridge'
. (Join-Path $PSScriptRoot 'lib\devbox-windows.ps1')

function Write-Ok([string]$Message) { Write-Host "[ok] $Message" -ForegroundColor Green }
function Write-Step([string]$Message) { Write-Host "`n==> $Message" -ForegroundColor Cyan }
function Write-Warn([string]$Message) { Write-Warning $Message }
function Fail([string]$Message) { throw $Message }

function Test-IsAdministrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]$identity
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Get-InstalledDistributions {
    $distributionOutput = @(& wsl.exe --list --quiet 2>$null)
    return @(
        $distributionOutput |
            ForEach-Object { $_.Trim([char]0).Trim() } |
            Where-Object { $_ }
    )
}

function Get-RepositoryRevision {
    if (-not (Get-Command git.exe -ErrorAction SilentlyContinue) -and -not (Get-Command git -ErrorAction SilentlyContinue)) {
        Fail 'Git is required so setup can bind WSL to this exact repository revision'
    }

    $revision = ((& git -C $PSScriptRoot rev-parse --verify HEAD 2>$null) | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or $revision -notmatch '^[a-f0-9]{40}$') {
        Fail 'Could not determine the exact Git revision for this checkout'
    }
    $origin = ((& git -C $PSScriptRoot remote get-url origin 2>$null) | Out-String).Trim()
    $allowedOrigins = @(
        $RepositoryUrl,
        'git@github.com:M3ndes/devbox-bridge.git',
        'ssh://git@github.com/M3ndes/devbox-bridge.git'
    )
    if ($LASTEXITCODE -ne 0 -or $origin -notin $allowedOrigins) {
        Fail "This checkout must use the canonical devbox-bridge origin; found: $origin"
    }
    $worktreeChanges = @(& git -C $PSScriptRoot status --porcelain --untracked-files=all 2>$null)
    if ($LASTEXITCODE -ne 0) { Fail 'Could not verify that the Windows checkout is clean' }
    if ($worktreeChanges.Count -gt 0) {
        Fail 'This checkout has local changes; commit or remove them before running setup'
    }
    return $revision
}

function Get-SuggestedResources($Computer) {
    $totalMemoryGb = [int][math]::Floor($Computer.TotalPhysicalMemory / 1GB)
    $memoryGb = [int][math]::Min(20, $totalMemoryGb - 8)
    if ($memoryGb -lt 4) {
        Fail 'At least 12 GB of physical RAM is required for the automatic resource policy'
    }

    $processors = [int][math]::Min(8, $Computer.NumberOfLogicalProcessors)
    return @{
        Memory = "${memoryGb}GB"
        Processors = [string]$processors
        Swap = '8GB'
    }
}

function Set-ConfigValues([string]$Path, [hashtable]$Values) {
    $content = [System.IO.File]::ReadAllText($Path)
    foreach ($key in $Values.Keys) {
        $value = [string]$Values[$key]
        if ($value -match '[\r\n]') { Fail "Invalid configuration value for $key" }

        $pattern = '(?m)^[ \t]*' + [regex]::Escape($key) + '[ \t]*=.*$'
        $replacement = "$key=$value"
        if ([regex]::IsMatch($content, $pattern)) {
            $content = [regex]::Replace($content, $pattern, $replacement)
        } else {
            if ($content.Length -gt 0 -and -not $content.EndsWith("`n")) { $content += "`r`n" }
            $content += "$replacement`r`n"
        }
    }

    $encoding = [System.Text.UTF8Encoding]::new($false)
    [System.IO.File]::WriteAllText($Path, $content, $encoding)
}

function Get-ConfigValue([string]$Path, [string]$Key) {
    foreach ($line in [System.IO.File]::ReadAllLines($Path)) {
        $trimmed = $line.Trim()
        if (-not $trimmed -or $trimmed.StartsWith('#')) { continue }
        $separator = $trimmed.IndexOf('=')
        if ($separator -lt 1) { continue }
        if ($trimmed.Substring(0, $separator).Trim() -eq $Key) {
            return $trimmed.Substring($separator + 1).Trim()
        }
    }
    return ''
}

function Invoke-WslScript([string]$Script) {
    & wsl.exe -d $Distro -- bash -lc $Script
    if ($LASTEXITCODE -ne 0) { Fail "Ubuntu command failed with exit code $LASTEXITCODE" }
}

function Invoke-WslCommand([string[]]$Arguments) {
    & wsl.exe -d $Distro -- @Arguments
    if ($LASTEXITCODE -ne 0) { Fail "Ubuntu command failed with exit code $LASTEXITCODE" }
}

function Invoke-WindowsBootstrapApply([string]$Script, [string]$ConfigPath) {
    if (Test-IsAdministrator) {
        & $Script -Mode Apply -ConfigPath $ConfigPath
        return
    }

    $powershell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$Script`" -Mode Apply -ConfigPath `"$ConfigPath`""
    Write-Step 'Requesting Administrator permission'
    $process = Start-Process -FilePath $powershell -Verb RunAs -ArgumentList $arguments -Wait -PassThru
    if ($process.ExitCode -ne 0) { Fail 'Elevated Windows bootstrap failed' }
}

function Install-WslDistribution {
    if (Test-IsAdministrator) {
        & wsl.exe --install -d $Distro
        if ($LASTEXITCODE -ne 0) { Fail 'WSL distribution installation failed' }
        return
    }

    Write-Step 'Requesting Administrator permission'
    $process = Start-Process -FilePath 'wsl.exe' -Verb RunAs -ArgumentList "--install -d `"$Distro`"" -Wait -PassThru
    if ($process.ExitCode -ne 0) { Fail 'WSL distribution installation failed' }
}

function Get-LanAddress {
    try {
        $route = Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' -ErrorAction Stop |
            Where-Object { $_.NextHop -ne '0.0.0.0' } |
            Sort-Object RouteMetric |
            Select-Object -First 1
        if (-not $route) { return '' }

        $address = Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex $route.InterfaceIndex -ErrorAction Stop |
            Where-Object { $_.IPAddress -notlike '169.254.*' } |
            Select-Object -First 1
        if ($address) { return $address.IPAddress }
    } catch {
        return ''
    }
    return ''
}

Write-Step 'Checking this Windows host'
if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) {
    Fail 'WSL is not available. Install current WSL before running this setup.'
}

$os = Get-CimInstance Win32_OperatingSystem
$computer = Get-CimInstance Win32_ComputerSystem
$build = [int]$os.BuildNumber
if ($build -lt 22621) { Fail 'Mirrored networking requires Windows 11 22H2 or newer' }

$resources = Get-SuggestedResources $computer
Write-Ok ("Windows build {0}; {1:N1} GB RAM; {2} logical processors" -f $build, ($computer.TotalPhysicalMemory / 1GB), $computer.NumberOfLogicalProcessors)
Write-Ok "automatic WSL policy: $($resources.Memory) RAM, $($resources.Processors) processors, $($resources.Swap) swap"
$RepositoryRevision = Get-RepositoryRevision
Write-Ok "repository revision verified: $RepositoryRevision"

if (-not $GitHubUser -and -not $Check -and -not $Yes) {
    $GitHubUser = (Read-Host 'GitHub username containing the Mac public key').Trim()
    if ($GitHubUser -notmatch '^[A-Za-z0-9-]+$') { Fail 'Invalid GitHub username' }
}
if ($Yes -and -not $Check -and (-not $GitHubUser -or -not $GitHubKeyFingerprint)) {
    Fail 'Non-interactive setup requires both -GitHubUser and -GitHubKeyFingerprint'
}

$SelectedKey = $null
$GitHubKeys = @()
if ($GitHubUser) {
    Write-Step "Reading public keys for the explicitly selected GitHub account: $GitHubUser"
    $GitHubKeys = @(Get-GitHubPublicKeys -GitHubUser $GitHubUser)
    for ($index = 0; $index -lt $GitHubKeys.Count; $index++) {
        $comment = if ($GitHubKeys[$index].Comment) { " ($($GitHubKeys[$index].Comment))" } else { '' }
        Write-Host ("  [{0}] {1}{2}" -f ($index + 1), $GitHubKeys[$index].Fingerprint, $comment)
    }
    $SelectedKey = Select-SshPublicKey -Keys $GitHubKeys -Fingerprint $GitHubKeyFingerprint
    if (-not $SelectedKey -and -not $Check -and -not $Yes) {
        $selection = (Read-Host "Select the dedicated Mac key [1-$($GitHubKeys.Count)]").Trim()
        if ($selection -notmatch '^[1-9][0-9]*$' -or [int]$selection -gt $GitHubKeys.Count) {
            Fail 'Invalid SSH key selection'
        }
        $SelectedKey = $GitHubKeys[[int]$selection - 1]
    }
}

if ($SelectedKey) {
    Write-Ok "selected exactly one Mac key: $($SelectedKey.Fingerprint)"
} elseif ($Check) {
    Write-Warn 'No key was selected; pass -GitHubUser USER and, when the account has multiple keys, -GitHubKeyFingerprint SHA256:...'
} elseif ($Yes) {
    Fail 'Non-interactive setup requires both -GitHubUser and -GitHubKeyFingerprint'
} else {
    Fail 'A GitHub account and one Mac public key must be selected before applying'
}

$distributions = Get-InstalledDistributions
$distroInstalled = $Distro -in $distributions
if ($distroInstalled) {
    Write-Ok "WSL distribution is installed: $Distro"
} else {
    Write-Warn "WSL distribution is missing: $Distro"
}

Write-Host "`nThis setup will:"
Write-Host "  - keep the working clone in $LinuxRepository"
Write-Host '  - merge a safe WSL resource policy and mirrored networking settings'
Write-Host '  - add the Hyper-V firewall rule for the configured SSH port'
Write-Host '  - install and harden OpenSSH inside Ubuntu'
Write-Host '  - authorize exactly the selected public key; private keys never leave the Mac'
if ($SelectedKey) {
    Write-Host "  - selected SSH key: $($SelectedKey.Fingerprint)"
}

if ($Check) {
    Write-Host "`nCheck complete. Run .\setup.cmd to apply."
    exit 0
}

if (-not $Yes) {
    $answer = (Read-Host 'Continue? [Y/n]').Trim()
    if ($answer -and $answer -notmatch '^(?i:y|yes|s|sim)$') {
        Write-Host 'Setup cancelled; no changes were applied.'
        exit 0
    }
}

if (-not $distroInstalled) {
    Write-Step "Installing $Distro"
    Install-WslDistribution
    Write-Host "`nFinish the first Ubuntu launch, then run .\setup.cmd again."
    exit 0
}

Write-Step 'Preparing the repository inside Ubuntu'
$prepareScript = @'
set -eu
expected_revision=__REPOSITORY_REVISION__
repository="$HOME/src/devbox-bridge"
if ! command -v git >/dev/null 2>&1; then
  sudo apt-get update
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y git
fi
mkdir -p "$HOME/src"
if [ -d "$repository/.git" ]; then
  origin=$(git -C "$repository" remote get-url origin)
  case "$origin" in
    https://github.com/M3ndes/devbox-bridge.git|git@github.com:M3ndes/devbox-bridge.git|ssh://git@github.com/M3ndes/devbox-bridge.git) ;;
    *) printf '[fail] unexpected WSL repository origin: %s\n' "$origin" >&2; exit 1 ;;
  esac
  if [ -n "$(git -c core.fsmonitor=false -c core.hooksPath=/dev/null -C "$repository" status --porcelain)" ]; then
    printf '[fail] WSL repository has local changes; commit or remove them before setup\n' >&2
    exit 1
  fi
  git -c protocol.ext.allow=never -c core.hooksPath=/dev/null -C "$repository" fetch --no-tags origin "$expected_revision"
  git -c core.hooksPath=/dev/null -C "$repository" checkout --detach "$expected_revision"
elif [ -e "$repository" ]; then
  printf '[fail] %s exists but is not a Git repository\n' "$repository" >&2
  exit 1
else
  git -c protocol.ext.allow=never clone --no-checkout https://github.com/M3ndes/devbox-bridge.git "$repository"
  git -c core.hooksPath=/dev/null -C "$repository" checkout --detach "$expected_revision"
fi
actual_revision=$(git -C "$repository" rev-parse --verify HEAD)
[ "$actual_revision" = "$expected_revision" ] || {
  printf '[fail] WSL repository revision mismatch: expected %s, got %s\n' "$expected_revision" "$actual_revision" >&2
  exit 1
}
printf '[ok] WSL repository is pinned to %s\n' "$actual_revision"
'@
$prepareScript = $prepareScript.Replace('__REPOSITORY_REVISION__', $RepositoryRevision)
Invoke-WslScript $prepareScript

$WslUser = (& wsl.exe -d $Distro -- sh -lc 'id -un').Trim()
if ($LASTEXITCODE -ne 0 -or -not $WslUser) { Fail 'Could not determine the Ubuntu user' }
Write-Ok "Ubuntu user: $WslUser"
$WslHome = (& wsl.exe -d $Distro -- sh -lc 'printf %s "$HOME"').Trim()
if ($LASTEXITCODE -ne 0 -or $WslHome -notmatch '^/') { Fail 'Could not determine the Ubuntu home directory' }

$repoPath = "\\wsl.localhost\$Distro\home\$WslUser\src\devbox-bridge"
if (-not (Test-Path -LiteralPath $repoPath -PathType Container)) {
    Fail "Windows cannot access the WSL repository at $repoPath"
}

$configPath = Join-Path $repoPath 'devbox.local.conf'
$configCreated = -not (Test-Path -LiteralPath $configPath -PathType Leaf)
if ($configCreated) {
    Copy-Item -LiteralPath (Join-Path $repoPath 'config\devbox.example.conf') -Destination $configPath
}

$configValues = @{
    ssh_user = $WslUser
    github_user = $GitHubUser
    ssh_public_key = $SelectedKey.PublicKey
    wsl_distribution = $Distro
}
if ($configCreated) {
    $configValues.wsl_memory = $resources.Memory
    $configValues.wsl_processors = $resources.Processors
    $configValues.wsl_swap = $resources.Swap
}
Set-ConfigValues -Path $configPath -Values $configValues
if ($configCreated) {
    Write-Ok "created configuration: $configPath"
} else {
    Write-Ok "updated detected values while preserving configuration: $configPath"
}

$sshPort = Get-ConfigValue -Path $configPath -Key 'ssh_port'
if ($sshPort -notmatch '^[1-9][0-9]*$' -or [int]$sshPort -gt 65535) {
    Fail "Invalid ssh_port in $configPath"
}

$windowsBootstrap = Join-Path $PSScriptRoot 'scripts\bootstrap-windows.ps1'
Write-Step 'Validating Windows and WSL changes'
& $windowsBootstrap -Mode Check -ConfigPath $configPath

Write-Step 'Applying Windows and WSL policy'
Invoke-WindowsBootstrapApply -Script $windowsBootstrap -ConfigPath $configPath

Write-Step 'Configuring Ubuntu and SSH'
$trustedRepositoryPath = (& wsl.exe -d $Distro -- wslpath -a -u $PSScriptRoot).Trim()
if ($LASTEXITCODE -ne 0 -or -not $trustedRepositoryPath) { Fail 'Could not map the trusted Windows checkout into WSL' }
$trustedBootstrapPath = "$trustedRepositoryPath/scripts/bootstrap-wsl.sh"
$linuxConfigPath = "$WslHome/src/devbox-bridge/devbox.local.conf"
$bootstrapArguments = @('bash', $trustedBootstrapPath, '--apply', '--config', $linuxConfigPath)
Invoke-WslCommand -Arguments $bootstrapArguments

$pidOne = (& wsl.exe -d $Distro -- sh -lc 'ps -p 1 -o comm=').Trim()
if ($pidOne -ne 'systemd') {
    Write-Step 'Restarting WSL once to activate systemd'
    & wsl.exe --shutdown
    if ($LASTEXITCODE -ne 0) { Fail 'Could not restart WSL' }
    Invoke-WslCommand -Arguments $bootstrapArguments
}

$sshCheckScript = 'systemctl is-active --quiet ssh && ss -lnt | grep -Eq ":{0}[[:space:]]"' -f $sshPort
$null = & wsl.exe -d $Distro -- bash -lc $sshCheckScript
$sshReady = $LASTEXITCODE -eq 0
if (-not $sshReady) { Fail "SSH did not become ready on port $sshPort" }

$lanAddress = Get-LanAddress
Write-Host "`nDevbox is ready." -ForegroundColor Green
if ($lanAddress) {
    Write-Host "Windows address: $lanAddress"
    Write-Host "On the Mac, set host=$lanAddress and ssh_user=$WslUser, then run:"
} else {
    Write-Warn 'The LAN address could not be detected; use ipconfig to find it'
    Write-Host "On the Mac, set ssh_user=$WslUser, then run:"
}
Write-Host '  devbox doctor'
Write-Host '  devbox connect'
Write-Host "`nDocker is optional for SSH connectivity. If needed, enable Docker Desktop > Resources > WSL Integration > $Distro."
