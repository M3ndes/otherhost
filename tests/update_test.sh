#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -P -- "$(dirname -- "$0")/.." && pwd)
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

export HOME="$TEST_DIR/home with spaces"
mkdir -p "$HOME/.ssh" "$TEST_DIR/bin"
touch "$HOME/.ssh/test-key" "$HOME/.ssh/test-known-hosts"
chmod 600 "$HOME/.ssh/test-key" "$HOME/.ssh/test-known-hosts"

SIDE_EFFECT="$TEST_DIR/config-must-not-run"
CONFIG_FILE="$TEST_DIR/otherhost.conf"
cat > "$CONFIG_FILE" <<EOF
otherhost_name=test-box
host=192.0.2.10
ssh_user=developer
ssh_port=2222
identity_file=.ssh/test-key
known_hosts_file=.ssh/test-known-hosts
unused=\$(touch $SIDE_EFFECT)
EOF

LOCAL_REVISION=1111111111111111111111111111111111111111
UPSTREAM_REVISION=2222222222222222222222222222222222222222
export LOCAL_REVISION UPSTREAM_REVISION
GIT_LOG="$TEST_DIR/git.log"
export GIT_LOG
FAKE_GIT="$TEST_DIR/bin/git"
cat > "$FAKE_GIT" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$GIT_LOG"
arguments=" $* "
case "$arguments" in
  *' rev-parse --verify FETCH_HEAD '*) printf '%s\n' "$UPSTREAM_REVISION" ;;
  *' rev-parse --verify HEAD '*) printf '%s\n' "$LOCAL_REVISION" ;;
  *' remote get-url origin '*) printf '%s\n' 'https://github.com/M3ndes/otherhost.git' ;;
  *' ls-remote https://github.com/M3ndes/otherhost.git refs/heads/main '*) printf '%s\trefs/heads/main\n' "$UPSTREAM_REVISION" ;;
  *' symbolic-ref --quiet --short HEAD '*) printf '%s\n' 'main' ;;
  *' status --porcelain --untracked-files=all '*) ;;
  *' fetch --no-tags https://github.com/M3ndes/otherhost.git main '*) ;;
  *' merge --ff-only '*) ;;
  *) printf 'unexpected fake git arguments: %s\n' "$*" >&2; exit 1 ;;
esac
EOF
chmod +x "$FAKE_GIT"

REMOTE_STATE_FILE="$TEST_DIR/remote-state"
cat > "$REMOTE_STATE_FILE" <<EOF
wsl.revision=$LOCAL_REVISION
wsl.compatibility=2
windows.revision=$LOCAL_REVISION
windows.compatibility=2
EOF
export REMOTE_STATE_FILE
SSH_LOG="$TEST_DIR/ssh.log"
export SSH_LOG
FAKE_SSH="$TEST_DIR/bin/ssh"
cat > "$FAKE_SSH" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$SSH_LOG"
cat >/dev/null
[ "${SSH_SHOULD_FAIL:-0}" -eq 0 ] || exit 255
cat "$REMOTE_STATE_FILE"
EOF
chmod +x "$FAKE_SSH"

BOOTSTRAP_LOG="$TEST_DIR/bootstrap.log"
export BOOTSTRAP_LOG
FAKE_BOOTSTRAP="$TEST_DIR/bin/bootstrap-mac"
cat > "$FAKE_BOOTSTRAP" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >> "$BOOTSTRAP_LOG"
EOF
chmod +x "$FAKE_BOOTSTRAP"
FAKE_SERVICE="$TEST_DIR/bin/service-mac"
cp "$FAKE_BOOTSTRAP" "$FAKE_SERVICE"

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

update_command() {
  OTHERHOST_GIT_BIN="$FAKE_GIT" \
    OTHERHOST_SSH_BIN="$FAKE_SSH" \
    OTHERHOST_BOOTSTRAP_MAC_BIN="$FAKE_BOOTSTRAP" \
    OTHERHOST_SERVICE_MAC_BIN="$FAKE_SERVICE" \
    "$ROOT_DIR/scripts/update.sh" --config "$CONFIG_FILE" "$@"
}

CHECK_OUTPUT=$(update_command --check)
printf '%s\n' "$CHECK_OUTPUT" | grep -F 'Mac client     111111111111  update available  compatibility 2' >/dev/null
printf '%s\n' "$CHECK_OUTPUT" | grep -F 'WSL host       111111111111  update available  compatibility 2' >/dev/null
printf '%s\n' "$CHECK_OUTPUT" | grep -F 'Windows setup  111111111111  update available  compatibility 2' >/dev/null
printf '%s\n' "$CHECK_OUTPUT" | grep -F 'setup.cmd -Update' >/dev/null
[ ! -e "$SIDE_EFFECT" ] || fail 'update check executed the configuration'
if grep -F 'fetch --no-tags' "$GIT_LOG" >/dev/null || grep -F 'merge --ff-only' "$GIT_LOG" >/dev/null; then
  fail 'read-only update check changed the Git checkout'
fi

: > "$GIT_LOG"
update_command --apply >/dev/null
grep -F 'fetch --no-tags https://github.com/M3ndes/otherhost.git main' "$GIT_LOG" >/dev/null
grep -F 'merge --ff-only' "$GIT_LOG" >/dev/null
grep -F -- "--apply --config $CONFIG_FILE" "$BOOTSTRAP_LOG" >/dev/null
[ ! -e "$SIDE_EFFECT" ] || fail 'update apply executed the configuration'

OFFLINE_OUTPUT=$(SSH_SHOULD_FAIL=1 update_command --check)
printf '%s\n' "$OFFLINE_OUTPUT" | grep -F 'remote versions are unknown' >/dev/null
printf '%s\n' "$OFFLINE_OUTPUT" | grep -F 'WSL host       unknown' >/dev/null
printf '%s\n' "$OFFLINE_OUTPUT" | grep -F 'Windows setup  unknown' >/dev/null

sed 's/windows.compatibility=2/windows.compatibility=3/' "$REMOTE_STATE_FILE" > "$TEST_DIR/incompatible-state"
mv "$TEST_DIR/incompatible-state" "$REMOTE_STATE_FILE"
if update_command --check >"$TEST_DIR/incompatible.out" 2>"$TEST_DIR/incompatible.err"; then
  fail 'compatibility mismatch returned success'
fi
grep -F 'Windows compatibility 3 does not match' "$TEST_DIR/incompatible.out" >/dev/null

# shellcheck source=../lib/otherhost.sh
. "$ROOT_DIR/lib/otherhost.sh"
STATE_DIRECTORY="$TEST_DIR/state"
OTHERHOST_GIT_BIN="$FAKE_GIT" otherhost_write_installation_state "$ROOT_DIR" "$STATE_DIRECTORY"
grep -F 'compatibility=2' "$STATE_DIRECTORY/install-state" >/dev/null
grep -F "revision=$LOCAL_REVISION" "$STATE_DIRECTORY/install-state" >/dev/null

printf '%s\n' 'update tests passed'
