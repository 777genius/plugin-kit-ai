---
title: "Choosing Runtime"
description: "How to choose between Go, Python, Node, and shell authoring paths."
canonicalId: "page:concepts:choosing-runtime"
section: "concepts"
locale: "en"
generated: false
translationRequired: true
---

# Choosing Runtime

The runtime choice is not just about language preference. It changes how the plugin runs, what the execution machine must have installed, and how simple CI and handoff will be.

<MermaidDiagram
  :chart="`
flowchart TD
  Start[Need a runtime path] --> Prod{Need the strongest production path}
  Prod -->|Yes| Go[go]
  Prod -->|No| Local{Is the plugin repo local by design}
  Local -->|Yes| Team{Is the team Python first or Node first}
  Team --> Python[python]
  Team --> Node[node or node --typescript]
  Local -->|No| Escape{Need only a bounded escape hatch}
  Escape --> Shell[shell beta]
`"
/>

## Choose Go When

- you want the strongest supported path
- you want typed handlers and the cleanest production story
- you want downstream plugin users to avoid installing Python or Node
- you want the least bootstrap friction in CI and on other machines

Go is the recommended default for production-oriented plugins.

## Choose Python Or Node When

- the plugin is repo-local by design
- your team already lives in that runtime
- you accept owning runtime bootstrap yourself
- you are comfortable with Python `3.10+` or Node.js `20+` being present on the execution machine

These are supported paths for local runtime projects, but they do not remove runtime dependencies from the execution machine.

## Choose Shell Only When

- you need a bounded escape hatch
- you explicitly accept a narrower beta contract

Shell is not the recommended default path.

## Safe Default Matrix

| Situation | Recommended choice |
| --- | --- |
| Strongest production path | `go` |
| Mainstream non-Go stable path | `node --typescript` |
| Automation-heavy local team | `python` |
| Bounded beta escape hatch | `shell` |

See [Quickstart](/en/guide/quickstart) for the shortest supported setup path and [Stability Model](/en/concepts/stability-model) for the contract vocabulary.
