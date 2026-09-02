![Universal Agent Plugins](assets/hero.png)

# Universal Agent Plugins

[![npm](https://img.shields.io/npm/v/universal-agent-plugins?color=7257FF)](https://www.npmjs.com/package/universal-agent-plugins)
[![Validate](https://github.com/777genius/universal-agent-plugins/actions/workflows/validate.yml/badge.svg)](https://github.com/777genius/universal-agent-plugins/actions/workflows/validate.yml)
[![Live E2E](https://github.com/777genius/universal-agent-plugins/actions/workflows/live-e2e.yml/badge.svg)](https://github.com/777genius/universal-agent-plugins/actions/workflows/live-e2e.yml)
[![Agent Plugins 1.0](https://img.shields.io/badge/Agent%20Plugins-1.0.0-7257FF)](https://agent-plugins.org/specification)
[![License](https://img.shields.io/badge/license-Apache--2.0-20A4C8)](LICENSE)

**Install an [Agent Plugins 1.0](https://agent-plugins.org/specification)
package across one or several AI agents with one CLI.**

```bash
npx universal-agent-plugins add context7 --target codex,cursor,kiro
```

The CLI downloads and verifies the plugin once, prepares it for every selected
client, and tells you if an agent needs a restart, import, sign-in, or OAuth
confirmation. It also updates, repairs, switches, and removes plugins without
making you edit each agent's configuration by hand.

## Quick start

You need [Node.js 22+](https://nodejs.org/). Context7 is a useful first plugin:
it finds current library documentation and needs no account.

```bash
npx universal-agent-plugins add context7
```

In an interactive terminal, the CLI detects installed supported agents and lets
you select one or several. In scripts, choose targets explicitly:

```bash
npx universal-agent-plugins add context7 --target cursor
npx universal-agent-plugins add context7 --target codex,cursor
npx universal-agent-plugins add context7 --target codex,cursor,kiro
```

Open a new chat in the selected agent and ask it to use Context7 for current
documentation. That's it.

## One CLI for the whole lifecycle

```bash
npx universal-agent-plugins update context7 --target codex,cursor,kiro
npx universal-agent-plugins repair context7 --target codex,cursor,kiro
npx universal-agent-plugins remove context7 --target codex,cursor,kiro
npx universal-agent-plugins switch cloudflare-docs --to 777genius/cloudflare-docs
```

| Command | What it does |
| --- | --- |
| `add` | Installs or prepares a plugin for the selected agents |
| `update` | Updates it from the same recorded source |
| `repair` | Restores missing or changed managed files |
| `remove` | Removes only files managed by the CLI |
| `switch` | Deliberately changes to another publisher or distribution |
| `doctor` | Checks the CLI, supported clients, and local state |

## Supported agents

| Agent | CLI integration |
| --- | --- |
| <img src="assets/client-icons/openai.svg" width="20" height="20" alt=""> Codex | Prepares the official plugin layout and prints any activation step |
| <img src="assets/client-icons/openai.svg" width="20" height="20" alt=""> ChatGPT | Prepares a verified app connection; confirmation stays visible in ChatGPT |
| <img src="assets/client-icons/cursor.svg" width="20" height="20" alt=""> Cursor | Installs the native Agent Plugin |
| <img src="assets/client-icons/github-copilot.svg" width="20" height="20" alt=""> GitHub Copilot CLI | Installs and verifies the native plugin |
| <img src="assets/client-icons/vscode.svg" width="20" height="20" alt=""> VS Code | Uses Copilot setup when available or prints the exact setting |
| <img src="assets/client-icons/kiro.svg" width="20" height="20" alt=""> Kiro | Prepares the native package and prints the import step |
| <img src="site/public/client-icons/claude.svg" width="20" height="20" alt=""> Claude Code | Manages the plugin and MCP configuration |
| <img src="site/public/client-icons/gemini.svg" width="20" height="20" alt=""> Gemini CLI | Manages the MCP configuration |
| <img src="site/public/client-icons/opencode.svg" width="20" height="20" alt=""> OpenCode | Manages the MCP configuration |
| <img src="site/public/client-icons/cline.svg" width="20" height="20" alt=""> Cline | Manages the MCP configuration and prints reload guidance |
| <img src="site/public/client-icons/windsurf.svg" width="20" height="20" alt=""> Windsurf | Prepares supported components and prints the activation step |

Agent activation and service authentication remain client-specific. The CLI
shows those steps instead of silently claiming that a prepared package is
already running.

## Find a plugin

Search more than **2500 Agent Plugins** discovered from public GitHub
repositories, plus 26 reviewed starter packages maintained here:

```bash
npx universal-agent-plugins search docs
npx universal-agent-plugins info context7
npx universal-agent-plugins add cloudflare-docs --target codex,cursor,kiro
```

Browse the optional [web directory](https://777genius.github.io/universal-agent-plugins/)
when you want a visual search experience. The Directory is an additional source
for short names, not the product's core: the CLI works without copying every
plugin into this repository.

## Install any Agent Plugin

The CLI is not limited to this Directory. It accepts a local Agent Plugins 1.0
package or an immutable GitHub source:

```bash
npx universal-agent-plugins validate ./my-plugin
npx universal-agent-plugins add ./my-plugin --target cursor
npx universal-agent-plugins add \
  777genius/universal-agent-plugins@2ddbb99dd190c1792b79904f9875e6322bccd243//plugins/cloudflare-docs \
  --target cursor
```

External packages do not need to be copied into it. Pin GitHub sources to a full
commit SHA so the installed bytes are reproducible.

Source labels stay explicit:

- **upstream**: the package lives in its owner's repository;
- **community bridge**: reviewed metadata is combined with pinned upstream content;
- **community**: a community-authored distribution;
- **direct source**: a local path or exact GitHub reference that bypasses short-name lookup.

Community packages and bridges are not official vendor packages. Search results
from automatic discovery are marked **unreviewed**, pinned to an exact commit,
and validated again before the CLI changes any client.

## How it works

```text
npx universal-agent-plugins
        ↓
verified Agent Plugins 1.0 package
        ↓
one installation plan for the selected agents
        ↓
client-specific adapters + clear activation guidance
```

This repository is the product home and public source for the npm facade.
`universal-agent-plugins` is the npm package; `agentplugins` is the installed
command. The facade installs the `agentplugins` binary, verifies its SHA-256,
and caches the correct build for the current platform.

[`plugin-kit-ai`](https://github.com/777genius/plugin-kit-ai) is the shared Go
implementation engine. It contains the package loader, lifecycle, and client
adapters; that engine is not duplicated in this repository.

## Trust and current evidence

All 26 packages pass standard schema validation. Historical evidence includes
15/15 runtime checks for five starter packages across Codex, Cursor, and Kiro,
with Notion OAuth tested in those three clients; Figma OAuth was tested
separately in Codex only. A materialized or installed package does not by itself
prove activation, tool runtime, or OAuth. ChatGPT and Copilot claims are narrower
and are not generalized from other clients.

Installation coverage is broader than runtime coverage. See the
[test matrix](docs/TEST_MATRIX.md), [verification report](docs/VERIFICATION.md),
and [compatibility guide](docs/COMPATIBILITY.md) for exact runtime-tested,
OAuth-tested, read-only, and not-proven boundaries.

## Safety

- Review a plugin's tools and permissions before enabling it.
- Start with read-only tasks, especially after OAuth.
- Never place tokens in `plugin.json`, `mcp.json`, or committed headers.
- A valid plugin can still expose destructive tools.

See [SECURITY.md](SECURITY.md) for reporting and security boundaries.

## Contributing

You can improve the CLI, submit an Agent Plugins 1.0 package, or propose an
external package by pull request. Start with [CONTRIBUTING.md](CONTRIBUTING.md).

Universal Agent Plugins is an independent community project maintained by
777genius. It is not affiliated with or endorsed by OpenAI or the vendors shown
above. Original project material is licensed under [Apache 2.0](LICENSE).
