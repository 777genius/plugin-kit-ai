---
name: code-impact-analysis
description: Analyze code impact before refactors, behavior changes, API changes, and cross-module edits. Use when missing a caller, schema consumer, route, or state dependency could break the product.
license: Apache-2.0
compatibility: Requires a local code workspace. Uses rg first; Semble, Serena, and CodeGraphContext are optional.
---

# Code Impact Analysis

Build a verified impact set before editing shared behavior or public contracts.

1. Locate the definition with an LSP tool or `rg`.
2. Find exact call sites and serialized field names with `rg`.
3. Find type-aware references with Serena or the language server.
4. Ask CGC for callers, callees, dependencies, and complexity when indexed.
5. Use Semble to find conceptually similar patterns that exact search may miss.
6. Read every high-risk caller and decide the minimum edit set.
7. Identify focused tests, type checks, migrations, and user-visible verification.

Examples:

```bash
rg -n "symbolName\\(" .
uvx --from semble==0.5.4 semble search "similar usage of symbolName" .
cgc analyze callers symbolName --context "$(basename "$PWD")"
cgc analyze calls symbolName --context "$(basename "$PWD")"
```

When tools disagree, inspect the conflicting results. Generated code, dynamic
dispatch, framework conventions, or an incomplete index can explain the gap.
Never reduce the impact set solely because a graph or semantic search returned
few results.
