---
title: "Build Custom Plugin Logic"
description: "The advanced path for plugins whose value lives in runtime code, hooks, and orchestration."
canonicalId: "page:guide:build-custom-plugin-logic"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Build Custom Plugin Logic

Choose this path when the plugin is not just wiring an existing service or local tool.

This is the advanced path for repos whose value lives in:

- runtime code you own
- hooks and orchestration logic
- policy, transformation, or guardrail behavior
- custom plugin behavior that would not exist without your code

If you are connecting a hosted service like Notion or Stripe, use [Choose What You Are Building](/en/guide/choose-what-you-are-building) and start with `online-service` instead.
If you are connecting a local tool like Docker Hub or HubSpot Developer, start with `local-tool` instead.

## Start Here

```bash
plugin-kit-ai init my-plugin --template custom-logic
cd my-plugin
plugin-kit-ai inspect . --authoring
plugin-kit-ai validate . --platform codex-runtime --strict
plugin-kit-ai test . --platform codex-runtime --event Notify
```

## What You Edit

The authored source of truth lives under `plugin/`.

The important files are usually:

- `plugin/plugin.yaml`
- `plugin/launcher.yaml`
- `plugin/targets/...`
- your runtime entrypoint such as `cmd/<name>/main.go` or `plugin/main.*`

Use `plugin-kit-ai inspect . --authoring` when you want the exact split between editable source, managed guidance files, and generated target outputs.

## What Gets Generated

`plugin-kit-ai generate` still owns the generated output files at the repo root.

That usually includes:

- root guidance files such as `README.md`, `CLAUDE.md`, `AGENTS.md`, and `GENERATED.md`
- native output for the target you are shipping, such as `.codex/config.toml`, `hooks/hooks.json`, or `gemini-extension.json`

Edit the source under `plugin/`.
Treat the root outputs as managed outputs.

## Why This Path Is More Advanced

Compared with `online-service` and `local-tool`, this path gives you:

- more control over behavior
- more responsibility for the runtime contract
- more room for tests, hooks, and policy logic

That is why it is visible on the first screen, but marked as an advanced path.

## First Run By Runtime Shape

### Go runtime

```bash
go test ./...
plugin-kit-ai validate . --platform codex-runtime --strict
plugin-kit-ai test . --platform codex-runtime --event Notify
```

### Node or Python runtime

```bash
plugin-kit-ai doctor .
plugin-kit-ai bootstrap .
plugin-kit-ai validate . --platform codex-runtime --strict
plugin-kit-ai test . --platform codex-runtime --event Notify
```

## How To Go Deeper

- Open [Quickstart](/en/guide/quickstart) when you want to compare this path against the simpler job-first starters.
- Open [Build Your First Plugin](/en/guide/first-plugin) when you intentionally want the narrow legacy-compatible Codex runtime tutorial.
- Open [Choose A Target](/en/guide/choose-a-target) when you need target-specific shipping decisions.
- Open [One Project, Multiple Targets](/en/guide/one-project-multiple-targets) when the repo is ready to grow into more outputs.
