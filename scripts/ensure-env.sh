#!/usr/bin/env bash
set -Eeuo pipefail

env_file=${1:-.env}
if [[ ! -f "$env_file" ]]; then
  printf 'Environment file not found: %s\n' "$env_file" >&2
  exit 1
fi

# An existing .env is ignored by Git, so new optional settings need to be
# added during an update. Never overwrite a value the operator already set.
if grep -Eq '^[[:space:]]*CABINET_SUBDOMAIN[[:space:]]*=' "$env_file"; then
  exit 0
fi

if [[ -s "$env_file" && -n $(tail -c 1 "$env_file") ]]; then
  printf '\n' >> "$env_file"
fi
printf 'CABINET_SUBDOMAIN=\n' >> "$env_file"
printf 'Added CABINET_SUBDOMAIN= to %s\n' "$env_file"
