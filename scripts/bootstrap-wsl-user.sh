#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/otherhost.sh
. "$ROOT_DIR/lib/otherhost.sh"

MODE=check
CONFIG_FILE=${OTHERHOST_CONFIG:-${DEVBOX_CONFIG:-"$ROOT_DIR/otherhost.local.conf"}}
if [ -z "${OTHERHOST_CONFIG:-}${DEVBOX_CONFIG:-}" ] && [ ! -f "$CONFIG_FILE" ] && [ -f "$ROOT_DIR/devbox.local.conf" ]; then
  CONFIG_FILE="$ROOT_DIR/devbox.local.conf"
fi
INSTALL_ROOT=${OTHERHOST_USER_INSTALL_ROOT:-${DEVBOX_USER_INSTALL_ROOT:-"$HOME/.local/lib/otherhost"}}
SERVICE_FILE="$HOME/.config/systemd/user/otherhost-sshd.service"
PAIRING_VERSION=v0.1.1

usage() {
  cat <<'EOF'
Usage: scripts/bootstrap-wsl-user.sh [--apply] [--config PATH]

Install a public-key-only SSH host entirely inside the current WSL user account.
This mode requires no sudo and is intended for an existing mirrored-network WSL
environment. It keeps SSH on an unprivileged port and permits forwarding only to
loopback services inside WSL.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --apply) MODE=apply; shift ;;
    --config)
      [ "$#" -ge 2 ] || { printf '%s\n' '--config requires a path' >&2; exit 2; }
      CONFIG_FILE=$2
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) printf 'Unknown argument: %s\n' "$1" >&2; usage >&2; exit 2 ;;
  esac
done

ok() { printf '[ok] %s\n' "$*"; }
warn() { printf '[warn] %s\n' "$*"; }
fail() { printf '[fail] %s\n' "$*" >&2; exit 1; }

grep -qi microsoft /proc/version 2>/dev/null || fail 'this bootstrap must run inside WSL'
otherhost_require_config "$CONFIG_FILE" || exit 1

SSH_PORT=$(otherhost_config_get ssh_port "$CONFIG_FILE")
CURRENT_USER=$(id -un)
if ! otherhost_is_positive_integer "$SSH_PORT" || [ "$SSH_PORT" -lt 1024 ] || [ "$SSH_PORT" -gt 65535 ]; then
  fail 'rootless ssh_port must be between 1024 and 65535'
fi
if [ "$(ps -p 1 -o comm= 2>/dev/null | tr -d ' ')" != systemd ]; then
  fail 'rootless mode requires systemd to be active in WSL'
fi
systemctl --user is-system-running >/dev/null 2>&1 || fail 'the systemd user manager is not available'

HOST_KEY="$INSTALL_ROOT/ssh_host_ed25519_key"
AUTHORIZED_KEYS="$HOME/.ssh/authorized_keys"
SSHD_CONFIG="$INSTALL_ROOT/sshd_config"
PAIRING_HELPER="$INSTALL_ROOT/otherhost-pair"

ok "WSL user environment detected: $CURRENT_USER"
ok "rootless SSH port: $SSH_PORT"

if [ "$MODE" = apply ]; then
  for command_name in apt-get dpkg-deb ssh-keygen systemctl; do
    command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
  done

  TEMPORARY_DIRECTORY=$(mktemp -d)
  trap 'rm -rf "$TEMPORARY_DIRECTORY"' EXIT HUP INT TERM
  (
    cd "$TEMPORARY_DIRECTORY"
    apt-get download openssh-server libwrap0
  )
  OPENSSH_PACKAGE=$(find "$TEMPORARY_DIRECTORY" -maxdepth 1 -type f -name 'openssh-server_*.deb' -print -quit)
  LIBWRAP_PACKAGE=$(find "$TEMPORARY_DIRECTORY" -maxdepth 1 -type f -name 'libwrap0_*.deb' -print -quit)
  if [ -z "$OPENSSH_PACKAGE" ] || [ -z "$LIBWRAP_PACKAGE" ]; then
    fail 'Ubuntu SSH packages were not downloaded'
  fi

  PACKAGE_VERSION=$(dpkg-deb -f "$OPENSSH_PACKAGE" Version | sed 's/[^A-Za-z0-9._+-]/_/g')
  PACKAGE_ROOT="$INSTALL_ROOT/openssh-$PACKAGE_VERSION"
  if [ ! -x "$PACKAGE_ROOT/usr/sbin/sshd" ]; then
    mkdir -p "$PACKAGE_ROOT"
    dpkg-deb -x "$OPENSSH_PACKAGE" "$PACKAGE_ROOT"
    dpkg-deb -x "$LIBWRAP_PACKAGE" "$PACKAGE_ROOT"
    ok "installed user-scoped OpenSSH from Ubuntu package $PACKAGE_VERSION"
  else
    ok "preserved user-scoped OpenSSH package $PACKAGE_VERSION"
  fi

  LIBWRAP=$(find "$PACKAGE_ROOT" -name 'libwrap.so.0' -print -quit)
  [ -n "$LIBWRAP" ] || fail 'the extracted libwrap runtime is missing'
  LIBRARY_PATH=$(dirname -- "$LIBWRAP")
  SSHD="$PACKAGE_ROOT/usr/sbin/sshd"

  mkdir -p "$INSTALL_ROOT" "$HOME/.ssh" "$(dirname -- "$SERVICE_FILE")"
  chmod 700 "$INSTALL_ROOT" "$HOME/.ssh"
  touch "$AUTHORIZED_KEYS"
  chmod 600 "$AUTHORIZED_KEYS"
  LEGACY_INSTALL_ROOT="$HOME/.local/lib/devbox-bridge"
  if [ ! -f "$HOST_KEY" ] && [ -f "$LEGACY_INSTALL_ROOT/ssh_host_ed25519_key" ] && [ -f "$LEGACY_INSTALL_ROOT/ssh_host_ed25519_key.pub" ]; then
    install -m 600 "$LEGACY_INSTALL_ROOT/ssh_host_ed25519_key" "$HOST_KEY"
    install -m 644 "$LEGACY_INSTALL_ROOT/ssh_host_ed25519_key.pub" "$HOST_KEY.pub"
    ok 'migrated the existing rootless SSH host identity'
  fi
  if [ ! -f "$HOST_KEY" ]; then
    ssh-keygen -q -t ed25519 -N '' -f "$HOST_KEY"
    ok 'created a dedicated rootless SSH host identity'
  fi
  chmod 600 "$HOST_KEY"
  chmod 644 "$HOST_KEY.pub"

  CONFIG_TEMPORARY="$TEMPORARY_DIRECTORY/sshd_config"
  cat > "$CONFIG_TEMPORARY" <<EOF
Port $SSH_PORT
ListenAddress 0.0.0.0
HostKey $HOST_KEY
PidFile $INSTALL_ROOT/sshd.pid
AuthorizedKeysFile $AUTHORIZED_KEYS
AuthenticationMethods publickey
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitEmptyPasswords no
UsePAM no
PermitRootLogin no
AllowUsers $CURRENT_USER
StrictModes yes
AllowTcpForwarding local
AllowStreamLocalForwarding no
PermitOpen localhost:* 127.0.0.1:* [::1]:*
GatewayPorts no
X11Forwarding no
AllowAgentForwarding no
PermitTunnel no
PermitUserEnvironment no
MaxAuthTries 3
LoginGraceTime 30
LogLevel VERBOSE
Subsystem sftp internal-sftp
EOF
  install -m 600 "$CONFIG_TEMPORARY" "$SSHD_CONFIG"

  LD_LIBRARY_PATH="$LIBRARY_PATH" "$SSHD" -t -f "$SSHD_CONFIG"
  EFFECTIVE=$(LD_LIBRARY_PATH="$LIBRARY_PATH" "$SSHD" -T -f "$SSHD_CONFIG" -C "user=$CURRENT_USER,host=localhost,addr=127.0.0.1")
  printf '%s\n' "$EFFECTIVE" | grep -Fx "port $SSH_PORT" >/dev/null || fail 'effective rootless SSH port is incorrect'
  printf '%s\n' "$EFFECTIVE" | grep -Fx 'passwordauthentication no' >/dev/null || fail 'rootless SSH password authentication is not disabled'
  printf '%s\n' "$EFFECTIVE" | grep -Fx 'kbdinteractiveauthentication no' >/dev/null || fail 'rootless SSH keyboard authentication is not disabled'
  printf '%s\n' "$EFFECTIVE" | grep -Fx 'allowtcpforwarding local' >/dev/null || fail 'rootless SSH forwarding policy is incorrect'
  ok 'verified the effective rootless SSH policy'

  SERVICE_TEMPORARY="$TEMPORARY_DIRECTORY/otherhost-sshd.service"
  cat > "$SERVICE_TEMPORARY" <<EOF
[Unit]
Description=Otherhost user-scoped SSH host
After=network-online.target

[Service]
Type=simple
Environment=LD_LIBRARY_PATH=$LIBRARY_PATH
ExecStart=$SSHD -D -e -f $SSHD_CONFIG
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
EOF
  install -m 600 "$SERVICE_TEMPORARY" "$SERVICE_FILE"
  systemctl --user daemon-reload
  LEGACY_SERVICE_WAS_ACTIVE=0
  if systemctl --user is-active --quiet devbox-bridge-sshd.service; then
    LEGACY_SERVICE_WAS_ACTIVE=1
    systemctl --user stop devbox-bridge-sshd.service
  fi
  if ! systemctl --user enable --now otherhost-sshd.service; then
    if [ "$LEGACY_SERVICE_WAS_ACTIVE" -eq 1 ]; then
      systemctl --user start devbox-bridge-sshd.service || true
    fi
    fail 'rootless SSH service did not start'
  fi
  systemctl --user is-active --quiet otherhost-sshd.service || fail 'rootless SSH service did not start'
  if systemctl --user is-enabled --quiet devbox-bridge-sshd.service 2>/dev/null; then
    systemctl --user disable devbox-bridge-sshd.service
  fi
  ok 'rootless SSH service is active'

  PAIRING_BIN_OVERRIDE=${OTHERHOST_PAIR_BIN:-${DEVBOX_PAIR_BIN:-}}
  if [ -n "$PAIRING_BIN_OVERRIDE" ]; then
    [ -x "$PAIRING_BIN_OVERRIDE" ] || fail 'OTHERHOST_PAIR_BIN is not executable'
    install -m 700 "$PAIRING_BIN_OVERRIDE" "$PAIRING_HELPER"
    ok "installed the supplied pairing helper: $PAIRING_HELPER"
  else
    INSTALLED_PAIRING_VERSION=''
    if [ -x "$PAIRING_HELPER" ]; then
      INSTALLED_PAIRING_VERSION=$("$PAIRING_HELPER" version 2>/dev/null | awk 'NR == 1 { print $2 }')
    fi
    if [ "$INSTALLED_PAIRING_VERSION" = "$PAIRING_VERSION" ]; then
      ok "pairing helper is installed: $PAIRING_HELPER"
    elif [ -x "$HOME/.local/lib/devbox-bridge/devbox-pair" ] && [ "$("$HOME/.local/lib/devbox-bridge/devbox-pair" version 2>/dev/null | awk 'NR == 1 { print $2 }')" = "$PAIRING_VERSION" ]; then
      install -m 700 "$HOME/.local/lib/devbox-bridge/devbox-pair" "$PAIRING_HELPER"
      ok "migrated pairing helper: $PAIRING_HELPER"
    else
      "$ROOT_DIR/scripts/install-pairing-helper-wsl.sh" "$PAIRING_HELPER"
    fi
  fi

  otherhost_write_installation_state "$ROOT_DIR" || fail 'could not record the WSL installation revision'
  ok "recorded WSL installation compatibility $(otherhost_compatibility_version "$ROOT_DIR")"
fi

if [ -x "$PAIRING_HELPER" ]; then
  ok "pairing helper is available: $PAIRING_HELPER"
else
  warn 'pairing helper is not installed; run with --apply after publishing the configured release'
fi
if systemctl --user is-active --quiet otherhost-sshd.service; then
  ok "SSH is listening as $CURRENT_USER on port $SSH_PORT"
else
  warn 'rootless SSH service is not active'
fi
if [ "$(loginctl show-user "$CURRENT_USER" -p Linger --value 2>/dev/null || true)" != true ]; then
  warn 'the user service starts when this WSL user session starts; Windows startup integration can be added later'
fi

printf '\nNext:\n'
printf '  1. In WSL, run: ./scripts/pair-wsl.sh\n'
printf '  2. On the Mac, run: otherhost pair\n'
