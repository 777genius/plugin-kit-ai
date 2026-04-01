---
title: "Managed Project Model"
description: "The core product model behind plugin-kit-ai: one authored repo, rendered outputs, strict validation, and honest path boundaries."
canonicalId: "page:concepts:managed-project-model"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Managed Project Model

If you only remember one idea about `plugin-kit-ai`, remember this:

- it is a managed plugin project system, not just a starter collection and not just a CLI

## In One Sentence

You keep one authored repo, render the outputs each path needs, validate the result strictly, and only standardize the paths whose support promise matches your team.

## What Stays Constant

These parts should stay stable in how you think about the product:

- one repo is the authored source of truth
- rendered files are outputs, not the long-term editing surface
- `validate --strict` matters before handoff and rollout
- support promises differ by path and should be chosen deliberately

## What Can Change

The repo can still grow and change without breaking the model:

- you can start from a starter repo or from `plugin-kit-ai init`
- you can keep one primary target or grow to more than one output shape
- you can stay on the strongest Go runtime path or adopt supported Node or Python lanes
- you can add package, extension, or workspace-config targets when the repo really owns those outputs

## Why This Is Not Just A Starter Story

Starters matter, but they are only the entrypoint.

The product promise is not "pick a starter name and stay inside that starter forever".

The product promise is:

- author the project in one place
- render the outputs the chosen path needs
- validate the repo against the declared standard
- keep support boundaries explicit instead of pretending every path is equally strong

## The Four-Step Loop

The public workflow is easier to understand when you collapse it into four steps:

1. Author the managed repo.
2. Render the output files for the chosen target and delivery model.
3. Validate strictly before handoff, CI, or rollout.
4. Expand only when the next path is justified by a real team or product need.

## What Good Teams Standardize

Healthy teams do not standardize "whatever happened to work once".

They standardize:

- one reference repo shape
- one clear primary path
- one explicit support story
- one repeatable validation gate

Everything else should be treated as an exception, an extension, or a later rollout decision.

## Read This With

- Read [Why plugin-kit-ai](/en/concepts/why-plugin-kit-ai) for the problem and tradeoff framing.
- Read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) for the guide-level explanation of how one repo can support more than one rendered output shape.
- Read [Target Model](/en/concepts/target-model) when you need the exact distinction between runtime, package, extension, and workspace-configuration targets.
- Read [Support Boundary](/en/reference/support-boundary) when your team needs the public contract, not just the mental model.
