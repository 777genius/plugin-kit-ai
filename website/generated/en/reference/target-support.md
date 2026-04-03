---
title: "Target Support"
description: "Generated target support summary"
canonicalId: "page:reference:target-support"
surface: "reference"
section: "reference"
locale: "en"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "docs/generated/target_support_matrix.md"
translationRequired: false
---
# Target Support

Use this page when you need to quickly see which target is production-ready and which remains packaging-only or a workspace-config lane.

| Target | Production Class | Runtime Contract | Install Model |
| --- | --- | --- | --- |
| claude | production-ready | stable runtime subset | marketplace or local |
| codex-package | package lane | official package only | marketplace or local |
| codex-runtime | runtime lane | stable notify runtime | repo-local |
| gemini | production-ready extension target | packaging, not runtime | copy install |
| cursor | packaging-only | workspace-config lane | workspace config |
| opencode | packaging-only | workspace-config lane | workspace config |

For full framing, pair this matrix with [Support Boundary](/en/reference/support-boundary) and [Target Model](/en/concepts/target-model).
