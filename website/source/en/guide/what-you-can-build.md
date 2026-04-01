---
title: "What You Can Build"
description: "A broad public overview of the real product shapes plugin-kit-ai supports."
canonicalId: "page:guide:what-you-can-build"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# What You Can Build

This page is the broad map of the product. Read it when you want to understand the real things `plugin-kit-ai` can produce before choosing a runtime, starter, or target.

<MermaidDiagram
  :chart="`
flowchart TD
  Product[plugin-kit-ai product shapes] --> Runtime[Runtime plugins]
  Product --> Multi[One managed repo with multiple outputs]
  Product --> Bundle[Portable bundle handoff]
  Product --> Shared[Shared runtime package]
  Product --> Package[Package and extension targets]
  Product --> Workspace[Workspace config targets]
`"
/>

## 1. Codex Runtime Plugins

This is the default public path.

Use it when you want:

- the strongest production-oriented starting point
- a managed project model instead of hand-edited target files
- a clear path through `render` and `validate --strict`

You can build Codex runtime plugins in:

- Go for the strongest default production contract
- Node/TypeScript for the mainstream non-Go stable lane
- Python for repo-local Python-first teams

## 2. Claude Hook Plugins

Use the Claude lane when Claude hooks are the actual product requirement.

This is the right choice when:

- you need Claude-specific runtime hooks
- the stable Claude subset is enough for your plugin
- you want a stronger authoring contract than native file editing

## 3. Team-Ready Plugin Repositories

`plugin-kit-ai` is not only about scaffolding. It is also about getting to a repo another teammate can understand, validate, and ship.

That means the system supports:

- strict readiness gates
- CI-friendly flows
- explicit lane and target choices
- predictable handoff between authors and downstream consumers

## 4. One Managed Project That Can Cover More Than One Output

The product is bigger than the starter names suggest.

This is not a side capability. It is one of the central product ideas.

The public starter families are split by the **first** runtime or target path, but the managed project model is broader than that.

That means one project can stay organized as one source of truth while it manages:

- a primary runtime path
- additional package or workspace-config targets
- and, when the product really needs it, more than one agent-facing output family

See [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) for the practical mental model.

## 5. Portable Python And Node Handoff Bundles

For supported Python and Node lanes, you can move beyond local authoring and produce portable bundle handoff artifacts.

This matters when:

- the delivery model needs fetched artifacts instead of a live repo
- you want a cleaner downstream install story for interpreted runtime lanes
- you are using the bundle publish/fetch flow as part of release handoff

See [Bundle Handoff](/en/guide/bundle-handoff) for the actual public flow.

## 6. Shared Runtime Package Flows

Python and Node helper behavior can live either:

- in vendored helper files inside the repo
- in the shared `plugin-kit-ai-runtime` package

This gives teams a supported path for:

- reusable runtime helpers across multiple repos
- cleaner dependency upgrades
- a standardized helper API without copying scaffolded files by hand

## 7. Package, Extension, And Workspace-Config Targets

Not every public shape is a repo-local runtime plugin.

`plugin-kit-ai` also covers:

- packaging-oriented lanes
- extension-style targets
- workspace-config integration targets

These targets matter when the end product is packaging or configuration, not an executable plugin.

See [Package And Workspace Targets](/en/guide/package-and-workspace-targets) before you treat these targets like runtime plugins.

## 8. Generated Public Reference

The docs site also gives you generated reference for:

- the real CLI command tree
- the Go SDK
- Node and Python runtime helpers
- platform events
- capability-level cross-platform views

That is how the public docs stay tied to real source-of-truth data instead of drifting into stale prose.

## Safe Reading Order

If you are still deciding what to do:

1. read this page
2. read [Choosing Runtime](/en/concepts/choosing-runtime)
3. read [Target Model](/en/concepts/target-model)
4. choose a starter or the default `init` path

Pair this page with [Examples And Recipes](/en/guide/examples-and-recipes), [Choose A Starter Repo](/en/guide/choose-a-starter), [Choose Delivery Model](/en/guide/choose-delivery-model), [Bundle Handoff](/en/guide/bundle-handoff), [Package And Workspace Targets](/en/guide/package-and-workspace-targets), and [API Surfaces](/en/api/).
