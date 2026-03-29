# Release Notes Template

Use this template for release notes after `v1.0.0`, including post-`v1` stable-subset promotions.

Pair this with [REHEARSAL_TEMPLATE.md](./REHEARSAL_TEMPLATE.md) when collecting the actual evidence that feeds the final decision.

## Summary

- release tag:
- candidate commit SHA:
- release type:

## Public-Stable In This Release

- list promoted stable surfaces
- list any post-`v1` promotion ledger reviewed, for example `INTERPRETED_STABLE_SUBSET_AUDIT.md`

## Still Public-Beta

- list remaining beta surfaces

## Beta Contract Changes

- list beta-contract changes and whether each affected surface stays `public-beta`

## External Smoke Status

- required:
- install-compat:
- polyglot-smoke:
- generated-config/runtime-contract drift:
- extended:
- live:
- Homebrew tap:
- npm publish:
- PyPI publish:
- waivers:

## Known Limitations

- list documented limitations
- include Codex external runtime-health note if applicable

## Decision Record

- final audit outcome:
- any `stays-beta` decision:
- maintainer sign-off:
