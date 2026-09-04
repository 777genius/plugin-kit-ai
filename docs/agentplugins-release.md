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

## Homebrew tap

A successful `Agentplugins Release Assets` run triggers
`.github/workflows/agentplugins-homebrew-tap.yml`. The publisher downloads only
the exact public `release-manifest.json`, requires its stable tag to resolve to
the embedded source commit, verifies the GitHub artifact attestation, and
generates `777genius/homebrew-agentplugins/Formula/agentplugins.rb` from the six
closed-set asset identities. The formula exposes the four macOS and Linux
assets and pins every URL to its release tag and SHA-256.

`HOMEBREW_TAP_TOKEN` must have contents-write access to the tap repository and
does not need write access to this source repository. The publisher is
idempotent: an identical formula is a no-op. If automatic publication fails,
fix the publisher or token and manually dispatch the workflow with the same
immutable release tag. Do not edit a released formula by hand or repoint it to
different bytes under the same version.

## Bootstrap publication record

`universal-agent-plugins@0.1.1` completed the one-time bootstrap publication.
Its exact registry tarball, provenance, clean-project lifecycle, and attestation
were verified before trusted publishing was enabled. The short-lived bootstrap
credential was then removed from both GitHub and npm, and package access was
changed to disallow bypass-2FA tokens.

Do not add a bootstrap token back to the workflow. If the package disappears
from npm, the registry preflight must fail closed instead of attempting to
recreate it.

## Producer cutover and npm publication

For this and later releases, plugin-kit-ai remains the binary producer. The
release workflow builds, attests, draft-proves, and promotes the same six assets
plus `checksums.txt` and `release-manifest.json`. The npm facade is staged from
that exact public release and never embeds the binaries in its tarball.

`.github/workflows/agentplugins-npm-publish.yml` is the manual trusted publisher.
Dispatch it from the exact `agentplugins-vX.Y.Z` tag with `publish=true`. The
prepare job requires a public, immutable GitHub release, verifies every asset
and GitHub attestation, stages the checked-in evidence, and uploads one exact
tarball. The publish job runs in the protected `npm-agentplugins` environment,
requires GitHub OIDC (`id-token: write`), refuses `NPM_TOKEN` and
`NODE_AUTH_TOKEN`, and publishes with npm provenance. The final job verifies
public metadata, SLSA provenance, npm audit signatures, the tarball digest, and
an isolated `codex,cursor,kiro` add/info/update/remove lifecycle.

With `publish=false`, only the immutable prepare checks run; no npm state is
changed. A published version is never overwritten and a stable tag is never
reused. If the package disappears from npm, rerun only with a new release tag;
the workflow never recreates an old version or uses a bootstrap token.

Never publish an empty placeholder, reuse a tag, overwrite release assets, or
resolve a binary through an unpinned GitHub `latest` release.
