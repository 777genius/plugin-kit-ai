---
name: code-architecture-map
description: Build a compact architecture map of a repository. Use when entering a large codebase, locating entry points and domains, identifying risky modules, or planning a multi-file change.
license: Apache-2.0
compatibility: Requires a local code workspace. Uses rg first; Semble and CodeGraphContext are optional.
---

# Code Architecture Map

Produce a compact map of entry points, core domains, adapters, data flows,
hotspots, verification points, and recommended next reads.

1. Read repository instructions and primary manifests.
2. Use `rg --files` to identify languages and major folders.
3. Locate exact entry-point names with `rg`.
4. Use Semble for semantic entry points by feature or domain.
5. Use CGC stats and complexity only when a suitable index is available.
6. Read source files that confirm every important claim.
7. Report architecture boundaries, risks, and the smallest useful next set of
   files.

Examples:

```bash
rg --files | sed -n '1,160p'
uvx --from semble==0.5.4 semble search -k 10 "main application flow" .
cgc stats . --context "$(basename "$PWD")"
cgc analyze complexity --limit 25 --context "$(basename "$PWD")"
```

Do not paste a giant file tree. Do not treat a semantic shortlist or graph as
complete until source reads confirm it. Prefer decisions, boundaries, risks,
and next reads over inventory noise.
