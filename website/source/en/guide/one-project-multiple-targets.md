---
title: "One Project, Multiple Targets"
description: "How one plugin-kit-ai project can stay managed while supporting more than one agent or output target."
canonicalId: "page:guide:one-project-multiple-targets"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# One Project, Multiple Targets

This is one of the most important ideas in `plugin-kit-ai`:

- a **starter repo** gives you a good first entrypoint
- a **managed project** can grow beyond that first entrypoint

Do not confuse the starter family with the long-term limit of the project.

## The Product Promise

The promise is not “one starter forever.”

The promise is:

- one managed project model
- one authored source of truth
- as many rendered outputs as the real product needs
- without pretending every target has the same maturity

## The Short Rule

Start with the runtime or target that is your **primary requirement today**.

After that, keep treating the repo as one managed source of truth and render the target-specific artifacts you actually need.

That means a project can begin as:

- a Codex-first plugin repo
- a Claude-first plugin repo
- a package/config-first repo

and still become a broader managed project over time.

## Why The Starters Look Agent-Specific

The official starters are split by primary path on purpose:

- Codex starters optimize the default Codex runtime path
- Claude starters optimize the stable Claude hook path
- language variants optimize the first runtime team choice

That makes the first run predictable.

What it does **not** mean:

- that `plugin-kit-ai` only supports one agent forever
- that you must keep separate repos for every agent
- that the starter name defines the final product boundary

## What Actually Stays Unified

The unifying part is the **managed project model**.

That means your team keeps one authored project and then uses `render`, `validate`, import/normalize flows, and target directories to manage the outputs that matter.

In practice, the unified part is:

- one repo layout
- one authoring workflow
- one validation story
- one CI story
- one place to review managed target outputs

## What “Multiple Targets” Means In Practice

There are two common cases.

### 1. One Primary Runtime, Several Managed Outputs

Example:

- your main plugin behavior is Codex runtime
- but the same repo also manages package/config targets such as Gemini, OpenCode, or Cursor

This is the most common broad-project shape.

### 2. One Managed Repo That Also Covers More Than One Agent Family

Example:

- a team starts with Codex as the default runtime path
- later the repo also needs Claude-specific managed artifacts or Claude-oriented support

The docs must be careful here:

- this is **not** a promise of fake runtime parity between every agent
- this **is** a promise that `plugin-kit-ai` gives you one managed project model instead of separate hand-maintained target files everywhere

## The Safe Mental Model

Use this model:

1. choose the best starter for the **first** real requirement
2. treat the starter as an entrypoint, not as a cage
3. keep the repo in the managed project model
4. add the targets and managed outputs you actually need

## When To Split Repos Anyway

Separate repos still make sense when:

- teams have clearly different release cadences
- the runtime logic is unrelated between the products
- ownership boundaries are more important than shared authoring

Do **not** split repos just because the starter names are agent-specific.

## Read This Next

- [Starter Templates](/en/guide/starter-templates)
- [Choose A Starter Repo](/en/guide/choose-a-starter)
- [What You Can Build](/en/guide/what-you-can-build)
- [Target Model](/en/concepts/target-model)
