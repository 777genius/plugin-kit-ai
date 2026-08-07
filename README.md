![Universal Agent Plugins](assets/hero.png)

# Universal Agent Plugins

[![Validate](https://github.com/777genius/universal-agent-plugins/actions/workflows/validate.yml/badge.svg)](https://github.com/777genius/universal-agent-plugins/actions/workflows/validate.yml)
[![Agent Plugins 1.0](https://img.shields.io/badge/Agent%20Plugins-1.0.0-7257FF)](https://agent-plugins.org/specification)
[![License](https://img.shields.io/badge/license-Apache--2.0-20A4C8)](LICENSE)

26 community-maintained plugins packaged for the
[Agent Plugins 1.0](https://agent-plugins.org/specification) open standard.
The format is supported by ChatGPT and Codex, Cursor, VS Code, GitHub Copilot,
Kiro, and other compatible agents. This repository has package-install evidence
for Codex, Cursor, and Kiro; installation still differs by client.

## Start in one minute

You only need **one** plugin. Context7 is a simple first choice and requires no
account. It requires a current Codex CLI and Node.js with `npx`:

```bash
codex plugin marketplace add 777genius/universal-agent-plugins
codex plugin add context7@universal-agent-plugins
```

Start a new Codex session and try:

```text
Use Context7 to find the current Playwright quick start and summarize it with source links.
```

That is the complete first run. Agent Code Navigator, Context7, and the other
plugins are independent alternatives, not required steps in a sequence.

Using another agent? Follow the short [client setup guide](docs/QUICKSTART.md)
for Cursor, VS Code, GitHub Copilot, Kiro, and ChatGPT.

## Pick what you need

| Plugin | What it adds | Login |
| --- | --- | --- |
| [`context7`](plugins/context7) | Current library documentation | No |
| [`agent-code-navigator`](plugins/agent-code-navigator) | Code search and architecture skills | No |
| [`cloudflare-docs`](plugins/cloudflare-docs) | Cloudflare documentation search | No |
| [`chrome-devtools`](plugins/chrome-devtools) | Browser debugging tools | Local browser |

More copy-ready examples are in [plugins to try first](docs/HERO_PLUGINS.md).
Installation status and authentication requirements are tracked in the
[test matrix](docs/TEST_MATRIX.md).

## Catalog

The repository includes plugins for code intelligence, browser automation,
design, cloud platforms, deployment, source control, project management,
databases, observability, payments, and analytics.

All 26 packages pass the standard schemas. Runtime depth is intentionally
separate: 4 packages have positive runtime behavior, 5 have authentication
discovery, 1 vendor endpoint has direct OAuth evidence, and 16 are schema-only.
The unchanged Context7 package has install/runtime evidence in three clients.

Browse all 26 packages in [`plugins/`](plugins) or check
[compatibility and authentication](docs/COMPATIBILITY.md) before connecting a
private service.

## How it works

Each portable package contains a standard `plugin.json` plus optional `mcp.json`
and Agent Skills. No package stores credentials. The portable packages under
`plugins/` are the source of truth; the OpenAI-specific layout under
[`compat/openai`](compat/openai) is generated from them.

Marketplace manifests are client adapters, not part of Agent Plugins 1.0. The
OpenAI marketplace file in this repository wraps the same portable packages;
other clients may require different catalog adapters.

Agent Plugins 1.0 standardizes packaging. Installation, OAuth, permissions, and
marketplace review are still managed by each client. The 1.0.0 specification is
published; verified behavior and current limits are recorded in
[verification](docs/VERIFICATION.md) and the [client matrix](docs/CLIENTS.md).

## Safety

- Review a plugin's tools and scopes before enabling it.
- Start with read-only tasks, especially after OAuth.
- Never place tokens in `plugin.json`, `mcp.json`, or committed headers.
- A valid package can still expose destructive tools.

See [SECURITY.md](SECURITY.md) for reporting and security boundaries.

## Project

This repository rebuilds the portable subset of
[`universal-plugins-for-ai-agents`](https://github.com/777genius/universal-plugins-for-ai-agents)
without `plugin-kit-ai` as its authoring layer. Contributions are welcome; see
[CONTRIBUTING.md](CONTRIBUTING.md).

This is an independent community project maintained by 777genius. It is not
affiliated with or endorsed by OpenAI or the vendors represented in the catalog.
Original project material is licensed under [Apache 2.0](LICENSE).
