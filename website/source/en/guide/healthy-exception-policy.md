---
title: "Healthy Exception Policy"
description: "When a special-case repo is acceptable, and when it has already become unhealthy drift instead of a justified exception."
canonicalId: "page:guide:healthy-exception-policy"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Healthy Exception Policy

Use this page when a repo really does need to be different, but the team wants a public rule for when that difference is justified instead of letting every special case become permanent folklore.

## Choose In 60 Seconds

- read this page when someone says “this repo is special” and you need to decide whether that is healthy or not
- read this page before turning a one-off repo into a permanent exception class
- read this page when the team keeps adding caveats around one repo instead of correcting the path
- do not read this page to excuse weak standards; read it to keep exceptions narrow and honest

## What This Page Helps You Decide

- whether a repo is a justified exception or unhealthy drift
- how narrow a special-case path must stay
- when an exception should be normalized, replaced, or retired

## A Healthy Exception Looks Like This

A healthy exception is:

- explicitly named
- narrow in scope
- explained in public docs or repo docs
- still compatible with the public support boundary
- still validated through a clear contract

If the exception needs private explanation, it is already becoming unhealthy.

## An Unhealthy Exception Looks Like This

An unhealthy exception usually shows up as one or more of these:

- the repo is “special” only because nobody wants to fix it
- the team cannot explain why the exception still exists
- CI, docs, and release guidance do not agree on the real rule
- new repos keep copying the exception by accident
- the exception quietly changes the support promise without being declared

## Good Reasons For An Exception

- one platform or target has a real product-specific constraint
- one repo is intentionally bridging a temporary migration phase
- a repo is proving a bounded beta path without pretending it is the new default
- a team needs one temporary exception while a cleaner baseline is being prepared

## Bad Reasons For An Exception

- “it already worked once”
- “rewriting it would be annoying”
- “only one maintainer understands it”
- “the docs do not cover this cleanly yet”

Those are signals to fix the baseline, not reasons to bless drift.

## The Exception Test

Treat an exception as healthy only if you can answer “yes” to all of these:

1. Is the exception explicitly documented?
2. Is the exception narrower than the team default?
3. Does the repo still pass a clear `doctor -> render -> validate --strict` contract?
4. Does the exception preserve the public support promise instead of silently widening it?
5. Do new repos know not to copy it unless they share the same real constraint?

## What To Do With A Healthy Exception

- keep it narrow
- document the reason once
- link the exact support or migration note behind it
- review whether it should still exist when the baseline changes

## What To Do With An Unhealthy Exception

- stop treating it as an acceptable standard
- decide whether it needs correction, recovery, or retirement
- move the team back toward the clean baseline
- do not let templates or starter guidance copy it further

## Best First Stops

- Need to recover after the wrong path became “special”: [Path Recovery](/en/guide/path-recovery)
- Need to detect whether the baseline already drifted: [Baseline Drift Signals](/en/guide/baseline-drift-signals)
- Need the rule for the team baseline: [Reference Repo Strategy](/en/guide/reference-repo-strategy)
- Need the public support limit: [Support Boundary](/en/reference/support-boundary)

## Final Rule

A special-case repo is healthy only when the team can explain why it is exceptional, prove it cleanly, and prevent other repos from copying it by accident.
