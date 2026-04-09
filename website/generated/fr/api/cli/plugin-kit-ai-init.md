---
title: "plugin-kit-ai init"
description: "Create a plugin-kit-ai package scaffold"
canonicalId: "command:plugin-kit-ai:init"
surface: "cli"
section: "api"
locale: "fr"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "cli:plugin-kit-ai init"
translationRequired: false
---
<DocMetaCard surface="cli" stability="public-stable" maturity="stable" source-ref="cli:plugin-kit-ai init" source-href="https://github.com/777genius/plugin-kit-ai/tree/main/cli/plugin-kit-ai" />

# plugin-kit-ai init

Généré à partir de l'arbre réel de commandes Cobra.

Create a plugin-kit-ai package scaffold

## plugin-kit-ai init

Create a plugin-kit-ai package scaffold

### Synopsis

Creates a package-standard plugin-kit-ai project scaffold.

Choose the lane that matches your goal:

Fast local plugin:
  Use --runtime python or --runtime node when repo-local iteration matters more than packaged distribution.
  These are supported executable-runtime paths, not equal production paths.

Production-ready plugin repo:
  Plain init keeps the strongest supported runtime path. --runtime go remains the default, and --platform codex-runtime remains the default target.
  Use --platform claude for Claude hooks, and add --claude-extended-hooks only when you intentionally want the wider runtime-supported subset.
  Use --platform codex-package for the official Codex plugin bundle without local notify/runtime wiring.
  Use --platform opencode for the OpenCode workspace-config lane without launcher/runtime scaffolding.
  Use --platform cursor for the Cursor workspace-config lane without launcher/runtime scaffolding.

Already have native config:
  Use plugin-kit-ai import to bring current Claude/Codex/Gemini/OpenCode/Cursor native files into the package-standard authored layout.
  init is for creating a new package-standard project, not for preserving native files as the authored source of truth.

Public flags:
  --platform   Supported: "codex-runtime" (default), "codex-package", "claude", "gemini", "opencode", and "cursor".
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
      --typescript                       generate a TypeScript scaffold on top of the node runtime lane
```

### SEE ALSO

* plugin-kit-ai	 - plugin-kit-ai CLI - scaffold and tooling for AI plugins
