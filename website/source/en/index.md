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
    Build your plugin once and easily export it to any AI agent, like Claude, Codex,
    or Gemini, without duplicating code.
  </p>
</div>

## One Repo, Many Supported Targets

- Start with one plugin repo, not a separate repo for each ecosystem.
- Add supported Claude, Codex, Gemini, and config/package outputs from that same repo as the product grows.
- Keep one validation workflow through `render`, `validate`, and CI.
- Avoid turning your setup into a pile of one-off templates and fragile glue scripts.

## What To Know Right Away

- The repo and workflow stay unified across targets.
- Support depth depends on the target you add.
- Runtime plugins, package outputs, and workspace-managed config do not all carry the same guarantees.
- The safe promise is one repo with many supported outputs, not fake parity everywhere.

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>Start Fast</h2>
    <p>Install the CLI, pick Go, Node/TypeScript, or Python, and get the first working plugin repo quickly.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>See Multi-Target Use Cases</h2>
    <p>See how one repo can expand into Claude, Codex, Gemini, bundle delivery, and workspace/config outputs.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-starter">
    <h2>Choose A Starter</h2>
    <p>Use a starter as the first step, not as the final limit of the product.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Check The Boundary</h2>
    <p>See which targets are strongest today and where support depth changes.</p>
  </a>
</div>

## Start Here

- Start with `go` when you want the strongest production path and the fewest moving parts.
- Choose `node --typescript` when you want the main supported non-Go path.
- Choose `python` when the repo is intentionally Python-first.
- Choose Claude first only when Claude hooks are already the real product requirement.
- Use `validate --strict` as the readiness gate before handoff or CI.

## Common Expansion Paths

- Start with a Codex runtime plugin, then add package/config outputs as needed.
- Start with Claude hooks, then keep the repo open for broader target coverage later.
- Start on Node or Python locally, then add portable bundle delivery when handoff matters.
- Keep deeper target decisions explicit instead of pretending every output behaves the same way.

## What This Site Helps You Do

- start one plugin repo instead of splitting by ecosystem too early
- expand the same repo to multiple supported outputs over time
- keep one review and validation story as the repo grows
- understand where support is strongest and where it is narrower

## Read In This Order

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>1. Quickstart</h2>
    <p>Start one repo on the strongest default path before you worry about expansion.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>2. What You Can Build</h2>
    <p>See how the same repo can cover multiple supported outputs.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-starter">
    <h2>3. Choose A Starter</h2>
    <p>Pick the best entrypoint without treating it like a permanent boundary.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>4. Support Boundary</h2>
    <p>Check where support depth changes before you promise the same thing everywhere.</p>
  </a>
</div>

If you are new, stop after those four pages. Everything else on this site is deeper reference.

## Latest Stable Release

- The current public baseline in this docs set is [`v1.0.6`](/en/releases/v1-0-6).
- That release made shared runtime-package delivery for Python and Node a real supported path instead of a partial story.
- Start there if you need the latest user-facing migration notes.

## What You Can Do With It

- Build from one repo and expand into multiple supported outputs as the product grows.
- Start on the strongest Go path or use supported local Python and Node paths.
- Add Claude, Codex, Gemini, bundle, and workspace/config outputs from the same managed workflow.
- Reuse helper behavior through `plugin-kit-ai-runtime` when a shared runtime package fits better than copied helper files.
- Keep support boundaries explicit instead of assuming identical runtime parity across targets.

## What This Site Covers

- Public guides for users and plugin authors.
- Generated API reference from the actual code and command tree.
- Public support and platform metadata.
- User-facing releases and migration notes.
- Public policy pages for versioning, compatibility, and support expectations.

## What Stays Out

- Internal release rehearsal material.
- Maintainer-only audit notes and operational checklists.
- Wrapper-package internals treated as API.
