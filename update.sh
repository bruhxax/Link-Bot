#!/usr/bin/env bash
set -Eeuo pipefail

if (($# > 1)) || { (($# == 1)) && [[ $1 != --standalone ]]; }; then
  printf 'Usage: bash update.sh [--standalone]\n' >&2
  exit 2
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_root"

# Re-execute the freshly pulled script so this remains the only command needed
# for both a code update and a restart after editing .env.
if [[ ${LINK_BOT_UPDATE_REEXEC:-} != 1 ]]; then
  git pull --ff-only
  exec env LINK_BOT_UPDATE_REEXEC=1 bash "$repo_root/update.sh" "$@"
fi

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
public_base_url=$(read_env_value PUBLIC_BASE_URL)
cabinet_subdomain=$(read_env_value CABINET_SUBDOMAIN)
if [[ ! $public_host =~ ^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?$ ]] ||
   { [[ -n $cabinet_subdomain ]] && {
     [[ ! $cabinet_subdomain =~ ^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$ ]] ||
     ((${#cabinet_subdomain} > 63));
   }; }; then
  printf 'Invalid CABINET_SUBDOMAIN or PUBLIC_HOST in .env\n' >&2
  exit 1
fi
if [[ $public_base_url != "https://${public_host}" ]]; then
  printf 'PUBLIC_BASE_URL must be https://%s when PUBLIC_HOST is %s. Update both values in .env.\n' "$public_host" "$public_host" >&2
  exit 1
fi

# A Caddy container can have the same name and Caddyfile mount while belonging
# to a different Compose project. Recreate it with its actual project name.
caddy_container_exists=false
caddyfile_mount=""
caddy_project=""
caddy_service=""
if docker inspect link-bot-caddy >/dev/null 2>&1; then
  caddy_container_exists=true
  caddyfile_mount=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/etc/caddy/Caddyfile"}}{{.Source}}{{end}}{{if eq .Destination "/etc/caddy"}}{{.Source}}/Caddyfile{{end}}{{end}}' link-bot-caddy)
  caddy_project=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' link-bot-caddy 2>/dev/null || true)
  caddy_service=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.service"}}' link-bot-caddy 2>/dev/null || true)
fi

mode=bot
if [[ -n $caddyfile_mount &&
      $(readlink -f -- "$caddyfile_mount") == $(readlink -f -- ./Caddyfile) &&
      $caddy_service == caddy && -n $caddy_project &&
      $caddy_project != '<no value>' && $caddy_project != '<nil>' ]]; then
  mode=managed
elif [[ -n $caddyfile_mount ]]; then
  mode=shared
elif ! "$caddy_container_exists"; then
  mode=new
else
  printf 'Cannot locate the active Caddyfile; refusing to start a second Caddy.\n' >&2
  exit 1
fi

if [[ $mode == managed || $mode == new ]] &&
   ! grep -Fq '{$PUBLIC_HOST' ./Caddyfile &&
   ! grep -Fq "$public_host" ./Caddyfile; then
  printf 'Bundled Caddyfile does not include PUBLIC_HOST or %s; update its site address before restarting.\n' "$public_host" >&2
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

docker compose up -d db
docker compose up -d --build --force-recreate --no-deps bot

if [[ $mode == managed ]]; then
  docker compose -p "$caddy_project" --profile standalone up -d --force-recreate --no-deps caddy
elif [[ $mode == new ]]; then
  docker compose --profile standalone up -d --no-deps caddy
fi

if ! command -v curl >/dev/null 2>&1; then
  printf 'curl is required to verify the HTTPS endpoints.\n' >&2
  exit 1
fi

verify_https() {
  local host=$1 path=$2 label=$3
  for ((attempt = 1; attempt <= 6; attempt++)); do
    if curl --noproxy '*' --silent --fail --output /dev/null --max-time 4 \
      --resolve "${host}:443:127.0.0.1" "https://${host}${path}"; then
      printf '%s HTTPS is ready: https://%s%s\n' "$label" "$host" "$path"
      return 0
    fi
    if ((attempt < 6)); then sleep 3; fi
  done
  printf '%s HTTPS is not ready at https://%s%s. Check docker logs link-bot-caddy for certificate errors.\n' "$label" "$host" "$path" >&2
  return 1
}

verify_https "$public_host" / Landing
if [[ -n $cabinet_subdomain ]]; then
  verify_https "${cabinet_subdomain}.${public_host}" /mini-app/ Cabinet
fi
