# Repo Docs

This directory is the canonical tracked maintainer corpus for internal process,
release, and operational documentation.

Rules:

- Treat this tree as maintainer-facing, not public-site content.
- Do not point the VitePress public site at this directory.
- Keep it versioned in git. Local scratch notes belong in untracked files
  outside this tree.

Boundary:

- Public hand-authored docs live under `website/source/`.
- Public generated docs live under `website/generated/`.
- The assembled VitePress source tree is `website/.site/`.
