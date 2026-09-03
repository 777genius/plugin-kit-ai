# Universal Agent Plugins: repository split, E2E, and launch plan

Status: corrected implementation plan, reviewed 2026-09-03

This document is the execution contract for the repository rename and the remaining
end-to-end work. It supersedes the older plan that still described 0.1.18 and
Directory sequence 13.

This is a gated implementation plan, not an instruction to rename repositories
immediately. The rename is the final cutover step and may start only after the
preparation, compatibility, and rollback checks below are green.

## 1. Final decision

The two public repositories have different jobs:

| Repository | Public role | What lives there |
| --- | --- | --- |
| 777genius/universal-agent-plugins | User-facing product | Go CLI engine, lifecycle, adapters, SDK, canonical npm facade, releases, product README |
| 777genius/universal-agent-plugins-registry | Community catalog | 26 reviewed packages, bridges, signed Directory and Discovery Index, site, submission workflow |

The current repositories will be renamed in place. GitHub's documented redirects
cover repository web links and Git clone/fetch/push operations; repository IDs,
issues, stars, forks, tags, releases, and commit history must be measured in the
baseline and verified again after each rename:

~~~text
777genius/plugin-kit-ai
  -> 777genius/universal-agent-plugins

777genius/universal-agent-plugins
  -> 777genius/universal-agent-plugins-registry
~~~

The rename is staged. GitHub redirects normal repository and Git traffic, but
project-site URLs are not automatically redirected and Action `uses:` references
to an action in a renamed repository fail. Reusing the old name also ends the old-name
redirect, so compatibility must be prepared before the second rename. See the
official GitHub rename rules:
https://docs.github.com/en/repositories/creating-and-managing-repositories/renaming-a-repository

### Non-negotiable invariants

- plugin.json remains the installed Agent Plugins 1.0 package authority.
- plugin.yaml is legacy authoring input only; it is never installed as part of an
  Agent Plugin package and cannot override plugin.json.
- Agent Plugins 1.0 is the production contract. Agent Plugins 1.1 is parse-only
  experimental coverage until its specification is stable; it is not advertised
  as production support.
- universal-agent-plugins is the only active publisher of the npm package with
  that name. The installed binary remains agentplugins.
- plugin-kit-ai remains the Go module path for this migration. Do not change
  go.mod or Go imports in the rename release.
  The module path is an API contract; see the [Go Modules Reference](https://go.dev/ref/mod#go-mod-file).
- plugin-kit-ai and plugin-kit-ai-runtime remain separate legacy npm packages.
- The registry is the only writer of reviewed Directory and Discovery data.
- Directory and Discovery are optional discovery services, not package
  requirements. Local paths and immutable Git selectors must keep working when
  either service is unavailable.
- A compatibility mirror is a verified byte-for-byte cache of signed feed files,
  never a second registry and never an independently editable source of truth.
- A short name resolves only to a reviewed Directory default. Discovery results
  always use a publisher-qualified exact-SHA selector.
- Targets are explicit. A command may target several selected clients
  (--target codex,cursor), but the CLI never silently installs everywhere.
- Existing add, info, update, repair, switch, outdated, validate, search, and
  remove lifecycle and rollback semantics remain intact.
- Schema validity, installation, activation, OAuth, and runtime/tool health are
  separate evidence states; a green schema or install check never implies a
  runtime or OAuth pass.
- No OAuth, telemetry, package execution, Docker, browser, or system-package
  installation is implicit.
- Multi-target planning is deterministic and complete before mutation. If one
  selected client fails, the CLI reports the failing target and rolls back targets
  already changed, subject to the existing journal/recovery contract.

## 2. Corrections to the previous plan

1. The old 0.1.18 and sequence-13 statements are historical, not the current
   baseline. A read-only check on 2026-09-03 observed package 0.1.35, Directory
   sequence 27 with 26 products, and Discovery sequence 25 with 2,676 complete
   records. These are dated observations only; all pointers, counts, and digests
   are re-read at cutover and none are predicted.
2. A Pages mirror alone does not preserve old installations. After the old
   repository name is reused, an old source such as
   777genius/universal-agent-plugins@SHA//plugins/context7 would resolve to the
   CLI repository. Required old registry commits must therefore remain reachable
   in the renamed CLI repository through a read-only compatibility history branch
   or tags, and the old CLI must be tested against it.
3. Downstream Action changes cannot be merged before the target repository exists.
   Prepare them first, then merge them immediately after the second rename.
4. A registry source can be upstream or a community bridge. Those states must be
   explicit in the signed record so a bridge is never presented as an upstream
   package by accident.

## 3. Options

### Option A - staged in-place rename with compatibility history and mirror (selected)

🎯 10/10   🛡️ 9/10   🧠 7/10
Approximate meaningful changes: 2,500-4,500 lines plus generated artifacts.

This delivers the requested public identity while retaining repository IDs and
history. The unavoidable risk is a short cutover window for Pages, Actions, and
external links.

### Option B - keep current names and create a new CLI repository

🎯 8/10   🛡️ 10/10   🧠 5/10
Approximate changes: 1,500-3,000 lines.

Safer redirects, but it leaves two similarly named products and does not deliver
the requested primary repository name.

### Option C - move the complete engine into a new monorepo

🎯 6/10   🛡️ 7/10   🧠 10/10
Approximate changes: 10,000-20,000 lines.

Unnecessary migration of the Go module, legacy generators, package workflows, and
external consumers. Rejected for this launch.

## 4. Phase 0 - immutable baseline and freeze

### Steps

1. Use fresh shallow clones for inspection and clean implementation worktrees;
   never use a dirty checkout as authoritative state.
2. Record a secret-free migration manifest with repository IDs, old and target
   names, exact main SHAs, tags, releases, branch protection, environments,
   Pages source/custom domains, Apps, webhooks, deploy keys, Action paths,
   npm metadata/trusted publisher, and active workflow runs.
3. Record the exact Directory and Discovery pointers, signed sequences,
   source commits, snapshot/envelope/search digests, signing key IDs, and every
   package source repository/SHA/path needed by existing installations.
4. Confirm universal-agent-plugins-registry is still unused immediately before
   the first rename. Do not create a placeholder repository.
5. Freeze release and Directory/Discovery publication dispatches for the short
   rename window. Keep ordinary PR validation enabled.
6. Audit open PRs and Dependabot branches. Refresh or close failed stale updates
   after the new identity is stable; never merge them as part of cutover.
7. Check hosting capacity. Current hosts are above the 80% disk safety threshold,
   so GitHub-hosted CI and lightweight disposable local tests are the default.
   Do not create another VM, LXC, snapshot, or delete foreign data.

Store the complete secret-free manifest as a short-lived private CI/release
artifact and retain its SHA-256 plus a redacted checklist in the migration issue.
Never commit tokens, cookies, private paths, or account identifiers. The artifact
must be sufficient to reproduce every pre/post comparison without exposing
credentials.

### Acceptance

- The manifest is reproducible without secrets.
- No active writer can publish against a moving identity.
- Every old package source required for compatibility has an exact SHA and path.

## 5. Phase 1 - prepare the CLI repository before rename

Work in plugin-kit-ai while it still has its current name. Keep changes in focused
PRs under roughly 2,000 changed lines.

### Canonical npm facade

- Make npm/agentplugins the sole source for the universal-agent-plugins package.
- Port the current hardened facade from the registry package with attribution and
  a source-commit note; remove the stale duplicate only after smoke tests pass.
- Keep plugin-kit-ai and plugin-kit-ai-runtime package names and workflows.
- Keep package name universal-agent-plugins, binary agentplugins, and the one-command UX:

~~~bash
npx universal-agent-plugins add context7 --target cursor
~~~

### Product README and metadata

Adapt the clear structure and language of the current catalog README; do not copy
catalog-only claims literally. The first screen explains installer/lifecycle
manager, then shows one command, multi-target examples, supported clients,
update/repair/remove, troubleshooting, and a registry link.

Update description, topics, badges, issue/security links, npm repository/homepage/
bugs fields, and all active product links. State that client activation and OAuth
remain client-specific and that a 2,500+ Discovery count is not runtime proof.

### Standard loader and conformance boundary

- Keep the lossless Agent Plugins 1.0 package model (`plugin.json`, `mcp.json`,
  skills, and supported components) separate from client adapters.
- Run a pinned Agent Plugins 1.0 conformance corpus in CI for every loader change.
  Keep the corpus revision and pass/fail totals in the evidence artifact; do not
  publish a claim to an external discussion without explicit maintainer approval.
- A CI-only Agent Plugins 1.1 inspection lane may report how a working-draft
  fixture would be handled. The production loader continues to reject 1.1 and
  the lane must not change production resolution, signing, or short-name defaults
  until the 1.1 specification is published and separately reviewed.

### Release and Pages preparation

- Replace hard-coded release URLs with GITHUB_REPOSITORY or a checked repository-ID
  allowlist accepting both names only during migration.
- Keep release tag prefixes and legacy package workflows stable.
- Make site base path and repository URL environment-driven.
- Add the compatibility feed job described in Phase 3, but do not deploy it until
  a post-rename signed registry snapshot has been verified.
- Keep signing and npm credentials out of pull-request jobs.

### Preserve old registry Git objects

Before the registry name is reused:

1. Fetch registry main and any non-main refs containing every old package source
   SHA and the current npm provenance source commit.
2. Push those objects into the future CLI repository under a protected,
   read-only branch such as compat/registry-history-before-rename.
3. Add immutable compatibility tags only for source SHAs not reachable from that
   branch. This branch is never a build or publication input.
4. Verify from the public repository that every old repository@SHA//path can be
   fetched and that its plugin.json tree digest matches the recorded Directory
   release.

This preserves source addressability, not every old GitHub web URL. Historical
PR, issue, Action, and release links may still need updating after name reuse.

## 6. Phase 2 - prepare the registry repository

Work in the current universal-agent-plugins repository.

- Change active configs, schemas, site metadata, badges, submission docs, and
  workflow self-checks to 777genius/universal-agent-plugins-registry.
- Change canonical Pages URL to
  https://777genius.github.io/universal-agent-plugins-registry/.
- Update package source metadata for packages physically kept in this repository.
  Regenerate records only in a new signed sequence after cutover.
- Keep the root README about the community directory and link to the CLI.
- Disable the registry-side npm publisher after the engine facade is verified;
  never leave two workflows able to publish the same package.
- Keep the static generated Discovery index and last-known-good pointer. Do not
  add a database or API service in this phase.

### Registry source model and precedence

- `upstream` means the pinned upstream repository commit physically contains the
  package's standard files. A `community-bridge` is a generated
  plugin.json/mcp.json package built from a pinned upstream commit while an
  upstream packaging PR is pending. A bridge is not a fork of the runtime source.
- A bridge records its upstream repository, exact commit, package path, generated
  file digests, and attribution. The registry owns the generated bridge metadata;
  it does not silently copy or execute upstream code.
- For a reviewed short name, an `upstream` record wins over a bridge only in a
  newly signed Directory sequence after the upstream files and tests are verified.
  The previous bridge sequence remains immutable. If two records disagree on an
  ID, path, or digest, publication fails closed and requires an explicit mapping
  decision; there is no arbitrary last-write-wins behavior.
- Discovery never receives short-name precedence. It always exposes a
  publisher-qualified repository, exact commit, and package path.

Historical snapshots, envelopes, ledger tags, release assets, attestations, and
evidence are immutable. They are never rewritten just to change a URL.

## 7. Phase 3 - compatibility contract

### Old CLI feed paths

The current 0.1.35 binary defaults to:

~~~text
https://777genius.github.io/universal-agent-plugins/registry/schemas/1/
https://777genius.github.io/universal-agent-plugins/discovery/
~~~

After the second rename those paths belong to the CLI Pages site. It must publish
the exact signed Directory and Discovery trees at those paths, including pointers,
snapshots, envelopes, keys, and search projections. The root HTML may be the CLI
product page; JSON paths remain compatibility assets.

### New CLI defaults

The renamed CLI release changes production defaults to:

~~~text
https://777genius.github.io/universal-agent-plugins-registry/registry/schemas/1/
https://777genius.github.io/universal-agent-plugins-registry/discovery/
~~~

AGENTPLUGINS_DIRECTORY_ORIGIN and AGENTPLUGINS_DISCOVERY_ORIGIN remain available
for tests and private mirrors. Production defaults are never changed by a remote
response.

### Mirror rules

- The registry is the only writer. After a successful registry Pages publication,
  its workflow sends a `repository_dispatch` event containing the exact source
  commit and Directory/Discovery sequences. A scheduled or manual reconciliation
  job is a fallback for a lost event; it never advances a mirror from an
  unverified or older source.
- The dispatch payload is only a wake-up hint. The mirror job re-fetches public
  pointers and verifies their signatures and digests; it never trusts a payload
  to identify or authorize bytes, and it never dispatches back to the registry.
- The job fetches the public pointer, verifies signature, sequence, key ID,
  source commit, and digest, then copies bytes without reserialization.
- It stages the complete candidate tree, validates every referenced object, and
  swaps the Pages artifact only after all checks pass. Invalid, incomplete,
  expired, or unverifiable candidates abort before Pages deployment and leave the
  previous mirror active.
- The mirror job has no signing key and cannot alter reviewed mappings.
- The active mirrored sequence and source commit are recorded in a small
  machine-readable marker so a delayed event cannot overwrite a newer mirror.
- Capture the first compatibility mirror before the second rename and test it
  immediately after cutover.

Preserved: old feed paths, old exact source SHAs/paths, installed state and
recorded distribution, and normal Git clone/fetch/push redirects while GitHub
still provides them.

Not promised: old web routes after name reuse, automatic source switching, or
historical provenance rewritten as if it came from the new repository ID.

## 8. Phase 4 - cutover sequence

Merge the CLI and registry preparation PRs, require green checks, and capture
their exact merge SHAs before starting either rename. The rename window has no
open migration patch that still changes release identity.

The two rename calls are independent. Stop on any mismatch.

### Step 1 - rename the registry

~~~text
universal-agent-plugins -> universal-agent-plugins-registry
~~~

Poll until the original repository ID is visible under the new name. Verify
branch, PRs, issues, environments, Pages, releases, tags, and the new Pages URL.
Publish no new sequence until these checks pass.

### Step 2 - rename the CLI engine

~~~text
plugin-kit-ai -> universal-agent-plugins
~~~

Verify the original engine repository ID, Go source, tags/releases, legacy
packages, Action path, environments, and Pages settings. Dispatch the prepared
compatibility Pages build.

### Step 3 - update consumers

GitHub does not redirect `uses:` calls to an action in a renamed repository. Merge
the prepared updates in every internal consumer found by the Phase 0 manifest
and a fresh `rg`/GitHub audit (13 was the initial observation, not a fixed
allowlist) from
777genius/plugin-kit-ai/... to 777genius/universal-agent-plugins/.... Keep the
action directory name setup-plugin-kit-ai for now; renaming that path is a
separate breaking-change decision. Warn external Action consumers in release
notes.

### Step 4 - restore publication

After both repositories and Pages are verified:

1. Rebind the single npm trusted-publisher configuration for
   `universal-agent-plugins` from the old repository/workflow to the renamed
   repository and exact workflow/environment, then read it back and verify the
   OIDC workflow identity before publishing. Do not delete the old configuration
   first and leave a gap; npm allows one trusted publisher per package and the
   existing configuration can be edited in place. See
   https://docs.npmjs.com/trusted-publishers/.
2. Verify Apps, webhooks, deploy keys, and environment references.
3. Re-enable registry publication and Discovery schedules.

## 9. Phase 5 - release and signed data

### CLI release 0.1.36

0.1.35 remains immutable. Release 0.1.36 from the renamed CLI repository with new
defaults and metadata.

Required evidence:

- one exact source commit for all six native artifacts;
- checksums and release manifest match every downloaded artifact;
- npm tarball contains the canonical facade and has trusted-publisher provenance;
- clean-project bootstrap on supported macOS, Linux, and Windows;
- add -> info -> update -> repair -> remove in a disposable sandbox for explicit
  codex,cursor,kiro targets;
- no real user project, OAuth token, cookie, or private identity.
- one deliberate multi-target failure in a disposable home proving no partial
  client state remains after rollback.

### New signed sequences

From the renamed registry:

1. Append a new signed Directory sequence; do not edit historical sequence 27.
2. Generate the next complete Discovery sequence from the exact registry main;
   preserve the previous pointer on a partial scan.
3. Deploy Pages and verify pointers, signatures, keys, source commit, and digests.
4. Refresh the CLI compatibility mirror from those exact public bytes.

### Old/new compatibility E2E

In fresh disposable homes and projects:

| Binary | Expected origin | Required proof |
| --- | --- | --- |
| universal-agent-plugins@0.1.35 | Old CLI paths on the CLI mirror | Resolve reviewed package, fetch old exact-SHA source, add/info/remove, no state leak |
| universal-agent-plugins@0.1.36 | New registry Pages URL | Resolve reviewed and discovered selectors, add/info/update/remove, exact source and state |

Also test cold/warm/offline cache, tampered pointer/envelope/snapshot, unknown
key, expired snapshot, rollback sequence, incomplete Discovery retention,
explicit source switching, old SHA reachability, search without package execution,
and protection against turning unreviewed results into short-name defaults. Test
bridge-to-upstream promotion as a new signed sequence and verify that historical
bridge bytes remain unchanged.

## 10. E2E and worker policy

- Heavy checks use GitHub Actions where possible so the user's Mac is not loaded.
- Hosted subscription workers are used only after a fresh machine-id,
  memory/swap, and disk check shows the single existing sandbox is safe. No new
  VM or snapshot. Each parallel job gets an isolated worktree and ownership.
- Implementation uses gpt-5.6-sol medium reasoning. xhigh is reserved for
  architecture planning and independent exact-head review.
- An unknown provider or billing result is inspected once before retry. Retry
  only the smallest idempotent phase after a transient failure.
- Schema validation, prepared files, and native projection are not runtime
  passes. Runtime requires discovery, a bounded read-only call where applicable,
  and cleanup.
- Kiro ACP 2.20.0 capability probes are recorded separately; capability probes do not replace the five-result launch matrix. Their sequence is not asserted until the ordered external evidence exists.
- ChatGPT remains a separately scoped client claim; Cloudflare Docs evidence does
  not generalize to all packages or ChatGPT Work.

## 11. User-facing cleanup

### CLI repository

- Description begins with: Secure CLI to install, update, repair, and remove
  Agent Plugins 1.0 across your AI agents.
- README begins with one-command Quick Start, multi-target examples, supported
  client matrix, lifecycle commands, troubleshooting, and registry link.
- Let GitHub naturally identify Go as the primary language; do not add generated
  Python just to influence the classifier.
- Explain one command for one or several selected agents, not implicit all-agent
  installation.

### Registry repository

- README and site describe community catalog, reviewed versus unreviewed status,
  PR submission, and the CLI link.
- Show trust status before stars. Stars are informational popularity, not quality
  or runtime proof.
- Keep generated index and site here; do not duplicate the Go engine or npm facade.
- Label every package card and install result as `upstream` or `community bridge`
  when the distinction is relevant; never use popularity stars as a trust signal.

## 12. Security, rollback, and edge cases

Security controls:

- Signing and npm credentials are environment-scoped and unavailable to PR jobs.
- Repository-ID and workflow checks accept both names only in the migration window,
  then narrow to the new identity.
- Compatibility history is immutable/read-only and excluded from builds.
- Every install records repository, revision, package path, tree digest, and
  distribution identity before mutation.
- Discovery never executes package code, scripts, containers, dependencies, or MCP.
- A bridge generator may read upstream files, but it never executes upstream
  install scripts or dependencies during indexing.

Rollback:

- Before the second rename, revert preparation PRs normally.
- Between renames, stop and repair the exact failed step; do not release.
- After both renames, prefer workflow/package reverts and publication kill switches
  over renaming back. Reusing names again can destroy redirects and worsen an outage.

Kill switches:

- stop Directory/Discovery pointer advancement while keeping the last complete
  signed snapshot;
- disable Discovery resolution while reviewed Directory and direct exact-SHA
  installs continue;
- stop the mirror job without deleting its last good assets;
- pause npm publication and keep the last immutable package;
- suspend a bad distribution in a higher signed sequence.

Required explicit handling:

- GitHub API eventual consistency or an occupied target name;
- one Pages deploy succeeding while the other fails;
- old exact SHA not reachable after compatibility import;
- npm publisher still bound to the old repository/workflow;
- uses: consumers failing after rename;
- unsigned, expired, mixed, or stale mirror;
- repository rename/transfer/archive changing a Discovery record;
- duplicate names across reviewed and unreviewed layers;
- partial publication or stale latest.json;
- bridge/upstream identity conflict or a bridge promoted without a new signed
  sequence;
- OAuth/activation failure after preparation;
- cancellation or crash during a multi-target mutation;
- old GitHub web links and old npm provenance pointing at a reused name.

## 13. Verification commands

Run against fresh clones and exact commits:

~~~bash
gh api repos/777genius/universal-agent-plugins
gh api repos/777genius/plugin-kit-ai
gh api repos/777genius/universal-agent-plugins-registry
gh run list --repo 777genius/universal-agent-plugins --limit 20
gh run list --repo 777genius/universal-agent-plugins-registry --limit 20
git ls-remote https://github.com/777genius/universal-agent-plugins.git
git ls-remote https://github.com/777genius/universal-agent-plugins-registry.git
npm view universal-agent-plugins version repository.url homepage dist-tags
npm view universal-agent-plugins@0.1.35 dist.integrity dist.attestations
npm view universal-agent-plugins@0.1.36 dist.integrity dist.attestations
~~~

For each Pages origin, verify with curl -fsS:

~~~text
registry/schemas/1/latest.json
registry/schemas/1/snapshots/<sequence>.json
registry/schemas/1/snapshots/<sequence>.envelope.json
discovery/latest.json
discovery/snapshots/<sequence>.json
discovery/search/<sequence>.json
~~~

Run focused tests before one exact-head full CI:

~~~bash
python3 scripts/build_bridges.py check
python3 scripts/build_registry.py --check
python3 -m unittest tests.test_build_bridges tests.test_build_registry
python3 -m unittest tests.test_workflow_contracts tests.test_run_launch_evidence_e2e
~~~

In the engine repository run pinned Go contract suites, npm facade smoke tests,
the six-platform release workflow, and the old/new disposable lifecycle matrix.

## 14. Acceptance checklist

- [ ] universal-agent-plugins is the CLI repository with the original engine ID/history.
- [ ] universal-agent-plugins-registry is the catalog with the original catalog ID/history.
- [ ] The canonical npm facade exists only in the CLI repository; legacy packages
      remain independently publishable.
- [ ] Go module path github.com/777genius/plugin-kit-ai still builds.
- [ ] Old registry SHAs are reachable from the renamed CLI and old 0.1.35
      exact-source install passes.
- [ ] Old and new feed paths verify the expected signed bytes.
- [ ] Local and immutable Git installs work without Directory or Discovery access.
- [ ] New 0.1.36 has six-platform artifacts, checksums, npm provenance, and
      clean-project lifecycle evidence.
- [ ] npm has one active trusted publisher for universal-agent-plugins.
- [ ] All known internal uses: consumers point to the renamed CLI repository.
- [ ] New Directory and Discovery sequences are appended, signed, complete, and
      public; historical sequences are unchanged.
- [ ] Discovery remains static, reproducible, and visibly distinct from Directory.
- [ ] Every reviewed source has an explicit upstream/bridge state and a pinned
      repository, commit, package path, and digest.
- [ ] The pinned Agent Plugins 1.0 corpus gate is green; any 1.1 lane is CI-only,
      clearly experimental, and cannot affect production resolution.
- [ ] Mirror dispatch/reconciliation is monotonic, byte-for-byte, and fail-closed.
- [ ] Reviewed/unreviewed results cannot be confused and explicit multi-target
  installation never mutates implicit clients; a failed target leaves no partial
  state after rollback.
- [ ] Pages, repository, npm, Action, and registry smoke checks are green on
      exact post-cutover commits.
- [ ] No credentials, private paths, OAuth tokens, or real user projects appear
      in evidence.

## 15. Estimate and order

Overall confidence is 🎯 9/10, reliability 🛡️ 9/10, complexity 🧠 7/10.

Expected meaningful changes: 2,500-4,500 lines:

- engine facade, README, metadata, workflow preparation: 900-1,600;
- registry identity/site/workflow preparation: 400-900;
- compatibility history, mirror, and regression tests: 600-1,200;
- consumer references, migration checks, and release tests: 600-1,000,
  excluding generated snapshots and lockfiles.

Execution order:

~~~text
baseline/freeze
  -> CLI preparation PR(s)
  -> registry preparation PR(s)
  -> compatibility history and mirror tests
  -> rename registry
  -> verify registry Pages
  -> rename CLI engine
  -> deploy compatibility Pages
  -> update Action consumers and npm trusted publisher
  -> release CLI 0.1.36
  -> append Directory/Discovery sequences
  -> old/new E2E and evidence
~~~

This is intentionally smaller than a monorepo rewrite. Existing Go engine,
lifecycle, adapters, signed feeds, and static site remain in service; only
product ownership, repository identity, package publishing, and registry
boundaries change.

## 16. Implementation checkpoint (2026-09-03)

The first production-facing vertical slice is now implemented and verified:

- Engine PR #79 (`728d066`) adds the signed Directory/Discovery compatibility
  mirror. It validates the existing signatures and trust anchors, preserves
  exact feed bytes, enforces monotonic sequences, and deploys only from a
  verified staging tree. It does not contain a signing key.
- Engine PR #80 (`e936e8d`) makes Universal Agent Plugins the user-facing
  product name while keeping the historical Go module path and browser cache
  key compatible with existing installs.
- Registry PR #217 (`72bb6d8`) makes the Discovery builder collapse exact
  duplicate identities and fail closed on conflicting duplicates. This fixed
  the production scan failure without weakening validation.
- Signed Discovery Actions run `33706680463` on exact main `72bb6d8` published
  sequence 26. Production Pages currently serve 2,836 records. Directory
  sequence 27 remains available alongside the separate Discovery feed.
- Read-only production asset checks returned HTTP 200 for both latest pointers,
  the signed snapshots, both envelopes, and the Discovery search projection.
  Local signature verification against the source-commit trust file passed:
  `verified Discovery sequence 26 with 2836 records`.
- A fresh disposable local sandbox ran the public npm
  `universal-agent-plugins@0.1.20` selector
  `discovery:0x7067/pstack` through add, info, update, and remove for explicit
  `codex,cursor,kiro` targets. The final `agentplugins search pstack` resolved
  Directory sequence 2 and Discovery sequence 26.
- A second disposable sandbox on the old hosted observer was attempted with
  the same explicit targets. Acquisition failed because that host's isolated
  Git environment intentionally removes credential helpers and its public
  GitHub egress requires authentication. This is recorded as an environment
  limitation, not bypassed by weakening source isolation; the local lifecycle
  proof remains valid.

The following claims remain intentionally separate from this checkpoint:

- no OAuth or vendor-account runtime proof was added;
- no real user project was touched;
- the repository rename/cutover remains a separately gated migration step;
- Discovery records are schema-validated and signed, but are not presented as
  manual runtime reviews or official certification.
