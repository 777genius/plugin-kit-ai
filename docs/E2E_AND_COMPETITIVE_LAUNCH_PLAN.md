# E2E and competitive launch plan

This document records the current launch closure for the Universal Agent
Plugins CLI. Implementation details belong in the workflows and test suites;
this page keeps the user-visible evidence and honest limits in one place.

## Current closure

| Area | Evidence | Result |
| --- | --- | --- |
| CLI security source | `fbc9206bcf663c8b6d3e372736335dab58b46a4b` | Direct and signed-index security checks, including canonical policy identity, merged |
| Security scanner | [`lintai v0.1.2`](https://github.com/777genius/lintai/releases/tag/v0.1.2), run `33935702746` | Agent Plugins 1.0 scan contract released for all supported platforms |
| Native release | [`agentplugins-v0.1.48`](https://github.com/777genius/universal-agent-plugins/releases/tag/agentplugins-v0.1.48), run `33953877137` | Six platform builds and native runtime proofs passed |
| npm release | [npm workflow `33954318627`](https://github.com/777genius/universal-agent-plugins/actions/runs/33954318627), attempt 2 | Trusted Publisher, provenance, signatures, and public verification passed |
| Public package | `universal-agent-plugins@0.1.48` | `latest` points to `0.1.48` |
| Lifecycle | Fresh GitHub-hosted sandbox in release CI | `add`, `info`, `update`, and `remove` passed for Codex, Cursor, and Kiro; OpenCode repair passed |
| Public signed-index canary | `npx --yes universal-agent-plugins@0.1.48 add 'discovery:upstash/context7//plugins/agent-plugins/context7' --target codex --dry-run` | Exact signed assessment accepted; no local scanner was downloaded and no client files were changed |
| Discovery | [sequence 37](https://777genius.github.io/universal-agent-plugins-registry/discovery/latest.json), run [`33950307791`](https://github.com/777genius/universal-agent-plugins-registry/actions/runs/33950307791), 3,024 records | Exact production verification passed; snapshot digest `sha256:d0dbc863741f57bbcfba51f1b00c3aabdc1dc75c0e2843433002cbcac9a7b5e1` |
| Security Index | [sequence 2](https://777genius.github.io/universal-agent-plugins-registry/security/latest.json), run [`33954246330`](https://github.com/777genius/universal-agent-plugins-registry/actions/runs/33954246330), 2,754 subjects | 2,749 exact package revisions assessed with LintAI and signed independently; 5 acquisition or scan checks unavailable; snapshot digest `sha256:955a4089fd9c9e19fbdaf34662bfa13121d0f1dc8538e9eef0dae6c256b3cb92` |
| Product site | main `aa6f627c6890963c3bafdcc15b1d5342281c097a`, [PR #144](https://github.com/777genius/universal-agent-plugins/pull/144), [Pages run `33955933096`](https://github.com/777genius/universal-agent-plugins/actions/runs/33955933096) | Signed Security sequence 2 mirrored byte-exactly; search, package pages, and exact-match labels passed Chromium E2E; 90 same-origin production assets returned without 4xx |

The npm package is published by GitHub Actions with npm Trusted Publisher
provenance. No long-lived npm token is required by the release workflow.

## Security behavior before installation

The CLI evaluates an acquired package before changing any managed client files:

1. It reuses a signed Security Index assessment only when the package tree,
   `plugin.json`, LintAI version, policy version, and policy digest all match.
2. Otherwise it runs the pinned LintAI release in the isolated staging tree.
3. The result is cached by that same exact identity. A package, scanner, or
   policy change invalidates the cache.
4. Warnings are displayed without an extra prompt. Blocking findings stop
   non-interactive installation unless `--accept-security-risk` is explicit;
   an interactive terminal asks for confirmation and defaults to no.

The signed Security Index is a separate, optional feed. An unavailable,
expired, stale, malformed, or mismatched feed cannot bypass the local scan. A
successful automated check means no configured blocking pattern was found; it
is not a guarantee that a package is safe.

## Reproduce the public checks

```bash
npm view universal-agent-plugins version dist-tags --json
npx universal-agent-plugins add context7
```

The first command confirms the public package and `latest` tag. The second is
the normal interactive path: the CLI detects compatible clients and lets the
user choose one or several. Target-specific automation can use
`--target codex,cursor,kiro`.

The exact CI evidence is available from the repositories:

- [native release run 33953877137](https://github.com/777genius/universal-agent-plugins/actions/runs/33953877137)
- [npm publish run 33954318627](https://github.com/777genius/universal-agent-plugins/actions/runs/33954318627)
- [LintAI release run 33935702746](https://github.com/777genius/lintai/actions/runs/33935702746)
- [Discovery sequence 37](https://777genius.github.io/universal-agent-plugins-registry/discovery/latest.json)
- [Security sequence 2](https://777genius.github.io/universal-agent-plugins-registry/security/latest.json)
- [public Directory](https://777genius.github.io/universal-agent-plugins-registry/)
- [public product site with the verified feed mirror](https://777genius.github.io/universal-agent-plugins/)

## Scope and honest limits

- Agent Plugins 1.0 packages are the input format; each client still has its
  own native projection, activation, and OAuth rules.
- The release workflow proves the CLI lifecycle and native binary delivery in
  isolated CI environments. It does not claim that every package works at
  runtime in every client.
- Discovery metadata supports search. It is not an endorsement or a substitute
  for package-specific runtime validation.
- LintAI reports known static patterns under a versioned policy. It does not
  execute package code and does not certify a package as safe.
- No real user project, account, OAuth consent, or private service was used by
  this security extension. Its final release, signed-index publication, and
  browser proofs ran in GitHub-hosted CI or fresh disposable local directories;
  it did not require a VM, LXC instance, or snapshot.

## Ongoing release contract

1. Keep LintAI, policy, signed assessment, and CLI evaluator identities exact.
2. Keep six-platform release and public npm lifecycle checks green for every
   versioned release.
3. Publish Security and Discovery as separately signed feeds; preserve the
   last-known-good snapshot whenever a refresh is incomplete.
4. Add client-specific runtime evidence only in disposable test surfaces and
   keep every claim package- and client-specific.
5. Track Agent Plugins 1.1 separately until its specification is stable.
