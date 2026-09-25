#!/usr/bin/env bash
set -Eeuo pipefail

if (($# > 1)) || { (($# == 1)) && [[ $1 != --standalone ]]; }; then
  printf 'Usage: bash update.sh [--standalone]\n' >&2
  exit 2
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
cd "$repo_root"

bash ./scripts/ensure-env.sh .env

if (($# == 1)); then
  docker compose --profile standalone up -d --build
else
  docker compose up -d --build bot
fi
