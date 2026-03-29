#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="${TAG:-}"
if [[ -z "$TAG" ]]; then
  echo "set TAG=vX.Y.Z for the release to publish into the Homebrew tap" >&2
  exit 1
fi

REPO="${PLUGIN_KIT_AI_REPOSITORY:-plugin-kit-ai/plugin-kit-ai}"
TAP_REPO="${HOMEBREW_TAP_REPO:-plugin-kit-ai/homebrew-plugin-kit-ai}"
API_BASE="${GITHUB_API_BASE:-https://api.github.com}"
RELEASE_BASE="${PLUGIN_KIT_AI_RELEASE_BASE_URL:-https://github.com}"
TOKEN="${HOMEBREW_TAP_TOKEN:-${GITHUB_TOKEN:-}}"

if [[ -z "$TOKEN" ]]; then
  echo "set HOMEBREW_TAP_TOKEN (or GITHUB_TOKEN) with push access to ${TAP_REPO}" >&2
  exit 1
fi

TAG="${TAG#v}"
TAG="v${TAG}"
DOWNLOAD_BASE="${RELEASE_BASE%/}/${REPO}/releases/download/${TAG}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

CHECKSUMS_PATH="$TMP/checksums.txt"
curl -fsSL "${DOWNLOAD_BASE}/checksums.txt" -o "$CHECKSUMS_PATH"

FORMULA_PATH="$TMP/plugin-kit-ai.rb"
go run ./cmd/plugin-kit-ai-homebrew-gen \
  --tag "$TAG" \
  --repo "$REPO" \
  --checksums "$CHECKSUMS_PATH" \
  --download-base "$DOWNLOAD_BASE" \
  --output "$FORMULA_PATH"

TAP_URL="https://x-access-token:${TOKEN}@github.com/${TAP_REPO}.git"
git clone "$TAP_URL" "$TMP/tap"
mkdir -p "$TMP/tap/Formula"
cp "$FORMULA_PATH" "$TMP/tap/Formula/plugin-kit-ai.rb"

pushd "$TMP/tap" >/dev/null
git config user.name "plugin-kit-ai-bot"
git config user.email "actions@users.noreply.github.com"
git add Formula/plugin-kit-ai.rb
if git diff --cached --quiet; then
  echo "homebrew tap already up to date for ${TAG}"
  exit 0
fi
git commit -m "Update plugin-kit-ai formula for ${TAG}"
git push origin HEAD
popd >/dev/null
