# opencode-basic

Reference OpenCode workspace-config example for `plugin-kit-ai`.

This example demonstrates the current finished OpenCode workspace/config lane:

- `targets/opencode/package.yaml` for `opencode.json.plugin`
- `targets/opencode/commands/` for `.opencode/commands/`
- `targets/opencode/agents/` for `.opencode/agents/`
- `targets/opencode/themes/` for `.opencode/themes/`
- portable `mcp/servers.json` for `opencode.json.mcp`
- portable `skills/` validated against the shared `SKILL.md` contract and mirrored into `.opencode/skills/`
- `targets/opencode/config.extra.json` for non-managed config passthrough
- native import compatibility for `opencode.json`, `opencode.jsonc`, project workspace directories, and explicit `--include-user-scope`

Validate it with:

```bash
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform opencode --strict
```
