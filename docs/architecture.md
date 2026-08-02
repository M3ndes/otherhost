# Architecture and decisions

## CLI first

The project uses small Bash and PowerShell entry points. This makes host changes
reviewable and keeps diagnostics available without a graphical application.

`setup.cmd` is the thin Windows entry point. Its PowerShell orchestrator composes
the Windows and WSL bootstraps; those lower-level commands remain available for
diagnostics and manual recovery.

The pairing helper is a small Go binary distributed for Windows, WSL Linux, and
Intel and Apple Silicon Macs. PowerShell and Bash own policy and orchestration.
The helper owns only local discovery and the short-lived cryptographic pairing
protocol.

## Device pairing

Pairing uses a numeric-comparison interaction:

1. `setup.cmd -Pair` opens one TCP and one UDP Windows Firewall rule for the
   private local subnet, or `scripts/pair-wsl.sh` listens directly in an existing
   mirrored-network WSL user session. Either mode is discoverable for two minutes.
2. `devbox pair` sends a versioned IPv4 multicast request. If WSL does not
   receive multicast, the same command probes the fixed pairing port on a
   bounded set of addresses in the Mac's local IPv4 subnet and lists matching
   Windows hosts.
3. The devices exchange fresh X25519 public keys and 256-bit random nonces.
4. Both derive separate AES-256-GCM direction keys and a six-digit comparison
   value from the complete transcript using HKDF-SHA-256.
5. The same value is displayed on both devices. Both users must confirm it.
6. The Mac public SSH key and host connection details travel in authenticated,
   encrypted messages only after confirmation.
7. The Mac pins the authenticated WSL Ed25519 host key before its first SSH
   connection.

The six digits are not a password and are never sent over the network. They bind
the device names, discovery instance, session identifier, ephemeral keys, and
nonces. A man-in-the-middle has approximately a one-in-one-million chance per
user-approved attempt of presenting a matching value. Pairing permits one active
session and closes after success, rejection, or timeout.

The direct discovery endpoint reveals only the same temporary instance, device
name, and port as multicast discovery. It accepts local-network requests only
and closes with pairing mode. The helper listens only while pairing is enabled.
Windows mode limits temporary firewall rules to the executable, private network
profiles, and `LocalSubnet`, then removes them in a `finally` block. WSL user
mode creates no Windows rules.

## Source and compute location

Projects live inside the WSL Linux filesystem, normally under `~/src`. Builds,
dependency installation, Git operations, and containers run on the desktop. The
Mac remains the interactive client.

This matters for Docker Compose projects with bind mounts: a remote Docker engine
alone is insufficient because the source paths must exist beside the engine.

## Network model

The Mac reaches OpenSSH inside WSL. `devbox connect` forwards selected service
ports to the Mac loopback interface. Binding only to `127.0.0.1` keeps forwarded
applications off the Mac's LAN interfaces.

Windows mirrored networking gives WSL direct LAN reachability. The bootstrap adds
one inbound Hyper-V firewall rule for the configured SSH port when run as
Administrator. SSH accepts public-key authentication only, rejects root login,
and uses the host identity pinned during pairing.

On a host where mirrored networking and inbound policy already permit WSL LAN
traffic, `bootstrap-wsl-user.sh` extracts Ubuntu's signed OpenSSH packages into
the user home and runs `sshd` as a systemd user service on an unprivileged port.
That mode requires neither Windows elevation nor Linux `sudo`; it restricts port
forwarding destinations to WSL loopback addresses.

## Configuration model

`devbox.local.conf` is a portable `key=value` file. Bash and PowerShell parse it
as inert text. Values are deliberately limited to plain strings; quoting,
variable expansion, and command substitution are unsupported.

Each clone has its own ignored config. Secure pairing is the default key
exchange. Windows may still discover public keys from an explicitly selected
GitHub profile during manual recovery, but setup authorizes only the confirmed
fingerprint. Private keys and secrets never cross devices.

The Windows launcher records its exact Git revision and pins the operational WSL
clone to it. The privileged WSL bootstrap runs from the same Windows checkout
that launched setup, preventing a stale or modified second checkout from becoming
an implicit code source at the `sudo` boundary.
