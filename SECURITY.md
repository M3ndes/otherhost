# Security policy

## Supported versions

Security fixes target the latest release and the default branch.

## Reporting a vulnerability

Please use the repository's private **Report a vulnerability** form under the
Security tab. Do not open a public issue containing credentials, network
addresses, exploit details, or private keys.

## Deployment boundaries

- Use this project only on machines and networks you control.
- Never commit `devbox.local.conf`, `.env` files, tokens, or private SSH keys.
- Publish only `.pub` keys.
- Confirm the dedicated Mac key fingerprint during setup; do not authorize an
  entire GitHub profile.
- Pair only on a private network and confirm that the same six-digit code appears
  on both devices. Reject any unexpected device name or code.
- Treat the comparison code as a yes/no check, not as a password to type or send.
- Keep pairing time-limited. The implementation accepts one active session and
  removes its temporary private-subnet firewall rules when it exits.
- Pin the WSL SSH host key delivered by the confirmed encrypted pairing session;
  fail closed if that identity changes later.
- Keep SSH password login disabled.
- Require the effective `sshd -T` policy to match the hardened settings before
  enabling the service.
- Do not forward the SSH port from a public router directly to the WSL host.
- Review generated SSH and firewall configuration before applying it.

The scripts avoid deleting user data and back up `.wslconfig`, but host owners are
responsible for OS updates, disk encryption, backups, and physical security.
