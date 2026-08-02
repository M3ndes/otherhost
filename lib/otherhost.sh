#!/usr/bin/env bash

# Shared, Bash 3.2-compatible helpers. Configuration files are parsed as data;
# they are never sourced or evaluated.

otherhost_trim() {
  printf '%s' "$1" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//'
}

otherhost_config_get() {
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

otherhost_migrate_config() {
  local source_file=$1
  local destination_file=$2
  local destination_directory
  local temporary_file

  [ -f "$source_file" ] || return 1
  destination_directory=$(dirname -- "$destination_file")
  mkdir -p "$destination_directory" || return 1
  temporary_file=$(mktemp "$destination_directory/.otherhost-config.XXXXXX") || return 1
  if ! awk '
    {
      sub(/\r$/, "")
      lines[NR] = $0
      candidate = $0
      sub(/^[[:space:]]+/, "", candidate)
      separator = index(candidate, "=")
      if (separator > 0) {
        key = substr(candidate, 1, separator - 1)
        gsub(/[[:space:]]+$/, "", key)
        if (key == "otherhost_name") has_otherhost_name = 1
      }
    }
    END {
      for (number = 1; number <= NR; number++) {
        candidate = lines[number]
        sub(/^[[:space:]]+/, "", candidate)
        separator = index(candidate, "=")
        key = separator > 0 ? substr(candidate, 1, separator - 1) : ""
        gsub(/[[:space:]]+$/, "", key)
        if (key == "devbox_name") {
          if (!has_otherhost_name) {
            value = substr(candidate, separator + 1)
            print "otherhost_name=" value
            has_otherhost_name = 1
          }
        } else {
          print lines[number]
        }
      }
    }
  ' "$source_file" > "$temporary_file"; then
    rm -f "$temporary_file"
    return 1
  fi
  chmod 600 "$temporary_file" || { rm -f "$temporary_file"; return 1; }
  mv "$temporary_file" "$destination_file"
}

otherhost_require_config() {
  local file=$1
  if [ ! -f "$file" ]; then
    printf 'Configuration not found: %s\n' "$file" >&2
    printf 'Copy config/otherhost.example.conf to otherhost.local.conf first.\n' >&2
    return 1
  fi
}

otherhost_is_positive_integer() {
  case "$1" in
    ''|*[!0-9]*) return 1 ;;
    *) [ "$1" -gt 0 ] ;;
  esac
}

otherhost_resolve_identity_file() {
  local configured=$1
  case "$configured" in
    /*) printf '%s\n' "$configured" ;;
    *) printf '%s/%s\n' "$HOME" "$configured" ;;
  esac
}

otherhost_validate_public_key() {
  local public_key=$1
  local key_file
  local line_count

  case "$public_key" in
    ssh-ed25519\ *|ssh-rsa\ *|ecdsa-sha2-*\ *) ;;
    *) return 1 ;;
  esac

  key_file=$(mktemp) || return 1
  printf '%s\n' "$public_key" > "$key_file"
  line_count=$(wc -l < "$key_file" | tr -d '[:space:]')
  if [ "$line_count" != 1 ] || ! ssh-keygen -lf "$key_file" >/dev/null 2>&1; then
    rm -f "$key_file"
    return 1
  fi
  rm -f "$key_file"
}

otherhost_authorize_public_key() {
  local public_key=$1
  local authorized_keys=$2

  otherhost_validate_public_key "$public_key" || return 1
  grep -Fqx "$public_key" "$authorized_keys" || printf '%s\n' "$public_key" >> "$authorized_keys"
}

otherhost_assert_sshd_policy() {
  local effective_config=$1
  local ssh_port=$2
  local ssh_user=$3
  local expected

  for expected in \
    "port $ssh_port" \
    'pubkeyauthentication yes' \
    'passwordauthentication no' \
    'kbdinteractiveauthentication no' \
    'permitrootlogin no' \
    "allowusers $ssh_user"
  do
    printf '%s\n' "$effective_config" | grep -Fqx "$expected" || {
      printf 'Effective sshd policy mismatch: expected %s\n' "$expected" >&2
      return 1
    }
  done
}
