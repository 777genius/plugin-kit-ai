---
title: "Production Readiness"
description: "A public checklist for deciding whether a plugin-kit-ai project is ready for CI, handoff, and community-facing use."
canonicalId: "page:guide:production-readiness"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Production Readiness

Use this checklist before you call a project production-ready, handoff-ready, or stable enough to share broadly.

<MermaidDiagram
  :chart="`
flowchart LR
  Path[Path chosen on purpose] --> Source[One source of truth]
  Source --> Checks[render and validate gates]
  Checks --> Boundary[Support boundary confirmed]
  Boundary --> Handoff[Docs and handoff are explicit]
  Handoff --> Ready[Production ready]
`"
/>

## 1. Pick The Right Path On Purpose

- default to Go when you want the strongest production contract
- choose Node/TypeScript only when the non-Go local runtime tradeoff is real
- choose Python only when the project stays local to the repo and the team is intentionally Python-first
- do not treat workspace-configuration or packaging targets as if they carried the same runtime guarantees

## 2. Keep One Source Of Truth

- keep the project source in the package-standard project layout
- treat generated target files as outputs, not the long-term source of truth
- do not patch rendered files by hand and then expect `render` to preserve those edits

## 3. Run The Contract Gates

At minimum, the repo should survive this flow cleanly:

```bash
plugin-kit-ai doctor .
plugin-kit-ai render .
plugin-kit-ai validate . --platform <target> --strict
```

For Python and Node runtime paths, `doctor` and `bootstrap` are part of readiness, not optional polish.

If the repo supports multiple targets, this gate should be repeated explicitly for each target in the declared support scope.

## 4. Verify The Support Boundary

- confirm that the primary target, and each additional target in scope, is actually inside the public support boundary
- confirm whether the path is stable, beta, or intentionally narrower than the main path
- check the generated target support matrix before you promise compatibility to downstream users

## 5. Keep Install Story And API Story Separate

- Homebrew, npm, and PyPI packages are install channels for the CLI
- they are not runtime APIs or SDK surfaces
- public API lives in the generated API section and in the documented stable workflows

## 6. Document The Handoff

A public-facing repo should make these things obvious:

- which target is primary and which additional targets are genuinely supported
- which runtime it uses and whether that changes by target
- how to install prerequisites
- which command, or set of commands, is the canonical validation gate
- whether it depends on a shared runtime package or a Go SDK path

## 7. Link To Current Release Notes

If the repo depends on the latest stable path, point users to the latest release note that explains the current default path and migration story.

Today, that baseline is [v1.0.6](/en/releases/v1-0-6).

## Final Rule

If a teammate cannot clone the repo, run the documented flow, pass `validate --strict`, and understand the chosen path without tribal knowledge, the project is not yet production-ready.

Pair this page with [Support Boundary](/en/reference/support-boundary), [Target Support](/en/reference/target-support), and [Authoring Workflow](/en/reference/authoring-workflow).
