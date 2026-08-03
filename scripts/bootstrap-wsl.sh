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

usage() {
  cat <<'EOF'
Usage: scripts/bootstrap-wsl.sh [--apply] [--config PATH]

The default mode is read-only. --apply installs and hardens the SSH server,
authorizes a selected Mac public key when present, and optionally clones a project.
Docker is intentionally supplied by Docker Desktop's WSL integration.
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

enable_systemd_config() {
  local source_file
  local merged_file
  local backup_file
  source_file=$(mktemp)
  merged_file=$(mktemp)
  if sudo test -f /etc/wsl.conf; then
    sudo cat /etc/wsl.conf | tee "$source_file" >/dev/null
  fi
  awk '
    BEGIN { in_boot = 0; boot_seen = 0; systemd_seen = 0 }
    /^\[[^]]+\][[:space:]]*$/ {
      if (in_boot && !systemd_seen) print "systemd=true"
      in_boot = (tolower($0) ~ /^\[boot\][[:space:]]*$/)
      if (in_boot) boot_seen = 1
      print
      next
    }
    in_boot && /^[[:space:]]*systemd[[:space:]]*=/ {
      print "systemd=true"
      systemd_seen = 1
      next
    }
    { print }
    END {
      if (in_boot && !systemd_seen) print "systemd=true"
      if (!boot_seen) {
        print ""
        print "[boot]"
        print "systemd=true"
      }
    }
  ' "$source_file" > "$merged_file"
  if sudo test -f /etc/wsl.conf; then
    backup_file="/etc/wsl.conf.$(date +%Y%m%d-%H%M%S).otherhost.bak"
    sudo cp /etc/wsl.conf "$backup_file"
    ok "backed up existing WSL config: $backup_file"
  fi
  sudo install -m 644 "$merged_file" /etc/wsl.conf
  rm -f "$source_file" "$merged_file"
}

grep -qi microsoft /proc/version 2>/dev/null || fail 'this bootstrap must run inside WSL'
otherhost_require_config "$CONFIG_FILE" || exit 1

SSH_PORT=$(otherhost_config_get ssh_port "$CONFIG_FILE")
SSH_PUBLIC_KEY=$(otherhost_config_get ssh_public_key "$CONFIG_FILE")
PROJECT_REPOSITORY=$(otherhost_config_get project_repository "$CONFIG_FILE")
PROJECT_DIRECTORY=$(otherhost_config_get project_directory "$CONFIG_FILE")

if ! otherhost_is_positive_integer "$SSH_PORT" || [ "$SSH_PORT" -gt 65535 ]; then
  fail 'invalid ssh_port'
fi
CURRENT_USER=$(id -un)

ok 'WSL environment detected'
if [ "$(ps -p 1 -o comm= 2>/dev/null)" = systemd ]; then
  ok 'systemd is active'
else
  warn 'systemd is not active'
  if [ "$MODE" = apply ]; then
    enable_systemd_config
    warn 'enabled systemd while preserving /etc/wsl.conf; run wsl --shutdown in PowerShell, then rerun this script'
  fi
fi

if [ "$MODE" = apply ]; then
  sudo apt-get update
  sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y ca-certificates curl git openssh-server
  ok 'installed base packages and OpenSSH server'
fi

for command_name in git curl ssh; do
  if command -v "$command_name" >/dev/null 2>&1; then
    ok "$command_name is available"
  else
    warn "$command_name is missing"
  fi
done

if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  ok 'Docker Desktop WSL integration is working'
else
  warn 'Docker is unavailable; enable this distribution under Docker Desktop > Resources > WSL integration'
fi

if [ "$MODE" = apply ]; then
  mkdir -p "$HOME/.ssh"
  chmod 700 "$HOME/.ssh"
  touch "$HOME/.ssh/authorized_keys"
  chmod 600 "$HOME/.ssh/authorized_keys"
  if [ -n "$SSH_PUBLIC_KEY" ]; then
    otherhost_authorize_public_key "$SSH_PUBLIC_KEY" "$HOME/.ssh/authorized_keys" || fail 'ssh_public_key is not a valid supported SSH public key'
    ok 'installed the selected Mac public SSH key'
  else
    ok 'prepared authorized_keys; secure pairing will add the Mac public key'
  fi

  SSHD_DROP_IN=$(mktemp)
  cat > "$SSHD_DROP_IN" <<EOF
Port $SSH_PORT
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
AllowUsers $CURRENT_USER
EOF
  sudo install -m 644 "$SSHD_DROP_IN" /etc/ssh/sshd_config.d/00-otherhost.conf
  sudo rm -f /etc/ssh/sshd_config.d/99-otherhost.conf
  sudo rm -f /etc/ssh/sshd_config.d/00-devbox-bridge.conf /etc/ssh/sshd_config.d/99-devbox-bridge.conf
  rm -f "$SSHD_DROP_IN"
  sudo install -d -m 755 /run/sshd
  sudo sshd -t
  SSHD_EFFECTIVE=$(sudo sshd -T -C "user=$CURRENT_USER,host=localhost,addr=127.0.0.1")
  otherhost_assert_sshd_policy "$SSHD_EFFECTIVE" "$SSH_PORT" "$CURRENT_USER" || fail 'effective sshd policy does not match the required hardened settings'
  ok 'verified the effective SSH authentication policy'

  if [ "$(ps -p 1 -o comm= 2>/dev/null)" = systemd ]; then
    sudo systemctl enable --now ssh
    ok "SSH server is enabled on port $SSH_PORT"
  else
    warn 'SSH configuration is ready; restart WSL and rerun to enable the service'
  fi

  if [ -n "$PROJECT_REPOSITORY" ] || [ -n "$PROJECT_DIRECTORY" ]; then
    if [ -z "$PROJECT_REPOSITORY" ] || [ -z "$PROJECT_DIRECTORY" ]; then
      fail 'project_repository and project_directory must be set together'
    fi
    case "$PROJECT_REPOSITORY" in
      https://*|ssh://*|git@*:*) ;;
      *) fail 'project_repository must use an HTTPS or SSH Git URL' ;;
    esac
    case "$PROJECT_DIRECTORY" in
      /*|\~/*) ;;
      *) fail 'project_directory must be an absolute path or start with ~/' ;;
    esac
    case "$PROJECT_DIRECTORY" in
      /mnt/*) fail 'project_directory must use the WSL Linux filesystem, not /mnt/c' ;;
    esac
    case "$PROJECT_DIRECTORY" in
      \~/*) PROJECT_DIRECTORY="$HOME/${PROJECT_DIRECTORY#\~/}" ;;
    esac
    if [ -e "$PROJECT_DIRECTORY" ]; then
      ok "preserved existing project directory: $PROJECT_DIRECTORY"
    else
      mkdir -p "$(dirname -- "$PROJECT_DIRECTORY")"
      git clone --recurse-submodules -- "$PROJECT_REPOSITORY" "$PROJECT_DIRECTORY"
      ok "cloned project into $PROJECT_DIRECTORY"
    fi
  fi

  otherhost_write_installation_state "$ROOT_DIR" || fail 'could not record the WSL installation revision'
  ok "recorded WSL installation compatibility $(otherhost_compatibility_version "$ROOT_DIR")"
fi

printf '\nWSL address candidates:\n'
hostname -I 2>/dev/null || true
printf 'For mirrored networking, the Windows LAN address usually works from the Mac.\n'
