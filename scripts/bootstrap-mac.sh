#!/usr/bin/env bash
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
# shellcheck source=../lib/devbox.sh
. "$ROOT_DIR/lib/devbox.sh"

MODE=check
GENERATE_KEY=0
CONFIG_FILE=${DEVBOX_CONFIG:-"$ROOT_DIR/devbox.local.conf"}

usage() {
  cat <<'EOF'
Usage: scripts/bootstrap-mac.sh [--apply] [--generate-key] [--config PATH]

The default mode only reports what would be needed. --apply installs a symlink
at ~/.local/bin/devbox and creates the local config when it is missing.
--generate-key also creates the dedicated Ed25519 SSH identity when absent.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --apply) MODE=apply; shift ;;
    --generate-key) GENERATE_KEY=1; shift ;;
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

[ "$(uname -s)" = Darwin ] || fail 'this bootstrap must run on macOS'

for command_name in git ssh ssh-keygen; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
  ok "$command_name is available"
done

if [ "$MODE" = apply ]; then
  mkdir -p "$HOME/.local/bin"
  COMMAND_LINK="$HOME/.local/bin/devbox"
  if [ -L "$COMMAND_LINK" ]; then
    [ "$(readlink "$COMMAND_LINK")" = "$ROOT_DIR/bin/devbox" ] || fail "$COMMAND_LINK is already a different symlink"
    ok "preserved existing command link: $COMMAND_LINK"
  elif [ -e "$COMMAND_LINK" ]; then
    fail "$COMMAND_LINK already exists and will not be overwritten"
  else
    ln -s "$ROOT_DIR/bin/devbox" "$COMMAND_LINK"
    ok "installed $COMMAND_LINK -> $ROOT_DIR/bin/devbox"
  fi

  if [ ! -f "$CONFIG_FILE" ]; then
    mkdir -p "$(dirname -- "$CONFIG_FILE")"
    cp "$ROOT_DIR/config/devbox.example.conf" "$CONFIG_FILE"
    chmod 600 "$CONFIG_FILE"
    ok "created local configuration: $CONFIG_FILE"
  else
    ok "preserved existing configuration: $CONFIG_FILE"
  fi
elif [ -L "$HOME/.local/bin/devbox" ] || [ -x "$HOME/.local/bin/devbox" ]; then
  ok 'devbox command is installed in ~/.local/bin'
else
  warn 'devbox is not installed; run this script with --apply'
fi

if [ -f "$CONFIG_FILE" ]; then
  IDENTITY_CONFIG=$(devbox_config_get identity_file "$CONFIG_FILE")
  [ -n "$IDENTITY_CONFIG" ] || fail "identity_file is missing from $CONFIG_FILE"
  IDENTITY_FILE=$(devbox_resolve_identity_file "$IDENTITY_CONFIG")
else
  IDENTITY_FILE="$HOME/.ssh/devbox_bridge_ed25519"
  warn "local configuration is missing: $CONFIG_FILE"
fi

if [ -f "$IDENTITY_FILE" ]; then
  ok "SSH identity already exists: $IDENTITY_FILE"
elif [ "$MODE" = apply ] && [ "$GENERATE_KEY" -eq 1 ]; then
  mkdir -p "$(dirname -- "$IDENTITY_FILE")"
  chmod 700 "$(dirname -- "$IDENTITY_FILE")"
  ssh-keygen -t ed25519 -a 64 -f "$IDENTITY_FILE" -C 'devbox-bridge client'
  chmod 600 "$IDENTITY_FILE"
  ok "created SSH identity: $IDENTITY_FILE"
else
  warn "SSH identity is missing: $IDENTITY_FILE"
  warn 'run with --apply --generate-key when you are ready to create it'
fi

case ":$PATH:" in
  *:"$HOME/.local/bin":*) ok '~/.local/bin is present in PATH' ;;
  *) warn 'add export PATH="$HOME/.local/bin:$PATH" to ~/.zshrc' ;;
esac

printf '\nNext: edit %s, then run devbox doctor.\n' "$CONFIG_FILE"
