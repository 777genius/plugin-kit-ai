![Universal Agent Plugins](assets/hero.png)

# Universal Agent Plugins

[![npm](https://img.shields.io/npm/v/universal-agent-plugins?color=7257FF)](https://www.npmjs.com/package/universal-agent-plugins)
[![Validate](https://github.com/777genius/universal-agent-plugins/actions/workflows/validate.yml/badge.svg)](https://github.com/777genius/universal-agent-plugins/actions/workflows/validate.yml)
[![Live E2E](https://github.com/777genius/universal-agent-plugins/actions/workflows/live-e2e.yml/badge.svg)](https://github.com/777genius/universal-agent-plugins/actions/workflows/live-e2e.yml)
[![Agent Plugins 1.0](https://img.shields.io/badge/Agent%20Plugins-1.0.0-7257FF)](https://agent-plugins.org/specification)
[![License](https://img.shields.io/badge/license-Apache--2.0-20A4C8)](LICENSE)

**Install one [Agent Plugins 1.0](https://agent-plugins.org/specification)
package in the AI agents you choose. One command, no manual JSON editing.**

```bash
npx universal-agent-plugins add context7
```

## Start in 30 seconds

1. Run the command above. You need [Node.js 22+](https://nodejs.org/).
2. Choose one or several agents detected on your computer.
3. Open a new chat and ask the agent to use Context7 for current documentation.

Context7 needs no account. If a plugin or agent needs a restart, import, sign-in,
or OAuth confirmation, the CLI shows that as a separate step.

For scripts and repeatable setup, name the agents explicitly:

```bash
npx universal-agent-plugins add context7 --target codex,cursor,kiro
```

The plugin is downloaded and verified once, then prepared for every selected
agent. You do not need to follow a different installation guide for each one.

## Why use it?

- **One command across agents.** Pick one or several clients in the same run.
- **One lifecycle.** Add, update, repair, switch, and remove with the same CLI.
- **Any standard package.** Use a short name, local folder, or pinned GitHub source.
- **No hidden success.** Activation and OAuth are reported separately from setup.
- **2500+ searchable plugins.** Start with reviewed packages or explore public ones.

## Supported agents

|  |  |  |
| --- | --- | --- |
| <img src="assets/client-icons/openai.svg" width="20" height="20" alt=""> Codex | <img src="assets/client-icons/openai.svg" width="20" height="20" alt=""> ChatGPT | <img src="assets/client-icons/cursor.svg" width="20" height="20" alt=""> Cursor |
| <img src="assets/client-icons/github-copilot.svg" width="20" height="20" alt=""> GitHub Copilot CLI | <img src="assets/client-icons/vscode.svg" width="20" height="20" alt=""> VS Code | <img src="assets/client-icons/kiro.svg" width="20" height="20" alt=""> Kiro |
| <img src="site/public/client-icons/claude.svg" width="20" height="20" alt=""> Claude Code | <img src="site/public/client-icons/gemini.svg" width="20" height="20" alt=""> Gemini CLI | <img src="site/public/client-icons/opencode.svg" width="20" height="20" alt=""> OpenCode |
| <img src="site/public/client-icons/cline.svg" width="20" height="20" alt=""> Cline | <img src="site/public/client-icons/windsurf.svg" width="20" height="20" alt=""> Windsurf |  |

The CLI handles each client's supported format and prints any remaining
activation step. Service accounts and permissions always stay under your control.

## Everyday commands

```bash
npx universal-agent-plugins search docs
npx universal-agent-plugins info context7
npx universal-agent-plugins add context7 --target cursor
npx universal-agent-plugins add context7 --target codex,cursor
npx universal-agent-plugins add cloudflare-docs --target codex,cursor,kiro
npx universal-agent-plugins update context7 --target codex,cursor,kiro
npx universal-agent-plugins repair context7 --target codex,cursor,kiro
npx universal-agent-plugins remove context7 --target codex,cursor,kiro
npx universal-agent-plugins switch cloudflare-docs --to 777genius/cloudflare-docs
npx universal-agent-plugins doctor
```

`update` keeps the recorded source. `remove` deletes only files managed by the
CLI. `switch` is the explicit way to move to another publisher or distribution.

## Find a plugin

The CLI searches more than **2500 Agent Plugins** discovered in public GitHub
repositories, plus 26 reviewed starter packages maintained here.

- [Browse plugins](https://777genius.github.io/universal-agent-plugins/)
- [Try the reviewed starters](docs/HERO_PLUGINS.md)
- [Submit a plugin](CONTRIBUTING.md)

The web Directory is optional. It makes discovery and short names convenient;
the installer remains the main product.

<details>
<summary><strong>Install a local or GitHub package</strong></summary>

The CLI is not limited to this Directory. External packages do not need to be copied into it.
Use a local Agent Plugins 1.0 package or an immutable GitHub source:

```bash
npx universal-agent-plugins validate ./my-plugin
npx universal-agent-plugins add ./my-plugin --target cursor
npx universal-agent-plugins add \
  777genius/universal-agent-plugins@2ddbb99dd190c1792b79904f9875e6322bccd243//plugins/cloudflare-docs \
  --target cursor
```

Pin GitHub sources to a full commit SHA so the installed bytes are reproducible.

Source labels are explicit: **upstream** lives in the owner's repository;
**community bridge** combines reviewed metadata with pinned upstream content;
**community** is community-authored; and **direct source** is a local path or
exact GitHub reference. Community packages are not official vendor packages.
Automatically discovered packages are marked **unreviewed** and validated again
before the CLI changes any client.

</details>

<details>
<summary><strong>How the CLI works</strong></summary>

```text
npx universal-agent-plugins
        ↓
verified Agent Plugins 1.0 package
        ↓
one plan for the selected agents
        ↓
client-specific setup + clear activation guidance
```

This repository is the product home and public source for the npm facade.
`universal-agent-plugins` is the npm package; `agentplugins` is the installed
command. The facade installs the `agentplugins` binary, verifies its SHA-256,
and caches the correct build for the current platform.

[`plugin-kit-ai`](https://github.com/777genius/plugin-kit-ai) is the shared Go
implementation engine. It contains the package loader, lifecycle, and client
adapters; that engine is not duplicated in this repository.

</details>

<details>
<summary><strong>Testing and current evidence</strong></summary>

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

</details>

## Safety

- Review a plugin's tools and permissions before enabling it.
- Start with read-only tasks, especially after OAuth.
- Never place tokens in `plugin.json`, `mcp.json`, or committed headers.
- A valid plugin can still expose destructive tools.

See [SECURITY.md](SECURITY.md) for reporting and security boundaries.

## Contributing

Contributions and Agent Plugins 1.0 package submissions are welcome. Start with
[CONTRIBUTING.md](CONTRIBUTING.md).

Universal Agent Plugins is an independent community project maintained by
777genius. It is not affiliated with or endorsed by OpenAI or the vendors shown
above. Original project material is licensed under [Apache 2.0](LICENSE).
