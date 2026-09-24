#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  printf 'Usage: bash rollback.sh [--dry-run] [--to vX.Y.Z]\n'
}

dry_run=false
requested_tag=""
while (($#)); do
  case "$1" in
    --dry-run) dry_run=true; shift ;;
    --to)
      if (($# < 2)); then usage >&2; exit 2; fi
      requested_tag="$2"
      shift 2
      ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

if [[ -n "$requested_tag" && ! "$requested_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  printf 'Invalid release tag: %s\n' "$requested_tag" >&2
  exit 2
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_root"
for command_name in git docker tar sort mktemp; do
  if ! command -v "$command_name" >/dev/null 2>&1; then
    printf 'Required command is missing: %s\n' "$command_name" >&2
    exit 1
  fi
done

if ! running_image=$(docker inspect --format '{{.Image}}' link-bot 2>/dev/null) ||
   [[ $(docker inspect --format '{{.State.Running}}' link-bot) != true ]]; then
  printf 'The link-bot container must be running before rollback.\n' >&2
  exit 1
fi

# A rollback can work offline when the previous release tags are already local.
if ! git fetch --quiet --tags origin; then
  printf 'Warning: could not fetch tags; using local release tags.\n' >&2
fi
mapfile -t release_tags < <(git tag --list | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | sort -V)
if ((${#release_tags[@]} < 2)); then
  printf 'At least two release tags are required for rollback.\n' >&2
  exit 1
fi

running_version=$(docker inspect --format '{{ index .Config.Labels "org.opencontainers.image.version" }}' link-bot 2>/dev/null || true)
current_tag=""
current_commit=$(git rev-parse HEAD)
if [[ "$running_version" =~ ^v?([0-9]+\.[0-9]+\.[0-9]+)$ ]]; then
  current_tag="v${BASH_REMATCH[1]}"
  if ! git rev-parse --verify --quiet "refs/tags/$current_tag^{commit}" >/dev/null; then
    printf 'Running version %s has no matching local release tag.\n' "$current_tag" >&2
    exit 1
  fi
  current_commit=$(git rev-parse "refs/tags/$current_tag^{commit}")
else
  for tag in "${release_tags[@]}"; do
    if [[ $(git rev-parse "refs/tags/$tag^{commit}") == "$current_commit" ]]; then
      current_tag="$tag"
    fi
  done
fi

target_tag=""
if [[ -n "$requested_tag" ]]; then
  if ! git rev-parse --verify --quiet "refs/tags/$requested_tag^{commit}" >/dev/null ||
     ! git merge-base --is-ancestor "$requested_tag" "$current_commit" ||
     [[ $(git rev-parse "refs/tags/$requested_tag^{commit}") == "$current_commit" ]]; then
    printf 'Release %s is not an older release of the running version.\n' "$requested_tag" >&2
    exit 1
  fi
  target_tag="$requested_tag"
else
  for ((i=${#release_tags[@]}-1; i>=0; i--)); do
    tag=${release_tags[i]}
    if [[ "$tag" == "$current_tag" ]]; then
      continue
    fi
    if [[ -n "$current_tag" && $(printf '%s\n%s\n' "$tag" "$current_tag" | sort -V | head -n 1) != "$tag" ]]; then
      continue
    fi
    if git merge-base --is-ancestor "$tag" "$current_commit"; then
      target_tag="$tag"
      break
    fi
  done
fi
if [[ -z "$target_tag" ]]; then
  printf 'No earlier release is available for rollback.\n' >&2
  exit 1
fi

target_commit=$(git rev-parse "refs/tags/$target_tag^{commit}")
printf 'Running release: %s\n' "${current_tag:-untagged image (source: $(git rev-parse --short HEAD))}"
printf 'Rollback target: %s (%s)\n' "$target_tag" "${target_commit:0:12}"
if "$dry_run"; then
  printf 'Dry run: no image, container, or database was changed.\n'
  exit 0
fi

docker compose config --quiet
database_state=$(docker compose exec -T db sh -c 'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -At -c "SELECT version, dirty FROM schema_migrations"')
IFS='|' read -r database_version database_dirty <<< "$database_state"
if [[ ! "$database_version" =~ ^[0-9]+$ || "$database_dirty" != f ]]; then
  printf 'Database migration state is missing or dirty; rollback stopped.\n' >&2
  exit 1
fi

# Build the target separately. The running image remains available if this fails.
build_dir=$(mktemp -d)
trap 'rm -rf -- "$build_dir"' EXIT
git archive "$target_tag" | tar -xf - -C "$build_dir"
# Older golang-migrate builds cannot start if the database version exceeds
# their bundled migrations. Include only migrations already applied to this DB.
for migration in "$repo_root"/db/migrations/*.up.sql "$repo_root"/db/migrations/*.down.sql; do
  [[ -f "$migration" ]] || continue
  migration_name=${migration##*/}
  migration_number=${migration_name%%_*}
  if ((10#$migration_number <= database_version)) && [[ ! -e "$build_dir/db/migrations/$migration_name" ]]; then
    cp -- "$migration" "$build_dir/db/migrations/$migration_name"
  fi
done
shopt -s nullglob
database_migration_files=("$build_dir"/db/migrations/"$(printf '%06d' "$database_version")"_*.up.sql)
shopt -u nullglob
if ((${#database_migration_files[@]} == 0)); then
  printf 'Migration %s is missing from the rollback image; no container was changed.\n' "$database_version" >&2
  exit 1
fi
candidate_image="link-bot:rollback-${target_tag#v}"
docker build --build-arg "VERSION=${target_tag#v}" --build-arg "COMMIT=$target_commit" -t "$candidate_image" "$build_dir"
candidate_id=$(docker image inspect --format '{{.Id}}' "$candidate_image")

# The application runs forward-only migrations. Keep an export of the current
# database before starting older code; never automatically downgrade the DB.
umask 077
backup_file=$(mktemp -p "$repo_root" "link-bot-backup-before-rollback-${target_tag}-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX.sql")
if ! docker compose exec -T db sh -c 'pg_dump -U "$POSTGRES_USER" "$POSTGRES_DB"' > "$backup_file" ||
   [[ ! -s "$backup_file" ]]; then
  rm -f -- "$backup_file"
  printf 'Database backup failed; the running bot was not changed.\n' >&2
  exit 1
fi
printf 'Database backup: %s\n' "$backup_file"

restore_running_image() {
  printf 'Rollback did not start successfully; restoring the previous image.\n' >&2
  docker tag "$running_image" link-bot:local
  docker compose up -d --no-build --no-deps --force-recreate bot
}

docker tag "$candidate_image" link-bot:local
if ! docker compose up -d --no-build --no-deps --force-recreate bot; then
  restore_running_image
  exit 1
fi
for ((attempt=0; attempt<3; attempt++)); do
  sleep 5
  if [[ $(docker inspect --format '{{.State.Running}}' link-bot 2>/dev/null || true) != true ]] ||
     [[ $(docker inspect --format '{{.Image}}' link-bot 2>/dev/null || true) != "$candidate_id" ]]; then
    restore_running_image
    exit 1
  fi
done

printf 'Link-Bot is running release %s. Source checkout remains unchanged.\n' "$target_tag"
docker compose ps bot
