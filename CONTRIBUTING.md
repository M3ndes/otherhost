# Contributing

Issues and pull requests are welcome. Keep changes small, portable, and safe to
rerun.

## Development

```bash
make test
```

Shell changes must support Bash 3.2 on macOS unless a documented decision changes
that baseline. PowerShell changes must support Windows PowerShell 5.1. Scripts
must preserve existing user configuration and keep check mode read-only.

Never add secrets or machine-specific `devbox.local.conf` files. New behavior
needs an automated test or a documented explanation when host integration cannot
be exercised in CI.

Use semantic branch, commit, and PR names such as:

```text
feat/tailscale-support
feat(network): add optional Tailscale addressing
```
