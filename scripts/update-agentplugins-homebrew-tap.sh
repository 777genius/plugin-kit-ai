#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

TAG="${TAG:-}"
SOURCE_REPOSITORY="${AGENTPLUGINS_REPOSITORY:-777genius/universal-agent-plugins}"
TAP_REPOSITORY="${HOMEBREW_TAP_REPOSITORY:-777genius/homebrew-agentplugins}"
SOURCE_TOKEN="$(printf '%s' "${SOURCE_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}" | tr -d '\r\n')"
TAP_TOKEN="$(printf '%s' "${HOMEBREW_TAP_TOKEN:-}" | tr -d '\r\n')"

fail() {
  echo "agentplugins Homebrew publisher: $*" >&2
  exit 1
}

[[ "$TAG" =~ ^agentplugins-v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$ ]] \
  || fail "TAG must be an exact stable agentplugins-vX.Y.Z tag"
[[ "$SOURCE_REPOSITORY" =~ ^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9][A-Za-z0-9._-]*$ ]] \
  || fail "invalid source repository"
[[ "$TAP_REPOSITORY" =~ ^[A-Za-z0-9][A-Za-z0-9-]*/homebrew-[A-Za-z0-9][A-Za-z0-9._-]*$ ]] \
  || fail "tap repository must use an owner/homebrew-name identity"
[[ -n "$SOURCE_TOKEN" ]] || fail "SOURCE_GITHUB_TOKEN or GITHUB_TOKEN is required"
[[ -n "$TAP_TOKEN" ]] || fail "HOMEBREW_TAP_TOKEN is required"
command -v gh >/dev/null 2>&1 || fail "gh is required"
command -v jq >/dev/null 2>&1 || fail "jq is required"
command -v node >/dev/null 2>&1 || fail "node is required"
command -v ruby >/dev/null 2>&1 || fail "ruby is required"

TEMP_ROOT="$(mktemp -d)"
cleanup() {
  rm -rf "$TEMP_ROOT"
}
trap cleanup EXIT HUP INT TERM

release_json="$(GH_TOKEN="$SOURCE_TOKEN" gh release view "$TAG" \
  --repo "$SOURCE_REPOSITORY" \
  --json isDraft,isPrerelease,tagName,assets)"
jq -e --arg tag "$TAG" \
  '.isDraft == false and .isPrerelease == false and .tagName == $tag and
   ([.assets[].name | select(. == "release-manifest.json")] | length) == 1' \
  <<<"$release_json" >/dev/null \
  || fail "source release is not an exact published Agentplugins release"

mkdir -p "$TEMP_ROOT/release"
GH_TOKEN="$SOURCE_TOKEN" gh release download "$TAG" \
  --repo "$SOURCE_REPOSITORY" \
  --pattern release-manifest.json \
  --dir "$TEMP_ROOT/release"

MANIFEST="$TEMP_ROOT/release/release-manifest.json"
COMMIT="$(node -e '
const fs = require("node:fs");
const value = JSON.parse(fs.readFileSync(process.argv[1], "utf8"));
if (!/^[0-9a-f]{40}$/.test(String(value.commit))) process.exit(1);
process.stdout.write(value.commit);
' "$MANIFEST")" || fail "release manifest does not contain a valid source commit"
TAG_COMMIT="$(GH_TOKEN="$SOURCE_TOKEN" gh api \
  "repos/${SOURCE_REPOSITORY}/commits/${TAG}" --jq .sha)"
[[ "$TAG_COMMIT" == "$COMMIT" ]] || fail "release tag does not match the attested source commit"

GH_TOKEN="$SOURCE_TOKEN" gh attestation verify "$MANIFEST" \
  --repo "$SOURCE_REPOSITORY" \
  --signer-workflow "github.com/${SOURCE_REPOSITORY}/.github/workflows/agentplugins-release.yml" \
  --source-digest "$COMMIT" >/dev/null \
  || fail "release manifest attestation verification failed"

FORMULA="$TEMP_ROOT/agentplugins.rb"
node npm/agentplugins/scripts/homebrew-formula.js \
  "$MANIFEST" "$FORMULA" "$SOURCE_REPOSITORY"
ruby -c "$FORMULA" >/dev/null

ASKPASS="$TEMP_ROOT/git-askpass.sh"
cat >"$ASKPASS" <<'EOF'
#!/usr/bin/env sh
case "$1" in
  *Username*) printf '%s\n' 'x-access-token' ;;
  *Password*) printf '%s\n' "$HOMEBREW_TAP_TOKEN" ;;
  *) exit 1 ;;
esac
EOF
chmod 0700 "$ASKPASS"

export HOMEBREW_TAP_TOKEN="$TAP_TOKEN"
export GIT_ASKPASS="$ASKPASS"
export GIT_TERMINAL_PROMPT=0
git clone --depth 1 "https://github.com/${TAP_REPOSITORY}.git" "$TEMP_ROOT/tap"
mkdir -p "$TEMP_ROOT/tap/Formula"
cp "$FORMULA" "$TEMP_ROOT/tap/Formula/agentplugins.rb"

git -C "$TEMP_ROOT/tap" config user.name "agentplugins-release"
git -C "$TEMP_ROOT/tap" config user.email "actions@users.noreply.github.com"
git -C "$TEMP_ROOT/tap" add Formula/agentplugins.rb
if git -C "$TEMP_ROOT/tap" diff --cached --quiet; then
  echo "Homebrew formula already matches $TAG"
  exit 0
fi

git -C "$TEMP_ROOT/tap" commit -m "chore: update agentplugins to ${TAG#agentplugins-v}"
git -C "$TEMP_ROOT/tap" push origin HEAD:main
echo "Published ${TAP_REPOSITORY}/Formula/agentplugins.rb for $TAG"
