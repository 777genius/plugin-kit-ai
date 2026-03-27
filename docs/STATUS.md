# Delivery Status Ledger

This ledger tracks the current delivery state of the shipped architecture after `v1.0.0`.

## Current Release Phase

- `v1.0.0 released`
- `post-v1.0.x hardening active`
- `release evidence refreshed after tag`

| Area | Status | Notes |
|------|--------|-------|
| Runtime foundation | done | Platform-neutral runtime, generated lookup, middleware chain, and platform registrars are shipped. |
| Descriptor system | done | Runtime wiring, scaffold rules, validate rules, registrars, and support docs are generated from descriptor definitions. |
| Generated contract docs | done | Support claims are emitted from descriptors into the generated support matrix and exposed through `plugin-kit-ai capabilities`. |
| Public contract freeze | done | `README`, support policy, and SDK stability docs now describe the current shipped contract instead of a transition state. |
| Codex GA path | done | Runtime, scaffold, validate, integration coverage, and repository-owned opt-in real `codex exec` smoke test exist. The supported invocation semantics are now frozen in the release audit, and external runtime-health failures are explicitly outside the plugin-kit-ai stable promise. |
| Claude stabilization | done | Deterministic coverage, real-CLI smoke policy, and declared event-set review are complete. External Claude runtime connectivity failures are now handled as documented release waivers when hook execution is never reached. |
| Quality gates | done | `required`, `extended`, and `live` lanes exist in repo automation. Install compatibility now has both a deterministic matrix and refreshed live raw-binary / supported-tarball / unsupported-layout evidence. |
| Release discipline | done | Changelog, CI lanes, release checklist, audit ledger, migration registry, release playbook, release-notes template, rehearsal worksheet, install verification, and version command exist. A full release rehearsal has been recorded. |
| Security and diagnostics | done | Threat model, diagnostics contract, checksum verification, install compatibility contract, deterministic regression coverage, and refreshed live install compatibility evidence now exist. |
| `v1.0` readiness | done | `v1.0.0` is tagged at `6e9379868a666e79d7530a02e171a160c2cb1689`. Rehearsal evidence, stable-approved decisions, and post-tag live install compatibility refresh are recorded. |

## Current Blockers

- none for `v1.0.0`

## Post-Release Notes

- `v1.0.0` tag: `6e9379868a666e79d7530a02e171a160c2cb1689`
- current `main` is ahead with `v1.0.x` hardening and evidence refresh work
- any future patch tag should be cut from a newly evidenced candidate SHA, not by rewriting `v1.0.0`
