#!/usr/bin/env bash
set -eu

PAIRING_VERSION=v0.1.1
DESTINATION=${1:-"$HOME/.local/lib/devbox-bridge/devbox-pair"}
REPOSITORY=https://github.com/M3ndes/devbox-bridge

fail() { printf '[fail] %s\n' "$*" >&2; exit 1; }

grep -qi microsoft /proc/version 2>/dev/null || fail 'this installer must run inside WSL'
case "$(uname -m)" in
  x86_64) ARCHITECTURE=amd64 ;;
  arm64|aarch64) ARCHITECTURE=arm64 ;;
  *) fail "unsupported WSL architecture: $(uname -m)" ;;
esac

for command_name in curl sha256sum; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done

ASSET="devbox-pair-linux-$ARCHITECTURE"
BASE_URL="$REPOSITORY/releases/download/$PAIRING_VERSION"
TEMPORARY_DIRECTORY=$(mktemp -d)
trap 'rm -rf "$TEMPORARY_DIRECTORY"' EXIT HUP INT TERM

curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
  "$BASE_URL/$ASSET" --output "$TEMPORARY_DIRECTORY/$ASSET"
curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
  "$BASE_URL/checksums.txt" --output "$TEMPORARY_DIRECTORY/checksums.txt"

EXPECTED=$(awk -v asset="$ASSET" '$2 == asset { print; exit }' "$TEMPORARY_DIRECTORY/checksums.txt")
[ -n "$EXPECTED" ] || fail "the release checksum is missing for $ASSET"
printf '%s\n' "$EXPECTED" > "$TEMPORARY_DIRECTORY/expected.txt"
(cd "$TEMPORARY_DIRECTORY" && sha256sum -c expected.txt >/dev/null) || fail 'the pairing helper checksum did not match'

mkdir -p "$(dirname -- "$DESTINATION")"
chmod 700 "$(dirname -- "$DESTINATION")"
install -m 700 "$TEMPORARY_DIRECTORY/$ASSET" "$DESTINATION"
printf '[ok] installed %s\n' "$DESTINATION"
