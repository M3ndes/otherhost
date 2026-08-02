#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/otherhost.sh
. "$ROOT_DIR/lib/otherhost.sh"

CONFIG_FILE=${OTHERHOST_CONFIG:-${DEVBOX_CONFIG:-"$ROOT_DIR/otherhost.local.conf"}}
if [ -z "${OTHERHOST_CONFIG:-}${DEVBOX_CONFIG:-}" ] && [ ! -f "$CONFIG_FILE" ] && [ -f "$ROOT_DIR/devbox.local.conf" ]; then
  CONFIG_FILE="$ROOT_DIR/devbox.local.conf"
fi
INSTALL_ROOT=${OTHERHOST_USER_INSTALL_ROOT:-${DEVBOX_USER_INSTALL_ROOT:-"$HOME/.local/lib/otherhost"}}
PAIRING_HELPER="$INSTALL_ROOT/otherhost-pair"
HOST_KEY="$INSTALL_ROOT/ssh_host_ed25519_key.pub"
AUTHORIZED_KEYS="$HOME/.ssh/authorized_keys"
PAIRING_DISCOVERY_ADDRESS=239.255.67.89:25370
PAIRING_PORT=25371

otherhost_require_config "$CONFIG_FILE" || exit 1
if [ ! -x "$PAIRING_HELPER" ] && [ -x "$HOME/.local/lib/devbox-bridge/devbox-pair" ]; then
  INSTALL_ROOT="$HOME/.local/lib/devbox-bridge"
  PAIRING_HELPER="$INSTALL_ROOT/devbox-pair"
  HOST_KEY="$INSTALL_ROOT/ssh_host_ed25519_key.pub"
fi
SSH_PORT=$(otherhost_config_get ssh_port "$CONFIG_FILE")
SSH_USER=$(otherhost_config_get ssh_user "$CONFIG_FILE")
[ "$SSH_USER" = "$(id -un)" ] || { printf '[fail] ssh_user must match the current WSL user\n' >&2; exit 1; }
[ -x "$PAIRING_HELPER" ] || { printf '[fail] rootless pairing helper is not installed\n' >&2; exit 1; }
[ -r "$HOST_KEY" ] || { printf '[fail] rootless SSH host key is missing\n' >&2; exit 1; }
if systemctl --user is-active --quiet otherhost-sshd.service; then
  :
elif systemctl --user is-active --quiet devbox-bridge-sshd.service; then
  :
else
  printf '[fail] rootless SSH service is not active\n' >&2
  exit 1
fi

exec "$PAIRING_HELPER" host \
  --name "$(hostname -s)" \
  --ssh-user "$SSH_USER" \
  --ssh-port "$SSH_PORT" \
  --pair-port "$PAIRING_PORT" \
  --discovery-address "$PAIRING_DISCOVERY_ADDRESS" \
  --authorized-keys "$AUTHORIZED_KEYS" \
  --ssh-host-key "$(cat "$HOST_KEY")"
