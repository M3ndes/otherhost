# Repository guidance

- Keep the project CLI-first until the command workflow is stable in real use.
- Treat `otherhost.local.conf` as untrusted data; never source, evaluate, or execute it.
- Keep macOS shell code compatible with Bash 3.2 and Windows code compatible with
  Windows PowerShell 5.1.
- All setup operations must be idempotent, preserve unrelated user configuration,
  and offer a read-only check before applying host changes.
- Never install or copy private SSH keys, passwords, tokens, or project secrets.
- Prefer Docker Desktop WSL integration; do not install a competing Docker daemon.
- Store Linux development repositories in the WSL filesystem, not under `/mnt/c`.
- Every behavior change requires tests and corresponding documentation.
- Run `make test` before declaring a change complete.
- Use semantic branches and commit messages: `<type>/<description>` and
  `<type>(<scope>): <summary>`.
