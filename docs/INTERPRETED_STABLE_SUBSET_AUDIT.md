# Interpreted Stable Subset Audit

This ledger records the post-`v1.0.0` promotion of the community-first interpreted local-runtime subset.

## Scope

Promoted to `public-stable` in the current source tree:

- targets: `codex-runtime`, `claude`
- runtimes: `python`, `node`
- commands in scope:
  - `plugin-kit-ai init --runtime python`
  - `plugin-kit-ai init --runtime node`
  - `plugin-kit-ai init --runtime node --typescript`
  - `plugin-kit-ai doctor`
  - `plugin-kit-ai bootstrap`
  - `plugin-kit-ai validate --strict`
  - `plugin-kit-ai export`
  - `plugin-kit-ai bundle install`

Explicitly **not** promoted in this audit:

- launcher-based `shell` runtime authoring
- `plugin-kit-ai bundle fetch`
- `plugin-kit-ai bundle publish`
- `plugin-kit-ai install` for interpreted bundles or dependency-preinstalled installs
- interpreted packaged distribution beyond bounded portable `export`
- TypeScript as a separate runtime contract

## Stable Boundary

Stable promise for this subset means:

- deterministic repo-local authoring on `codex-runtime` and `claude`
- deterministic readiness semantics through `doctor`
- deterministic lockfile-first bootstrap for supported Python and Node managers
- `validate --strict` as the CI-grade readiness gate
- deterministic portable handoff through `export`
- deterministic local unpack/install handoff through `bundle install`

Supported manager boundary:

- `python`: `requirements.txt`, repo-local `venv`, `uv`, `poetry`, `pipenv`
- `node`: `npm`, `pnpm`, `yarn`, `bun`
- `typescript`: stable authoring mode via `--runtime node --typescript`

## Evidence Required

Promotion requires:

- descriptor-backed docs and policy alignment
- scaffold alignment
- runtimecheck/bootstrap/doctor/validate/export alignment
- unit coverage
- integration coverage
- contract coverage
- deterministic `polyglot-smoke` evidence on Unix and Windows for the promoted subset

## Promotion Decision

Current status:

- `python`: `stable-approved`
- `node`: `stable-approved`
- `typescript via node`: `stable-approved`
- `bundle install for exported python/node local bundles`: `stable-approved`
- `shell`: `stays-beta`

Rationale:

- `node` and `python` provide the highest community value among non-Go authoring paths with bounded contract risk
- `shell` remains useful as an escape hatch, but still has a narrower reliability envelope and stays outside the stable subset
