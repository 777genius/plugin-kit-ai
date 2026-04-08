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
    Build in one repo, start with a recommended production lane, and expand later into packages,
    extensions, and repo-managed integrations without splitting your authoring workflow.
  </p>
</div>

## Recommended Production Lanes

- `Codex runtime Go` for the strongest default runtime lane.
- `Codex package` when the product is an official Codex package.
- `Gemini packaging` when the product is a Gemini extension package.
- `Gemini Go runtime` when you need the promoted 9-hook runtime lane.
- `Claude default lane` when Claude hooks are already the product requirement.

## What To Know Right Away

- one repo remains the source of truth as you add more lanes
- choose the lane that matches your delivery model today
- expand later from the same repo when the product needs more outputs
- use `generate` and `validate --strict` as the shared readiness workflow

<div class="docs-grid">
  <a class="docs-card" href="./guide/quickstart">
    <h2>Start Fast</h2>
    <p>Use the strongest default lane first, then expand only when the product needs more outputs.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>See The Product Shape</h2>
    <p>See how one repo grows into runtime, package, extension, and repo-managed integration lanes.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>Choose A Lane</h2>
    <p>Match the target to your delivery model instead of treating every output like the same product shape.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Check The Exact Contract</h2>
    <p>Use the reference pages when you need the precise support boundary and compatibility terms.</p>
  </a>
</div>

## Start Here

- Start with `go` when you want the strongest runtime and release story.
- Choose `node --typescript` when your team wants the main non-Go runtime lane.
- Choose `python` when the repo is intentionally Python-first and stays local.
- Choose package, extension, and repo-managed integration lanes when those are the real delivery outputs.
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
    <p>Start with one recommended lane before you think about expansion.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>2. What You Can Build</h2>
    <p>See the product shape across runtime, package, extension, and integration lanes.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>3. Choose A Target</h2>
    <p>Choose the lane that matches the delivery model you actually need today.</p>
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
- pick a recommended lane without overlearning the whole target taxonomy
- expand the same repo later into more delivery lanes
- keep one review and validation story as the repo grows
- find the exact contract only when you need it
