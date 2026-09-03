# E2E and competitive launch plan

This document records the current launch closure for the Universal Agent
Plugins CLI. It is intentionally short: implementation details belong in the
workflow and test suites, while this page keeps the user-visible evidence in
one place.

## Current closure

| Area | Evidence | Result |
| --- | --- | --- |
| Source | `main` commit `fc6050b6664eea717e5f2366d59098fd4c6519ef` | Merged and clean |
| Native release | `agentplugins-v0.1.44`, release run `33791852212` | Six platform builds and native runtime proofs passed |
| npm release | npm workflow `33792693406` | Prepare, OIDC publish, and public verification passed |
| Public package | `universal-agent-plugins@0.1.44` | `latest` points to `0.1.44` |
| Lifecycle | Isolated synthetic package in release CI | `add`, `info`, `update`, and `remove` passed for Codex, Cursor, and Kiro |
| Discovery | Signed production snapshot, sequence 30 | `2,875` records; exact production verification passed |
| Directory site | GitHub Pages production URL | HTTP 200 and the search UI loaded successfully |
| Documentation | Root and npm READMEs | Target-free one-command quick start is published |

The npm package is published by GitHub Actions with npm Trusted Publisher
provenance. No long-lived npm token is required by the release workflow.

## Reproduce the public checks

```bash
npm view universal-agent-plugins version dist-tags --json
npx universal-agent-plugins add context7
```

The first command confirms the public package and `latest` tag. The second is
the normal interactive path: the CLI detects compatible clients and lets the
user choose one or several. Target-specific automation can use
`--target codex,cursor,kiro`.

The exact CI evidence is available from the repository's Actions history:

- [native release run 33791852212](https://github.com/777genius/universal-agent-plugins/actions/runs/33791852212)
- [npm publish run 33792693406](https://github.com/777genius/universal-agent-plugins/actions/runs/33792693406)
- [release `agentplugins-v0.1.44`](https://github.com/777genius/universal-agent-plugins/releases/tag/agentplugins-v0.1.44)
- [signed Discovery run 33791156432](https://github.com/777genius/universal-agent-plugins-registry/actions/runs/33791156432)
- [public Directory](https://777genius.github.io/universal-agent-plugins-registry/)

## Scope and honest limits

- Agent Plugins 1.0 packages are the input format; each client still has its
  own native projection, activation, and OAuth rules.
- The release workflow proves the CLI lifecycle and native binary delivery in
  isolated CI environments. It does not claim that every package works at
  runtime in every client.
- Discovery metadata is useful for search, but it is not an endorsement or a
  substitute for package-specific runtime validation.
- No real user project, account, OAuth consent, or private service is used by
  the automated lifecycle proof.

## Next bounded work

1. Keep the release and npm workflows green on every versioned release.
2. Add client-specific runtime evidence only when a disposable test surface is
   available; keep those claims package-specific.
3. Track Agent Plugins 1.1 separately until its specification is stable.
