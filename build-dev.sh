#!/usr/bin/env bash

set -euo pipefail

IMAGE_BASE="${IMAGE_BASE:-ghcr.io/your-github-username/link-bot}"
DEV_TAG="dev"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")

docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION="${DEV_TAG}" \
  --build-arg COMMIT="${COMMIT}" \
  -t "${IMAGE_BASE}:${DEV_TAG}" \
  --push \
  .
