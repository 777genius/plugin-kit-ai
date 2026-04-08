---
title: "What You Can Build"
description: "A broad public overview of how one plugin repo can grow into multiple delivery lanes."
canonicalId: "page:guide:what-you-can-build"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# What You Can Build

`plugin-kit-ai` is built around one practical idea: keep one authored repo, start with one recommended path, and expand later only when the product needs more outputs.

<MermaidDiagram
  :chart="`
flowchart TD
  Product[One authored repo] --> Runtime[Runtime lane]
  Product --> Package[Package lane]
  Product --> Extension[Extension lane]
  Product --> Bundle[Bundle handoff]
  Product --> Integration[Repo managed integration lane]
  Product --> Shared[Shared runtime package]
`"
/>

## Recommended Starting Shape

Most teams should start with `Codex runtime Go` as the default path.

Supported Node and Python paths stay visible from day one:

- `Node/TypeScript`
- `Python`

Choosing Node or Python does not force you to decide every packaging or integration detail on day one.

## Expand Later From The Same Repo

Once the first path is healthy, the same repo can grow into:

- Claude outputs when hooks become part of the product
- Codex package outputs when package delivery matters
- Gemini extension packaging when Gemini is a real shipping requirement
- OpenCode and Cursor when the repo should manage integration setup
- portable bundle handoff for supported Python and Node repos

## One Repo, Many Supported Outputs

The real product shape is not "many random targets." It is one authored repo that can produce multiple supported outputs as the delivery model expands.

## Team-Ready Repos

The point is not only scaffolding. The point is ending up with a repo another teammate can understand, validate, and ship.

That means:

- one source of truth under `src/`
- one validation workflow through `generate`, `validate`, and CI
- explicit starting-path choices instead of hand-edited native files
- predictable handoff between authors and downstream users

## Bundle And Shared Runtime Paths

For supported Python and Node lanes, the repo can also produce:

- portable bundle handoff artifacts
- shared helper delivery through `plugin-kit-ai-runtime`

These are delivery choices layered on top of the same authored repo, not separate products.

## What You Can Ship From The Same Repo

`plugin-kit-ai` can cover:

- runtime paths for executable plugin behavior
- package paths for official package artifacts
- extension paths for extension-style delivery
- repo-owned integration setup for config and workspace ownership

That is the real multi-target story: one repo, one workflow, multiple shipping paths over time.
