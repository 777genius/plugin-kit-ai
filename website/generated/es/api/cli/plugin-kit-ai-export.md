---
title: "plugin-kit-ai export"
description: "Create a portable interpreted-runtime bundle without changing install semantics"
canonicalId: "command:plugin-kit-ai:export"
surface: "cli"
section: "api"
locale: "es"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai export"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai export" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai export

Generado a partir del árbol real de comandos Cobra.

Create a portable interpreted-runtime bundle without changing install semantics

## plugin-kit-ai export

Create a portable interpreted-runtime bundle without changing install semantics

### Synopsis

Create a deterministic portable .tar.gz bundle for launcher-based interpreted runtime projects.

This beta surface is a bounded handoff/export flow for python, node, and shell runtime repos.
It does not extend plugin-kit-ai install, and it does not imply marketplace packaging or dependency-preinstalled installs.

```
plugin-kit-ai export [path] [flags]
```

### Options

```
  -h, --help              help for export
      --output string     write bundle to this .tar.gz path (default: &lt;root&gt;/&lt;name&gt;_&lt;platform&gt;_&lt;runtime&gt;_bundle.tar.gz)
      --platform string   target override ("codex-runtime" or "claude")
```

### SEE ALSO

* plugin-kit-ai	 - plugin-kit-ai CLI - scaffold and tooling for AI plugins
