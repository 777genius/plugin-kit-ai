---
title: "Version And Compatibility Policy"
description: "How to think about releases, compatibility promises, wrappers, SDKs, and support vocabulary in plugin-kit-ai."
canonicalId: "page:reference:version-and-compatibility"
section: "reference"
locale: "en"
generated: false
translationRequired: true
---

# Version And Compatibility Policy

This page is the compact public answer to a team question:

- which versions define the current baseline, and which compatibility promises are strong enough to standardize on

## Choose In 60 Seconds

- read this page when your team needs one compact policy for releases, wrappers, SDKs, runtimes, and compatibility promises
- read [Support Boundary](/en/reference/support-boundary) when you want the shortest practical support answer
- read [Releases](/en/releases/) when you want the story of a specific release

## The Public Baseline

Think about versions in three layers:

- the release line you standardize across repos
- the support level of the lane you choose inside that release line
- the install or delivery mechanism you use around that lane

These layers are related, but they are not the same thing.

## What Compatibility Really Covers Here

The strongest public promise is around:

- the declared public CLI contract
- the recommended Go SDK path
- the recommended local Python and Node runtime lanes on supported targets
- the documented behavior of `public-stable` generated outputs

Compatibility does not mean that every wrapper, convenience path, or specialized surface moves with the same promise.

## Public Language Versus Formal Terms

Use this simple translation:

- `Recommended` usually means the lane is inside the strongest current `public-stable` contract
- `Advanced` means the surface is supported, but more specialized or narrower than the first default
- `Experimental` means opt-in churn with no normal compatibility expectation

When a team needs exact policy, use the formal terms `public-stable`, `public-beta`, and `public-experimental`.

## Wrappers, SDKs, And Runtime APIs

Do not mix these categories together.

- Homebrew, npm, PyPI, and the verified script are install channels for the CLI
- the Go SDK is a public SDK surface
- runtime APIs are tied to their declared runtime lanes

If you treat install wrappers as if they carry the same promise as an SDK, you will standardize the wrong layer.

## What Teams Should Standardize

Healthy teams usually standardize:

- one declared release baseline
- one primary lane with a clear support story
- one validation gate before handoff and rollout
- one shared interpretation of the formal compatibility terms

## Final Rule

Standardize on the release line and lane whose public promise your team is actually willing to defend in CI, handoff, and rollout.
