#!/usr/bin/env bash
set -uo pipefail

warning_count=0

warn() {
  printf 'WARN: %s\n' "$1"
  warning_count=$((warning_count + 1))
}

check_command() {
  local label="$1"
  local command_name="$2"
  shift 2

  printf '\n[%s]\n' "$label"
  if command -v "$command_name" >/dev/null 2>&1; then
    if output=$("$@" 2>&1); then
      printf '%s\n' "$output" | sed -n '1,3p'
      printf 'OK: %s is available\n' "$command_name"
    else
      printf '%s\n' "$output" | sed -n '1,3p'
      warn "$command_name exists but its version command failed"
    fi
  else
    warn "$command_name is not available"
  fi
}

check_uv_tool_version() {
  local label="$1"
  local package="$2"
  local expected="$3"
  local command_name="$4"

  printf '\n[%s]\n' "$label"
  if ! command -v "$command_name" >/dev/null 2>&1; then
    warn "$command_name is not available"
    return
  fi

  if ! output=$("$command_name" --version 2>&1); then
    printf '%s\n' "$output" | sed -n '1p'
    warn "$command_name exists but its version command failed"
    return
  fi
  printf '%s\n' "$output" | sed -n '1p'
  if command -v uv >/dev/null 2>&1 && uv tool list 2>/dev/null | grep -E "^${package} v${expected}([[:space:]]|$)" >/dev/null; then
    printf 'OK: %s==%s\n' "$package" "$expected"
  else
    warn "$command_name works, but ${package}==${expected} was not verified in uv tools"
  fi
}

check_command "ripgrep" "rg" rg --version
check_command "uv" "uv" uv --version
check_command "uvx" "uvx" uvx --version
check_uv_tool_version "Semble" "semble" "0.5.4" "semble"
check_uv_tool_version "CodeGraphContext" "codegraphcontext" "0.5.6" "cgc"
check_uv_tool_version "Serena (optional)" "serena-agent" "1.6.1" "serena"

printf '\n[Summary]\n'
printf 'warnings=%s\n' "$warning_count"
printf 'Use rg for exact search; treat Semble, Serena, and CGC as optional evidence sources.\n'

exit 0
