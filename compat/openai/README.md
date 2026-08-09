# OpenAI compatibility layer

Agent Plugins 1.0 uses root `plugin.json` and `mcp.json`. The
[official OpenAI plugin packaging contract](https://developers.openai.com/plugins/build/plugins)
uses a host-specific `.codex-plugin/plugin.json`. A remote MCP registered in
ChatGPT Developer Mode is referenced by a plugin-root `.app.json`, and the
manifest's compatibility `apps` field must point to `./.app.json`.

This directory is generated from the portable packages under `plugins/`:

```bash
python3 scripts/build_openai_compat.py
python3 scripts/build_openai_compat.py --check
python3 scripts/validate_openai_compat.py
```

Do not edit generated package files here. Update the portable source package and
the generator instead. OpenAI-only auth metadata is maintained explicitly in
the generator. Registered ChatGPT development connections are separately
allowlisted in `app-bindings.json`; this host-only sidecar is not part of the
portable package and must match one exact Streamable HTTP endpoint.

Only `cloudflare-docs` currently has a registered no-auth development binding.
The generated package passes static validation, and the direct connection has a
sanitized read-only runtime record. Installation of this repository package in
the ChatGPT Plugins UI is still pending and is not implied by either result.

The repo marketplace is at `.agents/plugins/marketplace.json`. These adapters
are intended for local compatibility testing. They are not public listings in
OpenAI's Plugins Directory and do not include the logos, legal pages, verified
publisher identity, domain verification, or review materials required for a
public submission.
