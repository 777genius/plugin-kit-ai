---
title: "Build Your First Plugin"
description: "A minimal end-to-end tutorial from init to strict validation."
canonicalId: "page:guide:first-plugin"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Build Your First Plugin

This tutorial gives you the simplest first working repo on the strongest default path.

It keeps the scope intentionally narrow:

- first target: `codex-runtime`
- first language: `go`
- first readiness gate: `validate --strict`

That narrow shape is only for the first run. If the broader one-repo, many-outputs story is the main thing you care about, read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) right after this tutorial.

## 1. Install The CLI

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

## 2. Scaffold A Project

```bash
plugin-kit-ai init my-plugin
cd my-plugin
```

The default `init` path is already the recommended production starting point.

## 3. Generate The Target Files

```bash
plugin-kit-ai generate .
```

Treat generated target files as outputs. Keep editing the repo through `plugin-kit-ai` instead of hand-maintaining generated files.

## 4. Run The Readiness Gate

```bash
plugin-kit-ai validate . --platform codex-runtime --strict
```

Use this as the main CI-grade gate for a local plugin project.

## What You Have Now

- one plugin repo
- authored files under `src/`
- generated Codex runtime output
- a clear readiness gate through `validate --strict`

## 5. When To Switch Paths

Switch to another path only when you actually need it:

- choose `claude` for Claude plugins
- choose `--runtime node --typescript` for the main supported non-Go path
- choose `--runtime python` when the project stays local to the repo and your team is Python-first
- choose `codex-package`, `gemini`, `opencode`, or `cursor` only when you really need a different way to ship the plugin

That does not mean the repo must stay single-target forever: start with the most important target today and add the others only when the product genuinely expands.

## Next Steps

- Read [Choosing Runtime](/en/concepts/choosing-runtime) before you leave the default path.
- Read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) if the one-repo, many-outputs idea is a core reason you care about the product.
- Use [Starter Templates](/en/guide/starter-templates) when you want a known-good example repo.
- Browse [CLI Reference](/en/api/cli/) when you need exact command behavior.
