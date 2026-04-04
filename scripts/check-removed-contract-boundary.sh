#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

has_error=0

check_pattern() {
  local label="$1"
  local pattern="$2"
  shift 2

  local args=(
    -n
    --hidden
    --glob=!**/node_modules/**
    --glob=!**/.git/**
    --glob=!scripts/check-removed-contract-boundary.sh
  )

  for exclude in "$@"; do
    args+=("--glob=!${exclude}")
  done

  local matches
  matches="$(rg "${args[@]}" -- "${pattern}" . || true)"
  if [[ -n "${matches}" ]]; then
    echo "forbidden ${label} references found:" >&2
    echo "${matches}" >&2
    has_error=1
  fi
}

check_pattern "removed Cursor rules import" '\.cursorrules' 'docs/research/**'
check_pattern "removed OpenCode env-config compatibility" 'OPENCODE_CONFIG(_DIR)?'
check_pattern "removed Gemini binary aliases" 'PLUGIN_KIT_AI_GEMINI_BIN|GEMINI_BIN'
check_pattern "Gemini migratedTo field outside research or runtime codec" 'migratedTo|migrated_to' 'docs/research/**' 'cli/plugin-kit-ai/internal/geminimanifest/**' 'cli/plugin-kit-ai/internal/platformexec/gemini.go' 'cli/plugin-kit-ai/internal/validate/validate_test.go' 'repotests/plugin_manifest_lifecycle_integration_test.go' 'sdk/platformmeta/platformmeta.go'
check_pattern "deleted maintainer docs tree" 'maintainer-docs' 'website/tools/quality/check-output.mjs'
check_pattern "removed guide slug" 'migrate''-existing-config'

if [[ "${has_error}" -ne 0 ]]; then
  echo "removed-contract boundary check failed" >&2
  exit 1
fi

echo "removed-contract boundary intact"
