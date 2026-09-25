#!/usr/bin/env bash
set -Eeuo pipefail

if (($# > 1)) || { (($# == 1)) && [[ $1 != --standalone ]]; }; then
  printf 'Usage: bash update.sh [--standalone]\n' >&2
  exit 2
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_root"

bash ./scripts/ensure-env.sh .env

read_env_value() {
  local key=$1 line value=""
  while IFS= read -r line || [[ -n $line ]]; do
    line=${line%$'\r'}
    if [[ $line =~ ^[[:space:]]*${key}[[:space:]]*=(.*)$ ]]; then
      value=${BASH_REMATCH[1]}
    fi
  done < .env
  value=${value%%#*}
  value=${value#"${value%%[![:space:]]*}"}
  value=${value%"${value##*[![:space:]]}"}
  if [[ $value == \"*\" || $value == \'*\' ]]; then
    value=${value:1:${#value}-2}
  fi
  printf '%s' "$value"
}

public_host=$(read_env_value PUBLIC_HOST)
cabinet_subdomain=$(read_env_value CABINET_SUBDOMAIN)
if [[ -n $cabinet_subdomain ]] && {
  [[ ! $cabinet_subdomain =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$ ]] ||
  ((${#cabinet_subdomain} > 63)) ||
  [[ ! $public_host =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ ]];
}; then
  printf 'Invalid CABINET_SUBDOMAIN or PUBLIC_HOST in .env\n' >&2
  exit 1
fi

# The bundled Caddy and a shared Caddy can use the same container name. The
# Caddyfile mount identifies which one is actually serving ports 80 and 443.
caddy_container_exists=false
caddyfile_mount=""
if docker inspect link-bot-caddy >/dev/null 2>&1; then
  caddy_container_exists=true
  caddyfile_mount=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/etc/caddy/Caddyfile"}}{{.Source}}{{end}}{{if eq .Destination "/etc/caddy"}}{{.Source}}/Caddyfile{{end}}{{end}}' link-bot-caddy)
fi

mode=bot
if [[ -n $caddyfile_mount && $(readlink -f -- "$caddyfile_mount") == $(readlink -f -- ./Caddyfile) ]]; then
  mode=standalone
elif [[ -n $caddyfile_mount ]]; then
  mode=shared
elif ! "$caddy_container_exists" && (($# == 1)); then
  mode=standalone
elif [[ -n $cabinet_subdomain ]]; then
  printf 'Cannot locate the active Caddyfile; cabinet HTTPS cannot be configured automatically.\n' >&2
  exit 1
fi

if [[ $mode == shared && -n $cabinet_subdomain ]]; then
  printf 'Shared Caddy detected; updating its existing site instead of starting bundled Caddy.\n'
  if [[ ! -f $caddyfile_mount || ! -w $caddyfile_mount ||
        $(docker inspect --format '{{.State.Running}}' link-bot-caddy) != true ]]; then
    printf 'The shared Caddyfile is unavailable or its container is stopped.\n' >&2
    exit 1
  fi

  cabinet_host="${cabinet_subdomain}.${public_host}"
  backup=$(mktemp "${caddyfile_mount}.link-bot-backup.XXXXXX")
  cp -p -- "$caddyfile_mount" "$backup"

  restore_caddyfile() {
    cat "$backup" > "$caddyfile_mount"
    docker exec link-bot-caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile >/dev/null 2>&1 || true
  }

  if ! bash ./scripts/ensure-caddy-host.sh "$caddyfile_mount" "$public_host" "$cabinet_host"; then
    restore_caddyfile
    printf 'Shared Caddyfile was not changed.\n' >&2
    exit 1
  fi
  if ! docker exec link-bot-caddy caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile ||
     ! docker exec link-bot-caddy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile; then
    restore_caddyfile
    printf 'Caddy rejected the updated config; restored backup %s\n' "$backup" >&2
    exit 1
  fi
  if cmp -s "$backup" "$caddyfile_mount"; then
    rm -f -- "$backup"
  else
    printf 'Shared Caddyfile backup: %s\n' "$backup"
  fi
fi

if [[ $mode == standalone ]]; then
  docker compose --profile standalone up -d --build
else
  docker compose up -d --build bot
fi

if [[ -n $cabinet_subdomain ]]; then
  cabinet_host="${cabinet_subdomain}.${public_host}"
  if ! command -v curl >/dev/null 2>&1; then
    printf 'curl is required to verify the cabinet HTTPS endpoint.\n' >&2
    exit 1
  fi
  for ((attempt = 1; attempt <= 12; attempt++)); do
    if curl --noproxy '*' --silent --fail --output /dev/null --max-time 4 \
      --resolve "${cabinet_host}:443:127.0.0.1" "https://${cabinet_host}/mini-app/"; then
      printf 'Cabinet HTTPS is ready: https://%s/mini-app/\n' "$cabinet_host"
      exit 0
    fi
    if ((attempt < 12)); then sleep 5; fi
  done
  printf 'Cabinet HTTPS is not ready at https://%s/mini-app/. Check docker logs link-bot-caddy for certificate errors.\n' "$cabinet_host" >&2
  exit 1
fi
