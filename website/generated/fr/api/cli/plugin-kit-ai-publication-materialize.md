---
title: "plugin-kit-ai publication materialize"
description: "Materialize a safe local marketplace root for Codex or Claude"
canonicalId: "command:plugin-kit-ai:publication:materialize"
surface: "cli"
section: "api"
locale: "fr"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai publication materialize"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai publication materialize" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai publication materialize

Généré à partir de l'arbre réel de commandes Cobra.

Materialize a safe local marketplace root for Codex or Claude

## plugin-kit-ai publication materialize

Materialize a safe local marketplace root for Codex or Claude

### Synopsis

Create or update a local marketplace root for a single publication-capable package target.

This workflow is intentionally limited to documented local/catalog flows:
- Codex marketplace roots with .agents/plugins/marketplace.json
- Claude marketplace roots with .claude-plugin/marketplace.json

It copies the materialized package bundle under a managed package root, then merges or creates the marketplace catalog artifact.

```
plugin-kit-ai publication materialize [path] [flags]
```

### Options

```
      --dest string           destination marketplace root directory
      --dry-run               preview the materialized package root and catalog changes without writing them
  -h, --help                  help for materialize
      --package-root string   relative package root inside the destination marketplace root (default: plugins/&lt;name&gt;)
      --target string         materialization target ("claude" or "codex-package")
```

### SEE ALSO

* plugin-kit-ai publication	 - Show the publication-oriented package and channel view
