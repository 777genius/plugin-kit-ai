---
title: "Path Recovery"
description: "How to recover safely when a repo already works, but the starter, target, runtime, or delivery model was the wrong long-term choice."
canonicalId: "page:guide:path-recovery"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Path Recovery

Use this page when the repo is not broken, but the chosen path is already teaching the team the wrong habit, promise, or operating model.

## Choose In 60 Seconds

- use this page when the repo works, but the path is clearly wrong for the next stage of the project
- use this page when you want to recover without pretending the current repo must be thrown away
- use this page before spreading a doubtful path across more repos or templates
- do not use this page for normal upgrades that stay on the same healthy path

## What This Page Helps You Decide

- whether the repo needs a targeted correction or a cleaner restart
- how to recover without losing trust in CI, generated outputs, or team guidance
- how to keep one reference repo clean before any broader rollout

## The Safe Recovery Pattern

1. Name the real mismatch.
   Decide whether the wrong choice lives in the starter, target, runtime, or delivery model.
2. Freeze the promise.
   Stop announcing stronger support than the current repo actually carries.
3. Pick one clean destination.
   Choose the corrected target, runtime, or delivery model before touching code.
4. Recover in one reference repo.
   Do not “repair everything everywhere” at once.
5. Re-run the canonical contract.
   `doctor -> render -> validate --strict`
6. Only then update templates, CI, and rollout guidance.

## Recover By Mismatch

### Wrong Starter, Correct Product Shape

If the team chose the wrong starter but the actual target and runtime are still right, do not overreact. The starter is an entrypoint, not the permanent identity of the repo.

**Best move:** normalize the repo toward the correct managed layout, then update starter guidance for future repos.

### Wrong Target

If the repo was built as runtime when the real product is packaging, extension, or workspace ownership, treat this as a product-shape correction, not a tiny patch.

**Best move:** go back to [Choose A Target](/en/guide/choose-a-target), correct the output shape first, then re-run render and strict validation.

### Wrong Runtime

If Node or Python were chosen but the real long-term need was simply the strongest production default, recovery is often easiest before the repo spreads.

**Best move:** compare with [Choosing Runtime](/en/concepts/choosing-runtime), decide whether the operational cost is justified, and if not, move back to the stronger default before rollout.

### Wrong Delivery Model

If the team introduced shared runtime packages too early, or vendored helpers are now blocking reuse across repos, correct the delivery model explicitly instead of layering exceptions on top.

**Best move:** re-check [Choose Delivery Model](/en/guide/choose-delivery-model) and migrate one reference repo cleanly before any team-wide standardization.

## Restart Or Repair?

Repair the existing repo when:

- the product shape is still correct
- generated outputs still reflect one understandable source of truth
- the team can explain the correction in one short public sentence

Restart from a cleaner baseline when:

- the chosen path already teaches the wrong product shape
- hand-edited generated files hide the real authored state
- CI proves only local luck, not the actual managed contract
- the repo is about to become a template for more repos

## What To Protect During Recovery

- one public explanation of the corrected path
- one reference repo that proves the new baseline cleanly
- one CI contract that matches the authored source of truth
- one linked release or policy note when the change affects team standards

## What Not To Do

- do not keep layering exceptions on top of a path you already know is wrong
- do not roll a doubtful fix across several repos before one reference repo is clean
- do not treat generated drift as a cosmetic nuisance during recovery
- do not announce a stronger support promise just because the repo “still works”

## Best First Stops

- Need to confirm you are on the wrong path: [Decision Anti-Patterns](/en/guide/decision-anti-patterns)
- Need the corrected target: [Choose A Target](/en/guide/choose-a-target)
- Need the corrected starter: [Choose A Starter Repo](/en/guide/choose-a-starter)
- Need the corrected runtime: [Choosing Runtime](/en/concepts/choosing-runtime)
- Need the corrected delivery model: [Choose Delivery Model](/en/guide/choose-delivery-model)
- Need team-level recovery discipline: [Team Adoption](/en/guide/team-adoption), [Upgrade And Migration Playbook](/en/guide/upgrade-and-migration-playbook), and [Team-Scale Rollout](/en/guide/team-scale-rollout)

## Final Rule

Recovery is complete only when the team no longer needs a private explanation for why this repo is “special.”
