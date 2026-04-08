---
title: "Stability Model"
description: "How plugin-kit-ai classifies public-stable, public-beta, and public-experimental areas."
canonicalId: "page:concepts:stability-model"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Stability Model

`plugin-kit-ai` uses formal contract terms so teams can decide exactly what to standardize on.

<MermaidDiagram
  :chart="`
flowchart TD
  Stable[public stable] --> Beta[public beta]
  Beta --> Experimental[public experimental]
  StableNote[Normal production expectations] -.-> Stable
  BetaNote[Supported but not frozen] -.-> Beta
  ExperimentalNote[Opt in churn] -.-> Experimental
`"
/>

## Public Language Versus Formal Language

The public docs use a simpler first-pass vocabulary:

- `Recommended` usually points at the strongest current `public-stable` lanes
- `Advanced` points at supported surfaces that are narrower or more specialized
- `Experimental` maps to `public-experimental`

When you are setting compatibility policy, the formal terms win.

## How To Read Recommended

`Recommended` is product language, not a replacement for the formal contract.

- it usually means a promoted `public-stable` production lane
- it does not mean parity across every target
- it does not upgrade `public-beta` or `public-experimental` surfaces by wording alone

## Public-Stable

Treat `public-stable` as the level you can build against with normal production expectations.

## Public-Beta

Treat `public-beta` as supported, but not frozen.

Use beta only when the tradeoff is explicit and worth it for the product.

## Public-Experimental

Treat `public-experimental` as opt-in churn outside the normal compatibility expectation.

## Practical Rule

1. Prefer the recommended lane for the product you are building.
2. Use the exact formal terms only when you need policy or compatibility precision.
3. Use `validate --strict` as the readiness gate for the repo you plan to ship.
