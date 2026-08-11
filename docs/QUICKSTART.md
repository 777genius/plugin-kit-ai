# Client quick start

Install one Agent Plugins 1.0 package, not the whole catalog. You need Node.js
22 or newer:

```bash
npx universal-agent-plugins add context7
```

The CLI shows the exact package plan and asks before changing anything. If it
finds several clients, select one. You can also name the target directly:

```bash
npx universal-agent-plugins@0.1.7 add context7 --target cursor
npx universal-agent-plugins@0.1.7 add context7 --target codex,cursor
```

Comma-separated targets install in the order shown. Each target keeps its own
result; if one fails, successful targets are not silently rolled back and the
command exits non-zero with a per-target summary.

Supported targets:

| Target | What the CLI does | Remaining user step |
| --- | --- | --- |
| `codex` | Generates a personal OpenAI marketplace package | Runs no hidden UI actions; prints exact Codex activation steps |
| `chatgpt` (`agentplugins >=0.1.6`) | Prepares a projected package only when catalog v2 has a verified app binding | Install or select the registered personal app manually in ChatGPT Plugins, then start a new chat |
| `cursor` | Places the native package in Cursor's local plugin directory | Reload Cursor, then verify the plugin appears |
| `copilot` | Registers a managed marketplace, installs, and verifies through Copilot CLI | Nothing when successful |
| `vscode` | Installs automatically through Copilot CLI when available | Otherwise prints the exact `chat.pluginLocations` setting |
| `kiro` | Prepares the native package folder | Prints the exact **Powers -> Add Custom Power -> Import** steps and folder |

Lifecycle commands use the same explicit target or comma-separated targets:

```bash
npx universal-agent-plugins info context7
npx universal-agent-plugins doctor context7
npx universal-agent-plugins update context7 --target cursor
npx universal-agent-plugins remove context7 --target cursor
npx universal-agent-plugins update context7 --target codex,cursor
```

`prepared`, `auth_pending`, and `manual_activation_required` are not reported as
installed. OAuth stays inside the client; the CLI never stores tokens or accepts
trust prompts automatically.

## Install a package outside the catalog

Any valid Agent Plugins 1.0 package can be installed directly. Use a local
folder while developing it, or pin a GitHub source to a full commit SHA:

```bash
npx universal-agent-plugins add ./my-plugin --target cursor
npx universal-agent-plugins add \
  owner/repo@FULL_COMMIT_SHA//path/to/plugin \
  --target cursor
```

Catalog membership is needed only for a reviewed short name such as `context7`;
it is not required for installation. Review an external package's skills, MCP
servers, hooks, permissions, and source before enabling it.

ChatGPT target support starts with `agentplugins 0.1.6`. Cloudflare Docs is the
only catalog-v2-verified target:

```bash
npx universal-agent-plugins add cloudflare-docs --target chatgpt
```

This prepares and validates the package; it does not silently install or attest
the ChatGPT UI step. The registered development app passed personal-app
discovery, chat activation, and read-only runtime, but its availability remains
account/workspace-specific. Follow the printed Plugins UI step and verify it in
a new chat. The five stdio MCP packages stay Codex-only.

The portable package can also be installed through a client's native Agent
Plugins flow. Exact client/runtime/OAuth evidence is kept separately in the
[test matrix](TEST_MATRIX.md).
