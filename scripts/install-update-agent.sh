#!/usr/bin/env bash
set -Eeuo pipefail

# Called by update.sh after building the new image, before replacing the bot.
# This service survives bot/container restarts and accepts no shell arguments
# from the web API. All credentials and the host checkout stay on the host.
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
if [[ ${EUID} != 0 ]] || ! command -v systemctl >/dev/null || [[ ! -d /run/systemd/system ]]; then
  printf 'Обновление из Mini App: для установки сервиса нужен root и systemd. Обычное обновление продолжается.\n'
  mkdir -p -- "$repo_root/.update-agent"
  exit 0
fi
if [[ $repo_root == *$'\n'* || $repo_root == *$'\r'* ]]; then
  printf 'Unsupported checkout path for updater service.\n' >&2
  exit 1
fi

agent_dir="$repo_root/.update-agent"
install -d -m 0755 -- "$agent_dir"
chown root:root "$agent_dir"
if [[ ! -f "$agent_dir/token" ]]; then
  umask 077
  od -An -N32 -tx1 /dev/urandom | tr -d ' \n' > "$agent_dir/token"
fi
chown 1000:1000 "$agent_dir/token"
chmod 0400 "$agent_dir/token"

instance=$(printf '%s' "$repo_root" | sha256sum | cut -c1-12)
service="link-bot-updater-${instance}.service"
binary="/usr/local/libexec/link-bot-updater-${instance}"
install -d -m 0755 /usr/local/libexec
temporary_container=$(docker create --entrypoint /app/update-agent link-bot:local)
cleanup() { docker rm "$temporary_container" >/dev/null 2>&1 || true; rm -f -- "${binary}.new"; }
trap cleanup EXIT
docker cp "${temporary_container}:/app/update-agent" "${binary}.new"
chmod 0755 "${binary}.new"
chown root:root "${binary}.new"
mv -f -- "${binary}.new" "$binary"

# systemd unit arguments are quoted; % is an instance specifier in unit files.
escape_unit() { local value=${1//\\/\\\\}; value=${value//\"/\\\"}; printf '%s' "${value//%/%%}"; }
unit="/etc/systemd/system/$service"
cat > "$unit" <<EOF
[Unit]
Description=Link-Bot update service (${instance})
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
ExecStart="$binary" --repo="$(escape_unit "$repo_root")" --dir="$(escape_unit "$agent_dir")"
Restart=on-failure
RestartSec=5
UMask=0077
Nice=10
IOSchedulingClass=best-effort
IOSchedulingPriority=7

[Install]
WantedBy=multi-user.target
EOF
chmod 0644 "$unit"
systemctl daemon-reload
systemctl enable "$service" >/dev/null
bash "$repo_root/scripts/refresh-update-agent.sh" "$service"
printf 'Обновление из Mini App подключено: %s\n' "$service"
