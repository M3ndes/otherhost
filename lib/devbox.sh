#!/usr/bin/env bash

# Shared, Bash 3.2-compatible helpers. Configuration files are parsed as data;
# they are never sourced or evaluated.

devbox_trim() {
  printf '%s' "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

devbox_config_get() {
  local key=$1
  local file=$2

  awk -v wanted="$key" '
    {
      sub(/\r$/, "")
      line = $0
      sub(/^[[:space:]]+/, "", line)
      if (line == "" || substr(line, 1, 1) == "#") next

      separator = index(line, "=")
      if (separator == 0) next

      current = substr(line, 1, separator - 1)
      value = substr(line, separator + 1)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", current)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)

      if (current == wanted) {
        print value
        exit
      }
    }
  ' "$file"
}

devbox_require_config() {
  local file=$1
  if [ ! -f "$file" ]; then
    printf 'Configuration not found: %s\n' "$file" >&2
    printf 'Copy config/devbox.example.conf to devbox.local.conf first.\n' >&2
    return 1
  fi
}

devbox_is_positive_integer() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) [ "$1" -gt 0 ] ;;
  esac
}

devbox_resolve_identity_file() {
  local configured=$1
  case "$configured" in
    /*) printf '%s\n' "$configured" ;;
    *) printf '%s/%s\n' "$HOME" "$configured" ;;
  esac
}
