# `plugin.yaml` V1 Spec

Spec date: 2026-04-04

This document defines the intended `plugin.yaml` v1 contract for `plugin-kit-ai`.

It is the field-level companion to:

- [Plugin Standard and Publish Plan](/Users/belief/dev/projects/claude/hookplex/docs/PLUGIN_STANDARD_AND_PUBLISH_PLAN.md)

## Purpose

`plugin.yaml` is the minimal core plugin manifest for `plugin-kit-ai`.

It is intentionally small.

It describes:

- plugin identity
- plugin release version
- short description
- enabled target adapters

It does **not** describe:

- vendor marketplace publication details
- vendor gallery metadata
- vendor-specific package internals
- vendor-specific UI metadata

## V1 Shape

```yaml
api_version: v1
name: my-plugin
version: 0.1.0
description: Short plugin description
targets:
  - codex-package
```

## Fields

### `api_version`

Required.

Meaning:

- version of the `plugin.yaml` schema itself

Rules:

- must be `v1`
- must be a string

Notes:

- this replaces the old `format: plugin-kit-ai/package` marker as the long-term canonical shape
- old `format` manifests may be read only as a migration path, but they are not the v1 canonical form

### `name`

Required.

Meaning:

- stable plugin identity

Rules:

- must be a machine-friendly project name accepted by current `plugin-kit-ai` validation
- should be lowercase and slug-like
- should remain stable across releases

Notes:

- `name` is the only identity field in v1
- there is no separate `id`

### `version`

Required.

Meaning:

- plugin release version

Rules:

- must be a non-empty string
- semantic versioning is recommended

Notes:

- this is the plugin release version
- it is not the schema version

### `description`

Required.

Meaning:

- short human-facing summary of the plugin

Rules:

- must be a non-empty string
- should fit in one short sentence or phrase

### `targets`

Required.

Meaning:

- enabled `plugin-kit-ai` target adapters

Rules:

- must be a non-empty YAML sequence
- entries must be supported target ids
- entries must not be duplicated

Notes:

- this is intentionally a `plugin-kit-ai` orchestration field
- it is not a claim that every external plugin ecosystem uses the same concept directly

## Excluded From V1

These fields are intentionally excluded from `plugin.yaml` v1:

- `id`
- `authors`
- `license`
- `homepage`
- `repository`
- `keywords`
- `category`
- marketplace source metadata
- marketplace install policy
- marketplace auth policy
- Codex interface fields
- Codex app fields
- Gemini settings
- Gemini themes
- Gemini hooks
- Claude marketplace entry metadata

Reason:

- those belong either to vendor-specific target authoring or to the future `publish/...` layer

## Migration Notes

Legacy shape:

```yaml
format: plugin-kit-ai/package
name: my-plugin
version: 0.1.0
description: Short plugin description
targets:
  - codex-package
```

Canonical v1 shape:

```yaml
api_version: v1
name: my-plugin
version: 0.1.0
description: Short plugin description
targets:
  - codex-package
```

Migration principle:

- old manifests may be normalized into v1
- new scaffolds and normalized output should emit `api_version: v1`
- long-term contract should not keep both `format` and `api_version` as equal first-class fields

## Relationship To Other Layers

### `targets/...`

Holds vendor-specific authored data.

Examples:

- `targets/codex-package/...`
- `targets/codex-runtime/...`
- `targets/claude/...`
- `targets/gemini/...`

### `publish/...`

Will hold marketplace, gallery, and catalog publication metadata.

Examples:

- `publish/codex/...`
- `publish/claude/...`
- `publish/gemini/...`

### Generated vendor manifests

Examples:

- `.codex-plugin/plugin.json`
- `.claude-plugin/plugin.json`
- `gemini-extension.json`

These are rendered artifacts, not the primary authored source of truth.

