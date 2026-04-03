---
title: "Package And Workspace Targets"
description: "How to use Codex package, Gemini, OpenCode, and Cursor targets without confusing them with executable plugin paths."
canonicalId: "page:guide:package-and-workspace-targets"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Package And Workspace Targets

Not every `plugin-kit-ai` target is an executable plugin.

Read this page before you choose `codex-package`, `gemini`, `opencode`, or `cursor`, because these targets solve a different problem than `codex-runtime` or `claude`.

## The Short Rule

- choose `codex-runtime` or `claude` when the product is an executable plugin
- choose `codex-package` or `gemini` when the product is a package or extension artifact
- choose `opencode` or `cursor` when the product is repo-owned workspace configuration

<MermaidDiagram
  :chart="`
flowchart TD
  Product[What is the product] --> Exec{Executable plugin}
  Product --> Artifact{Package or extension artifact}
  Product --> Config{Workspace config}
  Exec --> Runtime[codex-runtime or claude]
  Artifact --> Package[codex-package or gemini]
  Config --> Workspace[opencode or cursor]
`"
/>

## Codex Package

Use `codex-package` when the end result is a Codex package, not an executable plugin repo.

This is useful when:

- packaging is the real delivery contract
- you want the repo to stay unified in one place
- you do not want to pretend this target has the same runtime contract as `codex-runtime`

Codex package also has a strict bundle-layout contract:

- `.codex-plugin/` contains only `plugin.json`
- optional `.app.json` and `.mcp.json` stay at the plugin root, not inside `.codex-plugin/`
- those sidecars exist only when `.codex-plugin/plugin.json` references `./.app.json` or `./.mcp.json`

## Gemini

Use `gemini` when the goal is a Gemini CLI extension package.

This target is intentionally packaging-oriented.

Treat it as:

- a full extension-packaging lane through `render`, `import`, and `validate`
- not the main runtime path
- something you choose when Gemini extension artifacts are the actual product

## OpenCode

Use `opencode` when the repo owns OpenCode workspace configuration and related project assets.

This target is valuable when:

- the project needs managed `opencode.json`
- the repo should own workspace-level MCP and config shape
- you want a documented config authoring path instead of hand-edited files

Do not confuse that with the strongest runtime contract.

## Cursor

Use `cursor` when the repo should manage Cursor workspace configuration.

The documented subset includes:

- `.cursor/mcp.json`
- project-root `.cursor/rules/**`
- optional shared root `AGENTS.md`

This is a workspace-configuration target, not the main runtime path.

## Practical Decision Rule

Choose these targets when the output is:

- package artifacts
- extension packaging
- workspace config

Do not choose them just because they sound close to a runtime path.

If what you really need is executable plugin behavior, go back to [Choosing Runtime](/en/concepts/choosing-runtime) and start there.

## Readiness Rule

For these targets, the healthy repo rule is still the same:

- the repo stays unified in the package-standard layout
- rendered files are outputs
- `render --check` and `validate --strict` are the core checks

## Pair It With

Read this page with [Target Model](/en/concepts/target-model), [Target Support](/en/reference/target-support), and [Support Boundary](/en/reference/support-boundary).
