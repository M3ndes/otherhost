SHELL := /bin/bash
GO ?= go

.PHONY: test syntax shellcheck go-test build build-ui build-ui-windows release-build docker-check docker-up docker-down docker-logs

test: syntax go-test
	./tests/config_test.sh
	./tests/docker_ui_test.sh
	./tests/service_mac_test.sh
	./tests/security_controls_test.sh
	./tests/update_test.sh

go-test:
	$(GO) test ./...

build:
	mkdir -p build
	$(GO) build -trimpath -o build/otherhost-pair ./cmd/otherhost-pair

build-ui:
	mkdir -p build
	$(GO) build -trimpath -o build/otherhost-ui ./cmd/otherhost-ui

build-ui-windows:
	mkdir -p build
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -o build/otherhost-ui.exe ./cmd/otherhost-ui

docker-check:
	./scripts/docker-ui.sh --check

docker-up:
	./scripts/docker-ui.sh --apply

docker-down:
	./scripts/docker-ui.sh --down

docker-logs:
	./scripts/docker-ui.sh --logs

release-build:
	mkdir -p dist
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o dist/otherhost-pair-darwin-amd64 ./cmd/otherhost-pair
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w" -o dist/otherhost-pair-darwin-arm64 ./cmd/otherhost-pair
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o dist/otherhost-pair-windows-amd64.exe ./cmd/otherhost-pair
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w" -o dist/otherhost-pair-windows-arm64.exe ./cmd/otherhost-pair
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -trimpath -ldflags "-s -w" -o dist/otherhost-ui-windows-amd64.exe ./cmd/otherhost-ui
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GO) build -trimpath -ldflags "-s -w" -o dist/otherhost-ui-windows-arm64.exe ./cmd/otherhost-ui

syntax:
	bash -n bin/otherhost bin/devbox lib/otherhost.sh scripts/bootstrap-mac.sh scripts/bootstrap-wsl.sh scripts/bootstrap-wsl-user.sh scripts/docker-ui.sh scripts/service-mac.sh scripts/update.sh scripts/install-pairing-helper.sh scripts/install-pairing-helper-wsl.sh scripts/pair-wsl.sh tests/config_test.sh tests/docker_ui_test.sh tests/service_mac_test.sh tests/security_controls_test.sh tests/update_test.sh
	@if command -v pwsh >/dev/null 2>&1; then pwsh -NoProfile -File tests/powershell_syntax_test.ps1; else echo "pwsh not installed; skipping PowerShell syntax check"; fi

shellcheck:
	shellcheck -x -P SCRIPTDIR bin/otherhost bin/devbox lib/otherhost.sh scripts/bootstrap-mac.sh scripts/bootstrap-wsl.sh scripts/bootstrap-wsl-user.sh scripts/docker-ui.sh scripts/service-mac.sh scripts/update.sh scripts/install-pairing-helper.sh scripts/install-pairing-helper-wsl.sh scripts/pair-wsl.sh tests/config_test.sh tests/docker_ui_test.sh tests/service_mac_test.sh tests/security_controls_test.sh tests/update_test.sh
