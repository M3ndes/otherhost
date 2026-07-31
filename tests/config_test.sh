#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/devbox.sh
. "$ROOT_DIR/lib/devbox.sh"

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM
export HOME="$TEST_DIR/home with spaces"
mkdir -p "$HOME/.ssh"
touch "$HOME/.ssh/test key"
chmod 600 "$HOME/.ssh/test key"

CONFIG_FILE="$TEST_DIR/devbox.conf"
SIDE_EFFECT="$TEST_DIR/must-not-exist"
cat > "$CONFIG_FILE" <<EOF
# comments and whitespace are ignored
devbox_name = test-box
host=192.0.2.10
ssh_user = developer
ssh_port=2222
identity_file=.ssh/test key
ports=3000, 8080
unused=\$(touch $SIDE_EFFECT)
EOF

assert_equal() {
  if [ "$1" != "$2" ]; then
    printf 'expected <%s>, got <%s>\n' "$1" "$2" >&2
    exit 1
  fi
}

assert_equal test-box "$(devbox_config_get devbox_name "$CONFIG_FILE")"
assert_equal developer "$(devbox_config_get ssh_user "$CONFIG_FILE")"
assert_equal '.ssh/test key' "$(devbox_config_get identity_file "$CONFIG_FILE")"
assert_equal "$HOME/.ssh/test key" "$(devbox_resolve_identity_file '.ssh/test key')"
[ ! -e "$SIDE_EFFECT" ] || { printf '%s\n' 'configuration executed unexpectedly' >&2; exit 1; }

SSH_CONFIG=$(DEVBOX_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/devbox" ssh-config)
printf '%s\n' "$SSH_CONFIG" | grep -F 'Host test-box' >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F "IdentityFile \"$HOME/.ssh/test key\"" >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F 'LocalForward 127.0.0.1:8080 127.0.0.1:8080' >/dev/null

URLS=$(DEVBOX_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/devbox" urls)
assert_equal 'http://localhost:3000
http://localhost:8080' "$URLS"

printf '%s\n' 'config tests passed'
