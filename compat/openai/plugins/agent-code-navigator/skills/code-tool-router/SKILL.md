---
name: code-tool-router
description: Choose the right code intelligence tool before searching, navigating, refactoring, or analyzing a codebase. Use for exact search, semantic discovery, symbol references, call graphs, and architecture work.
license: Apache-2.0
compatibility: Requires a local code workspace. Uses rg first; Semble, Serena, and CodeGraphContext are optional.
---

# Code Tool Router

Classify the question before searching, then use the cheapest reliable tool.

| Intent | First tool | Verification |
| --- | --- | --- |
| Exact string, config key, error text, filename, or symbol spelling | `rg` | Open matching files |
| Broad semantic discovery or similar implementations | Semble | Confirm with `rg` and source reads |
| Definitions, references, rename, or type-aware navigation | Serena or another LSP tool | Check generated and dynamic call sites with `rg` |
| Callers, callees, complexity, or dependency graph | CodeGraphContext (CGC) | Confirm important edges in source |
| Current library or API behavior | Official documentation | Compare with local pinned usage |

Exact search examples:

```bash
rg -n "reset_session\\(" .
rg --files | rg "memory|context"
```

Semantic discovery examples:

```bash
uvx --from semble==0.5.4 semble search "where session context is assembled" .
uvx --from semble==0.5.4 semble find-related src/path/file.ts 42 .
```

Graph examples:

```bash
cgc stats . --context "$(basename "$PWD")"
cgc find name reset_session --context "$(basename "$PWD")"
cgc analyze callers reset_session --context "$(basename "$PWD")"
cgc analyze complexity --limit 20 --context "$(basename "$PWD")"
```

If a specialized tool is unavailable, continue with `rg`, direct source reads,
and the language's own analysis tools. Do not claim a rename or impact set is
complete without type-aware or source-level verification.

Use semantic and graph results as shortlists, not proof. Source code and
reproducible tests remain authoritative.
