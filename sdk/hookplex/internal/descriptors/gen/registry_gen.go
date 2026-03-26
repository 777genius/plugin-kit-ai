package gen

import (
	internal_claude "github.com/hookplex/hookplex/sdk/internal/platforms/claude"
	internal_codex "github.com/hookplex/hookplex/sdk/internal/platforms/codex"
	"github.com/hookplex/hookplex/sdk/internal/runtime"
)

type key struct {
	platform runtime.PlatformID
	event    runtime.EventID
}

var registry = map[key]runtime.Descriptor{
	{platform: "claude", event: "Stop"}: {
		Platform: "claude",
		Event:    "Stop",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeStop,
		Encode:   internal_claude.EncodeStop,
	},
	{platform: "claude", event: "PreToolUse"}: {
		Platform: "claude",
		Event:    "PreToolUse",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodePreToolUse,
		Encode:   internal_claude.EncodePreToolUse,
	},
	{platform: "claude", event: "UserPromptSubmit"}: {
		Platform: "claude",
		Event:    "UserPromptSubmit",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeUserPromptSubmit,
		Encode:   internal_claude.EncodeUserPromptSubmit,
	},
	{platform: "claude", event: "SessionStart"}: {
		Platform: "claude",
		Event:    "SessionStart",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeSessionStart,
		Encode:   internal_claude.EncodeSessionStart,
	},
	{platform: "claude", event: "SessionEnd"}: {
		Platform: "claude",
		Event:    "SessionEnd",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeSessionEnd,
		Encode:   internal_claude.EncodeSessionEnd,
	},
	{platform: "claude", event: "Notification"}: {
		Platform: "claude",
		Event:    "Notification",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeNotification,
		Encode:   internal_claude.EncodeNotification,
	},
	{platform: "claude", event: "PostToolUse"}: {
		Platform: "claude",
		Event:    "PostToolUse",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodePostToolUse,
		Encode:   internal_claude.EncodePostToolUse,
	},
	{platform: "claude", event: "PostToolUseFailure"}: {
		Platform: "claude",
		Event:    "PostToolUseFailure",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodePostToolUseFailure,
		Encode:   internal_claude.EncodePostToolUseFailure,
	},
	{platform: "claude", event: "PermissionRequest"}: {
		Platform: "claude",
		Event:    "PermissionRequest",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodePermissionRequest,
		Encode:   internal_claude.EncodePermissionRequest,
	},
	{platform: "claude", event: "SubagentStart"}: {
		Platform: "claude",
		Event:    "SubagentStart",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeSubagentStart,
		Encode:   internal_claude.EncodeSubagentStart,
	},
	{platform: "claude", event: "SubagentStop"}: {
		Platform: "claude",
		Event:    "SubagentStop",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeSubagentStop,
		Encode:   internal_claude.EncodeSubagentStop,
	},
	{platform: "claude", event: "PreCompact"}: {
		Platform: "claude",
		Event:    "PreCompact",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodePreCompact,
		Encode:   internal_claude.EncodePreCompact,
	},
	{platform: "claude", event: "Setup"}: {
		Platform: "claude",
		Event:    "Setup",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeSetup,
		Encode:   internal_claude.EncodeSetup,
	},
	{platform: "claude", event: "TeammateIdle"}: {
		Platform: "claude",
		Event:    "TeammateIdle",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeTeammateIdle,
		Encode:   internal_claude.EncodeTeammateIdle,
	},
	{platform: "claude", event: "TaskCompleted"}: {
		Platform: "claude",
		Event:    "TaskCompleted",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeTaskCompleted,
		Encode:   internal_claude.EncodeTaskCompleted,
	},
	{platform: "claude", event: "ConfigChange"}: {
		Platform: "claude",
		Event:    "ConfigChange",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeConfigChange,
		Encode:   internal_claude.EncodeConfigChange,
	},
	{platform: "claude", event: "WorktreeCreate"}: {
		Platform: "claude",
		Event:    "WorktreeCreate",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeWorktreeCreate,
		Encode:   internal_claude.EncodeWorktreeCreate,
	},
	{platform: "claude", event: "WorktreeRemove"}: {
		Platform: "claude",
		Event:    "WorktreeRemove",
		Carrier:  runtime.CarrierStdinJSON,
		Decode:   internal_claude.DecodeWorktreeRemove,
		Encode:   internal_claude.EncodeWorktreeRemove,
	},
	{platform: "codex", event: "Notify"}: {
		Platform: "codex",
		Event:    "Notify",
		Carrier:  runtime.CarrierArgvJSON,
		Decode:   internal_codex.DecodeNotify,
		Encode:   internal_codex.EncodeNotify,
	},
}

func Lookup(platform runtime.PlatformID, event runtime.EventID) (runtime.Descriptor, bool) {
	d, ok := registry[key{platform: platform, event: event}]
	return d, ok
}
