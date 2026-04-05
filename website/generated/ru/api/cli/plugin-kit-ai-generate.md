---
title: "plugin-kit-ai generate"
description: "Собирает нативные артефакты целевых платформ из package graph."
canonicalId: "command:plugin-kit-ai:generate"
surface: "cli"
section: "api"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai generate"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai generate" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai generate

Сгенерировано из реального Cobra command tree.

Собирает нативные артефакты целевых платформ из package graph.

## plugin-kit-ai generate

Собирает нативные артефакты целевых платформ из package graph.

### Описание

Собирает нативные артефакты целевых платформ из package graph на основе canonical `src/plugin.yaml` и стандартных authored directories.

Claude and Codex runtime/package lanes generate their managed native artifacts from the package graph.
Gemini generation always produces the native extension package artifacts and may also carry the optional Go runtime lane when the authored project includes it; that lane now exposes a production-ready 9-hook runtime surface, but it still does not imply blanket runtime parity for future hooks beyond the promoted contract.
OpenCode generation is workspace-config-only: it produces opencode.json plus mirrored skills, commands, agents, themes, local plugin code, and plugin-local package metadata without introducing a launcher/runtime contract.
Cursor generation is workspace-config-only: it produces .cursor/mcp.json and mirrored .cursor/rules/** without introducing a launcher/runtime contract. Root AGENTS.md and CLAUDE.md are boundary docs for the plugin root, not Cursor-native artifacts.

```
plugin-kit-ai generate [path] [flags]
```

### Опции

```
      --check           fail if generated artifacts are out of date
  -h, --help            справка по generate
      --target string   generate target ("all", "claude", "codex-package", "codex-runtime", "gemini", "opencode", or "cursor") (default "all")
```

### См. также

* plugin-kit-ai	 - CLI plugin-kit-ai для создания проектов и служебных операций вокруг AI-плагинов.
