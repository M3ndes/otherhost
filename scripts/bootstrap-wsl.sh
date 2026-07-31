#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/devbox.sh
. "$ROOT_DIR/lib/devbox.sh"

MODE=check
CONFIG_FILE=${DEVBOX_CONFIG:-"$ROOT_DIR/devbox.local.conf"}

usage() {
  cat <<'EOF'
Usage: scripts/bootstrap-wsl.sh [--apply] [--config PATH]

The default mode is read-only. --apply installs the SSH server, imports the
configured GitHub user's public keys, hardens SSH, and optionally clones a
project. Docker is intentionally supplied by Docker Desktop's WSL integration.
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
    sudo cat /etc/wsl.conf > "$source_file"
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
    backup_file="/etc/wsl.conf.$(date +%Y%m%d-%H%M%S).devbox-bridge.bak"
    sudo cp /etc/wsl.conf "$backup_file"
    ok "backed up existing WSL config: $backup_file"
  fi
  sudo install -m 644 "$merged_file" /etc/wsl.conf
  rm -f "$source_file" "$merged_file"
}

grep -qi microsoft /proc/version 2>/dev/null || fail 'this bootstrap must run inside WSL'
devbox_require_config "$CONFIG_FILE" || exit 1

SSH_PORT=$(devbox_config_get ssh_port "$CONFIG_FILE")
GITHUB_USER=$(devbox_config_get github_user "$CONFIG_FILE")
PROJECT_REPOSITORY=$(devbox_config_get project_repository "$CONFIG_FILE")
PROJECT_DIRECTORY=$(devbox_config_get project_directory "$CONFIG_FILE")

if ! devbox_is_positive_integer "$SSH_PORT" || [ "$SSH_PORT" -gt 65535 ]; then
  fail 'invalid ssh_port'
fi
case "$GITHUB_USER" in *[!A-Za-z0-9-]*) fail 'github_user contains unsupported characters' ;; esac
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
  [ -n "$GITHUB_USER" ] || fail 'set github_user before applying the WSL bootstrap'

  TEMP_KEYS=$(mktemp)
  trap 'rm -f "$TEMP_KEYS"' EXIT HUP INT TERM
  curl -fsSL "https://github.com/$GITHUB_USER.keys" > "$TEMP_KEYS"
  grep -Eq '^ssh-(ed25519|rsa|ecdsa-sha2-)' "$TEMP_KEYS" || fail "GitHub returned no usable SSH keys for $GITHUB_USER"

  mkdir -p "$HOME/.ssh"
  chmod 700 "$HOME/.ssh"
  touch "$HOME/.ssh/authorized_keys"
  chmod 600 "$HOME/.ssh/authorized_keys"
  while IFS= read -r public_key; do
    case "$public_key" in
      ssh-ed25519\ *|ssh-rsa\ *|ecdsa-sha2-*\ *)
        grep -Fqx "$public_key" "$HOME/.ssh/authorized_keys" || printf '%s\n' "$public_key" >> "$HOME/.ssh/authorized_keys"
        ;;
    esac
  done < "$TEMP_KEYS"
  ok "installed public SSH keys published by github.com/$GITHUB_USER"

  SSHD_DROP_IN=$(mktemp)
  cat > "$SSHD_DROP_IN" <<EOF
Port $SSH_PORT
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitRootLogin no
AllowUsers $CURRENT_USER
EOF
  sudo install -m 644 "$SSHD_DROP_IN" /etc/ssh/sshd_config.d/99-devbox-bridge.conf
  rm -f "$SSHD_DROP_IN"
  sudo install -d -m 755 /run/sshd
  sudo sshd -t

  if [ "$(ps -p 1 -o comm= 2>/dev/null)" = systemd ]; then
    sudo systemctl enable --now ssh
    ok "SSH server is enabled on port $SSH_PORT"
  else
    warn 'SSH configuration is ready; restart WSL and rerun to enable the service'
  fi

  if [ -n "$PROJECT_REPOSITORY" ] || [ -n "$PROJECT_DIRECTORY" ]; then
    [ -n "$PROJECT_REPOSITORY" ] && [ -n "$PROJECT_DIRECTORY" ] || fail 'project_repository and project_directory must be set together'
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
fi

printf '\nWSL address candidates:\n'
hostname -I 2>/dev/null || true
printf 'For mirrored networking, the Windows LAN address usually works from the Mac.\n'
