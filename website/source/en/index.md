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

## In One Sentence

`plugin-kit-ai` is a managed plugin project system: author one repo, render the outputs each target needs, validate the result, and hand off something another person or CI can trust.

If you want the one page that explains the product clearly, read [Managed Project Model](/en/concepts/managed-project-model).

## Three Product Layers

<div class="docs-grid docs-grid--layers">
  <div class="docs-card docs-card--static">
    <h2>1. Project Model</h2>
    <p>One authored repo stays the source of truth. This is the actual product core.</p>
  </div>
  <div class="docs-card docs-card--static">
    <h2>2. Workflow Surface</h2>
    <p>`init`, `render`, `validate`, CI, and generated API expose a reproducible workflow around that model.</p>
  </div>
  <div class="docs-card docs-card--static">
    <h2>3. Output Shapes</h2>
    <p>Runtime, package, extension, and workspace-config targets are the outputs the managed project can produce.</p>
  </div>
</div>

## At A Glance

- Starter names define the first correct path, not the long-term limit of the repo.
- Runtime, package, extension, and workspace-config targets are output shapes, not identical promises.
- The CLI and generated API expose the workflow; the product itself is the managed repo model behind them.

## Without vs With

<div class="docs-grid docs-grid--layers">
  <div class="docs-card docs-card--static">
    <h2>Without plugin-kit-ai</h2>
    <p>Teams hand-edit target files, duplicate helper code, explain the workflow in chat, and let repo drift build up over time.</p>
  </div>
  <div class="docs-card docs-card--static">
    <h2>With plugin-kit-ai</h2>
    <p>Teams keep one authored repo, render outputs on purpose, validate the result, and hand off something reproducible to CI and other humans.</p>
  </div>
</div>

## System Map

<div class="docs-flow" aria-label="plugin-kit-ai system map">
  <div class="docs-flow__step">
    <strong>Start From A Real Need</strong>
    <span>Use a starter or migrate an existing repo when you already know the first target or runtime you need.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Keep One Managed Project</strong>
    <span>Treat the package-standard project as the authored source of truth instead of hand-maintaining target files.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Render And Validate</strong>
    <span>Generate only the outputs you need, then prove they agree with the project through strict validation.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Grow Deliberately</strong>
    <span>Add runtime, package, extension, or workspace-config outputs without turning the repo into ad-hoc glue.</span>
  </div>
</div>

<div class="docs-grid">
  <a class="docs-card" href="./concepts/managed-project-model">
    <h2>Understand The Core Model</h2>
    <p>Read the shortest accurate definition of the product before you choose a runtime, starter, or target.</p>
  </a>
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

## What Counts As A “Plugin” Here

- a runtime plugin when the repo owns executable behavior
- a package or extension output when the repo renders installable artifacts
- a workspace-config output when the repo owns integration or editor configuration

The managed project model stays the same even when the output shape changes.

## Start Here By Scenario

- New plugin author: start with [Installation](/en/guide/installation), [Quickstart](/en/guide/quickstart), and [Build Your First Plugin](/en/guide/first-plugin).
- Team lead or maintainer: start with [Team Adoption](/en/guide/team-adoption), [Production Readiness](/en/guide/production-readiness), and [CI Integration](/en/guide/ci-integration).
- Python or Node team: start with [Choose Delivery Model](/en/guide/choose-delivery-model), [Bundle Handoff](/en/guide/bundle-handoff), and [v1.0.6](/en/releases/v1-0-6).
- Packaging or workspace config: start with [Choose A Target](/en/guide/choose-a-target), [Package And Workspace Targets](/en/guide/package-and-workspace-targets), and [Target Support](/en/reference/target-support).

## Find Your Role

<div class="docs-grid">
  <a class="docs-card" href="./guide/first-plugin">
    <h2>I Am A New Plugin Author</h2>
    <p>Start with the shortest supported path from installation to a validated first plugin.</p>
  </a>
  <a class="docs-card" href="./guide/team-adoption">
    <h2>I Lead A Team</h2>
    <p>Start with the team adoption path: repo standards, readiness gates, CI, release guidance, and safe rollout.</p>
  </a>
  <a class="docs-card" href="./guide/choose-delivery-model">
    <h2>I Own A Python Or Node Lane</h2>
    <p>Choose between vendored helpers, shared runtime packages, and portable bundle handoff.</p>
  </a>
  <a class="docs-card" href="./guide/package-and-workspace-targets">
    <h2>I Need Package Or Workspace Outputs</h2>
    <p>Start with package, extension, and workspace-config targets instead of forcing everything into runtime lanes.</p>
  </a>
</div>

## Choose By Job To Be Done

<div class="docs-grid">
  <a class="docs-card" href="./guide/choose-a-target">
    <h2>Pick The Right Target</h2>
    <p>Choose between runtime, package, extension, and workspace-config outputs without mixing their contracts.</p>
  </a>
  <a class="docs-card" href="./guide/choose-a-starter">
    <h2>Pick The Right Starter</h2>
    <p>Choose the best starter repo for the first real requirement, not for every future possibility.</p>
  </a>
  <a class="docs-card" href="./guide/examples-and-recipes">
    <h2>Open A Real Example</h2>
    <p>Jump straight into production examples, starter repos, local runtime references, and supporting skills.</p>
  </a>
  <a class="docs-card" href="./reference/support-boundary">
    <h2>Check The Safety Boundary</h2>
    <p>See what is stable, what is beta, and where package or workspace outputs stop behaving like runtime paths.</p>
  </a>
  <a class="docs-card" href="./reference/support-promise-by-path">
    <h2>Compare Support By Path</h2>
    <p>See how Go, Node, Python, shell, package, and workspace-config paths differ in promise strength and operational cost.</p>
  </a>
  <a class="docs-card" href="./reference/get-help-and-contribute">
    <h2>Get Help Or Contribute</h2>
    <p>Find the public path for issues, docs feedback, pull requests, and responsible security reporting.</p>
  </a>
</div>

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

## Current Public Baseline

- The current public baseline in this docs set is [`v1.0.6`](/en/releases/v1-0-6).
- That release made shared runtime-package delivery for Python and Node a real supported path instead of a partial story.
- Start there if you need the latest user-facing migration notes.
- Read [Version And Compatibility Policy](/en/reference/version-and-compatibility) if your team needs the compact rule for stable, beta, wrappers, and release baselines.

## What This Site Covers

- Public guides for users and plugin authors.
- Generated API reference from the actual code and command tree.
- Public support and platform metadata.
- User-facing releases and migration notes.

## What Stays Out

- Internal release rehearsal material.
- Maintainer-only audit notes and operational checklists.
- Wrapper-package internals treated as API.
