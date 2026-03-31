# cursor-basic

Reference Cursor workspace-config example for `plugin-kit-ai`.

This example demonstrates the current documented Cursor subset:

- portable `mcp/servers.yaml` rendered into `.cursor/mcp.json`
- target-authored `targets/cursor/rules/project.mdc` mirrored into `.cursor/rules/project.mdc`
- optional `targets/cursor/AGENTS.md` mirrored into shared root `AGENTS.md`
- strict documented-subset positioning: no root `CLAUDE.md`, no global `~/.cursor/mcp.json`, no nested non-root `.cursor/rules/**`, and no JSONC promise for `.cursor/mcp.json`

Validate it with:

```bash
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform cursor --strict
```
