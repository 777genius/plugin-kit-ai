---
title: "API"
description: "Generated API reference for plugin-kit-ai."
canonicalId: "page:api:index"
section: "api"
locale: "en"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">GENERATED REFERENCE</p>
  <h1>API Surfaces</h1>
  <p class="docs-lead">
    This reference is generated from the real CLI, packages, and structured metadata. It is split by
    public area so that API discovery stays predictable as the project grows.
  </p>
</div>

<div class="docs-grid">
  <a class="docs-card" href="./cli/">
    <h2>CLI</h2>
    <p>Commands exported from the live Cobra tree.</p>
  </a>
  <a class="docs-card" href="./go-sdk/">
    <h2>Go SDK</h2>
    <p>Public Go packages for stable integration paths.</p>
  </a>
  <a class="docs-card" href="./runtime-node/">
    <h2>Node Runtime</h2>
    <p>Typed runtime helpers for JS and TS consumers.</p>
  </a>
  <a class="docs-card" href="./runtime-python/">
    <h2>Python Runtime</h2>
    <p>Public Python runtime helpers only, not install wrappers.</p>
  </a>
  <a class="docs-card" href="./platform-events/">
    <h2>Platform Events</h2>
    <p>Event surfaces grouped by target platform.</p>
  </a>
  <a class="docs-card" href="./capabilities/">
    <h2>Capabilities</h2>
    <p>Capability-oriented view across platforms and events.</p>
  </a>
</div>

## Choose In 60 Seconds

- Open `CLI` when you are authoring, validating, bundling, or inspecting a plugin repo.
- Open `Go SDK` when you are building the strongest production-oriented runtime path.
- Open `Node Runtime` or `Python Runtime` when you already chose a supported repo-local interpreted runtime lane and need helper APIs.
- Open `Platform Events` when you already know the target platform and need the event-level contract.
- Open `Capabilities` when you want to compare similar behavior across platforms instead of reading one platform tree at a time.

## Best First Stops

- Need the main user-facing surface: start with [CLI](./cli/).
- Need the strongest production default: start with [Go SDK](./go-sdk/).
- Need interpreted runtime helpers: start with [Node Runtime](./runtime-node/) or [Python Runtime](./runtime-python/).
- Need event-level platform detail: start with [Platform Events](./platform-events/).
- Need a cross-platform behavior map: start with [Capabilities](./capabilities/).

## Choose By Role

<div class="docs-grid">
  <a class="docs-card" href="./cli/">
    <h2>I Author Plugin Repos</h2>
    <p>Start with CLI when you need the real command workflow for init, render, validate, inspect, and bundle operations.</p>
  </a>
  <a class="docs-card" href="./go-sdk/">
    <h2>I Build The Strongest Runtime Path</h2>
    <p>Start with Go SDK when you need the strongest supported runtime contract and the lightest downstream runtime burden.</p>
  </a>
  <a class="docs-card" href="./runtime-node/">
    <h2>I Own A Node Or TypeScript Lane</h2>
    <p>Start with Node Runtime when your repo already chose a supported local Node lane and now needs helper APIs.</p>
  </a>
  <a class="docs-card" href="./runtime-python/">
    <h2>I Own A Python Lane</h2>
    <p>Start with Python Runtime when your repo already chose a supported local Python lane and now needs helper APIs.</p>
  </a>
  <a class="docs-card" href="./platform-events/">
    <h2>I Integrate With One Platform Deeply</h2>
    <p>Start with Platform Events when the main question is event-level behavior for one target platform.</p>
  </a>
  <a class="docs-card" href="./capabilities/">
    <h2>I Compare Behavior Across Platforms</h2>
    <p>Start with Capabilities when you want one cross-platform map instead of reading platform trees one by one.</p>
  </a>
</div>

## Choose By Question

- “Which command do I run next?” Start with [CLI](./cli/).
- “Which packages should my Go plugin import?” Start with [Go SDK](./go-sdk/).
- “Which helper API fits my supported Node or Python lane?” Start with [Node Runtime](./runtime-node/) or [Python Runtime](./runtime-python/).
- “Which platform events exist for this target?” Start with [Platform Events](./platform-events/).
- “Which capability exists across several platforms?” Start with [Capabilities](./capabilities/).

## Open The Right Surface

- Open `CLI` when you need commands, flags, or the authored workflow.
- Open `Go SDK` when you are building the strongest production-oriented runtime path.
- Open `Node Runtime` or `Python Runtime` when you need helper APIs for supported local runtime projects.
- Open `Platform Events` when you are choosing target-specific events.
- Open `Capabilities` when you want a cross-platform view of what a plugin can react to or enforce.

## What This API Section Covers

- the live Cobra command tree
- public Go packages
- shared runtime helper APIs for Node and Python
- platform-specific events
- capability-level cross-platform metadata

## What This API Section Is Not

- It is not the best first entry point if you are still choosing a target, runtime, or starter.
- It does not replace the guides for first-time setup, delivery, or team handoff.
- It is generated reference tied to real source data, so it is best after you already know which surface you need.

## Read This With

- [What You Can Build](/en/guide/what-you-can-build) when you are still deciding whether the repo needs runtime, package, or workspace outputs.
- [Choose A Target](/en/guide/choose-a-target) when you still need the right target family.
- [Support Promise By Path](/en/reference/support-promise-by-path) when the real decision is about support strength and operational cost.
