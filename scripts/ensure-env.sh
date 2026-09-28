#!/usr/bin/env bash
set -Eeuo pipefail

env_file=${1:-.env}
template_file=${2:-$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/.env.example}

if [[ ! -f $env_file ]]; then
  printf 'Environment file not found: %s\n' "$env_file" >&2
  exit 1
fi
if [[ ! -f $template_file ]]; then
  printf 'Environment template not found: %s\n' "$template_file" >&2
  exit 1
fi

# Git ignores .env. Add every newly introduced setting from the tracked
# template without replacing any value already configured by the operator.
declare -A existing=()
while IFS= read -r line || [[ -n $line ]]; do
  line=${line%$'\r'}
  if [[ $line =~ ^[[:space:]]*([a-zA-Z_][a-zA-Z0-9_]*)[[:space:]]*= ]]; then
    existing[${BASH_REMATCH[1]}]=1
  fi
done < "$env_file"

missing=()
while IFS= read -r line || [[ -n $line ]]; do
  line=${line%$'\r'}
  if [[ $line =~ ^([a-zA-Z_][a-zA-Z0-9_]*)=(.*)$ ]]; then
    key=${BASH_REMATCH[1]}
    value=${BASH_REMATCH[2]}
    if [[ -z ${existing[$key]+x} ]]; then
      # Example credentials and domains must never become real defaults.
      case $key in
        TELEGRAM_TOKEN|ADMIN_TELEGRAM_ID|REMNAWAVE_URL|REMNAWAVE_TOKEN|POSTGRES_PASSWORD|PUBLIC_HOST|PUBLIC_BASE_URL)
          value=
          ;;
      esac
      missing+=("$key=$value")
      existing[$key]=1
    fi
  fi
done < "$template_file"

if ((${#missing[@]} == 0)); then
  exit 0
fi

if [[ -s $env_file && -n $(tail -c 1 -- "$env_file") ]]; then
  printf '\n' >> "$env_file"
fi
printf '\n# Added automatically by update.sh; set optional integrations when needed.\n' >> "$env_file"
printf '%s\n' "${missing[@]}" >> "$env_file"
printf 'Added %d missing environment setting(s) to %s\n' "${#missing[@]}" "$env_file"
