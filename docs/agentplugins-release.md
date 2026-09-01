# Agentplugins stable release

This runbook is for maintainers. Do not create a release without explicit owner
approval for that exact version. This repository must not publish the npm facade
or change its `latest` dist-tag.

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
- `agentplugins-release` requires a reviewer and allows deployment only from
  `main`. Dispatch it from the exact `main` commit referenced by the release
  tag; its source-commit guard rejects a newer or older workflow revision.
- Select the required `binary-only` producer mode. This repository publishes
  only the verified GitHub binary release; npm publication belongs to the UAP
  facade release process outside this repository.

For the first catalog release, merge its catalog PR with history preserved. If
it is squash-merged or rebase-merged, regenerate the catalog from the resulting
`main` commit, review the new digests, and resync this repository before either
project is tagged.

## GitHub assets

1. Create the approved annotated stable tag on current `main` and push only that
   tag.
2. Dispatch `Agentplugins Release Assets` with the exact tag and the required
   `binary-only` producer mode.
3. Review the frozen commit and approve the `agentplugins-release`
   environment deployment.
4. Confirm the workflow attests all six binaries, `checksums.txt`, and
   `release-manifest.json` before creating the non-public draft.
5. Confirm the read-only platform-proof workflow receives the same-run frozen
   asset artifact and succeeds on all six native targets. For a draft it cold
   bootstraps the npm launcher from the exact local asset only after matching
   the embedded filename, size, and SHA-256; no GitHub token reaches the
   launcher or released binary. It then proves a warm-cache invocation with
   the local proof source removed. Until the matrix aggregates successfully,
   no public GitHub Release may exist.
6. After all six proofs are green, approve the promotion deployment. Confirm
   it reverifies the draft identity, manifest, assets, and attestations.
   It promotes that exact draft only after all six native platform proofs succeed.
7. Verify the resulting public release contains six platform binaries,
   `checksums.txt`, and `release-manifest.json`, and verify GitHub attestations.
   This exact stable public release is the immutable producer handoff consumed
   by the separately owned UAP npm facade workflow.
   A standalone public-release platform proof continues to prove the normal
   anonymous GitHub release download without changing publication ownership.

The workflow rejects every existing public release. A rerun accepts an existing
draft only when its tag, frozen commit, draft status, complete asset set, and
every asset byte exactly match the rebuilt release; it never uploads over an
existing asset. A failed native proof intentionally leaves that exact draft
non-public. Fix the proof defect and rerun the same tag to resume. If draft
creation itself was interrupted and left an incomplete or non-matching draft,
an owner must verify that it was never public, delete only that draft release
(not the tag), and rerun. Never edit or replace assets in place.

For a schema-v1 historical platform audit, dispatch `Agentplugins Platform
Proof` with `--ref <exact-tag>` and `allow_legacy_manifest=true`. The workflow
must already exist at that tag, and its source SHA must equal the audited tag
commit; current `main` is never allowed to impersonate a historical harness.

## Bootstrap publication record

`universal-agent-plugins@0.1.1` completed the one-time bootstrap publication.
Its exact registry tarball, provenance, clean-project lifecycle, and attestation
were verified before trusted publishing was enabled. The short-lived bootstrap
credential was then removed from both GitHub and npm, and package access was
changed to disallow bypass-2FA tokens.

Do not add a bootstrap token back to the workflow. If the package disappears
from npm, the registry preflight must fail closed instead of attempting to
recreate it.

## Producer cutover and historical npm audits

For this and later cutover releases, plugin-kit-ai remains the binary producer.
The release workflow still builds, attests, draft-proves, and promotes the same
six assets plus `checksums.txt` and `release-manifest.json`; it does not publish
an npm package. The npm tarball created inside the platform proof is an ephemeral
test input only and is never a release asset or registry publication artifact.

`.github/workflows/agentplugins-npm-publish.yml` is retained at its historical
path as a read-only audit workflow. It has no npm publishing job, protected npm
environment, OIDC write permission, token, or publish-ready gate. Its
`verify_only` input defaults to `true`; explicitly setting it to `false` fails
closed. Dispatch it with `verify_only=true` and an existing historical tag to
verify the public registry version, signature, provenance, and isolated
lifecycle. Historical audit mode does not require the tag to point to current
`main` and does not rerun the schema-v2 six-platform release proof.

The UAP facade publisher must consume an exact public tag and verify the release
manifest version, source commit, six filenames, byte sizes, SHA-256 digests, and
GitHub artifact attestations before publication. Publisher implementation and
npm authority are intentionally outside this repository.

Never publish an empty placeholder, reuse a tag, overwrite release assets, or
resolve a binary through an unpinned GitHub `latest` release.
