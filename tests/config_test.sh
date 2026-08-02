#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/otherhost.sh
. "$ROOT_DIR/lib/otherhost.sh"

TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM
export HOME="$TEST_DIR/home with spaces"
mkdir -p "$HOME/.ssh"
touch "$HOME/.ssh/test key"
chmod 600 "$HOME/.ssh/test key"
touch "$HOME/.ssh/test known hosts"
chmod 600 "$HOME/.ssh/test known hosts"

CONFIG_FILE="$TEST_DIR/otherhost.conf"
SIDE_EFFECT="$TEST_DIR/must-not-exist"
cat > "$CONFIG_FILE" <<EOF
# comments and whitespace are ignored
otherhost_name = test-box
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

assert_equal test-box "$(otherhost_config_get otherhost_name "$CONFIG_FILE")"
assert_equal developer "$(otherhost_config_get ssh_user "$CONFIG_FILE")"
assert_equal '.ssh/test key' "$(otherhost_config_get identity_file "$CONFIG_FILE")"
assert_equal "$HOME/.ssh/test key" "$(otherhost_resolve_identity_file '.ssh/test key')"
[ ! -e "$SIDE_EFFECT" ] || { printf '%s\n' 'configuration executed unexpectedly' >&2; exit 1; }

HELP_OUTPUT=$("$ROOT_DIR/bin/otherhost" help)
printf '%s\n' "$HELP_OUTPUT" | grep -F '◉  otherhost' >/dev/null
printf '%s\n' "$HELP_OUTPUT" | grep -F 'Make the other host feel local.' >/dev/null

SSH_CONFIG=$(OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" ssh-config)
printf '%s\n' "$SSH_CONFIG" | grep -F 'Host test-box' >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F "IdentityFile \"$HOME/.ssh/test key\"" >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F "UserKnownHostsFile \"$HOME/.ssh/test known hosts\"" >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F 'StrictHostKeyChecking yes' >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F 'LocalForward 127.0.0.1:8080 127.0.0.1:8080' >/dev/null

URLS=$(OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" urls)
assert_equal 'http://localhost:3000
http://localhost:8080' "$URLS"

mkdir -p "$HOME/.local/bin"
ln -s "$ROOT_DIR/bin/otherhost" "$HOME/.local/bin/otherhost"
INSTALLED_URLS=$(OTHERHOST_CONFIG="$CONFIG_FILE" "$HOME/.local/bin/otherhost" urls)
assert_equal "$URLS" "$INSTALLED_URLS"

LEGACY_CONFIG="$TEST_DIR/devbox.local.conf"
sed 's/^otherhost_name/devbox_name/' "$CONFIG_FILE" > "$LEGACY_CONFIG"
LEGACY_URLS=$(DEVBOX_CONFIG="$LEGACY_CONFIG" "$ROOT_DIR/bin/devbox" urls 2> "$TEST_DIR/legacy-warning")
assert_equal "$URLS" "$LEGACY_URLS"
grep -F 'renamed to otherhost' "$TEST_DIR/legacy-warning" >/dev/null

MIGRATED_CONFIG="$TEST_DIR/otherhost.local.conf"
otherhost_migrate_config "$LEGACY_CONFIG" "$MIGRATED_CONFIG"
assert_equal test-box "$(otherhost_config_get otherhost_name "$MIGRATED_CONFIG")"
assert_equal '' "$(otherhost_config_get devbox_name "$MIGRATED_CONFIG")"
[ ! -e "$SIDE_EFFECT" ] || { printf '%s\n' 'configuration migration executed unexpectedly' >&2; exit 1; }

UI_ARGS="$TEST_DIR/ui-args"
UI_BIN="$TEST_DIR/otherhost-ui"
cat > "$UI_BIN" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$@" > "$UI_ARGS"
EOF
chmod +x "$UI_BIN"
export UI_ARGS
OTHERHOST_UI_BIN="$UI_BIN" OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" ui
assert_equal "--config
$CONFIG_FILE" "$(cat "$UI_ARGS")"

printf '%s\n' 'config tests passed'
