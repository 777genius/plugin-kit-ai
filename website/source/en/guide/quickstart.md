---
title: "Quickstart"
description: "The fastest supported path to a working plugin-kit-ai project."
canonicalId: "page:guide:quickstart"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Quickstart

This is the shortest supported path when you want a real plugin repo quickly, without hand-authoring target files.

It intentionally shows one recommended starting path, not the final limit of the product.

`plugin-kit-ai` is designed as a managed project model where one repo can own multiple targets and output shapes from one authored source of truth.

## If You Only Read One Thing

Start with the default Go path unless you already know you need Claude hooks, Node/TypeScript, or Python.

But do not confuse the starting path with a permanent limit: choosing the first target does not ban the others forever.

## Recommended Default

If you do not have a strong reason to choose another path, start here:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai render .
plugin-kit-ai validate . --platform codex-runtime --strict
```

That gives you the strongest default path:

- platform: `codex-runtime`
- runtime: `go`
- contract: public-stable default authoring path

## Choose The Right Path

| Goal | Best starting path |
| --- | --- |
| Strongest production path | `codex-runtime` with `--runtime go` |
| Claude runtime plugin | `claude` |
| Repo-local Python plugin | `codex-runtime --runtime python` |
| Repo-local TypeScript plugin | `codex-runtime --runtime node --typescript` |
| Official Codex package output | `codex-package` |
| Gemini extension packaging | `gemini` |
| OpenCode workspace config | `opencode` |
| Cursor workspace config | `cursor` |

If the product needs several targets, still start with the primary requirement today and then expand the same managed repo.

## Common First Commands

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai render ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## Read This Before Choosing Python Or Node

- Python and Node are supported first-class for the stable repo-local subset.
- They still require Python `3.10+` or Node.js `20+` on the machine that runs the plugin.
- Go remains the recommended default when you want the cleanest production and distribution story.

## After Quickstart

- Continue with [Build Your First Plugin](/en/guide/first-plugin) if you want the narrowest recommended tutorial.
- Continue with [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) if the multi-target path is one of the main reasons you care about the product.
- Continue with [What You Can Build](/en/guide/what-you-can-build) if you are still comparing product shapes.
- Continue with [Choose A Target](/en/guide/choose-a-target) if you understand the product but are still deciding between Codex, Claude, Gemini, Cursor, or OpenCode.
- Continue with [Choose A Starter Repo](/en/guide/choose-a-starter) if you want to start from a template instead of a blank repo.

See [Choosing Runtime](/en/concepts/choosing-runtime) for the decision model and [Installation](/en/guide/installation) for CLI install channels.
