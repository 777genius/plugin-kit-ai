# Client quick start

The portable packages live under `plugins/`. Installation is client-owned in
Agent Plugins 1.0, so use the flow for your client.

## ChatGPT and Codex

```bash
codex plugin marketplace add 777genius/universal-agent-plugins
codex plugin add context7@universal-agent-plugins
```

Then start a new Codex session. In the ChatGPT desktop app, restart the app,
open **Plugins**, select **Universal Agent Plugins**, and install from the same
marketplace. ChatGPT web can use public-directory plugins and developer-mode
MCP connections, but local repo marketplaces are a desktop authoring surface.

Remove the test installation with:

```bash
codex plugin remove context7@universal-agent-plugins
codex plugin marketplace remove universal-agent-plugins
```

## Cursor

Cursor loads conformant Agent Plugins directly from
`~/.cursor/plugins/local` while developing:

```bash
mkdir -p ~/.cursor/plugins/local
cp -R plugins/context7 ~/.cursor/plugins/local/context7
```

Open **Customize**, find the plugin, select **Install**, and choose user or
project scope. Team and Enterprise users can also import the GitHub repository
as a team marketplace from **Dashboard -> Plugins**.

## VS Code

Add the public repository to the `chat.plugins.marketplaces` setting:

```json
{
  "chat.plugins.marketplaces": [
    "777genius/universal-agent-plugins"
  ]
}
```

Open Extensions, search for `@agentPlugins`, select a plugin, and install it.
VS Code can also register local plugin paths through its Agent Plugins settings.

## GitHub Copilot CLI

```bash
copilot plugin marketplace add 777genius/universal-agent-plugins
copilot plugin install context7@universal-agent-plugins
```

Use `copilot plugin list` to verify the installation.

## Kiro

Open the Powers panel, select **Add Custom Power**, and import either the public
GitHub URL or a local package directory such as `plugins/context7`. Kiro calls
Agent Plugins "powers" and manages their MCP servers internally.

## Claude Code, Gemini CLI, and OpenCode

These installed tools can consume Agent Skills or MCP configurations through
their own extension systems, but they are not listed as Agent Plugins 1.0
clients. Do not point their plugin installers at this repository and assume
1.0 conformance. Reuse an individual MCP endpoint or skill only through that
client's documented import flow.

## Authentication

- Start with a no-auth hero plugin before testing OAuth.
- OAuth discovery and account selection are client-owned.
- Never paste tokens into `plugin.json`, `mcp.json`, or committed headers.
- GitHub's OpenAI adapter reads `GITHUB_PAT_TOKEN` from the environment.
- Figma, Linear, and Notion preserve the published OpenAI `oauth_resource`
  metadata in the generated compatibility packages.

Sources: [Agent Plugins clients](https://agent-plugins.org/compatible-clients),
[OpenAI packaging](https://developers.openai.com/plugins/build/plugins),
[Cursor plugins](https://cursor.com/docs/plugins),
[VS Code Agent Plugins](https://code.visualstudio.com/docs/agent-customization/agent-plugins),
[GitHub Copilot plugins](https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/plugins-finding-installing),
and [Kiro powers](https://kiro.dev/docs/powers/installation/).
