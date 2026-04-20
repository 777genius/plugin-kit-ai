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

## Recommended Default

The recommended default for new repos is the job-first path below.

## Start With The Job

Pick the repo shape by what you are building:

- online service: `plugin-kit-ai init my-plugin --template online-service`
- local tool: `plugin-kit-ai init my-plugin --template local-tool`
- custom logic - Advanced: `plugin-kit-ai init my-plugin --template custom-logic`

If you want the shortest decision page first, read [Choose What You Are Building](/en/guide/choose-what-you-are-building).

## Optional First Proof

If you want the fastest first proof that the published install flow is real, start here:

```bash
npx plugin-kit-ai@latest add notion --target claude
npx plugin-kit-ai@latest add notion
```

- The first command is the safe single-target path.
- The second installs every supported output for that plugin.
- This optional proof does not create the repo you will edit next.
- If your goal is to author a plugin repo, skip this proof and continue with the job-first `init` path above.

## If You Only Read One Thing

Start with the job-first path above.

Your first choice is the starting point, not the permanent boundary of the repo.

## Legacy Compatibility Path

Use this only when you are intentionally maintaining the older Codex runtime Go path or matching existing docs and scripts:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
plugin-kit-ai init my-plugin
cd my-plugin
go mod tidy
plugin-kit-ai generate .
plugin-kit-ai validate . --platform codex-runtime --strict
```

That keeps the older Codex runtime Go path working, but it is not the recommended first-run path for new repos anymore.

## Install The CLI For Daily Use

If you plan to use plugin-kit-ai every day, install the CLI permanently:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

## Why This Path Still Exists

- backward compatibility for older docs and scripts
- a stable Go-based Codex runtime path when you already need it
- a migration bridge, not the main recommendation for new users

## What You Get

- one plugin repo from day one
- authored files under `plugin/`
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

- edit the plugin under `plugin/`
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

- Continue with [Choose What You Are Building](/en/guide/choose-what-you-are-building) if you want the shortest decision page for new repos.
- Continue with [Build Custom Plugin Logic](/en/guide/build-custom-plugin-logic) if you are intentionally taking the advanced runtime path.
- Continue with [Build Your First Plugin](/en/guide/first-plugin) if you specifically want the narrow legacy-compatible Codex runtime tutorial.
- Continue with [What You Can Build](/en/guide/what-you-can-build) if you want the full product map.
- Continue with [Choose A Target](/en/guide/choose-a-target) when you are ready to match the repo to how you want to ship it.
- Continue with [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) when you are ready to expand beyond the first path.
