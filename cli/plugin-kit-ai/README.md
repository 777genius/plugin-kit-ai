# plugin-kit-ai CLI

Module: `github.com/plugin-kit-ai/plugin-kit-ai/cli`. Builds the **`plugin-kit-ai`** binary: `init`, `bootstrap`, `doctor`, `export`, `bundle install`, `render`, `import`, `inspect`, `normalize`, `validate`, `capabilities`, `install`, `version`, plus experimental `skills` authoring commands.

Current CLI contract status in this source tree: `public-stable` shipped in `v1.0.0`, with additional post-`v1.0.x` hardening on `main`. Repository-wide compatibility and release policy live in [../../docs/SUPPORT.md](../../docs/SUPPORT.md) and [../../docs/RELEASE.md](../../docs/RELEASE.md).

`plugin-kit-ai init` scaffolds a package-standard project for **Codex runtime** (`--platform codex-runtime`, default), **Codex package** (`--platform codex-package`), **Claude** (`--platform claude`), **Gemini** (`--platform gemini`), or **OpenCode** (`--platform opencode`). Runtime selection `--runtime go|python|node|shell` applies to launcher-based targets only; `--typescript` is available only with `--runtime node` on launcher-based lanes. Gemini, Codex package, and OpenCode authoring do not use `launcher.yaml` or executable runtime scaffolding. Claude defaults to the stable `Stop`/`PreToolUse`/`UserPromptSubmit` subset; use `--claude-extended-hooks` only when you intentionally want the full runtime-supported hook scaffold.
`plugin-kit-ai bootstrap` is the stable repo-local first-run helper for `python` and `node` launcher-based projects on `codex-runtime` and `claude`. It uses lockfile-first manager detection for Python and Node ecosystems, then installs dependencies and runs `build` for TypeScript-shaped Node projects. The same command remains `public-beta` for `shell`.
`plugin-kit-ai doctor` is the stable read-only readiness check for `python` and `node` launcher-based projects on `codex-runtime` and `claude`. It reports lane, runtime, detected manager, readiness status, and next commands without mutating files. The same command remains `public-beta` for `shell`.
`plugin-kit-ai export` is the stable portable handoff surface for `python` and `node` launcher-based projects on `codex-runtime` and `claude`. It writes a deterministic `.tar.gz` bundle, but does not extend `install`. The same command remains `public-beta` for `shell`.
`plugin-kit-ai bundle install` is the stable local bundle installer for exported Python/Node handoff archives. It only accepts local `.tar.gz` bundles created by `plugin-kit-ai export`, unpacks them into `--dest`, and prints next steps. It does not extend `install`, and it does not run `bootstrap` automatically.
`plugin-kit-ai bundle fetch` is the `public-beta` remote bundle fetch/install companion for exported Python/Node handoff archives. It supports direct HTTPS URLs and GitHub Releases bundle discovery, but remains separate from both stable local `bundle install` and binary-only `install`.
`plugin-kit-ai validate` checks package-standard projects, including generated-artifact drift, manifest warnings for unknown `plugin.yaml` keys, and Claude authored-hook routing consistency against `launcher.yaml.entrypoint`.
`plugin-kit-ai render` renders native target artifacts from the authored package-standard layout, `plugin-kit-ai import` backfills that layout from current native Claude/Codex/Gemini/OpenCode artifacts, and `plugin-kit-ai normalize` rewrites `plugin.yaml` into the package-standard shape.
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
| OpenCode workspace-config lane | `opencode` |

## Fast Local Plugin

For repo-local plugins where fast iteration matters more than packaged distribution:

- Good fit: Python or Node teams wiring a local Claude/Codex plugin into an existing repo
- Guarantee level: stable repo-local path for `python` and `node`, with `validate --strict` as the readiness gate
- Main non-goals: universal dependency management, packaged distribution, and runtime parity with the Go SDK

```bash
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime python
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
./bin/plugin-kit-ai doctor ./my-plugin
./bin/plugin-kit-ai bootstrap ./my-plugin
./bin/plugin-kit-ai export ./my-plugin --platform codex-runtime
./bin/plugin-kit-ai bundle install ./my-plugin/my-plugin_codex-runtime_python_bundle.tar.gz --dest ./handoff-plugin
./bin/plugin-kit-ai bundle fetch --url https://example.com/my-plugin_codex-runtime_python_bundle.tar.gz --dest ./handoff-plugin
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
./bin/plugin-kit-ai init my-plugin --platform opencode
```

Default `init my-plugin` expands to `--platform codex-runtime --runtime go`.

## Already Have Native Config

For migrating current Claude/Codex/Gemini native files into the package-standard authored layout:

- Good fit: teams adopting managed source-of-truth workflows without hand-editing vendor files
- Guarantee level: supported import bridge into the authored package model
- Main non-goals: keeping native target files as the long-term authored source of truth

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai import ./native-plugin --from codex-native
```

Current behavior and contract details:

- `init`: package-standard scaffold for `codex-runtime`, `codex-package`, `claude`, `gemini`, or `opencode`; launcher-based targets support Go-first or executable runtimes, while Gemini, Codex package, and OpenCode stay launcher-less
- `bootstrap`: stable repo-local first-run helper for `python` and `node` launcher-based projects on `codex-runtime` and `claude`; `public-beta` for `shell`; no-op for `go`, `codex-package`, and `gemini`
- `doctor`: stable read-only readiness check for `python` and `node` launcher-based projects on `codex-runtime` and `claude`; `public-beta` for `shell`
- `export`: stable deterministic `.tar.gz` handoff bundle for `python` and `node` launcher-based projects on `codex-runtime` and `claude`; `public-beta` for `shell`
- `bundle install`: stable local installer for exported Python/Node bundles; local-file-only, unpack-only, and intentionally separate from `install`
- `bundle fetch`: `public-beta` remote installer for exported Python/Node bundles; supports direct HTTPS URLs and GitHub release bundle selection, but stays separate from `install`
- `init --platform claude`: stable-default Claude scaffold; `--claude-extended-hooks` opts into the full runtime-supported hook set
- `init --platform gemini`: richer packaging starter with `targets/gemini/package.yaml`, `targets/gemini/contexts/GEMINI.md`, and no launcher/runtime scaffold
- `init --platform opencode`: workspace-config starter with `targets/opencode/package.yaml`, optional `targets/opencode/config.extra.json`, and no launcher/runtime scaffold
- `render`: render native Claude artifacts, Codex package/runtime lane artifacts, and Gemini CLI extension packaging artifacts from `plugin.yaml` plus `targets/<platform>/...`
- `import`: create the package-standard authored layout from current native Claude/Codex/Gemini/OpenCode artifacts; Gemini import remains extension-packaging-only and OpenCode import remains workspace-config-only
- `inspect`: explain the discovered package graph, target class, and managed artifacts
- `normalize`: rewrite `plugin.yaml` into the package-standard shape and drop unknown fields
- `validate`: package-standard project validation, generated-artifact drift checks, and non-failing manifest warnings; `--strict` promotes warnings to errors for CI
- `capabilities`: generated target/package support by default, or runtime support with `--mode runtime`
- `install`: plugin binary from GitHub Releases with checksum verification
- `version`: build/version info
- `skills init|validate|render`: experimental SKILL.md authoring and agent render tooling

For the experimental skills subsystem, handwritten `skills/<name>/SKILL.md` is supported directly. `skills init` is convenience scaffold, not a required entrypoint.
For `install`, the stable CLI promise is limited to verified installation of third-party plugin binaries from GitHub Releases. It does not include self-update for the `plugin-kit-ai` CLI itself.
Executable runtime scaffolds for `python` and `node` are the stable repo-local local-runtime subset on `codex-runtime` and `claude`; launcher-based `shell` authoring remains `public-beta`. These paths provide bounded ecosystem bootstrap rather than a universal dependency-management contract for interpreted runtimes. `plugin.yaml` plus `targets/<platform>/...` is the only supported authored package standard; native Claude/Codex/Gemini/OpenCode config files are rendered managed artifacts, and `import` exists to recover authored state from those native layouts. Unknown manifest keys warn via `validate`. Gemini is a `packaging-only Gemini CLI extension target` in this CLI surface, not a production-ready runtime target; the supported Gemini contract is the full official extension packaging lane through `gemini-extension.json`, inline `mcpServers`, target-native contexts, settings, themes, commands, hooks, policies, `manifest.extra.json`, and local `gemini extensions link|config|disable|enable` workflows. OpenCode is a `workspace-config OpenCode target` in this CLI surface, not a first-class local JS/TS plugin-code runtime target; the supported OpenCode contract is `opencode.json`, `plugin` package refs, inline `mcp`, mirrored `.opencode/skills/`, `targets/opencode/config.extra.json` passthrough, and explicit unsupported boundaries for local plugin code and workspace directories such as `agents`, `commands`, `themes`, and `modes`. `plugin-kit-ai capabilities` defaults to the target/package view so package authors can see target class, production boundary, and managed artifacts first. For generated Python and Node projects, `plugin-kit-ai doctor <path>` is the read-only readiness check, `plugin-kit-ai bootstrap <path>` is the supported first-run helper before `validate --strict`, and `plugin-kit-ai export <path> --platform <target>` is the stable portable handoff surface for that subset.
Generated Claude/Codex package-runtime config shapes are part of the repo-owned contract surface; `render --check` and the deterministic `polyglot-smoke` lane are the primary drift guards for that wiring. Claude authored hook routing consistency with `launcher.yaml.entrypoint` is enforced by `validate --strict`.

Executable runtime matrix:

| Runtime | Status | Scope | Bootstrap |
|---------|--------|-------|-----------|
| `go` | stable | default production path | Go `1.22+` |
| `python` | stable local-runtime subset | repo-local on `codex-runtime` and `claude` | lockfile-first manager detection; `venv`/`requirements.txt`/`uv` expect repo-local `.venv`, `poetry`/`pipenv` can validate via manager-owned envs |
| `node` | stable local-runtime subset | repo-local on `codex-runtime` and `claude` | lockfile-first manager detection (`bun`, `pnpm`, `yarn`, `npm`); JavaScript by default, TypeScript via `--runtime node --typescript` |
| `shell` | public-beta | repo-local only | POSIX shell on Unix, `bash` in `PATH` on Windows |

For interpreted runtimes, `validate --strict` is the canonical CI-grade readiness gate.
`plugin-kit-ai install` remains binary-only and does not bootstrap or distribute Python/Node/Shell plugin dependencies. `export` is the handoff bundle surface for interpreted runtimes; it is not an installer. `bundle install` is the stable local unpack/install companion for exported Python/Node bundles only. `bundle fetch` is the beta remote companion for direct HTTPS URLs and GitHub Releases bundle discovery, not a widening of `install`.

Production-ready target boundary in the current contract:

- Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` set
- Claude package authoring also supports first-class `targets/claude/settings.json`, `targets/claude/lsp.json`, `targets/claude/user-config.json`, and `targets/claude/manifest.extra.json`
- Codex runtime: production-ready within the stable `Notify` path
- Codex package: production-ready official plugin package lane
- Gemini: full packaging-only Gemini CLI extension lane through `render|import|validate` and local `extensions link|config|disable|enable`
- OpenCode: workspace-config-only lane through `render|import|validate`, package refs, inline MCP, and mirrored portable skills

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
