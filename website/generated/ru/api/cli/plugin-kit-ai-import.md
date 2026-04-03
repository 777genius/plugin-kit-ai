---
title: "plugin-kit-ai import"
description: "Импортирует текущие нативные артефакты в package-standard структуру."
canonicalId: "command:plugin-kit-ai:import"
surface: "cli"
section: "api"
locale: "ru"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai import"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai import" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai import

Сгенерировано из реального Cobra command tree.

Импортирует текущие нативные артефакты в package-standard структуру.

## plugin-kit-ai import

Импортирует текущие нативные артефакты в package-standard структуру.

### Описание

Import an existing native plugin into the package standard layout.

Claude import maps native plugin artifacts into the package-standard layout.
Codex import materializes either the official package lane or the local runtime lane from current native artifacts. Use codex-package or codex-runtime explicitly for the lane you want to preserve.
Gemini import backfills the extension package layout and may preserve an optional launcher-based Go beta lane when that authored project already uses one. It does not promote Gemini to a production-ready runtime target.
OpenCode import is workspace-config-only in the current contract: it normalizes project-native JSON/JSONC config, commands, agents, themes, local plugin code, plugin-local package metadata, compatible skill roots, and optional user-scope OpenCode sources into the canonical package-standard layout.
Cursor import is workspace-config-only in the current contract: it normalizes .cursor/mcp.json, .cursor/rules/**, and optional root AGENTS.md into the canonical package-standard layout.

```
plugin-kit-ai import [path] [flags]
```

### Опции

```
  -f, --force                overwrite plugin.yaml if it already exists
      --from string          source platform ("claude", "codex-package", "codex-runtime", "gemini", "opencode", or "cursor"; omit to auto-detect current native layouts)
  -h, --help                 справка по import
      --include-user-scope   include explicit user-scope native sources when supported by the import target
```

### См. также

* plugin-kit-ai	 - CLI plugin-kit-ai для создания проектов и служебных операций вокруг AI-плагинов.
