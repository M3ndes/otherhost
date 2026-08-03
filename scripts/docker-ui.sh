#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -P -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/otherhost.sh
. "$ROOT_DIR/lib/otherhost.sh"

ACTION=check
CONFIG_FILE=${OTHERHOST_CONFIG:-}
CONTAINER_NAME=${OTHERHOST_DOCKER_CONTAINER:-otherhost-ui}
IMAGE_NAME=${OTHERHOST_DOCKER_IMAGE:-otherhost-ui:local}
DOCKER_BIN=${OTHERHOST_DOCKER_BIN:-docker}
DASHBOARD_PORT=${OTHERHOST_UI_PORT:-7842}

usage() {
  cat <<'EOF'
Usage: ./scripts/docker-ui.sh [--check|--apply|--down|--logs|--status] [--config PATH]

Actions:
  --check   Validate Docker, configuration, and required read-only mounts.
  --apply   Build and run the localhost-only dashboard with automatic restart.
  --down    Remove only the managed otherhost-ui container.
  --logs    Follow dashboard container logs.
  --status  Show the managed container status.

Environment:
  OTHERHOST_CONFIG           Default configuration path.
  OTHERHOST_DOCKER_CONTAINER Container name (default: otherhost-ui).
  OTHERHOST_DOCKER_IMAGE     Image name (default: otherhost-ui:local).
  OTHERHOST_UI_PORT          Mac loopback port (default: 7842).
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --check|--apply|--down|--logs|--status)
      ACTION=${1#--}
      shift
      ;;
    --config)
      [ "$#" -ge 2 ] || { printf '%s\n' '--config requires a path' >&2; exit 2; }
      CONFIG_FILE=$2
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf 'Unknown argument: %s\n' "$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ -z "$CONFIG_FILE" ]; then
  if [ -f "$ROOT_DIR/otherhost.local.conf" ]; then
    CONFIG_FILE="$ROOT_DIR/otherhost.local.conf"
  elif [ -f "$ROOT_DIR/devbox.local.conf" ]; then
    CONFIG_FILE="$ROOT_DIR/devbox.local.conf"
  else
    CONFIG_FILE="$ROOT_DIR/otherhost.local.conf"
  fi
fi

fail() {
  printf '[fail] %s\n' "$1" >&2
  exit 1
}

absolute_file() {
  local source_file=$1
  local source_directory
  source_directory=$(CDPATH='' cd -P -- "$(dirname -- "$source_file")" 2>/dev/null && pwd) || return 1
  printf '%s/%s\n' "$source_directory" "$(basename -- "$source_file")"
}

validate_mount_path() {
  case "$1" in
    *','*|*"
"*) fail "Docker bind-mount paths cannot contain commas or newlines: $1" ;;
  esac
}

docker_available() {
  if [ -x "$DOCKER_BIN" ]; then
    return 0
  fi
  command -v "$DOCKER_BIN" >/dev/null 2>&1
}

require_docker() {
  docker_available || fail "Docker is not installed: $DOCKER_BIN"
  "$DOCKER_BIN" info >/dev/null 2>&1 || fail 'Docker Desktop is not running'
}

load_mounts() {
  otherhost_require_config "$CONFIG_FILE" || exit 1
  CONFIG_FILE=$(absolute_file "$CONFIG_FILE") || fail "Could not resolve configuration path: $CONFIG_FILE"

  identity_config=$(otherhost_config_get identity_file "$CONFIG_FILE")
  [ -n "$identity_config" ] || fail 'identity_file is required in the configuration'
  IDENTITY_FILE=$(otherhost_resolve_identity_file "$identity_config")
  [ -f "$IDENTITY_FILE" ] || fail "SSH identity not found: $IDENTITY_FILE"
  IDENTITY_FILE=$(absolute_file "$IDENTITY_FILE") || fail "Could not resolve SSH identity path: $IDENTITY_FILE"

  known_hosts_config=$(otherhost_config_get known_hosts_file "$CONFIG_FILE")
  [ -n "$known_hosts_config" ] || fail 'known_hosts_file is required for the Docker dashboard'
  KNOWN_HOSTS_FILE=$(otherhost_resolve_identity_file "$known_hosts_config")
  [ -f "$KNOWN_HOSTS_FILE" ] || fail "Pinned known-hosts file not found: $KNOWN_HOSTS_FILE"
  KNOWN_HOSTS_FILE=$(absolute_file "$KNOWN_HOSTS_FILE") || fail "Could not resolve pinned known-hosts path: $KNOWN_HOSTS_FILE"

  validate_mount_path "$CONFIG_FILE"
  validate_mount_path "$IDENTITY_FILE"
  validate_mount_path "$KNOWN_HOSTS_FILE"
}

validate_port() {
  otherhost_is_positive_integer "$DASHBOARD_PORT" || fail "Invalid dashboard port: $DASHBOARD_PORT"
  [ "$DASHBOARD_PORT" -le 65535 ] || fail "Invalid dashboard port: $DASHBOARD_PORT"
}

container_exists() {
  "$DOCKER_BIN" container inspect "$CONTAINER_NAME" >/dev/null 2>&1
}

case "$ACTION" in
  down)
    require_docker
    if container_exists; then
      "$DOCKER_BIN" rm -f "$CONTAINER_NAME" >/dev/null
      printf '[ok] removed container %s\n' "$CONTAINER_NAME"
    else
      printf '[ok] container %s is already absent\n' "$CONTAINER_NAME"
    fi
    exit 0
    ;;
  logs)
    require_docker
    container_exists || fail "Container does not exist: $CONTAINER_NAME"
    exec "$DOCKER_BIN" logs --follow "$CONTAINER_NAME"
    ;;
  status)
    require_docker
    if container_exists; then
      exec "$DOCKER_BIN" ps --all --filter "name=^/${CONTAINER_NAME}$" --format 'table {{.Names}}\t{{.Status}}\t{{.Ports}}'
    fi
    printf 'Container %s is not installed.\n' "$CONTAINER_NAME"
    exit 1
    ;;
esac

require_docker
validate_port
load_mounts

printf '[ok] Docker Desktop is available\n'
printf '[ok] configuration and pinned SSH files are readable\n'
printf '[ok] dashboard will listen only on http://127.0.0.1:%s\n' "$DASHBOARD_PORT"

if [ "$ACTION" = check ]; then
  if container_exists; then
    printf '[ok] managed container already exists: %s\n' "$CONTAINER_NAME"
  else
    printf '[plan] build %s and create container %s\n' "$IMAGE_NAME" "$CONTAINER_NAME"
  fi
  exit 0
fi

[ "$ACTION" = apply ] || fail "Unsupported action: $ACTION"

"$DOCKER_BIN" build \
  --build-arg "OTHERHOST_UID=$(id -u)" \
  --build-arg "OTHERHOST_GID=$(id -g)" \
  --tag "$IMAGE_NAME" \
  "$ROOT_DIR"

if container_exists; then
  "$DOCKER_BIN" rm -f "$CONTAINER_NAME" >/dev/null
fi

"$DOCKER_BIN" run --detach \
  --name "$CONTAINER_NAME" \
  --restart unless-stopped \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  --env "HOME=$HOME" \
  --publish "127.0.0.1:${DASHBOARD_PORT}:7842" \
  --mount "type=bind,src=${CONFIG_FILE},dst=${CONFIG_FILE},readonly" \
  --mount "type=bind,src=${IDENTITY_FILE},dst=${IDENTITY_FILE},readonly" \
  --mount "type=bind,src=${KNOWN_HOSTS_FILE},dst=${KNOWN_HOSTS_FILE},readonly" \
  "$IMAGE_NAME" \
  --config "$CONFIG_FILE" --listen-address 0.0.0.0 --port 7842 --no-open >/dev/null

printf '[ok] dashboard is running at http://127.0.0.1:%s\n' "$DASHBOARD_PORT"
printf '[ok] restart policy: unless-stopped\n'
