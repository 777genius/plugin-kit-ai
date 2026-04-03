---
title: "plugin-kit-ai render"
description: "Собирает нативные артефакты целевых платформ из package graph."
canonicalId: "command:plugin-kit-ai:render"
surface: "cli"
section: "api"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai render"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai render" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai render

Сгенерировано из реального Cobra command tree.

Собирает нативные артефакты целевых платформ из package graph.

## plugin-kit-ai render

Собирает нативные артефакты целевых платформ из package graph.

### Описание

Собирает нативные артефакты целевых платформ из package graph. discovered via plugin.yaml and standard directories.

Claude and Codex runtime/package lanes render their managed native artifacts from the package graph.
Gemini rendering always produces the native extension package artifacts and may also carry the optional Go beta hook lane when the authored project includes it; this still does not imply runtime parity or a production-ready Gemini runtime path.
OpenCode rendering is workspace-config-only: it produces opencode.json plus mirrored skills, commands, agents, themes, local plugin code, and plugin-local package metadata without introducing a launcher/runtime contract.
Cursor rendering is workspace-config-only: it produces .cursor/mcp.json, mirrored .cursor/rules/**, and optional root AGENTS.md without introducing a launcher/runtime contract.

```
plugin-kit-ai render [path] [flags]
```

### Опции

```
      --check           fail if generated artifacts are out of date
  -h, --help            справка по render
      --target string   render target ("all", "claude", "codex-package", "codex-runtime", "gemini", "opencode", or "cursor") (default "all")
```

### См. также

* plugin-kit-ai	 - CLI plugin-kit-ai для создания проектов и служебных операций вокруг AI-плагинов.
