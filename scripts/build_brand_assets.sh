#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

if ! command -v rsvg-convert >/dev/null 2>&1; then
  printf 'ERROR: rsvg-convert is required to render PNG assets\n' >&2
  exit 1
fi

rsvg-convert -w 512 -h 512 "$repo_root/assets/logo.svg" -o "$repo_root/assets/logo.png"
rsvg-convert -w 1600 -h 420 "$repo_root/assets/hero.svg" -o "$repo_root/assets/hero.png"
rsvg-convert -w 128 -h 128 "$repo_root/assets/logo.svg" -o "$repo_root/assets/icon.png"

printf 'Generated logo.png, icon.png, and hero.png\n'
