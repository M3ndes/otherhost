#!/usr/bin/env bash
set -eu

PAIRING_VERSION=v0.1.1
DESTINATION=${1:-"$HOME/.local/lib/otherhost/otherhost-pair"}
REPOSITORY=https://github.com/M3ndes/otherhost
LEGACY_REPOSITORY=https://github.com/M3ndes/devbox-bridge

fail() { printf '[fail] %s\n' "$*" >&2; exit 1; }

[ "$(uname -s)" = Darwin ] || fail 'the prebuilt pairing client supports macOS only'
case "$(uname -m)" in
  arm64) ARCHITECTURE=arm64 ;;
  x86_64) ARCHITECTURE=amd64 ;;
  *) fail "unsupported Mac architecture: $(uname -m)" ;;
esac

for command_name in curl shasum; do
  command -v "$command_name" >/dev/null 2>&1 || fail "$command_name is required"
done

ASSET="otherhost-pair-darwin-$ARCHITECTURE"
LEGACY_ASSET="devbox-pair-darwin-$ARCHITECTURE"
BASE_URL="$REPOSITORY/releases/download/$PAIRING_VERSION"
TEMPORARY_DIRECTORY=$(mktemp -d)
trap 'rm -rf "$TEMPORARY_DIRECTORY"' EXIT HUP INT TERM

if ! curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
  "$BASE_URL/$ASSET" --output "$TEMPORARY_DIRECTORY/$ASSET" 2>/dev/null; then
  ASSET=$LEGACY_ASSET
  BASE_URL="$LEGACY_REPOSITORY/releases/download/$PAIRING_VERSION"
  curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
    "$BASE_URL/$ASSET" --output "$TEMPORARY_DIRECTORY/$ASSET"
fi
curl --proto '=https' --tlsv1.2 --fail --silent --show-error --location \
  "$BASE_URL/checksums.txt" --output "$TEMPORARY_DIRECTORY/checksums.txt"

EXPECTED=$(awk -v asset="$ASSET" '$2 == asset { print; exit }' "$TEMPORARY_DIRECTORY/checksums.txt")
[ -n "$EXPECTED" ] || fail "the release checksum is missing for $ASSET"
printf '%s\n' "$EXPECTED" > "$TEMPORARY_DIRECTORY/expected.txt"
(cd "$TEMPORARY_DIRECTORY" && shasum -a 256 -c expected.txt >/dev/null) || fail 'the pairing helper checksum did not match'

mkdir -p "$(dirname -- "$DESTINATION")"
chmod 700 "$(dirname -- "$DESTINATION")"
install -m 700 "$TEMPORARY_DIRECTORY/$ASSET" "$DESTINATION"
printf '[ok] installed secure pairing helper: %s\n' "$DESTINATION"
