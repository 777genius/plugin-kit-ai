---
title: "Target Model"
description: "How plugin-kit-ai divides runtime, package, extension, and workspace-configuration targets."
canonicalId: "page:concepts:target-model"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Target Model

`plugin-kit-ai` supports several target types, but they do not all mean the same thing in practice.

## Quick Rule

- choose a runtime target when you want an executable plugin
- choose a package or extension target when the output is an artifact to install or publish
- choose a workspace-configuration target when the repo should manage editor or tool configuration

<MermaidDiagram
  :chart="`
flowchart TD
  Goal[What are you shipping] --> Runtime{Executable plugin}
  Goal --> Package{Install or publish artifact}
  Goal --> Workspace{Repo owned workspace config}
  Runtime --> CodexRuntime[codex-runtime]
  Runtime --> Claude[claude]
  Package --> CodexPackage[codex-package]
  Package --> Gemini[gemini]
  Workspace --> OpenCode[opencode]
  Workspace --> Cursor[cursor]
`"
/>

## Runtime Targets

Use runtime targets when the project owns executable plugin behavior directly.

Examples:

- `codex-runtime`
- `claude`

These are the targets where runtime choice, handler behavior, and strict validation matter the most.

## Package And Extension Targets

Use these when your delivery model is about publishing or installing an artifact rather than running a local plugin from the repo.

Examples:

- `codex-package`
- `gemini`

These targets are about producing the right package or extension artifacts. They do not give you the same executable-plugin contract as the main runtime path.

## Workspace-Configuration Targets

Use these when the repo should manage workspace configuration and integration shape instead of an executable plugin.

Examples:

- `opencode`
- `cursor`

These targets are useful, but they should not be confused with the main runtime path.

## Practical Rule

- choose runtime targets when you need executable plugin behavior
- choose package or extension targets when the product is an artifact to publish or install
- choose workspace-configuration targets when the real goal is repo-owned configuration

## Important Distinction

One project does not have to mean one target forever.

The important boundary is not "one starter name forever". The important boundary is:

- one managed authored project
- clear primary target choices
- honest support expectations for each rendered output

Read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) if you want the public explanation of that broader project shape.

See [Target Support](/en/reference/target-support) for the compact support matrix and [Support Boundary](/en/reference/support-boundary) for the public contract framing.
