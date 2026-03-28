# plugin-kit-ai

AI CLI plugin runtime with a first-class Go SDK.

`plugin-kit-ai` is a Go-first system for building plugins for AI coding CLIs.
It gives you:

- a typed Go SDK for Claude and Codex
- a package-standard authoring model for plugin repos
- managed render/validate tooling for Claude, Codex, and Gemini target artifacts
- a repo-local executable ABI for `python`, `node`, and `shell`

Use it when you want one of these outcomes:

- build a real plugin repo for Claude or Codex with a clear support boundary
- keep authored plugin state in versioned source files instead of hand-editing vendor config
- generate and validate native target files deterministically
- stay Go-first by default, but still allow repo-local plugins in Python, Node, or Shell

Do not use it if your main goal is:

- marketplace-style packaged distribution for Python or Node plugins
- dependency installation or runtime management for interpreted languages
- a fully stable runtime contract for every Claude hook or every target

## Who It Is For

`plugin-kit-ai` is aimed at three audiences:

- plugin authors who want a typed Go SDK and a production path for Claude or Codex
- teams that already have native Claude/Codex/Gemini config files and want to move to a managed source-of-truth model
- maintainers who need render, drift detection, strict validation, and deterministic release gates

If you are a solo hacker trying to wire a tiny local script into a CLI, this may still help, but the main value is stronger repo structure and clearer contracts.

## What Is Stable

Stable in `v1.0.0`:

- Go-first SDK authoring for the approved Claude and Codex event set
- CLI commands `init`, `validate`, `capabilities`, `inspect`, `install`, `version`
- Go-first scaffold contract for Claude and Codex

Currently `public-beta`:

- `render`, `import`, and `normalize`
- Gemini extension packaging target through `render|import`
- executable runtime scaffolds for `python`, `node`, and `shell`
- optional scaffold extras from `plugin-kit-ai init --extras`

Currently `public-experimental`:

- `plugin-kit-ai skills`
- any surface not explicitly promoted through the audit ledger

## Quick Start

Build the CLI:

```bash
go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai
```

Choose the path that matches your goal:

### Fast Local Plugin

For repo-local plugins where quick iteration matters more than packaged distribution:

- Good fit: Python or Node teams wiring a local Claude/Codex plugin into an existing repo
- Guarantee level: supported repo-local executable path with `validate --strict` as the readiness gate
- Main non-goals: managed dependency installation, packaged distribution, and runtime parity with the Go SDK

```bash
./bin/plugin-kit-ai init my-plugin --runtime python
./bin/plugin-kit-ai init my-plugin --runtime node
```

Reference repos: [examples/local/README.md](examples/local/README.md)

### Production-Ready Plugin Repo

For teams that want the strongest supported release and distribution story:

- Good fit: new plugin repos that want the clearest stable contract and typed handlers
- Guarantee level: strongest supported path in the current contract
- Main non-goals: interpreted-runtime packaging and dependency management

```bash
./bin/plugin-kit-ai init my-plugin
./bin/plugin-kit-ai init my-plugin --platform claude
./bin/plugin-kit-ai init my-plugin --platform claude --claude-extended-hooks
./bin/plugin-kit-ai init my-plugin --platform gemini
```

### Already Have Native Config

For teams migrating existing Claude/Codex/Gemini native files into the package-standard authored layout:

- Good fit: existing plugin repos that want one managed source of truth
- Guarantee level: import bridge into the authored package-standard model
- Main non-goals: preserving native files as the long-term authored source of truth

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex
```

Run the canonical authoring lane:

```bash
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai render ./my-plugin
./bin/plugin-kit-ai render ./my-plugin --check
./bin/plugin-kit-ai validate ./my-plugin --platform codex --strict
```

## Which Path Should You Choose

Choose Go if:

- you want the strongest supported path
- you want typed handlers and the cleanest production story
- you want the least bootstrap friction for other users of the repo

Choose `python`, `node`, or `shell` if:

- the plugin is repo-local
- your team already works in that runtime
- you are comfortable owning the runtime bootstrap yourself

The default recommendation remains:

- Go for production-ready plugin repos
- Python/Node/Shell for repo-local integration where language fit matters more than ecosystem packaging

## Project Model

`plugin-kit-ai` has one canonical authored project shape:

- repo-root `plugin.yaml`
- `targets/<platform>/...`
- your real sources such as `cmd/`, `scripts/`, `mcp/`, `skills/`, `agents/`, `contexts/`

Claude, Codex, and Gemini native config files are rendered managed artifacts.
They are not the authored source of truth.

That means:

- `render` produces native target files from `plugin.yaml` plus `targets/<platform>/...`
- `validate` checks the authored project plus generated-artifact drift
- `import` is the bridge from current native Claude/Codex/Gemini layouts back into the package-standard authored layout
- `normalize` rewrites `plugin.yaml` into canonical package-standard shape and removes unknown fields

`plugin-kit-ai validate` checks package-standard projects, generated-artifact drift, manifest warnings, and Claude authored-hook routing consistency against `plugin.yaml.entrypoint`.
`plugin-kit-ai capabilities` now defaults to target/package introspection. Use `--mode runtime` for Claude/Codex event support, and use the default target view for package class, production boundary, and managed-artifact coverage.
Generated Claude/Codex config shapes are part of the repo-owned contract surface and are guarded by `render --check` plus deterministic `polyglot-smoke` canaries. Claude authored hook routing consistency with `plugin.yaml.entrypoint` is enforced separately by `validate --strict`.

In practice, that gives the repo one clear split:

- authored files are what humans edit
- rendered files are what target CLIs consume

That split is the core idea behind the tool.

## Runtime And Target Boundary

Current runtime support:

- Claude: production-ready within the declared stable event set `Stop`, `PreToolUse`, `UserPromptSubmit`
- Claude scaffolds only that stable subset by default; use `--claude-extended-hooks` only for the wider runtime-supported set
- Claude: runtime-supported but not stable for `SessionStart`, `SessionEnd`, `Notification`, `PostToolUse`, `PostToolUseFailure`, `PermissionRequest`, `SubagentStart`, `SubagentStop`, `PreCompact`, `Setup`, `TeammateIdle`, `TaskCompleted`, `ConfigChange`, `WorktreeCreate`, `WorktreeRemove`
- Codex: production-ready within the declared stable `Notify` path
- Gemini: packaging-only beta target through `render|import`, not a production-ready runtime target

Release boundary notes:

- Claude stable support covers the declared stable event set only
- Codex stable support does not guarantee the health of the external `codex exec` runtime before hook execution
- additional official Claude hooks may be runtime-supported in `public-beta` before separate promotion
- the canonical production plugin lane is `normalize -> render -> render --check -> validate --strict -> target smoke`
- deterministic canaries protect generated Claude/Codex config wiring and rendered runtime artifact drift; external CLI health stays outside that repo-owned guarantee

Executable runtime boundary:

| Runtime | Status | Supported shape | Bootstrap contract |
|---------|--------|-----------------|--------------------|
| `go` | stable | default typed SDK path | Go `1.22+`, direct executable |
| `python` | public-beta | repo-local executable ABI | prefer `.venv`, fallback to system Python `3.10+` |
| `node` | public-beta | repo-local executable ABI | system Node.js `20+`, JS-first runtime |
| `shell` | public-beta | repo-local executable ABI | POSIX shell on Unix, `bash` required on Windows |

Interpreted runtimes are supported for scaffold, validate, launcher execution, and repo-local bootstrap only.
For interpreted runtimes, `validate --strict` is the canonical CI-grade readiness gate, and its runtime lookup order is expected to stay aligned with the generated launcher.
They are not covered by `plugin-kit-ai install`, dependency installation, or packaged distribution in this cycle.

## What The Community Should Expect

The project is intentionally opinionated.

- Go is the best-supported authoring path, not just one option among equals
- package-standard authoring is the source of truth; hand-editing rendered target files is not the intended workflow
- Python, Node, and Shell are supported because teams use them, but they are still a repo-local beta path
- Gemini is in scope as a packaging target, not as a production-ready runtime target

That means the promise is practical rather than inflated:

- strong support for Go-first plugin repos
- credible repo-local polyglot support
- explicit boundaries where stability or packaging is not promised yet

## SDK

Root package `plugin-kit-ai` is composition/runtime only. Platform APIs live in peer public packages:

- `github.com/plugin-kit-ai/plugin-kit-ai/sdk`
- `github.com/plugin-kit-ai/plugin-kit-ai/sdk/claude`
- `github.com/plugin-kit-ai/plugin-kit-ai/sdk/codex`

Claude example:

```go
package main

import (
	"os"

	pluginkitai "github.com/plugin-kit-ai/plugin-kit-ai/sdk"
	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/claude"
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

	pluginkitai "github.com/plugin-kit-ai/plugin-kit-ai/sdk"
	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/codex"
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

- [sdk/plugin-kit-ai/README.md](sdk/plugin-kit-ai/README.md)
- [docs/generated/support_matrix.md](docs/generated/support_matrix.md)
- [docs/SUPPORT.md](docs/SUPPORT.md)

## CLI

Common commands:

```bash
./bin/plugin-kit-ai init my-plugin
./bin/plugin-kit-ai init my-plugin --runtime python
./bin/plugin-kit-ai init my-plugin --platform claude --runtime shell
./bin/plugin-kit-ai render ./my-plugin
./bin/plugin-kit-ai render ./my-plugin --check
./bin/plugin-kit-ai import ./native-plugin --from codex
./bin/plugin-kit-ai inspect ./my-plugin
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai validate ./my-plugin --platform codex --strict
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

- Go `1.22+`

Common commands from repo root:

```bash
go run ./cmd/plugin-kit-ai-gen
go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai
./bin/plugin-kit-ai version
make test-polyglot-smoke

go test ./sdk/plugin-kit-ai/...
go test ./cli/plugin-kit-ai/...
go test ./install/plugininstall/...
go test ./repotests -run TestPluginKitAIInitGeneratesBuildableModule -count=1
go test ./...
```

## Repository Layout

- `sdk/plugin-kit-ai`: SDK runtime, public platform packages, descriptor generator
- `cli/plugin-kit-ai`: CLI authoring and validation flow
- `install/plugininstall`: installer subsystem
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

Reference repos:

- [examples/local/README.md](examples/local/README.md)
- [examples/plugins/README.md](examples/plugins/README.md)

Maintainer-only historical context:

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/FOUNDATION_REWRITE_VNEXT.md](docs/FOUNDATION_REWRITE_VNEXT.md)
- [docs/adr/README.md](docs/adr/README.md)
