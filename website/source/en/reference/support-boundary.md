---
title: "Support Boundary"
description: "The shortest practical answer to what plugin-kit-ai recommends, supports with care, and keeps experimental."
canonicalId: "page:reference:support-boundary"
section: "reference"
locale: "en"
generated: false
translationRequired: true
---

# Support Boundary

This page answers one practical question: what can you recommend today, what is advanced, and what stays experimental?

## Safe Defaults

- Go is the recommended default runtime lane.
- `validate --strict` is the main readiness gate for local Python and Node runtime repos.
- Codex package and Gemini packaging are recommended when package or extension delivery is the real product.
- OpenCode and Cursor fit when the repo should own integration config.

## How This Maps To The Formal Contract

- `Recommended` usually maps to the strongest current `public-stable` production lanes.
- `Advanced` means a supported surface with a narrower, more specialized, or more careful contract.
- `Experimental` means opt-in churn outside the normal compatibility expectation.

Use the formal `public-stable`, `public-beta`, and `public-experimental` terms when you are setting policy for a team or promising compatibility to downstream users.

## Today In Practice

- Claude is recommended on the default stable hook lane.
- Codex is recommended both for the `Notify` runtime lane and the official `codex-package` lane.
- Gemini packaging is recommended, and the promoted Gemini Go runtime is also production-ready.
- OpenCode and Cursor are repo-managed integration lanes, not the default runtime starting point.

## Advanced Surfaces

Use these when the tradeoff is intentional and explicit:

- narrower or specialized runtime expansions beyond the main recommended lanes
- install wrappers when your real concern is CLI delivery, not runtime APIs or SDKs
- specialized configuration surfaces that are useful but not the first default for most teams

## Experimental Surfaces

Treat experimental areas as opt-in and high-churn. They are useful for early adopters, but they should not silently become long-term team policy.
