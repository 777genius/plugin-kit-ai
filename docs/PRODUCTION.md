# Production Plugin Workflow

This document is the canonical production authoring path for plugin authors using `plugin-kit-ai`.

## Current Target Boundary

- Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` event set
- Codex: production-ready within the stable `Notify` path
- Gemini: packaging-only Gemini CLI extension target through `render|import|validate`; not a production-ready runtime target

Repo-local executable runtime boundary:

| Runtime | Current tier | Production guidance |
|---------|--------------|---------------------|
| `go` | stable | default production path |
| `python` | public-beta | repo-local only, prefer `.venv`, fallback to system Python `3.10+` |
| `node` | public-beta | repo-local only, system Node.js `20+`, TypeScript only after build-to-JS |
| `shell` | public-beta | repo-local only, POSIX shell on Unix, `bash` required on Windows |

Interpreted runtimes are production-hardened for scaffold, validate, launcher execution, and repo-local bootstrap only.
This workflow does not imply support for dependency installation, package management, or packaged distribution through `plugin-kit-ai install`.

Supported authored inputs are root `plugin.yaml` plus `targets/<platform>/...`.
Committed Claude/Codex/Gemini native config files are rendered managed artifacts and should be treated as generated outputs.

## Canonical Production Lane

Run this exact sequence before shipping a plugin repo:

```bash
plugin-kit-ai normalize .
plugin-kit-ai render .
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform <claude|codex> --strict
```

Then run the target-specific smoke:

- Claude: execute the built binary with documented stable hook payloads for `Stop`, `PreToolUse`, and `UserPromptSubmit`
- Codex: execute the built binary with a documented `notify` payload

For interpreted runtimes, add the bootstrap step before `validate --strict`:

- `python`: create `.venv` with `python3 -m venv .venv` when using a project-local runtime
- `node`: run `npm install`; commit `package-lock.json` or an equivalent deterministic lockfile for production repos
- `shell`: ensure the launcher target remains executable on Unix and `bash` is available on Windows

After bootstrap, treat `validate --strict` as the CI-grade readiness gate for interpreted runtimes.

## Claude Release-Ready Path

- Start from `plugin-kit-ai init --platform claude` or `plugin-kit-ai import --from claude`
- Keep `plugin.yaml` plus `targets/claude/...` as the authored source of truth
- Commit generated `.claude-plugin/plugin.json` and `hooks/hooks.json`
- `validate --strict` enforces that authored `targets/claude/hooks/hooks.json` command entries still match `launcher.yaml.entrypoint`
- Treat the stable promise as applying only to `Stop`, `PreToolUse`, and `UserPromptSubmit`
- The default Claude scaffold already matches that stable subset; use `--claude-extended-hooks` only as an explicit expansion step
- Treat additional runtime-supported Claude hooks as `public-beta` unless separately promoted

Reference implementation:

- [examples/plugins/claude-basic-prod](../examples/plugins/claude-basic-prod)

## Codex Release-Ready Path

- Start from `plugin-kit-ai init --platform codex` or `plugin-kit-ai import --from codex`
- Keep `plugin.yaml` plus `targets/codex/...` as the authored source of truth
- Commit generated `.codex-plugin/plugin.json` and `.codex/config.toml`
- Keep `AGENTS.md` repo-local and review it as part of release
- Treat the stable promise as applying only to the `Notify` path

Reference implementation:

- [examples/plugins/codex-basic-prod](../examples/plugins/codex-basic-prod)

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
- Use `gemini extensions link` for local development and restart Gemini CLI after changes

## What It Does Not Guarantee

- external Claude CLI health before hook execution
- external Codex CLI health before `notify` execution
- interactive runtime parity for Gemini sessions
- promotion of runtime-supported beta hooks into the stable promise
- dependency bootstrap or packaged distribution for interpreted runtimes
