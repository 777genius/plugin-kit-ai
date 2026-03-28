# Production Plugin Examples

These examples are reference implementations for the current production plugin workflow.

- [claude-basic-prod](./claude-basic-prod): Claude plugin repo with `plugin.yaml`, generated native artifacts, and deterministic local smoke path
- [codex-basic-prod](./codex-basic-prod): Codex plugin repo with `plugin.yaml`, generated native artifacts, and deterministic local smoke path
- [gemini-extension-package](./gemini-extension-package): Gemini CLI extension repo with `plugin.yaml`, rendered `gemini-extension.json`, shared MCP, and packaging-only validation coverage

Use them together with [../../docs/PRODUCTION.md](../../docs/PRODUCTION.md).
For repo-local Python/Node entrance examples, see [../local/README.md](../local/README.md).

These reference repos document the current stable Go-first production path.
Their authored source of truth is `plugin.yaml` plus `targets/<platform>/...`; committed native Claude/Codex files are rendered managed artifacts.
Gemini remains packaging-only in this reference set. Executable `python`, `node`, and `shell` plugins remain `public-beta`, repo-local only, and are covered through scaffold/runtime docs plus polyglot smoke tests rather than checked-in production example repos.
