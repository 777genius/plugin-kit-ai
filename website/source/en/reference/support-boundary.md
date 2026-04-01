---
title: "Support Boundary"
description: "A compact guide to what plugin-kit-ai treats as stable, beta, and intentionally out of scope."
canonicalId: "page:reference:support-boundary"
section: "reference"
locale: "en"
generated: false
translationRequired: true
---

# Support Boundary

This page is the compact answer to a simple question: what can you rely on today, and what should you treat with caution?

## Safe Defaults

- Go is the recommended production path.
- `validate --strict` is the main readiness check for local Python and Node runtime projects.
- The CLI install wrappers are ways to install the CLI, not runtime APIs.

## Stable By Default

- the main public CLI contract
- the recommended Go SDK path
- the stable local Python and Node subset on supported runtime targets
- the targets explicitly marked stable in the generated support matrix

## Use Carefully

- beta paths that are still evolving
- workspace-configuration targets when you actually need an executable plugin
- install wrappers when what you really want is a runtime or SDK API

## Out Of Scope

- treating every target as if it had the same runtime guarantees
- treating wrapper packages as SDKs or runtime contracts
- assuming experimental surfaces carry long-term compatibility promises

Pair this page with [Version And Compatibility Policy](/en/reference/version-and-compatibility), [Target Support](/en/reference/target-support), and [Stability Model](/en/concepts/stability-model).
