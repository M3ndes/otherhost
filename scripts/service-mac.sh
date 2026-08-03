#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -P -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/otherhost.sh
. "$ROOT_DIR/lib/otherhost.sh"

ACTION=check
CONFIG_FILE=${OTHERHOST_CONFIG:-}
PLATFORM=${OTHERHOST_PLATFORM:-$(uname -s)}
LAUNCHCTL_BIN=${OTHERHOST_LAUNCHCTL_BIN:-launchctl}
COMMAND_BIN=${OTHERHOST_COMMAND_BIN:-"$HOME/.local/bin/otherhost"}
SERVICE_LABEL=dev.otherhost.connect
LAUNCH_AGENT_DIRECTORY=${OTHERHOST_LAUNCH_AGENT_DIR:-"$HOME/Library/LaunchAgents"}
LOG_DIRECTORY=${OTHERHOST_SERVICE_LOG_DIR:-"$HOME/Library/Logs/otherhost"}
PLIST_FILE="$LAUNCH_AGENT_DIRECTORY/$SERVICE_LABEL.plist"
STANDARD_LOG="$LOG_DIRECTORY/connect.log"
ERROR_LOG="$LOG_DIRECTORY/connect-error.log"
SERVICE_DOMAIN="gui/$(id -u)"
SERVICE_TARGET="$SERVICE_DOMAIN/$SERVICE_LABEL"

usage() {
  cat <<'EOF'
Usage: scripts/service-mac.sh [--check|--apply|status|logs|remove] [--config PATH]

The check action is read-only. Apply installs or refreshes a macOS LaunchAgent
that keeps `otherhost connect` running and lets launchd restart lost tunnels.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --check) ACTION=check; shift ;;
    --apply) ACTION=apply; shift ;;
    status|logs|remove) ACTION=$1; shift ;;
    --config)
      [ "$#" -ge 2 ] || { printf '%s\n' '--config requires a path' >&2; exit 2; }
      CONFIG_FILE=$2
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

fail() {
  printf '[fail] %s\n' "$1" >&2
  exit 1
}

ok() {
  printf '[ok] %s\n' "$1"
}

absolute_file() {
  local source_file=$1
  local source_directory
  source_directory=$(CDPATH='' cd -P -- "$(dirname -- "$source_file")" 2>/dev/null && pwd) || return 1
  printf '%s/%s\n' "$source_directory" "$(basename -- "$source_file")"
}

xml_escape() {
  printf '%s' "$1" | sed \
    -e 's/&/\&amp;/g' \
    -e 's/</\&lt;/g' \
    -e 's/>/\&gt;/g' \
    -e 's/"/\&quot;/g' \
    -e "s/'/\\\&apos;/g"
}

resolve_config() {
  if [ -z "$CONFIG_FILE" ]; then
    if [ -f "$ROOT_DIR/otherhost.local.conf" ]; then
      CONFIG_FILE="$ROOT_DIR/otherhost.local.conf"
    elif [ -f "$ROOT_DIR/devbox.local.conf" ]; then
      CONFIG_FILE="$ROOT_DIR/devbox.local.conf"
    else
      CONFIG_FILE="$ROOT_DIR/otherhost.local.conf"
    fi
  fi
  otherhost_require_config "$CONFIG_FILE" || exit 1
  CONFIG_FILE=$(absolute_file "$CONFIG_FILE") || fail "Could not resolve configuration path: $CONFIG_FILE"
}

render_plist() {
  local escaped_command
  local escaped_config
  local escaped_output
  local escaped_error
  escaped_command=$(xml_escape "$COMMAND_BIN")
  escaped_config=$(xml_escape "$CONFIG_FILE")
  escaped_output=$(xml_escape "$STANDARD_LOG")
  escaped_error=$(xml_escape "$ERROR_LOG")
  cat <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>$SERVICE_LABEL</string>
  <key>ProgramArguments</key>
  <array>
    <string>$escaped_command</string>
    <string>--config</string>
    <string>$escaped_config</string>
    <string>connect</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>ThrottleInterval</key>
  <integer>10</integer>
  <key>ProcessType</key>
  <string>Background</string>
  <key>StandardOutPath</key>
  <string>$escaped_output</string>
  <key>StandardErrorPath</key>
  <string>$escaped_error</string>
</dict>
</plist>
EOF
}

service_loaded() {
  "$LAUNCHCTL_BIN" print "$SERVICE_TARGET" >/dev/null 2>&1
}

[ "$PLATFORM" = Darwin ] || fail 'the connection service currently supports macOS launchd only'
command -v "$LAUNCHCTL_BIN" >/dev/null 2>&1 || [ -x "$LAUNCHCTL_BIN" ] || fail "launchctl is not available: $LAUNCHCTL_BIN"

if [ "$ACTION" = logs ]; then
  [ -f "$STANDARD_LOG" ] || [ -f "$ERROR_LOG" ] || fail "Service logs do not exist yet: $LOG_DIRECTORY"
  exec tail -n 100 -F "$STANDARD_LOG" "$ERROR_LOG"
fi

if [ "$ACTION" = remove ]; then
  if service_loaded; then
    "$LAUNCHCTL_BIN" bootout "$SERVICE_TARGET" >/dev/null 2>&1 || fail "Could not stop $SERVICE_LABEL"
  fi
  if [ -f "$PLIST_FILE" ]; then
    rm -f "$PLIST_FILE"
    ok "removed $PLIST_FILE"
  else
    ok 'connection service is already absent'
  fi
  printf 'Logs were preserved in %s\n' "$LOG_DIRECTORY"
  exit 0
fi

if [ "$ACTION" = status ]; then
  [ -f "$PLIST_FILE" ] || fail 'connection service is not installed; run otherhost service --apply'
  if service_loaded; then
    service_description=$($LAUNCHCTL_BIN print "$SERVICE_TARGET" 2>/dev/null || true)
    if printf '%s\n' "$service_description" | grep -Eq 'state = running|pid = [0-9]+'; then
      ok 'connection service is running'
    else
      printf '[warn] connection service is loaded and waiting to restart\n'
    fi
    printf 'Logs: %s\n' "$LOG_DIRECTORY"
    exit 0
  fi
  fail 'connection service is installed but not loaded'
fi

resolve_config
[ -x "$COMMAND_BIN" ] || fail "Installed Otherhost command not found: $COMMAND_BIN"

expected_plist=$(render_plist)
if [ -f "$PLIST_FILE" ] && [ "$(cat "$PLIST_FILE")" = "$expected_plist" ]; then
  ok "LaunchAgent definition is current: $PLIST_FILE"
else
  printf '[plan] install or refresh %s\n' "$PLIST_FILE"
fi
if service_loaded; then
  ok 'connection service is loaded'
else
  printf '[plan] load %s and start the SSH tunnels\n' "$SERVICE_LABEL"
fi

[ "$ACTION" = check ] && exit 0
[ "$ACTION" = apply ] || fail "Unsupported service action: $ACTION"

mkdir -p "$LAUNCH_AGENT_DIRECTORY" "$LOG_DIRECTORY"
chmod 700 "$LAUNCH_AGENT_DIRECTORY" "$LOG_DIRECTORY"
temporary_plist=$(mktemp "$LAUNCH_AGENT_DIRECTORY/.otherhost-connect.XXXXXX") || fail 'Could not create a temporary LaunchAgent definition'
if ! render_plist > "$temporary_plist"; then
  rm -f "$temporary_plist"
  fail 'Could not render the LaunchAgent definition'
fi
chmod 600 "$temporary_plist"
mv "$temporary_plist" "$PLIST_FILE"

if service_loaded; then
  "$LAUNCHCTL_BIN" bootout "$SERVICE_TARGET" >/dev/null 2>&1 || fail "Could not reload $SERVICE_LABEL"
fi
"$LAUNCHCTL_BIN" bootstrap "$SERVICE_DOMAIN" "$PLIST_FILE" >/dev/null
"$LAUNCHCTL_BIN" enable "$SERVICE_TARGET" >/dev/null
"$LAUNCHCTL_BIN" kickstart -k "$SERVICE_TARGET" >/dev/null
service_loaded || fail 'launchd did not load the connection service'

ok 'connection service is installed and supervised by launchd'
printf 'Status: otherhost service status\n'
printf 'Logs: otherhost service logs\n'
