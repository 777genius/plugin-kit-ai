# Agentplugins 0.1.0 release

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
   `release-manifest.json` before creating the public release.
5. Verify the release contains six platform binaries, `checksums.txt`, and
   `release-manifest.json`.
6. Verify GitHub attestations before allowing npm publication.

The workflow refuses an existing release instead of replacing immutable
assets.

## First npm publication

The first publication cannot use npm trusted publishing because the package
does not exist yet.

1. Add a short-lived granular `NPM_TOKEN` only to the protected
   `npm-agentplugins` environment.
2. Set `NPM_AGENTPLUGINS_PUBLISH_READY=true` only after the GitHub assets pass
   verification.
3. Dispatch `Agentplugins NPM Publish` with the exact tag and
   `bootstrap_publish=true`.
4. Review the exact tag and approve the `npm-agentplugins` deployment.
5. Confirm the registry preflight succeeds. Only an npm `E404` is accepted as
   proof that the package does not exist; network, authentication, and other
   registry errors fail closed.
6. Confirm the workflow stages the exact npm package, verifies all embedded
   platform hashes, runs its tests, and exercises the verified release binary.
7. Confirm the post-publish clean-project smoke runs the exact registry version
   through `add`, `info`, read-only `doctor`, no-change `update`, `remove`, and
   final absent-state verification in an isolated HOME. It must prove the
   `latest` dist-tag resolves to the exact published version and JSON output
   contains no absolute runner paths.
8. Immediately set `NPM_AGENTPLUGINS_PUBLISH_READY=false` and remove the
   bootstrap token.
9. Configure npm trusted publishing for repository
   `777genius/plugin-kit-ai` and workflow
   `.github/workflows/agentplugins-npm-publish.yml`.

## Later stable publications

Use the same immutable asset workflow, temporarily open the publish-ready gate,
then dispatch the npm workflow with `bootstrap_publish=false`. The workflow must
publish through GitHub OIDC with provenance and must pass the exact-version
registry smoke before the gate is returned to `false`.

Never publish an empty placeholder, reuse a tag, overwrite release assets, or
resolve a binary through an unpinned GitHub `latest` release.
