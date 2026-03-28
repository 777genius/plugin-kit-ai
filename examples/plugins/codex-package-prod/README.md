# codex-package-prod

Reference Codex package repo for the official `codex-package` lane.

This example covers the official Codex plugin bundle only. It does not include `launcher.yaml` or repo-local notify integration.

## Workflow

```bash
plugin-kit-ai normalize .
plugin-kit-ai render .
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform codex-package --strict
```
