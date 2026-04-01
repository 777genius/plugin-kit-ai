---
title: "Guide"
description: "Start here for public plugin-kit-ai guides."
canonicalId: "page:guide:index"
section: "guide"
locale: "en"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">GUIDE</p>
  <h1>Start Here</h1>
  <p class="docs-lead">
    Use the guide section when you need the shortest path to a correct setup, not a deep tour of internals.
  </p>
</div>

## If You Remember One Thing

Start with the starter or target that matches your first real requirement, but keep thinking in terms of one managed project that can render more than one output shape over time.

If the project still feels fuzzy, read [Managed Project Model](/en/concepts/managed-project-model) before choosing a path.

## Read The Guide Like This

<div class="docs-flow" aria-label="How to read the guide">
  <div class="docs-flow__step">
    <strong>Understand The Product</strong>
    <span>Read <a href="./what-you-can-build">What You Can Build</a> and <a href="./one-project-multiple-targets">One Project, Multiple Targets</a>.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Choose The First Path</strong>
    <span>Pick the target, runtime, or starter that matches the first real requirement instead of optimizing for every future case.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Build And Validate</strong>
    <span>Use the narrowest supported tutorial path, then prove the repo with <code>validate --strict</code>.</span>
  </div>
  <div class="docs-flow__arrow" aria-hidden="true">→</div>
  <div class="docs-flow__step">
    <strong>Expand Only When Needed</strong>
    <span>Add delivery flows, more targets, and CI once the core managed project is already healthy.</span>
  </div>
</div>

## Common Journeys

- New here: read [Installation](/en/guide/installation), then [Quickstart](/en/guide/quickstart), then [Build Your First Plugin](/en/guide/first-plugin).
- Choosing a path: read [What You Can Build](/en/guide/what-you-can-build), [One Project, Multiple Targets](/en/guide/one-project-multiple-targets), [Choosing Runtime](/en/concepts/choosing-runtime), and [Package And Workspace Targets](/en/guide/package-and-workspace-targets).
- Team adoption: read [Team Adoption](/en/guide/team-adoption), [Production Readiness](/en/guide/production-readiness), and [CI Integration](/en/guide/ci-integration).
- Team-scale upgrades or migrations: read [Team-Scale Rollout](/en/guide/team-scale-rollout), [Upgrade And Migration Playbook](/en/guide/upgrade-and-migration-playbook), [Releases](/en/releases/), and [Migrate Existing Native Config](/en/guide/migrate-existing-config).
- Python or Node delivery: read [Choose Delivery Model](/en/guide/choose-delivery-model) and [Bundle Handoff](/en/guide/bundle-handoff).

## Choose By Role

- New plugin author: go to [Quickstart](/en/guide/quickstart), [Build Your First Plugin](/en/guide/first-plugin), and [Examples And Recipes](/en/guide/examples-and-recipes).
- Team lead or maintainer: go to [Team Adoption](/en/guide/team-adoption), [Production Readiness](/en/guide/production-readiness), and [CI Integration](/en/guide/ci-integration).
- Repo owner planning coordinated rollout: go to [Team-Scale Rollout](/en/guide/team-scale-rollout), [Upgrade And Migration Playbook](/en/guide/upgrade-and-migration-playbook), and [Version And Compatibility Policy](/en/reference/version-and-compatibility).
- Repo owner planning upgrades: go to [Upgrade And Migration Playbook](/en/guide/upgrade-and-migration-playbook), [Releases](/en/releases/), and [Migrate Existing Native Config](/en/guide/migrate-existing-config).
- Python or Node owner: go to [Choose Delivery Model](/en/guide/choose-delivery-model), [Bundle Handoff](/en/guide/bundle-handoff), and [Node/TypeScript Runtime](/en/guide/node-typescript-runtime).
- Packaging or workspace-config owner: go to [Choose A Target](/en/guide/choose-a-target), [Package And Workspace Targets](/en/guide/package-and-workspace-targets), and [Target Support](/en/reference/target-support).

## Choose By Immediate Job

- Need the first working plugin fast: [Quickstart](/en/guide/quickstart)
- Need the right starter or target first: [Choose A Starter Repo](/en/guide/choose-a-starter) and [Choose A Target](/en/guide/choose-a-target)
- Need a real example before deciding: [Examples And Recipes](/en/guide/examples-and-recipes)
- Need a safe production path: [Production Readiness](/en/guide/production-readiness)

<div class="docs-grid">
  <a class="docs-card" href="./quickstart">
    <h2>Quickstart</h2>
    <p>Use the shortest supported path to get from install to a validated plugin repo.</p>
  </a>
  <a class="docs-card" href="./installation">
    <h2>Installation</h2>
    <p>Choose the right install channel and understand what is public API versus a wrapper distribution path.</p>
  </a>
  <a class="docs-card" href="./what-you-can-build">
    <h2>What You Can Build</h2>
    <p>Scan the real product shapes: Codex runtime plugins, Claude hook plugins, bundles, shared runtime helpers, and packaging lanes.</p>
  </a>
  <a class="docs-card" href="./one-project-multiple-targets">
    <h2>One Project, Multiple Targets</h2>
    <p>Understand the key product idea: starters are entrypoints, while the managed project model can support more than one output family.</p>
  </a>
  <a class="docs-card" href="./choose-a-target">
    <h2>Choose A Target</h2>
    <p>Decide between Codex runtime, Claude, Codex package, Gemini, OpenCode, and Cursor without piecing it together from multiple pages.</p>
  </a>
  <a class="docs-card" href="./first-plugin">
    <h2>Build Your First Plugin</h2>
    <p>Follow the narrowest supported path from scaffold to `validate --strict`.</p>
  </a>
  <a class="docs-card" href="./team-adoption">
    <h2>Team Adoption</h2>
    <p>Use the public path for rolling plugin-kit-ai out across a team without relying on tribal knowledge.</p>
  </a>
  <a class="docs-card" href="./upgrade-and-migration-playbook">
    <h2>Upgrade And Migration Playbook</h2>
    <p>Use the safe public path for adopting new defaults, releases, and the managed project model across existing repos.</p>
  </a>
  <a class="docs-card" href="./team-scale-rollout">
    <h2>Team-Scale Rollout</h2>
    <p>Roll new defaults, release guidance, and support decisions across several repos without letting drift or folklore become the standard.</p>
  </a>
  <a class="docs-card" href="./team-ready-plugin">
    <h2>Build A Team-Ready Plugin</h2>
    <p>Go beyond the first green run and make the repo ready for CI, handoff, and broader team adoption.</p>
  </a>
  <a class="docs-card" href="./claude-plugin">
    <h2>Build A Claude Plugin</h2>
    <p>Use the stable Claude path when you are targeting hooks instead of the default Codex runtime lane.</p>
  </a>
  <a class="docs-card" href="./node-typescript-runtime">
    <h2>Node/TypeScript Runtime</h2>
    <p>Choose the mainstream non-Go stable lane for repo-local runtime plugins.</p>
  </a>
  <a class="docs-card" href="./starter-templates">
    <h2>Starter Templates</h2>
    <p>Clone an official starter when you want a known-good layout for Claude or Codex lanes.</p>
  </a>
  <a class="docs-card" href="./examples-and-recipes">
    <h2>Examples And Recipes</h2>
    <p>See real plugin repos, starter repos, local runtime references, and skill examples without digging through the repository tree.</p>
  </a>
  <a class="docs-card" href="./choose-a-starter">
    <h2>Choose A Starter Repo</h2>
    <p>Use the practical matrix for picking the right starter by platform, runtime, and handoff model.</p>
  </a>
  <a class="docs-card" href="./choose-delivery-model">
    <h2>Choose Delivery Model</h2>
    <p>Decide between vendored helpers and the shared runtime package for Python and Node lanes.</p>
  </a>
  <a class="docs-card" href="./bundle-handoff">
    <h2>Bundle Handoff</h2>
    <p>Use export, local install, remote fetch, and GitHub Releases publish when Python or Node plugins must travel as portable artifacts.</p>
  </a>
  <a class="docs-card" href="./package-and-workspace-targets">
    <h2>Package And Workspace Targets</h2>
    <p>Understand Codex package, Gemini, OpenCode, and Cursor targets before you treat them like runtime lanes.</p>
  </a>
  <a class="docs-card" href="./migrate-existing-config">
    <h2>Migrate Existing Config</h2>
    <p>Move from hand-managed native target files into the package-standard authored model.</p>
  </a>
  <a class="docs-card" href="./production-readiness">
    <h2>Production Readiness</h2>
    <p>Use the public checklist before you present a plugin repo as stable, handoff-ready, or CI-grade.</p>
  </a>
  <a class="docs-card" href="./ci-integration">
    <h2>CI Integration</h2>
    <p>Turn the public authored flow into a predictable CI gate that catches drift before handoff.</p>
  </a>
</div>
