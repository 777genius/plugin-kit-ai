#!/usr/bin/env bash
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
demo_tmp=$(mktemp -d /tmp/universal-agent-plugins-demo.XXXXXX)
font_ui="/System/Library/Fonts/SFNS.ttf"
font_mono="/System/Library/Fonts/SFNSMono.ttf"

render_frame() {
  local output_path="$1"
  local progress="$2"
  local line_one="$3"
  local line_two="$4"
  local line_three="$5"
  local status_text="$6"

  magick -size 1200x675 gradient:'#0B1020-#151C36' \
    -fill '#7257FF' -draw 'circle 1050,90 1100,90' \
    -fill '#20C4D9' -draw 'circle 1115,148 1148,148' \
    -fill '#FF6B4A' -draw 'circle 1010,175 1035,175' \
    -fill '#F7F8FC' -font "$font_ui" -pointsize 28 -annotate +62+75 'UNIVERSAL AGENT PLUGINS' \
    -fill '#8B96B8' -pointsize 19 -annotate +62+110 'One standard. Multiple agents. Reproducible in one minute.' \
    -fill '#11182C' -stroke '#2B365D' -strokewidth 2 \
    -draw 'roundrectangle 60,150 1140,565 24,24' \
    -stroke none -fill '#FF6B4A' -draw 'circle 92,181 98,181' \
    -fill '#F6C75C' -draw 'circle 114,181 120,181' \
    -fill '#4FD18B' -draw 'circle 136,181 142,181' \
    -fill '#6F7A9A' -font "$font_mono" -pointsize 16 -annotate +170+188 'launch-demo - disposable sandbox' \
    -fill '#DCE4FF' -pointsize 20 -annotate +92+252 "$line_one" \
    -fill '#DCE4FF' -annotate +92+315 "$line_two" \
    -fill '#DCE4FF' -annotate +92+378 "$line_three" \
    -fill '#4FD18B' -font "$font_ui" -pointsize 20 -annotate +92+450 "$status_text" \
    -fill '#202A49' -draw 'roundrectangle 92,495 1108,511 8,8' \
    -fill '#7257FF' -draw "roundrectangle 92,495 $progress,511 8,8" \
    -fill '#8995B8' -pointsize 16 -annotate +62+625 'Agent Plugins 1.0  •  Codex  •  ChatGPT  •  Cursor  •  VS Code  •  Copilot  •  Kiro' \
    "$output_path"
}

render_frame "$demo_tmp/01.png" 292 \
  '$ codex plugin marketplace add 777genius/universal-agent-plugins' \
  '' '' \
  'Adding the marketplace...'

render_frame "$demo_tmp/02.png" 485 \
  '$ codex plugin marketplace add 777genius/universal-agent-plugins' \
  '✓ Marketplace: universal-agent-plugins' '' \
  '26 portable packages discovered'

render_frame "$demo_tmp/03.png" 680 \
  '$ codex plugin add context7@universal-agent-plugins' \
  '✓ Installed context7 0.1.0' '' \
  'Pinned MCP runtime: @upstash/context7-mcp@4.0.0'

render_frame "$demo_tmp/04.png" 895 \
  '$ Ask: resolve official Playwright documentation' \
  '→ context7.resolve-library-id({ libraryName: "Playwright" })' '' \
  'Calling the installed MCP tool...'

render_frame "$demo_tmp/05.png" 1108 \
  '$ Ask: resolve official Playwright documentation' \
  '✓ CONTEXT7_OK  /microsoft/playwright' \
  '✓ Tested in Codex 0.144.1 and Cursor 3.9.16' \
  'From install to a real tool call in under a minute'

magick \
  -delay 115 "$demo_tmp/01.png" \
  -delay 105 "$demo_tmp/02.png" \
  -delay 115 "$demo_tmp/03.png" \
  -delay 125 "$demo_tmp/04.png" \
  -delay 240 "$demo_tmp/05.png" \
  -loop 0 -layers Optimize "$repo_root/assets/demo.gif"

magick "$demo_tmp/05.png" "$repo_root/assets/demo-poster.png"

printf 'Built assets/demo.gif and assets/demo-poster.png\n'
