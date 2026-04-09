---
title: "plugin-kit-ai skills init"
description: "Create a canonical SKILL.md skill package"
canonicalId: "command:plugin-kit-ai:skills:init"
surface: "cli"
section: "api"
locale: "zh"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai skills init"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai skills init" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai skills init

由实际的 Cobra 命令树生成。

Create a canonical SKILL.md skill package

## plugin-kit-ai skills init

Create a canonical SKILL.md skill package

```
plugin-kit-ai skills init [skill-name] [flags]
```

### Examples

```
  plugin-kit-ai skills init lint-repo --template go-command
  plugin-kit-ai skills init format-changed --template cli-wrapper --command "ruff format ."
  plugin-kit-ai skills init review-checklist --template docs-only
```

### Options

```
      --command string       default command for cli-wrapper template (default "replace-me")
      --description string   skill description
  -f, --force                overwrite existing authored files
  -h, --help                 help for init
  -o, --output string        project root containing skills/ (default ".")
      --template string      template ("go-command", "cli-wrapper", "docs-only") (default "go-command")
```

### SEE ALSO

* plugin-kit-ai skills	 - Experimental skill authoring tools
