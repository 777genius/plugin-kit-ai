---
title: "Package And Workspace Targets"
description: "How to use package, extension, and repo-owned integration setup without confusing them with executable runtime paths."
canonicalId: "page:guide:package-and-workspace-targets"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Package And Workspace Targets

Not every `plugin-kit-ai` path is an executable runtime path.

Read this page before you choose `codex-package`, `gemini`, `opencode`, or `cursor`, because these targets solve a different problem than `codex-runtime` or `claude`.

## The Short Rule

- choose `codex-runtime` or `claude` when the product is executable plugin behavior
- choose `codex-package` or `gemini` when the product is a package or extension artifact
- choose `opencode` or `cursor` when the product is repo-owned integration setup

## Recommended Package And Extension Lanes

### Codex Package

Use `codex-package` when the end result is a Codex package.

This is the right path when:

- packaging is the real delivery contract
- you want the repo to stay unified
- the product should ship an official Codex package artifact

### Gemini

Use `gemini` when the goal is a Gemini CLI extension package.

Treat it as:

- a recommended extension path through `generate`, `import`, and `validate`
- the right choice when Gemini extension artifacts are the real product
- separate from the default Codex runtime starting point

## Repo-Owned Integration Setup

### OpenCode

Use `opencode` when the repo should own OpenCode integration setup and related project assets.

### Cursor

Use `cursor` when the repo should own Cursor integration setup.

These paths are valuable when the output is integration setup in the repo, not executable behavior.

## Readiness Rule

For these paths, the healthy repo rule is still the same:

- the authored project stays in the package-standard layout
- generated files are outputs
- `generate --check` and `validate --strict` remain the core gates

If what you really need is executable behavior, go back to [Choosing Runtime](/en/concepts/choosing-runtime).
