# cursor-basic

Reference Cursor workspace-config example for `plugin-kit-ai`.

This example demonstrates the current documented Cursor subset:

- portable `src/mcp/servers.yaml` generated into `.cursor/mcp.json`
- target-authored `src/targets/cursor/rules/project.mdc` mirrored into `.cursor/rules/project.mdc`
- root `CLAUDE.md` and `AGENTS.md` are plugin boundary docs, not Cursor-native authored surfaces
- strict documented-subset positioning: no global `~/.cursor/mcp.json`, no nested non-root `.cursor/rules/**`, and no JSONC promise for `.cursor/mcp.json`

Validate it with:

```bash
plugin-kit-ai generate --check .
plugin-kit-ai validate . --platform cursor --strict
```
