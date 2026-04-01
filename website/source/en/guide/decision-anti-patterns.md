---
title: "Decision Anti-Patterns"
description: "How to recognize that you picked the wrong starter, target, runtime, or delivery model before the repo drifts further."
canonicalId: "page:guide:decision-anti-patterns"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Decision Anti-Patterns

Use this page when the repo technically works, but the path still feels wrong, too expensive, or harder to explain than it should be.

## Choose In 60 Seconds

- read this page when your repo works, but the chosen path already feels heavier than the job
- read this page when you keep explaining the same exceptions in chat or PR comments
- read this page before standardizing a starter, target, or delivery model across several repos
- do not read this page to optimize a healthy repo that already matches the real requirement

## What This Page Helps You Decide

- whether you picked the wrong starter, target, runtime, or delivery model
- whether the repo should restart from a cleaner baseline
- whether the real problem is product shape confusion instead of a technical bug

## Fastest Smell Test

You probably chose the wrong path if two or more of these are already true:

- the repo technically works, but the team keeps re-explaining why it was set up this way
- your first real requirement and your current target no longer match
- package, extension, or workspace outputs are being treated like runtime plugins
- Node or Python were chosen even though the real requirement was simply the strongest production default
- shared runtime packages were introduced before one clean repo pattern existed
- rollout is happening across multiple repos before one reference repo is clearly healthy

## Common Anti-Patterns

### 1. Treating Package Or Workspace Targets Like Runtime Paths

This is the most common category error.

If the output is a package, extension, or workspace configuration, do not expect it to behave like an executable runtime plugin. The repo may still be correct, but your expectations are pointed at the wrong product shape.

**Fix:** Go back to [Choose A Target](/en/guide/choose-a-target), [Package And Workspace Targets](/en/guide/package-and-workspace-targets), and [Support Promise By Path](/en/reference/support-promise-by-path).

### 2. Choosing Node Or Python When The Real Need Was The Strongest Default

Node and Python are real supported paths. They are not wrong by themselves.

They become the wrong choice when the team does not actually need language-specific ownership and only wanted the strongest, simplest production path. In that case you bought more runtime responsibility than necessary.

**Fix:** Re-check [Choosing Runtime](/en/concepts/choosing-runtime) and [Production Readiness](/en/guide/production-readiness). If nothing truly requires Node or Python, restart from the Go default before rollout spreads.

### 3. Treating The Starter Name As The Permanent Repo Boundary

A starter tells you how to begin. It does not tell you the final limit of the repo.

If the team is treating a Claude or Codex starter name as proof that the repo can never grow beyond that first lane, the mental model is already off.

**Fix:** Re-read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) and [Managed Project Model](/en/concepts/managed-project-model).

### 4. Choosing Shared Runtime Packages Too Early

`plugin-kit-ai-runtime` is a supported path, not a fallback. But it is still the wrong first move when the team does not yet have one clean reference repo pattern.

If each repo is still discovering its own shape, shared runtime packages add coordination cost before the team has earned that abstraction.

**Fix:** Start with vendored helpers or the strongest Go default, then introduce shared runtime packages only after one healthy repo pattern exists. Re-check [Choose Delivery Model](/en/guide/choose-delivery-model).

### 5. Treating CLI Install Wrappers Like Runtime APIs

If people are reading npm or PyPI install wrappers as if they were public runtime surfaces, the repo is already mixing installation and execution concepts.

**Fix:** Return to [Installation](/en/guide/installation), [Install Channels](/en/reference/install-channels), and [API Overview](/en/api/).

### 6. Standardizing A Path Before Checking The Support Promise

Teams sometimes standardize on a path because it "works once" and only later discover that the support promise, operational cost, or rollout burden is not what they assumed.

**Fix:** Check [Support Boundary](/en/reference/support-boundary), [Support Promise By Path](/en/reference/support-promise-by-path), and [Target Support](/en/reference/target-support) before you turn one repo experiment into team policy.

### 7. Scaling Rollout Before One Reference Repo Is Clean

If rollout has already started across multiple repos, but there is still debate about the correct starter, target, runtime, or delivery model, the team is scaling confusion.

**Fix:** Stop rollout, clean up one reference repo, then continue with [Team Adoption](/en/guide/team-adoption), [Team-Scale Rollout](/en/guide/team-scale-rollout), and [Upgrade And Migration Playbook](/en/guide/upgrade-and-migration-playbook).

## When To Restart And When Not To

Restart from a cleaner path when:

- the chosen path forces repeated explanation
- the support promise does not match the real operational need
- the repo shape is already teaching the team the wrong mental model

Do **not** restart just because another path looks more elegant on paper. Restart only when the current choice makes the repo harder to operate, harder to hand off, or harder to scale correctly.

## Best First Stops

- Wrong target suspicion: [Choose A Target](/en/guide/choose-a-target)
- Wrong starter suspicion: [Choose A Starter Repo](/en/guide/choose-a-starter)
- Wrong runtime suspicion: [Choosing Runtime](/en/concepts/choosing-runtime)
- Wrong delivery-model suspicion: [Choose Delivery Model](/en/guide/choose-delivery-model)
- Wrong support expectation: [Support Promise By Path](/en/reference/support-promise-by-path)
- Ready to correct the repo without spreading the mistake further: [Path Recovery](/en/guide/path-recovery)

## Final Rule

The right path is not the one that merely works today. It is the one that stays easy to explain, validate, hand off, and scale once the repo stops being a solo experiment.
