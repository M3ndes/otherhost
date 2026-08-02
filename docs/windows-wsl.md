# Windows and WSL host

## One-command setup

From PowerShell in a Windows checkout of the repository, run:

```powershell
.\setup.cmd
```

This is the recommended path. It runs the checks below, displays the planned
changes, asks for one confirmation, elevates through UAC, and orchestrates both
Windows and Ubuntu. The checkout used to launch it may live on Windows; the
script pins the operational clone under `~/src` in WSL to that checkout's exact
Git revision. Privileged bootstrap code runs from the reviewed Windows checkout,
and setup refuses local changes that are not part of that revision.

Normal host setup does not require a GitHub account or Mac key. After setup,
enable two-minute private-network discovery:

```powershell
.\setup.cmd -Pair
```

## Existing mirrored WSL without elevation

If Ubuntu is already installed, PID 1 is systemd, and mirrored networking is
already active, use the WSL user-scoped host instead of Windows setup:

```bash
cd ~/src/devbox-bridge
./scripts/bootstrap-wsl-user.sh --apply
./scripts/pair-wsl.sh
```

This path never invokes PowerShell, `sudo`, or an executable installer. It
extracts OpenSSH from Ubuntu's signed packages into `~/.local`, runs it through
the systemd user manager, permits public-key authentication only, and limits
port forwarding to loopback services. It does not install WSL, change the
Windows firewall, or persist when the WSL user manager is not running.
The Mac automatically falls back to bounded local-subnet discovery if mirrored
WSL networking does not receive multicast probes.

Windows displays the requesting Mac name and a six-digit code. Confirm only if
the Mac displays the same code. Temporary Windows Firewall rules are limited to
the pairing helper, the active profile, and the active IPv4 subnet. The launcher
does not change Public or Private network categories. The temporary rules are
removed on success, rejection, error, or timeout.

If direct WSL discovery is blocked by Windows, run `setup.cmd -Pair`. The same
command recognizes the user-scoped SSH host, opens its persistent SSH port only
for the active subnet through the Hyper-V firewall, and hosts the two-minute
pairing listener on Windows.

The GitHub key path remains available for manual recovery. It requires an
explicit account and exact fingerprint and never infers identity from the
repository owner:

```powershell
.\setup.cmd -GitHubUser YOUR_USER -GitHubKeyFingerprint SHA256:YOUR_FINGERPRINT -Yes
```

Use `.\setup.cmd -Check` for a read-only preflight. The sections below document
the individual layers and provide the manual fallback when troubleshooting.

## Recommended layout

- Windows 11 22H2 or newer;
- current Store version of WSL 2;
- Ubuntu with systemd;
- Docker Desktop with **Use the WSL 2 based engine** enabled;
- repositories under `~/src`, never `/mnt/c`, for Linux workloads.

Do not install another Docker Engine inside the same distribution when using
Docker Desktop integration. Competing daemons and CLIs make failures harder to
diagnose.

This release automates mirrored networking only. WSL's default NAT mode requires
a port proxy that follows the distribution's changing address and is intentionally
left for a later release.

## Manual first-run checklist

### 1. Check Windows and WSL

Run from PowerShell:

```powershell
winver
wsl --version
wsl --list --verbose
```

Expected result: Windows build 22621 or newer and an Ubuntu distribution with
version `2`. If Ubuntu is missing, install it with `wsl --install -d Ubuntu`,
restart when requested, and complete Ubuntu's first-launch user setup.

### 2. Prepare the WSL clone

Run inside Ubuntu:

```bash
sudo apt-get update
sudo apt-get install -y git
whoami
mkdir -p ~/src
cd ~/src
git clone https://github.com/M3ndes/devbox-bridge.git
cd devbox-bridge
cp config/devbox.example.conf devbox.local.conf
```

Use the `whoami` result as `ssh_user`. Leave `github_user` and `ssh_public_key`
empty when using secure pairing. Keep repositories under `~/src`; cloning under
`/mnt/c` causes slower Linux filesystem access and poor bind-mount behavior.

Review these configuration values before continuing:

| Setting | Meaning | Example |
| --- | --- | --- |
| `ssh_user` | Ubuntu user returned by `whoami` | `developer` |
| `ssh_port` | SSH port allowed through the Hyper-V firewall | `2222` |
| `github_user` | Optional recovery profile used to discover candidate keys | `M3ndes` |
| `ssh_public_key` | Optional exact recovery public key line | `ssh-ed25519 AAAA... devbox-bridge client` |
| `wsl_distribution` | Exact name shown by `wsl --list --quiet` | `Ubuntu` |
| `wsl_memory` | Maximum RAM assigned to WSL | `20GB` |
| `wsl_processors` | Logical processors assigned to WSL | `8` |
| `ports` | Application ports forwarded to the Mac | `3000,3001,8000,8080` |

### 3. Check and apply the Windows policy

Open Administrator PowerShell. Resolve the WSL path without manually replacing a
username placeholder:

```powershell
Set-ExecutionPolicy -Scope Process Bypass
$Distro = "Ubuntu"
$WslUser = (& wsl.exe -d $Distro -- whoami).Trim()
$RepoPath = "\\wsl.localhost\$Distro\home\$WslUser\src\devbox-bridge"

& "$RepoPath\scripts\bootstrap-windows.ps1" `
  -Mode Check `
  -ConfigPath "$RepoPath\devbox.local.conf"
```

The check is read-only. Confirm the reported Windows build, RAM, logical
processors, WSL distribution, and Docker state. Then apply:

```powershell
& "$RepoPath\scripts\bootstrap-windows.ps1" `
  -Mode Apply `
  -ConfigPath "$RepoPath\devbox.local.conf"
```

This creates a timestamped `.wslconfig` backup when necessary and ends with
`wsl --shutdown`.

### 4. Check and apply the Ubuntu policy

Start Docker Desktop and Ubuntu again, then run inside Ubuntu:

```bash
cd ~/src/devbox-bridge
./scripts/bootstrap-wsl.sh
./scripts/bootstrap-wsl.sh --apply
```

If the script enables systemd, run `wsl --shutdown` from PowerShell, reopen
Ubuntu, and run `./scripts/bootstrap-wsl.sh --apply` once more.

### 5. Verify each layer

Inside Ubuntu:

```bash
docker info
systemctl is-active ssh
ss -lnt | grep 2222
df -h /
free -h
```

Expected result: Docker returns server information, SSH is `active`, and TCP port
2222 is listening. Run `.\setup.cmd -Pair` on Windows and `devbox pair` on the
Mac. Discovery supplies the correct active Windows address automatically.

## Resource policy

The example assigns 20 GB RAM, 8 processors, and 8 GB swap to WSL. Adjust these
values to the desktop. The Windows bootstrap refuses a memory limit that would
leave Windows less than 8 GB.

It merges these settings into `%UserProfile%\.wslconfig`, preserving unrelated
content and creating a timestamped backup:

```ini
[wsl2]
memory=20GB
processors=8
swap=8GB
networkingMode=mirrored

[experimental]
autoMemoryReclaim=gradual
sparseVhd=true
```

`wsl --shutdown` is required after changing this file. It stops all WSL
distributions and their current workloads.

## Host bootstrap behavior

Run `bootstrap-windows.ps1 -Mode Check` without elevation for a read-only report.
Use `-Mode Apply` from Administrator PowerShell to update `.wslconfig` and create
the inbound Hyper-V firewall rule for the SSH port.

Then run `bootstrap-wsl.sh --apply` inside the configured distribution. It
installs OpenSSH, prepares `authorized_keys`, installs an early hardening
drop-in, checks the effective `sshd -T` policy, and only then enables the SSH
service. When a recovery `ssh_public_key` is configured, it validates and
authorizes only that exact key. Its optional project clone uses
`--recurse-submodules` and never overwrites an existing directory.

## Docker Desktop

In Docker Desktop, enable the WSL 2 engine and integration for the configured
distribution. Source and Compose files should be cloned inside WSL so bind mounts
refer to paths visible beside the Docker engine.

Relevant upstream documentation:

- [Docker Desktop WSL 2 backend](https://docs.docker.com/desktop/features/wsl/)
- [Docker WSL best practices](https://docs.docker.com/desktop/features/wsl/best-practices/)
- [Advanced WSL configuration](https://learn.microsoft.com/en-us/windows/wsl/wsl-config)
- [WSL networking](https://learn.microsoft.com/en-us/windows/wsl/networking)
- [Hyper-V firewall rule command](https://learn.microsoft.com/en-us/powershell/module/netsecurity/new-netfirewallhypervrule)
- [Working across Windows and Linux filesystems](https://learn.microsoft.com/en-us/windows/wsl/filesystems)
- [Use systemd in WSL](https://learn.microsoft.com/en-us/windows/wsl/systemd)

## Operational checks

The desktop must be powered on, awake, connected to the network, and running
Docker Desktop. Configure Windows power settings according to whether unattended
access is worth the energy use. Test LAN access before adding any remote-access
overlay network.

Useful commands inside WSL:

```bash
systemctl status ssh
ss -lnt | grep 2222
docker info
docker system df
df -h /
free -h
```

If an expected result is missing, use the layer-by-layer
[troubleshooting guide](troubleshooting.md) before applying a workaround.
