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

if [ -n "${ADMIN_SUBDOMAIN:-}" ]; then
  case "$ADMIN_SUBDOMAIN" in
    -*|*-|*[!a-zA-Z0-9-]*) echo 'ADMIN_SUBDOMAIN must be a single DNS label' >&2; exit 1 ;;
  esac
  if [ "${#ADMIN_SUBDOMAIN}" -gt 63 ] || [ -z "${PUBLIC_HOST:-}" ]; then
    echo 'ADMIN_SUBDOMAIN requires a DNS label and PUBLIC_HOST' >&2
    exit 1
  fi
  admin_label=$(printf '%s' "$ADMIN_SUBDOMAIN" | tr '[:upper:]' '[:lower:]')
  cabinet_label=$(printf '%s' "${CABINET_SUBDOMAIN:-}" | tr '[:upper:]' '[:lower:]')
  if [ "$admin_label" = "$cabinet_label" ]; then
    echo 'ADMIN_SUBDOMAIN and CABINET_SUBDOMAIN must be different' >&2; exit 1
  fi
  printf '\n%s.%s {\n encode zstd gzip\n reverse_proxy bot:8080\n}\n' "$admin_label" "$PUBLIC_HOST" >> /tmp/link-bot.Caddyfile
fi

exec caddy run --config /tmp/link-bot.Caddyfile --adapter caddyfile
