#!/usr/bin/env bash

set -Eeuo pipefail

readonly RESET='\033[0m'
readonly DIM='\033[2m'
readonly RED='\033[31m'
readonly GREEN='\033[32m'
readonly CYAN='\033[36m'
readonly WHITE='\033[97m'

repo_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd "$repo_dir"

temporary_dir="$(mktemp -d)"
cleanup() {
	case "$temporary_dir" in
	/tmp/*) rm -rf -- "$temporary_dir" ;;
	esac
}
trap cleanup EXIT

fail() {
	printf "\n${RED}✕ %s${RESET}\n" "$1" >&2
	exit 1
}

run_quiet() {
	local label="$1"
	local log_file="$2"
	shift 2
	local frames=('⠋' '⠙' '⠹' '⠸' '⠼' '⠴' '⠦' '⠧' '⠇' '⠏')
	local frame=0
	local started=$SECONDS

	"$@" >"$log_file" 2>&1 &
	local command_pid=$!
	while kill -0 "$command_pid" 2>/dev/null; do
		printf "\r\033[2K  ${CYAN}%s${RESET} %s" "${frames[$frame]}" "$label"
		frame=$(((frame + 1) % ${#frames[@]}))
		sleep 0.08
	done

	local exit_code=0
	wait "$command_pid" || exit_code=$?
	if ((exit_code != 0)); then
		printf "\r\033[2K  ${RED}✕${RESET} %s\n" "$label"
		tail -n 40 "$log_file" >&2
		exit "$exit_code"
	fi
	printf "\r\033[2K  ${GREEN}✓${RESET} %-34s ${DIM}%ss${RESET}\n" "$label" "$((SECONDS - started))"
}

[[ -d .git ]] || fail "Запустите update.sh из каталога Link-Bot"
[[ "$(git symbolic-ref --short -q HEAD || true)" == "main" ]] || fail "Для обновления переключитесь на ветку main"
git diff --quiet && git diff --cached --quiet || fail "Есть несохранённые изменения — сохраните их перед обновлением"
command -v docker >/dev/null 2>&1 || fail "Docker не установлен"
docker compose version >/dev/null 2>&1 || fail "Docker Compose недоступен"

printf "${WHITE}╭─ Link-Bot · обновление${RESET}\n"
old_revision="$(git rev-parse HEAD)"
run_quiet "Получаю обновление с GitHub" "$temporary_dir/git-fetch.log" \
	env GIT_TERMINAL_PROMPT=0 git \
	-c http.version=HTTP/1.1 \
	-c credential.helper= \
	-c http.extraHeader= \
	fetch --quiet --tags origin main

new_revision="$(git rev-parse origin/main)"
if [[ "$old_revision" != "$new_revision" ]]; then
	run_quiet "Применяю новые файлы" "$temporary_dir/git-merge.log" git merge --quiet --ff-only origin/main
fi

version="$(git describe --tags --abbrev=0 "$new_revision" 2>/dev/null || git rev-parse --short "$new_revision")"
short_revision="$(git rev-parse --short "$new_revision")"
mapfile -t changed_files < <(git diff --name-status "$old_revision" "$new_revision")
file_count=${#changed_files[@]}

printf "${WHITE}│${RESET}  ${GREEN}✓${RESET} Версия ${WHITE}%s${RESET} ${DIM}(%s)${RESET}\n" "$version" "$short_revision"
if ((file_count == 0)); then
	printf "${WHITE}│${RESET}  ${DIM}Файлы уже актуальны${RESET}\n"
else
	printf "${WHITE}│${RESET}  ${CYAN}%d файлов обновлено${RESET}\n" "$file_count"
	visible_count=$file_count
	((visible_count > 10)) && visible_count=10
	for ((index = 0; index < visible_count; index++)); do
		entry="${changed_files[$index]}"
		status="${entry%%$'\t'*}"
		path="${entry#*$'\t'}"
		case "$status" in
		A*) marker="+" ;;
		D*) marker="−" ;;
		R*) marker="→" ;;
		*) marker="•" ;;
		esac
		printf "${WHITE}│${RESET}    ${DIM}%s %s${RESET}\n" "$marker" "$path"
	done
	if ((file_count > visible_count)); then
		printf "${WHITE}│${RESET}    ${DIM}… и ещё %d${RESET}\n" "$((file_count - visible_count))"
	fi
fi

printf "${WHITE}│${RESET}\n${WHITE}├─ Docker · сборка и запуск${RESET}\n"
export LINK_BOT_VERSION="$version"
export LINK_BOT_COMMIT="$short_revision"
export BUILDKIT_PROGRESS="plain"

started=$SECONDS
run_quiet "Собираю Docker-образ" "$temporary_dir/docker-build.log" \
	docker compose --ansi never --progress plain build
run_quiet "Перезапускаю сервисы" "$temporary_dir/docker-up.log" \
	docker compose --ansi never up -d --force-recreate --remove-orphans --no-build

running_services="$(docker compose ps --status running --services 2>/dev/null | sed '/^[[:space:]]*$/d' | wc -l | tr -d ' ')"
total_services="$(docker compose config --services 2>/dev/null | sed '/^[[:space:]]*$/d' | wc -l | tr -d ' ')"
printf "${WHITE}╰─ ${GREEN}✓ Обновление завершено${RESET} · %s/%s сервисов запущено · ${DIM}%ss${RESET}\n" \
	"$running_services" "$total_services" "$((SECONDS - started))"
