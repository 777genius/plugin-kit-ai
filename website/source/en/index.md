---
title: "plugin-kit-ai Documentation"
description: "Public documentation for plugin-kit-ai."
canonicalId: "page:home"
section: "home"
locale: "en"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--feature">
  <p class="docs-kicker">PUBLIC DOCUMENTATION</p>
  <h1>plugin-kit-ai</h1>
  <p class="docs-lead">
    Keep one managed plugin project, render the outputs each agent or target actually needs,
    and avoid turning your repo into a pile of one-off templates and fragile glue scripts.
  </p>
</div>

## Core Idea

- one authored project instead of hand-maintained target files everywhere
- one managed workflow through `render`, `validate`, and CI
- multiple supported output shapes across runtime, package, extension, and workspace-config targets
- honest support boundaries instead of fake parity claims

<div class="docs-grid">
  <a class="docs-card" href="./guide/">
    <h2>Start Fast</h2>
    <p>Install the CLI, understand the supported paths, and get to the first working plugin quickly.</p>
  </a>
  <a class="docs-card" href="./reference/">
    <h2>Reference</h2>
    <p>Use the public reference for install channels, target support, and contracts that should stay stable.</p>
  </a>
  <a class="docs-card" href="./api/">
    <h2>Generated API</h2>
    <p>Browse the live CLI, Go SDK, Node runtime, Python runtime, platform events, and capabilities.</p>
  </a>
  <a class="docs-card" href="./releases/">
    <h2>Release Notes</h2>
    <p>Track user-facing changes, migration notes, and the breaking-change boundary as the project evolves.</p>
  </a>
</div>

## Recommended Starting Points

- Start with `go` when you want the strongest production path and the fewest moving parts.
- Choose `node --typescript` when you want a supported JavaScript or TypeScript path inside the repo.
- Treat npm and PyPI `plugin-kit-ai` packages as ways to install the CLI, not as runtime libraries.
- Use `validate --strict` as the final readiness check before you hand the repo to another person or machine.

## Find Your Scenario

- New plugin author: start with [Installation](/en/guide/installation), [Quickstart](/en/guide/quickstart), and [Build Your First Plugin](/en/guide/first-plugin).
- Team lead or maintainer: start with [Build A Team-Ready Plugin](/en/guide/team-ready-plugin), [Production Readiness](/en/guide/production-readiness), and [CI Integration](/en/guide/ci-integration).
- Python or Node team: start with [Choose Delivery Model](/en/guide/choose-delivery-model), [Bundle Handoff](/en/guide/bundle-handoff), and [v1.0.6](/en/releases/v1-0-6).
- Packaging or workspace config: start with [Choose A Target](/en/guide/choose-a-target), [Package And Workspace Targets](/en/guide/package-and-workspace-targets), and [Target Support](/en/reference/target-support).

## Who This Site Helps

- Individual plugin authors who want a reliable first setup.
- Teams that need a repo another person can validate and ship.
- Python and Node teams that need a supported delivery story, not just a local scaffold.
- Integrators who need the exact public API, target support, and release boundary.

## Choose Your Path

<div class="docs-grid">
  <a class="docs-card" href="./guide/first-plugin">
    <h2>First Production Plugin</h2>
    <p>Follow the narrowest recommended path from scaffold to a strict validation gate.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>See Real Product Shapes</h2>
    <p>Understand the actual things you can build with plugin-kit-ai before you commit to a lane or starter.</p>
  </a>
  <a class="docs-card" href="./concepts/why-plugin-kit-ai">
    <h2>Why This Exists</h2>
    <p>Understand the problem this project solves, the users it fits, and the tradeoffs it makes on purpose.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Know The Boundary</h2>
    <p>See what is stable, what is beta, and what the project intentionally does not promise yet.</p>
  </a>
</div>

## Latest Stable Release

- The current public baseline in this docs set is [`v1.0.6`](/en/releases/v1-0-6).
- That release made shared runtime-package delivery for Python and Node a real supported path instead of a partial story.
- Start there if you need the latest user-facing migration notes.

## What You Can Do With It

- Build Codex runtime plugins and Claude hook plugins from one managed project model.
- Use Go for the strongest production path or Python and Node for supported local runtime projects.
- Ship portable Python and Node bundles when the delivery model needs downloadable artifacts instead of a live repo.
- Reuse helper behavior through `plugin-kit-ai-runtime` when a shared runtime package fits better than copied helper files.
- Work across runtime, package, extension, and workspace-configuration targets with clear support boundaries.

## What This Site Covers

- Public guides for users and plugin authors.
- Generated API reference from the actual code and command tree.
- Public support and platform metadata.
- User-facing releases and migration notes.

## What Stays Out

- Internal release rehearsal material.
- Maintainer-only audit notes and operational checklists.
- Wrapper-package internals treated as API.
