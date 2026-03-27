# Delivery Status Ledger

This ledger tracks the current delivery state of the shipped architecture and the remaining blockers to `v1.0`.

## Current Release Phase

- `v0.9 freeze active`
- `release rehearsal completed`
- `release evidence refreshed`
- `git-backed release execution in progress`

| Area | Status | Notes |
|------|--------|-------|
| Runtime foundation | done | Platform-neutral runtime, generated lookup, middleware chain, and platform registrars are shipped. |
| Descriptor system | done | Runtime wiring, scaffold rules, validate rules, registrars, and support docs are generated from descriptor definitions. |
| Generated contract docs | done | Support claims are emitted from descriptors into the generated support matrix and exposed through `hookplex capabilities`. |
| Public contract freeze | done | `README`, support policy, and SDK stability docs now describe the current shipped contract instead of a transition state. |
| Codex GA path | done | Runtime, scaffold, validate, integration coverage, and repository-owned opt-in real `codex exec` smoke test exist. The supported invocation semantics are now frozen in the release audit, and external runtime-health failures are explicitly outside the hookplex stable promise. |
| Claude stabilization | done | Deterministic coverage, real-CLI smoke policy, and declared event-set review are complete. External Claude runtime connectivity failures are now handled as documented release waivers when hook execution is never reached. |
| Quality gates | done | `required`, `extended`, and `live` lanes exist in repo automation. Install compatibility now has both a deterministic matrix and refreshed live raw-binary / supported-tarball / unsupported-layout evidence. |
| Release discipline | done | Changelog, CI lanes, release checklist, audit ledger, migration registry, release playbook, release-notes template, rehearsal worksheet, install verification, and version command exist. A full release rehearsal has been recorded. |
| Security and diagnostics | done | Threat model, diagnostics contract, checksum verification, install compatibility contract, deterministic regression coverage, and refreshed live install compatibility evidence now exist. |
| `v1.0` readiness | partial | Rehearsal evidence and stable-approved decisions exist. Latest full-access evidence refresh passed deterministic, extended, and live checks. Remaining gap: final maintainer sign-off and tag execution. |

## Current Blockers

- Release notes still need final maintainer sign-off and tag execution.

## Exit Criteria For `v1.0`

- Carry rehearsal waivers and known limitations into the final release notes.
- Publish the final migration/deprecation guidance with the tag.
- Cut the release tag from the rehearsed candidate or rerun evidence on the new candidate SHA.
