# opencode-basic

Reference OpenCode workspace-config example for `plugin-kit-ai`.

This example demonstrates the current finished OpenCode workspace/config lane:

- `targets/opencode/package.yaml` for `opencode.json.plugin`
- `targets/opencode/commands/` for `.opencode/commands/`
- `targets/opencode/agents/` for `.opencode/agents/`
- `targets/opencode/themes/` for `.opencode/themes/`
- `targets/opencode/tools/` for first-class beta `.opencode/tools/`
- `targets/opencode/plugins/` for `.opencode/plugins/`
- `targets/opencode/package.json` for shared `.opencode/package.json` dependency metadata used by both standalone tools and plugin code
- portable `mcp/servers.yaml` for `opencode.json.mcp`
- portable `skills/` validated against the shared `SKILL.md` contract and mirrored into `.opencode/skills/`
- native import compatibility for `opencode.json`, `opencode.jsonc`, project workspace directories, local plugin code/package metadata, and explicit `--include-user-scope`

Plugin specifics in this example:

- `targets/opencode/plugins/example.js` uses the canonical official-style named async plugin export and doubles as the loader smoke fixture
- `targets/opencode/tools/echo.ts` shows first-class beta standalone tool authoring using `@opencode-ai/plugin`
- `targets/opencode/plugins/custom-tool.js` shows beta custom-tool support through plugin code using the same shared helper dependency
- `targets/opencode/package.json` is the canonical authored source for shared tool/plugin dependencies
- `make test-opencode-tools-live` is the dedicated opt-in live evidence path for standalone tools; `make test-opencode-live` remains the stable local-plugin-loading evidence path

Validate it with:

```bash
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform opencode --strict
```
