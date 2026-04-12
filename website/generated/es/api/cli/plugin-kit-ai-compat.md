---
title: "plugin-kit-ai compat"
description: "Inspect a native source and report target compatibility"
canonicalId: "command:plugin-kit-ai:compat"
surface: "cli"
section: "api"
locale: "es"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai compat"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai compat" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai compat

Generado a partir del árbol real de comandos Cobra.

Inspect a native source and report target compatibility

## plugin-kit-ai compat

Inspect a native source and report target compatibility

```
plugin-kit-ai compat &lt;source&gt; [flags]
```

### Options

```
      --format string        output format: text or json (default "text")
      --from string          source platform ("claude", "codex-package", "codex-runtime", "gemini", "opencode", "cursor", or "cursor-workspace"; omit to auto-detect current native layouts)
  -h, --help                 help for compat
      --include-user-scope   include explicit user-scope native sources when supported by the detected import target
      --target string        compatibility target ("all", "claude", "codex-package", "codex-runtime", "gemini", "opencode", "cursor", or "cursor-workspace") (default "all")
```

### SEE ALSO

* plugin-kit-ai	 - plugin-kit-ai CLI - scaffold and tooling for AI plugins
