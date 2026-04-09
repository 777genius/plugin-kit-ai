---
title: "plugin-kit-ai skills generate"
description: "Generate Claude/Codex artifacts from canonical SKILL.md files"
canonicalId: "command:plugin-kit-ai:skills:generate"
surface: "cli"
section: "api"
locale: "zh"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai skills generate"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai skills generate" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai skills generate

由实际的 Cobra 命令树生成。

Generate Claude/Codex artifacts from canonical SKILL.md files

## plugin-kit-ai skills generate

Generate Claude/Codex artifacts from canonical SKILL.md files

```
plugin-kit-ai skills generate [path] [flags]
```

### Examples

```
  plugin-kit-ai skills generate . --target all
  plugin-kit-ai skills generate ./examples/skills/cli-wrapper-formatter --target codex
```

### Options

```
  -h, --help            help for generate
      --target string   generate target ("all", "claude", "codex") (default "all")
```

### SEE ALSO

* plugin-kit-ai skills	 - Experimental skill authoring tools
