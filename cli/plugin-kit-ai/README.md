# plugin-kit-ai CLI

Canonical repo: `github.com/777genius/plugin-kit-ai`. The CLI lives in the submodule `github.com/777genius/plugin-kit-ai/cli` and builds the **`plugin-kit-ai`** binary: `init`, `bootstrap`, `doctor`, `dev`, `test`, `export`, `bundle install`, `bundle fetch`, `bundle publish`, `generate`, `import`, `inspect`, `normalize`, `validate`, `capabilities`, `install`, `version`, plus experimental `skills` authoring commands.

Current CLI contract status in this source tree: `public-stable` shipped in `v1.0.0`. Repository-wide compatibility and release policy live in [../../docs/SUPPORT.md](../../docs/SUPPORT.md) and [../../docs/RELEASE.md](../../docs/RELEASE.md).

`plugin-kit-ai init` scaffolds a package-standard project for **Codex runtime** (`--platform codex-runtime`, default), **Codex package** (`--platform codex-package`), **Claude** (`--platform claude`), **Gemini** (`--platform gemini`), **OpenCode** (`--platform opencode`), or **Cursor** (`--platform cursor`). Runtime selection `--runtime go|python|node|shell` applies to launcher-based targets only; `--typescript` is available only with `--runtime node` on launcher-based lanes. Gemini supports packaging-only authoring by default and a production-ready 9-hook Go runtime lane via `--platform gemini --runtime go`. Codex package, OpenCode, and Cursor remain non-launcher targets. Claude defaults to the stable `Stop`/`PreToolUse`/`UserPromptSubmit` subset; use `--claude-extended-hooks` only when you intentionally want the full runtime-supported hook scaffold.
`plugin-kit-ai bootstrap` is the stable repo-local first-run helper for `python` and `node` launcher-based projects on `codex-runtime` and `claude`. It uses lockfile-first manager detection for Python and Node ecosystems, then installs dependencies and runs `build` for TypeScript-shaped Node projects. The same command remains `public-beta` for `shell`.
`plugin-kit-ai doctor` is the stable read-only readiness check for `python` and `node` launcher-based projects on `codex-runtime` and `claude`. It reports lane, runtime, detected manager, readiness status, and next commands without mutating files. The same command remains `public-beta` for `shell`.
`plugin-kit-ai dev` is a `public-beta` watch loop for launcher-based runtime targets. It polls the workspace, re-generates managed artifacts, performs runtime-aware rebuilds, runs strict validation, and reruns the selected stable fixture smoke tests on every change.
`plugin-kit-ai test` is the stable fixture-driven smoke surface for the declared stable Claude and Codex runtime events. Generated launcher-based Claude and Codex runtime projects now pre-seed the default `fixtures/` and `goldens/` layout during `init`, so `test` and `dev` work out of the box. `plugin-kit-ai test` loads JSON fixtures, invokes the configured launcher entrypoint with the correct carrier, compares `stdout`, `stderr`, and `exitcode` against golden files, and supports `--update-golden` to record the current outputs.
`plugin-kit-ai export` is the stable portable handoff surface for `python` and `node` launcher-based projects on `codex-runtime` and `claude`. It writes a deterministic `.tar.gz` bundle, but does not extend `install`. The same command remains `public-beta` for `shell`.
`plugin-kit-ai bundle install` is the stable local bundle installer for exported Python/Node handoff archives. It only accepts local `.tar.gz` bundles created by `plugin-kit-ai export`, unpacks them into `--dest`, and prints next steps. It does not extend `install`, and it does not run `bootstrap` automatically.
`plugin-kit-ai bundle fetch` is the stable remote bundle fetch/install companion for exported Python/Node handoff archives. It supports direct HTTPS URLs and GitHub Releases bundle discovery, but remains separate from both stable local `bundle install` and binary-only `install`.
`plugin-kit-ai bundle publish` is the stable GitHub Releases publish companion for exported Python/Node handoff archives. It exports the same bundle contract, creates a published release by default, supports `--draft` as an opt-in safety mode, uploads the bundle plus a sibling `.sha256` asset, and stays separate from both stable local `bundle install` and binary-only `install`.
`plugin-kit-ai validate` checks package-standard projects, including generated-artifact drift, manifest warnings for unknown `plugin.yaml` keys, and Claude authored-hook routing consistency against `launcher.yaml.entrypoint`.
For CI and other tooling, `plugin-kit-ai validate --format json` emits the versioned `plugin-kit-ai/validate-report` contract with `schema_version: 1`, explicit `outcome` values, and summary counters.
Use [../../docs/VALIDATE_JSON_CONTRACT.md](../../docs/VALIDATE_JSON_CONTRACT.md) for the machine-readable validation ABI and [../../docs/CODEX_TARGET_BOUNDARY.md](../../docs/CODEX_TARGET_BOUNDARY.md) when choosing between `codex-runtime` and `codex-package`.
`plugin-kit-ai generate` generates native target artifacts from the authored package-standard layout, `plugin-kit-ai import` backfills that layout from current native Claude/Codex/Gemini/OpenCode/Cursor artifacts into canonical `src/` authored inputs, and `plugin-kit-ai normalize` rewrites `src/plugin.yaml` into the package-standard shape.
`plugin-kit-ai capabilities` defaults to the target/package view and supports `--mode runtime` for runtime-event metadata.

Supported bootstrap paths for the CLI itself:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

```bash
npm i -g plugin-kit-ai
plugin-kit-ai version
```

```bash
pipx install plugin-kit-ai
plugin-kit-ai version
```

```bash
curl -fsSL https://raw.githubusercontent.com/777genius/plugin-kit-ai/main/scripts/install.sh | sh
plugin-kit-ai version
```

```bash
# from repository root when developing plugin-kit-ai itself
go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai
```

Maintainer note: building the checked-in monorepo workspace currently requires
Go `1.23.x` for the CLI module and CI lanes. Generated Go plugin projects stay
on the public Go SDK path with Go `1.22+`.

Choose the path that matches your goal:

| Goal | Recommended lane |
|------|------------------|
| local notify/runtime plugin in your repo | `codex-runtime` |
| official Codex bundle/package output | `codex-package` |
| Claude hook runtime plugin | `claude` |
| Gemini CLI extension package | `gemini` |
| OpenCode workspace-config lane | `opencode` |
| Cursor workspace-config lane | `cursor` |

## Fast Local Plugin

For repo-local plugins where fast iteration matters more than packaged distribution:

- Good fit: Python or Node teams wiring a local Claude/Codex plugin into an existing repo
- Guarantee level: stable repo-local path for `python` and `node`, with `validate --strict` as the readiness gate
- Main non-goals: universal dependency management, packaged distribution, and runtime parity with the Go SDK

```bash
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime python
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime python --runtime-package --runtime-package-version 1.0.6
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript --runtime-package --runtime-package-version 1.0.6
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript --extras
./bin/plugin-kit-ai doctor ./my-plugin
./bin/plugin-kit-ai bootstrap ./my-plugin
./bin/plugin-kit-ai export ./my-plugin --platform codex-runtime
./bin/plugin-kit-ai bundle publish ./my-plugin --platform codex-runtime --repo owner/repo --tag v1
./bin/plugin-kit-ai bundle install ./my-plugin/my-plugin_codex-runtime_python_bundle.tar.gz --dest ./handoff-plugin
./bin/plugin-kit-ai bundle fetch --url https://example.com/my-plugin_codex-runtime_python_bundle.tar.gz --dest ./handoff-plugin
```

Fast starter repos: [../../examples/starters/README.md](../../examples/starters/README.md)
Reference repos: [../../examples/local/README.md](../../examples/local/README.md)
Helper delivery modes: [../../docs/CHOOSING_HELPER_DELIVERY_MODE.md](../../docs/CHOOSING_HELPER_DELIVERY_MODE.md)
Released CLIs auto-pin `plugin-kit-ai-runtime` to their own stable tag and print the chosen helper dependency; development builds require `--runtime-package-version`.

Official starter templates:

- [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- [plugin-kit-ai-starter-codex-python](https://github.com/777genius/plugin-kit-ai-starter-codex-python)
- [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)
- [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- [plugin-kit-ai-starter-claude-python](https://github.com/777genius/plugin-kit-ai-starter-claude-python)
- [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

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
./bin/plugin-kit-ai init my-plugin --platform cursor
```

Default `init my-plugin` expands to `--platform codex-runtime --runtime go`.

## Already Have Native Config

For migrating current Claude/Codex/Gemini/OpenCode/Cursor native files into the package-standard authored layout:

- Good fit: teams adopting managed source-of-truth workflows without hand-editing vendor files
- Guarantee level: supported import bridge into the authored package model
- Main non-goals: keeping native target files as the long-term authored source of truth

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
```

Current behavior and contract details:

- `init`: package-standard scaffold for `codex-runtime`, `codex-package`, `claude`, `gemini`, `opencode`, or `cursor`; launcher-based targets support Go-first or executable runtimes, Gemini defaults to launcher-less packaging but also supports `--runtime go` for the promoted 9-hook runtime lane, while Codex package, OpenCode, and Cursor stay launcher-less; Codex package extras now include first-class `interface.json`, `app.json`, and `manifest.extra.json` starters alongside richer package metadata authoring, and the empty Codex app starter stays inactive until you replace it with a real app manifest
- `bootstrap`: stable repo-local first-run helper for `python` and `node` launcher-based projects on `codex-runtime` and `claude`; `public-beta` for `shell`; no-op for `go`, `codex-package`, and `gemini`
- `doctor`: stable read-only readiness check for `python` and `node` launcher-based projects on `codex-runtime` and `claude`; `public-beta` for `shell`
- `dev`: `public-beta` watch loop for launcher-based runtime targets; auto-generates, performs runtime-aware rebuilds, runs strict validation, and reruns the selected stable fixtures; supports `--once` for a single cycle
- `test`: stable fixture-driven smoke command for the declared stable Claude and Codex runtime events; defaults to `fixtures/<platform>/<event>.json` plus `goldens/<platform>/<event>.*`, supports `--all`, and supports `--update-golden`
- `export`: stable deterministic `.tar.gz` handoff bundle for `python` and `node` launcher-based projects on `codex-runtime` and `claude`; `public-beta` for `shell`
- `bundle install`: stable local installer for exported Python/Node bundles; local-file-only, unpack-only, and intentionally separate from `install`
- `bundle fetch`: stable remote installer for exported Python/Node bundles; URL mode verifies `--sha256` or `<url>.sha256`, GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`, and the surface stays separate from `install`
- `bundle publish`: stable GitHub Releases publisher for exported Python/Node bundles; reuses `export`, creates a published release by default, supports `--draft` as an opt-in safety mode, uploads the bundle plus `<asset>.sha256`, and stays separate from `install`
- recommended CLI bootstrap: `brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai`
- official npm CLI bootstrap (`public-beta`): `npm i -g plugin-kit-ai` or `npx plugin-kit-ai@latest ...` downloads the matching published GitHub Releases binary, verifies `checksums.txt`, and keeps the binary origin aligned with Homebrew/script
- official pipx CLI bootstrap when a release is published to PyPI (`public-beta`): `pipx install plugin-kit-ai` or `pipx run plugin-kit-ai version` downloads the matching published GitHub Releases binary, verifies `checksums.txt`, and keeps the binary origin aligned with Homebrew/script
- for Python/Node plugin authoring helpers rather than CLI installation, the shared package path is `plugin-kit-ai-runtime` on PyPI and npm; the default scaffold still vendors helper files so `init -> bootstrap` stays hermetic
- official CLI bootstrap: `scripts/install.sh` resolves the latest published stable release by default, verifies `checksums.txt`, auto-detects OS/arch, and installs the matching `plugin-kit-ai` tarball into `BIN_DIR`
- official CI setup action: `777genius/plugin-kit-ai/setup-plugin-kit-ai@v1` reuses that same verified release contract and adds the installed binary to `PATH`
- `init --extras` on stable interpreted `python`/`node` launcher-based projects emits `.github/workflows/bundle-release.yml`, which uses `setup-plugin-kit-ai@v1` plus `doctor -> bootstrap -> validate --strict -> bundle publish`
- `init --platform claude`: stable-default Claude scaffold; `--claude-extended-hooks` opts into the full runtime-supported hook set
- `init --platform gemini`: richer packaging starter with `targets/gemini/package.yaml` and `targets/gemini/contexts/GEMINI.md`; add `--runtime go` for the optional Gemini Go production-ready 9-hook runtime lane
- `init --platform opencode`: workspace-config starter with `targets/opencode/package.yaml`, optional OpenCode command/agent/theme starters, and an OpenCode-compatible skill stub when `--extras` is used; OpenCode extras now use official-style named async plugin exports, and helper-based custom tools belong in `targets/opencode/package.json`; no launcher/runtime scaffold
- `init --platform cursor`: workspace-config starter with `src/targets/cursor/rules/project.mdc`, root boundary docs `CLAUDE.md` and `AGENTS.md`, and no launcher/runtime scaffold
- `generate`: generate native Claude artifacts, Codex package/runtime lane artifacts, Gemini CLI extension packaging artifacts, OpenCode workspace config artifacts, and Cursor workspace config artifacts from canonical `src/plugin.yaml`, optional `src/mcp/servers.yaml`, and `src/targets/<platform>/...`; when `src/publish/codex/marketplace.yaml` is authored for `codex-package`, `generate` also produces repo-level `.agents/plugins/marketplace.json`, and when `src/publish/claude/marketplace.yaml` is authored for `claude`, `generate` also produces `.claude-plugin/marketplace.json`
- `import`: create the package-standard authored layout from current native Claude/Codex/Gemini/OpenCode/Cursor artifacts; Gemini import preserves the extension package lane and optional Go runtime metadata when present, OpenCode import remains workspace-config-only with explicit `--include-user-scope` support for home-dir OpenCode sources, and Cursor import remains workspace-config-only for the documented `.cursor` subset
- `inspect`: explain the discovered package graph, target class, managed artifacts, and the publication summary; `--format json` now includes a `publication` block with publication-capable package targets and any authored `publish/...` channels discovered in the repo, including channel metadata such as Codex policies and Gemini gallery distribution hints
- `publish`: first-class bounded publish workflow for documented publication channels; `publish --channel codex-marketplace|claude-marketplace --dest <marketplace-root>` materializes the corresponding local marketplace root, `--dry-run` previews the write, `publish --channel gemini-gallery --dry-run` emits a repository/release publication plan with bounded Git and GitHub readiness diagnostics instead of pretending Gemini has a local marketplace-root flow, `publish --all --dry-run` orchestrates all authored `publish/...` channels in one combined plan, and `publish --format json` emits the versioned `plugin-kit-ai/publish-report` contract; `publish --all` apply mode stays intentionally absent because local materialization channels and repository/release planning channels still have different execution semantics
- `publication`: focused publication-layer view for package-capable targets and authored `publish/...` channels; use it when you want only package/channel publication data without the broader inspect surface; `publication --format json` emits the versioned `plugin-kit-ai/publication-report` contract, `publication doctor --format json` emits `plugin-kit-ai/publication-doctor-report` for readiness gating, `publication doctor --dest <marketplace-root>` also verifies an already materialized local Codex/Claude marketplace root, `publication materialize --target codex-package|claude --dest <marketplace-root>` builds a safe local marketplace root with a copied package bundle plus merged catalog artifact, `publication remove --target codex-package|claude --dest <marketplace-root>` prunes that materialized plugin back out of the local marketplace root, and `--dry-run` on materialize/remove previews those local marketplace mutations without writing

Typical publication commands:

```bash
./bin/plugin-kit-ai publish ./my-plugin --channel codex-marketplace --dest ./local-codex-marketplace --dry-run
./bin/plugin-kit-ai publish ./my-plugin --channel claude-marketplace --dest ./local-claude-marketplace
./bin/plugin-kit-ai publish ./my-plugin --channel gemini-gallery --dry-run --format json
./bin/plugin-kit-ai publish ./my-plugin --all --dry-run --dest ./local-marketplaces --format json
```

Need the short user-facing guide for choosing the right publication flow for Codex, Claude, or Gemini? See [How To Publish Plugins](https://777genius.github.io/plugin-kit-ai/docs/en/guide/how-to-publish-plugins.html).
- `normalize`: rewrite `plugin.yaml` into the package-standard shape and drop unknown fields
- `validate`: package-standard project validation, generated-artifact drift checks, authored `publish/...` schema validation, and non-failing manifest warnings; when publication channels are discoverable, text and JSON output now also surface a publication summary; `--strict` promotes warnings to errors for CI
- `capabilities`: generated target/package support by default, or runtime support with `--mode runtime`
- `install`: plugin binary from GitHub Releases with checksum verification
- `version`: build/version info
- `skills init|validate|generate`: experimental SKILL.md authoring and agent generate tooling

For the experimental skills subsystem, handwritten `skills/<name>/SKILL.md` is supported directly. `skills init` is convenience scaffold, not a required entrypoint.
For `install`, the stable CLI promise is limited to verified installation of third-party plugin binaries from GitHub Releases. It does not include self-update for the `plugin-kit-ai` CLI itself; use Homebrew as the recommended local install path, the `public-beta` npm wrapper as the official JS ecosystem path, the `public-beta` PyPI/pipx wrapper when that release was published to PyPI, `scripts/install.sh` as the verified fallback, or `setup-plugin-kit-ai@v1` in CI.
Executable runtime scaffolds for `python` and `node` are the stable repo-local local-runtime subset on `codex-runtime` and `claude`; launcher-based `shell` authoring remains `public-beta`. These paths provide bounded ecosystem bootstrap rather than a universal dependency-management contract for interpreted runtimes. Canonical new authoring uses `src/plugin.yaml`, optional `src/mcp/servers.yaml`, optional `src/launcher.yaml`, optional `src/skills/**`, optional `src/publish/**`, and `src/targets/<platform>/...`; plugin-root native Claude/Codex/Gemini/OpenCode/Cursor config files are generated managed artifacts, and `import` exists to recover authored state from those native layouts. Root `CLAUDE.md` and `AGENTS.md` are boundary docs that tell humans and agents to edit only `src/`. Unknown manifest keys warn via `validate`. Codex package authoring now treats `src/plugin.yaml` as the default source for shared package metadata, keeps `src/targets/codex-package/package.yaml` as an override surface when needed, keeps `src/targets/codex-package/interface.json` as the official interface surface, keeps `src/targets/codex-package/app.json` as the optional app surface, reserves `src/targets/codex-package/manifest.extra.json` for unsupported future manifest fields only, requires `.codex-plugin/` to contain only `plugin.json`, and keeps `.app.json` / `.mcp.json` as managed root sidecars only when `.codex-plugin/plugin.json` references them; Codex runtime keeps `src/targets/codex-runtime/config.extra.toml` as the explicit repo-local passthrough surface beyond managed `model` and `notify`. Gemini is a production-ready Gemini CLI extension target in this CLI surface; the supported Gemini contract is the full official extension packaging lane through `gemini-extension.json`, inline `mcpServers`, target-native contexts, settings, themes, commands, hooks, policies, `manifest.extra.json`, local `gemini extensions link|config|disable|enable` workflows, plus the production-ready 9-hook Go runtime lane behind `plugin-kit-ai init --platform gemini --runtime go`. OpenCode is a `workspace-config OpenCode target` in this CLI surface; the supported OpenCode contract is `opencode.json` or `opencode.jsonc`, `plugin` package refs, inline `mcp`, validated skills mirrored into `.opencode/skills/`, first-class workspace commands/agents/themes mirrored into `.opencode/{commands,agents,themes}/`, first-class standalone tools mirrored into `.opencode/tools/`, stable official-style local JS/TS plugin code mirrored into `.opencode/plugins/`, stable shared dependency metadata mirrored into `.opencode/package.json` for both tools and plugins, beta `custom_tools` spanning standalone tools and plugin code, explicit `--include-user-scope` import for `~/.config/opencode`, and `src/targets/opencode/config.extra.json` passthrough for broader permission-first config. Cursor is a `workspace-config Cursor target` in this CLI surface; the supported Cursor contract is `.cursor/mcp.json`, project-root `.cursor/rules/**`, and strict documented-subset behavior that defers root boundary docs `CLAUDE.md` and `AGENTS.md`, global `~/.cursor/mcp.json`, nested non-root `.cursor/rules/**`, and JSONC. `plugin-kit-ai capabilities` defaults to the target/package view so package authors can see target class, production boundary, and managed artifacts first. For generated Python and Node projects, `plugin-kit-ai doctor <path>` is the read-only readiness check, `plugin-kit-ai bootstrap <path>` is the supported first-run helper before `validate --strict`, `plugin-kit-ai test <path> --platform <target> --event <event>` is the stable fixture-driven smoke command for the declared stable Claude and Codex runtime events, and `plugin-kit-ai export <path> --platform <target>` is the stable portable handoff surface for that subset. Generated launcher-based Claude and Codex runtime projects now pre-seed `fixtures/<platform>/<event>.json` and `goldens/<platform>/<event>.*` during `init`, so the default scaffold is immediately ready for `plugin-kit-ai test` and `plugin-kit-ai dev`. `plugin-kit-ai test` uses that layout by default, emits JSON-friendly per-case summaries plus mismatch previews, supports `--all` to run every stable event for the selected platform, and supports `--update-golden` to refresh the checked-in output contract. For config-first `codex-package`, `gemini`, `opencode`, and `cursor` scaffolds, `plugin-kit-ai init --extras` now also pre-seeds `src/mcp/servers.yaml`, so the first portable MCP flow is ready before the first `generate`. When teams need a shared helper dependency instead of vendored helper files, `plugin-kit-ai init ... --runtime-package` is the official opt-in scaffold mode; see [../../docs/CHOOSING_HELPER_DELIVERY_MODE.md](../../docs/CHOOSING_HELPER_DELIVERY_MODE.md).
Generated Claude/Codex package-runtime config shapes are part of the repo-owned contract surface; `generate --check` and the deterministic `polyglot-smoke` lane are the primary drift guards for that wiring. Claude authored hook routing consistency with `launcher.yaml.entrypoint` is enforced by `validate --strict`.

Executable runtime matrix:

| Runtime | Status | Scope | Bootstrap |
|---------|--------|-------|-----------|
| `go` | stable | default production path | Go `1.22+` |
| `python` | stable local-runtime subset | repo-local on `codex-runtime` and `claude` | lockfile-first manager detection; `venv`/`requirements.txt`/`uv` expect repo-local `.venv`, `poetry`/`pipenv` can validate via manager-owned envs |
| `node` | stable local-runtime subset | repo-local on `codex-runtime` and `claude` | lockfile-first manager detection (`bun`, `pnpm`, `yarn`, `npm`); JavaScript by default, TypeScript via `--runtime node --typescript` |
| `shell` | public-beta | repo-local only | POSIX shell on Unix, `bash` in `PATH` on Windows |

For interpreted runtimes, `validate --strict` is the canonical CI-grade readiness gate.
`plugin-kit-ai install` remains binary-only and does not bootstrap or distribute Python/Node/Shell plugin dependencies. `export` is the handoff bundle surface for interpreted runtimes; it is not an installer. `bundle install` is the stable local unpack/install companion for exported Python/Node bundles only. `bundle fetch` is the stable remote companion for direct HTTPS URLs and GitHub Releases bundle discovery, with URL `--sha256`/sidecar verification and GitHub `checksums.txt`/sidecar verification, not a widening of `install`. `bundle publish` is the stable GitHub Releases producer-side companion for the same exported bundle contract; it uploads the bundle and `<asset>.sha256`, but does not introduce registry publishing or widen `install`.

Production-ready target boundary in the current contract:

- Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` set
- Claude package authoring also supports first-class `targets/claude/settings.json`, `targets/claude/lsp.json`, `targets/claude/user-config.json`, and `targets/claude/manifest.extra.json`
- Codex runtime: production-ready within the stable `Notify` path
- Codex package: production-ready official plugin package lane
- Gemini: full Gemini CLI extension lane through `generate|import|validate` and local `extensions link|config|disable|enable`, plus an optional production-ready Go runtime lane for `SessionStart`, `SessionEnd`, `BeforeModel`, `AfterModel`, `BeforeToolSelection`, `BeforeAgent`, `AfterAgent`, `BeforeTool`, and `AfterTool`, with dedicated opt-in real CLI runtime smoke via `make test-gemini-runtime-live`
- OpenCode: workspace-config-only lane through `generate|import|validate`, package refs, inline MCP, validated portable skills, first-class workspace commands/agents/themes, beta standalone tools, stable local plugin code plus stable shared package metadata for tools and plugins, JSON/JSONC plus explicit user-scope import, permission-first passthrough config semantics, and beta `custom_tools` spanning standalone tools and plugin code
- Cursor: workspace-config-only lane through `generate|import|validate`, `.cursor/mcp.json`, project-root `.cursor/rules/**`, and plugin-root boundary docs `CLAUDE.md` / `AGENTS.md`, without a VS Code extension packaging contract or broader undocumented Cursor surface claims

Canonical production plugin lane:

```bash
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai generate ./my-plugin
./bin/plugin-kit-ai generate ./my-plugin --check
./bin/plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

`plugin-kit-ai install` prints a deterministic success summary:

- installed file path
- release ref with source (`tag` or `latest`)
- selected asset name
- target GOOS/GOARCH
- overwrite notice only when an existing file was replaced

Supported and unsupported release layouts for `install` are documented in [../../docs/INSTALL_COMPATIBILITY.md](../../docs/INSTALL_COMPATIBILITY.md).
Production authoring guidance, fast starter repos, and deeper reference examples live in [../../docs/PRODUCTION.md](../../docs/PRODUCTION.md), [../../examples/starters/README.md](../../examples/starters/README.md), [../../examples/local/README.md](../../examples/local/README.md), and [../../examples/plugins/README.md](../../examples/plugins/README.md).

See the root [README.md](../../README.md) for current CLI behavior, shipped scope, and canonical support links.
See [../../docs/EXECUTABLE_ABI.md](../../docs/EXECUTABLE_ABI.md) for the low-level executable plugin contract.
See [../../docs/SKILLS.md](../../docs/SKILLS.md) for the skills workflow, positioning, and examples.

Repo-local maintainer development is handled by the checked-in workspace wiring in this monorepo.
Public Go starter and scaffold flows should consume the released SDK module directly:

- `go get github.com/777genius/plugin-kit-ai/sdk@v1.0.6`
