---
title: "plugin-kit-ai integrations add"
description: "Plan installation of an integration across supported agent targets"
canonicalId: "command:plugin-kit-ai:integrations:add"
surface: "cli"
section: "api"
locale: "zh"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai integrations add"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai integrations add" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai integrations add

由实际的 Cobra 命令树生成。

Plan installation of an integration across supported agent targets

## plugin-kit-ai integrations add

Plan installation of an integration across supported agent targets

```
plugin-kit-ai integrations add &lt;source&gt; [flags]
```

### Options

```
      --adopt-new-targets string   policy for newly supported targets: manual or auto (default "manual")
      --auto-update                desired auto-update policy (default true)
      --dry-run                    plan only without mutating native targets (default true)
  -h, --help                       help for add
      --pre                        allow prerelease updates
      --scope string               scope intent for the planned installation (default "user")
      --target strings             limit planning to one or more targets
```

### SEE ALSO

* plugin-kit-ai integrations	 - Foundation lifecycle commands for multi-agent integration management
