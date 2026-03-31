# Cursor Workspace Lane Plan

Implementation plan for adding a `cursor` target to `plugin-kit-ai` with a strict `MCP-first workspace lane + rules` scope.

**Plan date:** 2026-03-30

This document is a design and execution roadmap, not a product promise. It is grounded in the current repository architecture and the current public Cursor documentation surface.

## Executive Summary

Recommended `v1` scope:

- render/import/validate support for `.cursor/mcp.json`
- render/import/validate support for `.cursor/rules/**`
- explicit support for root `AGENTS.md` as a Cursor-compatible instruction surface
- optional compatibility import for legacy `.cursorrules`
- preserve Cursor `mcp.json` interpolation strings without semantic narrowing
- no attempt to implement a full VS Code extension packaging lane
- no attempt to model undocumented Cursor-native AI "plugin bundles" analogous to OpenCode npm plugin refs

Implementation defaults for `v1`:

- scaffold a default project rule under `targets/cursor/rules/project.mdc`
- keep `targets/cursor/AGENTS.md` optional and do not scaffold it by default
- treat standalone root `AGENTS.md` as insufficient for auto-detect import; use `.cursor/` markers or explicit `--from cursor`
- do not support `--include-user-scope` for Cursor in `v1`
- stay strict JSON-first for `.cursor/mcp.json` until JSONC is confirmed by official docs

Recommendation score:

- `Cursor MCP-first workspace lane + rules`: `Увер. 9/10`, `Надёж. 9/10`
- `Cursor MCP-only lane`: `Увер. 8/10`, `Надёж. 8/10`
- `Cursor IDE extension/plugin lane`: `Увер. 3/10`, `Надёж. 4/10`

## Why This Scope

Current documented Cursor surfaces split into two different layers:

- agent/runtime integration through `MCP`
- editor extensibility through VS Code extensions

For `plugin-kit-ai`, the correct first target is the first one.

That means the closest internal analogy is not `codex-runtime`, but a narrower `workspace_config_lane` similar in shape to `opencode`, with a much smaller authored surface.

## Non-Goals

Explicitly out of scope for `v1`:

- packaging or publishing VS Code extensions
- implementing `vscode.cursor.mcp.registerServer(...)` as an authored package target
- mirroring arbitrary Cursor editor settings
- modeling every Cursor instruction surface beyond `.cursor/rules/**` and root `AGENTS.md`
- supporting nested workspace-native rule directories like `backend/.cursor/rules/**` outside the project-root `.cursor/rules/**` lane
- owning or rendering root `CLAUDE.md` as a first-class managed artifact
- importing `~/.cursor/mcp.json` user-scope config in `v1`
- claiming a Cursor-native plugin package contract without a public documented spec

## Product Interpretation

For this repository, `cursor` should mean:

- a workspace-config target
- deterministic repo-local authored state
- managed render into Cursor-native files under `.cursor/`
- import back from native workspace files into package-standard authored layout
- strict validation of the supported stable subset

It should not mean:

- a general "Cursor integration umbrella"
- an IDE extension build system
- a speculative abstraction over all AI editors

## Proposed Target Contract

Add new target:

- id: `cursor`
- family: `code_plugin`
- target class: `workspace_config_lane`
- noun: `workspace`
- production class: `packaging-only target`
- install model: `workspace config files`
- dev model: `config authoring workspace`
- activation model: `config reload or restart`
- native root: `.cursor/mcp.json`

Recommended runtime contract wording:

- `workspace-config lane with first-class MCP config, project rules, and optional root AGENTS.md support`

Family rationale:

- even though Cursor is an IDE product, the current repository taxonomy uses `code_plugin` for repo-local config/plugin authoring lanes and `extension_package` for extension-style packaging lanes
- `workspace_config_lane` already exists under `code_plugin` for `opencode`
- introducing the first `ide_plugin` target here would create a new family branch without a clear contract-level benefit
- if we later add multiple Cursor-specific editor-extension/package targets, we can revisit `ide_plugin` with a real family boundary

Recommended portable component kinds:

- `mcp_servers`

Recommended target component kinds:

- `rules`
- `agents_md`
- `config_extra` only if we later find a stable documented need

For `v1`, do not force portable `skills` into Cursor. Cursor rules are not the same contract as our portable `SKILL.md`.

Recommended native docs:

- `agents_md` -> `targets/cursor/AGENTS.md` as `md`

Recommended managed artifacts:

- static: `AGENTS.md`
- portable MCP: `.cursor/mcp.json`
- mirrored rules: `.cursor/rules/**`

Additional documented Cursor surfaces that matter but are intentionally deferred:

- root `CLAUDE.md` is also read by Cursor CLI
- global `~/.cursor/mcp.json`
- nested `.cursor/rules` directories throughout the workspace

These should be treated as future compatibility opportunities, not as hidden unsupported behavior.

## Authored Package Shape

Recommended authored layout:

- `plugin.yaml`
- optional `mcp/servers.yaml`
- `targets/cursor/rules/project.mdc`
- optional additional `targets/cursor/rules/**/*.mdc`
- optional `targets/cursor/AGENTS.md`

Optional later:

- `targets/cursor/config.extra.json`

Rationale:

- `mcp/servers.yaml` is the canonical portable MCP authored source
- `.mdc` should stay target-native because Cursor rules are Cursor-specific
- root `AGENTS.md` should be authored under `targets/cursor/` because this target owns the render/import wiring for Cursor compatibility, not because `AGENTS.md` is Cursor-exclusive
- rendered root `AGENTS.md` may also affect other agent tools that read the same file, so it must remain optional and explicitly owned
- project-root `.cursor/rules/**` is the narrowest useful stable subset even though Cursor also supports nested `.cursor/rules` directories deeper in the workspace

## Managed Artifacts

Managed outputs for `v1`:

- `.cursor/mcp.json`
- `.cursor/rules/**`
- `AGENTS.md`

Notes:

- `AGENTS.md` should render to repo root, not under `.cursor/`
- root `AGENTS.md` should be treated as a managed artifact only for the `cursor` target
- `AGENTS.md` should only be rendered when `targets/cursor/AGENTS.md` exists
- if the repo already contains unrelated handwritten `AGENTS.md`, import must preserve content faithfully before render ownership is assumed
- docs must state clearly that rendered `AGENTS.md` is a shared instruction surface and may be consumed by tools beyond Cursor

## Mapping To Current Architecture

Primary code touchpoints:

- `sdk/platformmeta/platformmeta.go`
- `sdk/generator/generator.go`
- `cli/plugin-kit-ai/internal/platformexec/registry.go`
- new adapter file: `cli/plugin-kit-ai/internal/platformexec/cursor.go`
- `cli/plugin-kit-ai/internal/pluginmanifest/manifest.go`
- `cli/plugin-kit-ai/internal/targetcontracts/contracts.go`
- `cli/plugin-kit-ai/internal/app/init.go`
- `cli/plugin-kit-ai/internal/scaffold/platforms_gen.go`
- `cli/plugin-kit-ai/internal/validate/rules_gen.go`
- scaffold templates under `cli/plugin-kit-ai/internal/scaffold/templates/`
- CLI help text in `cli/plugin-kit-ai/cmd/plugin-kit-ai/{init,render,import}.go`
- target docs in `README.md`, `cli/plugin-kit-ai/README.md`, `docs/SUPPORT.md`, `docs/STATUS.md`
- generated docs and support matrix outputs
- integration coverage in `repotests/`

Likely implementation shape:

1. add `cursor` `PlatformProfile`
2. register `cursorAdapter{}`
3. add scaffold templates and generated scaffold/rules metadata
4. implement render
5. implement validate
6. implement import
7. wire CLI help text, docs, and support matrices
8. add example repo and tests

Implementation note:

- `discoverTarget(...)` already discovers directory-backed kinds from `profile.Contract.TargetComponentKinds` and file-backed kinds from `profile.NativeDocs`
- that means `rules` should stay a directory-backed component kind, while `agents_md` should be represented as a `NativeDocSpec` at `targets/cursor/AGENTS.md`

## DRY Strategy

Good DRY:

- reuse portable MCP authored source `mcp/servers.yaml`
- reuse `marshalJSON`, `decodeJSONObject`, `copyArtifacts`, `copyArtifactDirs`, `compactArtifacts`, `discoverFiles`
- reuse generic managed-artifact concepts already expressed in `platformmeta`
- reuse validation plumbing and diagnostics mapping
- reuse the existing "preserve portable MCP object without semantic narrowing" behavior already used by other targets

Bad DRY:

- inventing a generalized `workspace-config super adapter` before we have at least two truly similar targets
- forcing Cursor rules into the portable `skills` abstraction
- sharing import/render code with `opencode` just because both write into dot-directories

Recommended DRY boundary:

- share helper functions only where the native file contracts are actually the same
- keep `cursor.go` separate from `opencode.go`
- if helper extraction becomes obvious during implementation, extract only tiny focused helpers, not a framework

DRY recommendation score:

- `portable MCP reuse only`: `Увер. 9/10`, `Надёж. 9/10`
- `small helper extraction during implementation`: `Увер. 8/10`, `Надёж. 8/10`
- `preemptive unified opencode/cursor abstraction`: `Увер. 2/10`, `Надёж. 3/10`

## Detailed Implementation Phases

## Phase 0: Freeze Scope and Wording

Deliverables:

- final target naming: `cursor`
- final authored directories
- final stable-subset wording
- explicit `v1` non-goals in docs

Acceptance criteria:

- no ambiguity between Cursor MCP support and VS Code extension support
- no accidental promise of undocumented Cursor plugin packaging
- no ambiguity that root `AGENTS.md` is shared across tools and only opt-in for this target
- no ambiguity that nested non-root `.cursor/rules`, root `CLAUDE.md`, and global `~/.cursor/mcp.json` are deferred

## Phase 1: Platform Metadata

Add a new `PlatformProfile` in [platformmeta.go](/Users/belief/dev/projects/claude/hookplex/sdk/platformmeta/platformmeta.go).

Define:

- contract metadata
- surface tiers
- managed artifacts
- scaffold file list
- validate requirements

Recommended concrete `PlatformProfile` shape:

- `PlatformFamily`: `code_plugin`
- `TargetClass`: `workspace_config_lane`
- `TargetNoun`: `workspace`
- `ProductionClass`: `packaging-only target`
- `RuntimeContract`: `workspace-config lane with first-class MCP config, project rules, and optional root AGENTS.md support`
- `InstallModel`: `workspace config files`
- `DevModel`: `config authoring workspace`
- `ActivationModel`: `config reload or restart`
- `NativeRoot`: `.cursor/mcp.json`
- `PortableComponentKinds`: `mcp_servers`
- `TargetComponentKinds`: `rules`, `agents_md`
- `NativeDocs`: `agents_md -> targets/cursor/AGENTS.md`
- `ManagedArtifacts`: static `AGENTS.md`, portable MCP `.cursor/mcp.json`, mirror `targets/cursor/rules -> .cursor/rules`
- `Launcher.Requirement`: `ignored`
- `SDK.Status`: `scaffold_only`
- `SDK.LiveTestProfile`: `cursor_workspace`

Recommended surface tiers:

- `mcp`: `stable`
- `rules`: `stable`
- `agents_md`: `stable`
- anything else: `unsupported` or `passthrough_only`

Acceptance criteria:

- `platformmeta.Lookup("cursor")` works
- support matrix generation includes `cursor`
- wording clearly marks this as a workspace-config lane
- discovery sees `targets/cursor/AGENTS.md` as a file-backed native doc, not as a fake directory surface
- `pluginmanifest.DiscoveredTargetKinds(...)` reports `rules` and `agents_md` correctly when present

## Phase 2: Scaffold

Add templates:

- `cursor.README.md.tmpl`
- `cursor.AGENTS.md.tmpl`
- `cursor.rule.mdc.tmpl`

Recommended scaffold required files:

- `plugin.yaml`
- `README.md`
- `targets/cursor/rules/project.mdc`

Recommended scaffold optional files:

- `targets/cursor/AGENTS.md`
- `mcp/servers.yaml` should remain user-authored, not force-generated by scaffold when empty

Recommended scaffold behavior:

- `init --platform cursor` should behave like `gemini` and `opencode`: no runtime, no launcher, no `go.mod`
- default scaffold should produce a non-empty Cursor-native render via the default rule file
- `--extras` may include `targets/cursor/AGENTS.md`, but should not inject portable `skills/` because Cursor rules are target-native

Scaffold should not create:

- runtime launcher files
- VS Code extension manifest files
- arbitrary settings files

Acceptance criteria:

- `plugin-kit-ai init demo --platform cursor` produces a clean minimal target
- `validate --platform cursor --strict` passes on scaffold output
- scaffold output renders `.cursor/rules/project.mdc` deterministically on first init

## Phase 3: Render

Create `cursorAdapter.Render`.

Render behavior:

- optional `mcp/servers.yaml` -> `.cursor/mcp.json`
- `targets/cursor/rules/**` -> `.cursor/rules/**`
- `targets/cursor/AGENTS.md` -> `AGENTS.md`

Validation during render:

- `.cursor/mcp.json` must be generated from portable MCP only
- rules files must keep `.mdc` extension
- root `AGENTS.md` ownership must be explicit and deterministic
- Cursor variable interpolation strings like `${env:NAME}`, `${workspaceFolder}`, and `${input:token}` must be preserved exactly

Potential helper extraction:

- a tiny helper to render portable MCP into a target-specific native JSON path

Acceptance criteria:

- render is deterministic
- `render --check` detects drift correctly
- managed paths are reported correctly
- empty portable MCP does not force-create `.cursor/mcp.json`
- render never creates root `AGENTS.md` unless `targets/cursor/AGENTS.md` exists

## Phase 4: Import

Create `cursorAdapter.Import`.

Import sources for `v1`:

- `.cursor/mcp.json`
- `.cursor/rules/**`
- root `AGENTS.md`

Optional compatibility import:

- `.cursorrules` -> prefer normalizing into `targets/cursor/rules/legacy.mdc` with a warning; avoid mapping to `targets/cursor/AGENTS.md` unless we later have a documented reason
- root `CLAUDE.md` -> defer in `v1`; if added later, treat as compatibility import, not canonical authored state
- `~/.cursor/mcp.json` -> defer to a later `--include-user-scope` style import path if demand exists

Import rules:

- native MCP goes back into `mcp/servers.yaml`
- native rules go into `targets/cursor/rules/**`
- root `AGENTS.md` goes into `targets/cursor/AGENTS.md`
- preserve unknown structures with warnings instead of silently dropping them

Detect/import policy:

- `DetectNative(root)` should return true for `.cursor/mcp.json`, `.cursor/rules/**`, or legacy `.cursorrules`
- standalone root `AGENTS.md` must not auto-detect Cursor, because that would create false positives in repos using shared agent instructions outside Cursor
- explicit `plugin-kit-ai import . --from cursor` may import root `AGENTS.md` in addition to `.cursor` state
- `--include-user-scope` for Cursor should fail with a clear "not yet supported" error in `v1`

Acceptance criteria:

- import followed by render is stable
- warnings clearly explain any lossy normalization
- auto-detect import does not misclassify arbitrary repos that happen to contain only root `AGENTS.md`

## Phase 5: Validate

Create `cursorAdapter.Validate`.

Validation checks:

- `targets/cursor/rules/**` uses `.mdc`
- no path traversal
- no symlinks if we want parity with other config targets
- `targets/cursor/AGENTS.md` is valid markdown text and non-empty
- `mcp/servers.yaml` parses as a portable MCP envelope and renders successfully to `.cursor/mcp.json`
- case-folded path collision protection for rules tree
- MCP validation must not reject Cursor-supported interpolation syntax inside strings
- `targets/cursor/AGENTS.md` should be treated as optional, not required
- if `--include-user-scope` is used with Cursor import flows, the error should point to deferred global `~/.cursor/mcp.json` support rather than silently ignoring the flag

Optional warning:

- if both `targets/cursor/AGENTS.md` and legacy `.cursorrules` exist in native import scenarios

Acceptance criteria:

- strict mode catches malformed authored state
- diagnostics use clear Cursor-specific messages

## Phase 6: Example Repository

Add a production-style example, likely:

- `examples/plugins/cursor-basic/`

Example should contain:

- `plugin.yaml`
- one default `targets/cursor/rules/project.mdc`
- optional `mcp/servers.yaml`
- optional `targets/cursor/AGENTS.md`
- rendered `.cursor/rules/project.mdc`
- rendered `.cursor/mcp.json` only if `mcp/servers.yaml` exists
- rendered root `AGENTS.md` only if `targets/cursor/AGENTS.md` exists

Keep the example intentionally narrow.

Do not put speculative files into the example.

## Phase 7: Test Matrix

Required test layers:

- unit tests for `cursorAdapter`
- scaffold/init tests
- render/import/validate integration tests
- production example canary
- contract clarity tests for docs and support matrix wording

Recommended concrete tests:

- `TestCursorRenderWritesMCPRulesAndAgents`
- `TestCursorImportRoundTrip`
- `TestCursorValidateRejectsNonMdcRules`
- `TestCursorValidateRejectsTraversalOrSymlink`
- `TestCursorMCPPreservesInterpolationStrings`
- `TestCursorDetectNativeIgnoresStandaloneRootAgents`
- `TestCursorImportRejectsIncludeUserScope`
- `TestPluginKitAIInit_Cursor`
- extend `production_examples_integration_test.go` with `cursor`

Required repo test touchpoints:

- `cli/plugin-kit-ai/internal/app/app_test.go`
- `cli/plugin-kit-ai/internal/scaffold/scaffold_test.go`
- `cli/plugin-kit-ai/internal/pluginmanifest/manifest_test.go`
- `cli/plugin-kit-ai/internal/platformexec/*_test.go` or new `cursor_test.go`
- `cli/plugin-kit-ai/internal/targetcontracts/contracts_test.go`
- `repotests/cli_init_integration_test.go`
- `repotests/cli_capabilities_integration_test.go`
- `repotests/contract_clarity_integration_test.go`

Live smoke:

- not required for `v1`

Reason:

- this target manages deterministic workspace files only
- unlike OpenCode plugin loading, we do not need an external runtime smoke path to prove our stable boundary

Confidence on skipping live smoke in `v1`:

- `Увер. 8/10`, `Надёж. 8/10`

## Phase 8: Docs and Contract Updates

Must update:

- root `README.md`
- `cli/plugin-kit-ai/README.md`
- `docs/SUPPORT.md`
- `docs/STATUS.md`
- generated support matrices
- possibly `docs/PRODUCTION.md` if we want to explain when to use `cursor`
- `cli/plugin-kit-ai/cmd/plugin-kit-ai/init.go`
- `cli/plugin-kit-ai/cmd/plugin-kit-ai/render.go`
- `cli/plugin-kit-ai/cmd/plugin-kit-ai/import.go`

Recommended wording:

- "Cursor is currently supported as a workspace-config lane for MCP and rules, not as a VS Code extension packaging lane."
- "The optional rendered root `AGENTS.md` is a Cursor-compatible instruction surface, but it may also be read by other agent tools."

## Final `v1` Decisions

These decisions are intentionally fixed for implementation:

- `AGENTS.md` is optional
- legacy `.cursorrules` import is included in `v1` as compatibility import and should normalize into `targets/cursor/rules/legacy.mdc` with a warning
- `.cursor/mcp.json` is strict JSON-only in `v1`; do not claim JSONC support without official documentation
- root `CLAUDE.md` compatibility is deferred and must not be imported or rendered in `v1`
- nested non-root `.cursor/rules` authored support is deferred and must not be silently implied
- Cursor rules stay target-native; do not create a cross-target instruction abstraction in this feature
- auto-detect must ignore standalone root `AGENTS.md`
- default scaffold should include a rule file, not `AGENTS.md`
- `target_support_matrix.md` must be kept in sync with `targetcontracts.Markdown(All())`; updating its generation pipeline is a separate follow-up, not part of this feature

## Risks

## Risk 1: Over-modeling Cursor

Failure mode:

- we accidentally promise support for broader Cursor behavior than we can validate

Mitigation:

- keep `v1` strictly to MCP + rules + root `AGENTS.md`

## Risk 2: Wrong abstraction with OpenCode

Failure mode:

- we couple Cursor and OpenCode because both happen to be workspace-config targets

Mitigation:

- share only MCP/file helpers
- keep target-specific native contracts separate

## Risk 3: AGENTS.md ownership conflicts

Failure mode:

- repositories already use `AGENTS.md` for non-Cursor purposes

Mitigation:

- import before render
- document managed ownership clearly
- warn on conflicting existing unmanaged root `AGENTS.md` if needed

## Risk 4: Cursor docs surface is broader than the `v1` lane

Failure mode:

- users assume support for nested `.cursor/rules`, root `CLAUDE.md`, or global Cursor MCP config because Cursor itself supports them

Mitigation:

- document the exact stable subset
- mark `CLAUDE.md`, global MCP config, and nested non-root rules as deferred compatibility work

## Risk 5: Over-validating MCP config

Failure mode:

- we reject valid Cursor `mcp.json` files that use interpolation syntax or transport fields we do not semantically inspect

Mitigation:

- keep MCP validation structurally permissive
- preserve arbitrary supported strings and object fields that survive portable MCP round-trip

## Risk 6: `AGENTS.md` is a shared instruction surface, not a Cursor-only file

Failure mode:

- users assume rendered root `AGENTS.md` affects only Cursor, but it also changes behavior in other agent tools that read the same root file

Mitigation:

- keep `AGENTS.md` optional
- document the cross-tool effect explicitly in scaffold and support docs
- prefer `.cursor/rules/**` as the primary Cursor-native instruction surface and treat root `AGENTS.md` as an opt-in compatibility surface

## Acceptance Criteria For The Whole Feature

The feature is done when:

- `plugin-kit-ai init <name> --platform cursor` works
- `plugin-kit-ai render . --target cursor` writes deterministic Cursor-native files
- `plugin-kit-ai render --check . --target cursor` detects drift
- `plugin-kit-ai import . --from cursor` reconstructs authored state
- `plugin-kit-ai validate . --platform cursor --strict` meaningfully validates the supported subset
- docs and support matrix clearly describe the stable boundary
- example repo passes integration coverage
- CLI help and `capabilities` output mention `cursor` consistently
- auto-detect import does not claim Cursor solely because root `AGENTS.md` exists

## Recommended Execution Order

1. update `sdk/platformmeta/platformmeta.go`
2. implement `cursorAdapter` skeleton and registry entry
3. add scaffold templates
4. run generator refresh for `platforms_gen.go`, `rules_gen.go`, and descriptor docs
5. implement render
6. implement validate
7. implement import and detection policy
8. update CLI help text and target contract docs
9. refresh `docs/generated/target_support_matrix.md`
10. add unit/integration/example coverage
11. run `make test-required`, targeted `repotests`, and `make generated-check`

This order keeps the target visible early while postponing import complexity until the native shape is fixed.

Execution order score:

- `Увер. 9/10`, `Надёж. 9/10`

## Source Pointers

Current repository files most relevant to implementation:

- [platformmeta.go](/Users/belief/dev/projects/claude/hookplex/sdk/platformmeta/platformmeta.go)
- [generator.go](/Users/belief/dev/projects/claude/hookplex/sdk/generator/generator.go)
- [registry.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/platformexec/registry.go)
- [opencode.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/platformexec/opencode.go)
- [manifest.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/pluginmanifest/manifest.go)
- [contracts.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/targetcontracts/contracts.go)
- [init.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/app/init.go)
- [platforms_gen.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/scaffold/platforms_gen.go)
- [rules_gen.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/validate/rules_gen.go)
- [helpers.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/platformexec/helpers.go)
- [init.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/cmd/plugin-kit-ai/init.go)
- [render.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/cmd/plugin-kit-ai/render.go)
- [import.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/cmd/plugin-kit-ai/import.go)
- [production_examples_integration_test.go](/Users/belief/dev/projects/claude/hookplex/repotests/production_examples_integration_test.go)

## Implementation Checklist

1. Add `cursor` `PlatformProfile` in [platformmeta.go](/Users/belief/dev/projects/claude/hookplex/sdk/platformmeta/platformmeta.go) with `code_plugin + workspace_config_lane`, `rules` mirror, `agents_md` native doc, optional `AGENTS.md` managed artifact, and no launcher/runtime contract.
2. Add `cursorAdapter{}` in [registry.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/platformexec/registry.go) and new implementation file `cursor.go`.
3. Create templates `cursor.README.md.tmpl`, `cursor.rule.mdc.tmpl`, and `cursor.AGENTS.md.tmpl` under [templates](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/scaffold/templates).
4. Refresh generated files with `go run ./cmd/plugin-kit-ai-gen` and verify [platforms_gen.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/scaffold/platforms_gen.go), [rules_gen.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/validate/rules_gen.go), and [docs/generated/support_matrix.md](/Users/belief/dev/projects/claude/hookplex/docs/generated/support_matrix.md).
5. Update [docs/generated/target_support_matrix.md](/Users/belief/dev/projects/claude/hookplex/docs/generated/target_support_matrix.md) to match `targetcontracts.Markdown(All())`; keep [contracts_test.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/targetcontracts/contracts_test.go) green.
6. Update [init.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/app/init.go) and CLI help in [cmd init](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/cmd/plugin-kit-ai/init.go), [cmd render](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/cmd/plugin-kit-ai/render.go), and [cmd import](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/cmd/plugin-kit-ai/import.go) so `cursor` is treated like a workspace-config lane with no runtime flags.
7. Implement render/managed paths/validate/import in `cursor.go`, including the rule that auto-detect ignores standalone root `AGENTS.md`.
8. Add or update tests in `app_test.go`, `scaffold_test.go`, `manifest_test.go`, `contracts_test.go`, `cli_init_integration_test.go`, `cli_capabilities_integration_test.go`, and `contract_clarity_integration_test.go`.
9. Add `examples/plugins/cursor-basic/` and extend example coverage.
10. Update [README.md](/Users/belief/dev/projects/claude/hookplex/README.md), [cli README](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/README.md), [SUPPORT.md](/Users/belief/dev/projects/claude/hookplex/docs/SUPPORT.md), and [STATUS.md](/Users/belief/dev/projects/claude/hookplex/docs/STATUS.md) with the exact stable subset wording.
11. Verification gate: run `make test-required`, targeted `go test` for Cursor-specific packages, `go test ./repotests -run 'TestPluginKitAIInitGeneratesBuildableModule|TestPluginKitAICapabilities|TestContractClarity_RuntimeMetadataAndDocsStayAligned'`, and `make generated-check`.

External documentation snapshot used for the plan:

- Cursor MCP docs: <https://docs.cursor.com/advanced/model-context-protocol>
- Cursor MCP Extension API: <https://docs.cursor.com/en/context/mcp-extension-api>
- Cursor CLI using `AGENTS.md` and `.cursor/rules`: <https://docs.cursor.com/en/cli/using>
- Cursor rules overview: <https://docs.cursor.com/en/context>
- Cursor CLI MCP: <https://docs.cursor.com/cli/mcp>
- Cursor VS Code compatibility direction: <https://docs.cursor.com/fr/get-started/migrate-from-vs-code>
