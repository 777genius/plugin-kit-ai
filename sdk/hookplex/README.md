# hookplex SDK

Module: `github.com/hookplex/hookplex/sdk`

The SDK now exposes a platform-neutral runtime core with platform-specific public registrars.

Current contract status in this source tree: approved for `public-stable` in the pending `v1.0` release. Event-level support claims come from [../../docs/generated/support_matrix.md](../../docs/generated/support_matrix.md). Compatibility policy lives in [STABILITY.md](./STABILITY.md).

## Public API

Root package:

- `hookplex.New(hookplex.Config)`
- `(*hookplex.App).Use(...)`
- `(*hookplex.App).Claude()`
- `(*hookplex.App).Codex()`
- `(*hookplex.App).Run()`
- `(*hookplex.App).RunContext(ctx)`
- `hookplex.Supported()`

Platform packages:

- `github.com/hookplex/hookplex/sdk/claude`
- `github.com/hookplex/hookplex/sdk/codex`

## Supported Runtime Events

- `claude/Stop`
- `claude/PreToolUse`
- `claude/UserPromptSubmit`
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
- `codex/Notify`

Generated support matrix: [../../docs/generated/support_matrix.md](../../docs/generated/support_matrix.md)

## Experimental Custom Claude Hooks

When upstream `hookplex` support lags behind a new Claude hook, plugin projects can register a local typed hook without falling back to raw `map[string]any` handlers:

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
go run ./cmd/hookplex-gen
```

## Claude Example

```go
package main

import (
	"os"

	hookplex "github.com/hookplex/hookplex/sdk"
	"github.com/hookplex/hookplex/sdk/claude"
)

func main() {
	app := hookplex.New(hookplex.Config{Name: "claude-demo"})
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

	hookplex "github.com/hookplex/hookplex/sdk"
	"github.com/hookplex/hookplex/sdk/codex"
)

func main() {
	app := hookplex.New(hookplex.Config{Name: "codex-demo"})
	app.Codex().OnNotify(func(*codex.NotifyEvent) *codex.Response {
		return codex.Continue()
	})
	os.Exit(app.Run())
}
```
