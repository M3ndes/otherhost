#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/devbox.sh
. "$ROOT_DIR/lib/devbox.sh"

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM
export HOME="$TEST_DIR/home with spaces"
mkdir -p "$HOME/.ssh"
touch "$HOME/.ssh/test key"
chmod 600 "$HOME/.ssh/test key"
touch "$HOME/.ssh/test known hosts"
chmod 600 "$HOME/.ssh/test known hosts"

CONFIG_FILE="$TEST_DIR/devbox.conf"
SIDE_EFFECT="$TEST_DIR/must-not-exist"
cat > "$CONFIG_FILE" <<EOF
# comments and whitespace are ignored
devbox_name = test-box
host=192.0.2.10
ssh_user = developer
ssh_port=2222
identity_file=.ssh/test key
known_hosts_file=.ssh/test known hosts
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
printf '%s\n' "$SSH_CONFIG" | grep -F "UserKnownHostsFile \"$HOME/.ssh/test known hosts\"" >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F 'StrictHostKeyChecking yes' >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F 'LocalForward 127.0.0.1:8080 127.0.0.1:8080' >/dev/null

URLS=$(DEVBOX_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/devbox" urls)
assert_equal 'http://localhost:3000
http://localhost:8080' "$URLS"

mkdir -p "$HOME/.local/bin"
ln -s "$ROOT_DIR/bin/devbox" "$HOME/.local/bin/devbox"
INSTALLED_URLS=$(DEVBOX_CONFIG="$CONFIG_FILE" "$HOME/.local/bin/devbox" urls)
assert_equal "$URLS" "$INSTALLED_URLS"

UI_ARGS="$TEST_DIR/ui-args"
UI_BIN="$TEST_DIR/devbox-ui"
cat > "$UI_BIN" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" > "$UI_ARGS"
EOF
chmod +x "$UI_BIN"
export UI_ARGS
DEVBOX_UI_BIN="$UI_BIN" DEVBOX_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/devbox" ui
assert_equal "--config
$CONFIG_FILE" "$(cat "$UI_ARGS")"

printf '%s\n' 'config tests passed'
