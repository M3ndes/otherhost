# Troubleshooting

Work from the host outward: Windows, WSL, Docker and SSH, then the Mac client.
Avoid changing several layers at once; rerun the relevant check after each fix.

## Collect a safe diagnostic snapshot

From PowerShell:

```powershell
Get-ComputerInfo -Property WindowsProductName, WindowsVersion, OsBuildNumber
wsl --version
wsl --list --verbose
ipconfig
if (Get-Command Get-NetFirewallHyperVRule -ErrorAction SilentlyContinue) {
    Get-NetFirewallHyperVRule -Name devbox-bridge-ssh -ErrorAction SilentlyContinue
}
```

From Ubuntu:

```bash
uname -a
ps -p 1 -o comm=
systemctl status ssh --no-pager
ss -lnt | grep 2222
docker info
docker system df
df -h /
free -h
```

From the Mac:

```bash
devbox pair
devbox doctor
nc -vz YOUR_WINDOWS_IP 2222
```

Before posting output publicly, remove LAN addresses, usernames, hostnames, and
project paths. Never include private keys, tokens, `.env` files, or application
logs containing credentials.

## `devbox pair` finds no Windows devbox

Start `.\setup.cmd -Pair` on Windows first. Discovery remains active for two
minutes. Confirm that both devices use the same private LAN, the active Windows
network profile is **Private**, and the Wi-Fi access point does not isolate
wireless clients.

Pairing first uses IPv4 multicast UDP port `45870` for discovery. If multicast
is suppressed, the Mac automatically probes TCP port `45871` on a bounded set of
addresses in its local IPv4 subnet. The same TCP port carries the short-lived
encrypted session. Windows creates local-subnet firewall rules only while the
pairing command runs. Do not expose either port through a router.

If neither method finds the host, disconnect VPN software and verify that the
access point does not isolate wireless clients. The explicit GitHub public-key
recovery flow remains documented in [macOS client](macos.md).

## The pairing codes do not match

Choose **No** on both devices. Pairing stops without installing a key. Restart
pairing from Windows and compare a new code. Never proceed after a mismatch;
different values indicate that the session transcripts or ephemeral keys differ.

## Pairing succeeds but `devbox doctor` reports a host-key error

Pairing pins the WSL Ed25519 host key in
`~/.ssh/devbox_bridge_known_hosts`. A mismatch means the WSL host identity or
address changed after pairing. Verify the host locally before removing the
pinned entry, then pair again. Do not disable strict host-key checking.

## `git: command not found` inside WSL

The repository cannot bootstrap Git before it has been cloned. Install the one
initial dependency manually:

```bash
sudo apt-get update
sudo apt-get install -y git
```

Clone into `~/src`, not `/mnt/c`.

## PowerShell cannot find the repository path

Confirm that Ubuntu is running and that its registered name matches the config:

```powershell
wsl --list --quiet
$Distro = "Ubuntu"
$WslUser = (& wsl.exe -d $Distro -- whoami).Trim()
Test-Path "\\wsl.localhost\$Distro\home\$WslUser\src\devbox-bridge"
```

Use the exact distribution name returned by WSL. Opening Ubuntu once also ensures
that its Linux user and home directory have been created.

## Mirrored networking is rejected

Mirrored networking requires Windows 11 22H2, build 22621 or newer, and a current
Store version of WSL. Check `winver` and `wsl --version`, then install Windows and
WSL updates before applying the configuration again.

This release does not automate the NAT-mode port proxy. Do not switch to NAT and
assume the Mac will still reach SSH.

## Docker is unavailable inside Ubuntu

Open Docker Desktop and verify:

1. **Settings > General > Use the WSL 2 based engine** is enabled.
2. **Settings > Resources > WSL Integration** enables the configured Ubuntu
   distribution.
3. `docker info` works inside Ubuntu after Docker Desktop restarts.

Do not install a second Docker daemon inside Ubuntu to work around a disabled
Docker Desktop integration.

## systemd is inactive

Inspect the WSL configuration:

```bash
cat /etc/wsl.conf
ps -p 1 -o comm=
```

After the bootstrap adds `[boot]` and `systemd=true`, run `wsl --shutdown` from
PowerShell and reopen Ubuntu. PID 1 should then be `systemd`.

## SSH times out from the Mac

A timeout normally points to the address, network, or firewall layer.

Inside Ubuntu:

```bash
systemctl is-active ssh
ss -lnt | grep 2222
```

On Windows, confirm the firewall rule and active LAN address:

```powershell
Get-NetFirewallHyperVRule -Name devbox-bridge-ssh
ipconfig
```

On the Mac, make sure `host` uses the active Windows Ethernet or Wi-Fi IPv4
address, both machines are on the same trusted network, and then test:

```bash
nc -vz YOUR_WINDOWS_IP 2222
```

## SSH says `Permission denied (publickey)`

Compare the dedicated Mac fingerprint with the exact key selected in WSL:

```bash
# Mac
ssh-keygen -lf ~/.ssh/devbox_bridge_ed25519.pub -E sha256

# Ubuntu / WSL
ssh-keygen -lf ~/.ssh/authorized_keys -E sha256
grep '^ssh_public_key=' ~/src/devbox-bridge/devbox.local.conf
ls -ld ~/.ssh
ls -l ~/.ssh/authorized_keys
```

The `.ssh` directory should normally be mode `700` and `authorized_keys` mode
`600`. On the Mac, confirm `identity_file` points to the dedicated private key.
Never copy that private key into WSL or GitHub.

## A forwarded port is already in use on the Mac

Find the process holding the local port:

```bash
lsof -nP -iTCP:3000 -sTCP:LISTEN
```

Stop that process or choose a different unused port in `devbox.local.conf`. Keep
the configured port aligned with the service listening inside WSL.

## Disk or memory pressure returns

Inspect before deleting anything:

```bash
docker system df
df -h /
free -h
```

Do not run broad Docker prune commands until you have reviewed which images,
containers, volumes, and build cache are safe to remove.
