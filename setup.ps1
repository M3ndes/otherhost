[CmdletBinding()]
param(
    [ValidatePattern('^[A-Za-z0-9._ -]+$')]
    [string]$Distro = 'Ubuntu',
    [ValidatePattern('^[A-Za-z0-9-]*$')]
    [string]$GitHubUser = '',
    [ValidatePattern('^(|SHA256:[A-Za-z0-9+/]+)$')]
    [string]$GitHubKeyFingerprint = '',
    [switch]$Pair,
    [switch]$Check,
    [switch]$Update,
    [switch]$Yes
)

$ErrorActionPreference = 'Stop'
$RepositoryUrl = 'https://github.com/M3ndes/otherhost.git'
$LinuxRepository = '~/src/otherhost'
$PairingDiscoveryPort = 25370
$PairingSessionPort = 25371
$PairingDiscoveryAddress = "239.255.67.89:$PairingDiscoveryPort"
$CompatibilityVersion = [System.IO.File]::ReadAllText((Join-Path $PSScriptRoot 'config\compatibility-version')).Trim()
if ($CompatibilityVersion -notmatch '^[0-9]+$') { throw 'Invalid compatibility version in this checkout' }
. (Join-Path $PSScriptRoot 'lib\otherhost-windows.ps1')

function Write-Ok([string]$Message) { Write-Host "[ok] $Message" -ForegroundColor Green }
function Write-Step([string]$Message) { Write-Host "`n==> $Message" -ForegroundColor Cyan }
function Write-Warn([string]$Message) { Write-Host "[warn] $Message" -ForegroundColor Yellow }
function Write-Diag([string]$Message) { Write-Host "[diag] $Message" -ForegroundColor DarkGray }
function Fail([string]$Message) { throw $Message }

function Test-IsAdministrator {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = [Security.Principal.WindowsPrincipal]$identity
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function ConvertTo-ProcessArgument([string]$Value) {
    if ($Value -notmatch '[\s"]') { return $Value }
    return '"' + $Value.Replace('"', '\"') + '"'
}

function Invoke-CapturedProcess([string]$FilePath, [string[]]$Arguments) {
    $standardOutput = [System.IO.Path]::GetTempFileName()
    $standardError = [System.IO.Path]::GetTempFileName()
    try {
        $argumentList = @($Arguments | ForEach-Object { ConvertTo-ProcessArgument $_ })
        $process = Start-Process -FilePath $FilePath -ArgumentList $argumentList -NoNewWindow -Wait -PassThru `
            -RedirectStandardOutput $standardOutput -RedirectStandardError $standardError
        return [pscustomobject]@{
            ExitCode = $process.ExitCode
            Output = [System.IO.File]::ReadAllText($standardOutput)
            Error = [System.IO.File]::ReadAllText($standardError)
        }
    } finally {
        Remove-Item -LiteralPath $standardOutput -Force -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $standardError -Force -ErrorAction SilentlyContinue
    }
}

function Update-Repository {
    $gitCommand = Get-Command git.exe -ErrorAction SilentlyContinue
    if (-not $gitCommand) { $gitCommand = Get-Command git -ErrorAction SilentlyContinue }
    if (-not $gitCommand) { Fail 'Git is required to update Otherhost' }
    $gitExecutable = $gitCommand.Source

    $originResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-C', $PSScriptRoot, 'remote', 'get-url', 'origin')
    $origin = $originResult.Output.Trim()
    if ($originResult.ExitCode -ne 0 -or $origin -notin @(
        $RepositoryUrl,
        'git@github.com:M3ndes/otherhost.git',
        'ssh://git@github.com:M3ndes/otherhost.git'
    )) {
        Fail "This checkout must use the canonical otherhost origin before updating; found: $origin"
    }

    $branchResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-C', $PSScriptRoot, 'symbolic-ref', '--quiet', '--short', 'HEAD')
    $branch = $branchResult.Output.Trim()
    if ($branchResult.ExitCode -ne 0 -or $branch -ne 'main') {
        Fail "Windows updates require the main branch; current branch: $branch"
    }
    $statusResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-c', 'core.fsmonitor=false', '-c', 'core.hooksPath=NUL', '-C', $PSScriptRoot, 'status', '--porcelain', '--untracked-files=all')
    if ($statusResult.ExitCode -ne 0) { Fail 'Could not verify that the Windows checkout is clean' }
    if ($statusResult.Output.Trim()) { Fail 'The Windows checkout has local changes; commit or remove them before updating' }

    Write-Step 'Fetching the latest main revision'
    $fetchResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-c', 'protocol.ext.allow=never', '-c', 'core.hooksPath=NUL', '-C', $PSScriptRoot, 'fetch', '--no-tags', 'origin', 'main')
    if ($fetchResult.ExitCode -ne 0) { Fail "Could not fetch origin/main: $($fetchResult.Error.Trim())" }
    $targetResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-C', $PSScriptRoot, 'rev-parse', '--verify', 'FETCH_HEAD')
    $target = $targetResult.Output.Trim()
    if ($targetResult.ExitCode -ne 0 -or $target -notmatch '^[a-f0-9]{40}$') { Fail 'Git returned an invalid update revision' }
    $mergeResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-c', 'core.hooksPath=NUL', '-C', $PSScriptRoot, 'merge', '--ff-only', $target)
    if ($mergeResult.ExitCode -ne 0) { Fail "The Windows checkout could not be fast-forwarded: $($mergeResult.Error.Trim())" }
    Write-Ok "Windows checkout is current at $($target.Substring(0, 12))"
}

function Write-InstallationState([string]$Revision) {
    if ($Revision -notmatch '^[a-f0-9]{40}$') { Fail 'Cannot persist an invalid installation revision' }
    $stateDirectory = Join-Path $env:LOCALAPPDATA 'otherhost'
    [void][System.IO.Directory]::CreateDirectory($stateDirectory)
    $statePath = Join-Path $stateDirectory 'install-state'
    $content = "compatibility=$CompatibilityVersion`r`nrevision=$Revision`r`n"
    [System.IO.File]::WriteAllText($statePath, $content, [System.Text.UTF8Encoding]::new($false))
    Write-Ok "recorded Windows installation compatibility $CompatibilityVersion"
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
    $gitCommand = Get-Command git.exe -ErrorAction SilentlyContinue
    if (-not $gitCommand) { $gitCommand = Get-Command git -ErrorAction SilentlyContinue }
    if (-not $gitCommand) {
        Fail 'Git is required so setup can bind WSL to this exact repository revision'
    }
    $gitExecutable = $gitCommand.Source

    $revisionResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-C', $PSScriptRoot, 'rev-parse', '--verify', 'HEAD')
    $revision = $revisionResult.Output.Trim()
    if ($revisionResult.ExitCode -ne 0 -or $revision -notmatch '^[a-f0-9]{40}$') {
        Fail 'Could not determine the exact Git revision for this checkout'
    }
    $originResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-C', $PSScriptRoot, 'remote', 'get-url', 'origin')
    $origin = $originResult.Output.Trim()
    $allowedOrigins = @(
        $RepositoryUrl,
        'git@github.com:M3ndes/otherhost.git',
        'ssh://git@github.com/M3ndes/otherhost.git',
        'https://github.com/M3ndes/devbox-bridge.git',
        'git@github.com:M3ndes/devbox-bridge.git',
        'ssh://git@github.com/M3ndes/devbox-bridge.git'
    )
    if ($originResult.ExitCode -ne 0 -or $origin -notin $allowedOrigins) {
        Fail "This checkout must use the canonical otherhost origin; found: $origin"
    }
    $statusResult = Invoke-CapturedProcess -FilePath $gitExecutable -Arguments @('-C', $PSScriptRoot, 'status', '--porcelain', '--untracked-files=all')
    if ($statusResult.ExitCode -ne 0) { Fail 'Could not verify that the Windows checkout is clean' }
    if ($statusResult.Output.Trim()) {
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

function Convert-LegacyConfig([string]$Path) {
    $lines = [System.IO.File]::ReadAllLines($Path)
    $hasOtherhostName = $false
    foreach ($line in $lines) {
        if ($line -match '^\s*otherhost_name\s*=') { $hasOtherhostName = $true }
    }
    for ($index = 0; $index -lt $lines.Length; $index++) {
        if ($lines[$index] -match '^\s*devbox_name\s*=(.*)$') {
            if ($hasOtherhostName) {
                $lines[$index] = '# Legacy machine-name key migrated to otherhost_name'
            } else {
                $lines[$index] = 'otherhost_name=' + $Matches[1].Trim()
                $hasOtherhostName = $true
            }
        }
    }
    [System.IO.File]::WriteAllLines($Path, $lines, [System.Text.UTF8Encoding]::new($false))
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
    $wslHomeOutput = @(& wsl.exe -d $Distro -- sh -lc 'printf %s "$HOME"')
    $wslHomeExitCode = $LASTEXITCODE
    $wslHome = ($wslHomeOutput -join "`n").Trim()
    if ($wslHomeExitCode -ne 0 -or $wslHome -notmatch '^/') {
        Fail 'Could not determine the Ubuntu home directory for script handoff'
    }

    $temporaryName = '.otherhost-' + [guid]::NewGuid().ToString('N') + '.sh'
    $wslScriptPath = "$wslHome/$temporaryName"
    $windowsScriptPath = "\\wsl.localhost\$Distro" + $wslScriptPath.Replace('/', '\')
    try {
        $normalizedScript = $Script.Replace("`r`n", "`n").Replace("`r", "`n")
        [System.IO.File]::WriteAllText(
            $windowsScriptPath,
            $normalizedScript,
            [System.Text.UTF8Encoding]::new($false)
        )

        & wsl.exe -d $Distro -- bash $wslScriptPath
        if ($LASTEXITCODE -ne 0) { Fail "Ubuntu command failed with exit code $LASTEXITCODE" }
    } finally {
        Remove-Item -LiteralPath $windowsScriptPath -Force -ErrorAction SilentlyContinue
    }
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

function Get-PairingHelper {
    $pairingBinaryOverride = if ($env:OTHERHOST_PAIR_BIN) { $env:OTHERHOST_PAIR_BIN } else { $env:DEVBOX_PAIR_BIN }
    if ($pairingBinaryOverride) {
        $explicitHelper = [System.IO.Path]::GetFullPath($pairingBinaryOverride)
        if (-not (Test-Path -LiteralPath $explicitHelper -PathType Leaf)) {
            Fail "OTHERHOST_PAIR_BIN does not exist: $explicitHelper"
        }
        return $explicitHelper
    }
    $architecture = switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'amd64' }
        'ARM64' { 'arm64' }
        default { Fail "Unsupported Windows architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
    $version = 'v0.1.1'
    $asset = "otherhost-pair-windows-$architecture.exe"
    $installDirectory = Join-Path $env:LOCALAPPDATA 'otherhost\bin'
    $destination = Join-Path $installDirectory 'otherhost-pair.exe'
    $versionFile = "$destination.version"
    if ((Test-Path -LiteralPath $destination -PathType Leaf) -and
        (Test-Path -LiteralPath $versionFile -PathType Leaf) -and
        ([System.IO.File]::ReadAllText($versionFile).Trim() -eq $version)) {
        return $destination
    }
    $legacyDestination = Join-Path $env:LOCALAPPDATA 'devbox-bridge\bin\devbox-pair.exe'
    $legacyVersionFile = "$legacyDestination.version"
    if ((Test-Path -LiteralPath $legacyDestination -PathType Leaf) -and
        (Test-Path -LiteralPath $legacyVersionFile -PathType Leaf) -and
        ([System.IO.File]::ReadAllText($legacyVersionFile).Trim() -eq $version)) {
        return $legacyDestination
    }

    Write-Step "Installing secure pairing helper $version"
    $baseUrl = "https://github.com/M3ndes/otherhost/releases/download/$version"
    $temporaryDirectory = Join-Path ([System.IO.Path]::GetTempPath()) ("otherhost-pair-" + [guid]::NewGuid().ToString('N'))
    [void](New-Item -ItemType Directory -Path $temporaryDirectory)
    try {
        $download = Join-Path $temporaryDirectory $asset
        $checksums = Join-Path $temporaryDirectory 'checksums.txt'
        try {
            Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$asset" -OutFile $download
        } catch {
            $asset = "devbox-pair-windows-$architecture.exe"
            $baseUrl = "https://github.com/M3ndes/devbox-bridge/releases/download/$version"
            $download = Join-Path $temporaryDirectory $asset
            Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/$asset" -OutFile $download
        }
        Invoke-WebRequest -UseBasicParsing -Uri "$baseUrl/checksums.txt" -OutFile $checksums
        $checksumPattern = '^([0-9a-fA-F]{64})[ \t]+' + [regex]::Escape($asset) + '$'
        $checksumLine = [System.IO.File]::ReadAllLines($checksums) |
            Where-Object { $_ -match $checksumPattern } |
            Select-Object -First 1
        $checksumMatch = if ($checksumLine) { [regex]::Match($checksumLine, $checksumPattern) } else { $null }
        if (-not $checksumMatch -or -not $checksumMatch.Success) {
            Fail "Release checksum is missing for $asset"
        }
        $expected = $checksumMatch.Groups[1].Value.ToUpperInvariant()
        $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $download).Hash.ToUpperInvariant()
        if ($actual -cne $expected) { Fail 'The pairing helper checksum did not match' }

        [void](New-Item -ItemType Directory -Force -Path $installDirectory)
        Copy-Item -LiteralPath $download -Destination $destination -Force
        [System.IO.File]::WriteAllText($versionFile, "$version`r`n", [System.Text.UTF8Encoding]::new($false))
        Write-Ok "installed secure pairing helper: $destination"
    } finally {
        if (Test-Path -LiteralPath $temporaryDirectory) {
            Remove-Item -LiteralPath $temporaryDirectory -Recurse -Force
        }
    }
    return $destination
}

function Get-ActivePairingNetworkPolicy {
    $profiles = @(Get-NetConnectionProfile -ErrorAction Stop | Where-Object {
        $_.IPv4Connectivity -ne 'Disconnected'
    })
    if ($profiles.Count -eq 0) {
        Fail 'Pairing requires an active Windows IPv4 network'
    }
    $profileNames = @($profiles |
        ForEach-Object {
            if ($_.NetworkCategory -eq 'DomainAuthenticated') { 'Domain' } else { $_.NetworkCategory.ToString() }
        } |
        Where-Object { $_ -in @('Public', 'Private', 'Domain') } |
        Sort-Object -Unique)
    if ($profileNames.Count -eq 0) {
        Fail 'Could not determine the active Windows firewall profile'
    }
    $remoteSubnets = @($profiles | ForEach-Object {
        Get-NetIPAddress -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
    } | Where-Object {
        $_.IPAddress -notmatch '^169\.254\.' -and $_.PrefixLength -ge 1 -and $_.PrefixLength -le 30
    } | ForEach-Object {
        "$($_.IPAddress)/$($_.PrefixLength)"
    } | Sort-Object -Unique)
    if ($remoteSubnets.Count -eq 0) {
        Fail 'Could not determine the active Windows IPv4 subnet'
    }
    return [pscustomobject]@{
        Profiles = $profileNames
        RemoteSubnets = $remoteSubnets
    }
}

function Start-PairingTranscript {
    $logDirectory = Join-Path $env:LOCALAPPDATA 'otherhost\logs'
    [void][System.IO.Directory]::CreateDirectory($logDirectory)
    $logPath = Join-Path $logDirectory 'pairing-latest.log'
    Start-Transcript -LiteralPath $logPath -Force | Out-Null
    Write-Host "[diag] pairing log: $logPath"
    return $logPath
}

function Ensure-HyperVSSHRule([int]$SSHPort, [string[]]$RemoteSubnets) {
    foreach ($commandName in @('Get-NetFirewallHyperVRule', 'New-NetFirewallHyperVRule', 'Set-NetFirewallHyperVRule', 'Remove-NetFirewallHyperVRule')) {
        if (-not (Get-Command $commandName -ErrorAction SilentlyContinue)) {
            Fail "Windows does not provide the required Hyper-V firewall command: $commandName"
        }
    }
    $vmCreatorId = '{40E0AC32-46A5-438A-A0B2-2B479E8F2E90}'
    $ruleName = 'otherhost-ssh'
    $existingRule = Get-NetFirewallHyperVRule -Name $ruleName -ErrorAction SilentlyContinue
    if ($existingRule) {
        Set-NetFirewallHyperVRule -Name $ruleName -VMCreatorId $vmCreatorId -Protocol TCP `
            -LocalPorts ([string]$SSHPort) -RemoteAddresses $RemoteSubnets | Out-Null
        Write-Ok "updated Hyper-V SSH rule for TCP $SSHPort and the active local subnet"
    } else {
        New-NetFirewallHyperVRule -Name $ruleName -DisplayName 'Otherhost SSH' `
            -Direction Inbound -Action Allow -VMCreatorId $vmCreatorId -Protocol TCP `
            -LocalPorts $SSHPort -RemoteAddresses $RemoteSubnets | Out-Null
        Write-Ok "opened Hyper-V SSH port $SSHPort for the active local subnet"
    }
    $legacyRule = Get-NetFirewallHyperVRule -Name 'devbox-bridge-ssh' -ErrorAction SilentlyContinue
    if ($legacyRule) {
        Remove-NetFirewallHyperVRule -Name 'devbox-bridge-ssh'
        Write-Ok 'removed superseded devbox-bridge Hyper-V SSH rule'
    }
}

function Start-PairingMode {
    if (-not (Test-IsAdministrator)) {
        Write-Step 'Requesting Administrator permission for temporary private-network discovery'
        $powershell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
        $arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$PSCommandPath`" -Pair -Distro `"$Distro`""
        $process = Start-Process -FilePath $powershell -Verb RunAs -ArgumentList $arguments -Wait -PassThru
        if ($process.ExitCode -ne 0) { Fail 'Windows pairing mode failed' }
        return
    }

    $distributions = Get-InstalledDistributions
    if ($Distro -notin $distributions) { Fail "WSL distribution is missing: $Distro" }
    $wslUser = (& wsl.exe -d $Distro -- sh -lc 'id -un').Trim()
    if ($LASTEXITCODE -ne 0 -or -not $wslUser) { Fail 'Could not determine the Ubuntu user' }
    $repoPath = "\\wsl.localhost\$Distro\home\$wslUser\src\otherhost"
    $configPath = Join-Path $repoPath 'otherhost.local.conf'
    if (-not (Test-Path -LiteralPath $configPath -PathType Leaf)) {
        $legacyRepoPath = "\\wsl.localhost\$Distro\home\$wslUser\src\devbox-bridge"
        $legacyConfigPath = Join-Path $legacyRepoPath 'devbox.local.conf'
        if (Test-Path -LiteralPath $legacyConfigPath -PathType Leaf) {
            $repoPath = $legacyRepoPath
            $configPath = $legacyConfigPath
        } else {
            Fail "Windows setup must be completed before pairing: $configPath"
        }
    }
    $sshPort = Get-ConfigValue -Path $configPath -Key 'ssh_port'
    if ($sshPort -notmatch '^[1-9][0-9]*$' -or [int]$sshPort -gt 65535) {
        Fail "Invalid ssh_port in $configPath"
    }
    $null = & wsl.exe -d $Distro -- sh -lc '(test -r "$HOME/.local/lib/otherhost/ssh_host_ed25519_key.pub" && systemctl --user is-active --quiet otherhost-sshd.service) || (test -r "$HOME/.local/lib/devbox-bridge/ssh_host_ed25519_key.pub" && systemctl --user is-active --quiet devbox-bridge-sshd.service)'
    $userScopedWSL = $LASTEXITCODE -eq 0
    if ($userScopedWSL) {
        Write-Ok 'user-scoped WSL SSH host is active'
    } else {
        $null = & wsl.exe -d $Distro -- sh -c 'test -r /etc/ssh/ssh_host_ed25519_key.pub'
        if ($LASTEXITCODE -ne 0) { Fail 'SSH must be configured in WSL before pairing' }
        Write-Ok 'system WSL SSH host is available'
    }

    $networkPolicy = Get-ActivePairingNetworkPolicy
    Write-Ok "temporary pairing access uses profile(s): $($networkPolicy.Profiles -join ', ')"
    Write-Ok "temporary pairing access is limited to: $($networkPolicy.RemoteSubnets -join ', ')"
    Ensure-HyperVSSHRule -SSHPort ([int]$sshPort) -RemoteSubnets $networkPolicy.RemoteSubnets

    $helper = Get-PairingHelper
    $helperVersion = ((& $helper version 2>&1) | Out-String).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $helperVersion) {
        Fail 'Could not read the installed pairing helper version'
    }
    Write-Diag "pairing helper: $helperVersion ($helper)"
    if ($userScopedWSL) {
        $helperCapabilitiesResult = Invoke-CapturedProcess -FilePath $helper -Arguments @('host', '-h')
        $helperCapabilities = $helperCapabilitiesResult.Output + $helperCapabilitiesResult.Error
        if ($helperCapabilities -notmatch '(?m)user-scoped-wsl') {
            Fail "Installed pairing helper $helperVersion does not support user-scoped WSL; publish and install a helper built from this revision"
        }
    }
    $ruleSuffix = "$PID-$([guid]::NewGuid().ToString('N'))"
    $discoveryRule = "otherhost-pairing-udp-$ruleSuffix"
    $sessionRule = "otherhost-pairing-tcp-$ruleSuffix"
    try {
        New-NetFirewallRule -Name $discoveryRule -DisplayName 'Otherhost temporary discovery' `
            -Direction Inbound -Action Allow -Program $helper -Protocol UDP -LocalPort $PairingDiscoveryPort `
            -RemoteAddress $networkPolicy.RemoteSubnets -Profile $networkPolicy.Profiles | Out-Null
        New-NetFirewallRule -Name $sessionRule -DisplayName 'Otherhost temporary pairing' `
            -Direction Inbound -Action Allow -Program $helper -Protocol TCP -LocalPort $PairingSessionPort `
            -RemoteAddress $networkPolicy.RemoteSubnets -Profile $networkPolicy.Profiles | Out-Null
        Write-Diag "temporary UDP $PairingDiscoveryPort and TCP $PairingSessionPort firewall rules are active"

        $helperArguments = @(
            'host', '--name', $env:COMPUTERNAME, '--distro', $Distro,
            '--ssh-user', $wslUser, '--ssh-port', $sshPort,
            '--pair-port', $PairingSessionPort,
            '--discovery-address', $PairingDiscoveryAddress
        )
        if ($userScopedWSL) { $helperArguments += '--user-scoped-wsl' }
        Write-Diag 'starting the pairing helper; the TCP listener should remain active for two minutes'
        & $helper @helperArguments
        if ($LASTEXITCODE -ne 0) { Fail "Pairing helper failed with exit code $LASTEXITCODE" }
    } finally {
        Remove-NetFirewallRule -Name $discoveryRule -ErrorAction SilentlyContinue
        Remove-NetFirewallRule -Name $sessionRule -ErrorAction SilentlyContinue
        Write-Diag 'temporary pairing firewall rules were removed'
    }
}

if ($Update) {
    if ($Pair -or $Check -or $GitHubUser -or $GitHubKeyFingerprint) {
        Fail '-Update cannot be combined with pairing, check, or GitHub recovery options'
    }
    Write-Step 'Updating the Windows checkout safely'
    Update-Repository
    Write-Step 'Running setup from the updated checkout'
    $updatedSetup = Join-Path $PSScriptRoot 'setup.ps1'
    & powershell.exe -NoProfile -ExecutionPolicy Bypass -File $updatedSetup -Distro $Distro -Yes
    exit $LASTEXITCODE
}

Write-Step 'Checking this Windows host'
if (-not (Get-Command wsl.exe -ErrorAction SilentlyContinue)) {
    Fail 'WSL is not available. Install current WSL before running this setup.'
}

if ($Pair) {
    $pairingLogPath = ''
    if (Test-IsAdministrator) { $pairingLogPath = Start-PairingTranscript }
    try {
        $RepositoryRevision = Get-RepositoryRevision
        Write-Ok "repository revision verified: $RepositoryRevision"
        Start-PairingMode
    } catch {
        if ($pairingLogPath) { Write-Host "[diag] pairing failed; log saved to: $pairingLogPath" -ForegroundColor Yellow }
        throw
    } finally {
        if ($pairingLogPath) { Stop-Transcript | Out-Null }
    }
    exit 0
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

if ($GitHubKeyFingerprint -and -not $GitHubUser) { Fail '-GitHubKeyFingerprint requires -GitHubUser' }

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
} elseif ($GitHubUser -and $Yes) {
    Fail 'Non-interactive GitHub key recovery requires -GitHubKeyFingerprint'
} elseif ($GitHubUser) {
    Fail 'Select one GitHub public key before applying'
} else {
    Write-Ok 'no SSH client key selected; secure device pairing will add one later'
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
Write-Host '  - prepare hardened public-key-only SSH access'
if ($SelectedKey) {
    Write-Host "  - selected SSH key: $($SelectedKey.Fingerprint)"
}

if ($Check) {
    Write-Host "`nCheck complete. Run .\setup.cmd to apply."
    exit 0
}

if (-not $Yes) {
    $answer = (Read-Host 'Continue? [Y/n]').Trim()
    if ($answer -and $answer -notmatch '^(?i:y|yes)$') {
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
repository="$HOME/src/otherhost"
if ! command -v git >/dev/null 2>&1; then
  sudo apt-get update
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y git
fi
mkdir -p "$HOME/src"
if [ -d "$repository/.git" ]; then
  origin=$(git -C "$repository" remote get-url origin)
  case "$origin" in
    https://github.com/M3ndes/otherhost.git|git@github.com:M3ndes/otherhost.git|ssh://git@github.com/M3ndes/otherhost.git) ;;
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
  git -c protocol.ext.allow=never clone --no-checkout https://github.com/M3ndes/otherhost.git "$repository"
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

$repoPath = "\\wsl.localhost\$Distro\home\$WslUser\src\otherhost"
if (-not (Test-Path -LiteralPath $repoPath -PathType Container)) {
    Fail "Windows cannot access the WSL repository at $repoPath"
}

$configPath = Join-Path $repoPath 'otherhost.local.conf'
$configCreated = -not (Test-Path -LiteralPath $configPath -PathType Leaf)
$configMigrated = $false
if ($configCreated) {
    $legacyConfigPath = "\\wsl.localhost\$Distro\home\$WslUser\src\devbox-bridge\devbox.local.conf"
    if (Test-Path -LiteralPath $legacyConfigPath -PathType Leaf) {
        Copy-Item -LiteralPath $legacyConfigPath -Destination $configPath
        Convert-LegacyConfig -Path $configPath
        $configMigrated = $true
        Write-Ok "migrated legacy configuration: $configPath"
    } else {
        Copy-Item -LiteralPath (Join-Path $repoPath 'config\otherhost.example.conf') -Destination $configPath
    }
}

$configValues = @{
    ssh_user = $WslUser
    wsl_distribution = $Distro
}
if ($SelectedKey) {
    $configValues.github_user = $GitHubUser
    $configValues.ssh_public_key = $SelectedKey.PublicKey
}
if ($configCreated -and -not $configMigrated) {
    $configValues.wsl_memory = $resources.Memory
    $configValues.wsl_processors = $resources.Processors
    $configValues.wsl_swap = $resources.Swap
}
Set-ConfigValues -Path $configPath -Values $configValues
if ($configMigrated) {
    Write-Ok "updated detected values while preserving migrated configuration: $configPath"
} elseif ($configCreated) {
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
$linuxConfigPath = "$WslHome/src/otherhost/otherhost.local.conf"
$bootstrapArguments = @('bash', $trustedBootstrapPath, '--apply', '--config', $linuxConfigPath)
Invoke-WslCommand -Arguments $bootstrapArguments

Write-Step 'Building the Windows host dashboard'
$dashboardPath = Join-Path $PSScriptRoot 'build\otherhost-ui.exe'
$dashboardInUse = @(
    Get-Process -Name 'otherhost-ui' -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and ([System.IO.Path]::GetFullPath($_.Path) -eq [System.IO.Path]::GetFullPath($dashboardPath)) }
).Count -gt 0
if ($dashboardInUse) {
    Write-Warn 'The host dashboard is running; its executable will be rebuilt on the next setup or update.'
} else {
    $dashboardBuildScript = 'set -eu; cd "$OTHERHOST_REPOSITORY"; mkdir -p build; CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -o build/otherhost-ui.exe ./cmd/otherhost-ui'
    Invoke-WslCommand -Arguments @('env', "OTHERHOST_REPOSITORY=$trustedRepositoryPath", 'bash', '-c', $dashboardBuildScript)
}

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

Write-InstallationState -Revision $RepositoryRevision

$lanAddress = Get-LanAddress
Write-Host "`nOtherhost is ready." -ForegroundColor Green
if ($lanAddress) {
    Write-Host "Windows address: $lanAddress"
} else {
    Write-Warn 'The LAN address could not be detected; use ipconfig to find it'
}
Write-Host "`nNext:"
Write-Host '  1. On Windows, run: .\host-ui.cmd'
Write-Host '  2. In Host setup, enable pairing for two minutes'
Write-Host '  3. On the Mac, run: otherhost pair'
Write-Host "`nDocker is optional for SSH connectivity. If needed, enable Docker Desktop > Resources > WSL Integration > $Distro."
