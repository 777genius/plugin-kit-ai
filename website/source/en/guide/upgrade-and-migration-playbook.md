---
title: "Upgrade And Migration Playbook"
description: "Upgrade plugin-kit-ai safely across existing repos and teams without relying on guesswork."
canonicalId: "page:guide:upgrade-and-migration-playbook"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Upgrade And Migration Playbook

Use this page when your team already has live repos and the real question is how to adopt new guidance, new defaults, or the managed project model without breaking trust.

## Choose In 60 Seconds

- Upgrading an existing managed repo: start here, then read the newest matching [release note](/en/releases/).
- Moving from native target files into the managed project model: start here, then read [Migrate Existing Native Config](/en/guide/migrate-existing-config).
- Rolling a new default across several repos: start here, then read [Team Adoption](/en/guide/team-adoption) and [Production Readiness](/en/guide/production-readiness).
- Rolling a new default across several repos: start here, then read [Team Adoption](/en/guide/team-adoption), [Production Readiness](/en/guide/production-readiness), and [Team-Scale Rollout](/en/guide/team-scale-rollout).

## What This Playbook Helps You Decide

- whether a repo needs a simple version bump, a delivery-path change, or a deeper migration
- how to roll out new guidance without making each repo improvise its own interpretation
- when to stop and re-check the support boundary before promising a new path to downstream users

## The Safe Upgrade Pattern

1. Read the newest release note that matches your runtime or delivery path.
2. Separate "what changed" from "what did not change".
3. Decide whether this repo is:
   - staying on the same path with newer defaults
   - changing delivery model
   - migrating from native config into the managed project model
4. Re-run the canonical contract:
   `doctor -> render -> validate --strict`
5. Roll the change into CI and repo docs before calling the upgrade complete.

## Read By Scenario

- Existing Go repo with no path change:
  read the matching release note, check SDK or CLI changes, then rerun the standard validation flow.
- Existing Python or Node repo:
  read the latest delivery guidance first, especially if the release changes `--runtime-package` recommendations.
- Existing native-config repo:
  treat the change as a migration into the managed project model, not as a small patch.
- Several repos owned by one team:
  choose one reference repo first, validate the path there, then roll it out deliberately through [Team-Scale Rollout](/en/guide/team-scale-rollout).

## What To Check Before You Change Anything

- whether the chosen target is still inside the public support boundary
- whether the release changes the recommended default or only clarifies existing guidance
- whether the repo already relies on hand-edited generated files
- whether CI currently proves the authored source of truth or only proves one developer's local state

## Good Migration Discipline

- upgrade one repo first before changing every repo in parallel
- keep release notes linked in the migration plan
- update starter or internal repo templates only after one real repo passes the new path cleanly
- treat generated drift as signal, not as cosmetic noise

## What Not To Do

- do not roll out a new default because it “sounds newer” without checking support and release notes
- do not treat a migration from native config as a tiny formatting cleanup
- do not standardize a new runtime or delivery path without updating CI and repository documentation
- do not read one release note as a promise of equal support across every language or target

## Best First Stops

- Need the current user-facing changes: [Releases](/en/releases/)
- Need the team rollout path: [Team Adoption](/en/guide/team-adoption)
- Need the multi-repo rollout path: [Team-Scale Rollout](/en/guide/team-scale-rollout)
- Need the migration from native config: [Migrate Existing Native Config](/en/guide/migrate-existing-config)
- Need the exact repository contract: [Repository Standard](/en/reference/repository-standard)
- Need the exact support limits: [Support Boundary](/en/reference/support-boundary)

## Final Rule

An upgrade is complete only when another teammate can clone the repo, reproduce the rendered outputs, pass `validate --strict`, and explain the new chosen path from public docs and release notes.
