#!/usr/bin/env bash
# Manual, deliberate publish for cmdop-care-mcp. Never invoked from cmdop_go's
# own release pipeline or any CI — see @dev/active/cmdop-care-mcp/PLAN.md
# ("Architecture decisions: No submodule-as-release-channel; no auto-publish").
#
# Usage:
#   release/publish.sh              # build, tag, push image + git tag
#   release/publish.sh --dry-run    # build + tag locally, skip both pushes
#
# Credentials: source release/.env first (gitignored), or export inline.
#   GHCR_TOKEN   - a GitHub PAT/token with `write:packages` scope
#   GHCR_USER    - the GitHub username/org to authenticate as (default: commandoperator)
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

DRY_RUN=0
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    *) echo "unknown flag: $arg" >&2; exit 1 ;;
  esac
done

if [[ -f "release/.env" ]]; then
  # shellcheck disable=SC1091
  source "release/.env"
fi

GHCR_USER="${GHCR_USER:-commandoperator}"
IMAGE="ghcr.io/commandoperator/cmdop-care"
TAG="$(cat VERSION)"
# server.json / main.go's stamped version omit the git-tag "v" prefix
# (matches server.json's "version": "0.1.1"); TAG keeps it (matches the git
# tag and the ghcr.io image tag, e.g. v0.1.1).
BUILD_VERSION="${TAG#v}"
PLATFORMS="linux/amd64,linux/arm64"

echo "== cmdop-care-mcp release =="
echo "version:   ${BUILD_VERSION}"
echo "image:     ${IMAGE}:${TAG}"
echo "platforms: ${PLATFORMS}"
echo "dry-run:   ${DRY_RUN}"
echo

echo "-- build --"
go build ./...
go vet ./...
go test ./...

echo "-- docker build (local, native platform only — for the non-root/label check below) --"
docker build --build-arg "VERSION=${BUILD_VERSION}" -t "${IMAGE}:${TAG}" .

echo "-- verify non-root + label (never skip this) --"
actual_user="$(docker inspect "${IMAGE}:${TAG}" --format='{{.Config.User}}')"
actual_label="$(docker inspect "${IMAGE}:${TAG}" --format='{{index .Config.Labels "io.modelcontextprotocol.server.name"}}')"
if [[ "$actual_user" == "0" || "$actual_user" == "0:0" || -z "$actual_user" ]]; then
  echo "REFUSING to publish: image does not run as a non-root user (got '${actual_user}')" >&2
  exit 1
fi
if [[ "$actual_label" != "io.github.commandoperator/cmdop-care" ]]; then
  echo "REFUSING to publish: OCI label '${actual_label}' does not match server.json name" >&2
  exit 1
fi
echo "user=${actual_user} label=${actual_label} — OK"

if [[ "$DRY_RUN" == "1" ]]; then
  echo
  echo "-- dry-run: stopping before any push. Built and verified locally only. --"
  exit 0
fi

if [[ -z "${GHCR_TOKEN:-}" ]]; then
  echo "REFUSING to push: GHCR_TOKEN not set (source release/.env or export it)" >&2
  exit 1
fi

echo "-- docker login --"
echo "${GHCR_TOKEN}" | docker login ghcr.io -u "${GHCR_USER}" --password-stdin

echo "-- docker buildx push (multi-arch: ${PLATFORMS}) --"
# A plain `docker build`+`docker push` only ever produces ONE platform (the
# build host's native arch) — that previously shipped an arm64-labeled image
# index with no linux/amd64 manifest at all from an Apple Silicon build host.
# buildx --platform ... --push builds each requested platform natively (via
# QEMU where cross-compiling the final stage's distroless base) and pushes a
# single multi-platform image index in one step.
docker buildx build --platform "${PLATFORMS}" --build-arg "VERSION=${BUILD_VERSION}" -t "${IMAGE}:${TAG}" --push .

echo "-- verify the pushed multi-arch index (anonymous, no credentials) --"
docker logout ghcr.io >/dev/null 2>&1 || true
docker manifest inspect "${IMAGE}:${TAG}"

echo "-- git tag + push --"
git tag "${TAG}"
git push origin "${TAG}"

echo
echo "== published ${IMAGE}:${TAG} (${PLATFORMS}) + git tag ${TAG} =="
