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

## Host bootstrap

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
