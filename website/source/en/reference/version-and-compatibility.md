---
title: "Version And Compatibility Policy"
description: "How to think about releases, support promises, wrappers, SDKs, and compatibility boundaries in plugin-kit-ai."
canonicalId: "page:reference:version-and-compatibility"
section: "reference"
locale: "en"
generated: false
translationRequired: true
---

# Version And Compatibility Policy

This page is the compact public answer to a practical team question:

- which versions define the current baseline, and which compatibility promises are real enough to standardize on

## Choose In 60 Seconds

- read this page when your team needs one compact policy for releases, wrappers, SDKs, runtimes, and support promises
- read [Support Boundary](/en/reference/support-boundary) when you want the shortest current-state summary
- read [Releases](/en/releases/) when you want the story of what changed in a specific release

## The Public Baseline

Think about versions in three layers:

- the release line you standardize across repos
- the support level of the path you choose inside that release line
- the install or delivery mechanism you use around that path

These layers are related, but they are not the same thing.

## What Versioning Really Covers Here

When plugin-kit-ai talks about compatibility, the strongest public promise is around:

- the declared public CLI contract
- the recommended Go SDK path
- the stable local Python and Node subset on supported runtime targets
- the documented behavior of public-stable generated outputs

Compatibility does not mean that every target, every wrapper, and every convenience path moves with the same promise.

## Stable Baseline Versus Moving Edges

Use this rule:

- treat `public-stable` paths as normal production candidates
- treat `public-beta` paths as usable, but not frozen
- treat install wrappers as CLI delivery channels, not runtime or SDK contracts

This is why a release can be healthy and production-worthy even when some paths are still beta.

## Wrappers, SDKs, And Runtime APIs

One of the most common mistakes is mixing these categories together.

- Homebrew, npm, PyPI, and the verified script are install channels for the CLI
- the Go SDK is a public SDK surface
- the runtime APIs are tied to their declared supported runtime lanes

If you treat install wrappers as if they carried the same compatibility promise as an SDK, you will standardize the wrong thing.

## What Teams Should Standardize

Healthy teams usually standardize:

- one declared release baseline
- one primary path with a clear support story
- one validation gate before handoff and rollout
- one interpretation of stable, beta, and experimental for the whole team

Healthy teams do not standardize:

- a wrapper package as if it were the product contract
- a beta convenience path as if it were the strongest baseline
- a single exceptional repo as if it defined the default for every other repo

## How To Read Release Notes Safely

Use release notes as guidance for change, not as the only place where your policy lives.

The safe order is:

1. Keep one baseline policy for the team.
2. Read release notes for what changed relative to that baseline.
3. Update the baseline only after the new path matches your support expectations.

This keeps your team from converting every fresh release detail into an accidental standard.

## Best First Stops

- Read [Support Boundary](/en/reference/support-boundary) for the shortest current contract framing.
- Read [Stability Model](/en/concepts/stability-model) when you need the exact meaning of stable, beta, and experimental.
- Read [Install Channels](/en/reference/install-channels) when the confusion is really about wrapper installs versus public APIs.
- Read [v1.0.6](/en/releases/v1-0-6) when you want the latest user-facing baseline in this docs set.

## Final Rule

Standardize on the release line and path whose public promise you actually want to defend in CI, handoff, and team rollout.
