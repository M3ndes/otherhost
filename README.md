<p align="center">
  <img src="docs/assets/otherhost-icon.png" width="112" alt="Otherhost otter using a laptop">
</p>

<h1 align="center">otherhost</h1>

<p align="center"><strong>Make the other host feel local.</strong></p>
<p align="center"><code>localhost → otherhost</code></p>

<p align="center">
  <a href="https://github.com/M3ndes/otherhost/actions/workflows/ci.yml"><img src="https://github.com/M3ndes/otherhost/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue.svg" alt="MIT License"></a>
</p>

Otherhost turns a Windows desktop into a private WSL 2 development machine for
your Mac. Source code, builds, containers, and databases stay on the more
powerful host while the Mac remains the editor, terminal, and browser.

Pair the computers on your local network, connect through pinned SSH, and work
without a cloud relay, project account, or copied private key.

<p align="center">
  <img src="docs/assets/screenshots/dashboard-workspace.jpg" width="1200" alt="Otherhost projects, editor actions, integrated terminal, and fictional remote machine capacity">
</p>

<p align="center"><em>The product view uses an intentionally fictional host, projects, terminal output, and hardware.</em></p>

> **Project status:** Otherhost is under active development before `1.0`.
> Portable scripts and the pairing protocol run in CI; complete Windows, WSL,
> and macOS behavior still requires validation on real machines.

## Why Otherhost?

- **Keep the Mac responsive.** Run compilation, containers, databases, and
  other heavy workloads on the Windows host.
- **Work project-first.** Discover WSL repositories and open them with Codex,
  Claude Code, VS Code, or the integrated terminal.
- **See the remote environment.** Inspect CPU, memory, graphics, disks, WSL
  allocation, and project state from a localhost-only dashboard.
- **Control both sides.** Pause or reconnect a Mac without losing its pairing,
  and use the Windows host view to prepare the machine, enable pairing, review
  authorized clients, and revoke access.
- **Stay private.** Use public-key SSH, pinned host identity, encrypted pairing,
  and local-only service tunnels—without a hosted control plane.
- **Keep control.** The CLI, configuration, setup scripts, and protocol are open
  source and intentionally inspectable.

## How it works

The Windows computer is the **host**. It owns WSL, source files, development
tools, and Docker containers. The Mac is the **client** and reaches those
resources through a hardened SSH connection.

```mermaid
flowchart LR
    Mac["Mac<br/>editor · browser · terminal"]
    Pair["Local pairing<br/>compare six digits"]
    WSL["Windows + WSL 2<br/>code · builds · containers"]

    Mac -.-> Pair
    Pair --> WSL
    Mac -->|"pinned SSH + localhost tunnels"| WSL
```

Pairing is temporary and installs only the Mac's public SSH key. Daily work uses
the saved, pinned SSH connection. Read [How it works](docs/how-it-works.md) for
the network and trust model in plain language.

## Quick start

You need Windows 11 with WSL 2 and Ubuntu, a Mac with Git and OpenSSH, and both
computers on the same trusted local network during pairing. See the complete
[Windows](docs/windows-wsl.md) and [macOS](docs/macos.md) requirements when
preparing a new machine.

### 1. Prepare Windows

Clone the repository on Windows, open PowerShell in the checkout, and run:

```powershell
.\setup.cmd
```

The command checks the host, shows the planned changes, asks for confirmation,
and configures Windows, WSL, the firewall, and SSH. Use `.\setup.cmd -Check`
for a read-only preflight.

After setup, open the Windows host console with:

```powershell
.\host-ui.cmd
```

Its guided checklist can rerun host configuration, open the temporary pairing
window, and show authorized Macs and active SSH sessions.

### 2. Install the Mac client

```bash
git clone https://github.com/M3ndes/otherhost.git
cd otherhost
./scripts/bootstrap-mac.sh --apply
```

### 3. Pair once

Start a two-minute discovery window on Windows, then pair from the Mac:

```text
Windows                         Mac
.\setup.cmd -Pair               otherhost pair

Pairing code: 482 731          Pairing code: 482 731
                 compare and confirm
```

Confirm only when the device names and six-digit codes match. If discovery does
not work, follow the [pairing decision guide](docs/troubleshooting.md#otherhost-pair-finds-no-windows-otherhost).

### 4. Connect and work

```bash
otherhost connect
```

Keep that command running to maintain configured localhost tunnels. In another
terminal, open the project dashboard:

```bash
otherhost ui
```

The dashboard runs only on `127.0.0.1:7842`. It discovers primary Git checkouts
inside WSL, opens projects with Codex, Claude Code, or VS Code, provides an
interactive remote shell, and shows the host resources available to development
workloads. See the [dashboard guide](docs/dashboard.md) for its complete behavior
and security boundaries.

To keep the dashboard running across terminal closures and Mac restarts, use
Docker Desktop instead:

```bash
make docker-check
make docker-up
```

The container has an `unless-stopped` restart policy, still publishes only on
Mac loopback, and receives the configuration, dedicated SSH identity, and pinned
known-hosts file as individual read-only mounts. See the
[macOS Docker service](docs/macos.md#keep-the-dashboard-running-with-docker).

For daily use, install the native macOS connection service so the SSH tunnels
recover after a network interruption, login, or restart:

```bash
otherhost service --check
otherhost service --apply
otherhost service status
```

The LaunchAgent keeps the same pinned, localhost-only SSH connection alive and
stores its diagnostics in `~/Library/Logs/otherhost`. The dashboard's
**Disconnect** and **Reconnect** actions pause or resume that supervisor while
preserving the saved key and pinned host identity. It is independent of the
Docker service that keeps the dashboard available.

Check whether the Mac, Windows, and WSL installations still agree before
updating either computer:

```bash
otherhost update
otherhost update --apply
```

The check is read-only. Applying on the Mac permits only a clean, canonical
`main` checkout and a fast-forward update. When Windows or WSL needs an update,
run `.\setup.cmd -Update` from the Windows checkout so the reviewed Windows
revision and operational WSL clone advance together.

## Commands

| Command | Where | Purpose |
| --- | --- | --- |
| `.\setup.cmd` | Windows | Check and configure the Windows + WSL host |
| `.\setup.cmd -Check` | Windows | Run the host preflight without making changes |
| `.\setup.cmd -Pair` | Windows | Make the host discoverable for two minutes |
| `.\setup.cmd -Update` | Windows | Fast-forward Otherhost and repin WSL to the same revision |
| `.\host-ui.cmd` | Windows | Open host setup, authorized clients, and active connections |
| `otherhost pair` | Mac | Discover, verify, and save a host |
| `otherhost doctor` | Mac | Validate configuration, keys, dependencies, and SSH |
| `otherhost connect` | Mac | Keep configured SSH port forwards open |
| `otherhost disconnect` | Mac | Pause managed tunnels without deleting the saved pairing |
| `otherhost reconnect` | Mac | Verify the saved host and resume managed tunnels |
| `otherhost service --check\|--apply` | Mac | Review or install the persistent launchd tunnel service |
| `otherhost service status\|logs\|remove` | Mac | Operate the persistent connection service |
| `otherhost status` | Mac | Show remote uptime, memory, disk, and Docker usage |
| `otherhost urls` | Mac | Print forwarded localhost URLs |
| `otherhost ssh-config [--apply]` | Mac | Review or install the managed SSH host block |
| `otherhost update [--apply]` | Mac | Compare revisions or safely fast-forward the Mac client |
| `otherhost version` | Mac | Print the client revision and compatibility version |
| `otherhost ui` | Mac | Open the client projects, terminal, machine, and connection dashboard |

## Security and privacy

- Pairing encrypts the exchange and binds it to the six-digit code shown on both
  computers.
- Only the Mac's public SSH key crosses the network; the private key never
  leaves the Mac.
- The authenticated WSL host key is pinned, and SSH password and root login are
  disabled.
- Dashboard, terminal, and forwarded services bind to Mac localhost rather than
  LAN interfaces.
- Configuration is parsed as untrusted data and is never executed as shell or
  PowerShell code.

Do not expose the SSH or pairing ports through a public router. Read the full
[security policy and trust model](SECURITY.md) before changing network or
deployment boundaries.

## Documentation

| Guide | Start here when... |
| --- | --- |
| [How it works](docs/how-it-works.md) | You want the product, network, and trust model in plain language |
| [Windows and WSL host](docs/windows-wsl.md) | You are preparing or operating the Windows computer |
| [macOS client](docs/macos.md) | You are installing, pairing, or using the Mac command |
| [Project dashboard](docs/dashboard.md) | You want project discovery, editor actions, terminal, and machine details |
| [Troubleshooting](docs/troubleshooting.md) | Setup, discovery, SSH, Docker, or the dashboard failed |
| [Migration from devbox-bridge](docs/migration.md) | You installed the project before the Otherhost rename |
| [Architecture](docs/architecture.md) | You want protocol details or plan a code change |
| [Brand](docs/brand.md) | You need the name, voice, logo, or visual tokens |
| [Contributing](CONTRIBUTING.md) | You want to report, test, document, or implement a change |

## Scope

Otherhost creates a trusted remote-development path. It does not install Docker
Desktop, expose the host publicly, synchronize source code or private keys,
provide a cloud relay, manage operating-system updates, or replace a private
network when the machines are in different locations.

## Contributing

Issues and focused pull requests are welcome. Redact private keys, tokens,
usernames, hostnames, and LAN addresses from logs, and run `make test` before
submitting code. See [CONTRIBUTING.md](CONTRIBUTING.md) for the complete workflow.

Licensed under the [MIT License](LICENSE).
