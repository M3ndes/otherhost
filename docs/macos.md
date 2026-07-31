# macOS client

## Install and configure

Run `./scripts/bootstrap-mac.sh --apply --generate-key` from the repository. It:

1. links `devbox` into `~/.local/bin`;
2. creates an ignored local config when needed;
3. creates a dedicated Ed25519 key only when explicitly requested;
4. leaves existing config and keys untouched.

Add `~/.local/bin` to `PATH` if the script reports it missing:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Place that line in `~/.zshrc` to keep it across terminal sessions.

## Choose the host address

On the Windows desktop, run `ipconfig` and use the active Ethernet or Wi-Fi IPv4
address. A DHCP reservation on the home router is the simplest way to keep it
stable. A local DNS name or private overlay-network address can also be used.

## SSH key flow

The default key is `~/.ssh/devbox_bridge_ed25519`. Only its `.pub` counterpart is
shared. The WSL bootstrap downloads public keys published on the configured GitHub
profile. Remove an obsolete key from both GitHub and `~/.ssh/authorized_keys` in
WSL.

## Daily use

Run `devbox connect` in a dedicated terminal. The process uses SSH keepalives and
fails immediately if a requested local forward cannot be opened. `Ctrl-C` closes
the tunnel without stopping workloads on the desktop.
