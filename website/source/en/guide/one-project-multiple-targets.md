---
title: "One Project, Multiple Targets"
description: "How one plugin repo can grow into more supported outputs without splitting into separate setups."
canonicalId: "page:guide:one-project-multiple-targets"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# One Project, Multiple Targets

This is one of the most important ideas in `plugin-kit-ai`: start with the best first repo for today, then expand that same repo into more supported outputs later.

Do not confuse the starter family with the long-term limit of the repo.

## The Short Rule

Start with the runtime or target that is your **primary requirement today**.

After that, keep one repo, keep one validation story, and add only the outputs you actually need.

<MermaidDiagram
  :chart="`
flowchart LR
  Repo[One repo] --> Generate[generate]
  Generate --> CodexRuntime[codex-runtime]
  Generate --> Claude[claude]
  Generate --> CodexPackage[codex-package]
  Generate --> Gemini[gemini]
  Generate --> OpenCode[opencode]
  Generate --> Cursor[cursor]
`"
/>

That means a project can begin as:

- a Codex-first plugin repo
- a Claude-first plugin repo
- a package/config-first repo

and still grow into a broader multi-output repo over time.

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

The unifying part is the repo and workflow.

Your team keeps one repo and then uses `generate`, `validate`, import/normalize flows, and target directories to manage the outputs that matter.

In practice, the unified part is:

- one repo layout
- one build-and-maintain workflow
- one validation story
- one CI story
- one place to review generated target outputs

## What “Multiple Targets” Means In Practice

There are two common cases.

### 1. One Primary Runtime, Several Additional Outputs

Example:

- your main plugin behavior is Codex runtime
- but the same repo also renders package/config targets such as Gemini, OpenCode, or Cursor

This is the most common broad-project shape.

### 2. One Repo That Also Covers More Than One Agent Family

Example:

- a team starts with Codex as the default runtime path
- later the repo also needs Claude-specific outputs or Claude-oriented support

The docs must be careful here:

- this is **not** a promise of fake runtime parity between every agent
- this **is** a promise that `plugin-kit-ai` gives you one repo and one workflow instead of separate hand-maintained target files everywhere

## The Safe Mental Model

Use this model:

1. choose the best starter for the **first** real requirement
2. treat the starter as an entrypoint, not as a cage
3. keep the repo unified
4. add the targets and outputs you actually need

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
