---
title: "Team Adoption"
description: "Adopt plugin-kit-ai across a team without relying on tribal knowledge or one-person local success."
canonicalId: "page:guide:team-adoption"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Team Adoption

Use this page when the question is no longer “can one person make it work?” but “can our team use the same model, repo rules, and release guidance without confusion?”

## Choose In 60 Seconds

- Starting a fresh team repo: begin here, then read [Build A Team-Ready Plugin](/en/guide/team-ready-plugin).
- Standardizing an existing repo: begin here, then read [Migrate Existing Config](/en/guide/migrate-existing-config).
- Rolling plugin-kit-ai out across several repos: begin here, then read [Repository Standard](/en/reference/repository-standard) and [CI Integration](/en/guide/ci-integration).

## Read This If

- you are the team lead, repo owner, or maintainer who will be asked “what is the official path?”
- you need one public sequence for setup, validation, CI, and handoff
- you want to stop repo-by-repo improvisation before it becomes team folklore

## The Adoption Path

1. Pick one supported path on purpose.
   Start from the narrowest real requirement instead of promising every target or runtime at once.
2. Make the repo contract visible.
   The team should be able to see authored state, generated outputs, the chosen target, and the main validation command.
3. Turn the contract into CI.
   `doctor`, `render`, and `validate --strict` should be repeatable on another machine before the repo is treated as healthy.
4. Keep release guidance in the loop.
   Team standards should point to the current public release note, not to old chat messages or one maintainer's memory.

## Best First Stops

- Need the repository contract: [Repository Standard](/en/reference/repository-standard)
- Need the canonical authoring flow: [Authoring Workflow](/en/reference/authoring-workflow)
- Need the minimum CI gate: [CI Integration](/en/guide/ci-integration)
- Need the public readiness checklist: [Production Readiness](/en/guide/production-readiness)
- Need the current delivery recommendation: [v1.0.6](/en/releases/v1-0-6)

## Fresh Repo vs Existing Repo

- Fresh repo:
  pick the supported path first, scaffold once, then lock in the repo standard before the team starts copying local habits.
- Existing repo:
  map the current target files back to the authored source of truth, then move toward the canonical render-and-validate loop deliberately.

## What Good Adoption Looks Like

- new teammates know which guide to start from
- the repo tells them which target and runtime were chosen
- CI reproduces generated outputs instead of trusting hand-edited files
- release notes are used to confirm changing guidance instead of reinventing it

## What To Avoid

- standardizing a path before checking [Support Boundary](/en/reference/support-boundary)
- treating starter names as the long-term boundary of the repo
- letting each repo invent its own validation flow
- keeping critical setup decisions only in Slack, chat, or reviewer memory

## What To Read Next

- Read [Build A Team-Ready Plugin](/en/guide/team-ready-plugin) for the practical repo-level tutorial.
- Read [Production Readiness](/en/guide/production-readiness) for the checklist you can apply before broader rollout.
- Read [CI Integration](/en/guide/ci-integration) when you are ready to make the contract executable.
