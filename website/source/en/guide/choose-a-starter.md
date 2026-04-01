---
title: "Choose A Starter Repo"
description: "A practical matrix for choosing the right official starter by target, runtime, and delivery path."
canonicalId: "page:guide:choose-a-starter"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Choose A Starter Repo

Use this page when you want the fastest path to a known-good repo and do not want to guess the right starter from template names alone.

Before you choose, remember one important rule:

- the starter tells you how to begin
- it does not tell you the final limit of the project

If that distinction is still fuzzy, read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) first.

## Choose In 60 Seconds

- choose Go when you want the strongest production path
- choose Node/TypeScript when you want the main supported non-Go path
- choose Python when the repo is intentionally Python-first and stays local to the repo
- choose Claude starters only when Claude hooks are the actual product requirement

## Best Defaults

- Best general Codex default: [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- Best general Claude default: [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- Best non-Go Codex default: [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)
- Best non-Go Claude default: [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

## Quick Decision Tree

1. Need the strongest self-contained delivery path? Choose Go.
2. Need the main supported non-Go path? Choose Node/TypeScript.
3. Need a repo-local Python-first path on purpose? Choose Python.
4. Need Claude hook coverage as the first real target? Choose Claude. Otherwise start from Codex.

## Starter Matrix

| Goal | Best starter family | Why |
| --- | --- | --- |
| Strongest Codex production path | [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go) | Go-first production path with the cleanest handoff story |
| Repo-local Codex plugin in Python | [plugin-kit-ai-starter-codex-python](https://github.com/777genius/plugin-kit-ai-starter-codex-python) | Stable Python subset with a known-good repo layout |
| Repo-local Codex plugin in Node/TS | [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript) | Main supported non-Go path |
| Strongest Claude production path | [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go) | Stable Claude subset plus the cleanest production path |
| Repo-local Claude plugin in Python | [plugin-kit-ai-starter-claude-python](https://github.com/777genius/plugin-kit-ai-starter-claude-python) | Stable Claude hook subset with Python helpers |
| Repo-local Claude plugin in Node/TS | [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript) | Stable Claude hook subset for TypeScript-first teams |

## When To Use Shared-Package Reference Starters

Use the shared-package reference starters when you already know that your team wants `plugin-kit-ai-runtime` as a reusable dependency instead of vendored helper files.

That path is best when:

- you want a shared dependency across multiple plugin repos
- you are comfortable pinning and upgrading the runtime package explicitly
- you do not want helper files copied into every repo

Reference starters:

- [codex-python-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/codex-python-runtime-package-starter)
- [claude-node-typescript-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/claude-node-typescript-runtime-package-starter)

## When To Avoid Over-Optimizing The Choice

Do not spend too long searching for the perfect starter.

If you are unsure:

1. start with the Go starter for the strongest default
2. start with the Node/TypeScript starter for the main supported non-Go path
3. only choose Python or shared-package variants when the team tradeoff is already real

## Good Team Policy

A team-wide starter choice should stay consistent long enough that:

- everyone recognizes the repo layout
- CI uses the same readiness flow
- handoff does not depend on maintainer explanation

Pair this page with [Starter Templates](/en/guide/starter-templates), [Choose Delivery Model](/en/guide/choose-delivery-model), and [Repository Standard](/en/reference/repository-standard).
