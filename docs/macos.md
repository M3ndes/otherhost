# macOS client

The Mac is the interactive client: it discovers and authenticates one Windows
host, opens SSH sessions, and makes selected WSL services available on Mac
localhost ports. Source code and containers remain on the Windows/WSL host.

Start with the repository [Quick start](../README.md#quick-start) if this is your
first setup. This page documents Mac-specific behavior and recovery.

## Install

Requirements are Git, OpenSSH, and a normal macOS terminal. Clone the repository
and run the installer:

```bash
git clone https://github.com/M3ndes/otherhost.git
cd otherhost
./scripts/bootstrap-mac.sh --apply
```

The installer:

1. links `bin/otherhost` to `~/.local/bin/otherhost`;
2. creates an ignored `otherhost.local.conf` when needed;
3. installs the checksum-verified pairing helper for Intel or Apple Silicon;
4. leaves existing configuration and SSH keys untouched;
5. installs `devbox` as a temporary compatibility alias.

It fails instead of overwriting an unrelated `~/.local/bin/otherhost` file.

If the script reports that `~/.local/bin` is not in `PATH`, add this line to
`~/.zshrc`:

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Open a new terminal or load the change with `source ~/.zshrc`, then check:

```bash
otherhost help
```

Running `bootstrap-mac.sh` without `--apply` is a read-only check. The optional
`--generate-key` flag creates the dedicated SSH identity during installation;
otherwise `otherhost pair` creates it when first needed.

## Pair with Windows

Pairing is required once per Mac identity or whenever you intentionally replace
the host identity. It exchanges public information over an encrypted temporary
session; it does not copy the Mac private key.

1. On Windows, open the repository checkout, run `.\setup.cmd -Pair`, and keep
   the PowerShell window open.
2. On the Mac, run:

   ```bash
   otherhost pair
   ```

3. Check the device names and the complete six-digit code on both screens.
4. Confirm on both devices only when they match.

The Mac listens for discovery responses for a short period. It first uses local
IPv4 multicast and automatically scans a bounded local subnet on TCP port
`25371` when multicast is unavailable. One compatible host is selected
automatically; several hosts are displayed as a numbered choice.

Successful pairing writes these authenticated values to the local config:

- Windows address;
- WSL user;
- SSH port;
- dedicated known-hosts path.

It also pins the WSL Ed25519 host key and immediately runs `otherhost doctor`.
Pairing is therefore complete only when the final SSH check succeeds.

## Files created on the Mac

| File | Purpose | Safe to share? |
| --- | --- | --- |
| `~/.local/bin/otherhost` | Link to this clone's CLI | The link itself contains no secret |
| `~/.local/lib/otherhost/otherhost-pair` | Installed pairing helper | Yes, when obtained from a verified release |
| `otherhost.local.conf` | Machine-local connection settings | Treat as private diagnostic data |
| `~/.ssh/otherhost_ed25519` | Dedicated private SSH identity for new installations | **No** |
| `~/.ssh/otherhost_ed25519.pub` | Public half installed in WSL | Yes |
| `~/.ssh/otherhost_known_hosts` | Pinned WSL public host identity | Public key data, but identifies the host |

The default private key is permission-restricted and never leaves the Mac. All
paired SSH commands use `IdentitiesOnly=yes` and `StrictHostKeyChecking=yes`, so
an unexpected host identity fails closed instead of prompting the user to trust
a replacement automatically.

## Daily use

First verify the connection when the host or network has changed:

```bash
otherhost doctor
```

Open all configured tunnels:

```bash
otherhost connect
```

Keep that process in a dedicated terminal. SSH keepalives detect a lost host,
and the command fails immediately when a requested local port cannot be opened.
`Ctrl-C` closes only the SSH connection; it does not stop applications or
containers on the Windows host.

## Migrate an existing devbox-bridge client

Pull the rebrand, update the clone's remote, and rerun the installer:

```bash
git remote set-url origin https://github.com/M3ndes/otherhost.git
git pull
./scripts/bootstrap-mac.sh --apply
```

The installer converts the legacy machine-name key and local config filename,
then points both `otherhost` and the deprecated `devbox` alias at the current
clone. It preserves any configured legacy SSH identity and pinned host-key file,
so the existing Windows connection continues to work without pairing again.

In another terminal, inspect the available URLs and remote machine:

```bash
otherhost urls
otherhost status
```

For example, a configured WSL application on port `3000` becomes
`http://127.0.0.1:3000` on the Mac while `otherhost connect` runs. The CLI opens the
tunnel but does not start that application.

## Use an editor over Remote SSH

Generate an OpenSSH host block:

```bash
otherhost ssh-config
```

Review the output, then add it to `~/.ssh/config` if appropriate. Editors with
Remote SSH support can use that host entry to open the WSL workspace. The editor
server and project commands then run inside WSL, while the editor interface runs
on the Mac.

## Project dashboard

Build the local dashboard once when working from a source checkout:

```bash
make build-ui
otherhost ui
```

Without a local build, `otherhost ui` uses `go run` when Go 1.22 or newer is
available. The dashboard opens at `http://127.0.0.1:7842`, collects its inventory
through the same pinned SSH identity as the CLI, and stops when its terminal
process receives `Ctrl-C`.

Repositories are automatically discovered within three directory levels of the
WSL home. The configured `projects_root`, which defaults to `~/src`, remains the
preferred clone location and always has its direct children scanned. Hidden
directories and dependency trees are excluded, and only directories containing
`.git` are shown. The inventory refreshes every 30 seconds while the dashboard
is visible, when its browser tab returns to the foreground, and immediately when
**Projects** is opened. **Open project** launches a discovered path in VS Code
and expects the reviewed `otherhost ssh-config` block to already exist in
`~/.ssh/config` with the `code` command-line tool installed.

The **Terminal** view starts an interactive login shell through the configured
pinned SSH connection. Start a session from that view to use the WSL home
directory, or select the terminal icon on a project card to start in that exact
inventoried path. The terminal is not a sandbox: after connecting, it has the
same access as the paired WSL user and can navigate the rest of the remote
machine. Closing the page, clicking **Close**, or stopping `otherhost ui`
terminates its SSH process.

The server binds only to Mac loopback, embeds all of its assets, and exposes no
LAN or internet listener. Its interface, notifications, empty states, and error
messages are intentionally English-only. Terminal components are vendored with
the application and do not load scripts, styles, or telemetry from a CDN.

## Change forwarded ports

The `ports` field in `otherhost.local.conf` is a comma-separated list. For example:

```ini
ports=3000,5432,8000
```

Each number maps the same Mac localhost port to the same port on WSL loopback.
Stop any existing `otherhost connect` process, edit the list, run `otherhost doctor`,
and reconnect. If a local port is already in use, choose another application
port or stop the conflicting Mac process.

Configuration values are plain data. Do not add shell quotes, `$VARIABLES`, or
command substitutions; the parser intentionally rejects executable syntax.

## Pair again safely

Pair again when the Windows address changes permanently, WSL is rebuilt, the
Mac key is replaced, or the verified host key intentionally changes. Do not
delete the known-hosts entry merely to silence an unexplained mismatch.

1. Verify locally that you are connecting to the intended Windows and WSL host.
2. Start a new pairing window on Windows.
3. Run `otherhost pair` and compare a new code.
4. Let the final `otherhost doctor` verify the replacement state.

An existing Mac identity is preserved unless you intentionally remove it. If a
Mac is lost or no longer trusted, remove its public key from WSL
`~/.ssh/authorized_keys` before authorizing another client.

## Manual recovery

If both automatic discovery methods are blocked, the explicit GitHub public-key
flow can recover access. It is less convenient because the host address and SSH
identity must then be configured and verified manually.

Print the dedicated public-key fingerprint:

```bash
ssh-keygen -lf ~/.ssh/otherhost_ed25519.pub -E sha256
```

On Windows, run setup with both the selected GitHub account and this exact
fingerprint. WSL authorizes only the matching public key, not the entire GitHub
profile. See [Windows and WSL host](windows-wsl.md#one-command-setup).

For discovery, authentication, host-key, or port-forward failures, continue with
[Troubleshooting](troubleshooting.md).
