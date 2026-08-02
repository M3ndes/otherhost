#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/devbox.sh
. "$ROOT_DIR/lib/devbox.sh"

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

ssh-keygen -q -t ed25519 -N '' -C 'devbox-test-one' -f "$TEST_DIR/key-one"
ssh-keygen -q -t ed25519 -N '' -C 'devbox-test-two' -f "$TEST_DIR/key-two"
KEY_ONE=$(cat "$TEST_DIR/key-one.pub")
KEY_TWO=$(cat "$TEST_DIR/key-two.pub")
AUTHORIZED_KEYS="$TEST_DIR/authorized_keys"
: > "$AUTHORIZED_KEYS"

devbox_authorize_public_key "$KEY_ONE" "$AUTHORIZED_KEYS"
devbox_authorize_public_key "$KEY_ONE" "$AUTHORIZED_KEYS"
[ "$(wc -l < "$AUTHORIZED_KEYS" | tr -d '[:space:]')" = 1 ] || {
  printf '%s\n' 'selected key was not installed idempotently' >&2
  exit 1
}
grep -Fqx "$KEY_ONE" "$AUTHORIZED_KEYS"
if grep -Fqx "$KEY_TWO" "$AUTHORIZED_KEYS"; then
  printf '%s\n' 'an unselected profile key was authorized' >&2
  exit 1
fi
if devbox_authorize_public_key 'ssh-ed25519 not-base64 invalid' "$AUTHORIZED_KEYS"; then
  printf '%s\n' 'invalid public key was accepted' >&2
  exit 1
fi

SAFE_POLICY=$(cat <<'EOF'
port 2222
pubkeyauthentication yes
passwordauthentication no
kbdinteractiveauthentication no
permitrootlogin no
allowusers developer
EOF
)
devbox_assert_sshd_policy "$SAFE_POLICY" 2222 developer

UNSAFE_POLICY=$(printf '%s\n' "$SAFE_POLICY" | sed 's/passwordauthentication no/passwordauthentication yes/')
if devbox_assert_sshd_policy "$UNSAFE_POLICY" 2222 developer >/dev/null 2>&1; then
  printf '%s\n' 'unsafe effective sshd policy was accepted' >&2
  exit 1
fi
if devbox_assert_sshd_policy "$SAFE_POLICY" 2222 another-user >/dev/null 2>&1; then
  printf '%s\n' 'unexpected SSH user policy was accepted' >&2
  exit 1
fi

WSL_BOOTSTRAP="$ROOT_DIR/scripts/bootstrap-wsl.sh"
ROOTLESS_BOOTSTRAP="$ROOT_DIR/scripts/bootstrap-wsl-user.sh"
ROOTLESS_PAIR="$ROOT_DIR/scripts/pair-wsl.sh"
ROOTLESS_INSTALLER="$ROOT_DIR/scripts/install-pairing-helper-wsl.sh"
SETUP_SCRIPT="$ROOT_DIR/setup.ps1"
grep -F "devbox_authorize_public_key \"\$SSH_PUBLIC_KEY\"" "$WSL_BOOTSTRAP" >/dev/null
grep -F "if [ -n \"\$SSH_PUBLIC_KEY\" ]; then" "$WSL_BOOTSTRAP" >/dev/null
grep -F '/etc/ssh/sshd_config.d/00-devbox-bridge.conf' "$WSL_BOOTSTRAP" >/dev/null
grep -F "SSHD_EFFECTIVE=\$(sudo sshd -T -C" "$WSL_BOOTSTRAP" >/dev/null
grep -F "devbox_assert_sshd_policy \"\$SSHD_EFFECTIVE\"" "$WSL_BOOTSTRAP" >/dev/null
if grep -F "https://github.com/\$GITHUB_USER.keys" "$WSL_BOOTSTRAP" >/dev/null; then
  printf '%s\n' 'WSL bootstrap still authorizes an entire GitHub profile' >&2
  exit 1
fi
grep -F 'apt-get download openssh-server libwrap0' "$ROOTLESS_BOOTSTRAP" >/dev/null
grep -F 'AuthenticationMethods publickey' "$ROOTLESS_BOOTSTRAP" >/dev/null
grep -F 'PasswordAuthentication no' "$ROOTLESS_BOOTSTRAP" >/dev/null
grep -F 'KbdInteractiveAuthentication no' "$ROOTLESS_BOOTSTRAP" >/dev/null
grep -F 'AllowTcpForwarding local' "$ROOTLESS_BOOTSTRAP" >/dev/null
grep -F 'PermitOpen localhost:* 127.0.0.1:* [::1]:*' "$ROOTLESS_BOOTSTRAP" >/dev/null
grep -F 'systemctl --user enable --now devbox-bridge-sshd.service' "$ROOTLESS_BOOTSTRAP" >/dev/null
if grep -E '^[[:space:]]*sudo([[:space:]]|$)' "$ROOTLESS_BOOTSTRAP" >/dev/null; then
  printf '%s\n' 'rootless WSL bootstrap invokes sudo' >&2
  exit 1
fi
grep -F -- "--authorized-keys \"\$AUTHORIZED_KEYS\"" "$ROOTLESS_PAIR" >/dev/null
grep -F -- "--ssh-host-key \"\$(cat \"\$HOST_KEY\")\"" "$ROOTLESS_PAIR" >/dev/null

awk '
  { sub(/\r$/, "") }
  $0 == "$prepareScript = @\047" { capture = 1; next }
  capture && $0 == "\047@" { exit }
  capture { print }
' "$SETUP_SCRIPT" > "$TEST_DIR/prepare-wsl-repository.sh"
[ -s "$TEST_DIR/prepare-wsl-repository.sh" ] || {
  printf '%s\n' 'could not extract the embedded WSL repository preparation script' >&2
  exit 1
}
bash -n "$TEST_DIR/prepare-wsl-repository.sh"

PAIRING_MAIN="$ROOT_DIR/cmd/devbox-pair/main.go"
PAIRING_PROTOCOL="$ROOT_DIR/internal/pairing/protocol.go"
PAIRING_HOST="$ROOT_DIR/internal/pairing/host.go"
PAIRING_CLIENT="$ROOT_DIR/internal/pairing/client.go"
MAC_COMMAND="$ROOT_DIR/bin/devbox"
grep -F 'ecdh.X25519()' "$PAIRING_HOST" >/dev/null
grep -F 'ecdh.X25519()' "$PAIRING_CLIENT" >/dev/null
grep -F 'cipher.NewGCM' "$PAIRING_PROTOCOL" >/dev/null
grep -F 'numeric-comparison/v1' "$PAIRING_PROTOCOL" >/dev/null
grep -F 'StrictHostKeyChecking=yes' "$MAC_COMMAND" >/dev/null
grep -F 'UserKnownHostsFile' "$MAC_COMMAND" >/dev/null
grep -F '[fail] pairing stopped before the local connection was updated' "$MAC_COMMAND" >/dev/null
grep -F "Remove-NetFirewallRule -Name \$discoveryRule" "$SETUP_SCRIPT" >/dev/null
grep -F "Remove-NetFirewallRule -Name \$sessionRule" "$SETUP_SCRIPT" >/dev/null
grep -F -- "-RemoteAddress \$networkPolicy.RemoteSubnets -Profile \$networkPolicy.Profiles" "$SETUP_SCRIPT" >/dev/null
grep -F -- "-RemoteAddresses \$RemoteSubnets" "$SETUP_SCRIPT" >/dev/null
grep -F -- '--user-scoped-wsl' "$SETUP_SCRIPT" >/dev/null
grep -F 'does not support user-scoped WSL' "$SETUP_SCRIPT" >/dev/null
grep -F 'temporary UDP 45870 and TCP 45871 firewall rules are active' "$SETUP_SCRIPT" >/dev/null
grep -F 'Pairing is enabled' "$PAIRING_MAIN" >/dev/null

PAIRING_VERSION=$(sed -n 's/^PAIRING_VERSION=//p' "$ROOT_DIR/scripts/install-pairing-helper.sh")
[ -n "$PAIRING_VERSION" ] || { printf '%s\n' 'macOS pairing helper version is missing' >&2; exit 1; }
grep -F "\$version = '$PAIRING_VERSION'" "$SETUP_SCRIPT" >/dev/null || {
  printf '%s\n' 'Windows and macOS pairing helper versions differ' >&2
  exit 1
}
ROOTLESS_PAIRING_VERSION=$(sed -n 's/^PAIRING_VERSION=//p' "$ROOTLESS_INSTALLER")
[ "$ROOTLESS_PAIRING_VERSION" = "$PAIRING_VERSION" ] || {
  printf '%s\n' 'WSL and macOS pairing helper versions differ' >&2
  exit 1
}
grep -F "PAIRING_VERSION=$PAIRING_VERSION" "$MAC_COMMAND" >/dev/null || {
  printf '%s\n' 'macOS command and installer pairing helper versions differ' >&2
  exit 1
}
grep -F "PAIRING_VERSION=$PAIRING_VERSION" "$ROOTLESS_BOOTSTRAP" >/dev/null || {
  printf '%s\n' 'WSL bootstrap and installer pairing helper versions differ' >&2
  exit 1
}
grep -F 'asset: devbox-pair-linux-amd64' "$ROOT_DIR/.github/workflows/release.yml" >/dev/null
grep -F 'asset: devbox-pair-linux-arm64' "$ROOT_DIR/.github/workflows/release.yml" >/dev/null

if grep -Eni 'private[_ -]?key.*(send|copy|transfer)|ssh_private_key' \
  "$PAIRING_MAIN" "$PAIRING_PROTOCOL" "$PAIRING_HOST" "$PAIRING_CLIENT" >/dev/null; then
  printf '%s\n' 'pairing code appears to transfer a private key' >&2
  exit 1
fi

printf '%s\n' 'security control tests passed'
