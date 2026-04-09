---
title: "Choose What You Are Building"
description: "Pick the right plugin-kit-ai starting path before you think about target taxonomy."
canonicalId: "page:guide:choose-what-you-are-building"
section: "guide"
locale: "en"
generated: false
translationRequired: true
---

# Choose What You Are Building

Start with the job first. You do not need to understand `codex-package`, `runtime lanes`, or `local MCP over stdio` before you create the repo.

## Connect An Online Service

Use this when the plugin should connect to a hosted service like Notion, Stripe, Cloudflare, or Vercel.

```bash
plugin-kit-ai init my-plugin --template online-service
```

This creates:

- one authored repo under `src/`
- shared portable MCP source
- generated outputs for the supported package and workspace targets
- no launcher code by default

## Connect A Local Tool

Use this when the plugin should call into a repo-owned CLI, container, or local executable tool like Docker Hub, Chrome DevTools, or HubSpot Developer.

```bash
plugin-kit-ai init my-plugin --template local-tool
```

This creates:

- one authored repo under `src/`
- shared local tool MCP wiring
- generated outputs for the supported package and workspace targets
- no launcher code by default

## Build Custom Plugin Logic

Use this when the product is defined by hooks, runtime behavior, or custom code.

```bash
plugin-kit-ai init my-plugin --template custom-logic
```

This keeps the strongest backward-compatible runtime-first path and maps to the current launcher-backed authoring model.

## What To Do Next

After any of those starts:

```bash
cd my-plugin
plugin-kit-ai inspect . --authoring
plugin-kit-ai generate .
plugin-kit-ai generate --check .
```

Then validate the supported output you actually plan to ship first.

## When To Open The Advanced Pages

- Open [Quickstart](/en/guide/quickstart) when you want the shortest first-run flow.
- Open [Choose A Target](/en/guide/choose-a-target) when you need target-specific shipping decisions.
- Open [What You Can Build](/en/guide/what-you-can-build) when you want the full product map.
