# plugin-kit-ai CLI

Module: `github.com/plugin-kit-ai/plugin-kit-ai/cli`. Builds the **`plugin-kit-ai`** binary: `init`, `bootstrap`, `render`, `import`, `inspect`, `normalize`, `validate`, `capabilities`, `install`, `version`, plus experimental `skills` authoring commands.

Current CLI contract status in this source tree: `public-stable` shipped in `v1.0.0`, with additional post-`v1.0.x` hardening on `main`. Repository-wide compatibility and release policy live in [../../docs/SUPPORT.md](../../docs/SUPPORT.md) and [../../docs/RELEASE.md](../../docs/RELEASE.md).

`plugin-kit-ai init` scaffolds a package-standard project for **Codex runtime** (`--platform codex-runtime`, default), **Codex package** (`--platform codex-package`), **Claude** (`--platform claude`), or **Gemini** (`--platform gemini`). Runtime selection `--runtime go|python|node|shell` applies to launcher-based targets only; `--typescript` is available only with `--runtime node` on launcher-based lanes. Gemini and Codex package authoring do not use `launcher.yaml` or executable runtime scaffolding. Claude defaults to the stable `Stop`/`PreToolUse`/`UserPromptSubmit` subset; use `--claude-extended-hooks` only when you intentionally want the full runtime-supported hook scaffold.
`plugin-kit-ai bootstrap` is the bounded repo-local first-run helper for interpreted launcher-based projects. It creates `.venv` and installs `requirements.txt` for Python when present, runs `npm install` for Node, and runs `npm run build` for TypeScript-shaped Node projects.
`plugin-kit-ai validate` checks package-standard projects, including generated-artifact drift, manifest warnings for unknown `plugin.yaml` keys, and Claude authored-hook routing consistency against `launcher.yaml.entrypoint`.
`plugin-kit-ai render` renders native target artifacts from the authored package-standard layout, `plugin-kit-ai import` backfills that layout from current native Claude/Codex/Gemini artifacts, and `plugin-kit-ai normalize` rewrites `plugin.yaml` into the package-standard shape.
`plugin-kit-ai capabilities` defaults to the target/package view and supports `--mode runtime` for runtime-event metadata.

```bash
# from repository root
go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai
```

Choose the path that matches your goal:

| Goal | Recommended lane |
|------|------------------|
| local notify/runtime plugin in your repo | `codex-runtime` |
| official Codex bundle/package output | `codex-package` |
| Claude hook runtime plugin | `claude` |
| Gemini CLI extension package | `gemini` |

## Fast Local Plugin

For repo-local plugins where fast iteration matters more than packaged distribution:

- Good fit: Python or Node teams wiring a local Claude/Codex plugin into an existing repo
- Guarantee level: supported repo-local executable path with `validate --strict` as the readiness gate
- Main non-goals: managed dependency installation, packaged distribution, and runtime parity with the Go SDK

```bash
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime python
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
./bin/plugin-kit-ai bootstrap ./my-plugin
```

Reference repos: [../../examples/local/README.md](../../examples/local/README.md)

## Production-Ready Plugin Repo

For the strongest supported path in the current CLI contract:

- Good fit: new plugin repos that want typed handlers and the clearest release story
- Guarantee level: strongest production-default path in the shipped contract
- Main non-goals: interpreted-runtime packaging and dependency management

```bash
./bin/plugin-kit-ai init my-plugin
./bin/plugin-kit-ai init my-plugin --platform claude
./bin/plugin-kit-ai init my-plugin --platform codex-package
./bin/plugin-kit-ai init my-plugin --platform gemini
```

Default `init my-plugin` expands to `--platform codex-runtime --runtime go`.

## Already Have Native Config

For migrating current Claude/Codex/Gemini native files into the package-standard authored layout:

- Good fit: teams adopting managed source-of-truth workflows without hand-editing vendor files
- Guarantee level: supported import bridge into the authored package model
- Main non-goals: keeping native target files as the long-term authored source of truth

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
```

Legacy migration bridge for older native Codex layouts:

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex
```

Current behavior and contract details:

- `init`: package-standard scaffold for `codex-runtime`, `codex-package`, `claude`, or `gemini`; launcher-based targets support Go-first or executable runtimes, while Gemini and Codex package stay packaging-only
- `bootstrap`: repo-local first-run helper for launcher-based interpreted runtimes; no-op for `go`, `codex-package`, and `gemini`
- `init --platform claude`: stable-default Claude scaffold; `--claude-extended-hooks` opts into the full runtime-supported hook set
- `init --platform gemini`: richer packaging starter with `targets/gemini/package.yaml`, `contexts/GEMINI.md`, and no launcher/runtime scaffold
- `render`: render native Claude artifacts, Codex package/runtime lane artifacts, and Gemini CLI extension packaging artifacts from `plugin.yaml` plus `targets/<platform>/...`
- `import`: create the package-standard authored layout from current native Claude/Codex/Gemini artifacts; Gemini import remains extension-packaging-only
- `inspect`: explain the discovered package graph, target class, and managed artifacts
- `normalize`: rewrite `plugin.yaml` into the package-standard shape and drop unknown fields
- `validate`: package-standard project validation, generated-artifact drift checks, and non-failing manifest warnings; `--strict` promotes warnings to errors for CI
- `capabilities`: generated target/package support by default, or runtime support with `--mode runtime`
- `install`: plugin binary from GitHub Releases with checksum verification
- `version`: build/version info
- `skills init|validate|render`: experimental SKILL.md authoring and agent render tooling

For the experimental skills subsystem, handwritten `skills/<name>/SKILL.md` is supported directly. `skills init` is convenience scaffold, not a required entrypoint.
For `install`, the stable CLI promise is limited to verified installation of third-party plugin binaries from GitHub Releases. It does not include self-update for the `plugin-kit-ai` CLI itself.
Executable runtime scaffolds for `python`, `node`, and `shell` are `public-beta`, repo-local, and do not add managed install/update handling for interpreted runtimes. `plugin.yaml` plus `targets/<platform>/...` is the only supported authored package standard; native Claude/Codex/Gemini config files are rendered managed artifacts, and `import` exists to recover authored state from those native layouts. Unknown manifest keys warn via `validate`. Gemini is a `packaging-only Gemini CLI extension target` in this CLI surface, not a production-ready runtime target; the supported Gemini contract is the full official extension packaging lane through `gemini-extension.json`, inline `mcpServers`, contexts, settings, themes, commands, hooks, policies, `manifest.extra.json`, and local `gemini extensions link|config|disable|enable` workflows. `plugin-kit-ai capabilities` defaults to the target/package view so package authors can see target class, production boundary, and managed artifacts first. For generated Python and Node projects, `plugin-kit-ai bootstrap <path>` is the supported first-run helper before `validate --strict`.
Generated Claude/Codex package-runtime config shapes are part of the repo-owned contract surface; `render --check` and the deterministic `polyglot-smoke` lane are the primary drift guards for that wiring. Claude authored hook routing consistency with `launcher.yaml.entrypoint` is enforced by `validate --strict`.

Executable runtime matrix:

| Runtime | Status | Scope | Bootstrap |
|---------|--------|-------|-----------|
| `go` | stable | default production path | Go `1.22+` |
| `python` | public-beta | repo-local only | prefer `.venv`, fallback to system Python `3.10+` |
| `node` | public-beta | repo-local only | system Node.js `20+`; JavaScript by default, TypeScript via `--runtime node --typescript` |
| `shell` | public-beta | repo-local only | POSIX shell on Unix, `bash` in `PATH` on Windows |

For interpreted runtimes, `validate --strict` is the canonical CI-grade readiness gate.
`plugin-kit-ai install` remains binary-only and does not bootstrap or distribute Python/Node/Shell plugin dependencies.

Production-ready target boundary in the current contract:

- Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` set
- Codex runtime: production-ready within the stable `Notify` path
- Codex package: production-ready official plugin package lane
- Gemini: full packaging-only Gemini CLI extension lane through `render|import|validate` and local `extensions link|config|disable|enable`

Canonical production plugin lane:

```bash
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai render ./my-plugin
./bin/plugin-kit-ai render ./my-plugin --check
./bin/plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

`plugin-kit-ai install` prints a deterministic success summary:

- installed file path
- release ref with source (`tag` or `latest`)
- selected asset name
- target GOOS/GOARCH
- overwrite notice only when an existing file was replaced

Supported and unsupported release layouts for `install` are documented in [../../docs/INSTALL_COMPATIBILITY.md](../../docs/INSTALL_COMPATIBILITY.md).
Production authoring guidance and reference examples live in [../../docs/PRODUCTION.md](../../docs/PRODUCTION.md) and [../../examples/plugins/README.md](../../examples/plugins/README.md).

See the root [README.md](../../README.md) for current CLI behavior, shipped scope, and canonical support links.
See [../../docs/EXECUTABLE_ABI.md](../../docs/EXECUTABLE_ABI.md) for the low-level executable plugin contract.
See [../../docs/SKILLS.md](../../docs/SKILLS.md) for the skills workflow, positioning, and examples.

`go.mod` uses:

- `replace github.com/plugin-kit-ai/plugin-kit-ai/sdk => ../../sdk/plugin-kit-ai`
- `replace github.com/plugin-kit-ai/plugin-kit-ai/plugininstall => ../../install/plugininstall`
