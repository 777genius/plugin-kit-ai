# plugin-kit-ai

Build your plugin once and easily export it to any AI agent, like Claude, Codex, or Gemini, without duplicating code.

`plugin-kit-ai` helps you create, validate, and maintain a single plugin repo that can later cover supported outputs for Codex, Claude, Gemini, and other targets. Build it once, keep one workflow, start from `init` or a starter repo, pick Go, Node/TypeScript, or Python for the first path, and use `validate --strict` before handoff or CI.

Common use cases:

- start one repo and keep expanding it to more supported outputs over time
- build a Codex runtime plugin on the strongest production path first
- add Claude hooks, Gemini packaging, or workspace/config outputs later from that same repo

Docs site:

- overview: [plugin-kit-ai documentation](https://777genius.github.io/plugin-kit-ai/en/)
- fastest start: [Quickstart](https://777genius.github.io/plugin-kit-ai/en/guide/quickstart.html)
- one repo, many outputs: [What You Can Build](https://777genius.github.io/plugin-kit-ai/en/guide/what-you-can-build.html)
- honest caveat: [Support Boundary](https://777genius.github.io/plugin-kit-ai/en/reference/support-boundary.html)

## What To Know Right Away

- one repo and one workflow can cover many supported outputs
- support depth depends on the target you add
- runtime plugins, package outputs, and workspace-managed config do not all behave the same way
- the honest promise is `one repo / many supported outputs`, not fake parity everywhere

## Use It When

- you want a real plugin repo instead of one-off scripts and manually edited config files
- you want a clear first path for Go, Node/TypeScript, or Python
- you want a repeatable validation step before another person, machine, or CI uses the repo
- you may later need Claude, bundles, package outputs, or workspace-managed config from the same repo

## Skip It When

- you only need a tiny throwaway local script
- you want universal dependency management for every interpreted runtime ecosystem
- you need every target and every hook family to have the same production promise today

## Quick Start

Default install path:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

Create the strongest default repo first:

```bash
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai render .
plugin-kit-ai validate . --platform codex-runtime --strict
```

That gives you:

- one plugin repo from day one
- the strongest default path today: `codex-runtime` with `--runtime go`
- the cleanest base for later expansion into more supported outputs

Need another install channel:

- npm: `npm i -g plugin-kit-ai` or `npx plugin-kit-ai@latest ...`
- pipx (`public-beta`, only when that release is published to PyPI): `pipx install plugin-kit-ai`
- fallback installer: `curl -fsSL https://raw.githubusercontent.com/777genius/plugin-kit-ai/main/scripts/install.sh | sh`
- source build for maintainers of this repo: `go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai`

## Choose Your First Path

Pick the first path by stack and delivery goal. Do not start by learning the full target taxonomy.

| If you want | First path |
|---------|----------|
| the strongest production path | `plugin-kit-ai init my-plugin` |
| a TypeScript-first repo | `plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript` |
| a Python-first repo | `plugin-kit-ai init my-plugin --platform codex-runtime --runtime python` |

Practical default:

- choose Go when you want the cleanest production story
- choose Node/TypeScript when your team already ships TypeScript
- choose Python when the plugin stays repo-local and your team is Python-first
- choose Claude first only when Claude hooks are already the real product requirement

## Expand Later From The Same Repo

- add Claude hooks when hooks become part of the product
- add Gemini, Codex package, OpenCode, or Cursor outputs when packaging or workspace integration becomes necessary
- keep one repo and one validation workflow while the product grows
- check support depth before you promise the same thing everywhere

## What Else It Supports

- a typed Go SDK for Claude, Codex, and Gemini
- stable repo-local Python and Node paths on `codex-runtime` and `claude`
- portable bundle handoff for supported Python and Node plugin repos
- starter templates for Codex and Claude across Go, Python, and Node/TypeScript
- package and workspace-config paths for Codex package, Gemini, OpenCode, and Cursor

## Keep This Rule In Mind

- start with one strong path
- keep the repo and validation workflow unified
- add supported outputs only when the product really needs them
- check support depth before you promise the same thing everywhere

## Deep Product Details

Everything below this point is for people comparing delivery models and detailed support boundaries. If you only needed the main promise and first path, you can stop above.

## Go Deeper By Goal

### Fast Local Plugin

Choose this when the plugin stays local to the repo and your team already works in Python or Node.

- Main flow: `init -> doctor -> bootstrap -> render -> validate --strict`
- Runtime note: the execution machine still needs Python `3.10+` or Node.js `20+`
- Delivery options: vendored helper by default, shared `plugin-kit-ai-runtime` when you want a reusable dependency, bundle handoff when the repo must travel

Start here:

- [examples/starters/README.md](examples/starters/README.md)
- [examples/local/README.md](examples/local/README.md)
- [docs/CHOOSING_HELPER_DELIVERY_MODE.md](docs/CHOOSING_HELPER_DELIVERY_MODE.md)

### Production-Ready Plugin Repo

Choose this when you want the strongest supported release and distribution story.

- Best default: `plugin-kit-ai init my-plugin`
- Claude-first path: `plugin-kit-ai init my-plugin --platform claude`
- Package/config expansion later: `codex-package`, `gemini`, `opencode`, `cursor`

### Already Have Native Config

Choose this when you are migrating existing Claude/Codex/Gemini/OpenCode/Cursor native files into the repo-owned workflow.

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai render ./my-plugin
./bin/plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
```

## Stability Snapshot

Stable by default:

- the main public CLI contract
- the recommended Go SDK path
- Go scaffolds for the default Codex and Claude runtime lanes
- the stable local Python and Node subset on `codex-runtime` and `claude`
- `doctor`, `bootstrap`, `validate --strict`, `export`, and bundle handoff for that stable local subset

Use carefully:

- `render`, `import`, and `normalize` are still `public-beta`
- package and workspace-config targets have different guarantees than runtime targets
- `shell` remains a bounded `public-beta` escape hatch

For the precise contract:

- [docs/generated/target_support_matrix.md](docs/generated/target_support_matrix.md)
- [docs/generated/support_matrix.md](docs/generated/support_matrix.md)
- [docs/SUPPORT.md](docs/SUPPORT.md)

## Path Summary

- Go is the recommended path when you want the strongest production story and the least downstream runtime friction.
- Node/TypeScript is the main supported non-Go path for repo-local runtime plugins.
- Python is the supported Python-first repo-local path.
- Package and workspace-config targets are for packaging and configuration outputs, not for pretending every target behaves like a runtime plugin.

## SDK And CLI

Go SDK packages:

- `github.com/777genius/plugin-kit-ai/sdk`
- `github.com/777genius/plugin-kit-ai/sdk/claude`
- `github.com/777genius/plugin-kit-ai/sdk/codex`
- `github.com/777genius/plugin-kit-ai/sdk/gemini`

Useful starting points:

- [sdk/README.md](sdk/README.md)
- [docs/generated/support_matrix.md](docs/generated/support_matrix.md)
- [docs/SUPPORT.md](docs/SUPPORT.md)

Common CLI commands:

```bash
./bin/plugin-kit-ai init my-plugin
./bin/plugin-kit-ai doctor ./my-plugin
./bin/plugin-kit-ai bootstrap ./my-plugin
./bin/plugin-kit-ai render ./my-plugin
./bin/plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai capabilities --format json
```

`plugin-kit-ai install` stays intentionally narrow: it installs third-party plugin binaries from GitHub Releases, verifies `checksums.txt`, and does not act as a self-update path for the CLI itself.

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
go test ./...
```

## Repository And Docs Map

Main repo areas:

- `sdk`
- `cli/plugin-kit-ai`
- `install/plugininstall`
- `examples/starters`
- `examples/local`
- `examples/plugins`
- `repotests`
- `docs`

Canonical docs:

- [docs/generated/support_matrix.md](docs/generated/support_matrix.md)
- [docs/generated/target_support_matrix.md](docs/generated/target_support_matrix.md)
- [docs/SUPPORT.md](docs/SUPPORT.md)
- [docs/PRODUCTION.md](docs/PRODUCTION.md)
- [docs/INSTALL_COMPATIBILITY.md](docs/INSTALL_COMPATIBILITY.md)
- [docs/STATUS.md](docs/STATUS.md)

Maintainer-only historical context:

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/FOUNDATION_REWRITE_VNEXT.md](docs/FOUNDATION_REWRITE_VNEXT.md)
- [docs/adr/README.md](docs/adr/README.md)
