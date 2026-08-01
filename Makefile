SHELL := /bin/bash

.PHONY: test syntax shellcheck

test: syntax
	./tests/config_test.sh
	./tests/security_controls_test.sh

syntax:
	bash -n bin/devbox lib/devbox.sh scripts/bootstrap-mac.sh scripts/bootstrap-wsl.sh tests/config_test.sh tests/security_controls_test.sh
	@if command -v pwsh >/dev/null 2>&1; then pwsh -NoProfile -File tests/powershell_syntax_test.ps1; else echo "pwsh not installed; skipping PowerShell syntax check"; fi

shellcheck:
	shellcheck -x -P SCRIPTDIR bin/devbox lib/devbox.sh scripts/bootstrap-mac.sh scripts/bootstrap-wsl.sh tests/config_test.sh tests/security_controls_test.sh
