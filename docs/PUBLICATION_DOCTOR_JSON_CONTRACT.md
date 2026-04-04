# Publication Doctor JSON Contract

`plugin-kit-ai publication doctor --format json` is the automation-facing readiness report for the `publish/...` layer.

## Stability

- Contract id: `plugin-kit-ai/publication-doctor-report`
- Current schema version: `1`
- Stability tier: public contract for CI and tooling

## Envelope

Every JSON report includes:

- `format`: always `plugin-kit-ai/publication-doctor-report`
- `schema_version`: currently `1`
- `requested_target`: the explicit `--target` value when present
- `ready`: convenience boolean for publication readiness
- `status`: one of `ready`, `needs_channels`, or `inactive`
- `warning_count`
- `warnings`
- `issue_count`
- `issues`
- `next_steps`
- `publication`

When publication-capable package targets are missing authored channels, the report also includes:

- `missing_package_targets`

## Status Semantics

- `ready`: every publication-capable package target has an authored `publish/...` channel
- `needs_channels`: at least one publication-capable package target exists, but one or more required `publish/...` channels are missing
- `inactive`: no publication-capable package targets are enabled for the requested scope

## Issue Records

`issues` is the structured explanation surface for publication gaps.

Each issue record includes:

- `code`
- `message`
- `target` when the issue belongs to a specific package target
- `channel_family` when the issue belongs to a specific publication family
- `path` when a concrete authored path is relevant

Current issue codes:

- `no_publication_targets`
- `missing_channel`

## Array Guarantees

The following fields are always arrays in schema version `1`, never `null`:

- `warnings`
- `issues`
- `next_steps`
- `publication.packages`
- `publication.channels`

## Publication Payload

`publication` reuses the normalized publication model surfaced by:

- `plugin-kit-ai publication --format json`
- `plugin-kit-ai inspect --format json`
- `plugin-kit-ai validate --format json`

It includes:

- `core`
- `packages`
- `channels`

## Exit Semantics

- exit `0`: `ready` is `true`
- exit `1`: `ready` is `false`

Consumers should rely on both:

- shell exit code
- structured `ready` and `status`

## Compatibility Rules

- Additive fields may appear in future schema versions
- Breaking changes require a new `schema_version`
- Consumers should branch first on `format`, then on `schema_version`
- Consumers should prefer `status` over re-deriving readiness from package or channel counts
