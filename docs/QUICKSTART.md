# Client quick start

Install one Agent Plugins 1.0 package, not the whole catalog:

```bash
npx --yes universal-agent-plugins@0.1.1 add context7
```

The CLI shows the exact package plan and asks before changing anything. If it
finds several clients, select one. Automation must name one target explicitly:

```bash
npx --yes universal-agent-plugins@0.1.1 add context7 --target cursor --yes
```

Supported targets:

| Target | What the CLI does | Remaining user step |
| --- | --- | --- |
| `codex` | Generates the OpenAI/Codex compatibility package | Confirm installation in Codex or ChatGPT |
| `cursor` | Places the native package in Cursor's local plugin directory | Reload Cursor, then verify the plugin appears |
| `copilot` | Prepares the native package | Run the shown Copilot install command and review trust |
| `vscode` | Uses the Copilot bridge when available, otherwise prepares the package | Reload VS Code or confirm in Chat UI |
| `kiro` | Prepares the native package folder | Import it from **Powers -> Add Custom Power** |

Lifecycle commands use the same explicit target:

```bash
npx --yes universal-agent-plugins@0.1.1 info context7
npx --yes universal-agent-plugins@0.1.1 doctor context7
npx --yes universal-agent-plugins@0.1.1 update context7 --target cursor --yes
npx --yes universal-agent-plugins@0.1.1 remove context7 --target cursor --yes
```

`prepared`, `auth_pending`, and `manual_activation_required` are not reported as
installed. OAuth stays inside the client; the CLI never stores tokens or accepts
trust prompts automatically.

The portable package can also be installed through a client's native Agent
Plugins flow. Exact client/runtime/OAuth evidence is kept separately in the
[test matrix](TEST_MATRIX.md).
