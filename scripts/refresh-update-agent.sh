#!/usr/bin/env bash
set -Eeuo pipefail
service=${1:-}
if [[ ! $service =~ ^link-bot-updater-[0-9a-f]{12}\.service$ ]]; then exit 2; fi
if ! systemctl is-active --quiet "$service"; then
  systemctl start "$service"
  exit 0
fi

# Do not kill an update running inside this very service, including older
# agents which did not pass any environment marker to update.sh.
main_pid=$(systemctl show --property MainPID --value "$service")
ancestor=$$
while [[ $ancestor =~ ^[0-9]+$ ]] && ((ancestor > 1)); do
  if [[ $ancestor == "$main_pid" ]]; then exit 0; fi
  ancestor=$(ps -o ppid= -p "$ancestor" | tr -d ' ')
done
# A manual update must activate the new agent, not leave the old code running.
systemctl restart "$service"
