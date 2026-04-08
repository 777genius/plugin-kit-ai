# Release Notes Template

Use this template for release notes after `v1.0.0`, including post-`v1` stable-subset promotions.

Pair this with [REHEARSAL_TEMPLATE.md](./REHEARSAL_TEMPLATE.md) when collecting the actual evidence that feeds the final decision.

## Summary

- release tag:
- candidate commit SHA:
- release type:
- one-sentence user summary:

## Public-Stable In This Release

- list promoted stable surfaces
- list any post-`v1` promotion ledger reviewed, for example `INTERPRETED_STABLE_SUBSET_AUDIT.md`

## Why This Release Matters

- explain the main user-facing outcome in plain language
- say who benefits first
- avoid release-process wording in the first paragraph

## What Changed For Users

- list the main user-facing changes
- prefer concrete outcomes over internal implementation details

## What To Do Now

- list the default recommendation after this release
- list any migration or upgrade action a user should actually take

## Still Public-Beta

- list remaining beta surfaces

## Beta Contract Changes

- list beta-contract changes and whether each affected surface stays `public-beta`

## External Smoke Status

- required:
- install-compat:
- polyglot-smoke:
- generated-config/runtime-contract drift:
- version-sync-check:
- extended:
- live:
- release-preflight:
- release-assets:
- Homebrew tap:
- npm publish:
- PyPI publish:
- npm runtime-package publish:
- npm runtime-package postpublish registry smoke:
- npm runtime-package live install:
- PyPI runtime-package publish:
- PyPI runtime-package postpublish registry smoke:
- PyPI runtime-package live install:
- waivers:

## Known Limitations

- list documented limitations
- include Codex external runtime-health note if applicable

## Decision Record

- final audit outcome:
- any `stays-beta` decision:
- maintainer sign-off:
