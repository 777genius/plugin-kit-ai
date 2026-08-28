# E2E and Competitive Launch Plan

Status: implementation plan and evidence record, updated 2026-08-28

## 1. Goal

Finish the existing Universal Agent Plugins implementation without another
rewrite, prove its public promises against the current stable release, and make
the product competitive with `plugins.sh`.

The release is complete when:

1. The current public CLI has one exact release, npm provenance, and disposable
   lifecycle evidence set. The separately protected hero matrix remains bound
   to its approved release tuple until all consent-bound rows pass.
2. The five hero packages pass runtime E2E in Codex, Cursor, and Kiro: 15/15
   client/package results.
3. The named Cloudflare Docs ChatGPT binding retains its separately scoped
   personal-app activation and read-only runtime evidence. No broader ChatGPT
   package claim is inferred.
4. The signed Directory is publicly reachable and the CLI proves cold, cached,
   offline, tampered, expired, and rollback behavior against it.
5. Three real upstream Agent Plugins 1.0 packaging PRs are open with isolated
   installation evidence.
6. Users can search a broad public discovery index, install a result, check for
   updates, and manage it across explicitly selected clients.

This plan replaces the previous universal architecture plan. Decisions already
implemented in the code remain valid unless this document explicitly changes
them.

## 2. Product boundary

### 2.1 Keep

- Agent Plugins 1.0.0 `plugin.json` is the portable package authority.
- `plugin-kit-ai` remains the CLI engine and release home for now.
- `universal-agent-plugins` remains the npm package.
- `agentplugins` remains the installed binary.
- Existing Go state, journal, rollback, source binding, client adapters, and
  signed Directory client remain the lifecycle foundation.
- Direct local and immutable GitHub sources continue to work without Directory
  submission.
- Targets remain explicit in non-interactive use. A command never silently
  installs into every detected client.
- User scope remains the only advertised scope until project scope has its own
  adapter and E2E evidence.
- Package preparation, native registration, manual activation, authentication,
  and runtime verification remain distinct outcomes.
- Update, repair, switch, and normal removal preserve `PLUGIN_DATA`; only an
  explicit ownership-checked purge deletes it.
- The CLI never installs Node, Python, Docker, browsers, or system packages
  implicitly.
- Existing product aliases remain reserved and cannot be reassigned to a new
  publisher.
- The product sends no install telemetry.

### 2.2 Do not build in this cycle

- a package blob hosting service;
- a database-backed registry;
- user accounts, ratings, comments, or moderation services;
- install-count telemetry;
- registry federation;
- a generic bridge transformation language;
- automatic PRs to thousands of upstream repositories;
- a second installer engine;
- CLI repository extraction;
- Agent Plugins 1.1 draft support.

## 3. Current truth

### 3.1 Already implemented

- standard-first package loading;
- exact-SHA GitHub and local acquisition;
- one-acquisition multi-target lifecycle;
- add, update, repair, remove, source switch, list, info, and doctor;
- Codex, ChatGPT-specific binding, Cursor, GitHub Copilot/VS Code, and Kiro
  delivery paths;
- 26 reviewed Directory products;
- three deterministic community bridges;
- signed snapshot generation, verification, cache, expiry, and sequence floor;
- static website and Git-native external submission flow;
- public-release-only evidence outside the protected hero contract:
  six-platform `0.1.21` binaries and npm facade, with public provenance and a
  clean-registry lifecycle proof;
- contributor fork-PR E2E.

### 3.2 Launch status and remaining blockers

1. The accepted exact-tuple matrix is currently 11/15: Codex 5/5, Cursor 3/5,
   and Kiro 3/5. The selected upstream Context7 package uses
   `https://mcp.context7.com/mcp/oauth`, so Cursor and Kiro need separate vendor
   consent; Notion remains the other consent-bound result in both clients.
   Separately, the active no-account community Context7 package passed real
   disposable add, native discovery, challenge-bound read-only tool call, and
   remove flows in Codex, Cursor, and Kiro from exact release source commit
   `dcd94db0bfafe5ff5c4b1f1154ee1f7c656c19e4`. That product-level result does
   not replace the approved upstream Context7 tuple in the protected matrix.
   The Kiro ACP capture remains a sanitized shape summary. Its capability probes do not replace the five-result launch matrix.
   Their sequence is not asserted.
2. Outside the protected hero contract, the latest public CLI is
   `universal-agent-plugins@0.1.21`, built from exact commit
   `f875dc6b45b2cd8322d05c2c9bf5c899bae1b09c`. Its release manifest,
   checksums, attestations, six native platform proofs, npm provenance, and
   clean-project npm verification passed in
   [release run 33196477229](https://github.com/777genius/plugin-kit-ai/actions/runs/33196477229)
   and [npm run 33197346470](https://github.com/777genius/plugin-kit-ai/actions/runs/33197346470).
   The public npm integrity is
   `sha512-mPy+oe7NwE6y2In+k+mcajSedWjPSC1JDDh+3zFuW+n9+SDkQGaohfGuNCYGWXy10nBrch8UCM1a3+s6zW4wlw==`.
   The accepted protected hero matrix remains bound to `0.1.18` until all four
   consent-bound rows pass and a new exact tuple is approved.
3. Signed production Directory sequence 13 is live and verifies. Signed
   sequence 16 is staged but is not production: the protected observer gate
   correctly blocked promotion. The staged snapshot digest is
   `sha256:80718f74941c948ec7731a7b4a8de63e381b70d0ce96e23be80fa4542c7907d8`,
   its source marker is `6ccc54978e06b73f2758b052b505de51362522b6`,
   and its ledger commit is `16ff04cac59756c15413b267ea5c6398a2df2c8f`.
4. The scheduled fixture, production Directory observation, and public-read
   jobs passed together on exact `main` commit
   `8934d258ecc1ef15d05b5bc8ef075e5dffd902cc` in
   [run 33142162465](https://github.com/777genius/universal-agent-plugins/actions/runs/33142162465).
   The protected evidence job was intentionally skipped in that credential-free
   regression and is not counted as complete.
5. Three real upstream packaging PRs are open with public exact-fork-SHA
   lifecycle evidence: [Chrome DevTools #2623](https://github.com/ChromeDevTools/chrome-devtools-mcp/pull/2623),
   [Cloudflare #465](https://github.com/cloudflare/mcp-server-cloudflare/pull/465),
   and [GitHub MCP Server #3169](https://github.com/github/github-mcp-server/pull/3169).
6. Released `0.1.18` ships the `install` alias, `search`, read-only `validate`,
   `outdated`, and `update --all`. Released `0.1.19` additionally keeps grouped
   dry-runs process-inert while preserving filesystem and managed-package
   identity checks; actual mutations still run native preflight before any
   client changes.
7. The public sequence-13 short-name Context7 install currently fails closed:
   its community distribution is suspended for the operation and the upstream
   alternative lacks current passed Kiro materialization evidence. Exact-SHA
   direct installation through public `0.1.19` passed one process-inert grouped
   dry-run for Codex, Cursor, and Kiro with tree digest
   `sha256:663f92049d29218aa8a5506a4f40fcc3002583a63730d4584ec12c84d481503d`.
   Do not advertise the short alias until a higher signed sequence restores one
   eligible reviewed default.
8. The public README Quick Start remains usable: `cloudflare-docs` resolved
   through production sequence 13 and public `0.1.19` passed one grouped
   Codex/Cursor/Kiro dry-run with no mutation and tree digest
   `sha256:2b1d984194324b50b756a893a576f3d795262bd7edfec6d7167863ca8be93a2c`.
9. Signed production Discovery sequence 3 was generated from exact `main`
   commit `a8488d99da04545ad53c61c48e1c91a1edc00805` and published in
   [run 33195617464](https://github.com/777genius/universal-agent-plugins/actions/runs/33195617464).
   It contains 2,437 unpadded conformant package paths, is marked complete,
   expires at `2026-08-31T17:38:56Z`, and has snapshot digest
   `sha256:02c2bf79ba440f4dc4af7125ecd28cb41a8b3e75d664de7f80bbe08afda41705`.
   The ledger branch and immutable sequence tag both resolve to commit
   `002261860bb342f7bff4f9f1da22534ab6d4df3b`; independent verification from
   the public Pages assets passed with the pinned Discovery key.
10. The live website returned HTTP 200 with no browser console errors, rendered
    `2437 unreviewed packages from signed index 3`, and found the exact
    `discovery:upstash/context7//plugins/agent-plugins/context7` selector after a
    real Chromium search for `context7`.
11. Public `universal-agent-plugins@0.1.21` passed one fresh disposable Linux
    lifecycle against that discovered upstream package for explicit
    `codex,cursor,kiro` targets: one acquisition, 3/3 add, a shared installation
    ID, info, 3/3 no-change update with an unchanged state digest, native Codex
    removal, 3/3 manager removal, and final doctor with zero installations and
    zero open operations. The final Codex command returned
    `No marketplace plugins found`; Cursor contained no plugin files and Kiro
    retained an empty `mcpServers` object. No real user project or identity was
    used. The package resolved to upstream revision
    `4e980f6b494d6f970cc5ec1df417ba684b2f6e0b`, tree digest
    `sha256:08eed3b67f2e71a11b68baa594380c2f69ec1bc97584d701deaf7942ac34c0d8`,
    and manifest digest
    `sha256:d01781acd899aefa9445a290cf43a481230321934d62f9c8a2aab06a89718236`.
12. Public `0.1.21` also rejected
    `discovery:netresearch/context7-skill` in group preflight because its skill
    has an invalid `allowed-tools` value. All three targets reported no mutation
    and no managed files or installation state were created. This proves that
    Discovery conformance is metadata, not permission to bypass the CLI's full
    package validation.

### 3.3 Implementation checkpoint

- Exact `0.1.18` release tuple: complete.
- Latest public CLI release `0.1.21`: public-release evidence only, outside the
  protected hero tuple; published, provenance-verified, and clean-registry
  lifecycle-verified.
- Capability-specific native projection (`skill` versus `mcp`): implemented.
- Real disposable Agent Code Navigator skill runtime: passed in Codex 0.147.0,
  Cursor 2026.08.25, and Kiro CLI 2.20.0. Cursor also passed a native Context7
  MCP runtime probe. These capability results are not the complete 15/15 matrix.
- Linux contract suites: 98 portable and 190 launch/workflow tests pass. The
  privileged run passed 96/97, exposed one stale Cursor fixture, and that exact
  corrected test then passed. This is pre-merge evidence, not the protected
  15/15 result.
- Last runtime-verified observer code checkpoint:
  `c4db619984795998f0a40a9f391f52534f6eb382`.
- Production identity tracking is monotonic and protected by publisher-only
  creation/update plus a no-bypass deletion guard. Production remains signed
  sequence 13; signed sequence 16 remains staged.
- Production Discovery is independently signed in its own domain. Sequence 3
  is live with 2,437 records and is consumed successfully by both the website
  and public CLI. This does not change the reviewed Directory sequence.
- Observer first-install import was fixed in
  [PR #94](https://github.com/777genius/universal-agent-plugins/pull/94).
  A real Kiro run then exposed an ACP readiness race: the prompt was sent before
  the native MCP catalog connected. [PR #95](https://github.com/777genius/universal-agent-plugins/pull/95)
  now waits for the exact MCP or skill catalog and passed Linux contract tests
  plus the disposable Context7 runtime above.
- Codex CLI 0.150.1 introduced a mandatory sibling
  `codex-code-mode-host` executable and a new structured MCP event envelope.
  The protected runtime now binds and verifies that sibling and recognizes only
  a matched started/completed MCP call plus the exact final marker.
- The pinned Cursor launcher also requires `bash`, `basename`, `dirname`, and
  `realpath`. They are now explicit digest-bound members of its existing bundle,
  and every Cursor runtime receives that complete closure on its fixed `PATH`.
- Final protected profiles still cannot be represented as 15/15 until the four
  consent-bound upstream Context7 and Notion rows for Cursor and Kiro pass.

## 4. Competitive decision

Fresh research on 2026-08-27 found one direct competitor:

- [`plugins.sh`](https://plugins.sh) implements Agent Plugins 1.0 search,
  install, update, outdated,
  remove, client registration, and a public index reporting more than 2,000
  packages through its [public API](https://plugins.sh/api/v1/plugins?limit=3&offset=0&sort=newest).
- Its npm metadata points to a GitHub repository that is currently not publicly
  resolvable.
- Fresh installs send a pseudonymous count unless users opt out.

Universal Agent Plugins must not claim to be the first universal package
manager. Its defensible position is:

> Open-source, auditable multi-client lifecycle management for Agent Plugins
> 1.0, with explicit targets, no telemetry, transactional recovery, deliberate
> source switching, and verifiable provenance.

### 4.1 Discovery architecture options

#### Option A - Signed static Discovery Index plus reviewed Directory

Recommended.

```text
Confidence: 10/10
Reliability: 9/10
Complexity: 6/10
Approximate changes: 1,300-2,500 lines
```

- The reviewed Directory remains the only authority for short names, reviewed
  defaults, evidence, suspensions, and release policies.
- A separate generated Discovery Index lists conformant public packages found
  at immutable GitHub commits.
- Discovery records are clearly labelled `unreviewed`; signature means the
  index is authentic, not that package code is endorsed.
- The site and CLI search both layers.
- A discovered result uses a publisher-qualified index slug that resolves to an
  exact owner/repository/SHA/path tuple. It never silently becomes a reviewed
  short-name default.
- Existing signing, static hosting, cache, and fail-closed code are reused.

This provides broad coverage without a database or another online service.

#### Option B - Database and hosted registry API now

```text
Confidence: 7/10
Reliability: 8/10
Complexity: 9/10
Approximate changes: 4,000-8,000 lines plus ongoing operations
```

This improves server-side search and future analytics, but creates migrations,
availability, backups, abuse controls, and a new security boundary before real
usage requires them. Defer it until static index size or update latency becomes
a measured problem.

#### Option C - Keep only the reviewed PR Directory

```text
Confidence: 5/10
Reliability: 10/10
Complexity: 2/10
Approximate changes: 200-500 lines
```

This is safest but leaves the product materially behind competitors in
discovery. It is not selected.

### 4.2 Selected model

```text
GitHub discovery
  -> bounded manifest acquisition at full commit SHA
  -> schema and package validation without execution
  -> signed static Discovery Index (unreviewed)
  -> website and `agentplugins search`
  -> publisher-qualified install resolved to an exact-SHA direct source

Maintainer/community PR
  -> reviewed Directory
  -> reviewed short name and default distribution
  -> release policy and exact evidence
```

The two layers may contain the same package. Identity and status must remain
explicit rather than deduplicating away the trust difference.

## 5. Phase 0 - Restore one exact baseline

### Summary

Make every following result refer to the same repository and release identity.

### Steps

1. Work from current protected `main` in both repositories.
2. Freeze the launch tuple:
   - catalog repository commit;
   - CLI repository commit;
   - `agentplugins-v0.1.18` tag;
   - npm package version and integrity;
   - release manifest and checksum digest;
   - Directory source digest;
   - scenario contract digest.
3. Replace stable-launch pins and fixtures with `0.1.18` only after verifying
   its released assets. Do not predict checksums or npm integrity before the
   release exists.
4. Do not regenerate historical evidence in place. New evidence gets a new
   immutable identity.

### Tests

- Reject tag/commit/checksum/npm integrity mismatch.
- Reject mixed fixtures from two CLI releases.
- Confirm all six native artifacts belong to the same release manifest.

### Acceptance criteria

- One machine-readable launch tuple identifies only `0.1.18`.
- Docs, fixture directories, workflow inputs, and npm assertions agree.

## 6. Phase 1 - Repair the scheduled and protected gates

### Summary

Make CI report product truth instead of dependency setup failures.

### Steps

1. Add exact pinned Python dependencies to both scheduled jobs in
   `live-e2e.yml` before importing shared launch modules.
2. Add a workflow contract test proving every Python-importing job installs the
   dependencies required by its imports.
3. Keep scheduled read-only observation separate from protected runtime
   evidence.
4. Run the scheduled fixture contract manually at the exact `main` commit.
   Completed on `8934d258ecc1ef15d05b5bc8ef075e5dffd902cc` in
   [run 33142162465](https://github.com/777genius/universal-agent-plugins/actions/runs/33142162465).
5. Do not mark a skipped protected gate as success.

### Edge cases

- dependency registry temporarily unavailable;
- scheduled workflow runs without publication inputs;
- public Directory endpoint not yet deployed;
- rerun belongs to a different workflow attempt;
- protected observer variables are absent or empty.

### Acceptance criteria

- The public Live E2E badge is green on current `main`.
- Scheduled fixture validation succeeds without secrets. Completed in
  [run 33142162465](https://github.com/777genius/universal-agent-plugins/actions/runs/33142162465).
- Missing protected evidence still fails closed with a precise reason.

## 7. Phase 2 - Complete current-release runtime E2E

### Summary

Prove behavior in disposable environments, never in a real user project.

### Required matrix

The fixed hero packages are:

1. Agent Code Navigator;
2. Context7;
3. Cloudflare Docs;
4. Chrome DevTools;
5. Notion.

They must complete add, update, discovery, runtime, and remove in:

- Codex;
- Cursor;
- Kiro.

This produces 15 required client/package runtime results. Repair is injected
once per client adapter. Notion authentication remains a separate human-consent
artifact bound to the same package and client result.

### Execution rules

- Use only disposable homes, workspaces, projects, browser profiles, and vendor
  test identities.
- Use the hosted observer/runtime where available; do not use a real local
  project to reduce setup time.
- Run exactly once after an unknown billing or provider result; inspect effects
  before any retry.
- Capture client version, package tree digest, manifest digest, source revision,
  command trace, native discovery, runtime marker, cleanup, and final state.
- A prepared folder or copied file is not runtime success.
- A successful runtime call does not imply OAuth success.

### ChatGPT boundary

Retain the separately scoped Cloudflare Docs result:

- exact registered development binding;
- Plugins UI discovery;
- user-attested personal-app activation;
- one read-only runtime call;
- exact `.app.json` linkage.

Do not claim general ChatGPT installation, ChatGPT Work package activation, or
package-routed runtime until separately proved.

### Failure handling

- Retry only the minimal unproved idempotent phase after a transient 5xx.
- Do not rerun an already proven exact-SHA client/package tuple.
- A client version change invalidates only evidence that depends on that client.
- A package or manifest digest change invalidates only evidence for that package.
- Authentication evidence may be reused only when its binding, identity scope,
  package digest, and vendor state remain exact.

### Acceptance criteria

- 15/15 results pass for the `0.1.18` launch tuple.
- Notion has three separately bound authentication/runtime results.
- Cleanup proves no artifacts remain outside owned test roots.
- The public evidence bundle contains no credentials, home paths, account IDs,
  cookies, raw OAuth tokens, or unsanitized prompts.

## 8. Phase 3 - Publish the signed production Directory

### Summary

Promote the already staged design into a publicly consumable product.

### Steps

1. Restore observer verification inputs only in the protected
   `stable-launch-e2e` environment.
2. Run the exact protected launch gate against `0.1.18` evidence.
3. Sign and append the next Directory sequence.
4. Materialize the static site from that exact signed snapshot.
5. Deploy only after snapshot identity, source commit, and ledger commit match.
6. Verify the public `latest.json`, envelope, snapshot, and site assets.
7. Leave signing and publisher credentials restricted to protected
   environments; never expose them to pull-request jobs.

### Required client checks

- cold fetch and signature verification;
- warm cache;
- offline last-known-good cache;
- tampered pointer, envelope, and snapshot;
- expired snapshot;
- rollback sequence;
- unknown key and key rotation overlap;
- interrupted cache write;
- unavailable production origin.

### Rollback

- Never republish an older sequence as newest.
- Suspend a bad distribution in a higher signed sequence.
- Keep direct exact-SHA installs and the embedded last-known-good snapshot
  available if remote Directory updates are disabled.

### Acceptance criteria

- The production Directory endpoint returns `200` with a valid signature.
- A clean `0.1.18` CLI resolves a reviewed short name from production.
- Offline/tampered tests prove fail-closed behavior without losing installed
  state.

## 9. Phase 4 - Add competitive CLI discovery commands

### Summary

Close the user-visible lifecycle gaps without changing the installer engine.

### Commands

1. Add `install` as a documented alias of `add`; retain `add` forever.
2. Add read-only `search <query>` with filters for:
   - reviewed/unreviewed;
   - components;
   - compatible clients;
   - authentication requirement;
   - source owner.
3. Add read-only `validate <local-or-full-sha-source>`.
4. Add `outdated [name]` and `outdated --all`.
5. Add `update --all` with a complete dry-run plan before mutation.
6. Keep `repair` and deliberate `switch` as explicit advantages.

### UX rules

- Non-interactive install requires explicit `--target`.
- Interactive TTY may show a multi-select, but the final plan must be displayed
  and confirmed by the user's command selection.
- Search never downloads or executes plugin dependencies.
- Unreviewed search results always display owner, repository, path, exact SHA,
  conformance status, and the absence of runtime review.
- Static conformance never implies ChatGPT compatibility. An unreviewed
  Discovery record excludes ChatGPT until a reviewed Directory release binds a
  registered app ID to the exact package and MCP endpoint.
- Reviewed packages may use their reserved short name. Unreviewed packages must
  use the canonical publisher-qualified slug returned by the signed index.
- If the Discovery Index is unavailable, a user can still install the same
  package through the full `owner/repository@SHA//path` direct locator.
- `update --all` preserves each installation's recorded distribution and never
  silently changes source.
- `outdated` compares release identity, not only SemVer.

### Tests

- command aliases produce identical plans and state;
- deterministic search ordering;
- duplicate and ambiguous names;
- partial target compatibility;
- mixed outdated/current/revoked installs;
- all-update preflight failure produces zero mutation;
- JSON output contracts and exit codes;
- non-TTY behavior never blocks on an invisible prompt.

### Acceptance criteria

- A new user can search, validate, install, list, inspect, check updates, update,
  repair, switch, and remove with one CLI.
- Existing scripts using `add` remain unchanged.

## 10. Phase 5 - Build the static Discovery Index

### Summary

Index public Agent Plugins 1.0 packages at scale without turning discovery into
endorsement or operating a database.

### Record contract

Each record contains only bounded, derived metadata:

- canonical lowercase owner/repository identity;
- normalized package path;
- exact 40-character commit SHA;
- manifest name, version, description, author, and license;
- Agent Plugins schema version;
- component counts and MCP transports;
- validation status and deterministic package digest;
- repository stars and last public update as informational metadata;
- first-seen and last-seen timestamps;
- reviewed Directory distribution ID when an exact match exists.

Do not ingest secrets, README bodies, package code, issue content, user data, or
installation telemetry.

### Discovery sources

1. Authenticated GitHub code search for exact published Agent Plugins schema
   URLs and canonical manifest shapes.
2. Repositories submitted through the existing PR journey.
3. Official Agent Plugins organization examples and explicitly configured seed
   repositories.

`plugins.sh` may be sampled for competitive coverage measurement, but it is not
an ingestion dependency and its records are not copied into our index.

### Discovery spike before implementation

Before committing to the scale estimate:

1. Run the exact GitHub search queries and record query text, result counts,
   rate-limit cost, duplicate rate, and how many candidates validate.
2. Sample 100 current `plugins.sh` records only for coverage comparison. Measure
   live repositories, duplicate package paths, schema validity, and overlap with
   our queries; do not import the sample.
3. Prove a deterministic partition strategy using only documented GitHub search
   qualifiers. A partition is complete only when it stays below GitHub's
   per-query result cap.
4. If a partition still exceeds the cap, mark the scan incomplete and preserve
   the previous index. Never infer completeness from the first 1,000 results.
5. Update the line estimate before implementing the crawler if the spike shows
   that another discovery source is required.

### Pipeline

1. Add `.github/workflows/discovery-index.yml`. Its scan job uses only the
   GitHub-provided read-only job token to search public repositories; it receives
   no private-repository, signing, or publication credential. Do not reuse a
   maintainer's broad CLI token or the Directory publisher credential. Run four
   triggers:
   - every six hours, refresh default-head identities and availability for
     already indexed repositories;
   - once per day, discover new candidate `plugin.json` packages;
   - once per week, perform a complete reconciliation for renamed, transferred,
     archived, deleted, or previously missed repositories;
   - `workflow_dispatch`, for a bounded maintainer-requested refresh or recovery.
   A merged reviewed submission triggers the smallest affected-source refresh
   through the normal protected publication path instead of waiting for cron.
2. Partition search queries deterministically using supported qualifiers when
   GitHub's 1,000-result search cap is reached. Persist the partition manifest
   so the same scan can be reproduced.
3. Resolve repository default head to a full commit SHA.
4. Acquire only candidate package paths with existing sparse-fetch safeguards.
5. Validate without running package code, scripts, containers, or dependencies.
6. Deduplicate by case-normalized repository plus normalized package path.
7. Generate a canonical static index and compact search projection.
8. Keep acquisition and publication in separate environments. The scan job runs
   in `discovery-read`; only the later `discovery-publication` job receives the
   Discovery-specific Ed25519 key and narrowly scoped publisher credential.
   Neither scheduled environment requires a human reviewer, but both are
   restricted to the default branch. The manually reviewed Directory continues
   to use its existing protected publication boundary. Discovery has a separate
   artifact schema, key ID, and signature context from the reviewed Directory.
9. Publish immutable versioned snapshots plus a small `discovery/latest.json`
   pointer. Replace the pointer only after the complete snapshot, envelope,
   signature, and compact search projection validate.
10. Retain the last-known-good index on partial scans, rate limits, invalid
    signatures, or publication failure. Scheduled jobs update the generated
    static artifacts; they do not mutate a database.
11. A scheduled run publishes only immutable files and then atomically advances
    `discovery/latest.json`. It never edits reviewed short-name mappings and
    never replaces the last-known-good pointer with an incomplete snapshot.

### Trust states

- `reviewed`: exact release exists in the reviewed Directory.
- `conformant_unreviewed`: schema and package structure passed static checks.
- `invalid`: never published as installable; retained only in private job
  diagnostics.
- `unavailable`: previously indexed source can no longer be acquired; search may
  show it as unavailable, but it cannot be newly installed.

Avoid the word `verified` for schema-only validation.

### Scale target

- Index every conformant public package returned by reproducible discovery
  queries.
- Target at least 2,000 unique conformant package paths for competitive launch,
  but do not pad the number with duplicates, invalid manifests, stale branches,
  or multiple aliases for one package.
- If fewer than 2,000 valid packages are discoverable, publish the measured
  coverage and validation breakdown rather than weakening validation.
- A single static index is acceptable until its compressed payload exceeds
  10 MB or measured client search latency exceeds 500 ms. Only then consider
  sharding or a service.

### Edge cases

- multiple `plugin.json` packages in one monorepo;
- repository rename, transfer, archive, deletion, or visibility change;
- case-only owner/repository differences;
- duplicate manifest names across owners;
- mutable branch results changing between search and acquisition;
- draft or unknown schema versions;
- symlinks, path traversal, LFS, submodules, sparse files, and oversized trees;
- missing or incompatible license metadata;
- rate-limit exhaustion and partial search windows;
- source disappears after installation;
- Discovery Index is newer than the reviewed Directory;
- a reviewed distribution is suspended while its discovery record remains
  structurally conformant.

### Acceptance criteria

- Search results are reproducible from exact source and query identities.
- A partial discovery run never replaces a complete last-known-good index.
- A discovered package installs by one publisher-qualified command; the CLI
  records and reports the exact resolved SHA before mutation.
- Reviewed and unreviewed results cannot be confused in CLI or website output.
- Compromise or unavailability of the competitor API has no effect on us.

## 11. Phase 6 - Website integration

### Steps

1. Search reviewed Directory and Discovery Index from one input. The application
   bundle does not hardcode the Discovery Index: it reads the published
   `discovery/latest.json` pointer and corresponding immutable search projection
   from static hosting. A newly published index therefore appears on the site
   without rebuilding or redeploying the frontend bundle.
2. Use HTTP `ETag`/conditional requests and a last-known-good browser cache. A
   normal page load never invokes GitHub search or waits for a discovery crawl.
3. Display the index generation time and a visible stale/unavailable state. If
   discovery cannot be loaded or verified, keep the reviewed Directory usable
   and do not generate install commands from untrusted discovery bytes.
4. Default ranking order:
   - reviewed exact match;
   - reviewed prefix match;
   - conformant unreviewed exact match;
   - remaining deterministic text relevance;
   - stars only as a final informational tie-breaker.
5. Add visible filters for trust state, component, client compatibility, auth,
   and owner.
6. Generate the exact install command with a client multi-select.
7. Show provenance, immutable commit, schema, component summary, and trust state
   before the copy button.
8. Keep `Add a plugin` visible and link to the PR submission path.

### Accessibility and browser E2E

- keyboard-complete combobox and multi-select;
- screen-reader labels and live result count;
- focus restoration after closing popovers;
- responsive desktop/mobile layout;
- no native select regressions;
- copy command, filters, empty state, invalid URL, unavailable package, and
  reviewed/unreviewed badge tests;
- reduced-motion support.

### Acceptance criteria

- A first-time user can find a package, understand its trust level, choose one
  or several clients, and copy one correct command without reading docs.
- A successfully published index update becomes visible without rebuilding the
  application bundle, while failed or partial refreshes leave the previous
  complete index active.
- The site remains fully static and deployable from signed artifacts.

## 12. Phase 7 - First upstream PR cohort

### Targets

Start with the three existing bridges:

1. `ChromeDevTools/chrome-devtools-mcp`;
2. `cloudflare/mcp-server-cloudflare`;
3. `github/github-mcp-server`.

### PR contract

- Title and purpose: `Add Agent Plugins 1.0 package`.
- Add the smallest upstream-owned standard package accepted by that repository.
- Do not present the PR as advertising our installer.
- Include the official Agent Plugins specification and explain package contents.
- Mention our CLI only as one tested consumer among the clients used for E2E.
- Record exact fork head SHA and package digest.
- Test the PR package from the fork commit in disposable Codex, Cursor, and Kiro
  homes before opening the PR.
- State exactly which install, discovery, runtime, auth, and cleanup outcomes
  passed; do not collapse them into `works everywhere`.

### Before merge

- The existing community bridge remains available and keeps its provenance.
- No upstream label is shown until `plugin.json` physically exists in the
  upstream owner's repository.
- New users keep the reviewed bridge default unless a separately reviewed
  Directory promotion changes it.

### After merge

1. Compare the merged package digest with the reviewed PR head.
2. Re-run package eligibility and isolated installation checks.
3. Open a manual Directory promotion PR adding the upstream distribution.
4. Keep existing installations bound to their recorded bridge.
5. Enable scheduled merge observation only after this flow succeeds once.

### Acceptance criteria

- Three real upstream PRs are open with public, exact-SHA evidence.
- At least one merge can be promoted without changing existing installations.

Current cohort:

- [Chrome DevTools #2623](https://github.com/ChromeDevTools/chrome-devtools-mcp/pull/2623)
  at fork head `4ff0c06eab3c2e7af216b24a9350867d5dccb4c8`;
- [Cloudflare #465](https://github.com/cloudflare/mcp-server-cloudflare/pull/465)
  at fork head `3897acdb389b453205c559e2b79acc8bd5909bfa`;
- [GitHub MCP Server #3169](https://github.com/github/github-mcp-server/pull/3169)
  at fork head `14092de9940741511d224e0d7071ac02910bda11`.

All three exact heads passed schema validation plus isolated add, info, and
remove lifecycle checks for Codex, Cursor, and Kiro in
[run 33115759932](https://github.com/777genius/universal-agent-plugins/actions/runs/33115759932).
That evidence does not claim tool runtime or authentication.

## 13. Verification gates

### Repository validation

Run focused checks first, then the existing complete validation workflows:

```bash
python3 scripts/build_bridges.py check
python3 scripts/build_registry.py --check
python3 -m unittest tests.test_build_bridges tests.test_build_registry
python3 -m unittest tests.test_workflow_contracts tests.test_run_launch_evidence_e2e
```

For the website:

```bash
pnpm --dir site test
pnpm --dir site generate
pnpm --dir site test:e2e
```

For the CLI engine, run its pinned Go unit/contract suites and npm facade smoke
tests in `plugin-kit-ai`, followed by the six-platform release workflow.

### Required public checks

- current-main validation workflow green;
- scheduled Live E2E green;
- protected launch evidence green for exact `0.1.18` tuple;
- signed Directory production endpoint returns and verifies;
- site loads the same signed sequence;
- npm clean install resolves the matching binary and checksum;
- one reviewed short-name install;
- one community bridge install;
- one exact-SHA discovered install;
- one real three-target lifecycle;
- external fork contributor submission;
- three upstream packaging PRs.

## 14. Rollout and kill switches

1. Fix CI and complete runtime evidence before changing public discovery.
2. Publish the signed reviewed Directory.
3. Ship CLI discovery commands behind read-only behavior first.
4. Publish Discovery Index in shadow mode and compare site/CLI results.
5. Enable exact-SHA installation from discovery results.
6. Open the first upstream PR cohort.

Kill switches:

- keep the last-known-good Discovery Index and stop scheduled replacement;
- disable Discovery Index resolution while retaining reviewed Directory and
  direct-source installation;
- suspend or revoke a reviewed release in a higher signed Directory sequence;
- disable scheduled upstream observation without disabling manual promotion;
- roll back the npm facade before any incompatible state write.

## 15. Final acceptance checklist

- [x] Stable launch tuple is entirely `0.1.18`.
- [x] Scheduled credential-free E2E is green on the recorded checkpoint.
- [ ] Protected E2E is green for the exact final evidence tuple.
- [ ] Five heroes pass 15/15 across Codex, Cursor, and Kiro.
- [ ] Notion evidence is separately authenticated and sanitized.
- [ ] Cloudflare Docs ChatGPT claim stays within the proved personal-app scope.
- [x] Signed production Directory sequence 13 is publicly reachable and
      fail-closed.
- [ ] The current staged sequence 16 is promoted only after protected evidence
      passes.
- [x] `install`, `search`, `validate`, `outdated`, and `update --all` are
      shipped.
- [x] Reviewed Directory and unreviewed Discovery Index are visibly distinct
      in labels, provenance text, commands, and browser-tested cards.
- [x] Production Discovery sequence 3 covers 2,437 reproducibly found,
      conformant package paths without padding and verifies from public assets.
- [x] Public `0.1.21` installs one exact-SHA discovered package across explicit
      Codex, Cursor, and Kiro targets from one acquisition.
- [x] Public `0.1.21` passes add, info, no-change update, external Codex remove,
      manager remove, and final doctor in a disposable sandbox.
- [x] Malformed unreviewed packages fail before mutation.
- [x] Website search and client multi-select pass 16 desktop/mobile browser E2E
      cases in isolated profiles.
- [x] The production site consumes sequence 3, finds the tested external
      selector, and emits no browser console errors.
- [x] No install telemetry, analytics, or tracking scripts are introduced.
- [x] Three upstream Agent Plugins 1.0 PRs are open with exact-SHA lifecycle
      evidence.
- [ ] Existing installs never change source implicitly.
- [x] No real user project or identity was used for the public Discovery and
      lifecycle E2E recorded above.

## 16. Estimate

```text
Confidence: 9/10
Reliability: 9/10
Complexity: 7/10
```

Expected changes:

- E2E workflow and `0.1.18` closure: 100-400 lines plus external runs;
- CLI parity commands and tests: 700-1,400 lines;
- static Discovery Index pipeline and contracts: 1,000-2,000 lines;
- website integration and browser tests: 300-700 lines;
- first three upstream packages and evidence: 300-900 lines excluding generated
  dependency lockfiles.

Total: approximately 2,400-5,400 materially changed lines, with no lifecycle
rewrite and no database service.

## 17. Implementation order

```text
Protected 0.1.18 evidence baseline + public CLI 0.1.19
  -> scheduled CI repair
  -> 15/15 runtime evidence
  -> signed production Directory
  -> install/search/validate/outdated/update-all CLI UX
  -> signed static Discovery Index
  -> website integration
  -> three upstream packaging PRs
  -> first manual bridge-to-upstream promotion
```

The first four steps are the launch blocker. Discovery and upstream PR work may
be prepared in parallel, but broad promotion begins only after the public badge,
Directory endpoint, and exact current-release evidence are green.
