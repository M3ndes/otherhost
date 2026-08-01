# Contributing

Issues and pull requests are welcome. Keep changes small, portable, and safe to
rerun.

## Development

```bash
make test
```

Shell changes must support Bash 3.2 on macOS unless a documented decision changes
that baseline. PowerShell changes must support Windows PowerShell 5.1. Scripts
must preserve existing user configuration and keep check mode read-only. The
pairing helper requires Go 1.22 or newer and must remain free of third-party Go
dependencies.

Run `make release-build` to cross-compile the four supported pairing binaries.
Tags matching `v*` publish those binaries and `checksums.txt` through the release
workflow. The installer version in the Bash and PowerShell entry points must
match the release tag.

For a local end-to-end test, build the helper and explicitly select it with
`DEVBOX_PAIR_BIN`. Runtime scripts never execute an ignored local build artifact
unless this override is set.

Never add secrets or machine-specific `devbox.local.conf` files. New behavior
needs an automated test or a documented explanation when host integration cannot
be exercised in CI.

Use semantic branch, commit, and PR names such as:

```text
feat/tailscale-support
feat(network): add optional Tailscale addressing
```
