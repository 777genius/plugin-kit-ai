# codex-package-prod

Reference Codex package repo for the official `codex-package` lane.

This example covers the official Codex plugin bundle only. It does not include `src/launcher.yaml` or repo-local notify integration.
It demonstrates first-class package metadata in `src/targets/codex-package/package.yaml`, structured prompt UX in `src/targets/codex-package/interface.json`, an optional app manifest in `src/targets/codex-package/app.json`, and shared MCP wiring from `src/mcp/servers.yaml`.

## Workflow

```bash
plugin-kit-ai normalize .
plugin-kit-ai generate .
plugin-kit-ai generate --check .
plugin-kit-ai validate . --platform codex-package --strict
```

Use [../../../docs/CODEX_TARGET_BOUNDARY.md](../../../docs/CODEX_TARGET_BOUNDARY.md) if you need to decide between this package lane and the repo-local `codex-runtime` lane.
