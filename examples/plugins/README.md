# Production Plugin Examples

These examples are reference implementations for the current production plugin workflow.

- [claude-basic-prod](./claude-basic-prod): Claude plugin repo with `plugin.yaml`, generated native artifacts, and deterministic local smoke path
- [codex-basic-prod](./codex-basic-prod): Codex runtime lane repo with `plugin.yaml`, generated `.codex/config.toml`, deterministic local notify smoke path, and repo-local MCP passthrough example
- [codex-package-prod](./codex-package-prod): official Codex package lane with `plugin.yaml`, rendered `.codex-plugin/plugin.json`, optional `.app.json`, shared `.mcp.json`, and skills-first bundle output
- [gemini-extension-package](./gemini-extension-package): Gemini CLI extension repo with `plugin.yaml`, rendered `gemini-extension.json`, shared MCP, and packaging-only validation coverage
- [cursor-basic](./cursor-basic): Cursor workspace-config repo with `plugin.yaml`, rendered `.cursor/mcp.json`, mirrored `.cursor/rules/**`, and optional shared root `AGENTS.md`
- [opencode-basic](./opencode-basic): OpenCode workspace-config repo with `plugin.yaml`, rendered `opencode.json`, shared MCP, and mirrored portable skills

Use them together with [../../docs/PRODUCTION.md](../../docs/PRODUCTION.md).
For copy-first Go/Python/Node starter repos, see [../starters/README.md](../starters/README.md).
For deeper repo-local Python/Node entrance references, including the checked-in helper-layer examples, see [../local/README.md](../local/README.md).

These reference repos document the current stable production path where Go is the recommended default because it yields the most self-contained plugin delivery story.
Their authored source of truth is `plugin.yaml` plus `targets/<platform>/...`; committed native Claude/Codex/Gemini/Cursor/OpenCode files are rendered managed artifacts.
Gemini, Cursor, and OpenCode remain packaging/workspace-config-only in this reference set. Gemini's Go hook lane is documented through the generated scaffold README, `plugin-kit-ai inspect`, `plugin-kit-ai capabilities --mode runtime --platform gemini`, the deterministic `make test-gemini-runtime` runtime gate, and the dedicated `make test-gemini-runtime-live` smoke path rather than a checked-in production example repo. Executable `python` and `node` plugins are stable supported repo-local local-runtime lanes and are covered through scaffold/runtime docs plus polyglot smoke tests rather than checked-in production example repos. Those interpreted lanes still require Python or Node to be installed on the machine running the plugin. Launcher-based `shell` authoring remains `public-beta`.
