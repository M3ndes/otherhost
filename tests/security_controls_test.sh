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
SETUP_SCRIPT="$ROOT_DIR/setup.ps1"
grep -F "devbox_authorize_public_key \"\$SSH_PUBLIC_KEY\"" "$WSL_BOOTSTRAP" >/dev/null
grep -F '/etc/ssh/sshd_config.d/00-devbox-bridge.conf' "$WSL_BOOTSTRAP" >/dev/null
grep -F "SSHD_EFFECTIVE=\$(sudo sshd -T -C" "$WSL_BOOTSTRAP" >/dev/null
grep -F "devbox_assert_sshd_policy \"\$SSHD_EFFECTIVE\"" "$WSL_BOOTSTRAP" >/dev/null
if grep -F "https://github.com/\$GITHUB_USER.keys" "$WSL_BOOTSTRAP" >/dev/null; then
  printf '%s\n' 'WSL bootstrap still authorizes an entire GitHub profile' >&2
  exit 1
fi

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

printf '%s\n' 'security control tests passed'
