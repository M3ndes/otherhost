#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/devbox.sh
. "$ROOT_DIR/lib/devbox.sh"

CONFIG_FILE=${DEVBOX_CONFIG:-"$ROOT_DIR/devbox.local.conf"}
INSTALL_ROOT=${DEVBOX_USER_INSTALL_ROOT:-"$HOME/.local/lib/devbox-bridge"}
PAIRING_HELPER="$INSTALL_ROOT/devbox-pair"
HOST_KEY="$INSTALL_ROOT/ssh_host_ed25519_key.pub"
AUTHORIZED_KEYS="$HOME/.ssh/authorized_keys"
PAIRING_DISCOVERY_ADDRESS=239.255.67.89:25370
PAIRING_PORT=25371

devbox_require_config "$CONFIG_FILE" || exit 1
SSH_PORT=$(devbox_config_get ssh_port "$CONFIG_FILE")
SSH_USER=$(devbox_config_get ssh_user "$CONFIG_FILE")
[ "$SSH_USER" = "$(id -un)" ] || { printf '[fail] ssh_user must match the current WSL user\n' >&2; exit 1; }
[ -x "$PAIRING_HELPER" ] || { printf '[fail] rootless pairing helper is not installed\n' >&2; exit 1; }
[ -r "$HOST_KEY" ] || { printf '[fail] rootless SSH host key is missing\n' >&2; exit 1; }
systemctl --user is-active --quiet devbox-bridge-sshd.service || { printf '[fail] rootless SSH service is not active\n' >&2; exit 1; }

exec "$PAIRING_HELPER" host \
  --name "$(hostname -s)" \
  --ssh-user "$SSH_USER" \
  --ssh-port "$SSH_PORT" \
  --pair-port "$PAIRING_PORT" \
  --discovery-address "$PAIRING_DISCOVERY_ADDRESS" \
  --authorized-keys "$AUTHORIZED_KEYS" \
  --ssh-host-key "$(cat "$HOST_KEY")"
