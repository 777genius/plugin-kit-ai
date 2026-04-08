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

`plugin-kit-ai` is built around one practical idea: keep one authored repo, start with one recommended lane, and expand later into more outputs only when the product needs them.

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

Most teams should start with one of these lanes:

- `Codex runtime Go`
- `Codex package`
- `Gemini packaging`
- `Claude default lane`

If your runtime stack is already fixed, you can also start on:

- `Node/TypeScript`
- `Python`

## Expand Later From The Same Repo

Once the first lane is healthy, the same repo can grow into:

- Claude outputs when hooks become part of the product
- Codex package outputs when package delivery matters
- Gemini extension packaging when Gemini is a real delivery lane
- OpenCode and Cursor when the repo should manage integration config
- portable bundle handoff for supported Python and Node repos

## One Repo, Many Supported Outputs

The real product shape is not "many random targets." It is one authored repo that can produce multiple supported outputs as the delivery model expands.

## Team-Ready Repos

The point is not only scaffolding. The point is ending up with a repo another teammate can understand, validate, and ship.

That means:

- one source of truth under `src/`
- one validation workflow through `generate`, `validate`, and CI
- explicit lane choices instead of hand-edited native files
- predictable handoff between authors and downstream users

## Bundle And Shared Runtime Paths

For supported Python and Node lanes, the repo can also produce:

- portable bundle handoff artifacts
- shared helper delivery through `plugin-kit-ai-runtime`

These are delivery choices layered on top of the same authored repo, not separate products.

## Delivery Models Covered Here

`plugin-kit-ai` can cover:

- runtime lanes for executable plugin behavior
- package lanes for official package artifacts
- extension lanes for extension-style delivery
- repo-managed integration lanes for config and workspace ownership

That is the real multi-target story: one repo, one workflow, multiple delivery lanes over time.
