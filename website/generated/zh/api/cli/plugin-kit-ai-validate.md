---
title: "plugin-kit-ai validate"
description: "Validate a package-standard plugin-kit-ai project"
canonicalId: "command:plugin-kit-ai:validate"
surface: "cli"
section: "api"
locale: "zh"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai validate"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai validate" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai validate

由实际的 Cobra 命令树生成。

Validate a package-standard plugin-kit-ai project

## plugin-kit-ai validate

Validate a package-standard plugin-kit-ai project

### Synopsis

Validate a package-standard plugin-kit-ai project.

Text mode is the human-readable default and prints Warning:/Failure: lines.
Use --format json for CI or automation. That mode emits the versioned
"plugin-kit-ai/validate-report" contract with schema_version=1 and an
explicit outcome of "passed", "failed", or "failed_strict_warnings".

```
plugin-kit-ai validate [path] [flags]
```

### Options

```
      --format string     output format ("text" or "json") (default "text")
  -h, --help              help for validate
      --platform string   target override ("codex-package", "codex-runtime", "claude", "gemini", "opencode", or "cursor")
      --strict            treat validation warnings as errors
```

### SEE ALSO

* plugin-kit-ai	 - plugin-kit-ai CLI - scaffold and tooling for AI plugins
