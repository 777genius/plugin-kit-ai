# Upstream promotion readiness

The manual `Upstream promotion readiness` workflow produces review input; it
does not publish, edit the Directory, open a pull request, or merge one. Its
only permission is `contents: read`. Its artifact contains bounded PR metadata,
diagnostics, and, on success, a canonical promotion-candidate JSON file.

Run it with the official repository, one positive merged pull-request number,
the package path, and an independently prepared version 3 review record. The
record binds the reviewed package digests, Directory product and manifest
name, distribution and release sequence, complete release policy, and
Directory-schema evidence records. It does not establish GitHub PR, merge, or
default-branch facts.
Each proposed target needs exactly one passed `materialization` record for the
official merged SHA, exact package tuple, and policy installer version. Evidence
must include its immutable artifact repository, commit, path, and digest.

The workflow queries the official public repository and PR with `gh`, requires
a non-draft merged PR, and writes bounded canonical `pr-metadata.json` retaining
both the historical PR base and the current default branch. It then performs a
credential-free, blob-filtered sparse clone and fetches the current default
branch plus `refs/pull/<number>/head`. Only the selected package subtree is
checked out, and its file count, individual file sizes, and total size are
bounded before materialization. The fetched PR ref must equal GitHub's full
`headRefOid`, and the merge commit must be reachable from the current default
branch.

The workflow acquires public repository refs before invoking the validator.
The validator performs no fetch and requires all objects to be present locally.
It derives reviewed and candidate revisions only from PR metadata, binds
`origin` to that repository, requires the merge commit to remain reachable from
the explicit `refs/remotes/origin/<default-branch>`, and records the PR number,
URL, merge time, reviewed head, and official merge commit.

Readiness classifies the official path as `exact`, `changed`, `moved`, or
`missing`. Only `exact` emits a candidate. A moved package is deliberately not
accepted even when its Git tree is byte-identical, and exact bytes still need
materialization evidence rebound to the official SHA. The emitted artifact is
validated against `schemas/promotion-candidate.schema.json`; a later,
human-reviewed Directory pull request remains a separate operation.

The package gate constructs the exact proposed upstream release and invokes
`build_registry.validate_release_package`, including its closed-runtime policy.
It also enforces the existing Directory product's `minimum_capabilities`.
Unversioned Agent Plugins 1.0 packages retain the Directory-compatible empty
package version; no version is invented. A new upstream distribution must use
release sequence 1. An existing upstream distribution for the same product
uses its maximum release sequence plus one; identity collisions are rejected.

Existing installations are unaffected. Source stickiness, bridge fallbacks,
and historical releases remain governed by the existing Directory data and
publication process.
