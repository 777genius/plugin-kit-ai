---
title: "Target Model"
description: "How plugin-kit-ai divides runtime, package, extension, and repo-managed integration lanes."
canonicalId: "page:concepts:target-model"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Target Model

`plugin-kit-ai` supports several target types because products need different delivery models.

## Quick Rule

- choose a runtime lane when you want executable plugin behavior
- choose a package or extension lane when the output is an installable artifact
- choose a repo-managed integration lane when the repo should own configuration and integration shape

<MermaidDiagram
  :chart="`
flowchart TD
  Goal[What are you shipping] --> Runtime{Executable behavior}
  Goal --> Package{Installable artifact}
  Goal --> Integration{Repo managed integration}
  Runtime --> CodexRuntime[codex-runtime]
  Runtime --> Claude[claude]
  Package --> CodexPackage[codex-package]
  Package --> Gemini[gemini]
  Integration --> OpenCode[opencode]
  Integration --> Cursor[cursor]
`"
/>

## Runtime Lanes

Use these when the project owns executable plugin behavior directly.

Examples:

- `codex-runtime`
- `claude`

## Package And Extension Lanes

Use these when the product is an artifact to install, publish, or ship.

Examples:

- `codex-package`
- `gemini`

## Repo-Managed Integration Lanes

Use these when the repo should own integration config and workspace behavior.

Examples:

- `opencode`
- `cursor`

## Important Distinction

One project does not have to mean one target forever.

The important boundary is:

- one managed authored project
- clear primary lane choices
- honest support expectations for each generated output
