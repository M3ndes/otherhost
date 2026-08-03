#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -P -- "$(dirname -- "$0")/.." && pwd)
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

export HOME="$TEST_DIR/home & workspace"
mkdir -p "$HOME/.local/bin" "$TEST_DIR/bin"
CONFIG_FILE="$TEST_DIR/config & host.conf"
SIDE_EFFECT="$TEST_DIR/config-must-not-run"
cat > "$CONFIG_FILE" <<EOF
otherhost_name=test-box
unused=\$(touch $SIDE_EFFECT)
EOF

COMMAND_BIN="$HOME/.local/bin/otherhost"
cat > "$COMMAND_BIN" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$COMMAND_BIN"

LAUNCHCTL_LOG="$TEST_DIR/launchctl.log"
LAUNCHCTL_STATE="$TEST_DIR/launchctl.loaded"
export LAUNCHCTL_LOG LAUNCHCTL_STATE
FAKE_LAUNCHCTL="$TEST_DIR/bin/launchctl"
cat > "$FAKE_LAUNCHCTL" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$LAUNCHCTL_LOG"
case "${1:-}" in
  print)
    [ -f "$LAUNCHCTL_STATE" ] || exit 1
    printf '%s\n' 'state = running' 'pid = 4242'
    ;;
  bootstrap|kickstart) touch "$LAUNCHCTL_STATE" ;;
  bootout) rm -f "$LAUNCHCTL_STATE" ;;
esac
EOF
chmod +x "$FAKE_LAUNCHCTL"

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

service_command() {
  OTHERHOST_PLATFORM=Darwin \
    OTHERHOST_LAUNCHCTL_BIN="$FAKE_LAUNCHCTL" \
    OTHERHOST_COMMAND_BIN="$COMMAND_BIN" \
    "$ROOT_DIR/scripts/service-mac.sh" --config "$CONFIG_FILE" "$@"
}

CHECK_OUTPUT=$(service_command --check)
printf '%s\n' "$CHECK_OUTPUT" | grep -F '[plan] install or refresh' >/dev/null
[ ! -e "$HOME/Library/LaunchAgents" ] || fail 'service check created a persistent directory'
[ ! -e "$SIDE_EFFECT" ] || fail 'service check executed the configuration'

service_command --apply >/dev/null
PLIST_FILE="$HOME/Library/LaunchAgents/dev.otherhost.connect.plist"
[ -f "$PLIST_FILE" ] || fail 'LaunchAgent definition was not installed'
grep -F '<string>dev.otherhost.connect</string>' "$PLIST_FILE" >/dev/null
grep -F '<key>KeepAlive</key>' "$PLIST_FILE" >/dev/null
grep -F '<integer>10</integer>' "$PLIST_FILE" >/dev/null
grep -F 'home &amp; workspace' "$PLIST_FILE" >/dev/null
grep -F 'config &amp; host.conf' "$PLIST_FILE" >/dev/null
grep -F 'connect</string>' "$PLIST_FILE" >/dev/null
grep -F 'bootstrap gui/' "$LAUNCHCTL_LOG" >/dev/null
grep -F 'kickstart -k gui/' "$LAUNCHCTL_LOG" >/dev/null
[ ! -e "$SIDE_EFFECT" ] || fail 'service apply executed the configuration'

cp "$PLIST_FILE" "$TEST_DIR/first.plist"
service_command --apply >/dev/null
cmp "$TEST_DIR/first.plist" "$PLIST_FILE"

STATUS_OUTPUT=$(service_command status)
printf '%s\n' "$STATUS_OUTPUT" | grep -F '[ok] connection service is running' >/dev/null

service_command remove >/dev/null
[ ! -e "$PLIST_FILE" ] || fail 'service remove preserved the managed LaunchAgent'
[ ! -e "$LAUNCHCTL_STATE" ] || fail 'service remove left the job loaded'

if OTHERHOST_PLATFORM=Linux OTHERHOST_LAUNCHCTL_BIN="$FAKE_LAUNCHCTL" OTHERHOST_COMMAND_BIN="$COMMAND_BIN" \
  "$ROOT_DIR/scripts/service-mac.sh" --config "$CONFIG_FILE" --check >"$TEST_DIR/linux.out" 2>"$TEST_DIR/linux.err"; then
  fail 'connection service accepted a non-macOS platform'
fi
grep -F 'supports macOS launchd only' "$TEST_DIR/linux.err" >/dev/null

printf '%s\n' 'macOS service tests passed'
