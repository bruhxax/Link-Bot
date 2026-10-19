#!/usr/bin/env bash
set -Eeuo pipefail

# update.sh holds the update lock. Resolve only the two deployment configs;
# source edits and staged changes require a deliberate manual review.
repo_root=$(git rev-parse --show-toplevel)
cd "$repo_root"
if ! git diff --cached --quiet || [[ -n $(git ls-files --unmerged) ]]; then
  printf 'Обновление остановлено: в Git есть подготовленные изменения или неразрешённые конфликты.\n' >&2
  exit 20
fi
if [[ -n $(git status --porcelain --untracked-files=no -- . ':(exclude)docker-compose.yaml' ':(exclude)Caddyfile') ]]; then
  printf 'Обновление остановлено: изменены исходники. Локальные настройки docker-compose.yaml и Caddyfile разрешены.\n' >&2
  exit 20
fi
git fetch --tags --force origin
before=$(git rev-parse HEAD)
target=$(git rev-parse --verify '@{upstream}^{commit}')
if ! git merge-base --is-ancestor "$before" "$target"; then
  printf 'Обновление остановлено: локальная ветка расходится с удалённой.\n' >&2
  exit 20
fi

changed=()
for file in docker-compose.yaml Caddyfile; do
  if ! git diff --quiet HEAD -- "$file"; then
    if [[ ! -f $file || -L $file ]]; then
      printf 'Обновление остановлено: %s должен быть обычным файлом.\n' "$file" >&2
      exit 20
    fi
    changed+=("$file")
  fi
done
if ((${#changed[@]} == 0)); then
  git merge --ff-only "$target"
  exit 0
fi

umask 077
backup_root=$(git rev-parse --git-path link-bot-update-backups)
mkdir -p -- "$backup_root"
backup=$(mktemp -d "$backup_root/$(date -u +%Y%m%dT%H%M%SZ).XXXXXX")
printf 'Резервная копия локальных настроек: %s\n' "$backup"
restore_needed=false
finish() {
  local result=$?
  if "$restore_needed"; then
    for file in "${changed[@]}"; do
      cat "$backup/$file.local" > "$file" || printf 'Не удалось вернуть %s. Копия: %s\n' "$file" "$backup" >&2
    done
  fi
  exit "$result"
}
trap finish EXIT

# Resolve all files before modifying the checkout. Conflicted merge results and
# validation output stay in the private backup, never in the live configuration.
for file in "${changed[@]}"; do cp -p -- "$file" "$backup/$file.local"; done
for file in "${changed[@]}"; do
  git show "$before:$file" > "$backup/$file.base"
  if ! git show "$target:$file" > "$backup/$file.incoming"; then
    printf 'Обновление остановлено: %s удалён из новой версии.\n' "$file" >&2
    exit 21
  fi
  if ! git merge-file -p "$backup/$file.local" "$backup/$file.base" "$backup/$file.incoming" > "$backup/$file.merged"; then
    printf 'Конфликт настроек %s. Рабочие файлы не изменены; сравните версии в %s.\n' "$file" "$backup" >&2
    exit 21
  fi
done

compose="$backup/docker-compose.yaml.merged"
if [[ ! -f $compose ]]; then
  git show "$target:docker-compose.yaml" > "$backup/docker-compose.yaml.incoming"
  compose="$backup/docker-compose.yaml.incoming"
fi
if ! docker compose --project-directory "$repo_root" -f "$compose" config --quiet > "$backup/compose-check.log" 2>&1; then
  printf 'Проверка объединённого Compose не прошла. Рабочие файлы не изменены. Журнал: %s/compose-check.log\n' "$backup" >&2
  exit 22
fi
if [[ -f $backup/Caddyfile.merged ]] && [[ $(docker inspect --format '{{.State.Running}}' link-bot-caddy 2>/dev/null || true) == true ]]; then
  if ! docker exec -i -w /etc/caddy link-bot-caddy caddy adapt --adapter caddyfile --config - < "$backup/Caddyfile.merged" > "$backup/caddy-check.log" 2>&1; then
    printf 'Проверка объединённого Caddyfile не прошла. Рабочие файлы не изменены. Журнал: %s/caddy-check.log\n' "$backup" >&2
    exit 22
  fi
fi

for file in "${changed[@]}"; do
  if ! cmp -s "$file" "$backup/$file.local"; then
    printf '%s изменён во время проверки. Обновление остановлено.\n' "$file" >&2
    exit 23
  fi
done
if [[ $(git rev-parse HEAD) != "$before" ]]; then
  printf 'Ветка изменилась во время проверки. Обновление остановлено.\n' >&2
  exit 23
fi
restore_needed=true
for file in "${changed[@]}"; do cat "$backup/$file.base" > "$file"; done
git merge --ff-only "$target"
for file in "${changed[@]}"; do cat "$backup/$file.merged" > "$file"; done
restore_needed=false
printf 'Локальные настройки сохранены и объединены с новой версией.\n'
