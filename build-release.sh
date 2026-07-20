#!/usr/bin/env bash

set -euo pipefail

IMAGE_BASE="${IMAGE_BASE:-ghcr.io/your-github-username/link-bot}"

read -rp "Enter release version (e.g. 3.1.3): " VERSION
if [[ -z "${VERSION}" ]]; then
  echo "Version must not be empty" >&2
  exit 1
fi

MAJOR_VERSION="${VERSION%%.*}"
if [[ -z "${MAJOR_VERSION}" ]]; then
  echo "Failed to derive major version from '${VERSION}'" >&2
  exit 1
fi

COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "none")

docker buildx build \
  --platform linux/amd64,linux/arm64 \
  --build-arg VERSION="${VERSION}" \
  --build-arg COMMIT="${COMMIT}" \
  -t "${IMAGE_BASE}:${VERSION}" \
  -t "${IMAGE_BASE}:${MAJOR_VERSION}" \
  -t "${IMAGE_BASE}:latest" \
  --push \
  .
