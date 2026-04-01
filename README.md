# plugin-kit-ai

Polyglot AI CLI plugin runtime with a first-class Go SDK.

`plugin-kit-ai` is a polyglot system for building plugins for AI coding CLIs.
It gives you:

- a typed Go SDK for Claude and Codex
- a package-standard authoring model for plugin repos
- managed render/validate tooling for Claude, Codex package/runtime lanes, Gemini extension artifacts, OpenCode workspace config, and Cursor workspace config
- stable repo-local authoring lanes for `python` and `node`, plus a beta `shell` lane

Go remains the recommended default when you want the smoothest production path:

- typed handlers and the strongest supported authoring contract
- a self-contained compiled binary instead of a required external language runtime
- less bootstrap friction for downstream users and CI

Python and Node are still supported first-class for the stable repo-local subset:

- use them when your team already works in that runtime
- expect `plugin-kit-ai doctor`, `bootstrap`, `validate --strict`, `export`, and bundle handoff support on `codex-runtime` and `claude`
- expect to install and manage the external runtime yourself: Python `3.10+` or Node.js `20+`

Use it when you want one of these outcomes:

- build a real plugin repo for Claude, Codex package/runtime lanes, Gemini packaging, OpenCode workspace config, or Cursor workspace config with a clear support boundary
- keep authored plugin state in versioned source files instead of hand-editing vendor config
- generate and validate native target files deterministically
- recommend Go where it has real operational advantages, without blocking Python or Node teams from a stable supported path

Do not use it if your main goal is:

- marketplace-style packaged distribution for Python or Node plugins
- a universal dependency-management layer for interpreted-language ecosystems
- a fully stable runtime contract for every Claude hook or every target

## Who It Is For

`plugin-kit-ai` is aimed at three audiences:

- plugin authors who want either a typed Go SDK or a stable repo-local Python/Node path for Claude or the Codex runtime lane
- teams that already have native Claude/Codex/Gemini/OpenCode/Cursor config files and want to move to a managed source-of-truth model
- maintainers who need render, drift detection, strict validation, and deterministic release gates

If you are a solo hacker trying to wire a tiny local script into a CLI, this may still help, but the main value is stronger repo structure and clearer contracts.

## What Is Stable

Stable in the current source tree:

- typed Go SDK authoring for the approved Claude and Codex event set
- CLI commands `init`, `validate`, `test`, `capabilities`, `inspect`, `install`, `version`
- Go scaffold contract for Claude and Codex
- repo-local local-runtime authoring for `python` and `node` on `codex-runtime` and `claude`, including `doctor`, `bootstrap`, `validate --strict`, and `export`
- fixture-driven `test` for the declared stable Claude and Codex runtime events in that same repo-local subset
- generated helper-layer authoring API for `python` and `node` scaffolds, so users write handlers instead of hand-parsing argv/stdin
- TypeScript as the stable `node` authoring mode via `--runtime node --typescript`
- `bundle install` for local exported Python/Node bundles on `codex-runtime` and `claude`
- `bundle fetch` for remote exported Python/Node bundles on `codex-runtime` and `claude`
- `bundle publish` for GitHub Releases handoff of exported Python/Node bundles on `codex-runtime` and `claude`

Currently `public-beta`:

- `render`, `import`, and `normalize`
- `dev` watch mode for launcher-based runtime targets, with auto-render, auto-validate, runtime-aware rebuilds, and fixture reruns
- full Gemini CLI extension packaging lane through `render|import|validate`, with official-style `gemini-extension.json`, inline `mcpServers`, target-native contexts, settings, themes, commands, hooks, policies, and deterministic local extension dev flows
- OpenCode workspace-config lane through `render|import|validate`, with official-style `opencode.json`, first-class npm plugin package refs, inline MCP, mirrored portable skills, first-class workspace commands/agents/themes, first-class standalone tools in `public-beta`, stable official-style local JS/TS plugin subtree support plus stable shared package metadata for tools and plugins, layered project/user/env config-source import fidelity, permission-first passthrough config semantics, and `custom_tools` still in `public-beta`
- Cursor workspace-config lane through `render|import|validate`, with `.cursor/mcp.json`, project-root `.cursor/rules/**`, optional root `AGENTS.md`, compatibility import for `.cursorrules`, and a strict documented-subset boundary that defers `CLAUDE.md`, global `~/.cursor/mcp.json`, nested non-root rules, and JSONC
- launcher-based `shell` runtime authoring on `codex-runtime` and `claude`, including `init --runtime shell`, `doctor`, `bootstrap`, `validate --strict`, and `export`
- optional scaffold extras from `plugin-kit-ai init --extras`

Currently `public-experimental`:

- `plugin-kit-ai skills`
- any surface not explicitly promoted through the audit ledger

## Quick Start

Install the CLI the supported way:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

Official npm path (`public-beta`):

```bash
npm i -g plugin-kit-ai
plugin-kit-ai version
```

The official JavaScript ecosystem path is `npm i -g plugin-kit-ai` or `npx plugin-kit-ai@latest ...`.

Python wrapper path (`public-beta`, published releases only):

```bash
pipx install plugin-kit-ai
plugin-kit-ai version
```

When the PyPI wrapper has been published for a release, the Python ecosystem path is `pipx install plugin-kit-ai` or `pipx run plugin-kit-ai version`.

Verified fallback:

```bash
curl -fsSL https://raw.githubusercontent.com/777genius/plugin-kit-ai/main/scripts/install.sh | sh
plugin-kit-ai version
```

Install a specific release or a custom bin dir:

```bash
curl -fsSL https://raw.githubusercontent.com/777genius/plugin-kit-ai/main/scripts/install.sh | VERSION=v1.0.0 BIN_DIR="$HOME/.local/bin" sh
```

Build from source when you are developing this repo itself:

```bash
go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai
```

Choose your authoring runtime explicitly:

| Runtime | Best fit | Runtime requirement |
|---------|----------|---------------------|
| `go` | recommended default for production plugins and the least downstream friction | no separately installed language runtime for plugin users after you ship the binary |
| `python` | repo-local automation-heavy teams that already live in Python | Python `3.10+` available on the machine running the plugin |
| `node` | repo-local JavaScript/TypeScript teams that want the mainstream non-Go lane | Node.js `20+` available on the machine running the plugin |

Choose the path that matches your goal:

| Goal | Recommended lane |
|------|------------------|
| local notify/runtime plugin in your repo | `codex-runtime` |
| official Codex bundle/package output | `codex-package` |
| Claude hook runtime plugin | `claude` |
| Gemini CLI extension package | `gemini` |
| OpenCode workspace-config lane | `opencode` |
| Cursor workspace-config lane | `cursor` |

### Fast Local Plugin

For repo-local plugins where quick iteration matters more than packaged distribution:

- Good fit: Python or Node teams wiring a local Claude/Codex plugin into an existing repo
- Guarantee level: stable repo-local path for `python` and `node`, with `validate --strict` as the readiness gate
- Main non-goals: universal dependency management, packaged distribution, and full typed parity with the Go SDK
- Important runtime note: these lanes require an installed external runtime on the machine that executes the plugin
- Authoring surface: generated helper files such as `src/plugin_runtime.py` and `src/plugin-runtime.ts` give the supported handler-oriented API for these lanes
- Shared package path: official authoring helpers are also published as `plugin-kit-ai-runtime` on PyPI and npm; the scaffold stays self-contained by default so `init -> bootstrap` remains hermetic
- Opt-in shared-package scaffold: add `--runtime-package` when you want the generated project to import `plugin-kit-ai-runtime` instead of vendoring the helper file into `src/`
- Version pinning: released CLIs pin `plugin-kit-ai-runtime` to their own stable tag automatically and print the chosen helper dependency; development builds require `--runtime-package-version`
- Delivery-mode guide: [docs/CHOOSING_HELPER_DELIVERY_MODE.md](docs/CHOOSING_HELPER_DELIVERY_MODE.md)
- `doctor` now reports which runtimes and build tools the current shell can actually see, so PATH mismatches show up before `bootstrap` or `validate`

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

Fast starter repos: [examples/starters/README.md](examples/starters/README.md)
Reference repos: [examples/local/README.md](examples/local/README.md)

Official starter templates:

- [plugin-kit-ai-starter-codex-go](https://github.com/777genius/plugin-kit-ai-starter-codex-go)
- [plugin-kit-ai-starter-codex-python](https://github.com/777genius/plugin-kit-ai-starter-codex-python)
- [plugin-kit-ai-starter-codex-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-codex-node-typescript)
- [plugin-kit-ai-starter-claude-go](https://github.com/777genius/plugin-kit-ai-starter-claude-go)
- [plugin-kit-ai-starter-claude-python](https://github.com/777genius/plugin-kit-ai-starter-claude-python)
- [plugin-kit-ai-starter-claude-node-typescript](https://github.com/777genius/plugin-kit-ai-starter-claude-node-typescript)

Public Go SDK consumption now uses the canonical module path and submodule tag contract:

- `go get github.com/777genius/plugin-kit-ai/sdk@v1.0.6`
- release tags that change the SDK surface also cut `sdk/vX.Y.Z` from the same commit

Use `v1.0.6` or newer for normal Go module consumption. `v1.0.3` remains published as a root release, but it is known-bad for the Go SDK module path.

### Production-Ready Plugin Repo

For teams that want the strongest supported release and distribution story:

- Good fit: new plugin repos that want the clearest stable contract and typed handlers
- Guarantee level: strongest supported path in the current contract
- Main advantage: downstream users run a compiled plugin binary and do not need a separate Python or Node install just to execute your plugin

```bash
./bin/plugin-kit-ai init my-plugin
./bin/plugin-kit-ai init my-plugin --platform claude
./bin/plugin-kit-ai init my-plugin --platform claude --claude-extended-hooks
./bin/plugin-kit-ai init my-plugin --platform codex-package
./bin/plugin-kit-ai init my-plugin --platform gemini
./bin/plugin-kit-ai init my-plugin --platform opencode
./bin/plugin-kit-ai init my-plugin --platform cursor
```

Default `init my-plugin` is the strongest repo-local Codex runtime path: `--platform codex-runtime --runtime go`.

### Already Have Native Config

For teams migrating existing Claude/Codex/Gemini/OpenCode/Cursor native files into the package-standard authored layout:

- Good fit: existing plugin repos that want one managed source of truth
- Guarantee level: import bridge into the authored package-standard model
- Main non-goals: preserving native files as the long-term authored source of truth

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai import ./native-plugin --from codex-native
```

Run the canonical authoring lane:

```bash
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai render ./my-plugin
./bin/plugin-kit-ai render ./my-plugin --check
./bin/plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## Which Path Should You Choose

Choose Go if:

- you want the strongest supported path
- you want typed handlers and the cleanest production story
- you want the least bootstrap friction for other users of the repo
- you want plugin users to avoid installing a separate Python or Node runtime

Choose `python` or `node` if:

- the plugin is repo-local
- your team already works in that runtime
- you are comfortable owning the runtime bootstrap yourself
- you are fine requiring Python `3.10+` or Node.js `20+` on the machine running the plugin

Choose `shell` if:

- you need a bounded beta escape hatch and accept the narrower support contract

The default recommendation remains:

- Go on `codex-runtime` or `claude` when you want the strongest runtime lane and the most self-contained distribution story
- Node/TypeScript on launcher-based lanes when you want the most mainstream non-Go stable path
- Python on launcher-based lanes when your team is automation-heavy and repo-local by design
- `codex-package` when you want the official Codex package/bundle lane
- `shell` only as a bounded beta escape hatch on launcher-based lanes

## Project Model

`plugin-kit-ai` has one canonical authored project shape:

- repo-root `plugin.yaml`
- `targets/<platform>/...`
- your real sources such as `cmd/`, `scripts/`, `mcp/`, `skills/`, `agents/`, `contexts/`

Claude, Codex package/runtime lanes, Gemini native config files, OpenCode workspace config, and Cursor workspace config are rendered managed artifacts.
They are not the authored source of truth.

That means:

- `render` produces native target files from `plugin.yaml` plus `targets/<platform>/...`
- `validate` checks the authored project plus generated-artifact drift
- `import` is the bridge from current native Claude/Codex/Gemini/OpenCode/Cursor layouts back into the package-standard authored layout
- `normalize` rewrites `plugin.yaml` into canonical package-standard shape and removes unknown fields

`plugin-kit-ai validate` checks package-standard projects, generated-artifact drift, manifest warnings, and Claude authored-hook routing consistency against `launcher.yaml.entrypoint`.
`plugin-kit-ai capabilities` now defaults to target/package introspection. Use `--mode runtime` for Claude/Codex event support, and use the default target view for package class, production boundary, and managed-artifact coverage.
Generated Claude/Codex package-runtime config shapes are part of the repo-owned contract surface and are guarded by `render --check` plus deterministic `polyglot-smoke` canaries. Claude authored hook routing consistency with `launcher.yaml.entrypoint` is enforced separately by `validate --strict`.

In practice, that gives the repo one clear split:

- authored files are what humans edit
- rendered files are what target CLIs consume

That split is the core idea behind the tool.

## Runtime And Target Boundary

Current runtime support:

- Claude: production-ready within the declared stable event set `Stop`, `PreToolUse`, `UserPromptSubmit`
- Claude scaffolds only that stable subset by default; use `--claude-extended-hooks` only for the wider runtime-supported set
- Claude package authoring also supports first-class `targets/claude/settings.json`, `targets/claude/lsp.json`, `targets/claude/user-config.json`, and `targets/claude/manifest.extra.json`
- Claude: runtime-supported but not stable for `SessionStart`, `SessionEnd`, `Notification`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `SubagentStart`, `SubagentStop`, `PreCompact`, `Setup`, `TeammateIdle`, `TaskCompleted`, `ConfigChange`, `WorktreeCreate`, `WorktreeRemove`
- Codex runtime: production-ready within the declared stable `Notify` path
- Codex package: production-ready official plugin package lane
- Gemini: full packaging-only extension lane through `render|import|validate` plus local `extensions link|config|disable|enable`, not a production-ready runtime target
- OpenCode: workspace-config-only lane through `render|import|validate`, including JSON/JSONC plus explicit user-scope and env-config import compatibility, beta first-class standalone tools, stable local JS/TS plugin subtree/shared package-metadata support, permission-first passthrough config semantics with deprecated tools-config compatibility, and beta `custom_tools`, but still not a launcher/runtime target
- Cursor: workspace-config-only lane through `render|import|validate`, including `.cursor/mcp.json`, project-root `.cursor/rules/**`, optional shared root `AGENTS.md`, and `.cursorrules` compatibility import, but not `CLAUDE.md`, global `~/.cursor/mcp.json`, nested non-root rule directories, or a VS Code extension packaging lane

Release boundary notes:

- Claude stable support covers the declared stable event set only
- Codex runtime stable support does not guarantee the health of the external `codex exec` runtime before hook execution
- additional official Claude hooks may be runtime-supported in `public-beta` before separate promotion
- the canonical production plugin lane is `normalize -> render -> render --check -> validate --strict -> plugin-kit-ai test`
- deterministic canaries protect generated Claude/Codex config wiring and rendered runtime artifact drift; external CLI health stays outside that repo-owned guarantee

Executable runtime boundary:

| Runtime | Status | Supported shape | Runtime requirement and bootstrap |
|---------|--------|-----------------|---------------------------------|
| `go` | stable | default typed SDK path | Go `1.22+` to build; downstream plugin users run the compiled binary directly without a separately installed language runtime |
| `python` | stable local-runtime subset | repo-local executable ABI on `codex-runtime` and `claude` | Python `3.10+`; lockfile-first manager detection; `venv`/`requirements`/`uv` use repo-local `.venv`, `poetry`/`pipenv` can use manager-owned envs |
| `node` | stable local-runtime subset | repo-local executable ABI on `codex-runtime` and `claude` | system Node.js `20+`; JavaScript by default, TypeScript via `--runtime node --typescript` |
| `shell` | public-beta | repo-local executable ABI | POSIX shell on Unix, `bash` required on Windows |

Node/TypeScript and Python are the stable repo-local interpreted subset for scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, and bounded portable export bundles on `codex-runtime` and `claude`.
They are supported paths, but they are not zero-runtime-dependency paths: the target machine still needs Python or Node installed.
Generated Python and Node scaffolds now include an official helper layer so plugin authors implement handlers instead of manually parsing launcher argv/stdin.
Shell remains `public-beta` and stays outside that stable local-runtime subset.
For interpreted runtimes, `validate --strict` is the canonical CI-grade readiness gate, and its runtime lookup order is expected to stay aligned with the generated launcher.
For generated Python and Node projects, `plugin-kit-ai doctor <path>` is the read-only readiness check, `plugin-kit-ai bootstrap <path>` is the supported first-run helper before `validate --strict`, `plugin-kit-ai test <path> --platform <target> --event <event>` is the stable fixture-driven smoke command for the declared stable Claude and Codex runtime events, and `plugin-kit-ai export <path> --platform <target>` is the stable portable handoff surface for that subset.
Generated launcher-based Claude and Codex runtime projects now pre-seed `fixtures/<platform>/<event>.json` and `goldens/<platform>/<event>.*` during `plugin-kit-ai init`, so the authored scaffold is immediately usable with both `plugin-kit-ai test` and `plugin-kit-ai dev`.
`plugin-kit-ai test` loads JSON fixtures from `fixtures/<platform>/<event>.json` by default, invokes the configured launcher entrypoint with the correct carrier, compares `stdout`, `stderr`, and `exitcode` against `goldens/<platform>/<event>.*`, emits a machine-readable summary plus mismatch previews, supports `--all` to run every stable event for the selected platform, and supports `--update-golden` to record the current outputs.
For config-first `codex-package`, `gemini`, `opencode`, and `cursor` scaffolds, `plugin-kit-ai init --extras` now also pre-seeds `mcp/servers.yaml`, so the first authored portable MCP path is ready before the first `render`.
`plugin-kit-ai bundle install <bundle.tar.gz> --dest <path>` is the stable local bundle installer for exported Python/Node handoff bundles. It accepts only local `.tar.gz` archives, unpacks into `--dest`, and does not run `bootstrap` or `validate` for you.
`plugin-kit-ai bundle fetch` is the stable remote handoff companion for exported Python/Node bundles. URL mode verifies `--sha256` or `<url>.sha256`; GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`. It remains separate from both stable local `bundle install` and binary-only `install`.
`plugin-kit-ai bundle publish <path> --platform <target> --repo <owner/repo> --tag <tag>` is the stable producer-side companion for exported Python/Node bundles. It runs the same export contract, creates a published release by default, supports `--draft` as an opt-in safety mode, uploads the bundle plus a sibling `.sha256` asset, and remains separate from both stable local `bundle install` and binary-only `install`.
`plugin-kit-ai install` remains binary-only; marketplace packaging, dependency-preinstalled installs, and a universal package-management contract stay out of scope in this cycle.
The recommended package-manager install path for the `plugin-kit-ai` CLI itself is `brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai`.
The official JavaScript ecosystem path is `npm i -g plugin-kit-ai` or `npx plugin-kit-ai@latest ...`. This npm wrapper stays `public-beta`, downloads the matching published GitHub Releases binary, verifies `checksums.txt`, and does not widen `plugin-kit-ai install`.
When the PyPI wrapper has been published for a release, the Python ecosystem path is `pipx install plugin-kit-ai` or `pipx run plugin-kit-ai version`. This PyPI wrapper stays `public-beta`, downloads the matching published GitHub Releases binary, verifies `checksums.txt`, and does not widen `plugin-kit-ai install`.
For plugin authoring helpers rather than CLI installation, the shared package path is `plugin-kit-ai-runtime` on PyPI and npm. Those packages mirror the scaffold helper API, while Go remains the recommended path when you want the most self-contained delivery model. See [docs/CHOOSING_HELPER_DELIVERY_MODE.md](docs/CHOOSING_HELPER_DELIVERY_MODE.md) for the supported `vendored helper` vs `shared runtime package` tradeoff.
The verified fallback path is `scripts/install.sh`: it resolves the latest published stable release by default, verifies `checksums.txt`, auto-detects OS/arch, and installs the correct GitHub Releases tarball into your chosen `BIN_DIR`.
The official CI setup path for the CLI itself is `777genius/plugin-kit-ai/setup-plugin-kit-ai@v1`, which reuses the same verified release contract instead of rebuilding the CLI from source in every downstream workflow.
For stable interpreted `python`/`node` projects on `codex-runtime` and `claude`, `plugin-kit-ai init --extras` now emits `.github/workflows/bundle-release.yml`, an opt-in GitHub Actions workflow that runs `doctor -> bootstrap -> validate --strict -> bundle publish` through the official setup action.

## What The Community Should Expect

The project is intentionally opinionated.

- Go is the recommended authoring path when you want the most self-contained and least fragile operational story
- package-standard authoring is the source of truth; hand-editing rendered target files is not the intended workflow
- Node/TypeScript and Python form the stable repo-local interpreted subset for the community-first local-runtime path
- Shell is still supported because teams use it, but it remains a repo-local beta path
- Gemini is in scope as a full extension-packaging target, not as a production-ready runtime target
- OpenCode is in scope as a workspace-config target, not as a first-class local JS/TS plugin-code runtime lane
- Cursor is in scope as a workspace-config target, not as a VS Code extension packaging lane or a full umbrella for every documented Cursor surface

That means the promise is practical rather than inflated:

- strong support for Go plugin repos
- credible repo-local polyglot support
- explicit boundaries where stability, packaging, or external runtime management is not promised yet

## SDK

Root package `plugin-kit-ai` is composition/runtime only. Platform APIs live in peer public packages:

- `github.com/777genius/plugin-kit-ai/sdk`
- `github.com/777genius/plugin-kit-ai/sdk/claude`
- `github.com/777genius/plugin-kit-ai/sdk/codex`

Claude example:

```go
package main

import (
	"os"

	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"
	"github.com/777genius/plugin-kit-ai/sdk/claude"
)

func main() {
	app := pluginkitai.New(pluginkitai.Config{Name: "claude-demo"})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response {
		return claude.Allow()
	})
	os.Exit(app.Run())
}
```

Codex example:

```go
package main

import (
	"os"

	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"
	"github.com/777genius/plugin-kit-ai/sdk/codex"
)

func main() {
	app := pluginkitai.New(pluginkitai.Config{Name: "codex-demo"})
	app.Codex().OnNotify(func(*codex.NotifyEvent) *codex.Response {
		return codex.Continue()
	})
	os.Exit(app.Run())
}
```

SDK references:

- [sdk/README.md](sdk/README.md)
- [docs/generated/support_matrix.md](docs/generated/support_matrix.md)
- [docs/SUPPORT.md](docs/SUPPORT.md)

## CLI

Common commands:

```bash
./bin/plugin-kit-ai init my-plugin
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime python
./bin/plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript
./bin/plugin-kit-ai doctor ./my-plugin
./bin/plugin-kit-ai bootstrap ./my-plugin
./bin/plugin-kit-ai dev ./my-plugin --platform claude --event Stop
./bin/plugin-kit-ai test ./my-plugin --platform codex-runtime --event Notify --update-golden
./bin/plugin-kit-ai test ./my-plugin --platform claude --all
./bin/plugin-kit-ai bundle install ./bundle.tar.gz --dest ./plugin-copy
./bin/plugin-kit-ai init my-plugin --platform claude --runtime shell
./bin/plugin-kit-ai render ./my-plugin
./bin/plugin-kit-ai render ./my-plugin --check
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai inspect ./my-plugin
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
./bin/plugin-kit-ai capabilities --format json
./bin/plugin-kit-ai capabilities --mode runtime --format json --platform claude
./bin/plugin-kit-ai install owner/repo --tag v1.0.0 --goos linux --goarch amd64
```

`plugin-kit-ai install` is intentionally narrow:

- installs third-party plugin binaries from GitHub Releases
- verifies `checksums.txt`
- prints a deterministic success summary
- does not provide self-update for the `plugin-kit-ai` CLI

## Build And Test

Requirements:

- Go `1.23.x` for this monorepo workspace and its CI lanes
- generated Go plugin projects created by `plugin-kit-ai init` remain on Go `1.22+`

Common commands from repo root:

```bash
go run ./cmd/plugin-kit-ai-gen
go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai
./bin/plugin-kit-ai version
make test-polyglot-smoke

go test ./sdk/...
go test ./cli/plugin-kit-ai/...
go test ./install/plugininstall/...
go test ./repotests -run TestPluginKitAIInitGeneratesBuildableModule -count=1
go test ./...
```

Canonical community release flow for the stable Python/Node subset:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript --extras
git tag v1.0.0
# generated .github/workflows/bundle-release.yml runs bundle publish
plugin-kit-ai bundle fetch owner/repo --tag v1.0.0 --platform codex-runtime --runtime node --dest ./handoff-plugin
```

## Repository Layout

- `sdk`: SDK runtime, public platform packages, descriptor generator
- `cli/plugin-kit-ai`: CLI authoring and validation flow
- `install/plugininstall`: installer subsystem
- `examples/starters`: copy-first Go/Python/Node starter repos
- `examples/local`: repo-local executable-runtime reference examples
- `examples/plugins`: reference production repos
- `repotests`: integration and guard tests
- `docs`: support policy, production workflow, release policy, and generated contract docs

## Documentation Map

Use these as the canonical contract docs:

- [docs/generated/support_matrix.md](docs/generated/support_matrix.md): event-level runtime support
- [docs/generated/target_support_matrix.md](docs/generated/target_support_matrix.md): target/package contract
- [docs/SUPPORT.md](docs/SUPPORT.md): compatibility and public-surface policy
- [docs/PRODUCTION.md](docs/PRODUCTION.md): production authoring workflow
- [docs/EXECUTABLE_ABI.md](docs/EXECUTABLE_ABI.md): low-level executable ABI
- [docs/INSTALL_COMPATIBILITY.md](docs/INSTALL_COMPATIBILITY.md): installer release layout contract
- [docs/STATUS.md](docs/STATUS.md): shipped status and current hardening state
- [docs/RELEASE.md](docs/RELEASE.md): maintainer release flow

Fast starter repos:

- [examples/starters/README.md](examples/starters/README.md)

Reference repos:

- [examples/local/README.md](examples/local/README.md)
- [examples/plugins/README.md](examples/plugins/README.md)

Maintainer-only historical context:

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/FOUNDATION_REWRITE_VNEXT.md](docs/FOUNDATION_REWRITE_VNEXT.md)
- [docs/adr/README.md](docs/adr/README.md)
