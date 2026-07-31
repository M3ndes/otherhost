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
- Keep SSH password login disabled.
- Do not forward the SSH port from a public router directly to the WSL host.
- Review generated SSH and firewall configuration before applying it.

The scripts avoid deleting user data and back up `.wslconfig`, but host owners are
responsible for OS updates, disk encryption, backups, and physical security.
