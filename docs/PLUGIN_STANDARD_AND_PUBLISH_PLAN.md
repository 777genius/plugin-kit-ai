# Plugin Standard And Publish Plan

Plan date: 2026-04-04

This document fixes the current design direction for `hookplex` package authoring, vendor manifests, and future marketplace or gallery publication.

It describes the proposed long-term standard direction for this repository and ecosystem strategy. It does **not** claim that this standard is already adopted outside `hookplex`.

It combines:

- current repository facts
- confirmed vendor constraints from official docs
- the architectural decisions agreed in discussion
- an implementation plan from start to finish

Related research:

- [Codex, Claude, and Gemini publication research](/Users/belief/dev/projects/claude/hookplex/docs/research/plugin-marketplaces/README.md)
- [Codex target boundary](/Users/belief/dev/projects/claude/hookplex/docs/CODEX_TARGET_BOUNDARY.md)
- [Publish Layer Spec](/Users/belief/dev/projects/claude/hookplex/docs/PUBLISH_LAYER_SPEC.md)

## Goal

Create a durable architecture where:

- `plugin.yaml` becomes the minimal universal plugin core standard used by `hookplex`
- `targets/...` stay the vendor-specific authored adaptation layer
- `publish/...` becomes the vendor publication layer for marketplaces and galleries
- vendor-visible manifests remain real generated artifacts in the filesystem
- one repository can produce installable or publishable outputs for multiple AI plugin ecosystems

This plan explicitly avoids collapsing all vendor ecosystems into one fake universal manifest format. The universal core layer should be small and stable. Vendor-specific and publication-specific concerns must stay separated.

## Confirmed Vendor Constraints

### Codex

Confirmed from official OpenAI docs:

- Codex plugin bundles are separate from Codex marketplace catalogs.
- The plugin bundle contract is centered on `.codex-plugin/plugin.json` with optional sidecars such as `.app.json` and `.mcp.json`.
- Codex marketplace catalogs live at `.agents/plugins/marketplace.json` under a repo root or user home root.
- `source.path` in Codex marketplace entries resolves relative to the marketplace root and should remain inside that root.
- Marketplace install state and plugin cache are separate from the plugin bundle itself.

Implication:

- Codex package manifests and Codex marketplace catalogs must stay separate concepts in our model.

Official docs:

- [OpenAI Plugins overview](https://developers.openai.com/codex/plugins/)
- [OpenAI Build plugins](https://developers.openai.com/codex/plugins/build)

### Claude

Confirmed from official Anthropic docs:

- Claude marketplaces are separate catalog roots using `.claude-plugin/marketplace.json`.
- Claude plugins have their own `.claude-plugin/plugin.json`.
- Marketplace roots and plugin roots are distinct, although they can live in the same repository.
- Marketplace sources resolve relative to the marketplace root.
- Claude has stronger first-class marketplace tooling than Codex, including CLI and slash-command workflows.

Implication:

- Claude plugin package metadata and Claude marketplace metadata must also stay separate concepts in our model.

Official docs:

- [Anthropic plugin marketplaces](https://code.claude.com/docs/en/plugin-marketplaces)
- [Anthropic discover plugins](https://code.claude.com/docs/en/discover-plugins)

### Gemini

Confirmed from official Gemini CLI docs:

- Gemini has an extension ecosystem and official gallery.
- Extensions can be installed from GitHub URLs or local paths.
- To appear in the gallery, the repository must be public, tagged with the `gemini-cli-extension` GitHub topic, and the `gemini-extension.json` manifest must be at the absolute repository root or archive root.
- Gemini gallery indexing is not modeled as a local `marketplace.json` catalog in the same way as Codex or Claude.

Implication:

- Gemini publication is a gallery or release channel, not a marketplace manifest contract equivalent to Codex or Claude.

Official docs:

- [Gemini CLI extensions](https://geminicli.com/docs/extensions/)
- [Gemini release extensions](https://geminicli.com/docs/extensions/releasing/)
- [Gemini gallery](https://geminicli.com/extensions/)

## Key Architectural Conclusion

The three ecosystems share the same high-level idea:

- package something
- publish or expose it through a discovery channel
- install it into the vendor environment

But they do **not** share the same filesystem or metadata format for publication.

Therefore:

- we should **not** build one universal marketplace manifest
- we **should** build one universal plugin core standard for `hookplex`
- we **should** build separate vendor package adapters
- we **should** build separate vendor publication channel adapters

## Final Layering Decision

We fix the architecture into four layers.

### 1. `plugin.yaml`

Universal plugin core standard for `hookplex`.

Purpose:

- identify the plugin
- describe the plugin at a high level
- declare which target adapters are enabled

This layer must stay intentionally small.

### 2. `targets/...`

Vendor-specific authored adaptation layer.

Purpose:

- hold authored data that is real and necessary for a specific vendor
- avoid forcing vendor-specific semantics into the universal core standard

Examples:

- `targets/codex-package/...`
- `targets/codex-runtime/...`
- `targets/claude/...`
- `targets/gemini/...`

### 3. `publish/...`

Marketplace, gallery, and catalog publication layer.

Purpose:

- hold publication-channel metadata
- describe how a plugin should be listed, discovered, installed, or indexed
- stay separate from package identity and separate from vendor runtime content

Examples:

- `publish/codex/...`
- `publish/claude/...`
- `publish/gemini/...`

### 4. Generated vendor artifacts

Rendered public artifacts that vendor tooling actually reads.

Examples:

- `.codex-plugin/plugin.json`
- `.app.json`
- `.mcp.json`
- `.claude-plugin/plugin.json`
- `gemini-extension.json`

These are not the primary authored source of truth. They are generated from the authored layers above. But they must physically exist where vendor tooling expects them.

## Final `plugin.yaml` Direction

### Decision

`plugin.yaml` becomes the minimal universal core standard for `hookplex`.

### Minimal fields

The agreed minimal shape is:

```yaml
api_version: v1
name: my-plugin
version: 0.1.0
description: Short plugin description
targets:
  - codex-package
```

### Field semantics

#### `api_version`

- version of the `plugin.yaml` schema
- replaces the current `format` magic string
- is about the manifest contract, not the plugin release

#### `name`

- stable plugin identity
- machine-friendly slug
- plays the same role that `name` plays in `package.json`
- we explicitly do **not** introduce `id` at this stage

Reason:

- `id` semantics vary too much across ecosystems
- a universal standard should avoid prematurely locking a second identity field
- `name` is enough if we define it strictly

#### `version`

- plugin release version
- independent from `api_version`

#### `description`

- short human-facing description of the plugin

#### `targets`

- enabled vendor package or runtime adapters
- keeps the universal layer aware of intended output families without embedding vendor-specific package details
- this is explicitly a `hookplex` orchestration field in the core manifest, not a claim that every external plugin ecosystem uses the same concept in the same way

### Fields explicitly excluded from `plugin.yaml`

Do not add these to the core standard:

- `id`
- `authors`
- `license`
- `homepage`
- `repository`
- `keywords`
- `category`
- marketplace installation policies
- marketplace authentication policies
- Codex interface details
- Codex app metadata
- Gemini settings
- Gemini themes
- Gemini hooks
- Claude marketplace source metadata
- any vendor-specific publication metadata

Reason:

- these are package-distribution, publication, or vendor adaptation concerns
- they would make the standard vendor-shaped too early
- they would make `plugin.yaml` harder to stabilize as a durable core contract

## Why `format` Should Be Replaced

Current `plugin.yaml` uses:

```yaml
format: plugin-kit-ai/package
```

This is fine as an internal repository-era marker, but weak as a long-term core standard because:

- it is tool-branded
- it is ambiguous whether it means kind, schema version, or both
- it looks like an implementation marker rather than an ecosystem contract

Final direction:

- move from `format` to `api_version`
- avoid adding `kind` for now

Reason:

- `api_version` is clearer
- `kind` is only useful if we are standardizing multiple top-level manifest families right now
- today we only need one core plugin manifest plus separate publication schemas

## Why `name` Is Enough For Now

We explicitly decided:

- `name` is enough
- `id` is not needed in the minimal standard

Reason:

- this mirrors the ergonomics of `package.json`
- plugin identity should remain simple
- most ecosystems already accept a stable package-style name as the main identity

If a separate identity field is ever needed in the future, it can be introduced later with a very strong semantic contract. It should not be added speculatively.

## Role of `targets/...`

`targets/...` remains the place where vendor-specific authored data lives.

This is necessary because vendor ecosystems expect real vendor-specific structures and metadata.

Examples:

- Codex package needs authored package metadata, interface, and optional app content
- Codex runtime needs runtime-specific config extras
- Gemini needs settings, themes, hooks, contexts, and extension-specific metadata
- Claude needs plugin-specific authored structures that are not universal

This layer exists to prevent `plugin.yaml` from becoming polluted with vendor semantics.

## Role of `publish/...`

`publish/...` is the future home for publication channel data.

This layer is for:

- marketplace entries
- gallery metadata
- release channel configuration
- installation and discovery settings

This layer is **not** the same as package metadata.

That distinction is critical:

- package metadata describes the plugin package itself
- publication metadata describes how that package appears in a marketplace, gallery, or catalog

### Likely future shape

Illustrative examples of possible publication roots:

- `publish/codex/marketplace.yaml`
- `publish/claude/marketplace.yaml`
- `publish/gemini/gallery.yaml`

These paths are directionally correct examples, not yet frozen final file names.

This is intentionally a separate tree, not part of `plugin.yaml`.

## Why Vendor Files Must Still Exist

Even with a universal authored core standard, vendor-facing files must still physically exist because vendor tooling indexes or validates those concrete files.

Examples:

- Codex bundle tooling expects `.codex-plugin/plugin.json`
- Claude plugin tooling expects `.claude-plugin/plugin.json`
- Gemini gallery expects `gemini-extension.json` at the repository root or archive root

So the correct model is:

- authored universal data and authored target data exist in our package-standard layout
- vendor-visible manifests are generated into the actual filesystem locations that vendor tooling expects

We should not hide everything behind an internal abstraction that never materializes those vendor files.

## Current State In This Repository

Today `plugin.yaml` already exists and is minimal.

Current fields:

- `format`
- `name`
- `version`
- `description`
- `targets`

Current validation is implemented in:

- [pluginmodel/model.go](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/pluginmodel/model.go)

Current scaffold template:

- [plugin.yaml.tmpl](/Users/belief/dev/projects/claude/hookplex/cli/plugin-kit-ai/internal/scaffold/templates/plugin.yaml.tmpl)

This means the new direction is evolutionary, not a greenfield rewrite. We are already close to the desired end state.

## Migration Direction

### Principle

Do not keep long-lived dual semantics.

We do not want a forever state where both `format` and `api_version` are treated as equal first-class fields.

### Preferred migration

1. Introduce `api_version: v1`
2. Update scaffold and normalization to emit `api_version`
3. Update validation and loading logic to understand the new field
4. Add one clear migration path from old `format` manifests
5. Remove long-term dependence on `format`

This should be a deliberate migration, not an open-ended compatibility mode.

## Non-Goals

This plan does **not** do the following:

- define a universal vendor manifest format
- define a universal marketplace schema shared by all vendors
- collapse Codex, Claude, and Gemini publication channels into one file format
- move all authored data into `plugin.yaml`
- remove generated vendor files from the repository contract

## Proposed End-State UX

The intended author workflow becomes:

1. Author the plugin core in `plugin.yaml`
2. Author vendor-specific behavior under `targets/...`
3. Author portable subsystems such as MCP in portable files like `mcp/servers.yaml`
4. Optionally author publication metadata under `publish/...`
5. Run render to materialize vendor-visible manifests and artifacts
6. Run validate to confirm authored and rendered state match
7. Publish to one or more vendor channels

The intended mental model becomes:

- `plugin.yaml` = what this plugin is
- `targets/...` = how this plugin adapts to each vendor ecosystem
- `publish/...` = how this plugin is listed or distributed
- generated files = what vendors actually consume

## Recommended Rollout Plan

### Phase 1. Freeze the standard direction

Deliverables:

- this plan document
- agreement on minimal `plugin.yaml`
- agreement that `name` is the only identity field
- agreement that `publish/...` is a separate layer

Status:

- fixed by this document

### Phase 2. Define the exact `plugin.yaml v1` contract

Deliverables:

- formal field rules for:
  - `api_version`
  - `name`
  - `version`
  - `description`
  - `targets`
- rules for valid `name`
- rules for version format expectations

Output:

- spec doc
- validation tests

### Phase 3. Replace `format` with `api_version`

Deliverables:

- loader and normalizer updates
- scaffold updates
- migration guidance
- explicit validation behavior for old manifests

Constraints:

- avoid indefinite dual support
- provide one clean migration path

### Phase 4. Introduce an internal normalized publication model

Deliverables:

- internal model that combines:
  - `plugin.yaml`
  - `targets/...`
  - portable authored files
  - future `publish/...`
- no public layout break required yet

This is an internal code model, not a new user-facing super-file.

Status:

- in progress and partially implemented
- current implementation exposes a normalized `publication` summary through `plugin-kit-ai inspect --format json`
- current implementation keeps publication modeling internal and does not freeze `publish/...` filesystem layout yet

### Phase 5. Define `publish/...`

Deliverables:

- layout for publication channels
- first draft of:
  - `publish/codex/...`
  - `publish/claude/...`
  - `publish/gemini/...`

This phase must keep channel metadata clearly separate from package identity.

Status:

- started and partially implemented
- current implementation reads and validates authored publication schemas at:
  - `publish/codex/marketplace.yaml`
  - `publish/claude/marketplace.yaml`
  - `publish/gemini/gallery.yaml`
- current implementation exposes publication channels through `plugin-kit-ai inspect`
- current implementation does not yet render vendor publication artifacts

### Phase 6. Implement publication channel adapters

Deliverables:

- Codex marketplace adapter
- Claude marketplace adapter
- Gemini gallery or release adapter

Expected behavior:

- render publication-channel artifacts from authored data
- validate those artifacts
- keep vendor-specific publication details out of `plugin.yaml`

### Phase 7. Add publish workflows

Deliverables:

- publish-oriented CLI workflows
- validation for publication outputs
- optional automation such as PR generation or sync steps

Example future command shape:

```bash
plugin-kit-ai publish --channel codex-marketplace
plugin-kit-ai publish --channel claude-marketplace
plugin-kit-ai publish --channel gemini-gallery
```

Or a multi-channel wrapper:

```bash
plugin-kit-ai publish --all
```

## Top Design Choices That Are Now Fixed

### 1. `plugin.yaml` stays minimal

`(🎯 10/10) (🛡️ 10/10) (🧠 4/10)`  

Fixed decision:

- keep `plugin.yaml` small and stable

### 2. Use `api_version`, not `format`, in the long-term standard

`(🎯 10/10) (🛡️ 10/10) (🧠 4/10)`  

Fixed decision:

- move toward `api_version: v1`
- do not keep `format` as the final standard marker

### 3. Do not add `id`

`(🎯 10/10) (🛡️ 10/10) (🧠 2/10)`  

Fixed decision:

- `name` is enough for now

### 4. Publication metadata belongs in `publish/...`

`(🎯 10/10) (🛡️ 10/10) (🧠 6/10)`  

Fixed decision:

- do not overload `plugin.yaml`
- do not overload target package files with marketplace-only concerns

### 5. Vendor manifests remain generated

`(🎯 10/10) (🛡️ 10/10) (🧠 3/10)`  

Fixed decision:

- vendor-facing manifests remain real filesystem artifacts
- they are not the main authored source of truth

## Final Summary

The final architectural position is:

- `plugin.yaml` becomes the universal plugin core standard for `hookplex`
- it stays intentionally minimal
- `name` is the only identity field for now
- `api_version` replaces `format`
- vendor-specific authored details stay in `targets/...`
- marketplace and gallery publication details live in `publish/...`
- vendor-consumed manifests remain generated artifacts in the places vendor tooling expects

This gives `hookplex` the right long-term shape for:

- one repository
- one core plugin identity
- multiple vendor package targets
- multiple publication channels
- no fake cross-vendor marketplace abstraction
