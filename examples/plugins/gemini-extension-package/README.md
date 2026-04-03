# gemini-extension-package

Reference Gemini CLI extension repo for the current `plugin-kit-ai` packaging workflow.

Packaging contract:

- official-style `gemini-extension.json`
- inline `mcpServers` rendered from `mcp/servers.yaml`
- one primary target-native context source plus extra extension contexts rendered to the Gemini root layout
- native Gemini commands, hooks, and policies
- manifest-driven settings, themes, and `plan.directory`
- `targets/gemini/manifest.extra.json` as the forward-compatible escape hatch

This example is intentionally `packaging-only`, but it is the canonical full Gemini extension packaging lane in this repo. It does not claim Gemini runtime parity with Claude or Codex.

## Workflow

```bash
plugin-kit-ai normalize .
plugin-kit-ai render .
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform gemini --strict
plugin-kit-ai inspect . --target gemini
plugin-kit-ai capabilities --platform gemini
gemini extensions link .
gemini extensions config gemini-extension-package release-profile --scope user
gemini extensions disable gemini-extension-package --scope user
gemini extensions enable gemini-extension-package --scope user
```

If you intentionally want the Gemini Go hook lane instead of this packaging-only example, start from `plugin-kit-ai init --platform gemini --runtime go`, inspect the supported runtime surface with `plugin-kit-ai capabilities --mode runtime --platform gemini`, use `make test-gemini-runtime` for the deterministic runtime gate, and use `make test-gemini-runtime-live` for the dedicated opt-in real CLI runtime smoke.
