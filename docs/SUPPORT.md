# Support And Compatibility Policy

This document defines the approved public contract for `plugin-kit-ai` after the `v1.0.0` release.

## Contract Levels

- `public-stable`: backward-compatible within a major release. Removal requires deprecation first.
- `public-beta`: supported and documented, but may change before promotion. Breaking changes require changelog notes.
- `public-experimental`: opt-in surface with no compatibility promise.
- `internal`: not part of the public contract.

## Current Contract

The current source tree is split between `public-stable`, `public-beta`, and `public-experimental`.

The declared `v1` candidate set was reviewed through [V0_9_AUDIT.md](./V0_9_AUDIT.md). Post-`v1.0.0` community-first interpreted promotion is reviewed through [INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md). Anything not listed remains `public-beta` or `internal`.

Canonical event-level support claims live in [generated/support_matrix.md](./generated/support_matrix.md). That table is the source of truth for:

- platform and event names
- runtime support status
- maturity
- `v1` target flag
- invocation and carrier shape
- scaffold and validate support
- transport modes
- capability tags
- live-test profile labels

The generated support matrix is runtime-event-only. Runtime-supported Gemini beta hooks and the Claude/Codex runtime lanes appear there; packaging-only or workspace-config-only targets such as OpenCode and Cursor are documented in this policy and in CLI docs.
The target/package contract matrix lives in [generated/target_support_matrix.md](./generated/target_support_matrix.md). That table is the source of truth for target class, production class, import/render/validate support, portable component kinds, target-native component kinds, and managed artifact sets.

## Contract Vocabulary

Use these terms consistently in public docs, generated artifacts, and CLI output:

- `production-ready`: runtime path covered by the current stable promise
- `public-stable`: compatibility tier for promoted public surfaces
- `public-beta`: supported but not yet covered by the stable promise
- `public-experimental`: opt-in surface with no compatibility promise
- `runtime-supported but not stable`: implemented runtime path that still remains `public-beta`
- `packaging-only target`: target with manifest/render/import support but without any runtime contract

## Current Public-Stable

SDK packages and stable root API:

- `github.com/777genius/plugin-kit-ai/sdk`
- `github.com/777genius/plugin-kit-ai/sdk/claude`
- `github.com/777genius/plugin-kit-ai/sdk/codex`
- `plugin-kit-ai.New`, `plugin-kit-ai.Config`, `plugin-kit-ai.App`
- `(*plugin-kit-ai.App).Use`
- `(*plugin-kit-ai.App).Claude`
- `(*plugin-kit-ai.App).Codex`
- `(*plugin-kit-ai.App).Run`
- `(*plugin-kit-ai.App).RunContext`
- `plugin-kit-ai.Supported`

Stable event surfaces:

- Claude:
  - `Stop`
  - `PreToolUse`
  - `UserPromptSubmit`
- Codex:
  - `Notify`

Current production-ready target boundary:

- Claude: production-ready within the stable `Stop`, `PreToolUse`, and `UserPromptSubmit` event set
- Claude package authoring also supports first-class `targets/claude/settings.json`, `targets/claude/lsp.json`, `targets/claude/user-config.json`, and `targets/claude/manifest.extra.json`
- Codex runtime: production-ready within the stable `Notify` path
- Codex package: production-ready official plugin package lane
- Gemini: full Gemini CLI extension packaging lane through `plugin-kit-ai render|import|validate` and local `extensions link|config|disable|enable`, plus a `public-beta` Go runtime lane for `SessionStart`, `SessionEnd`, `BeforeTool`, and `AfterTool` with dedicated opt-in real CLI runtime smoke; still not production-ready
- OpenCode: workspace-config lane through `plugin-kit-ai render|import|validate`, `opencode.json.plugin`, inline `mcp`, validated mirrored `.opencode/skills/`, first-class `.opencode/{commands,agents,themes,tools}/`, stable `.opencode/plugins/` plus `.opencode/package.json`, and JSON/JSONC plus explicit user-scope and env-config import compatibility; not a production-ready runtime target

Stable CLI commands:

- `plugin-kit-ai init`
- `plugin-kit-ai bootstrap` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`
- `plugin-kit-ai doctor` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`
- `plugin-kit-ai export` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`
- `plugin-kit-ai bundle install` for local exported Python/Node bundles on `codex-runtime` and `claude`
- `plugin-kit-ai bundle fetch` for remote exported Python/Node bundles on `codex-runtime` and `claude`
- `plugin-kit-ai bundle publish` for GitHub Releases handoff of exported Python/Node bundles on `codex-runtime` and `claude`
- `plugin-kit-ai validate`
- `plugin-kit-ai capabilities`
- `plugin-kit-ai inspect`
- `plugin-kit-ai install`
- `plugin-kit-ai version`

Stable CLI bootstrap/setup path for `plugin-kit-ai` itself:

- `brew install 777genius/homebrew-plugin-kit-ai/plugin-kit-ai` is the recommended package-manager install path for the CLI itself
- `npm i -g plugin-kit-ai` or `npx plugin-kit-ai@latest ...` is the official JavaScript ecosystem path for the CLI itself; this wrapper stays `public-beta`, downloads the matching published GitHub Releases binary, and verifies `checksums.txt`
- when the PyPI wrapper is published for a release, `pipx install plugin-kit-ai` or `pipx run plugin-kit-ai version` is the Python ecosystem path for the CLI itself; this wrapper stays `public-beta`, downloads the matching published GitHub Releases binary, and verifies `checksums.txt`
- for Python/Node plugin authoring helpers, the shared package path is `plugin-kit-ai-runtime` on PyPI and npm; this mirrors the scaffold helper API and stays separate from the CLI wrappers above
- helper delivery modes are documented in [CHOOSING_HELPER_DELIVERY_MODE.md](./CHOOSING_HELPER_DELIVERY_MODE.md); the default scaffold vendors helper files, while `init ... --runtime-package` switches to the shared dependency path; released CLIs pin the helper version automatically and development builds accept `--runtime-package-version`
- `scripts/install.sh` resolves the latest published stable release by default, verifies `checksums.txt`, auto-detects OS/arch, and installs the matching GitHub Releases tarball into `BIN_DIR`
- `777genius/plugin-kit-ai/setup-plugin-kit-ai@v1` is the official CI setup action and reuses the same verified release contract instead of rebuilding from source in downstream repos

Current beta CLI commands:

- `plugin-kit-ai bootstrap` for launcher-based `shell` projects
- `plugin-kit-ai doctor` for launcher-based `shell` projects
- `plugin-kit-ai export` for launcher-based `shell` projects

Stable `plugin-kit-ai install` contract:

- installs third-party plugin binaries from GitHub Releases only
- requires `checksums.txt` in the selected release for verified installation
- supports `--tag` or `--latest` selection, but not both together
- writes a deterministic success summary with installed path, release ref/source, selected asset, and target GOOS/GOARCH
- permits overwrite only for existing files when `--force` is set
- does not include self-update or auto-update behavior for the `plugin-kit-ai` CLI itself

The release-layout compatibility boundary for `plugin-kit-ai install` is documented separately in [INSTALL_COMPATIBILITY.md](./INSTALL_COMPATIBILITY.md).

Stable generated scaffold contract:

- Codex runtime required authored files: `go.mod`, `README.md`, `plugin.yaml`, `launcher.yaml`, generated `cmd/<project>/main.go`
- Codex package required authored files: `README.md`, `plugin.yaml`, `targets/codex-package/package.yaml`
- Codex runtime optional authored docs: `targets/codex-runtime/config.extra.toml` for supported repo-local config passthrough beyond managed `model` and `notify`
- Codex package optional authored docs: `targets/codex-package/interface.json`, `targets/codex-package/app.json`, and `targets/codex-package/manifest.extra.json`; first-class package metadata lives in `targets/codex-package/package.yaml`
- Claude required authored files: `go.mod`, `README.md`, `plugin.yaml`, generated `cmd/<project>/main.go`
- stable launcher-based local-runtime scaffold subset on `codex-runtime` and `claude`:
  - `python`: `plugin.yaml`, `launcher.yaml`, `README.md`, launcher under `bin/`, plus supported manager manifests; default helper delivery vendors `src/plugin_runtime.py`, while `init ... --runtime-package` imports `plugin_kit_ai_runtime`; official shared helper package: `plugin-kit-ai-runtime`
  - `node`: `plugin.yaml`, `launcher.yaml`, `README.md`, launcher under `bin/`, plus supported manager manifests; default helper delivery vendors `src/plugin-runtime.{mjs,ts}`, while `init ... --runtime-package` imports `plugin-kit-ai-runtime`; TypeScript is the stable authoring mode via `--runtime node --typescript`; official shared helper package: `plugin-kit-ai-runtime`
  - `init --extras` for the stable interpreted `python`/`node` subset also emits `.github/workflows/bundle-release.yml`, an opt-in GitHub Actions workflow that uses `setup-plugin-kit-ai@v1` and runs `doctor -> bootstrap -> validate --strict -> bundle publish`
- native vendor files generated from `plugin.yaml` remain part of the scaffolded project contract

Runtime recommendation contract:

- Go is the recommended default when users want typed handlers, the strongest supported authoring path, and the least downstream runtime friction
- Go plugins normally ship as compiled binaries, so plugin users do not need a separately installed Python or Node runtime just to execute them
- Python and Node are stable supported authoring lanes for the repo-local interpreted subset on `codex-runtime` and `claude`
- Python and Node projects must make their external runtime requirement explicit to users up front:
  - Python plugins require Python `3.10+` on the machine running the plugin
  - Node plugins require Node.js `20+` on the machine running the plugin
- vendored helper files and shared runtime packages are both supported delivery modes for the same Python/Node helper API

## Current Public-Beta Surfaces

Current beta surfaces that remain intentionally outside the stable set:

- Gemini full Gemini CLI extension packaging lane through `plugin-kit-ai render|import|validate`, covering official-style `gemini-extension.json`, inline `mcpServers`, target-native contexts, settings, themes, commands, hooks, policies, `manifest.extra.json`, and deterministic local extension lifecycle checks
- OpenCode workspace-config lane through `plugin-kit-ai render|import|validate`, covering official-style `opencode.json` and `opencode.jsonc`, package refs, inline `mcp`, validated portable skills mirrored into `.opencode/skills/`, first-class workspace commands/agents/themes, first-class beta standalone tools mirrored into `.opencode/tools/`, stable official-style local JS/TS plugin code mirrored into `.opencode/plugins/`, stable shared dependency metadata mirrored into `.opencode/package.json` for tools and plugins, explicit `--include-user-scope` import from `~/.config/opencode`, env-config import compatibility from `OPENCODE_CONFIG` and `OPENCODE_CONFIG_DIR`, `config.extra.json`, and passthrough config surfaces like `agent`, `permission`, and `instructions`; `custom_tools` remain beta across standalone tools and plugin code
- Cursor workspace-config lane through `plugin-kit-ai render|import|validate`, covering `.cursor/mcp.json`, project-root `.cursor/rules/**`, optional shared root `AGENTS.md`, and strict documented-subset behavior that defers root `CLAUDE.md`, global `~/.cursor/mcp.json`, nested non-root `.cursor/rules/**`, and JSONC; not a production-ready runtime target
- optional extras generated by `plugin-kit-ai init --extras`
- `plugin-kit-ai init --platform claude --claude-extended-hooks` for the wider runtime-supported Claude hook scaffold beyond the stable default subset
- `plugin-kit-ai render`, `plugin-kit-ai import`, and `plugin-kit-ai normalize`
- launcher-based `shell` runtime authoring on `codex-runtime` and `claude`, including `init --runtime shell`, `bootstrap`, `doctor`, `validate --strict`, and `export`
- experimental `plugin-kit-ai skills` authoring/render subsystem and generated skill artifacts
- Claude official runtime-supported hooks not yet promoted to `public-stable`:
  - `SessionStart`
  - `SessionEnd`
  - `Notification`
  - `PostToolUse`
  - `PostToolUseFailure`
  - `PermissionRequest`
  - `SubagentStart`
  - `SubagentStop`
  - `PreCompact`
  - `Setup`
  - `TeammateIdle`
  - `TaskCompleted`
  - `ConfigChange`
  - `WorktreeCreate`
  - `WorktreeRemove`
- any newly added surfaces after the first stable set, until separately reviewed and promoted
- experimental local typed Claude hook registration helpers in `sdk/claude`
- experimental local typed Codex hook registration helper in `sdk/codex`

Config contract:

- repo-root `plugin.yaml` is the canonical authoring manifest for supported plugin projects
- the package-standard `plugin.yaml` schema is intentionally limited to package/build intent; unknown keys warn in `plugin-kit-ai validate`
- `plugin-kit-ai normalize` is the canonical cleanup path for rewriting unknown manifest content into the package-standard shape
- `plugin-kit-ai import` is the supported bridge from current native Claude/Codex/Gemini/OpenCode layouts back into the authored package-standard layout
- Codex runtime project-local config generated by `plugin-kit-ai render` or `plugin-kit-ai init --platform codex-runtime`
- Codex runtime passthrough config lives in `targets/codex-runtime/config.extra.toml`; managed `model` and `notify` stay owned by `launcher.yaml` plus `targets/codex-runtime/package.yaml`
- Codex package manifest generated by `plugin-kit-ai render` or `plugin-kit-ai init --platform codex-package`; first-class package metadata and `interface` live under `targets/codex-package/`, while `manifest.extra.json` remains passthrough-only for unsupported future fields
- Claude plugin metadata and hook routing files generated by `plugin-kit-ai render` or `plugin-kit-ai init --platform claude`
- Gemini CLI extension manifest generated by `plugin-kit-ai render --target gemini`, with optional Go launcher-based runtime support in the current beta contract
- OpenCode workspace config generated by `plugin-kit-ai render --target opencode`, with workspace-config-only status in the current contract
- Cursor workspace config generated by `plugin-kit-ai render --target cursor`, with workspace-config-only status in the current contract
- OpenCode local plugin loading stable subset is guarded by `render --check`, strict validation, the production example canary, and the documented `test-opencode-live` smoke path
- OpenCode standalone tools beta subset is guarded by `render --check`, strict validation, the production example canary, and the documented `test-opencode-tools-live` smoke path
- package-standard authored projects are defined by root `plugin.yaml` plus `targets/<platform>/...`
- rendered native target files remain managed artifacts, not authored source-of-truth files
- generated Claude/Codex config wiring is a repo-owned contract surface guarded by `render --check`, deterministic generated-project canaries, and the `polyglot-smoke` lane
- Claude authored hook routing must stay aligned with `launcher.yaml.entrypoint`; `validate --strict` is the enforcing gate for that consistency
- executable-runtime hardening currently includes generated launcher smoke for `go`, `python`, `node`, and `shell`, plus Windows `.cmd` validation coverage and ABI passthrough e2e
- stable local-runtime interpreted subset:
  - targets: `codex-runtime`, `claude`
  - runtimes: `python`, `node`
  - stable scope is scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, bounded portable export bundles, local exported bundle install, remote bundle fetch, and GitHub Releases bundle publish
  - `python`: Python `3.10+`; lockfile-first manager detection; `venv`, `requirements.txt`, and `uv` use repo-local `.venv`, while `poetry` and `pipenv` can validate against manager-owned envs
  - `node`: system Node.js `20+`; lockfile-first manager detection for `bun`, `pnpm`, `yarn`, or `npm`; JavaScript by default, TypeScript via `--runtime node --typescript`
  - operational tradeoff: interpreted runtimes are supported, but they are not zero-runtime-dependency delivery modes; the target machine still needs the appropriate external runtime installed
- beta local-runtime remainder:
  - `shell`: POSIX shell on Unix, `bash` required on Windows
  - supported scope is scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, and bounded portable export bundles
  - unsupported scope is universal package-management policy and packaged distribution through `plugin-kit-ai install`
- stable local bundle-install subset:
  - `bundle install` accepts only local `.tar.gz` bundles created by `plugin-kit-ai export`
  - supported subset: exported `python` and `node` bundles for `codex-runtime` and `claude`
  - unsupported scope: `shell`, remote URLs, registries, GitHub Releases, and implicit `bootstrap` or `validate`
- stable remote bundle-fetch subset:
  - `bundle fetch` supports direct HTTPS bundle URLs and GitHub Releases bundle discovery
  - URL mode verifies `--sha256` or `<url>.sha256`
  - GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`
  - supported subset: exported `python` and `node` bundles for `codex-runtime` and `claude`
  - unsupported scope: `shell`, registries, generic authenticated HTTPS distribution, and implicit `bootstrap` or `validate`
- stable GitHub bundle-publish subset:
  - `bundle publish` exports the same `python`/`node` bundle contract and uploads it to GitHub Releases
  - creates a published release by default; `--draft` keeps the target release as draft
  - uploaded assets are `<asset>.tar.gz` plus `<asset>.sha256`
  - supported subset: exported `python` and `node` bundles for `codex-runtime` and `claude`
  - unsupported scope: `shell`, registries, package-manager publishing, and generic HTTPS publishing
- community-first downstream setup path:
  - local recommended install path uses Homebrew
  - local JS ecosystem install path uses `npm i -g plugin-kit-ai` as `public-beta`
  - local Python ecosystem install path uses `pipx install plugin-kit-ai` as `public-beta` when that release was published to PyPI
  - local CLI bootstrap uses `scripts/install.sh`
  - CI bootstrap uses `777genius/plugin-kit-ai/setup-plugin-kit-ai@v1`
  - root GitHub Release assets come from `.github/workflows/release-assets.yml` and are the source of truth for downstream CLI channels
  - Homebrew tap updates follow successful `Release Assets` completion or a manual tag-scoped rerun and remain separate from `plugin-kit-ai install`
  - npm publishes follow successful `Release Assets` completion or a manual tag-scoped rerun and remain separate from `plugin-kit-ai install`
  - PyPI publishes follow successful `Release Assets` completion when trusted publishing is enabled, or a manual tag-scoped rerun when maintainers need that channel, and remain separate from `plugin-kit-ai install`
  - this setup path is separate from binary-only `plugin-kit-ai install`

Declared release review:

- production plugin authoring guide: [PRODUCTION.md](./PRODUCTION.md)
- stable-candidate ledger: [V0_9_AUDIT.md](./V0_9_AUDIT.md)
- post-`v1` interpreted stable-subset ledger: [INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md)
- release playbook: [RELEASE.md](./RELEASE.md)
- release notes template: [RELEASE_NOTES_TEMPLATE.md](./RELEASE_NOTES_TEMPLATE.md)
- rehearsal worksheet: [REHEARSAL_TEMPLATE.md](./REHEARSAL_TEMPLATE.md)

## Internal Surfaces

These areas are not supported as public dependencies:

- `sdk/internal/...`
- `cli/plugin-kit-ai/internal/...`
- `install/plugininstall/internal/...`
- `install/plugininstall/adapters/...`
- `install/plugininstall/domain/...`
- `install/plugininstall/ports/...`
- generator implementation details and generated package internals

## Current Public-Experimental Surfaces

- `plugin-kit-ai skills init`
- `plugin-kit-ai skills validate`
- `plugin-kit-ai skills render`
- canonical authored skill format under `skills/<name>/SKILL.md`
- generated Claude/Codex skill artifacts under `generated/skills/...`
- local typed Claude hook registration helpers:
  - `claude.RegisterCustomCommonJSON`
  - `claude.RegisterCustomContextJSON`
  - `claude.RegisterCustomPostToolUseJSON`
  - `claude.RegisterCustomPermissionRequestJSON`
- local typed Codex hook registration helper:
  - `codex.RegisterCustomJSON`

The skills subsystem is a compatibility-first authoring layer: `SKILL.md` remains the source of truth, execution remains language-neutral, and generated artifacts are derived outputs. None of this surface is covered by the stable compatibility promise yet.
Handwritten `SKILL.md` is supported; `plugin-kit-ai skills init` is convenience scaffold only, not a required authoring path.
See [SKILLS.md](./SKILLS.md) for usage guidance, examples, and when not to use it.

The custom hook helpers are intended as an escape hatch when Claude or Codex add hooks before `plugin-kit-ai` ships first-class support. They preserve typed handlers, but are not covered by the stable compatibility promise.

## Compatibility Rules

- `public-beta` changes must be called out in changelogs or release notes when user code, scaffold output, readiness semantics, or bundle contents change.
- `public-beta` surfaces are not covered by a backward-compatibility promise; before promotion, older beta-only paths may be removed directly as long as the current contract and resulting breakage are documented.
- The declared `v1` candidate set must be reviewed through [V0_9_AUDIT.md](./V0_9_AUDIT.md) before any surface is promoted.
- post-`v1` stable-promotion candidates must be reviewed through a dedicated promotion ledger such as [INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md)
- OpenCode local plugin loading is promoted through [OPENCODE_STABLE_PROMOTION_AUDIT.md](./OPENCODE_STABLE_PROMOTION_AUDIT.md); helper-based custom tools remain `public-beta`
- OpenCode standalone tools beta evidence is tracked through [OPENCODE_TOOLS_BETA_AUDIT.md](./OPENCODE_TOOLS_BETA_AUDIT.md); `custom_tools` remain `public-beta`
- `public-stable` defines the post-`v1.0` compatibility promise for the approved set.
- No surface is promoted to `public-stable` until it has descriptor-backed docs, scaffold/validate alignment, and test coverage across unit, integration, contract, and smoke layers.
- Unified cross-platform abstractions are out of scope for the `v1` public contract unless they are explicitly declared later.

## Target Stable Boundary For The `v1` Candidate Set

This section defines what promotion means once a candidate surface moves from `public-beta` to `public-stable`.

SDK and CLI stable promotion means:

- no breaking changes outside a future major release
- removal only through deprecation first
- documented replacement path required for future replacements
- support docs and generated support metadata must match shipped behavior

Codex stable promotion means:

- stable registration API for `OnNotify`
- stable invocation mapping to `Notify`
- stable decode semantics for valid notify payload input
- stable response behavior
- stable scaffold and validate support for Codex plugin layout

Codex stable promotion does **not** mean:

- availability or health of local Codex installation
- success of Codex transport/auth/network/session startup
- absence of Codex runtime panics before hook firing
- stability of Codex internal logs or retry wording

Claude stable promotion means:

- stable registration APIs for `Stop`, `PreToolUse`, and `UserPromptSubmit`
- stable decode and response semantics for those events
- stable scaffold and validate support for Claude plugin layout

Claude runtime-supported beta expansion currently includes:

- `SessionStart`
- `SessionEnd`
- `Notification`
- `PostToolUse`
- `PostToolUseFailure`
- `PermissionRequest`
- `SubagentStart`
- `SubagentStop`
- `PreCompact`
- `Setup`
- `TeammateIdle`
- `TaskCompleted`
- `ConfigChange`
- `WorktreeCreate`
- `WorktreeRemove`

OpenCode stable promotion means:

- stable repo-local authored/render/import/validate contract for `targets/opencode/plugins/**`
- stable repo-local authored/render/import/validate contract for `targets/opencode/package.json`
- stable dependency-free official-style named async plugin scaffold/example shape
- stable explicit user-scope import normalization for project-local and `--include-user-scope` OpenCode plugin tree/package metadata
- stable deterministic loader smoke evidence through the documented `TestOpenCodeLoaderSmoke` path

OpenCode stable promotion does **not** mean:

- guaranteed availability of a local `opencode` binary
- guaranteed success of external OpenCode startup/auth/provider/network health before plugin load
- stable support for every possible helper-based custom tool implementation
- stable support for every possible standalone `.opencode/tools/**` behavior or helper pattern

Stable diagnostics boundary is limited to:

- runtime failure families
- validate failure kinds
- install exit-code families
- declared high-signal phrasing in `DIAGNOSTICS.md`

Release evidence note:

- Codex real smoke passed in the latest release-evidence refresh.
- Claude real smoke passed in the latest release-evidence refresh.
- Live install checks passed in the latest release-evidence refresh.
- Final release execution records the candidate SHA and tag in git history.

## Deprecation Rules

- Deprecated public surfaces must be marked in docs and changelogs before removal.
- Removal requires a documented replacement path.
- Deprecated `public-beta` surface may still change before `v1`, but removal should not be silent.
