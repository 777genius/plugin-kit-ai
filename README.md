# plugin-kit-ai

Build one plugin repo, start with Go by default, and later add packaging, Claude hooks, Gemini, or repo-owned integration setup without splitting the project.

`plugin-kit-ai` keeps your source in one repo, gives you one clear starting path, and still leaves room for Node, Python, packages, and integrations when you need them.

Common use cases:

- start with one plugin repo and keep expanding it as the product grows
- ship the strongest default path first with Codex runtime on Go
- keep Node/TypeScript and Python visible as supported non-Go paths when the team already works there
- add Codex package, Claude, Gemini, or repo-owned integration setup later from the same source of truth

Docs site:

- overview: [plugin-kit-ai documentation](https://777genius.github.io/plugin-kit-ai/docs/en/)
- fastest start: [Quickstart](https://777genius.github.io/plugin-kit-ai/docs/en/guide/quickstart.html)
- product overview: [What You Can Build](https://777genius.github.io/plugin-kit-ai/docs/en/guide/what-you-can-build.html)
- delivery model guide: [Choose A Target](https://777genius.github.io/plugin-kit-ai/docs/en/guide/choose-a-target.html)
- exact support contract: [Support Boundary](https://777genius.github.io/plugin-kit-ai/docs/en/reference/support-boundary.html)

## Default Start

If you want the clearest production story today, start here:

- `codex-runtime` with Go for the strongest default starting path

## Supported Non-Go Paths

These are strong supported choices for teams already living in those stacks:

- `codex-runtime --runtime node --typescript`
- `codex-runtime --runtime python`

Tradeoff to keep visible:

- both are supported local interpreted runtime lanes
- the target machine still needs Node.js `20+` or Python `3.10+`
- Go remains the default when you want the cleanest runtime and release story

## If You Need More Later

When you need a different way to ship the plugin, add:

- `codex-package`
- `gemini`
- `claude`
- `opencode`
- `cursor`

## What To Know Right Away

- one repo stays the source of truth as you add more lanes
- choose the starting path that matches what you need today
- expand later from the same repo when the product needs more outputs
- use `generate` and `validate --strict` as the shared readiness workflow

## Quick Start

Recommended install path:

```bash
brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai
plugin-kit-ai version
```

Start on the strongest default path:

```bash
plugin-kit-ai init my-plugin
cd my-plugin
plugin-kit-ai generate .
plugin-kit-ai validate . --platform codex-runtime --strict
```

That gives you:

- one plugin repo from day one
- the strongest default runtime path today: `codex-runtime` with `--runtime go`
- the cleanest base for later expansion into packages, extensions, and integration setup
- canonical new repos that keep authored sources under `src/`

## What You Get

- one plugin repo that stays the source of truth
- authored files under `src/`
- generated Codex runtime output from the same repo
- a clean readiness check through `validate --strict`

Supported non-Go paths stay visible from the same starting point:

- Node/TypeScript: `plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript`
- Python: `plugin-kit-ai init my-plugin --platform codex-runtime --runtime python`
- both remain supported local runtime lanes, but they require Node.js `20+` or Python `3.10+` on the target machine

## What To Do Next

- edit the plugin under `src/`
- run `plugin-kit-ai generate .` again after changes
- run `plugin-kit-ai validate . --platform codex-runtime --strict` again
- only then add packaging, Claude, Gemini, or integration setup if the product needs it

Other supported CLI install methods:

- npm: `npm i -g plugin-kit-ai` or `npx plugin-kit-ai@latest ...`
- pipx: `pipx install plugin-kit-ai` when that release is published to PyPI
- verified script: `curl -fsSL https://raw.githubusercontent.com/777genius/plugin-kit-ai/main/scripts/install.sh | sh`
- source build for maintainers of this repo: `go build -o bin/plugin-kit-ai ./cli/plugin-kit-ai/cmd/plugin-kit-ai`

## Choose Your First Language

| If you want | Recommended start |
|---------|----------|
| the strongest runtime lane | `plugin-kit-ai init my-plugin` |
| a TypeScript-first runtime repo | `plugin-kit-ai init my-plugin --platform codex-runtime --runtime node --typescript` |
| a Python-first runtime repo | `plugin-kit-ai init my-plugin --platform codex-runtime --runtime python` |

Practical default:

- choose Go when you want the cleanest runtime and release story
- choose Node or Python when the repo stays local and your team already lives there

## Choose A Different Shipping Path Only When You Need It

| If you want | Recommended first lane |
|---------|----------|
| Claude hooks as the real product requirement | `plugin-kit-ai init my-plugin --platform claude` |
| an official Codex package | `plugin-kit-ai init my-plugin --platform codex-package` |
| a Gemini extension package | `plugin-kit-ai init my-plugin --platform gemini` |
| repo-owned integration/config outputs | `plugin-kit-ai init my-plugin --platform opencode` or `--platform cursor` |

## Expand Later From The Same Repo

- add Claude when hooks become part of the product
- add Codex package or Gemini when packaging becomes the real delivery lane
- add OpenCode or Cursor when the repo should manage integration config
- keep one repo and one validation workflow as the product grows

## What Else It Supports

- a typed Go SDK for Claude, Codex, and Gemini
- supported local Python and Node runtime authoring on `codex-runtime` and `claude`
- portable bundle handoff for supported Python and Node runtime repos
- starter templates for Codex and Claude across Go, Python, and Node/TypeScript
- package and repo-managed integration lanes for Codex package, Gemini, OpenCode, and Cursor

## Keep This Rule In Mind

- start with one recommended lane
- keep the authored repo unified
- add more lanes only when the product needs them
- use the support/reference docs when you need the exact contract details
## Deep Product Details

Everything below this point is for people comparing delivery models, import paths, and detailed support boundaries. If you only needed the main promise and first path, you can stop above.

## Go Deeper By Goal

### Fast Local Plugin

Choose this when the plugin stays local to the repo and your team already works in Python or Node.

- Main flow: `init -> doctor -> bootstrap -> generate -> validate --strict`
- Runtime note: the execution machine still needs Python `3.10+` or Node.js `20+`
- Delivery options: vendored helper by default, shared `plugin-kit-ai-runtime` when you want a reusable dependency, bundle handoff when the repo must travel

This is a supported non-Go path, not a hidden fallback.

Start here:

- [examples/starters/README.md](examples/starters/README.md)
- [examples/local/README.md](examples/local/README.md)
- [docs/CHOOSING_HELPER_DELIVERY_MODE.md](docs/CHOOSING_HELPER_DELIVERY_MODE.md)

### Production-Ready Plugin Repo

Choose this when you want the strongest supported release and distribution story.

- Best default: `plugin-kit-ai init my-plugin`
- Claude-first path: `plugin-kit-ai init my-plugin --platform claude`
- Package/config expansion later: `codex-package`, `gemini`, `opencode`, `cursor`
- Real multi-target MCP-first example: [`context7` in universal-plugins-for-ai-agents](https://github.com/777genius/universal-plugins-for-ai-agents/tree/main/plugins/context7)

### Already Have Native Config

Choose this when you are migrating existing Claude/Codex/Gemini/OpenCode/Cursor native files into the repo-owned workflow.

```bash
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai normalize ./my-plugin
./bin/plugin-kit-ai generate ./my-plugin
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

- `generate`, `import`, and `normalize` are still `public-beta`
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
./bin/plugin-kit-ai generate ./my-plugin
./bin/plugin-kit-ai validate ./my-plugin --platform codex-runtime --strict
./bin/plugin-kit-ai import ./native-plugin --from codex-runtime
./bin/plugin-kit-ai capabilities --format json
```

`plugin-kit-ai install` stays intentionally narrow: it installs third-party plugin binaries from GitHub Releases, verifies `checksums.txt`, and does not act as a self-update path for the CLI itself.

For automation, `plugin-kit-ai validate --format json` now emits the versioned `plugin-kit-ai/validate-report` contract with `schema_version: 1` and explicit outcomes `passed`, `failed`, or `failed_strict_warnings`.
For Codex lane selection, use [docs/CODEX_TARGET_BOUNDARY.md](docs/CODEX_TARGET_BOUNDARY.md). For the validation ABI itself, use [docs/VALIDATE_JSON_CONTRACT.md](docs/VALIDATE_JSON_CONTRACT.md).

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
- [docs/CODEX_TARGET_BOUNDARY.md](docs/CODEX_TARGET_BOUNDARY.md)
- [docs/VALIDATE_JSON_CONTRACT.md](docs/VALIDATE_JSON_CONTRACT.md)
- [docs/PRODUCTION.md](docs/PRODUCTION.md)
- [docs/INSTALL_COMPATIBILITY.md](docs/INSTALL_COMPATIBILITY.md)
- [docs/STATUS.md](docs/STATUS.md)

Maintainer-only historical context:

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/FOUNDATION_REWRITE_VNEXT.md](docs/FOUNDATION_REWRITE_VNEXT.md)
- [docs/adr/README.md](docs/adr/README.md)
