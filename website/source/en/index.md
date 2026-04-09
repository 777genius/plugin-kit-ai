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
    Build one plugin repo and ship it to many AI agents without learning the whole target model on day one.
  </p>
</div>

## Start By Job

- [Connect an online service](/en/guide/choose-what-you-are-building#connect-an-online-service)
- [Connect a local tool](/en/guide/choose-what-you-are-building#connect-a-local-tool)
- [Build custom plugin logic - Advanced](/en/guide/build-custom-plugin-logic)

## What To Know Right Away

- one repo remains the source of truth as you add more lanes
- choose the starting path that matches the job you need today
- expand later from the same repo when the product needs more outputs
- use `generate` and `validate --strict` as the shared readiness workflow

<div class="docs-grid">
  <a class="docs-card" href="./guide/choose-what-you-are-building">
    <h2>Choose What You Are Building</h2>
    <p>Start with the job first, then learn the deeper target model only when you need it.</p>
  </a>
  <a class="docs-card" href="./guide/quickstart">
    <h2>Start Fast</h2>
    <p>Get a working repo fast from the new job-first entry path.</p>
  </a>
  <a class="docs-card" href="./guide/build-custom-plugin-logic">
    <h2>Advanced Custom Logic</h2>
    <p>Open the guided path for runtime code, hooks, and orchestration when wiring alone is not enough.</p>
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

## Read In This Order

<div class="docs-grid">
  <a class="docs-card" href="./guide/choose-what-you-are-building">
    <h2>1. Choose What You Are Building</h2>
    <p>Pick online service, local tool, or custom logic before you go deeper.</p>
  </a>
  <a class="docs-card" href="./guide/quickstart">
    <h2>2. Quickstart</h2>
    <p>Turn that choice into a working repo and a clean first validation loop.</p>
  </a>
  <a class="docs-card" href="./guide/build-custom-plugin-logic">
    <h2>3. Advanced Custom Logic</h2>
    <p>Use this when the plugin's value lives in your code, hooks, and runtime behavior.</p>
  </a>
  <a class="docs-card" href="./guide/what-you-can-build">
    <h2>4. What You Can Build</h2>
    <p>See the product shape across runtime, package, extension, and integration lanes.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>5. Choose A Target</h2>
    <p>Use this later when you are ready for target-specific shipping decisions.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>6. Support Boundary</h2>
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
