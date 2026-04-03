# API Stability Tiers (plugin-kit-ai SDK)

The SDK contract is split between `public-stable` and `internal`. Future additions remain `public-beta` until separately promoted.

The declared `v1` candidate set is tracked repo-wide in [../../docs/V0_9_AUDIT.md](../../docs/V0_9_AUDIT.md). Beta-breaking moves should be called out in the changelog and release notes.

Promotion to `public-stable` is driven only by the final audit ledger and release rehearsal evidence. A candidate surface is not stable merely because it exists or is documented.

The generated support matrix and `plugin-kit-ai.Supported()` describe runtime-event metadata only. In that runtime view:

- `stable` event entries are the production-ready runtime paths
- `beta` event entries are runtime-supported but not stable

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
- approved-export-shaped Gemini event and response types for:
  - `SessionStart`
  - `SessionEnd`
  - `Notification`
  - `PreCompress`
  - `BeforeModel`
  - `AfterModel`
  - `BeforeToolSelection`
  - `BeforeAgent`
  - `AfterAgent`
  - `BeforeTool`
  - `AfterTool`
- approved exported Gemini helper constructors for the current beta lane, including:
  - lifecycle/system no-op/context helpers such as `SessionStartContinue`, `SessionStartAddContext`, `SessionEndContinue`, `NotificationContinue`, and `PreCompressContinue`
  - advisory `systemMessage` helpers such as `SessionStartMessage`, `SessionEndMessage`, `NotificationMessage`, and `PreCompressMessage`
  - model steering helpers such as `BeforeModelContinue`, `BeforeModelDeny`, `BeforeModelOverrideRequestValue`, `BeforeModelSyntheticResponseValue`, `AfterModelContinue`, `AfterModelDeny`, and `AfterModelReplaceResponseValue`
  - loop-stop helpers such as `AfterModelStop`, `BeforeAgentStop`, `AfterAgentStop`, `BeforeToolStop`, and `AfterToolStop`
  - tool-selection helpers such as `BeforeToolSelectionContinue`, `BeforeToolSelectionConfig`, `BeforeToolSelectionAllowOnly`, `BeforeToolSelectionForceAny`, `BeforeToolSelectionForceAuto`, and `BeforeToolSelectionDisableAll`
  - agent-turn helpers such as `BeforeAgentContinue`, `BeforeAgentAddContext`, `BeforeAgentDeny`, `AfterAgentContinue`, `AfterAgentDeny`, and `AfterAgentClearContext`
  - tool decision helpers such as `BeforeToolContinue`, `BeforeToolAllow`, `BeforeToolDeny`, `AfterToolContinue`, `AfterToolAllow`, and `AfterToolDeny`
  - typed JSON-object helpers such as `BeforeToolRewriteInputValue` and `AfterToolTailCallValue`
  - `AfterToolAddContext` for appending tool-result context

These hooks are runtime-supported and scaffolded, but remain outside the stable compatibility promise until they are promoted through the audit ledger.

## Public-Stable
Approved stable SDK surface:

- `plugin-kit-ai.New`, `plugin-kit-ai.Config`, `plugin-kit-ai.App`
- `(*plugin-kit-ai.App).Use`
- `(*plugin-kit-ai.App).Claude`
- `(*plugin-kit-ai.App).Codex`
- `(*plugin-kit-ai.App).Gemini`
- `(*plugin-kit-ai.App).Run`
- `(*plugin-kit-ai.App).RunContext`
- `plugin-kit-ai.Supported`
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

- `sdk/internal/...`
- generated descriptor/runtime internals under `sdk/internal/descriptors/gen`
- repository-only generator implementation

HTTP / prompt / agent Claude hooks remain out of scope for the current shipped SDK contract.
