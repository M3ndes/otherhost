SHELL := /bin/bash
GO ?= go

.PHONY: test syntax shellcheck go-test build release-build

test: syntax go-test
	./tests/config_test.sh
	./tests/security_controls_test.sh

go-test:
	$(GO) test ./...

build:
	mkdir -p build
	$(GO) build -trimpath -o build/devbox-pair ./cmd/devbox-pair

release-build:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o dist/devbox-pair-darwin-amd64 ./cmd/devbox-pair
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w" -o dist/devbox-pair-darwin-arm64 ./cmd/devbox-pair
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o dist/devbox-pair-windows-amd64.exe ./cmd/devbox-pair
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w" -o dist/devbox-pair-windows-arm64.exe ./cmd/devbox-pair

syntax:
	bash -n bin/devbox lib/devbox.sh scripts/bootstrap-mac.sh scripts/bootstrap-wsl.sh scripts/install-pairing-helper.sh tests/config_test.sh tests/security_controls_test.sh
	@if command -v pwsh >/dev/null 2>&1; then pwsh -NoProfile -File tests/powershell_syntax_test.ps1; else echo "pwsh not installed; skipping PowerShell syntax check"; fi

shellcheck:
	shellcheck -x -P SCRIPTDIR bin/devbox lib/devbox.sh scripts/bootstrap-mac.sh scripts/bootstrap-wsl.sh scripts/install-pairing-helper.sh tests/config_test.sh tests/security_controls_test.sh
