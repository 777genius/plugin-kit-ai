#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
: "${GITHUB_REPOSITORY_OWNER:?GITHUB_REPOSITORY_OWNER is required}"
: "${GITHUB_REF_NAME:?GITHUB_REF_NAME is required}"

repository_name="${GITHUB_REPOSITORY#*/}"
release_state="$(
  gh api graphql \
    -f query='query($owner: String!, $name: String!, $tag: String!) { repository(owner: $owner, name: $name) { release(tagName: $tag) { tagName isDraft } } }' \
    -F owner="$GITHUB_REPOSITORY_OWNER" \
    -F name="$repository_name" \
    -F tag="$GITHUB_REF_NAME" \
    --jq 'if .data.repository.release == null then "missing" else [.data.repository.release.tagName, .data.repository.release.isDraft] | @tsv end'
)"

if [[ "$release_state" == "missing" ]]; then
  gh release create "$GITHUB_REF_NAME" \
    --repo "$GITHUB_REPOSITORY" \
    --verify-tag \
    --generate-notes \
    --title "$GITHUB_REF_NAME"
  exit 0
fi

IFS=$'\t' read -r release_tag release_is_draft <<<"$release_state"
if [[ "$release_tag" != "$GITHUB_REF_NAME" || "$release_is_draft" != "false" ]]; then
  echo "Release $GITHUB_REF_NAME exists but is not a matching published release." >&2
  exit 1
fi

echo "Release $GITHUB_REF_NAME already exists and is published."
