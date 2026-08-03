#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -P -- "$(dirname -- "$0")/.." && pwd)
TEST_DIR=$(mktemp -d)
trap 'rm -rf "$TEST_DIR"' EXIT HUP INT TERM

export HOME="$TEST_DIR/home with spaces"
mkdir -p "$HOME/.ssh" "$TEST_DIR/bin"
IDENTITY_FILE="$HOME/.ssh/otherhost key"
KNOWN_HOSTS_FILE="$HOME/.ssh/otherhost known hosts"
touch "$IDENTITY_FILE" "$KNOWN_HOSTS_FILE"
chmod 600 "$IDENTITY_FILE" "$KNOWN_HOSTS_FILE"

SIDE_EFFECT="$TEST_DIR/config-must-not-run"
CONFIG_FILE="$TEST_DIR/otherhost config.conf"
cat > "$CONFIG_FILE" <<EOF
otherhost_name=test-box
host=192.0.2.10
ssh_user=developer
ssh_port=2222
identity_file=.ssh/otherhost key
known_hosts_file=.ssh/otherhost known hosts
unused=\$(touch $SIDE_EFFECT)
EOF

DOCKER_LOG="$TEST_DIR/docker.log"
export DOCKER_LOG
FAKE_DOCKER="$TEST_DIR/bin/docker"
cat > "$FAKE_DOCKER" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' '---' >> "$DOCKER_LOG"
printf '%s\n' "$@" >> "$DOCKER_LOG"
case "${1:-} ${2:-}" in
  'container inspect') exit 1 ;;
  *) exit 0 ;;
esac
EOF
chmod +x "$FAKE_DOCKER"

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

assert_log_contains() {
  grep -F -- "$1" "$DOCKER_LOG" >/dev/null || fail "Docker log is missing: $1"
}

canonical_file() {
  local source_file=$1
  local source_directory
  source_directory=$(CDPATH='' cd -P -- "$(dirname -- "$source_file")" && pwd)
  printf '%s/%s\n' "$source_directory" "$(basename -- "$source_file")"
}

CHECK_OUTPUT=$(OTHERHOST_DOCKER_BIN="$FAKE_DOCKER" OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/scripts/docker-ui.sh" --check)
printf '%s\n' "$CHECK_OUTPUT" | grep -F '[plan] build otherhost-ui:local and create container otherhost-ui' >/dev/null
[ ! -e "$SIDE_EFFECT" ] || fail 'configuration was executed during Docker check'
if grep -Fx 'build' "$DOCKER_LOG" >/dev/null || grep -Fx 'run' "$DOCKER_LOG" >/dev/null; then
  fail 'read-only Docker check changed container state'
fi

: > "$DOCKER_LOG"
OTHERHOST_DOCKER_BIN="$FAKE_DOCKER" OTHERHOST_CONFIG="$CONFIG_FILE" "$ROOT_DIR/scripts/docker-ui.sh" --apply >/dev/null
[ ! -e "$SIDE_EFFECT" ] || fail 'configuration was executed during Docker apply'

CONFIG_FILE=$(canonical_file "$CONFIG_FILE")
IDENTITY_FILE=$(canonical_file "$IDENTITY_FILE")
KNOWN_HOSTS_FILE=$(canonical_file "$KNOWN_HOSTS_FILE")
CONNECTION_STATE_DIRECTORY="$HOME/Library/Application Support/otherhost"
CONNECTION_STATE_FILE="$CONNECTION_STATE_DIRECTORY/connection-state"

assert_log_contains 'build'
assert_log_contains '--restart'
assert_log_contains 'unless-stopped'
assert_log_contains '--read-only'
assert_log_contains '--cap-drop'
assert_log_contains 'ALL'
assert_log_contains 'no-new-privileges'
assert_log_contains '127.0.0.1:7842:7842'
assert_log_contains "type=bind,src=$CONFIG_FILE,dst=$CONFIG_FILE,readonly"
assert_log_contains "type=bind,src=$IDENTITY_FILE,dst=$IDENTITY_FILE,readonly"
assert_log_contains "type=bind,src=$KNOWN_HOSTS_FILE,dst=$KNOWN_HOSTS_FILE,readonly"
assert_log_contains "type=bind,src=$CONNECTION_STATE_DIRECTORY,dst=$CONNECTION_STATE_DIRECTORY"
assert_log_contains '--connection-state'
assert_log_contains "$CONNECTION_STATE_FILE"
grep -Fx connected "$CONNECTION_STATE_FILE" >/dev/null || fail 'Docker apply did not initialize the shared connection state'
assert_log_contains '--no-open'
assert_log_contains '--listen-address'
assert_log_contains '0.0.0.0'

grep -Fx 'otherhost.local.conf' "$ROOT_DIR/.dockerignore" >/dev/null || fail 'local configuration is not excluded from the Docker build context'
grep -Fx 'devbox.local.conf' "$ROOT_DIR/.dockerignore" >/dev/null || fail 'legacy local configuration is not excluded from the Docker build context'
grep -F 'USER otherhost' "$ROOT_DIR/Dockerfile" >/dev/null || fail 'Docker image does not use the unprivileged dashboard user'

printf '%s\n' 'docker UI tests passed'
