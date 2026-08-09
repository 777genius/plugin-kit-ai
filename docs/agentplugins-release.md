# Agentplugins stable release

This runbook is for maintainers. Do not create a release, publish npm, or change
the `latest` dist-tag without explicit owner approval for that exact version.

## Required gates

- The release tag matches `agentplugins-vX.Y.Z` and points to current
  `main`.
- Required CI, CodeQL, dependency review, and vulnerability checks are green.
- The release workflow independently requires a merged pull request into
  `main`, successful dependency review on its head, and successful post-merge
  Required, docs, CodeQL, platform smoke, and vulnerability checks on the exact
  tagged commit. Repository settings are not treated as the release proof.
- A versioned `universal-agent-plugins` catalog release exists on its `main`,
  its package revision is an ancestor of that release commit, and its lifecycle
  and runtime evidence cover the released package trees.
- The catalog embedded in the `agentplugins` binary is byte-identical to that
  released catalog and its compiled SHA-256 pin matches.
- `agentplugins-release` and `npm-agentplugins` require a reviewer and allow
  deployment only from `main`.
- npm trusted publishing is bound to repository `777genius/plugin-kit-ai`,
  workflow `agentplugins-npm-publish.yml`, environment `npm-agentplugins`, and
  the `npm publish` permission.
- `NPM_AGENTPLUGINS_PUBLISH_READY` stays `false` until the exact publish is
  approved.

For the first catalog release, merge its catalog PR with history preserved. If
it is squash-merged or rebase-merged, regenerate the catalog from the resulting
`main` commit, review the new digests, and resync this repository before either
project is tagged.

## GitHub assets

1. Create the approved annotated stable tag on current `main` and push only that
   tag.
2. Dispatch `Agentplugins Release Assets` with the exact tag.
3. Review the frozen commit and approve the `agentplugins-release`
   environment deployment.
4. Confirm the workflow attests all six binaries, `checksums.txt`, and
   `release-manifest.json` before creating the non-public draft.
5. Confirm the authenticated platform-proof workflow downloads that exact
   draft and succeeds on all six native targets. Until the matrix aggregates
   successfully, no public GitHub Release may exist.
6. After all six proofs are green, approve the promotion deployment. Confirm
   it reverifies the draft identity, manifest, assets, and attestations.
   It promotes that exact draft only after all six native platform proofs succeed.
7. Verify the resulting public release contains six platform binaries,
   `checksums.txt`, and `release-manifest.json`, and verify GitHub attestations
   before allowing npm publication.

The workflow rejects every existing public release. A rerun accepts an existing
draft only when its tag, frozen commit, draft status, complete asset set, and
every asset byte exactly match the rebuilt release; it never uploads over an
existing asset. A failed native proof intentionally leaves that exact draft
non-public. Fix the proof defect and rerun the same tag to resume. If draft
creation itself was interrupted and left an incomplete or non-matching draft,
an owner must verify that it was never public, delete only that draft release
(not the tag), and rerun. Never edit or replace assets in place.

## Bootstrap publication record

`universal-agent-plugins@0.1.1` completed the one-time bootstrap publication.
Its exact registry tarball, provenance, clean-project lifecycle, and attestation
were verified before trusted publishing was enabled. The short-lived bootstrap
credential was then removed from both GitHub and npm, and package access was
changed to disallow bypass-2FA tokens.

Do not add a bootstrap token back to the workflow. If the package disappears
from npm, the registry preflight must fail closed instead of attempting to
recreate it.

## Later stable publications

Use the same immutable asset workflow, temporarily open the publish-ready gate,
then dispatch the npm workflow with the exact tag. Review and approve the
`npm-agentplugins` environment deployment. The workflow must publish through
GitHub OIDC with provenance and must pass the exact-version registry smoke
after an exact-version, scripts-disabled install verifies both the registry
signature and SLSA provenance attestation. Only then may it run `add`, `info`,
read-only `doctor`, no-change `update`, `remove`, and final absent-state
verification in an isolated HOME.
The publish-ready gate must be returned to `false` immediately after the publish
job finishes, regardless of the verification result. Registry verification
must still prove the `latest` dist-tag resolves to the exact published version
and that JSON output contains no absolute runner paths. It runs as a separate
job after publication, waits up to five minutes with online metadata refreshes,
and can be retried without attempting to republish an immutable npm version.
The same workflow can be dispatched with `verify_only=true` and the existing
historical tag after publication. That mode skips release identity,
schema-v2 six-platform proof, and the protected publish job entirely; it does
not require the tag to point to current `main` or require opening the
publish-ready gate, and only runs public registry, provenance, and isolated
lifecycle verification.

Never publish an empty placeholder, reuse a tag, overwrite release assets, or
resolve a binary through an unpinned GitHub `latest` release.
