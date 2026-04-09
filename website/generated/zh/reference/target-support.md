---
title: "Target 支持"
description: "生成的 target 支持摘要"
canonicalId: "page:reference:target-support"
surface: "reference"
section: "reference"
locale: "zh"
generated: true
editLink: false
stability: "public-stable"
maturity: "stable"
sourceRef: "docs/generated/target_support_matrix.md"
translationRequired: false
---
# Target 支持

Use this page when you need the compact lane map across runtime, package, extension, and repo-managed integration outputs.

| Target | Production Class | Runtime Contract | Install Model |
| --- | --- | --- | --- |
| claude | production-ready package+runtime lane | stable runtime subset | marketplace or local |
| codex-package | recommended package lane | official package only | marketplace or local |
| codex-runtime | recommended runtime lane | stable notify runtime | repo-local |
| cursor | repo-managed integration lane | workspace-config lane | workspace config |
| gemini | production-ready extension packaging lane | packaging, not runtime | copy install |
| opencode | repo-managed integration lane | workspace-config lane | workspace config |

For full framing, pair this matrix with [Support Boundary](/zh/reference/support-boundary) and [Target Model](/zh/concepts/target-model).
