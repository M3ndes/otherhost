# Migrating from devbox-bridge

The project and GitHub repository are now named **Otherhost**. Existing
installations remain compatible during the transition, and the rename alone
does not require pairing the machines again.

## Update an existing clone

The previous GitHub URL redirects, but existing clones should use the current
remote explicitly:

```bash
git remote set-url origin https://github.com/M3ndes/otherhost.git
git pull
```

Rerun the platform bootstrap on each machine to install the current command and
paths. The setup remains idempotent and preserves unrelated configuration.

## Compatibility behavior

- `devbox` remains available as a deprecated command alias for `otherhost`.
- `devbox.local.conf` is read as a fallback. Running
  `bootstrap-mac.sh --apply` migrates it to `otherhost.local.conf` without
  evaluating its contents.
- Existing SSH identities and pinned `known_hosts` paths are reused.
- Already-installed v0.1.1 pairing helpers remain wire-compatible.

After upgrading, use `otherhost doctor` to verify configuration, dependencies,
and the saved SSH connection.
