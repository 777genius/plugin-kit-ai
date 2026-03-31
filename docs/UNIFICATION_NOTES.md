# Unification Notes

Date: 2026-03-30

This document records the current conclusions about what is worth unifying across Claude, Codex, Gemini, OpenCode, and Cursor in `plugin-kit-ai`, and where the current `portable MCP` contract stands today.

## Sources Reviewed

- `docs/research/claude-code-plugins/README.md`
- `docs/research/codex-cli-plugins/README.md`
- `docs/research/gemini-cli-extensions/README.md`
- `docs/CURSOR_WORKSPACE_LANE_PLAN.md`
- `examples/plugins/opencode-basic/README.md`
- `docs/SUPPORT.md`
- `docs/generated/target_support_matrix.md`

## What Is Actually Worth Unifying

The strongest real cross-target common subset is:

1. `portable MCP`
2. package identity and metadata (`name`, `version`, `description`, author/repo/homepage-style fields where relevant)
3. the authored lifecycle (`import -> normalize -> render -> validate`)
4. a higher-level concept of instruction surfaces, but not one forced file format
5. target-specific extras as explicit escape hatches

The key product point is:

- the repository should unify authoring, render/import/validate, and managed artifacts
- it should not promise that all targets share one runtime model

## What Should Not Be Force-Unified

Do not over-unify these surfaces:

- lifecycle hooks
- commands
- agents/subagents
- policies
- themes
- marketplace and install models
- runtime event semantics
- Cursor rules and OpenCode plugin code as if they were the same abstraction

These surfaces are too asymmetric across vendor ecosystems and are better represented as target-native components with clear support boundaries.

## Current Portable MCP State

`portable MCP` already exists as a first-class portable component in the current architecture.

Current authored source:

- `mcp/servers.yaml`

Current internal model:

- portable component kind: `mcp_servers`
- graph carrier: `PortableMCP`
- typed authored payload: `PortableMCPFile`
- projected native payload: target-specific JSON object maps generated from the typed file

Important implementation details:

- discovery reads `mcp/servers.yaml` as the canonical authored portable MCP file
- `plugin.yaml` does not currently contain MCP definitions; it only carries package metadata and enabled targets
- render projects the typed portable MCP model into target-native MCP artifacts
- import normalizes native target MCP back into canonical `mcp/servers.yaml`
- validation checks that the selected targets support `mcp_servers`, then applies target-specific rules

Representative code points:

- `cli/internal/pluginmodel/model.go`: `PortableMCP`
- `cli/internal/pluginmanifest/manifest.go`: `discoverMCP(...)`
- `cli/internal/validate/validate.go`: unsupported portable-kind checks
- `sdk/platformmeta/platformmeta.go`: per-target `PortableComponentKinds`

## Current Strengths Of Portable MCP

The current design is already strong in several ways:

- canonical authored path: `mcp/servers.yaml`
- the same authored MCP source is reused by Claude, Codex package, Gemini, OpenCode, and Cursor
- import normalizes divergent native MCP layouts back into one package-standard source
- the system preserves target-native strings and object fields where possible instead of over-normalizing too early
- target support is explicit through contract metadata instead of implicit convention

This means the project already has a real shared core for MCP, not just a loose idea.

## Current Weaknesses Of Portable MCP

The current design is still relatively low-level:

- the authored file is now typed, but the public contract still needs careful limits around what is truly portable versus merely projected
- target-aware ergonomics are limited
- there is little first-class guidance for expressing intent such as local stdio server vs SSE vs streamable HTTP vs target-only fields
- target-specific incompatibilities are mostly caught by validation after authoring, not guided during authoring
- `plugin.yaml` knows nothing about MCP beyond target selection, so package metadata and portable capability authoring live in separate places

In short:

- today `portable MCP` is a solid normalization and transport layer
- it is now a typed authoring layer, but still a deliberately narrow one

## Recommendation

Recommended direction:

- keep `plugin.yaml` lean as package metadata and target selection
- keep MCP in a dedicated authored file instead of embedding a large MCP object directly into `plugin.yaml`
- evolve that authored MCP file into a more ergonomic package-standard format

Recommended next-step contract:

1. keep `mcp/servers.yaml` as the canonical human-authored format
2. require the small package-standard envelope instead of raw object-only authoring
3. keep the rendered target output native and target-specific
4. preserve a target-specific passthrough area for fields that should not be force-normalized
5. validate against both package-standard rules and target projection rules

## Recommended Authored MCP Shape

The best direction is not "put every native MCP shape directly into `plugin.yaml`".

The better direction is:

- `plugin.yaml` stays small
- `mcp/servers.yaml` becomes the ergonomic portable source

Suggested package-standard shape:

```yaml
version: 1
servers:
  context7:
    transport: stdio
    command: npx
    args:
      - -y
      - "@upstash/context7-mcp"
    env: {}
    cwd: null
    enabled: true
    targets:
      claude: {}
      codex-package: {}
      gemini: {}
      opencode: {}
      cursor: {}
```

Possible later additions:

- `profiles` or `variants` for dev/prod
- `capabilities` metadata for docs and generated summaries
- `notes` for author intent
- `target_overrides` only where a target genuinely differs

## Design Principle For MCP

The right abstraction is:

- shared portable MCP intent
- native target projection
- explicit target overrides

The wrong abstraction is:

- pretending all targets consume one identical MCP schema with identical semantics

## Product Framing

If the goal is to remove the pain of maintaining several plugins, the most credible promise is:

- one authored package
- one canonical MCP source
- one render/import/validate workflow
- several native outputs

The product should not promise:

- one universal runtime model
- one universal hook model
- one universal instruction model

## Decision Snapshot

Recommended decisions for now:

1. keep `portable MCP` as the main shared cross-target core
2. do not move full MCP payloads into `plugin.yaml`
3. improve authored MCP ergonomics in `mcp/servers.yaml`
4. keep target-native escape hatches explicit
5. continue treating OpenCode and Cursor as workspace-config lanes, not evidence for a fake universal runtime abstraction

## Confidence

- `portable MCP is the strongest real cross-target abstraction`: `Увер. 10/10`, `Надёж. 10/10`
- `render/import/validate is the second strongest shared layer`: `Увер. 10/10`, `Надёж. 9/10`
- `instruction surfaces should be unified only at a higher concept level`: `Увер. 8/10`, `Надёж. 8/10`
- `forcing hooks/commands/agents/rules/tools into one common model would be a mistake`: `Увер. 9/10`, `Надёж. 9/10`
- `current portable MCP is strong as normalization, but not yet strong as authoring UX`: `Увер. 9/10`, `Надёж. 9/10`
