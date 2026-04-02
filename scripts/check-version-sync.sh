#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

source "$ROOT/scripts/version-contract.env"

if [[ -z "${GO_SDK_VERSION:-}" || -z "${RUNTIME_PACKAGE_VERSION:-}" ]]; then
  echo "version contract is incomplete; expected GO_SDK_VERSION and RUNTIME_PACKAGE_VERSION" >&2
  exit 1
fi

trimmed_go_sdk_version="${GO_SDK_VERSION#v}"
if [[ "${trimmed_go_sdk_version}" != "${RUNTIME_PACKAGE_VERSION}" ]]; then
  echo "version contract mismatch: GO_SDK_VERSION=${GO_SDK_VERSION} RUNTIME_PACKAGE_VERSION=${RUNTIME_PACKAGE_VERSION}" >&2
  exit 1
fi

check_rg_matches() {
  local label="$1"
  local pattern="$2"
  local expected="$3"
  shift 3
  local files=("$@")
  local out
  if command -v rg >/dev/null 2>&1; then
    out="$(rg -n -o -- "$pattern" "${files[@]}" || true)"
  else
    out="$(
      grep -n -E -o -- "$pattern" "${files[@]}" 2>/dev/null || true
    )"
  fi
  if [[ -z "$out" ]]; then
    echo "version sync check found no matches for ${label}" >&2
    exit 1
  fi
  local bad
  bad="$(printf '%s\n' "$out" | sed -E 's#^[^:]+:[0-9]+:##' | awk -v want="$expected" '$0 != want { print $0 }')"
  if [[ -n "$bad" ]]; then
    echo "version sync drift for ${label}; expected ${expected}:" >&2
    printf '%s\n' "$bad" >&2
    exit 1
  fi
}

go_sdk_files=(
  README.md
  cli/plugin-kit-ai/README.md
  sdk/README.md
  examples/starters/README.md
  examples/starters/codex-go-starter/README.md
  examples/starters/codex-go-starter/go.mod
  examples/starters/codex-go-starter/go.sum
  examples/starters/claude-go-starter/README.md
  examples/starters/claude-go-starter/go.mod
  examples/starters/claude-go-starter/go.sum
  examples/plugins/codex-basic-prod/go.mod
  examples/plugins/codex-basic-prod/go.sum
  examples/plugins/claude-basic-prod/go.mod
  examples/plugins/claude-basic-prod/go.sum
  cli/plugin-kit-ai/internal/scaffold/version_contract.go
  cli/plugin-kit-ai/internal/scaffold/templates/go.mod.tmpl
  cli/plugin-kit-ai/internal/scaffold/templates/codex.go.mod.tmpl
  cli/plugin-kit-ai/internal/scaffold/templates/README.md.tmpl
  cli/plugin-kit-ai/internal/scaffold/templates/codex-runtime.README.md.tmpl
)

runtime_package_files=(
  README.md
  cli/plugin-kit-ai/README.md
  docs/CHOOSING_HELPER_DELIVERY_MODE.md
  examples/starters/README.md
  examples/starters/codex-python-runtime-package-starter/README.md
  examples/starters/codex-python-runtime-package-starter/requirements.txt
  examples/starters/claude-node-typescript-runtime-package-starter/README.md
  examples/starters/claude-node-typescript-runtime-package-starter/package.json
  cli/plugin-kit-ai/internal/scaffold/version_contract.go
  cli/plugin-kit-ai/internal/scaffold/templates/python.requirements.txt.tmpl
  cli/plugin-kit-ai/internal/scaffold/templates/node.package.json.tmpl
)

check_rg_matches "Go SDK direct module refs" 'github\.com/777genius/plugin-kit-ai/sdk@v[0-9]+\.[0-9]+\.[0-9]+' "github.com/777genius/plugin-kit-ai/sdk@${GO_SDK_VERSION}" "${go_sdk_files[@]}"
check_rg_matches "Go SDK go.mod refs" 'github\.com/777genius/plugin-kit-ai/sdk v[0-9]+\.[0-9]+\.[0-9]+' "github.com/777genius/plugin-kit-ai/sdk ${GO_SDK_VERSION}" "${go_sdk_files[@]}"
check_rg_matches "Runtime package command pins" '--runtime-package-version [0-9]+\.[0-9]+\.[0-9]+' "--runtime-package-version ${RUNTIME_PACKAGE_VERSION}" README.md cli/plugin-kit-ai/README.md docs/CHOOSING_HELPER_DELIVERY_MODE.md
check_rg_matches "Runtime package pip pins" 'plugin-kit-ai-runtime==[0-9]+\.[0-9]+\.[0-9]+' "plugin-kit-ai-runtime==${RUNTIME_PACKAGE_VERSION}" "${runtime_package_files[@]}"
check_rg_matches "Runtime package npm pins" 'plugin-kit-ai-runtime@[0-9]+\.[0-9]+\.[0-9]+' "plugin-kit-ai-runtime@${RUNTIME_PACKAGE_VERSION}" "${runtime_package_files[@]}"
check_rg_matches "Runtime package package.json pins" '"plugin-kit-ai-runtime": "[0-9]+\.[0-9]+\.[0-9]+"' "\"plugin-kit-ai-runtime\": \"${RUNTIME_PACKAGE_VERSION}\"" "${runtime_package_files[@]}"

echo "version references are in sync"
