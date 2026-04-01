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

## Choose In 60 Seconds

- Need the safest default production path: choose Go.
- Need the safest rule for interpreted runtimes: trust `validate --strict` for supported Python and Node lanes.
- Need the safest rule about wrappers: treat them as CLI install paths, not runtime APIs.
- Need the safest quick matrix by target: pair this page with [Target Support](/en/reference/target-support).
- Need the shortest comparison of promises by path: open [Support Promise By Path](/en/reference/support-promise-by-path).

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

## What This Page Helps You Decide

- whether a path is safe as a default
- whether a path is stable enough for long-term team use
- whether you are accidentally treating install, packaging, or workspace lanes like runtime contracts

Pair this page with [Support Promise By Path](/en/reference/support-promise-by-path), [Target Support](/en/reference/target-support), and [Stability Model](/en/concepts/stability-model).
If the real question is whether one repo may stay special without becoming unhealthy drift, read [Healthy Exception Policy](/en/guide/healthy-exception-policy).
