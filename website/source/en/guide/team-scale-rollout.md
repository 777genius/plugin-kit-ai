---
title: "Team-Scale Rollout"
description: "Roll out new plugin-kit-ai defaults, release guidance, and support decisions across several repos without guesswork."
canonicalId: "page:guide:team-scale-rollout"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Team-Scale Rollout

Use this page when the question is no longer “can one repo adopt the new guidance?” but “how do we roll the new baseline out across several repos without confusion, drift, or team folklore?”

## Choose In 60 Seconds

- Rolling out a new baseline across several repos: start here, then read the newest matching [release note](/en/releases/).
- Standardizing a new runtime or delivery default across a team: start here, then pair it with [Version And Compatibility Policy](/en/reference/version-and-compatibility).
- Migrating older repos toward the managed project model in phases: start here, then read [Upgrade And Migration Playbook](/en/guide/upgrade-and-migration-playbook).

## What This Page Helps You Decide

- whether the new guidance should be rolled out now or trialed first
- how to choose one reference repo before touching the rest
- how to turn a release note into a team standard instead of a one-person memory
- how to stop partial rollout from becoming permanent repo drift

## The Safe Rollout Pattern

1. Choose one published baseline.
   Link one release note and one support rule, not several half-remembered states.
2. Pick one reference repo.
   Prove the new path in one real repo before changing templates or every active repo.
3. Run the canonical contract.
   `doctor -> render -> validate --strict` must pass on another machine, not only on the maintainer's laptop.
4. Update repo docs and CI.
   The new rule is real only when the repo, its CI, and team-facing docs say the same thing.
5. Roll out deliberately.
   Move the next repos in batches, not all at once, and keep the release note linked in rollout tracking.

## Read By Scenario

- New Python or Node default:
  start with the newest delivery note, currently [v1.0.6](/en/releases/v1-0-6), then align [Choose Delivery Model](/en/guide/choose-delivery-model) and CI.
- New Go baseline:
  start with the matching Go-facing release note and [Go SDK](/en/api/go-sdk/), then standardize the runtime path and repo contract.
- Mixed repo estate:
  split the estate into `already managed`, `needs simple upgrade`, and `still native-config`, then apply different rollout tracks.
- New support boundary decision:
  confirm it with [Support Promise By Path](/en/reference/support-promise-by-path) and [Support Boundary](/en/reference/support-boundary) before announcing it team-wide.

## The Reference Repo Rule

- Choose one repo that is representative, active, and visible to the team.
- Make that repo pass the full contract first.
- Update starter guidance, internal templates, and rollout checklists only after that reference repo is clean.

## What Mature Rollout Looks Like

- every repo in scope links the same public baseline
- CI proves the same authored contract across repos
- runtime or delivery changes are documented once and reused
- team discussions point to published pages and release notes instead of chat history

## What Not To Do

- do not roll a new default everywhere at once because it “sounds better”
- do not update templates before one real repo proves the path cleanly
- do not mix old and new defaults across repos without an explicit transition note
- do not announce support promises that are stronger than the public docs

## Best First Stops

- Need the current public baseline: [Releases](/en/releases/)
- Need the version rule behind the rollout: [Version And Compatibility Policy](/en/reference/version-and-compatibility)
- Need the repo-level adoption path: [Team Adoption](/en/guide/team-adoption)
- Need the upgrade mechanics inside one repo: [Upgrade And Migration Playbook](/en/guide/upgrade-and-migration-playbook)
- Need the exact support limit before rollout: [Support Promise By Path](/en/reference/support-promise-by-path)

## Final Rule

A team-scale rollout is complete only when another maintainer can pick any repo in the rollout set, identify the same public baseline, run the same validation contract, and explain the chosen path from public docs alone.
