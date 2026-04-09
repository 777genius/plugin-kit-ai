---
title: "plugin-kit-ai publication doctor"
description: "Inspect publication readiness without mutating files"
canonicalId: "command:plugin-kit-ai:publication:doctor"
surface: "cli"
section: "api"
locale: "fr"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai publication doctor"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai publication doctor" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai publication doctor

Généré à partir de l'arbre réel de commandes Cobra.

Inspect publication readiness without mutating files

## plugin-kit-ai publication doctor

Inspect publication readiness without mutating files

### Synopsis

Read-only publication readiness check for package-capable targets and authored publish/... channels.

```
plugin-kit-ai publication doctor [path] [flags]
```

### Options

```
      --dest string           optional materialized marketplace root to verify for local codex-package or claude publication flows
      --format string         output format: text or json (default "text")
  -h, --help                  help for doctor
      --package-root string   relative package root inside the destination marketplace root (default: plugins/&lt;name&gt;)
      --target string         publication target ("all", "claude", "codex-package", or "gemini") (default "all")
```

### SEE ALSO

* plugin-kit-ai publication	 - Show the publication-oriented package and channel view
