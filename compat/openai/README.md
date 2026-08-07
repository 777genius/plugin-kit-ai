# OpenAI compatibility layer

Agent Plugins 1.0 uses root `plugin.json` and `mcp.json`. Current OpenAI builder
documentation also describes a host-specific `.codex-plugin/plugin.json` and
`.mcp.json` package layout for local marketplaces and directory submission.

This directory is generated from the portable packages under `plugins/`:

```bash
python3 scripts/build_openai_compat.py
python3 scripts/build_openai_compat.py --check
python3 scripts/validate_openai_compat.py
```

Do not edit generated package files here. Update the portable source package and
the generator instead. OpenAI-only auth metadata cannot be represented by Agent
Plugins 1.0 and is therefore maintained explicitly in the generator. Currently
this covers the published GitHub, Figma, Linear, and Notion host packages.

The repo marketplace is at `.agents/plugins/marketplace.json`. These adapters
are intended for local compatibility testing. They are not public listings in
OpenAI's Plugins Directory and do not include the logos, legal pages, verified
publisher identity, domain verification, or review materials required for a
public submission.
