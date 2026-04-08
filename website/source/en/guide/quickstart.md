---
title: "Quickstart"
description: "The fastest recommended path to a working plugin-kit-ai project."
canonicalId: "page:guide:quickstart"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Quickstart

This is the shortest recommended path when you want one plugin repo that can later expand into more delivery lanes.

Start with one strong lane first. Add package, extension, or repo-managed integration lanes later when the product actually needs them.

## If You Only Read One Thing

Start with the default Go lane unless you already know that Claude hooks, Node/TypeScript, or Python define the product requirement.

Your first lane is the starting point, not the permanent boundary of the repo.

## Recommended Default

If you do not have a strong reason to choose another lane, start here:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai generate .
plugin-kit-ai validate . --platform codex-runtime --strict
```

That gives you the strongest default lane today: a Go-based Codex runtime repo that stays easy to validate, hand off, and expand later.

## Why This Is The Default

- one repo from day one
- the cleanest runtime and release story today
- the easiest base for later package, extension, and integration lanes

## Choose The First Lane

| If you want | Recommended first lane |
| --- | --- |
| Strongest runtime lane | `codex-runtime` with `--runtime go` |
| Official Codex package | `codex-package` |
| Gemini extension package | `gemini` |
| Repo-local TypeScript runtime | `codex-runtime --runtime node --typescript` |
| Repo-local Python runtime | `codex-runtime --runtime python` |

Choose `claude` first only when Claude hooks are already the real product requirement.

## Common First Commands

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai generate ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## Read This Before Choosing Python Or Node

- Python and Node are recommended local runtime lanes for teams that already live in those stacks.
- They still require Python `3.10+` or Node.js `20+` on the machine that runs the plugin.
- Go remains the recommended default when you want the strongest runtime and distribution story.

## What Expands Later

- the repo stays unified as you add more lanes
- package and extension lanes come from the same authored source
- OpenCode and Cursor fit when the repo should own integration config
- the exact support boundary stays in the reference docs, not in your first-start flow

## After Quickstart

- Continue with [Build Your First Plugin](/en/guide/first-plugin) if you want the narrowest recommended tutorial.
- Continue with [What You Can Build](/en/guide/what-you-can-build) if you want the full product map.
- Continue with [Choose A Target](/en/guide/choose-a-target) when you are ready to match the repo to a delivery model.
- Continue with [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) when you are ready to expand beyond the first lane.
