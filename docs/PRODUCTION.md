# Production Plugin Workflow

This document is the canonical production authoring path for plugin authors using `plugin-kit-ai`.

## Current Target Boundary

- Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` event set
- Codex runtime: production-ready within the stable `Notify` path
- Codex package: production-ready official plugin package lane
- Gemini: full Gemini CLI extension packaging lane through `render|import|validate` and local `extensions link|config|disable|enable`; not a production-ready runtime target
- Cursor: workspace-config-only target with repo-local `.cursor/mcp.json`, project-root `.cursor/rules/**`, optional shared root `AGENTS.md`, and compatibility import for legacy `.cursorrules`; not a production-ready runtime target
- OpenCode: workspace-config-only target with a stable repo-local local-plugin-loading subset for official-style plugin subtree ownership and shared package metadata, plus first-class beta standalone tools, explicit env-config import compatibility, and permission-first passthrough config semantics; `custom_tools` remain beta

Repo-local executable runtime boundary:

| Runtime | Current tier | Production guidance |
|---------|--------------|---------------------|
| `go` | stable | default production path |
| `python` | stable local-runtime subset | repo-local on `codex-runtime` and `claude`; lockfile-first manager detection with repo-local `.venv` readiness for `requirements`/`venv`/`uv`, manager-owned env readiness for `poetry`/`pipenv` |
| `node` | stable local-runtime subset | repo-local on `codex-runtime` and `claude`; system Node.js `20+`; lockfile-first manager detection plus TypeScript via `--runtime node --typescript` |
| `shell` | public-beta | repo-local only, POSIX shell on Unix, `bash` required on Windows |

Node/TypeScript and Python are the stable interpreted subset for scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, bounded portable export bundles, and local/remote bundle handoff on `codex-runtime` and `claude`.
Shell remains beta-hardening-only in this workflow.
This workflow does not imply a universal package-management contract or packaged distribution through `plugin-kit-ai install`.
Use Homebrew to install the `plugin-kit-ai` CLI locally when possible, use `npm i -g plugin-kit-ai` as the official `public-beta` JavaScript ecosystem path, use `pipx install plugin-kit-ai` as the `public-beta` Python ecosystem path only when that release was published to PyPI, use `scripts/install.sh` as the verified fallback, and use `777genius/plugin-kit-ai/setup-plugin-kit-ai@v1` to bootstrap the same verified CLI in CI.

Supported authored inputs are root `plugin.yaml` plus `targets/<platform>/...`.
Committed Claude/Codex/Gemini/Cursor/OpenCode native config files are rendered managed artifacts and should be treated as generated outputs.

## Canonical Production Lane

Run this exact sequence before shipping a plugin repo:

```bash
plugin-kit-ai normalize .
plugin-kit-ai render .
plugin-kit-ai render --check .
plugin-kit-ai validate . --platform <claude|codex-runtime|codex-package|cursor|opencode> --strict
```

Then run the fixture-driven smoke:

- Claude: run `plugin-kit-ai test . --platform claude --all`; generated launcher-based Claude projects already scaffold `fixtures/claude/{Stop,PreToolUse,UserPromptSubmit}.json` and matching `goldens/claude/*`, and `--update-golden` is only needed when you intentionally want to refresh that checked-in output contract
- Codex: run `plugin-kit-ai test . --platform codex-runtime --event Notify`; generated launcher-based Codex runtime projects already scaffold `fixtures/codex-runtime/Notify.json` and matching `goldens/codex-runtime/*`, and `--update-golden` is only needed when you intentionally want to refresh that checked-in output contract
- Cursor: treat `render --check` plus `validate --strict` as the repo-local readiness gate for the documented workspace-config subset
- OpenCode: run `make test-opencode-live` when recording stable local-plugin-loading evidence, and run `make test-opencode-tools-live` when recording standalone-tools beta evidence; both remain opt-in and require `opencode` in `PATH`

For interpreted runtimes, add the bootstrap step before `validate --strict`:

- `doctor`: run `plugin-kit-ai doctor .` first when you want a read-only readiness verdict; it now reports the runtime/build-tool binaries visible to the current shell so PATH problems are obvious before bootstrap
- `python`: run `plugin-kit-ai bootstrap .`; `venv`, `requirements.txt`, and `uv` end in repo-local `.venv`, while `poetry` and `pipenv` can end in manager-owned envs
- `node`: run `plugin-kit-ai bootstrap .`; it chooses the detected install manager, and TypeScript-shaped Node projects also run `build`
- `test`: run `plugin-kit-ai test . --platform <claude|codex-runtime> --event <event>` for a single stable event, or `--all` for the full stable event set on the selected platform; fixtures default to `fixtures/<platform>/<event>.json`, goldens default to `goldens/<platform>/<event>.*`, generated launcher-based Claude and Codex runtime projects now pre-seed that layout during `init`, and `--update-golden` rewrites the current stdout/stderr/exitcode contract
- `test` output: the command now emits a per-run summary plus short mismatch previews, and `--format json` carries the same counters for CI consumers
- `shared helper packages`: `plugin-kit-ai-runtime` on PyPI and npm mirrors the supported Python/Node helper API when teams want a shared dependency instead of per-repo helper files; the default scaffold remains self-contained for hermetic first run, and `plugin-kit-ai init ... --runtime-package` is the official opt-in path when you want new projects to start on the shared dependency mode; released CLIs auto-pin the helper version, while development builds should pass `--runtime-package-version`
- helper delivery tradeoff: see [CHOOSING_HELPER_DELIVERY_MODE.md](./CHOOSING_HELPER_DELIVERY_MODE.md)
- `shell`: ensure the launcher target remains executable on Unix and `bash` is available on Windows; this path remains `public-beta`
- `export`: run `plugin-kit-ai export . --platform <codex-runtime|claude>` when you need a portable handoff bundle after readiness is already green; this is stable for `python` and `node`, beta for `shell`
- `bundle publish`: run `plugin-kit-ai bundle publish . --platform <codex-runtime|claude> --repo <owner/repo> --tag <tag>` when you want a producer-side GitHub Releases handoff for exported Python/Node bundles; it creates a published release by default, supports `--draft` when you want to keep the release as draft, and uploads the bundle plus `<asset>.sha256`; this path is stable and stays separate from `plugin-kit-ai install`
- `bundle install`: run `plugin-kit-ai bundle install <bundle.tar.gz> --dest <path>` when you need a local unpack/install handoff for exported Python/Node bundles; this path is stable for the Python/Node local-bundle subset and stays separate from `plugin-kit-ai install`
- `bundle fetch`: run `plugin-kit-ai bundle fetch --url <https://...tar.gz> --dest <path>` or `plugin-kit-ai bundle fetch <owner/repo> --tag <tag> --dest <path>` when you need a remote handoff path for exported Python/Node bundles; URL mode verifies `--sha256` or `<url>.sha256`, GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`, and this path is stable and separate from `plugin-kit-ai install`
- `init --extras`: for the stable interpreted `python`/`node` subset on `codex-runtime` and `claude`, this now emits `.github/workflows/bundle-release.yml`, which uses `setup-plugin-kit-ai@v1` and runs `doctor -> bootstrap -> validate --strict -> bundle publish`

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
- Keep `plugin.yaml`, optional `mcp/servers.yaml`, plus `targets/codex-package/...` as the authored source of truth
- Commit generated `.codex-plugin/plugin.json` plus optional `.mcp.json` and `.app.json`
- Treat this lane as the official Codex plugin bundle, separate from local notify/runtime wiring

Reference implementation:

- [examples/plugins/codex-package-prod](../examples/plugins/codex-package-prod)

## OpenCode Release-Ready Path

- Start from `plugin-kit-ai init --platform opencode` or `plugin-kit-ai import --from opencode`
- Keep `plugin.yaml`, optional `mcp/servers.yaml`, plus `targets/opencode/...` as the authored source of truth
- Commit generated `opencode.json`, `.opencode/tools/**`, `.opencode/plugins/**`, and `.opencode/package.json`
- Treat the stable promise as applying to repo-local authored/render/import/validate for local plugin subtree ownership and shared dependency metadata in `.opencode/package.json`
- Treat standalone `.opencode/tools/**` authoring as first-class `public-beta`
- Treat `custom_tools` as `public-beta` whether they ship through standalone tool files or plugin code
- Record `make test-opencode-live` evidence whenever you are refreshing or asserting the OpenCode stable boundary
- Record `make test-opencode-tools-live` evidence whenever you are refreshing or asserting the standalone-tools beta boundary

Reference implementation:

- [examples/plugins/opencode-basic](../examples/plugins/opencode-basic)

## Cursor Workspace Path

- Start from `plugin-kit-ai init --platform cursor` or `plugin-kit-ai import --from cursor`
- Keep `plugin.yaml`, `mcp/servers.yaml`, and `targets/cursor/...` as the authored source of truth
- Commit generated `.cursor/mcp.json`, `.cursor/rules/**`, and optional shared root `AGENTS.md`
- Treat this lane as the documented Cursor workspace-config subset only; do not assume support for root `CLAUDE.md`, global `~/.cursor/mcp.json`, nested non-root `.cursor/rules/**`, JSONC, or VS Code extension packaging through this target

Reference implementation:

- [examples/plugins/cursor-basic](../examples/plugins/cursor-basic)

## What This Workflow Guarantees

- normalized `plugin.yaml` with no unknown fields
- generated native artifacts are in sync
- strict validation passes with no manifest drift and no Claude authored-hook entrypoint drift
- the committed example-shaped repo can build and execute a deterministic local smoke path
- OpenCode stable local plugin loading is evidenced through the deterministic marker-based `test-opencode-live` smoke path
- OpenCode standalone tools beta evidence is recorded separately through the deterministic marker-based `test-opencode-tools-live` smoke path
- Cursor workspace-config readiness is bounded to deterministic render/import/validate behavior for the documented repo-local subset

## Gemini Packaging Boundary

- Start from `plugin-kit-ai init --platform gemini` or `plugin-kit-ai import --from gemini`
- Keep `plugin.yaml`, optional `mcp/servers.yaml`, plus `targets/gemini/...` as the authored source of truth
- Commit generated `gemini-extension.json` plus rendered `hooks/`, `commands/`, `policies/`, and selected context artifacts
- Treat Gemini as official extension packaging only: inline `mcpServers`, `contextFileName`, `settings`, `themes`, `excludeTools`, `plan.directory`, and `manifest.extra.json`
- Use `gemini extensions link` for local development, `gemini extensions config` for install-time settings, and `gemini extensions disable|enable` to exercise scope changes; restart Gemini CLI after changes

## What It Does Not Guarantee

- external Claude CLI health before hook execution
- external Codex CLI health before `notify` execution
- interactive runtime parity for Gemini sessions
- arbitrary OpenCode custom tool semantics beyond the documented tool/plugin/package-metadata contract
- promotion of runtime-supported beta hooks into the stable promise
- dependency bootstrap beyond the bounded helpers, or packaged distribution through `plugin-kit-ai install`
