---
title: "plugin-kit-ai init"
description: "Create a plugin-kit-ai package scaffold"
canonicalId: "command:plugin-kit-ai:init"
surface: "cli"
section: "api"
locale: "zh"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai init"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai init" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai init

由实际的 Cobra 命令树生成。

Create a plugin-kit-ai package scaffold

## plugin-kit-ai init

Create a plugin-kit-ai package scaffold

### Synopsis

Creates a package-standard plugin-kit-ai project scaffold.

Start with the job you want to solve:

Connect an online service:
  Use --template online-service for hosted integrations like Notion, Stripe, Cloudflare, or Vercel.
  This starter creates an MCP-first repo with shared authored source under src/ and no launcher code.

Connect a local tool:
  Use --template local-tool for local MCP-backed tools like Docker Hub, Chrome DevTools, or HubSpot Developer.
  This starter creates an MCP-first repo with local command wiring under src/ and no launcher code.

Build custom plugin logic:
  Use --template custom-logic when you need launcher-backed code, hooks, or your own runtime behavior.
  Plain init stays backward-compatible here: codex-runtime plus --runtime go remains the default path.

Already have native config:
  Use plugin-kit-ai import to bring current Claude/Codex/Gemini/OpenCode/Cursor native files into the package-standard authored layout.
  init is for creating a new package-standard project, not for preserving native files as the authored source of truth.

Public flags:
  --template   Recommended start: "online-service", "local-tool", or "custom-logic".
  --platform   Advanced override: "codex-runtime" (default), "codex-package", "claude", "gemini", "opencode", or "cursor".
  --runtime    Supported: "go" (default), "python", "node", "shell" for launcher-based targets only.
  --typescript Generate a TypeScript scaffold on top of the node runtime lane (requires --runtime node).
  --runtime-package
               For --runtime python or --runtime node, import the shared plugin-kit-ai-runtime package instead of vendoring the helper file into src/.
  --runtime-package-version
               Pin the generated plugin-kit-ai-runtime dependency version. Required on development builds; released CLIs default to their own stable tag.
  -o, --output Target directory (default: ./&lt;project-name&gt;).
  -f, --force  Allow writing into a non-empty directory and overwrite generated files.
  --extras     Also emit optional release helpers such as Makefile, .goreleaser.yml, portable skills/, and stable Python/Node bundle-release workflow scaffolding where supported.
  --claude-extended-hooks
               For --platform claude, scaffold the full runtime-supported hook set instead of the stable default subset.

```
plugin-kit-ai init [project-name] [flags]
```

### Options

```
      --claude-extended-hooks            for --platform claude, scaffold the full runtime-supported hook set instead of the stable default subset
      --extras                           include optional scaffold files (runtime-dependent extras plus skills and commands)
  -f, --force                            overwrite generated files; allow non-empty output directory
  -h, --help                             help for init
  -o, --output string                    output directory (default: ./&lt;project-name&gt;)
      --platform string                  target lane ("codex-runtime", "codex-package", "claude", "gemini", "opencode", or "cursor") (default "codex-runtime")
      --runtime string                   runtime ("go", "python", "node", or "shell") (default "go")
      --runtime-package                  for --runtime python or --runtime node, import the shared plugin-kit-ai-runtime package instead of vendoring the helper file
      --runtime-package-version string   pin the generated plugin-kit-ai-runtime dependency version
      --template string                  recommended start ("online-service", "local-tool", or "custom-logic")
      --typescript                       generate a TypeScript scaffold on top of the node runtime lane
```

### SEE ALSO

* plugin-kit-ai	 - plugin-kit-ai CLI - scaffold and tooling for AI plugins
