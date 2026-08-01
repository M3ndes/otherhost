function Normalize-SshPublicKey {
    param(
        [Parameter(Mandatory = $true)]
        [string]$PublicKey
    )

    if ($PublicKey -match '[\r\n]') { throw 'SSH public key must contain exactly one line' }
    $parts = @($PublicKey.Trim() -split '\s+', 3)
    if ($parts.Count -lt 2) { throw 'SSH public key is incomplete' }
    if ($parts[0] -notmatch '^(ssh-ed25519|ssh-rsa|ecdsa-sha2-[A-Za-z0-9@._+-]+)$') {
        throw "Unsupported SSH public key type: $($parts[0])"
    }

    try {
        $decoded = [Convert]::FromBase64String($parts[1])
    } catch {
        throw 'SSH public key payload is not valid base64'
    }
    if ($decoded.Length -eq 0) { throw 'SSH public key payload is empty' }

    return "$($parts[0]) $($parts[1])"
}

function Get-SshPublicKeyFingerprint {
    param(
        [Parameter(Mandatory = $true)]
        [string]$PublicKey
    )

    $normalized = Normalize-SshPublicKey -PublicKey $PublicKey
    $parts = @($normalized -split '\s+', 3)
    $keyBytes = [Convert]::FromBase64String($parts[1])
    $sha256 = [Security.Cryptography.SHA256]::Create()
    try {
        $digest = $sha256.ComputeHash($keyBytes)
    } finally {
        $sha256.Dispose()
    }
    return 'SHA256:' + [Convert]::ToBase64String($digest).TrimEnd('=')
}

function Get-GitHubPublicKeys {
    param(
        [Parameter(Mandatory = $true)]
        [ValidatePattern('^[A-Za-z0-9-]+$')]
        [string]$GitHubUser
    )

    try {
        $response = Invoke-WebRequest -UseBasicParsing -Uri "https://github.com/$GitHubUser.keys"
    } catch {
        throw "Could not download public SSH keys for GitHub user $GitHubUser`: $($_.Exception.Message)"
    }

    $keys = @()
    $seen = @{}
    foreach ($line in ([string]$response.Content -split "`r?`n")) {
        if (-not $line.Trim()) { continue }
        $normalized = Normalize-SshPublicKey -PublicKey $line
        $fingerprint = Get-SshPublicKeyFingerprint -PublicKey $normalized
        if ($seen.ContainsKey($fingerprint)) { continue }
        $seen[$fingerprint] = $true
        $keys += [pscustomobject]@{
            PublicKey = $normalized
            Fingerprint = $fingerprint
            Comment = ''
        }
    }

    if ($keys.Count -eq 0) { throw "GitHub returned no usable SSH keys for $GitHubUser" }
    return @($keys)
}

function Select-SshPublicKey {
    param(
        [Parameter(Mandatory = $true)]
        [object[]]$Keys,
        [string]$Fingerprint = ''
    )

    if ($Keys.Count -eq 0) { throw 'No SSH public keys are available for selection' }
    if ($Fingerprint) {
        $matches = @($Keys | Where-Object { $_.Fingerprint -ceq $Fingerprint })
        if ($matches.Count -ne 1) { throw "SSH key fingerprint was not found: $Fingerprint" }
        return $matches[0]
    }
    if ($Keys.Count -eq 1) { return $Keys[0] }
    return $null
}
