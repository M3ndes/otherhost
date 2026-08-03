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
printf '%s\n' "$HELP_OUTPUT" | grep -F 'disconnect  Pause managed SSH tunnels' >/dev/null

SSH_CONFIG=$(OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" ssh-config)
printf '%s\n' "$SSH_CONFIG" | grep -F 'Host test-box' >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F "IdentityFile \"$HOME/.ssh/test key\"" >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F "UserKnownHostsFile \"$HOME/.ssh/test known hosts\"" >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F 'StrictHostKeyChecking yes' >/dev/null
printf '%s\n' "$SSH_CONFIG" | grep -F 'LocalForward 127.0.0.1:8080 127.0.0.1:8080' >/dev/null

cat > "$HOME/.ssh/config" <<'EOF'
Host unrelated.example
  User existing-user
EOF
APPLY_OUTPUT=$(OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" ssh-config --apply)
printf '%s\n' "$APPLY_OUTPUT" | grep -F '[ok] installed managed SSH host test-box' >/dev/null
assert_equal '# BEGIN otherhost: test-box' "$(head -1 "$HOME/.ssh/config")"
grep -F 'Host unrelated.example' "$HOME/.ssh/config" >/dev/null
grep -F 'Host test-box' "$HOME/.ssh/config" >/dev/null
grep -F "IdentityFile \"$HOME/.ssh/test key\"" "$HOME/.ssh/config" >/dev/null
assert_equal 1 "$(grep -Fxc '# BEGIN otherhost: test-box' "$HOME/.ssh/config")"
case "$(uname -s)" in
  Darwin) permissions=$(stat -f '%Lp' "$HOME/.ssh/config") ;;
  *) permissions=$(stat -c '%a' "$HOME/.ssh/config") ;;
esac
assert_equal 600 "$permissions"
cp "$HOME/.ssh/config" "$TEST_DIR/first-applied-ssh-config"
OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" ssh-config --apply >/dev/null
cmp "$TEST_DIR/first-applied-ssh-config" "$HOME/.ssh/config"

cat > "$HOME/.ssh/config" <<'EOF'
# BEGIN otherhost: test-box
Host test-box
  HostName stale.example
Host unrelated.example
  User existing-user
EOF
cp "$HOME/.ssh/config" "$TEST_DIR/malformed-ssh-config"
if OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" ssh-config --apply >"$TEST_DIR/malformed-stdout" 2>"$TEST_DIR/malformed-stderr"; then
  printf '%s\n' 'malformed managed SSH markers were unexpectedly accepted' >&2
  exit 1
fi
grep -F 'managed SSH block markers are inconsistent' "$TEST_DIR/malformed-stderr" >/dev/null
cmp "$TEST_DIR/malformed-ssh-config" "$HOME/.ssh/config"

if OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" ui --apply >"$TEST_DIR/apply-stdout" 2>"$TEST_DIR/apply-stderr"; then
  printf '%s\n' '--apply unexpectedly worked with ui' >&2
  exit 1
fi
grep -F -- 'ui does not support --check or --apply' "$TEST_DIR/apply-stderr" >/dev/null

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

CONNECTION_STATE="$TEST_DIR/state directory/connection-state"
DISCONNECT_OUTPUT=$(OTHERHOST_CONNECTION_STATE="$CONNECTION_STATE" OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" disconnect)
printf '%s\n' "$DISCONNECT_OUTPUT" | grep -F 'pairing and host identity were preserved' >/dev/null
assert_equal disconnected "$(cat "$CONNECTION_STATE")"
assert_equal disconnected "$(otherhost_connection_state "$CONNECTION_STATE")"

FAKE_SSH="$TEST_DIR/ssh"
cat > "$FAKE_SSH" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$FAKE_SSH"
RECONNECT_OUTPUT=$(PATH="$TEST_DIR:$PATH" OTHERHOST_CONNECTION_STATE="$CONNECTION_STATE" OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/bin/otherhost" reconnect)
printf '%s\n' "$RECONNECT_OUTPUT" | grep -F 'connection resumed' >/dev/null
assert_equal connected "$(cat "$CONNECTION_STATE")"

printf '%s\n' 'config tests passed'
