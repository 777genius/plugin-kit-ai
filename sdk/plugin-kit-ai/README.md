# plugin-kit-ai SDK

Module: `github.com/777genius/plugin-kit-ai/sdk`

Normal consumption path:

```bash
go get github.com/777genius/plugin-kit-ai/sdk@v1.0.3
```

The canonical release contract for this subdirectory module is:

- root release tag: `vX.Y.Z`
- SDK module tag from the same commit: `sdk/vX.Y.Z`

The SDK exposes a platform-neutral runtime core with platform-specific public registrars.

Current contract status in this source tree: the root SDK plus the approved Claude/Codex stable event set shipped as `public-stable` in `v1.0.0`. Additional officially supported runtime surfaces remain `public-beta`. Event-level support claims come from [../../docs/generated/support_matrix.md](../../docs/generated/support_matrix.md). Compatibility policy lives in [STABILITY.md](./STABILITY.md).

`plugin-kit-ai.Supported()` returns runtime-event metadata only. Stable Claude/Codex runtime paths are production-ready within the declared contract; runtime-supported beta hooks remain outside that promise until promoted.

## Public API

Root package:

- `plugin-kit-ai.New(plugin-kit-ai.Config)`
- `(*plugin-kit-ai.App).Use(...)`
- `(*plugin-kit-ai.App).Claude()`
- `(*plugin-kit-ai.App).Codex()`
- `(*plugin-kit-ai.App).Run()`
- `(*plugin-kit-ai.App).RunContext(ctx)`
- `plugin-kit-ai.Supported()`

Platform packages:

- `github.com/777genius/plugin-kit-ai/sdk/claude`
- `github.com/777genius/plugin-kit-ai/sdk/codex`

## Runtime Contract Boundary

- Production-ready stable runtime paths:
  - `claude/Stop`
  - `claude/PreToolUse`
  - `claude/UserPromptSubmit`
  - `codex/Notify`
- Runtime-supported but not stable:
- `claude/SessionStart` (`public-beta`)
- `claude/SessionEnd` (`public-beta`)
- `claude/Notification` (`public-beta`)
- `claude/PostToolUse` (`public-beta`)
- `claude/PostToolUseFailure` (`public-beta`)
- `claude/PermissionRequest` (`public-beta`)
- `claude/SubagentStart` (`public-beta`)
- `claude/SubagentStop` (`public-beta`)
- `claude/PreCompact` (`public-beta`)
- `claude/Setup` (`public-beta`)
- `claude/TeammateIdle` (`public-beta`)
- `claude/TaskCompleted` (`public-beta`)
- `claude/ConfigChange` (`public-beta`)
- `claude/WorktreeCreate` (`public-beta`)
- `claude/WorktreeRemove` (`public-beta`)

Generated support matrix: [../../docs/generated/support_matrix.md](../../docs/generated/support_matrix.md)

## Experimental Custom Claude Hooks

When upstream `plugin-kit-ai` support lags behind a new Claude hook, plugin projects can register a local typed hook without falling back to raw `map[string]any` handlers:

```go
type TeamHeartbeat struct {
	HookEventName string `json:"hook_event_name"`
	Message       string `json:"message"`
}

err := claude.RegisterCustomContextJSON(app.Claude(), "TeamHeartbeat", func(e *TeamHeartbeat) *claude.ContextResponse {
	return &claude.ContextResponse{AdditionalContext: "seen"}
})
```

This extension path is `public-experimental`: typed and usable, but outside the stable compatibility promise.

Codex has a matching experimental escape hatch for future argv-JSON hooks:

```go
type TaskEvent struct {
	Client string `json:"client"`
	Task   string `json:"task"`
}

err := codex.RegisterCustomJSON(app.Codex(), "task_event", func(e *TaskEvent) *codex.Response {
	return codex.Continue()
})
```

## Generation

Runtime/scaffold/validate registries are generated from descriptor definitions.

```bash
go run ./cmd/plugin-kit-ai-gen
```

## Claude Example

```go
package main

import (
	"os"

	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"
	"github.com/777genius/plugin-kit-ai/sdk/claude"
)

func main() {
	app := pluginkitai.New(pluginkitai.Config{Name: "claude-demo"})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response {
		return claude.Allow()
	})
	os.Exit(app.Run())
}
```

## Codex Example

```go
package main

import (
	"os"

	pluginkitai "github.com/777genius/plugin-kit-ai/sdk"
	"github.com/777genius/plugin-kit-ai/sdk/codex"
)

func main() {
	app := pluginkitai.New(pluginkitai.Config{Name: "codex-demo"})
	app.Codex().OnNotify(func(*codex.NotifyEvent) *codex.Response {
		return codex.Continue()
	})
	os.Exit(app.Run())
}
```
