# macOS client

## Install

Run `./scripts/bootstrap-mac.sh --apply` from the repository. It:

1. links `devbox` into `~/.local/bin`;
2. creates an ignored local config when needed;
3. installs the checksum-verified pairing helper for the Mac architecture;
4. leaves existing configuration and keys untouched.

Add `~/.local/bin` to `PATH` if the script reports it missing:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Place that line in `~/.zshrc` to keep it across terminal sessions.

## Pair with Windows

First run `.\setup.cmd -Pair` in the Windows checkout. Then run:

```bash
devbox pair
```

The Mac searches the private local network for five seconds. One result is
selected automatically; several results are shown as a numbered list. Confirm
only when the same six-digit code appears on both devices.

Successful pairing writes the authenticated Windows address, WSL user, SSH port,
and dedicated known-hosts path to `devbox.local.conf`. It then runs
`devbox doctor`, so the command also verifies the new SSH path.

## SSH identities

The default client key is `~/.ssh/devbox_bridge_ed25519`. `devbox pair` creates it
when missing. Only its `.pub` counterpart is shared. The private key never leaves
the Mac.

The paired WSL host key is saved at
`~/.ssh/devbox_bridge_known_hosts`. All paired devbox SSH commands use
`StrictHostKeyChecking=yes`, so a later host-identity change fails closed.

## Manual recovery

If local multicast is unavailable, use the explicit GitHub public-key recovery
flow. Print the dedicated key fingerprint:

```bash
ssh-keygen -lf ~/.ssh/devbox_bridge_ed25519.pub -E sha256
```

Run Windows setup with `-GitHubUser` and `-GitHubKeyFingerprint`. WSL authorizes
only that exact public key. You must then set the host values and pin its SSH
identity manually.

## Daily use

Run `devbox connect` in a dedicated terminal. The process uses SSH keepalives,
strict host-key verification, and fails immediately if a requested local forward
cannot be opened. `Ctrl-C` closes the tunnel without stopping desktop workloads.
