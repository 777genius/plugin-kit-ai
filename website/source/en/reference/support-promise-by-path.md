---
title: "Support Promise By Path"
description: "Compare the support promise, operational cost, and safe default status of Go, Node, Python, shell, package, and workspace-config paths."
canonicalId: "page:reference:support-promise-by-path"
section: "reference"
locale: "en"
generated: false
translationRequired: true
---

# Support Promise By Path

Use this page when the team already understands the product model and now needs one practical answer: which path carries the strongest promise, and which tradeoffs become your responsibility?

## Choose In 60 Seconds

- Need the strongest production default: choose Go.
- Need a supported interpreted runtime lane: choose Node or Python, then treat `validate --strict` and runtime bootstrap as part of your contract.
- Need a bounded escape hatch: treat shell as beta and keep it narrow.
- Need an artifact, extension, or workspace-owned config instead of executable plugin logic: choose package or workspace-config targets on purpose, not by name similarity.

## What This Page Helps You Decide

- which path is the safest default for a new team
- which path keeps the support promise strongest
- which path moves more operational cost onto your repo and execution machines
- when a target is no longer a runtime story at all

## The Short Rule

- Go is the strongest supported runtime path.
- Node and Python are supported local runtime paths, but your repo owns more runtime bootstrap.
- Shell is a narrow beta escape hatch, not a default.
- Package and workspace-config targets are real outputs, but they are not runtime contracts.

## Promise Sheet

| Path | Public promise | What your team still owns | Best default for | Avoid when |
| --- | --- | --- | --- | --- |
| Go runtime | strongest stable runtime path | normal repo, CI, and release discipline | long-lived production plugin repos | the team only wants a quick local experiment in another runtime |
| Node/TypeScript runtime | stable local runtime path on supported targets | Node.js presence, runtime bootstrap, dependency hygiene | repo-local teams already living in Node | you want the lightest operational handoff across machines |
| Python runtime | stable local runtime path on supported targets | Python presence, environment bootstrap, dependency hygiene | automation-heavy local teams already living in Python | you want zero interpreter dependency on execution machines |
| Shell runtime | bounded beta escape hatch | shell portability, narrower contract, extra caution | tightly scoped one-off escape hatches | you need the main long-term production path |
| Package or extension outputs | stable packaging-oriented output when explicitly supported | packaging workflow, release discipline, target-specific expectations | installable artifacts such as Codex package or Gemini extension outputs | you actually need executable runtime behavior |
| Workspace-config outputs | stable workspace ownership where documented | repo-owned config lifecycle and editor/tool integration checks | Cursor or OpenCode style repo-managed integration files | you need runtime handlers, not configuration files |

## How To Read The Table

- `Public promise` tells you how strong the documented support line is.
- `What your team still owns` tells you where operational cost moves onto your repo.
- `Avoid when` is there to stop the most common category mistake: treating package or workspace outputs like runtime plugins, or treating interpreted runtimes like zero-bootstrap paths.

## Safest Defaults By Situation

| Situation | Safest default |
| --- | --- |
| New team, strongest production default | Go runtime |
| Local team already committed to Node | Node/TypeScript runtime |
| Local team already committed to Python | Python runtime |
| Artifact or extension is the actual product | package or extension target |
| Repo-managed editor or tool integration is the actual product | workspace-config target |

## The Real Cost Difference

- Go removes the most downstream runtime friction.
- Node and Python keep the managed project model, but they do not remove interpreter ownership.
- Package and workspace outputs can be the correct product, but they should not be sold internally as “basically the same as runtime.”
- Shell is useful only when you accept a narrower promise on purpose.

## What People Commonly Get Wrong

- They assume every target has roughly the same runtime promise. It does not.
- They treat install wrappers like runtime APIs. They are install channels.
- They assume Node or Python are weaker because they are unofficial. They are supported, but they shift more operational cost to the repo.
- They treat package or workspace outputs as second-class. They are real supported outputs, but they solve different problems.

## Pair It With

- Read [Support Boundary](/en/reference/support-boundary) for the short stable vs beta line.
- Read [Target Support](/en/reference/target-support) for the compact target matrix.
- Read [Choosing Runtime](/en/concepts/choosing-runtime) when you are still choosing between Go, Node, Python, and shell.
- Read [Package And Workspace Targets](/en/guide/package-and-workspace-targets) when the product is not executable runtime behavior.
