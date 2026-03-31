# gemini-extension-package

Reference Gemini CLI extension repo for the current `plugin-kit-ai` packaging workflow.

Packaging contract:

- official-style `gemini-extension.json`
- inline `mcpServers` rendered from `mcp/servers.yaml`
- one primary target-native context source plus extra extension contexts rendered to the Gemini root layout
- native Gemini commands, hooks, and policies
- manifest-driven `migratedTo`, settings, themes, and `plan.directory`
- `targets/gemini/manifest.extra.json` as the forward-compatible escape hatch

This example is intentionally `packaging-only`, but it is the canonical full Gemini extension packaging lane in this repo. It does not claim Gemini runtime parity with Claude or Codex.

## Workflow

```bash
plugin-kit-ai normalize .
plugin-kit-ai render .
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform gemini --strict
gemini extensions link .
gemini extensions config gemini-extension-package release-profile --scope user
gemini extensions disable gemini-extension-package --scope user
gemini extensions enable gemini-extension-package --scope user
```
