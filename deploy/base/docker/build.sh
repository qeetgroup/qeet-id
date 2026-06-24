#!/usr/bin/env bash
# Build the qeet-id Docker image. The build context is the repo ROOT (the Go
# module + embedded migrations live there); the Dockerfile lives under deploy/base/docker/.
# Usage: ./deploy/base/docker/build.sh <tag>
# Example: ./deploy/base/docker/build.sh dev
#          ./deploy/base/docker/build.sh v1.2.3

set -euo pipefail

TAG=${1:?Usage: $0 <tag>}
# script is at deploy/base/docker/ → repo root is three levels up.
REPO_ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
DOCKERDIR="${REPO_ROOT}/deploy/base/docker"

echo "Building qeet-id:${TAG} from ${REPO_ROOT}"
docker build \
  --file "${DOCKERDIR}/Dockerfile" \
  --tag "ghcr.io/qeetgroup/qeet-id:${TAG}" \
  --build-arg VERSION="${TAG}" \
  --build-arg COMMIT="$(git -C "${REPO_ROOT}" rev-parse --short HEAD 2>/dev/null || echo none)" \
  --build-arg DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  "${REPO_ROOT}"

echo ""
echo "Built:"
echo "  ghcr.io/qeetgroup/qeet-id:${TAG}"
