#!/bin/sh
set -eu

cp /etc/caddy/Caddyfile /tmp/link-bot.Caddyfile

if [ -n "${CABINET_SUBDOMAIN:-}" ]; then
  case "$CABINET_SUBDOMAIN" in
    -*|*-|*[!a-zA-Z0-9-]*) echo 'CABINET_SUBDOMAIN must be a single DNS label' >&2; exit 1 ;;
  esac
  if [ "${#CABINET_SUBDOMAIN}" -gt 63 ] || [ -z "${PUBLIC_HOST:-}" ]; then
    echo 'CABINET_SUBDOMAIN requires a DNS label and PUBLIC_HOST' >&2
    exit 1
  fi
  printf '\n%s.%s {\n encode zstd gzip\n reverse_proxy bot:8080\n}\n' "$CABINET_SUBDOMAIN" "$PUBLIC_HOST" >> /tmp/link-bot.Caddyfile
fi

exec caddy run --config /tmp/link-bot.Caddyfile --adapter caddyfile
