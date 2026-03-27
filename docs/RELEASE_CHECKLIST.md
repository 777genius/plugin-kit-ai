# Release Checklist

Use this checklist for post-`v1.0.0` hardening releases and any beta surface that is still shipping outside the stable contract boundary.

## Required Before Tagging

- `make release-gate` green
- `make release-gate` includes `test-required`, `vet`, and `generated-check`
- `make release-rehearsal` may be used as the canonical deterministic local rehearsal shortcut
- `make test-install-compat` green
- `make test-polyglot-smoke` green when executable-runtime beta claims or launcher logic changed
- generated artifacts in sync
- support matrix matches shipped claims
- changelog updated
- support/status/release docs updated if contract changed
- candidate commit SHA recorded

## Extended / Live Recording

- `polyglot-smoke` workflow result recorded when executable-runtime beta claims, launcher logic, or Windows runtime resolution changed
- `extended` workflow result recorded
- `live` workflow result recorded, or an explicit waiver is noted in release notes
- any skipped real-CLI smoke reason is written down
- waiver justification explicitly states why the failure is outside plugin-kit-ai contract scope
- release notes use the same evidence fields as the release playbook

## Beta-Breaking Changes

- migration note written when beta user code or scaffold output changes
- deprecation or removal called out in docs/changelog
- stable-candidate set impact reviewed
- [V0_9_AUDIT.md](./V0_9_AUDIT.md) updated when the declared `v1` candidate set changes

## Rehearsal Completion

- each candidate surface is marked `stable-approved`, `stays-beta`, or `blocked`
- no core stable-set surface remains `blocked`
- release notes draft exists
- rehearsal worksheet exists
- known limitations are written down

## `v0.9` Freeze Check

- no new public-beta surfaces added unless required to finish the declared `v1` set
- remaining work limited to bug fixes, docs, migration, e2e hardening, and release tightening
