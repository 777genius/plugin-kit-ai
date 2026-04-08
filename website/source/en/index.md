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
    Build in one repo, start with Go by default, and later add packages, Claude hooks, Gemini,
    or repo-owned integration setup without splitting the project.
  </p>
</div>

## Default Start

- `Codex runtime Go` is the default start when you want the strongest runtime and release story.

## What To Know Right Away

- one repo remains the source of truth as you add more lanes
- choose the starting path that matches what you need today
- expand later from the same repo when the product needs more outputs
- use `generate` and `validate --strict` as the shared readiness workflow

## Supported Node And Python Paths

- `codex-runtime --runtime node --typescript` is the main supported non-Go path.
- `codex-runtime --runtime python` is the supported Python-first path.
- both are local interpreted runtime paths, so the target machine still needs Node.js `20+` or Python `3.10+`.
- they are clear early options for teams already working in those stacks, but they are not the default start.

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>Start Fast</h2>
    <p>Use the strongest default path first, then expand only when the product needs more outputs.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>See The Product Shape</h2>
    <p>See how one repo grows into runtime, package, extension, and repo-owned integration setup.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>Choose A Target</h2>
    <p>Match the target to how you want to ship the plugin instead of treating every output like the same thing.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Check The Exact Contract</h2>
    <p>Use the reference pages when you need the precise support boundary and compatibility terms.</p>
  </a>
</div>

## If You Need More Later

- Add `Claude default lane` when Claude hooks are the product requirement.
- Add `Codex package` or `Gemini packaging` when the product is a package or extension output.
- Add `OpenCode` or `Cursor` when the repo should own integration setup.
- Use `validate --strict` as the readiness gate before handoff or CI.

## Common Expansion Paths

- Start with a Codex runtime repo, then add Codex package or Gemini when packaging becomes part of the product.
- Start with Claude when Claude hooks are the product, then keep the repo open for broader delivery lanes later.
- Start on Node or Python locally, then add bundle handoff when downstream delivery matters.
- Add OpenCode or Cursor when the repo should manage integration config, not just executable behavior.

## Read In This Order

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>1. Quickstart</h2>
    <p>Start with one recommended path before you think about expansion.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>2. What You Can Build</h2>
    <p>See the product shape across runtime, package, extension, and integration lanes.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>3. Choose A Target</h2>
    <p>Choose the target that matches how you actually want to ship the plugin.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>4. Support Boundary</h2>
    <p>Use the reference cluster when you need exact compatibility language and support details.</p>
  </a>
</div>

If you are new, stop after those four pages. Everything else is deeper reference or implementation detail.

## Current Repo Baseline

- The current public baseline in this docs set is [`v1.0.6`](/en/releases/v1-0-6).
- That release made shared runtime-package delivery for Python and Node a fully supported story.
- Start there when you want the current recommended baseline.

## What This Site Helps You Do

- start one plugin repo instead of splitting source of truth by ecosystem
- pick a recommended starting path without learning every target detail up front
- expand the same repo later into more shipping paths
- keep one review and validation story as the repo grows
- find the exact contract only when you need it
