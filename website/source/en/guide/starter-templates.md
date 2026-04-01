---
title: "Starter Templates"
description: "Official starter repositories for common plugin-kit-ai entrypoints, not the limit of the managed project model."
canonicalId: "page:guide:starter-templates"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Starter Templates

If you want a known-good starting point instead of scaffolding from a blank directory, use the official starter repositories.

## Choose In 60 Seconds

- Choose **Go** when you want the strongest self-contained production path.
- Choose **Node/TypeScript** when your team wants the main supported non-Go path.
- Choose **Python** only when the repo is intentionally Python-first and stays repo-local.
- Choose **Codex** or **Claude** based on the first real target you must support, not on what you might support someday.

## Important: Starters Are Entry Points

The starter names are intentionally split by primary path such as Codex or Claude.

That does **not** mean the product model is permanently locked to one agent family.

A starter helps you choose the best first shape for:

- your primary runtime requirement
- your team language choice
- your first supported target

After that, keep the repo in the managed project model and grow it as needed.

Read [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) if you want the broader system view.

## Best Defaults

- Strongest Codex default: [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- Strongest Claude default: [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- Main supported non-Go Codex path: [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)
- Main supported non-Go Claude path: [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

## Codex Runtime

- [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- [plugin-kit-ai-starter-codex-python](https://github.com/777genius/plugin-kit-ai-starter-codex-python)
- [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)

## Claude

- [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- [plugin-kit-ai-starter-claude-python](https://github.com/777genius/plugin-kit-ai-starter-claude-python)
- [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

## Shared-Package Reference Starters

These are useful when you already know you want the shared `plugin-kit-ai-runtime` dependency instead of vendored helper files.

- [codex-python-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/codex-python-runtime-package-starter)
- [claude-node-typescript-runtime-package-starter](https://github.com/777genius/plugin-kit-ai/tree/main/examples/starters/claude-node-typescript-runtime-package-starter)

These are reference starters in the main repo, not separate GitHub template repos.

## When To Prefer A Starter

Use a starter when:

- you want a known-good repo layout immediately
- you want to compare your project against a minimal supported example
- you want to onboard teammates with less scaffolding guesswork

Use `plugin-kit-ai init` directly when:

- you want a fresh repo from first principles
- you need to choose flags explicitly
- you are building around an existing repository structure

## Practical Rule

- Use a **template repo** when you want the cleanest public "Use this template" flow.
- Use a **starter in the main repo** when you want to inspect the canonical source, compare layouts, or start from the shared-package reference path.
- Use **`plugin-kit-ai init`** when you already have a repo and want to adopt the managed project model without copying a starter.

## Safe Mental Model

- choose a starter for the **first** correct path
- do not treat the starter family as the final boundary of the repo
- treat the managed project model as the long-term source of truth

Pair this page with [Choose A Starter Repo](/en/guide/choose-a-starter), [Examples And Recipes](/en/guide/examples-and-recipes), and [Managed Project Model](/en/concepts/managed-project-model).
