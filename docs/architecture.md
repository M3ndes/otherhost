# Architecture and decisions

## CLI first

The first release uses small Bash and PowerShell entry points. This makes every
change reviewable, works over SSH, and avoids a desktop application's packaging,
code-signing, permission, and update lifecycle.

A future TUI can call the same commands after real usage shows which operations
need guided interaction. A web or native UI should only be considered if the
project needs continuous dashboards, multiple devboxes, or non-technical users.

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
Administrator. SSH accepts public-key authentication only and rejects root login.

## Configuration model

`devbox.local.conf` is a portable `key=value` file. Bash and PowerShell both parse
it as inert text. Values are deliberately limited to plain strings; quoting,
variable expansion, and command substitution are unsupported.

Each clone has its own ignored config. Public SSH keys may be imported from a
GitHub profile, but private keys and secrets never cross machines through Git.
