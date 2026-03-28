# Delivery Status Ledger

This ledger tracks the current delivery state of the shipped architecture after `v1.0.0`.

## Current Release Phase

- `v1.0.0 released`
- `post-v1.0.x hardening active`
- `community-first interpreted stable subset promoted on main`
- `release evidence refreshed after tag`
- latest deterministic patch candidate rehearsal recorded at `8b3bdbbf400805c410ea05bec8b0c5215dacd131`

| Area | Status | Notes |
|------|--------|-------|
| Runtime foundation | done | Platform-neutral runtime, generated lookup, middleware chain, and platform registrars are shipped. |
| Descriptor system | done | Runtime wiring, scaffold rules, validate rules, registrars, and support docs are generated from descriptor definitions. |
| Generated contract docs | done | Support claims are emitted from descriptors into the generated support matrix and exposed through `plugin-kit-ai capabilities`. |
| Public contract freeze | done | `README`, support policy, and SDK stability docs now describe the current shipped contract instead of a transition state. |
| Codex GA path | done | Runtime, scaffold, validate, integration coverage, and repository-owned opt-in real `codex exec` smoke test exist. The supported invocation semantics are now frozen in the release audit, and external runtime-health failures are explicitly outside the plugin-kit-ai stable promise. |
| Claude stabilization | done | Deterministic coverage, real-CLI smoke policy, and declared event-set review are complete. External Claude runtime connectivity failures are now handled as documented release waivers when hook execution is never reached. |
| Quality gates | done | `required`, `polyglot-smoke`, `extended`, and `live` lanes exist in repo automation. `polyglot-smoke` now covers launcher/ABI checks plus generated Claude/Codex config canaries and rendered runtime-artifact drift protection. Install compatibility now has both a deterministic matrix and refreshed live raw-binary / supported-tarball / unsupported-layout evidence. |
| Community polyglot subset | done | `python` and `node` repo-local local-runtime authoring plus local exported bundle install on `codex-runtime` and `claude` is promoted in the source tree through [INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md); remote bundle fetch is now a separate `public-beta` handoff path and `shell` remains `public-beta`. |
| Release discipline | done | Changelog, CI lanes, release checklist, audit ledger, release playbook, release-notes template, rehearsal worksheet, install verification, generated-sync gate, and version command exist. Release rehearsal now includes the executable-runtime deterministic gate. A full release rehearsal has been recorded. |
| Security and diagnostics | done | Threat model, diagnostics contract, checksum verification, install compatibility contract, deterministic regression coverage, and refreshed live install compatibility evidence now exist. |
| `v1.0` readiness | done | `v1.0.0` is tagged at `6e9379868a666e79d7530a02e171a160c2cb1689`. Rehearsal evidence, stable-approved decisions, and post-tag live install compatibility refresh are recorded. |

## Current Blockers

- none for `v1.0.0`

## Post-Release Notes

- `v1.0.0` tag: `6e9379868a666e79d7530a02e171a160c2cb1689`
- current `main` is ahead with `v1.0.x` hardening and evidence refresh work
- the current source tree also carries the post-`v1` interpreted stable-subset promotion ledger in [INTERPRETED_STABLE_SUBSET_AUDIT.md](./INTERPRETED_STABLE_SUBSET_AUDIT.md)
- latest deterministic `v1.0.x` candidate rehearsal:
  - candidate SHA: `8b3bdbbf400805c410ea05bec8b0c5215dacd131`
  - date: `2026-03-27`
  - `make release-gate`: `pass`
  - `make test-install-compat`: `pass`
  - `make test-polyglot-smoke`: `pass`
  - generated-config/runtime-contract drift protection included in the recorded `polyglot-smoke` evidence
- any future patch tag should be cut from a newly evidenced candidate SHA, not by rewriting `v1.0.0`
