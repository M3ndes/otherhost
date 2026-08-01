# Windows and WSL host

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

## First-run checklist

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

Use the `whoami` result as `ssh_user`. Set `github_user` to the GitHub profile
containing the Mac's public key. Keep repositories under `~/src`; cloning under
`/mnt/c` causes slower Linux filesystem access and poor bind-mount behavior.

Review these configuration values before continuing:

| Setting | Meaning | Example |
| --- | --- | --- |
| `ssh_user` | Ubuntu user returned by `whoami` | `developer` |
| `ssh_port` | SSH port allowed through the Hyper-V firewall | `2222` |
| `github_user` | GitHub profile used to import public SSH keys | `M3ndes` |
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
2222 is listening. On Windows, run `ipconfig` and note the IPv4 address of the
active Ethernet or Wi-Fi adapter. Use that address as `host` in the Mac's local
config, then run `devbox doctor` on the Mac.

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

Then run `bootstrap-wsl.sh --apply` inside the configured distribution. It installs
OpenSSH, imports public keys from `https://github.com/USERNAME.keys`, disables SSH
passwords, and enables the SSH service. Its optional project clone uses
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
