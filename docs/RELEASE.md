# Release And Quality Gate Policy

This document defines the expected test lanes and release ladder for the current post-`v1.0.0` repository.

## Test Lanes

- `required`: deterministic local tests that must stay green on every change. This includes generator drift, unit tests, integration tests, and repository guard tests that do not require live external CLIs or network access.
- `polyglot-smoke`: deterministic cross-platform launcher and executable-ABI smoke for `go`, `python`, `node`, and `shell`, including Windows `.cmd` behavior and path-with-spaces coverage.
- `extended`: subprocess smoke and platform-CLI tests that may depend on locally installed tools or opt-in environment variables, but should still stay narrowly scoped and finish quickly.
- `nightly/live`: real network or externally authenticated scenarios, including live install compatibility checks and live-model sanity runs.

`extended` should prefer one external-CLI smoke class per `go test` invocation. This avoids mixed-process hangs from combining multiple real CLI harnesses in a single test process.

Current workflow mapping:

- `ci.yml`: blocking `required` lane
- `polyglot-smoke.yml`: deterministic Ubuntu/Windows polyglot smoke lane
- `extended.yml`: manual `extended` lane with artifact upload
- `live.yml`: manual live lane with artifact upload

## Branch And Flow Policy

- `main` remains the canonical releasable branch.
- Release rehearsal must be performed on one fixed candidate commit SHA.
- If any repo-tracked change lands after rehearsal evidence is recorded, rehearsal evidence must be refreshed for the new candidate commit.
- Promotion decisions come from the audit ledger, not from branch naming or tag naming alone.
- A rehearsal tag or notes artifact may be created before `v1.0`, but it does not imply promotion until the audit ledger is complete.

## Release Playbook

Use this exact order for stable or beta release work:

1. checkout the candidate commit
2. run `make test-required`
3. run `make vet`
4. run `make test-install-compat`
5. run `make test-polyglot-smoke`
6. verify generated artifacts are in sync
7. review `docs/V0_9_AUDIT.md`
8. review `docs/MIGRATIONS.md`
9. run or record `extended`
10. run or record `live`
11. record waivers for skipped external smoke if needed
12. draft release notes from the release-notes template
13. update each candidate row to `stable-approved`, `stays-beta`, or `blocked`
14. cut the rehearsal or release tag only after the audit ledger is complete

Required release artifacts:

- candidate commit SHA
- required lane result
- vet result
- install compatibility matrix result
- polyglot smoke result
- generated-artifact sync result
- extended result
- live result or waiver
- updated audit ledger
- updated migration registry
- release notes draft

No stable tag should be cut without one completed rehearsal cycle using this playbook.

## Shipping Gate For New Stable Functionality

No event or public contract claim should be treated as shipped unless all of the following exist:

- descriptor definition
- runtime wiring
- public registrar API
- scaffold support
- validate rules
- generated support-matrix row
- unit coverage
- integration coverage
- contract or golden coverage
- smoke e2e coverage

## Release Ladder

- `dev`: normal mainline delivery, still `public-beta`
- `beta`: feature-complete enough for targeted external validation
- `rc`: release-candidate stabilization; only bug fixes, docs, migration, and hardening work
- `stable`: reserved for `v1.0` and later major-compatible releases

See also [RELEASE_CHECKLIST.md](./RELEASE_CHECKLIST.md) for pre-tag execution steps.

## Waiver Policy

Waivers are allowed only for failures outside the plugin-kit-ai contract boundary:

- external Claude/Codex runtime-health failures before hook execution
- live/network failures in external systems that do not indicate a repo regression

Waivers are not allowed for:

- repo-controlled test failures
- deterministic required-lane failures
- scaffold/validate/runtime contract regressions
- smoke failures that show plugin-kit-ai misbehavior after the hook path should have executed

Every waiver must record:

- date
- candidate commit SHA
- affected lane
- exact skipped or failed surface
- reason
- why it is outside plugin-kit-ai contract scope
- maintainer sign-off in release notes or rehearsal notes

## `v0.9` Freeze Criteria

After `v0.9`, only these change classes are expected:

- bug fixes
- docs corrections
- migration notes
- quality-gate hardening
- e2e stabilization
- release process tightening

New public-beta surface should not be added after the freeze unless it is required to complete the declared `v1` stable set.

## Post-`v1.0` Hardening Mode

Immediately after `v1.0`, the repository should enter a short `v1.0.x` hardening loop. See [V1_0_X_HARDENING.md](./V1_0_X_HARDENING.md) for the allowed scope and first post-release backlog.
