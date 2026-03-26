# API Stability Tiers (hookplex SDK)

The SDK contract is split between `public-stable` and `internal`. Future additions remain `public-beta` until separately promoted.

The declared `v1` candidate set is tracked repo-wide in [../../docs/V0_9_AUDIT.md](../../docs/V0_9_AUDIT.md), and beta-breaking moves are recorded in [../../docs/MIGRATIONS.md](../../docs/MIGRATIONS.md).

Promotion to `public-stable` is driven only by the final audit ledger and release rehearsal evidence. A candidate surface is not stable merely because it exists or is documented.

## Public-Beta
Current SDK beta surface added after the first promotion:

- approved-export-shaped Claude event and response types for:
  - `SessionStart`
  - `SessionEnd`
  - `Notification`
  - `PostToolUse`
  - `PostToolUseFailure`
  - `PermissionRequest`
  - `SubagentStart`
  - `SubagentStop`
  - `PreCompact`
  - `Setup`
  - `TeammateIdle`
  - `TaskCompleted`
  - `ConfigChange`
  - `WorktreeCreate`
  - `WorktreeRemove`

These hooks are runtime-supported and scaffolded, but remain outside the stable compatibility promise until they are promoted through the audit ledger.

## Public-Stable
Approved stable SDK surface:

- `hookplex.New`, `hookplex.Config`, `hookplex.App`
- `(*hookplex.App).Use`
- `(*hookplex.App).Claude`
- `(*hookplex.App).Codex`
- `(*hookplex.App).Run`
- `(*hookplex.App).RunContext`
- `hookplex.Supported`
- approved exported Claude event and response types for:
  - `Stop`
  - `PreToolUse`
  - `UserPromptSubmit`
- approved exported Codex event and response types for:
  - `Notify`

The stable SDK promise covers only:

- the approved root API
- approved exported Claude event/response types
- approved exported Codex event/response types

It does not cover:

- internal packages
- generator implementation details
- generated runtime internals

## Public-Experimental

- `claude.RegisterCustomCommonJSON`
- `claude.RegisterCustomContextJSON`
- `claude.RegisterCustomPostToolUseJSON`
- `claude.RegisterCustomPermissionRequestJSON`
- `codex.RegisterCustomJSON`

These helpers are intentionally outside the stable promise. They exist to let plugin projects add typed local Claude or Codex hooks before upstream promotion.

## Internal

These areas are not part of the SDK compatibility promise:

- `sdk/hookplex/internal/...`
- generated descriptor/runtime internals under `sdk/hookplex/internal/descriptors/gen`
- repository-only generator implementation

HTTP / prompt / agent Claude hooks remain out of scope for the current shipped SDK contract.
