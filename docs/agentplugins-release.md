# Agentplugins beta release

This runbook is for maintainers. Do not create a release, publish npm, or change
the `latest` dist-tag without explicit owner approval for that exact version.

## Required gates

- The release tag matches `agentplugins-vX.Y.Z-beta.N` and points to current
  `main`.
- Required CI, CodeQL, dependency review, and vulnerability checks are green.
- Catalog lifecycle and runtime evidence match the release commit.
- `agentplugins-release` and `npm-agentplugins` require a reviewer and allow
  deployment only from `main`.
- `NPM_AGENTPLUGINS_PUBLISH_READY` stays `false` until the exact publish is
  approved.

## GitHub assets

1. Create the approved annotated beta tag on current `main` and push only that
   tag.
2. Dispatch `Agentplugins Release Assets` with the exact tag.
3. Review the frozen commit and approve the `agentplugins-release`
   environment deployment.
4. Verify the prerelease contains six platform binaries, `checksums.txt`, and
   `release-manifest.json`.
5. Verify GitHub attestations before allowing npm publication.

The workflow refuses an existing release instead of replacing immutable
assets.

## First npm publication

The first publication cannot use npm trusted publishing because the package
does not exist yet.

1. Add a short-lived granular `NPM_TOKEN` only to the protected
   `npm-agentplugins` environment.
2. Set `NPM_AGENTPLUGINS_PUBLISH_READY=true` only after the GitHub assets pass
   verification.
3. Dispatch `Agentplugins NPM Beta Publish` with the exact tag and
   `bootstrap_publish=true`.
4. Review the exact tag and approve the `npm-agentplugins` deployment.
5. Confirm the workflow's clean-project exact-version smoke passes.
6. Immediately set `NPM_AGENTPLUGINS_PUBLISH_READY=false` and remove the
   bootstrap token.
7. Configure npm trusted publishing for repository
   `777genius/plugin-kit-ai` and workflow
   `.github/workflows/agentplugins-npm-publish.yml`.

## Later beta publications

Use the same immutable asset workflow, temporarily open the publish-ready gate,
then dispatch the npm workflow with `bootstrap_publish=false`. The workflow must
publish through GitHub OIDC with provenance and must pass the exact-version
registry smoke before the gate is returned to `false`.

Never publish an empty placeholder, reuse a tag, overwrite release assets, fall
back to `latest`, or move the npm `latest` dist-tag during beta.
