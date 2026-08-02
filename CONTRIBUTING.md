# Contributing to devbox-bridge

Thank you for helping make remote development between Windows, WSL, and macOS
simpler. Code, documentation, bug reports, compatibility results, and focused
design discussions are all useful contributions.

Please keep discussions respectful, technical, and welcoming to developers who
do not specialize in Windows administration or networking.

## Before opening an issue

Search existing issues first. For setup or connection failures, work through the
[Troubleshooting guide](docs/troubleshooting.md) and include the first layer that
failed.

A useful bug report contains:

- the Windows version and build;
- `wsl --version` and the Ubuntu distribution name;
- Mac model/architecture and macOS version;
- whether Windows uses Ethernet or Wi-Fi;
- the exact command and error;
- relevant `[diag]` lines and which expected listener or service was absent;
- whether the behavior is reproducible from a clean pairing attempt.

Redact LAN addresses, usernames, hostnames, local paths, and comparison codes
when they are not needed. Never post private keys, tokens, `.env` contents, or
application credentials. Report possible vulnerabilities privately as described
in [SECURITY.md](SECURITY.md).

For a substantial feature, open an issue before investing in a large patch. A
good proposal explains the user problem, intended command experience, platform
impact, security boundaries, and a recovery path.

## Development environment

The repository intentionally has a small toolchain:

- Git;
- Go 1.22 or newer for the pairing helper;
- Bash 3.2 compatibility for Mac-facing shell scripts;
- Windows PowerShell 5.1 compatibility for Windows scripts;
- ShellCheck for shell linting.

Clone the repository and run the portable test suite:

```bash
git clone https://github.com/M3ndes/devbox-bridge.git
cd devbox-bridge
make test
```

Run the shell linter separately:

```bash
make shellcheck
```

`make test` checks Bash syntax, Go packages, configuration parsing, and security
controls. CI repeats portable tests on Ubuntu and macOS, runs Go tests with the
race detector, checks ShellCheck, and parses PowerShell on Windows.

Run the project dashboard against safe demonstration data during UI work:

```bash
go run ./cmd/devbox-ui --demo
```

The dashboard must remain usable without external fonts, scripts, CDNs, or
telemetry. All user-facing copy is English. Keep its local server bound to
loopback and preserve the per-session authorization check for local actions.

## Repository map

| Path | Responsibility |
| --- | --- |
| `bin/devbox` | Mac user command and OpenSSH orchestration |
| `cmd/devbox-pair/` | Go discovery and encrypted pairing helper |
| `cmd/devbox-ui/`, `internal/dashboard/` | Local project dashboard and remote inventory |
| `lib/` | Shared Bash and PowerShell configuration parsing |
| `scripts/bootstrap-mac.sh` | Mac client installation |
| `setup.cmd`, `setup.ps1` | Windows preflight, UAC handoff, and orchestration |
| `scripts/bootstrap-windows.ps1` | Windows and Hyper-V network policy |
| `scripts/bootstrap-wsl*.sh` | System and user-scoped WSL host setup |
| `scripts/pair-wsl.sh` | Direct WSL pairing host launcher |
| `config/devbox.example.conf` | Documented configuration schema |
| `tests/` | Portable policy, parsing, and syntax tests |
| `docs/` | User, operator, security, and architecture explanations |

Read [Architecture and decisions](docs/architecture.md) before changing
discovery, pairing, privilege boundaries, SSH policy, configuration parsing, or
network exposure.

## Make a change

Create a focused semantic branch:

```bash
git switch -c feat/clear-description
```

Use semantic commit subjects such as:

```text
feat(network): add optional overlay addressing
fix(pairing): close listener after rejection
docs(readme): explain the SSH tunnel model
test(config): reject duplicate security fields
```

Keep these project constraints intact:

- `devbox.local.conf` is untrusted input. Parse it as data; never source,
  evaluate, dot-source, or execute it.
- Setup must be safe to rerun, preserve unrelated user configuration, and offer
  a read-only check before applying host changes.
- Never install, copy, log, or commit private SSH keys, passwords, tokens, or
  project secrets.
- Keep Mac shell code compatible with Bash 3.2 and Windows code compatible with
  Windows PowerShell 5.1.
- Prefer Docker Desktop WSL integration; do not install a competing Docker
  daemon.
- Keep Linux repositories under the WSL filesystem rather than `/mnt/c`.
- Fail closed on protocol, host-key, authentication, or hardening errors. Do not
  silently weaken a security check as a recovery path.
- Add an automated test for changed behavior. If CI cannot exercise a host
  integration, document the limitation and provide reproducible manual steps.
- Update the relevant user and architecture documentation with behavior changes.

Use LF line endings except for PowerShell files, which follow the repository
`.editorconfig` policy.

## Test a local pairing helper

Build the current platform binary:

```bash
make build
```

Runtime scripts do not execute ignored local build artifacts automatically.
Select the candidate explicitly:

```bash
DEVBOX_PAIR_BIN="$PWD/build/devbox-pair" devbox pair
```

Use the equivalent `DEVBOX_PAIR_BIN` environment override for a host-mode test.
This opt-in prevents a stale local binary from silently replacing the
checksum-verified released helper.

Cross-compile the four Mac and Windows assets used by the local release target:

```bash
make release-build
```

The GitHub release workflow additionally builds Linux `amd64` and `arm64`
helpers for WSL-hosted pairing, producing six release assets plus
`checksums.txt`. A tag matching `v*` publishes those files. The helper version
declared by Bash and PowerShell installers must match the release tag before a
release is cut.

## Pull request checklist

Before requesting review, confirm:

- [ ] The change solves one clearly described problem.
- [ ] `make test` passes.
- [ ] `make shellcheck` passes for shell changes.
- [ ] PowerShell changes parse on Windows PowerShell 5.1.
- [ ] Tests cover new success and failure behavior where practical.
- [ ] Documentation and examples match the implementation.
- [ ] Check mode remains read-only and apply mode remains idempotent.
- [ ] Logs and fixtures contain no credentials or identifying machine data.
- [ ] The PR explains any manual Windows/WSL/macOS test that was performed.

Small, reviewable pull requests are easier to validate across all three
environments than changes that mix protocol, setup, and documentation concerns.
