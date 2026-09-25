#!/usr/bin/env bash
set -Eeuo pipefail

if (($# != 3)); then
  printf 'Usage: bash ensure-caddy-host.sh CADDYFILE PUBLIC_HOST CABINET_HOST\n' >&2
  exit 2
fi

caddyfile=$1
public_host=$2
cabinet_host=$3
if [[ ! -f "$caddyfile" ]]; then
  printf 'Caddyfile not found: %s\n' "$caddyfile" >&2
  exit 1
fi

temp_file=$(mktemp "${caddyfile}.link-bot.XXXXXX")
trap 'rm -f -- "$temp_file"' EXIT

# Add the cabinet name to the existing site block. This keeps its upstream,
# headers, and all other sites intact, even when Caddy runs outside Compose.
if ! awk -v public_host="$public_host" -v cabinet_host="$cabinet_host" '
  function has_address(header, host, count, addresses, i) {
    sub(/\{[[:space:]]*$/, "", header)
    gsub(/,/, " ", header)
    count = split(header, addresses, /[[:space:]]+/)
    for (i = 1; i <= count; i++) {
      if (addresses[i] == host || addresses[i] == "https://" host ||
          addresses[i] == host ":443" || addresses[i] == "https://" host ":443") return 1
    }
    return 0
  }
  {
    lines[NR] = $0
    header = $0
    sub(/[[:space:]]*#.*/, "", header)
    if (header !~ /\{[[:space:]]*$/) next
    if (has_address(header, public_host)) {
      public_count++
      public_line = NR
    }
    if (has_address(header, cabinet_host)) cabinet_count++
  }
  END {
    if (cabinet_count > 0) {
      for (i = 1; i <= NR; i++) print lines[i]
      exit 0
    }
    if (public_count != 1) {
      print "Could not identify exactly one public site in the shared Caddyfile" > "/dev/stderr"
      exit 1
    }
    for (i = 1; i <= NR; i++) {
      line = lines[i]
      if (i == public_line) sub(/[[:space:]]*\{/, ", " cabinet_host " {", line)
      print line
    }
  }
' "$caddyfile" > "$temp_file"; then
  exit 1
fi

if cmp -s "$caddyfile" "$temp_file"; then
  printf 'Cabinet host already present in %s\n' "$caddyfile"
  exit 0
fi

# Overwrite the existing inode: Docker bind mounts a file, so renaming a new
# file over it would leave the running container reading the old inode.
cat "$temp_file" > "$caddyfile"
printf 'Added %s to %s\n' "$cabinet_host" "$caddyfile"
