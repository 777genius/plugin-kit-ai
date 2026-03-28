# Production Plugin Workflow

This document is the canonical production authoring path for plugin authors using `plugin-kit-ai`.

## Current Target Boundary

- Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` event set
- Codex runtime: production-ready within the stable `Notify` path
- Codex package: production-ready official plugin package lane
- Gemini: full Gemini CLI extension packaging lane through `render|import|validate` and local `extensions link|config|disable|enable`; not a production-ready runtime target

Repo-local executable runtime boundary:

| Runtime | Current tier | Production guidance |
|---------|--------------|---------------------|
| `go` | stable | default production path |
| `python` | stable local-runtime subset | repo-local on `codex-runtime` and `claude`; lockfile-first manager detection with repo-local `.venv` readiness for `requirements`/`venv`/`uv`, manager-owned env readiness for `poetry`/`pipenv` |
| `node` | stable local-runtime subset | repo-local on `codex-runtime` and `claude`; system Node.js `20+`; lockfile-first manager detection plus TypeScript via `--runtime node --typescript` |
| `shell` | public-beta | repo-local only, POSIX shell on Unix, `bash` required on Windows |

Node/TypeScript and Python are the stable repo-local interpreted subset for scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, and bounded portable export bundles on `codex-runtime` and `claude`.
Shell remains beta-hardening-only in this workflow.
This workflow does not imply a universal package-management contract or packaged distribution through `plugin-kit-ai install`.

Supported authored inputs are root `plugin.yaml` plus `targets/<platform>/...`.
Committed Claude/Codex/Gemini native config files are rendered managed artifacts and should be treated as generated outputs.

## Canonical Production Lane

Run this exact sequence before shipping a plugin repo:

```bash
plugin-kit-ai normalize .
plugin-kit-ai render .
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform <claude|codex-runtime|codex-package> --strict
```

Then run the target-specific smoke:

- Claude: execute the built binary with documented stable hook payloads for `Stop`, `PreToolUse`, and `UserPromptSubmit`
- Codex: execute the built binary with a documented `notify` payload

For interpreted runtimes, add the bootstrap step before `validate --strict`:

- `doctor`: run `plugin-kit-ai doctor .` first when you want a read-only readiness verdict
- `python`: run `plugin-kit-ai bootstrap .`; `venv`, `requirements.txt`, and `uv` end in repo-local `.venv`, while `poetry` and `pipenv` can end in manager-owned envs
- `node`: run `plugin-kit-ai bootstrap .`; it chooses the detected install manager, and TypeScript-shaped Node projects also run `build`
- `shell`: ensure the launcher target remains executable on Unix and `bash` is available on Windows; this path remains `public-beta`
- `export`: run `plugin-kit-ai export . --platform <codex-runtime|claude>` when you need a portable handoff bundle after readiness is already green; this is stable for `python` and `node`, beta for `shell`
- `bundle install`: run `plugin-kit-ai bundle install <bundle.tar.gz> --dest <path>` when you need a local unpack/install handoff for exported Python/Node bundles; this path is stable for the Python/Node local-bundle subset and stays separate from `plugin-kit-ai install`
- `bundle fetch`: run `plugin-kit-ai bundle fetch --url <https://...tar.gz> --dest <path>` or `plugin-kit-ai bundle fetch <owner/repo> --tag <tag> --dest <path>` when you need a remote handoff path for exported Python/Node bundles; this path is `public-beta` and stays separate from `plugin-kit-ai install`

After bootstrap, treat `validate --strict` as the CI-grade readiness gate for interpreted runtimes.

## Claude Release-Ready Path

- Start from `plugin-kit-ai init --platform claude` or `plugin-kit-ai import --from claude`
- Keep `plugin.yaml` plus `targets/claude/...` as the authored source of truth
- First-class Claude package docs include `targets/claude/settings.json`, `targets/claude/lsp.json`, `targets/claude/user-config.json`, and `targets/claude/manifest.extra.json`
- Commit generated `.claude-plugin/plugin.json` and `hooks/hooks.json`
- `validate --strict` enforces that authored `targets/claude/hooks/hooks.json` command entries still match `launcher.yaml.entrypoint`
- Treat the stable promise as applying only to `Stop`, `PreToolUse`, and `UserPromptSubmit`
- The default Claude scaffold already matches that stable subset; use `--claude-extended-hooks` only as an explicit expansion step
- Treat additional runtime-supported Claude hooks as `public-beta` unless separately promoted

Reference implementation:

- [examples/plugins/claude-basic-prod](../examples/plugins/claude-basic-prod)

## Codex Runtime Release-Ready Path

- Start from `plugin-kit-ai init --platform codex-runtime` or `plugin-kit-ai import --from codex-runtime`
- Keep `plugin.yaml` plus `targets/codex-runtime/...` as the authored source of truth
- Commit generated `.codex/config.toml`
- Treat the stable promise as applying only to the `Notify` path

Reference implementation:

- [examples/plugins/codex-basic-prod](../examples/plugins/codex-basic-prod)

## Codex Package Release-Ready Path

- Start from `plugin-kit-ai init --platform codex-package` or `plugin-kit-ai import --from codex-package`
- Keep `plugin.yaml` plus `targets/codex-package/...` as the authored source of truth
- Commit generated `.codex-plugin/plugin.json` plus optional `.mcp.json` and `.app.json`
- Treat this lane as the official Codex plugin bundle, separate from local notify/runtime wiring

Reference implementation:

- [examples/plugins/codex-package-prod](../examples/plugins/codex-package-prod)

## What This Workflow Guarantees

- normalized `plugin.yaml` with no unknown fields
- generated native artifacts are in sync
- strict validation passes with no manifest drift and no Claude authored-hook entrypoint drift
- the committed example-shaped repo can build and execute a deterministic local smoke path

## Gemini Packaging Boundary

- Start from `plugin-kit-ai init --platform gemini` or `plugin-kit-ai import --from gemini`
- Keep `plugin.yaml` plus `targets/gemini/...` as the authored source of truth
- Commit generated `gemini-extension.json` plus rendered `hooks/`, `commands/`, `policies/`, and selected context artifacts
- Treat Gemini as official extension packaging only: inline `mcpServers`, `contextFileName`, `settings`, `themes`, `excludeTools`, `plan.directory`, and `manifest.extra.json`
- Use `gemini extensions link` for local development, `gemini extensions config` for install-time settings, and `gemini extensions disable|enable` to exercise scope changes; restart Gemini CLI after changes

## What It Does Not Guarantee

- external Claude CLI health before hook execution
- external Codex CLI health before `notify` execution
- interactive runtime parity for Gemini sessions
- promotion of runtime-supported beta hooks into the stable promise
- dependency bootstrap beyond the bounded helpers, or packaged distribution through `plugin-kit-ai install`
