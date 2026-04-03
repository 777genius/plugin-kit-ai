---
title: "plugin-kit-ai import"
description: "Import current native target artifacts into the package standard layout"
canonicalId: "command:plugin-kit-ai:import"
surface: "cli"
section: "api"
locale: "en"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai import"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai import" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai import

Generated from the live Cobra command tree.

Import current native target artifacts into the package standard layout

## plugin-kit-ai import

Import current native target artifacts into the package standard layout

### Synopsis

Import an existing native plugin into the package standard layout.

Claude import maps native plugin artifacts into the package-standard layout.
Codex import can materialize the official package lane, the local runtime lane, or both from current native artifacts. Use codex-native when you want the combined current Codex native layout; use codex-package or codex-runtime when you already know the target lane.
Gemini import is packaging-only in the current contract: it backfills manifest metadata, but does not promote Gemini to a production-ready runtime target.
OpenCode import is workspace-config-only in the current contract: it normalizes project-native JSON/JSONC config, commands, agents, themes, local plugin code, plugin-local package metadata, compatible skill roots, and optional user-scope OpenCode sources into the canonical package-standard layout.
Cursor import is workspace-config-only in the current contract: it normalizes .cursor/mcp.json, .cursor/rules/**, and optional root AGENTS.md into the canonical package-standard layout.

```
plugin-kit-ai import [path] [flags]
```

### Options

```
  -f, --force                overwrite plugin.yaml if it already exists
      --from string          source platform ("claude", "codex-native", "codex-package", "codex-runtime", "gemini", "opencode", or "cursor"; omit to auto-detect current native layouts)
  -h, --help                 help for import
      --include-user-scope   include explicit user-scope native sources when supported by the import target
```

### SEE ALSO

* plugin-kit-ai	 - plugin-kit-ai CLI - scaffold and tooling for AI plugins
