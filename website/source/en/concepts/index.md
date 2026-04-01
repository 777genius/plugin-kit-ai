---
title: "Concepts"
description: "Core concepts behind plugin-kit-ai."
canonicalId: "page:concepts:index"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
aside: false
outline: false
---

<div class="docs-hero docs-hero--compact">
  <p class="docs-kicker">CONCEPTS</p>
  <h1>Mental Model</h1>
  <p class="docs-lead">
    Public concepts explain the product model, support tiers, and target classes without pulling maintainer-only release mechanics into the user docs.
  </p>
</div>

## The Main Idea

The project is easiest to understand when you treat it as a managed plugin project system, not as a collection of starters, commands, or target files.

## Core Ideas

- Public docs describe supported user-facing behavior, not internal process.
- API reference is generated from the real sources of truth.
- Install wrappers are distribution channels, not programmatic API surfaces.
- Stability and maturity matter as much as signatures.

## Read These In Order

- Start with [Why plugin-kit-ai](/en/concepts/why-plugin-kit-ai) if you are still deciding whether this project fits your team.
- Read [Choosing Runtime](/en/concepts/choosing-runtime) before choosing Go, Python, Node, or shell.
- Read [Target Model](/en/concepts/target-model) before assuming every target behaves like a runtime plugin.
- Read [Stability Model](/en/concepts/stability-model) before you promise long-term compatibility to other users.

<div class="docs-grid">
  <a class="docs-card" href="./why-plugin-kit-ai">
    <h2>Why plugin-kit-ai</h2>
    <p>Understand the problem this project solves and when it is the wrong tool.</p>
  </a>
  <a class="docs-card" href="./authoring-architecture">
    <h2>Authoring Architecture</h2>
    <p>See how the project source, generated files, validation, targets, and handoff fit together as one system.</p>
  </a>
  <a class="docs-card" href="./stability-model">
    <h2>Stability Model</h2>
    <p>Understand what public-stable, beta, and experimental mean before you commit to a surface.</p>
  </a>
  <a class="docs-card" href="./target-model">
    <h2>Target Model</h2>
    <p>See the practical difference between runtime, package, extension, and workspace-configuration targets.</p>
  </a>
  <a class="docs-card" href="./choosing-runtime">
    <h2>Choosing Runtime</h2>
    <p>Decide between Go, Python, Node, and shell based on operational reality, not preference alone.</p>
  </a>
</div>
