---
title: "Stability Model"
description: "How plugin-kit-ai classifies public-stable, public-beta, and experimental areas."
canonicalId: "page:concepts:stability-model"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Stability Model

`plugin-kit-ai` is intentionally explicit about which areas are stable and which are still moving.

## Public-Stable

Treat `public-stable` as the level you can build against with normal production expectations.

Examples in the current project direction include:

- the core supported CLI commands such as `init`, `validate`, `test`, `capabilities`, `inspect`, `install`, and `version`
- the recommended Go SDK path
- the stable local Python and Node subset on supported runtime targets
- strict validation and deterministic generated-artifact checks

## Public-Beta

Treat `public-beta` as supported, but not frozen.

Typical beta areas are:

- targets that are still widening their supported behavior
- higher-churn config or packaging areas
- convenience workflows that are useful, but not yet at the same guarantee level as the core path

You can use beta in real projects when the tradeoff is worth it, but you should not treat beta as if it had the same long-term compatibility promise as the stable path.

## Public-Experimental

Experimental means exactly that:

- useful for early adopters
- intentionally outside the normal compatibility expectation
- allowed to change sharply or be removed

Do not make experimental areas part of a long-lived production contract unless you are willing to absorb churn.

## Practical Rule

The safe default is:

1. Prefer `go` when you want the strongest path.
2. Prefer declared stable CLI and runtime areas over convenience beta paths.
3. Use `validate --strict` as the main readiness check for local Python and Node runtimes.

See [Choosing Runtime](/en/concepts/choosing-runtime) for the runtime decision model and [Version And Compatibility Policy](/en/reference/version-and-compatibility) for the public policy layer teams can standardize on.
