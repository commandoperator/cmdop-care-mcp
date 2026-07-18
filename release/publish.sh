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
VERSION="$(cat VERSION)"
TAG="${VERSION}"

echo "== cmdop-care-mcp release =="
echo "version: ${VERSION}"
echo "image:   ${IMAGE}:${TAG}"
echo "dry-run: ${DRY_RUN}"
echo

echo "-- build --"
go build ./...
go vet ./...
go test ./...

echo "-- docker build --"
docker build -t "${IMAGE}:${TAG}" .

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

echo "-- docker push --"
echo "${GHCR_TOKEN}" | docker login ghcr.io -u "${GHCR_USER}" --password-stdin
docker push "${IMAGE}:${TAG}"

echo "-- git tag + push --"
git tag "${TAG}"
git push origin "${TAG}"

echo
echo "== published ${IMAGE}:${TAG} + git tag ${TAG} =="
