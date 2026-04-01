---
title: "Authoring Architecture"
description: "How the project source, render, validation, targets, and handoff fit together in plugin-kit-ai."
canonicalId: "page:concepts:authoring-architecture"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Authoring Architecture

`plugin-kit-ai` is easiest to understand when you stop thinking in terms of hand-edited target files and start thinking in terms of one managed project system.

## The Core Shape

```text
project source -> render -> target outputs -> validate --strict -> handoff
```

<MermaidDiagram
  :chart="`
flowchart LR
  Source[Project source] --> Render[plugin-kit-ai render]
  Render --> Runtime[Runtime outputs]
  Render --> Package[Package or extension outputs]
  Render --> Workspace[Workspace config outputs]
  Runtime --> Validate[validate --strict]
  Package --> Validate
  Workspace --> Validate
  Doctor[doctor and bootstrap when needed] -. supports .-> Validate
  Validate --> Handoff[Handoff to teammate, CI, machine, or downstream user]
`"
/>

This is the core loop behind the public documentation, the generated API, and the supported authoring flows.

## Project Source

The project source lives in the package-standard layout. It is the place where the repo owns intent.

That means:

- the project source is the long-term source of truth
- target files are outputs
- migration exists to bring native config into this model, not to preserve native files as the primary contract

## Render

`render` converts the project source into target-specific artifacts.

You should treat it as part of the normal workflow, not as a convenience helper that runs only at the end.

## Targets

Targets are not all equivalent.

- runtime targets are about executable behavior
- package and extension targets are about delivery artifacts
- workspace-configuration targets are about repo-owned integration shape

That is why target choice changes the operational contract, not just the file output.

## Validation

`validate --strict` is the readiness gate that proves the project source, rendered artifacts, and declared target actually agree.

For Python and Node runtime targets, `doctor` and `bootstrap` often belong next to validation as part of the same practical workflow.

## Handoff

The goal of the system is reliable handoff:

- to another teammate
- to CI
- to another machine
- to a downstream consumer

If the repo only works for the original author, this architecture has failed.

## Practical Consequence

The project is intentionally opinionated because the public goal is not “maximum flexibility at any cost.” The goal is predictable authoring, explicit stability boundaries, and less drift between intent and output.

Pair this page with [Target Model](/en/concepts/target-model), [Authoring Workflow](/en/reference/authoring-workflow), and [Production Readiness](/en/guide/production-readiness).
