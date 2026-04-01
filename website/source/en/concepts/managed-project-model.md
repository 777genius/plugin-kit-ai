---
title: "Managed Project Model"
description: "The central plugin-kit-ai idea: one authored project, rendered outputs, strict validation, and deliberate growth across targets."
canonicalId: "page:concepts:managed-project-model"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Managed Project Model

If you want one page that explains what `plugin-kit-ai` actually is, read this page first.

## In One Sentence

`plugin-kit-ai` is a managed plugin project system: keep one authored repo, render the outputs each target needs, validate the result, and grow deliberately without turning the repo into ad-hoc glue.

## Quick Mental Reset

| What you notice first | Wrong conclusion | Correct reading |
| --- | --- | --- |
| Starter names like Codex or Claude | The repo is permanently locked to one agent family | Starter names only optimize the first correct path |
| A visible CLI workflow | The product is mainly a generator tool | The CLI is the reproducible workflow surface for the managed project |
| Runtime, package, and workspace targets | Everything has the same operational contract | Outputs differ on purpose and carry explicit support boundaries |

## The Model In Four Parts

1. **One authored project**
   The package-standard project is the long-term source of truth.
2. **Rendered target outputs**
   Runtime, package, extension, and workspace-config artifacts are produced from that source of truth.
3. **Strict readiness gates**
   `render`, `validate --strict`, and related checks prove that intent and output still agree.
4. **Deliberate expansion**
   The repo can grow to more outputs and targets without pretending every surface has the same maturity.

## What People Mistake It For

People often mistake `plugin-kit-ai` for one of these:

- a starter collection
- a CLI that writes files once
- a runtime helper package
- a target matrix with no unifying model

Those are all real parts of the ecosystem, but none of them is the product definition.

## What Actually Stays Unified

The thing that stays unified is the managed project model itself:

- one repo layout
- one authored source of truth
- one reproducible workflow
- one validation story
- one handoff story for teammates and CI

## What Can Vary

These parts can vary without breaking the model:

- the first starter you choose
- the runtime you choose first
- the targets you render
- whether Python or Node use vendored helpers or `plugin-kit-ai-runtime`
- which stable and beta surfaces your team is willing to adopt

## Product Promise

The promise is not “every target behaves the same.”

The promise is:

- one managed project instead of hand-maintained target files
- one system that makes rendered outputs reproducible
- one public support boundary that tells you what is stable and what is not

## What This Means In Practice

- Start with the narrowest real requirement.
- Keep the repo in the managed project model.
- Add outputs only when the product actually needs them.
- Split repos for ownership or release reasons, not because starter names look specific.

## Read This Next

- [Why plugin-kit-ai](/en/concepts/why-plugin-kit-ai)
- [One Project, Multiple Targets](/en/guide/one-project-multiple-targets)
- [Target Model](/en/concepts/target-model)
- [Authoring Workflow](/en/reference/authoring-workflow)
