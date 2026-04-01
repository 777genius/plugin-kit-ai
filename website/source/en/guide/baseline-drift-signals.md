---
title: "Baseline Drift Signals"
description: "How to spot that a repo is drifting away from the declared standard even when it still looks workable."
canonicalId: "page:guide:baseline-drift-signals"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Baseline Drift Signals

Use this page when the repo still appears healthy, but you suspect the real team baseline and the actual repo behavior are starting to diverge.

## Choose In 60 Seconds

- read this page when CI is green, but the repo already feels harder to explain than before
- read this page when one repo still “works” but new repos no longer know what to copy
- read this page before promoting a repo as the reference baseline
- do not use this page as a substitute for normal bug fixing; use it when the problem is standard drift

## What This Page Helps You Decide

- whether the repo is drifting away from the declared standard
- whether the team is still teaching one clean baseline
- when to stop calling a repo “healthy enough” and correct the path

## Fastest Drift Check

Treat the baseline as drifting if two or more of these are already true:

- CI still passes, but another maintainer cannot explain the chosen path without private help
- generated outputs are reproducible only after local habit or undocumented steps
- README, release note, and support policy no longer point to the same current path
- the repo is now the “special case” other repos are told not to copy directly
- teams start asking whether the reference repo is still really the standard

## Common Drift Signals

### 1. The Repo Passes CI But Fails Explainability

If another maintainer can clone the repo and pass CI, but still cannot explain why this starter, target, runtime, or delivery model was chosen, the baseline is already drifting.

### 2. Rendered Outputs Need Private Fixes

If `render` technically works, but the team quietly expects manual edits or local cleanup after generation, the repo is no longer proving the public contract cleanly.

### 3. The Reference Repo Has Become The Special Repo

If people keep saying “do what that repo does, except for these three special cases,” the reference repo is no longer teaching the baseline. It is teaching exceptions.

### 4. Public Guidance And Repo Reality No Longer Match

If release notes, support promises, repo docs, and CI each imply a slightly different path, the team is already running on drift instead of one standard.

### 5. New Repos Start With A Different Mental Model

If new repos keep choosing different starters, targets, or delivery paths because the existing standard is no longer clear, the problem is not onboarding. The baseline itself is weak.

## What To Check First

- can a new maintainer identify the chosen path from public docs alone?
- does `doctor -> render -> validate --strict` still prove the intended path without local folklore?
- does the repo still match the support promise the team claims publicly?
- is the reference repo still simple enough to copy, or only old enough to be trusted?

## What To Do When You See Drift

1. Stop promoting the repo as the clean baseline.
2. Decide whether the issue is explanation drift, support drift, or actual path drift.
3. Correct one reference repo first.
4. Update CI, repo docs, and public guidance together.
5. Only then resume rollout or template updates.

## Best First Stops

- Need the reference-repo rule: [Reference Repo Strategy](/en/guide/reference-repo-strategy)
- Need the repository contract: [Repository Standard](/en/reference/repository-standard)
- Need safe correction after the wrong path: [Path Recovery](/en/guide/path-recovery)
- Need team rollout discipline: [Team-Scale Rollout](/en/guide/team-scale-rollout)
- Need support-level confirmation: [Support Promise By Path](/en/reference/support-promise-by-path)
- Need the rule for whether one repo is a justified exception: [Healthy Exception Policy](/en/guide/healthy-exception-policy)

## Final Rule

A baseline is drifting the moment the team trusts it out of habit more than it trusts it out of reproducible proof.
