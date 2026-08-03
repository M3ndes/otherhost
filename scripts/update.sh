#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -P -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/otherhost.sh
. "$ROOT_DIR/lib/otherhost.sh"

ACTION=check
CONFIG_FILE=${OTHERHOST_CONFIG:-}
GIT_BIN=${OTHERHOST_GIT_BIN:-git}
SSH_BIN=${OTHERHOST_SSH_BIN:-ssh}
BOOTSTRAP_MAC_BIN=${OTHERHOST_BOOTSTRAP_MAC_BIN:-"$ROOT_DIR/scripts/bootstrap-mac.sh"}
SERVICE_MAC_BIN=${OTHERHOST_SERVICE_MAC_BIN:-"$ROOT_DIR/scripts/service-mac.sh"}
CANONICAL_REPOSITORY=https://github.com/M3ndes/otherhost.git

usage() {
  cat <<'EOF'
Usage: scripts/update.sh [--check|--apply|--version] [--config PATH]

Check compares the Mac checkout, upstream main, the pinned WSL operational
checkout, and Windows installation state without changing them. Apply safely
fast-forwards a clean Mac main checkout. Windows and WSL remain bound together
and are updated explicitly from Windows with `setup.cmd -Update`.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --check) ACTION=check; shift ;;
    --apply) ACTION=apply; shift ;;
    --version) ACTION=version; shift ;;
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

warn() {
  printf '[warn] %s\n' "$1"
}

valid_revision() {
  case "$1" in
    ''|*[!0-9a-f]*) return 1 ;;
  esac
  [ "${#1}" -eq 40 ]
}

valid_compatibility() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
  esac
}

short_revision() {
  if valid_revision "$1"; then
    printf '%.12s' "$1"
  else
    printf 'unknown'
  fi
}

command_available() {
  if [ -x "$1" ]; then
    return 0
  fi
  command -v "$1" >/dev/null 2>&1
}

command_available "$GIT_BIN" || fail "Git is not available: $GIT_BIN"

LOCAL_REVISION=$(otherhost_repository_revision "$ROOT_DIR") || fail 'Could not read the Mac checkout revision'
valid_revision "$LOCAL_REVISION" || fail 'The Mac checkout revision is invalid'
LOCAL_COMPATIBILITY=$(otherhost_compatibility_version "$ROOT_DIR") || fail 'Could not read the local compatibility version'

if [ "$ACTION" = version ]; then
  pairing_version=$(sed -n 's/^PAIRING_VERSION=//p' "$ROOT_DIR/scripts/install-pairing-helper.sh" | sed -n '1p')
  printf 'otherhost source %s\n' "$(short_revision "$LOCAL_REVISION")"
  printf 'compatibility %s\n' "$LOCAL_COMPATIBILITY"
  printf 'pairing-helper %s\n' "${pairing_version:-unknown}"
  exit 0
fi

origin=$($GIT_BIN -C "$ROOT_DIR" remote get-url origin 2>/dev/null) || fail 'Could not read the Mac checkout origin'
case "$origin" in
  "$CANONICAL_REPOSITORY"|git@github.com:M3ndes/otherhost.git|ssh://git@github.com/M3ndes/otherhost.git) ;;
  *) fail "The Mac checkout does not use the canonical Otherhost origin: $origin" ;;
esac

UPSTREAM_LINE=$($GIT_BIN -c protocol.ext.allow=never ls-remote "$CANONICAL_REPOSITORY" refs/heads/main 2>/dev/null) || fail 'Could not read the latest Otherhost revision from GitHub'
UPSTREAM_REVISION=$(printf '%s\n' "$UPSTREAM_LINE" | awk 'NR == 1 { print $1 }')
valid_revision "$UPSTREAM_REVISION" || fail 'GitHub returned an invalid main revision'

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

HOST=$(otherhost_config_get host "$CONFIG_FILE")
SSH_USER=$(otherhost_config_get ssh_user "$CONFIG_FILE")
SSH_PORT=$(otherhost_config_get ssh_port "$CONFIG_FILE")
IDENTITY_CONFIG=$(otherhost_config_get identity_file "$CONFIG_FILE")
KNOWN_HOSTS_CONFIG=$(otherhost_config_get known_hosts_file "$CONFIG_FILE")
IDENTITY_FILE=$(otherhost_resolve_identity_file "$IDENTITY_CONFIG")
KNOWN_HOSTS_FILE=$(otherhost_resolve_identity_file "$KNOWN_HOSTS_CONFIG")

case "$HOST" in ''|*[!A-Za-z0-9._:-]*) fail 'host is invalid in the local configuration' ;; esac
case "$SSH_USER" in ''|*[!A-Za-z0-9._-]*) fail 'ssh_user is invalid in the local configuration' ;; esac
if ! otherhost_is_positive_integer "$SSH_PORT" || [ "$SSH_PORT" -gt 65535 ]; then
  fail 'ssh_port must be between 1 and 65535'
fi
[ -f "$IDENTITY_FILE" ] || fail "SSH identity does not exist: $IDENTITY_FILE"
if [ -z "$KNOWN_HOSTS_CONFIG" ] || [ ! -f "$KNOWN_HOSTS_FILE" ]; then
  fail 'A pinned known_hosts_file is required to inspect host versions'
fi
command_available "$SSH_BIN" || fail "OpenSSH is not available: $SSH_BIN"

SSH_ARGS=(
  -p "$SSH_PORT"
  -i "$IDENTITY_FILE"
  -o IdentitiesOnly=yes
  -o BatchMode=yes
  -o StrictHostKeyChecking=yes
  -o "UserKnownHostsFile=$KNOWN_HOSTS_FILE"
  -o ConnectTimeout=5
  -o ServerAliveInterval=5
  -o ServerAliveCountMax=1
)

if REMOTE_STATE=$($SSH_BIN "${SSH_ARGS[@]}" "$SSH_USER@$HOST" 'bash -s' <<'REMOTE_SCRIPT'
set -u

read_state_value() {
  key=$1
  file=$2
  awk -F= -v wanted="$key" '$1 == wanted { print $2; exit }' "$file" 2>/dev/null
}

repository="$HOME/src/otherhost"
if [ ! -d "$repository/.git" ] && [ -d "$HOME/src/devbox-bridge/.git" ]; then
  repository="$HOME/src/devbox-bridge"
fi
wsl_state="$HOME/.local/state/otherhost/install-state"
wsl_revision=$(read_state_value revision "$wsl_state")
wsl_compatibility=$(read_state_value compatibility "$wsl_state")
if [ -z "$wsl_revision" ] && [ -d "$repository/.git" ]; then
  wsl_revision=$(git -c core.fsmonitor=false -c core.hooksPath=/dev/null -C "$repository" rev-parse --verify HEAD 2>/dev/null || true)
fi
if [ -z "$wsl_compatibility" ] && [ -f "$repository/config/compatibility-version" ]; then
  wsl_compatibility=$(sed -n '1p' "$repository/config/compatibility-version")
fi

windows_state=''
if command -v powershell.exe >/dev/null 2>&1; then
  windows_state=$(powershell.exe -NoProfile -NonInteractive -Command '$path = Join-Path $env:LOCALAPPDATA "otherhost\install-state"; if (Test-Path -LiteralPath $path -PathType Leaf) { [Console]::Out.Write([IO.File]::ReadAllText($path)) }' 2>/dev/null | tr -d '\r' || true)
fi
windows_revision=$(printf '%s\n' "$windows_state" | awk -F= '$1 == "revision" { print $2; exit }')
windows_compatibility=$(printf '%s\n' "$windows_state" | awk -F= '$1 == "compatibility" { print $2; exit }')

printf 'wsl.revision=%s\n' "$wsl_revision"
printf 'wsl.compatibility=%s\n' "$wsl_compatibility"
printf 'windows.revision=%s\n' "$windows_revision"
printf 'windows.compatibility=%s\n' "$windows_compatibility"
REMOTE_SCRIPT
); then
  :
else
  REMOTE_STATE=''
  warn 'Could not inspect Windows and WSL over pinned SSH; remote versions are unknown'
fi

remote_value() {
  printf '%s\n' "$REMOTE_STATE" | awk -F= -v wanted="$1" '$1 == wanted { print $2; exit }'
}

WSL_REVISION=$(remote_value wsl.revision)
WSL_COMPATIBILITY=$(remote_value wsl.compatibility)
WINDOWS_REVISION=$(remote_value windows.revision)
WINDOWS_COMPATIBILITY=$(remote_value windows.compatibility)
valid_revision "$WSL_REVISION" || WSL_REVISION=''
valid_compatibility "$WSL_COMPATIBILITY" || WSL_COMPATIBILITY=''
valid_revision "$WINDOWS_REVISION" || WINDOWS_REVISION=''
valid_compatibility "$WINDOWS_COMPATIBILITY" || WINDOWS_COMPATIBILITY=''

component_status() {
  local revision=$1
  if [ -z "$revision" ]; then
    printf 'unknown'
  elif [ "$revision" = "$UPSTREAM_REVISION" ]; then
    printf 'current'
  else
    printf 'update available'
  fi
}

printf 'Otherhost update check\n\n'
printf 'Upstream main  %s\n' "$(short_revision "$UPSTREAM_REVISION")"
printf 'Mac client     %s  %s  compatibility %s\n' "$(short_revision "$LOCAL_REVISION")" "$(component_status "$LOCAL_REVISION")" "$LOCAL_COMPATIBILITY"
printf 'WSL host       %s  %s  compatibility %s\n' "$(short_revision "$WSL_REVISION")" "$(component_status "$WSL_REVISION")" "${WSL_COMPATIBILITY:-unknown}"
printf 'Windows setup  %s  %s  compatibility %s\n' "$(short_revision "$WINDOWS_REVISION")" "$(component_status "$WINDOWS_REVISION")" "${WINDOWS_COMPATIBILITY:-unknown}"

INCOMPATIBLE=0
if [ -n "$WSL_COMPATIBILITY" ] && [ "$WSL_COMPATIBILITY" != "$LOCAL_COMPATIBILITY" ]; then
  INCOMPATIBLE=1
  warn "WSL compatibility $WSL_COMPATIBILITY does not match the Mac client compatibility $LOCAL_COMPATIBILITY"
fi
if [ -n "$WINDOWS_COMPATIBILITY" ] && [ "$WINDOWS_COMPATIBILITY" != "$LOCAL_COMPATIBILITY" ]; then
  INCOMPATIBLE=1
  warn "Windows compatibility $WINDOWS_COMPATIBILITY does not match the Mac client compatibility $LOCAL_COMPATIBILITY"
fi
if [ -n "$WINDOWS_REVISION" ] && [ -n "$WSL_REVISION" ] && [ "$WINDOWS_REVISION" != "$WSL_REVISION" ]; then
  warn 'Windows and WSL revisions differ; rerun setup.cmd -Update on Windows to restore the pinned revision boundary'
fi
if [ -z "$WINDOWS_REVISION" ] || [ -z "$WSL_COMPATIBILITY" ]; then
  warn 'The host predates installation-state reporting; run setup.cmd -Update on Windows once'
fi

if [ "$ACTION" = check ]; then
  if [ "$LOCAL_REVISION" != "$UPSTREAM_REVISION" ]; then
    printf '\nRun otherhost update --apply to fast-forward this Mac checkout.\n'
  fi
  if [ "$WSL_REVISION" != "$UPSTREAM_REVISION" ] || [ "$WINDOWS_REVISION" != "$UPSTREAM_REVISION" ]; then
    printf 'On Windows, run .\\setup.cmd -Update to update Windows and repin WSL together.\n'
  fi
  [ "$INCOMPATIBLE" -eq 0 ]
  exit $?
fi

[ "$ACTION" = apply ] || fail "Unsupported update action: $ACTION"

branch=$($GIT_BIN -C "$ROOT_DIR" symbolic-ref --quiet --short HEAD 2>/dev/null || true)
[ "$branch" = main ] || fail "Mac updates require the main branch; current branch: ${branch:-detached}"
working_changes=$($GIT_BIN -c core.fsmonitor=false -c core.hooksPath=/dev/null -C "$ROOT_DIR" status --porcelain --untracked-files=all)
[ -z "$working_changes" ] || fail 'The Mac checkout has local changes; commit or remove them before updating'

if [ "$LOCAL_REVISION" != "$UPSTREAM_REVISION" ]; then
  $GIT_BIN -c protocol.ext.allow=never -c core.hooksPath=/dev/null -C "$ROOT_DIR" fetch --no-tags "$CANONICAL_REPOSITORY" main
  fetched_revision=$($GIT_BIN -C "$ROOT_DIR" rev-parse --verify FETCH_HEAD)
  [ "$fetched_revision" = "$UPSTREAM_REVISION" ] || fail 'Fetched revision does not match the verified upstream revision'
  $GIT_BIN -c core.hooksPath=/dev/null -C "$ROOT_DIR" merge --ff-only "$fetched_revision"
  printf '[ok] updated the Mac checkout to %s\n' "$(short_revision "$fetched_revision")"
else
  printf '[ok] Mac checkout is already current\n'
fi

"$BOOTSTRAP_MAC_BIN" --apply --config "$CONFIG_FILE"
if [ -f "$HOME/Library/LaunchAgents/dev.otherhost.connect.plist" ]; then
  "$SERVICE_MAC_BIN" --apply --config "$CONFIG_FILE"
fi

printf '\nMac update complete.\n'
if [ "$WSL_REVISION" != "$UPSTREAM_REVISION" ] || [ "$WINDOWS_REVISION" != "$UPSTREAM_REVISION" ]; then
  printf 'On Windows, run .\\setup.cmd -Update to update Windows and repin WSL together.\n'
fi
