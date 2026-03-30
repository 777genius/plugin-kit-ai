# Portable MCP Standard Proposal

Date: 2026-03-30

This document proposes the next package-standard authored format for portable MCP in `plugin-kit-ai`.

It is written as a public-facing standard proposal, not just an internal implementation note.

## Goal

Remove the maintenance pain of carrying multiple native MCP configurations across:

- Claude plugin packages
- Codex plugin packages
- Gemini extensions
- OpenCode workspace config
- Cursor workspace config

The goal is not to pretend these tools have one identical MCP runtime model.

The goal is:

- one clear authored MCP source
- one understandable stable core
- several native target projections
- explicit escape hatches where platforms differ

## Sources Reviewed

Local repository sources:

- `docs/research/claude-code-plugins/README.md`
- `docs/research/codex-cli-plugins/README.md`
- `docs/research/gemini-cli-extensions/README.md`
- `docs/CURSOR_WORKSPACE_LANE_PLAN.md`
- `examples/plugins/opencode-basic/README.md`
- `docs/generated/target_support_matrix.md`
- `cli/plugin-kit-ai/internal/pluginmanifest/manifest.go`
- `cli/plugin-kit-ai/internal/platformexec/{claude,codex,gemini,opencode,cursor}.go`

Official vendor docs reviewed during this proposal pass:

- Anthropic Claude Code MCP and plugin docs
- OpenAI Codex MCP and plugin docs
- Gemini CLI configuration and extensions docs
- OpenCode MCP docs
- current public Cursor MCP/rules surface, plus the repo’s own documented Cursor target scope

## Current State

Today portable MCP is already first-class, but still low-level.

What is already good:

- canonical authored path already exists: `mcp/servers.json|yaml|yml`
- native import already normalizes back into one portable location
- portable MCP is supported by `claude`, `codex-package`, `gemini`, `cursor`, and `opencode`
- current renderer preserves vendor-native object shapes instead of over-normalizing too early

What is still weak:

- the authored file is basically an opaque `map[string]any`
- users still think in vendor-native shapes instead of one clear package-standard contract
- validation catches incompatibilities after authoring instead of guiding authoring
- variable interpolation is still effectively vendor-shaped in examples

Current maturity:

- normalization/render/import layer: `Увер. 9/10`, `Надёж. 9/10`
- authoring UX layer: `Увер. 9/10`, `Надёж. 6/10`

## Decision Options

1. Keep raw object authoring and only improve docs. `Увер. 7/10`, `Надёж. 6/10`
This is the smallest change, but it does not actually remove multi-target authoring pain. Users still need to know too much about each vendor.

2. Invent a very broad universal MCP schema and force every target into it. `Увер. 5/10`, `Надёж. 3/10`
This looks ambitious, but it will become misleading fast because vendor surfaces already diverge on transport details, trust, OAuth, tool filtering, and variable interpolation.

3. Define a small stable core plus explicit target overrides and passthrough. `Увер. 10/10`, `Надёж. 9/10`
This gives the best balance: understandable for broad users, honest about platform differences, and durable as a public standard.

Recommended choice: option 3.

## Canonical Path And File Name

Recommended canonical authored path:

- `mcp/servers.yaml`

Why this is the best default:

- keeps `plugin.yaml` small and readable
- matches current discovery behavior
- scales if we later add neighboring files under `mcp/`
- aligns with vendor wording around `mcpServers`

Alternative names:

1. `mcp/servers.yaml`. `Увер. 10/10`, `Надёж. 9/10`
2. `mcp.yaml`. `Увер. 6/10`, `Надёж. 7/10`
3. `mcp/portable.yaml`. `Увер. 5/10`, `Надёж. 6/10`

## What Is Truly Common Across Targets

The real stable shared subset is narrower than "all MCP fields", but strong enough to be useful.

Common stable concepts:

- server alias
- enablement
- connection transport
- process launch data for stdio servers
- remote URL data for network servers
- basic process environment
- basic working directory
- basic HTTP headers
- target selection

This is the actual cross-target core.

## What Should Not Be In The Stable Core

These fields should not be force-standardized in `v1`:

- OAuth configuration
- trust / approval semantics
- Gemini-specific `includeTools` and `excludeTools`
- OpenCode-specific `type: local|remote` naming
- Cursor-native interpolation strings
- Claude-specific plugin-root variables
- Codex runtime-specific MCP config surface
- target-specific auth helpers or secret storage semantics

Reason:

- they are real, but not stable across all supported targets
- if they enter the common core too early, the standard becomes confusing and fragile

## Proposed Standard Shape

Recommended authored shape for `mcp/servers.yaml`:

```yaml
format: plugin-kit-ai/mcp
version: 1

servers:
  context7:
    description: Documentation MCP server
    enabled: true

    type: stdio
    stdio:
      command: npx
      args:
        - -y
        - "@upstash/context7-mcp"
      cwd: "${workspace.root}"
      env: {}

    targets:
      include:
        - claude
        - codex-package
        - gemini
        - opencode
        - cursor

    overrides:
      gemini:
        excludeTools:
          - "run_shell_command(rm -rf)"

    passthrough:
      opencode:
        timeout: 10000
```

## Proposed Schema

Top-level fields:

- `format`
- `version`
- `servers`

`servers.<alias>` fields:

- `description`
- `enabled`
- `type`
- `stdio`
- `remote`
- `targets`
- `overrides`
- `passthrough`

`stdio` fields:

- `command`
- `args`
- `cwd`
- `env`

`remote` fields:

- `protocol`
- `url`
- `headers`

`targets` fields:

- `include`
- `exclude`

`type` allowed values:

- `stdio`
- `remote`

`remote.protocol` allowed values when `type: remote`:

- `streamable_http`
- `sse`

Implementation note:

- `http` may be accepted as a user-friendly alias and normalized to `streamable_http`

This is the smallest clear vocabulary that still maps well to the vendor set you support.

## Why The Format Should Use A Discriminated Union

Recommended connection modeling:

1. `type: stdio|remote` plus `stdio:` / `remote:` blocks. `Увер. 10/10`, `Надёж. 9/10`
This is the clearest shape for broad users because invalid field combinations become structurally obvious.

2. generic nested `transport` object with shared fields. `Увер. 8/10`, `Надёж. 8/10`
This is workable, but slightly more abstract and less self-documenting for people who are not thinking in protocol jargon.

3. vendor-shaped raw objects only. `Увер. 4/10`, `Надёж. 4/10`
This preserves fidelity but fails as a public standard.

Recommended choice: option 1.

## Why `type + block` Is Better Than A Flat Vendor-Like Shape

After comparing the native models more carefully, the standard should distinguish:

- whether the server is launched locally or reached remotely
- which remote wire protocol it uses when remote

That leads to:

- `type: stdio | remote`
- `remote.protocol: streamable_http | sse` only for remote servers

Why this is better:

- broad users naturally think "local command" vs "remote URL" first
- OpenCode already uses a local/remote split
- Claude and Cursor expose remote transport variants explicitly
- Gemini separates remote variants by `url` vs `httpUrl`
- Codex package/runtime docs clearly support stdio and streamable HTTP, but not a broad "all remote transports are equal" promise

Recommended connection model options:

1. `type: stdio|remote` plus `stdio:` / `remote:` blocks. `Увер. 10/10`, `Надёж. 9/10`
2. single `type: stdio|http|sse`. `Увер. 7/10`, `Надёж. 7/10`
3. flat vendor-like `command|url|httpUrl` without explicit mode. `Увер. 5/10`, `Надёж. 5/10`

Recommended choice: option 1.

## Variable Standard

The authored standard should not expose vendor-native variables directly.

Recommended standard variables:

- `${package.root}`
- `${workspace.root}`
- `${env.NAME}`
- `${path.sep}`

Meaning:

- `${package.root}`: absolute root of the rendered package or workspace-owned target output
- `${workspace.root}`: current active project workspace when the target supports it
- `${env.NAME}`: environment lookup
- `${path.sep}`: platform path separator

Why this matters:

- authors should think in package-standard concepts
- renderers should translate these to vendor-native forms where needed
- examples should stop teaching vendor-specific interpolation as the portable contract

Recommended variable design:

1. package-standard variable namespace with renderer translation. `Увер. 10/10`, `Надёж. 9/10`
2. preserve vendor-native variables in the authored file. `Увер. 5/10`, `Надёж. 5/10`
3. ban variables in the authored file. `Увер. 3/10`, `Надёж. 4/10`

Recommended choice: option 1.

## Projection Rules Per Target

The portable file should project into each target like this.

### Claude

Portable projection:

- render portable MCP to `.mcp.json`
- plugin manifest points `mcpServers` to `./.mcp.json`
- `${package.root}` can project to `${CLAUDE_PLUGIN_ROOT}`

Notes:

- Claude supports package-local MCP
- Claude has plugin-specific variable semantics that should stay renderer-owned

### Codex Package

Portable projection:

- render portable MCP to `.mcp.json`
- plugin manifest points `mcpServers` to `./.mcp.json`

Notes:

- Codex package lane supports portable MCP
- Codex runtime lane does not and must stay outside this contract
- Codex docs clearly cover stdio and streamable HTTP in `config.toml`
- because the package-specific `.mcp.json` shape is documented less fully than Gemini/Claude, `v1` should keep the common core conservative

### Gemini

Portable projection:

- render portable MCP inline into `gemini-extension.json` under `mcpServers`
- `${package.root}` can project to `${extensionPath}`
- `${workspace.root}` can project to `${workspacePath}`

Notes:

- Gemini has more MCP-specific knobs than the proposed portable core
- Gemini-specific filters belong in `overrides.gemini` or `passthrough.gemini`

### OpenCode

Portable projection:

- render portable MCP into `opencode.json` under `mcp`
- map `type: stdio` into `type: local`
- map `type: remote` into `type: remote`

Notes:

- OpenCode calls this `type: local|remote`
- that naming should remain a renderer concern, not authored-standard vocabulary
- OpenCode OAuth belongs in target passthrough

### Cursor

Portable projection:

- render portable MCP to `.cursor/mcp.json`
- preserve Cursor-native string interpolation only at projection time if needed

Notes:

- Cursor support in this repo is intentionally scoped to workspace MCP plus rules plus optional `AGENTS.md`
- do not let Cursor-specific rule semantics leak into portable MCP

## Common Core Validation Rules

Package-standard validation should reject or flag:

- missing `format`
- unsupported `version`
- empty `servers`
- duplicate or invalid server aliases
- `type: stdio` without `stdio.command`
- `type: stdio` with a `remote` block
- `type: remote` without `remote.url`
- `type: remote` without `remote.protocol`
- `type: remote` with a `stdio` block
- invalid `targets.include` or `targets.exclude`
- both `include` and `exclude` producing an empty effective target set

Projection validation should additionally check:

- the target supports portable MCP at all
- the chosen transport is supported by that target
- variable usage is legal for that target
- target overrides do not conflict with generated managed keys
- target passthrough is well-typed JSON/YAML-object data

## Stable Core Field Recommendation

Recommended stable core fields for `v1`:

- `description`
- `enabled`
- `type`
- `stdio.command`
- `stdio.args`
- `stdio.cwd`
- `stdio.env`
- `remote.protocol`
- `remote.url`
- `remote.headers`
- `targets.include`
- `targets.exclude`

Field candidates that should stay out of the stable core in `v1`:

- `timeout`
- `oauth`
- `includeTools`
- `excludeTools`
- `trust`
- vendor-native root variables

Why `timeout` stays out for now:

- it is attractive, but current evidence across all five supported targets is not clean enough to call it stable and universal

## Example: Stdio Server

```yaml
format: plugin-kit-ai/mcp
version: 1

servers:
  release-checks:
    description: Release validation server
    type: stdio
    stdio:
      command: node
      args:
        - "${package.root}/bin/release-checks.mjs"
      cwd: "${workspace.root}"
      env:
        LOG_LEVEL: info
```

## Example: Remote Server

```yaml
format: plugin-kit-ai/mcp
version: 1

servers:
  docs:
    type: remote
    remote:
      protocol: streamable_http
      url: "https://example.com/mcp"
      headers:
        Authorization: "Bearer ${env.DOCS_MCP_TOKEN}"

    overrides:
      gemini:
        excludeTools:
          - "delete_docs"

    passthrough:
      opencode:
        oauth:
          clientId: "${env.DOCS_MCP_CLIENT_ID}"
          clientSecret: "${env.DOCS_MCP_CLIENT_SECRET}"
```

## Migration Plan From Current State

Recommended migration path:

1. keep accepting `mcp/servers.json`, `mcp/servers.yml`, and `mcp/servers.yaml`. `Увер. 10/10`, `Надёж. 10/10`
2. make `mcp/servers.yaml` the preferred scaffolded human-authored format. `Увер. 10/10`, `Надёж. 9/10`
3. support both current raw object shape and the new envelope during a transition period. `Увер. 9/10`, `Надёж. 9/10`
4. normalize imports into the new envelope when safe, otherwise preserve vendor-specific data in `passthrough`. `Увер. 8/10`, `Надёж. 8/10`

Transition rule:

- if the file is a raw object map, treat it as legacy `servers`
- if the file has `format` and `version`, treat it as the new authored standard

## What This Standard Should Promise Publicly

Good promise:

- write one portable MCP file
- render native MCP artifacts for supported targets
- validate where projection is safe
- use overrides and passthrough when platforms differ

Bad promise:

- one MCP schema with identical runtime semantics everywhere
- total parity of auth, trust, tool filtering, and approval behavior across vendors

## Final Recommendation

Recommended product decision:

1. standardize on `mcp/servers.yaml` as the canonical human-authored path. `Увер. 10/10`, `Надёж. 9/10`
2. introduce a package-standard envelope with `format`, `version`, and `servers`. `Увер. 9/10`, `Надёж. 9/10`
3. use a small stable core with `type: stdio|remote` and explicit `stdio:` / `remote:` blocks. `Увер. 10/10`, `Надёж. 9/10`
4. keep `plugin.yaml` lean and do not move full MCP into it. `Увер. 10/10`, `Надёж. 10/10`
5. add first-class `overrides.<target>` and `passthrough.<target>` rather than pretending every vendor field is portable. `Увер. 10/10`, `Надёж. 10/10`

If the goal is broad audience adoption, this is the right balance:

- clear enough for humans
- honest enough for serious users
- narrow enough to stay stable
- flexible enough to remove real multi-target maintenance pain
