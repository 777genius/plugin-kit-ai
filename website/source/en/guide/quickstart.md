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

This is the shortest recommended path when you want one plugin repo that can later grow into more ways to ship the plugin.

Start with one strong path first. Add packages, extensions, or repo-owned integration setup later when the product actually needs them.

## If You Only Read One Thing

Start with the default Go path unless you already know that Claude hooks, Node/TypeScript, or Python define the product requirement.

Your first choice is the starting point, not the permanent boundary of the repo.

## Recommended Default

If you do not have a strong reason to choose another path, start here:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai generate .
plugin-kit-ai validate . --platform codex-runtime --strict
```

That gives you the strongest default path today: a Go-based Codex runtime repo that stays easy to validate, hand off, and expand later.

## Why This Is The Default

- one repo from day one
- the cleanest runtime and release story today
- the easiest base for later package, extension, and integration lanes

## What You Get

- one plugin repo from day one
- authored files under `src/`
- generated Codex runtime output from the same repo
- a clean readiness check through `validate --strict`

## Supported Node And Python Paths

If your team already lives in Node/TypeScript or Python, those paths are supported and visible from the start:

- `codex-runtime --runtime node --typescript`
- `codex-runtime --runtime python`
- both are local interpreted runtime paths, so the target machine still needs Node.js `20+` or Python `3.10+`
- Go still stays the default when you want the strongest general production story

## If You Are Intentionally Starting On Node Or Python

Use this alternate flow only when the language choice is already part of the product requirement:

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai generate ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

Or start with Python:

```bash
plugin-kit-ai init my-plugin --platform codex-runtime --runtime python
plugin-kit-ai doctor ./my-plugin
plugin-kit-ai bootstrap ./my-plugin
plugin-kit-ai generate ./my-plugin
plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## What To Do Next

- edit the plugin under `src/`
- run `plugin-kit-ai generate ./my-plugin` again after changes
- run `plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict` again
- only then add another way to ship it if the product needs that

## Expand Later

| If you want | Add this later |
| --- | --- |
| Claude hooks as the real product | `claude` |
| Official Codex package | `codex-package` |
| Gemini extension package | `gemini` |
| Repo-owned integration setup | `opencode` or `cursor` |

Choose `claude` first only when Claude hooks are already the real product requirement.

## What Expands Later

- the repo stays unified as you add more lanes
- package and extension lanes come from the same authored source
- OpenCode and Cursor fit when the repo should own integration setup
- the exact support boundary stays in the reference docs, not in your first-start flow

## After Quickstart

- Continue with [Build Your First Plugin](/en/guide/first-plugin) if you want the narrowest recommended tutorial.
- Continue with [What You Can Build](/en/guide/what-you-can-build) if you want the full product map.
- Continue with [Choose A Target](/en/guide/choose-a-target) when you are ready to match the repo to how you want to ship it.
- Continue with [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) when you are ready to expand beyond the first path.
