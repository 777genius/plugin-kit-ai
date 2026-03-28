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

The generated support matrix is runtime-event-only. Packaging-only or workspace-config-only targets such as Gemini and OpenCode are documented in this policy and in CLI docs, but are intentionally absent from the runtime event matrix.
The target/package contract matrix lives in [generated/target_support_matrix.md](./generated/target_support_matrix.md). That table is the source of truth for target class, production class, import/render/validate support, portable component kinds, target-native component kinds, and managed artifact sets.

## Contract Vocabulary

Use these terms consistently in public docs, generated artifacts, and CLI output:

- `production-ready`: runtime path covered by the current stable promise
- `public-stable`: compatibility tier for promoted public surfaces
- `public-beta`: supported but not yet covered by the stable promise
- `public-experimental`: opt-in surface with no compatibility promise
- `runtime-supported but not stable`: implemented runtime path that still remains `public-beta`
- `packaging-only target`: target with manifest/render/import support but without a production-ready runtime contract

## Current Public-Stable

SDK packages and stable root API:

- `github.com/plugin-kit-ai/plugin-kit-ai/sdk`
- `github.com/plugin-kit-ai/plugin-kit-ai/sdk/claude`
- `github.com/plugin-kit-ai/plugin-kit-ai/sdk/codex`
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
- Gemini: full Gemini CLI extension packaging lane through `plugin-kit-ai render|import|validate` and local `extensions link|config|disable|enable`; not a production-ready runtime target
- OpenCode: workspace-config lane through `plugin-kit-ai render|import|validate`, `opencode.json.plugin`, inline `mcp`, validated mirrored `.opencode/skills/`, first-class `.opencode/{commands,agents,themes}/`, and JSON/JSONC plus explicit user-scope import compatibility; not a production-ready runtime target

Stable CLI commands:

- `plugin-kit-ai init`
- `plugin-kit-ai bootstrap` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`
- `plugin-kit-ai doctor` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`
- `plugin-kit-ai export` for `python` and `node` launcher-based projects on `codex-runtime` and `claude`
- `plugin-kit-ai bundle install` for local exported Python/Node bundles on `codex-runtime` and `claude`
- `plugin-kit-ai validate`
- `plugin-kit-ai capabilities`
- `plugin-kit-ai inspect`
- `plugin-kit-ai install`
- `plugin-kit-ai version`

Current beta CLI commands:

- `plugin-kit-ai bootstrap` for launcher-based `shell` projects
- `plugin-kit-ai doctor` for launcher-based `shell` projects
- `plugin-kit-ai export` for launcher-based `shell` projects
- `plugin-kit-ai bundle fetch` for remote exported Python/Node bundles
- `plugin-kit-ai bundle publish` for GitHub Releases handoff of exported Python/Node bundles

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
- Claude required authored files: `go.mod`, `README.md`, `plugin.yaml`, generated `cmd/<project>/main.go`
- stable launcher-based local-runtime scaffold subset on `codex-runtime` and `claude`:
  - `python`: `plugin.yaml`, `launcher.yaml`, `README.md`, launcher under `bin/`, runtime sources, plus supported manager manifests
  - `node`: `plugin.yaml`, `launcher.yaml`, `README.md`, launcher under `bin/`, runtime sources, plus supported manager manifests; TypeScript is the stable authoring mode via `--runtime node --typescript`
- native vendor files generated from `plugin.yaml` remain part of the scaffolded project contract

## Current Public-Beta Surfaces

Current beta surfaces that remain intentionally outside the stable set:

- Gemini full Gemini CLI extension packaging lane through `plugin-kit-ai render|import|validate`, covering official-style `gemini-extension.json`, inline `mcpServers`, target-native contexts, settings, themes, commands, hooks, policies, `manifest.extra.json`, and deterministic local extension lifecycle checks
- OpenCode workspace-config lane through `plugin-kit-ai render|import|validate`, covering official-style `opencode.json` and `opencode.jsonc`, package refs, inline `mcp`, validated portable skills mirrored into `.opencode/skills/`, first-class workspace commands/agents/themes, compatibility import from `.claude/skills` and `.agents/skills`, explicit `--include-user-scope` import from `~/.config/opencode`, `config.extra.json`, passthrough config surfaces like `agent`, `permission`, `instructions`, and `tools`, and explicit warnings for unsupported local JS/TS plugin code
- optional extras generated by `plugin-kit-ai init --extras`
- `plugin-kit-ai init --platform claude --claude-extended-hooks` for the wider runtime-supported Claude hook scaffold beyond the stable default subset
- `plugin-kit-ai render`, `plugin-kit-ai import`, and `plugin-kit-ai normalize`
- launcher-based `shell` runtime authoring on `codex-runtime` and `claude`, including `init --runtime shell`, `bootstrap`, `doctor`, `validate --strict`, and `export`
- `plugin-kit-ai bundle fetch` for remote exported Python/Node bundles via direct HTTPS URLs or GitHub Releases
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
- Codex package manifest generated by `plugin-kit-ai render` or `plugin-kit-ai init --platform codex-package`
- Claude plugin metadata and hook routing files generated by `plugin-kit-ai render` or `plugin-kit-ai init --platform claude`
- Gemini CLI extension manifest generated by `plugin-kit-ai render --target gemini`, with packaging-only status in the current contract
- OpenCode workspace config generated by `plugin-kit-ai render --target opencode`, with workspace-config-only status in the current contract
- package-standard authored projects are defined by root `plugin.yaml` plus `targets/<platform>/...`
- rendered native target files remain managed artifacts, not authored source-of-truth files
- generated Claude/Codex config wiring is a repo-owned contract surface guarded by `render --check`, deterministic generated-project canaries, and the `polyglot-smoke` lane
- Claude authored hook routing must stay aligned with `launcher.yaml.entrypoint`; `validate --strict` is the enforcing gate for that consistency
- executable-runtime hardening currently includes generated launcher smoke for `go`, `python`, `node`, and `shell`, plus Windows `.cmd` validation coverage and ABI passthrough e2e
- stable local-runtime interpreted subset:
  - targets: `codex-runtime`, `claude`
  - runtimes: `python`, `node`
  - stable scope is scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, bounded portable export bundles, and local exported bundle install
  - `python`: lockfile-first manager detection; `venv`, `requirements.txt`, and `uv` use repo-local `.venv`, while `poetry` and `pipenv` can validate against manager-owned envs
  - `node`: system Node.js `20+`; lockfile-first manager detection for `bun`, `pnpm`, `yarn`, or `npm`; JavaScript by default, TypeScript via `--runtime node --typescript`
- beta local-runtime remainder:
  - `shell`: POSIX shell on Unix, `bash` required on Windows
  - supported scope is scaffold, validate, launcher execution, repo-local bootstrap, read-only doctor checks, and bounded portable export bundles
  - unsupported scope is universal package-management policy and packaged distribution through `plugin-kit-ai install`
- stable local bundle-install subset:
  - `bundle install` accepts only local `.tar.gz` bundles created by `plugin-kit-ai export`
  - supported subset: exported `python` and `node` bundles for `codex-runtime` and `claude`
  - unsupported scope: `shell`, remote URLs, registries, GitHub Releases, and implicit `bootstrap` or `validate`
- beta remote bundle-fetch subset:
  - `bundle fetch` supports direct HTTPS bundle URLs and GitHub Releases bundle discovery
  - URL mode verifies `--sha256` or `<url>.sha256`
  - GitHub Releases mode prefers `checksums.txt` and falls back to `<asset>.sha256`
  - supported subset: exported `python` and `node` bundles for `codex-runtime` and `claude`
  - unsupported scope: `shell`, registries, generic authenticated HTTPS distribution, and implicit `bootstrap` or `validate`
- beta GitHub bundle-publish subset:
  - `bundle publish` exports the same `python`/`node` bundle contract and uploads it to GitHub Releases
  - creates a published release by default; `--draft` keeps the target release as draft
  - uploaded assets are `<asset>.tar.gz` plus `<asset>.sha256`
  - supported subset: exported `python` and `node` bundles for `codex-runtime` and `claude`
  - unsupported scope: `shell`, registries, package-manager publishing, generic HTTPS publishing, and promotion of `bundle fetch` to stable

Declared release review:

- production plugin authoring guide: [PRODUCTION.md](./PRODUCTION.md)
- stable-candidate ledger: [V0_9_AUDIT.md](./V0_9_AUDIT.md)
- post-`v1` interpreted stable-subset ledger: [INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md)
- release playbook: [RELEASE.md](./RELEASE.md)
- release notes template: [RELEASE_NOTES_TEMPLATE.md](./RELEASE_NOTES_TEMPLATE.md)
- rehearsal worksheet: [REHEARSAL_TEMPLATE.md](./REHEARSAL_TEMPLATE.md)

## Internal Surfaces

These areas are not supported as public dependencies:

- `sdk/plugin-kit-ai/internal/...`
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
- `public-beta` surfaces are not covered by a backward-compatibility promise; before promotion, legacy paths may be removed directly as long as the current contract and resulting breakage are documented.
- The declared `v1` candidate set must be reviewed through [V0_9_AUDIT.md](./V0_9_AUDIT.md) before any surface is promoted.
- post-`v1` stable-promotion candidates must be reviewed through a dedicated promotion ledger such as [INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md)
- `public-stable` defines the post-`v1.0` compatibility promise for the approved set.
- No surface is promoted to `public-stable` until it has descriptor-backed docs, scaffold/validate alignment, and test coverage across unit, integration, contract, and smoke layers.
- Unified cross-platform abstractions are out of scope for the `v1` public contract unless they are explicitly declared later.

## Target Stable Boundary For The `v1` Candidate Set

This section defines what promotion means once a candidate surface moves from `public-beta` to `public-stable`.

SDK and CLI stable promotion means:

- no breaking changes outside a future major release
- removal only through deprecation first
- migration path required for future replacements
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
- Removal requires a documented migration path.
- Deprecated `public-beta` surface may still change before `v1`, but removal should not be silent.
