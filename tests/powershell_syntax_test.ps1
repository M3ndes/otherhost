$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
$scripts = @(
    (Join-Path $root 'setup.ps1'),
    (Join-Path $root 'host-ui.ps1'),
    (Join-Path $root 'lib/otherhost-windows.ps1'),
    (Join-Path $root 'scripts/bootstrap-windows.ps1')
)

foreach ($script in $scripts) {
    $tokens = $null
    $errors = $null
    [void][System.Management.Automation.Language.Parser]::ParseFile($script, [ref]$tokens, [ref]$errors)
    if ($errors.Count -gt 0) {
        $errors | ForEach-Object { Write-Error "$script`: $($_.Message)" }
        exit 1
    }
}

$launcher = [System.IO.File]::ReadAllText((Join-Path $root 'setup.cmd'))
if ($launcher -notmatch 'setup\.ps1') {
    Write-Error 'setup.cmd does not invoke setup.ps1'
    exit 1
}

$hostLauncher = [System.IO.File]::ReadAllText((Join-Path $root 'host-ui.cmd'))
foreach ($requiredHostLauncher in @('host-ui.ps1', 'ExecutionPolicy Bypass')) {
    if (-not $hostLauncher.Contains($requiredHostLauncher)) {
        Write-Error "host-ui.cmd is missing: $requiredHostLauncher"
        exit 1
    }
}
$hostDashboard = [System.IO.File]::ReadAllText((Join-Path $root 'host-ui.ps1'))
foreach ($requiredHostDashboardControl in @('--mode', 'host', '--repository', 'Get-FileHash', 'checksums.txt')) {
    if (-not $hostDashboard.Contains($requiredHostDashboardControl)) {
        Write-Error "host-ui.ps1 is missing: $requiredHostDashboardControl"
        exit 1
    }
}

. (Join-Path $root 'lib/otherhost-windows.ps1')
$keyOne = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDSZaD3EhKIadfnHAoP5FI2lDwzjk6xZ4H8vS2gFVrKe test-one'
$keyTwo = 'ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIAstAi+iQmttXXJI8elqpE0ansjPtXa3y07PoTm4hV9y test-two'
$fingerprintOne = Get-SshPublicKeyFingerprint -PublicKey $keyOne
$fingerprintTwo = Get-SshPublicKeyFingerprint -PublicKey $keyTwo
if ($fingerprintOne -eq $fingerprintTwo) {
    Write-Error 'different public keys produced the same test fingerprint'
    exit 1
}
$keys = @(
    [pscustomobject]@{ PublicKey = $keyOne; Fingerprint = $fingerprintOne; Comment = 'test-one' },
    [pscustomobject]@{ PublicKey = $keyTwo; Fingerprint = $fingerprintTwo; Comment = 'test-two' }
)
$selected = Select-SshPublicKey -Keys $keys -Fingerprint $fingerprintTwo
$normalizedKeyTwo = Normalize-SshPublicKey -PublicKey $keyTwo
if ((Normalize-SshPublicKey -PublicKey $selected.PublicKey) -cne $normalizedKeyTwo) {
    Write-Error 'fingerprint selection did not return exactly the selected public key'
    exit 1
}
if ($null -ne (Select-SshPublicKey -Keys $keys)) {
    Write-Error 'multiple public keys were selected without an explicit fingerprint'
    exit 1
}
try {
    [void](Normalize-SshPublicKey -PublicKey "${keyOne}`n${keyTwo}")
    Write-Error 'multiline public key input was accepted'
    exit 1
} catch {
    if ($_.Exception.Message -notmatch 'exactly one line') { throw }
}

$setup = [System.IO.File]::ReadAllText((Join-Path $root 'setup.ps1'))
if ($setup -match 'Get-InferredGitHubUser') {
    Write-Error 'setup still infers an SSH identity from the repository owner'
    exit 1
}
foreach ($requiredControl in @(
    'Get-GitHubPublicKeys',
    'ssh_public_key',
    'rev-parse --verify HEAD',
    "'status', '--porcelain', '--untracked-files=all'",
    'Invoke-CapturedProcess -FilePath $gitExecutable',
    'checkout --detach',
    'wslpath -a -u',
    'Invoke-WslCommand -Arguments $bootstrapArguments',
    '[switch]$Pair',
    'Start-PairingMode',
    'Start-PairingTranscript',
    "'pairing-latest.log'",
    'Get-ActivePairingNetworkPolicy',
    'Ensure-HyperVSSHRule',
    'pairing helper:',
    'Invoke-CapturedProcess -FilePath $helper',
    'does not support user-scoped WSL',
    '-RemoteAddress $networkPolicy.RemoteSubnets -Profile $networkPolicy.Profiles',
    '-RemoteAddresses $RemoteSubnets',
    '--user-scoped-wsl',
    'Remove-NetFirewallRule -Name $discoveryRule',
    'Remove-NetFirewallRule -Name $sessionRule',
    'Get-FileHash -Algorithm SHA256',
    'Building the Windows host dashboard',
    'OTHERHOST_REPOSITORY=$trustedRepositoryPath'
)) {
    if (-not $setup.Contains($requiredControl)) {
        Write-Error "setup is missing required security control: $requiredControl"
        exit 1
    }
}
if ($setup.Contains("Read-Host 'GitHub username containing the Mac public key'")) {
    Write-Error 'normal setup still requires a GitHub username'
    exit 1
}
if ($setup.Contains('cd "$HOME/src/otherhost" && ./scripts/bootstrap-wsl.sh --apply')) {
    Write-Error 'setup still executes the privileged bootstrap from the secondary WSL checkout'
    exit 1
}
if ($setup.Contains('-- bash -lc $Script')) {
    Write-Error 'setup passes a multiline script through Windows native argument quoting'
    exit 1
}
foreach ($requiredWslHandoff in @(
    '$Script.Replace("`r`n", "`n").Replace("`r", "`n")',
    '[System.Text.UTF8Encoding]::new($false)',
    '"\\wsl.localhost\$Distro" + $wslScriptPath.Replace(''/'', ''\'')',
    '-- bash $wslScriptPath'
)) {
    if (-not $setup.Contains($requiredWslHandoff)) {
        Write-Error "setup is missing safe WSL script handoff control: $requiredWslHandoff"
        exit 1
    }
}

Write-Host 'PowerShell syntax test passed'
