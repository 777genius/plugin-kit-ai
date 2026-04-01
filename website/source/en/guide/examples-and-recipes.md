---
title: "Examples And Recipes"
description: "A guided map of the public example repos, starter repos, local runtime references, and skill examples in plugin-kit-ai."
canonicalId: "page:guide:examples-and-recipes"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Examples And Recipes

Use this page when you want to see what `plugin-kit-ai` looks like in real repositories instead of only reading abstract guidance.

## 1. Production Plugin Examples

These are the clearest examples of finished public shapes:

- `codex-basic-prod`: Codex runtime production repo
- `claude-basic-prod`: Claude production repo
- `codex-package-prod`: Codex package target
- `gemini-extension-package`: Gemini extension packaging target
- `cursor-basic`: Cursor workspace-config target
- `opencode-basic`: OpenCode workspace-config target

Read these when you want:

- a concrete repo layout
- real rendered outputs
- a truthful public example of what “healthy” looks like

Important: these examples show distinct public product shapes. They do not imply that a real system must be split into a separate repo for every target.

## 2. Starter Repos

Use starter repos when you want to begin from a known-good baseline instead of from an empty directory.

They are best for:

- first-time setup
- team onboarding
- choosing between Go, Python, Node, Claude, and Codex starting points

But do not confuse the starter catalog with a product limitation: one managed repo can still grow to own multiple targets later.

If you are still choosing, pair this with [Choose A Starter Repo](/en/guide/choose-a-starter).

## 3. Local Runtime References

The `examples/local` area shows repo-local Python and Node runtime references.

These are useful when:

- you want to understand the interpreted runtime story more deeply
- you want to compare JavaScript, TypeScript, and Python local-runtime setups
- you need a concrete reference beyond the starter repos

## 4. Skill Examples

The `examples/skills` area shows supporting skill examples and helper integrations.

These are not the main entrypoint for most plugin authors, but they are valuable when:

- you want to wire docs, review, or formatting helpers into the broader workflow
- you want to understand how adjacent skills can fit around plugin repos

## Suggested Reading By Goal

- Want the strongest runtime example: start with the Codex or Claude production example, then read [Build A Team-Ready Plugin](/en/guide/team-ready-plugin).
- Want packaging or workspace-config examples: start with Codex package, Gemini, Cursor, or OpenCode examples, then read [Package And Workspace Targets](/en/guide/package-and-workspace-targets).
- Want a clean starting point, not a finished example: go to [Starter Templates](/en/guide/starter-templates).
- Want to choose the product target before looking at repos: read [Choose A Target](/en/guide/choose-a-target).
- Want the full product map first: read [What You Can Build](/en/guide/what-you-can-build).

## Final Rule

Examples should clarify the public contract, not replace it.

Use example repos to see shape, layout, and healthy outputs. Use the rest of the docs to understand what is stable, what is optional, and what the project actually promises.

If you want to understand how those shapes can live inside the same repo, read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets).
