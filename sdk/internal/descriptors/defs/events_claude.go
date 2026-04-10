package defs

import "github.com/777genius/plugin-kit-ai/sdk/internal/runtime"

func claudeEvents() []EventDescriptor {
	return []EventDescriptor{
		{
			Platform: "claude",
			Event:    "Stop",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "Stop",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityStable,
				V1Target: true,
			},
			DecodeFunc: "DecodeStop",
			EncodeFunc: "EncodeStop",
			Registrar: RegistrarMeta{
				MethodName:   "OnStop",
				EventType:    "*StopEvent",
				ResponseType: "*Response",
				WrapFunc:     "wrapStop",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-stop",
				TableGroup: "claude",
				Summary:    "Claude Stop command hook",
			},
			Capabilities: []runtime.CapabilityID{"stop_gate"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "stop_gate", Platform: "stop_gate"},
			},
		},
		{
			Platform: "claude",
			Event:    "PreToolUse",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "PreToolUse",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityStable,
				V1Target: true,
			},
			DecodeFunc: "DecodePreToolUse",
			EncodeFunc: "EncodePreToolUse",
			Registrar: RegistrarMeta{
				MethodName:   "OnPreToolUse",
				EventType:    "*PreToolUseEvent",
				ResponseType: "*PreToolResponse",
				WrapFunc:     "wrapPreToolUse",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-pretooluse",
				TableGroup: "claude",
				Summary:    "Claude PreToolUse command hook",
			},
			Capabilities: []runtime.CapabilityID{"tool_gate"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "tool_gate", Platform: "tool_gate"},
			},
		},
		{
			Platform: "claude",
			Event:    "UserPromptSubmit",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "UserPromptSubmit",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityStable,
				V1Target: true,
			},
			DecodeFunc: "DecodeUserPromptSubmit",
			EncodeFunc: "EncodeUserPromptSubmit",
			Registrar: RegistrarMeta{
				MethodName:   "OnUserPromptSubmit",
				EventType:    "*UserPromptEvent",
				ResponseType: "*UserPromptResponse",
				WrapFunc:     "wrapUserPromptSubmit",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-userpromptsubmit",
				TableGroup: "claude",
				Summary:    "Claude UserPromptSubmit command hook",
			},
			Capabilities: []runtime.CapabilityID{"prompt_submit_gate"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "prompt_submit_gate", Platform: "prompt_submit_gate"},
			},
		},
		{
			Platform: "claude",
			Event:    "SessionStart",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "SessionStart",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeSessionStart",
			EncodeFunc: "EncodeSessionStart",
			Registrar: RegistrarMeta{
				MethodName:   "OnSessionStart",
				EventType:    "*SessionStartEvent",
				ResponseType: "*SessionStartResponse",
				WrapFunc:     "wrapSessionStart",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-sessionstart",
				TableGroup: "claude",
				Summary:    "Claude SessionStart beta hook",
			},
			Capabilities: []runtime.CapabilityID{"session_start"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "session_start", Platform: "session_start"},
			},
		},
		{
			Platform: "claude",
			Event:    "SessionEnd",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "SessionEnd",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeSessionEnd",
			EncodeFunc: "EncodeSessionEnd",
			Registrar: RegistrarMeta{
				MethodName:   "OnSessionEnd",
				EventType:    "*SessionEndEvent",
				ResponseType: "*SessionEndResponse",
				WrapFunc:     "wrapSessionEnd",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-sessionend",
				TableGroup: "claude",
				Summary:    "Claude SessionEnd beta hook",
			},
			Capabilities: []runtime.CapabilityID{"session_end"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "session_end", Platform: "session_end"},
			},
		},
		{
			Platform: "claude",
			Event:    "Notification",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "Notification",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeNotification",
			EncodeFunc: "EncodeNotification",
			Registrar: RegistrarMeta{
				MethodName:   "OnNotification",
				EventType:    "*NotificationEvent",
				ResponseType: "*NotificationResponse",
				WrapFunc:     "wrapNotification",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-notification",
				TableGroup: "claude",
				Summary:    "Claude Notification beta hook",
			},
			Capabilities: []runtime.CapabilityID{"notify"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "notify", Platform: "notify"},
			},
		},
		{
			Platform: "claude",
			Event:    "PostToolUse",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "PostToolUse",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodePostToolUse",
			EncodeFunc: "EncodePostToolUse",
			Registrar: RegistrarMeta{
				MethodName:   "OnPostToolUse",
				EventType:    "*PostToolUseEvent",
				ResponseType: "*PostToolUseResponse",
				WrapFunc:     "wrapPostToolUse",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-posttooluse",
				TableGroup: "claude",
				Summary:    "Claude PostToolUse beta hook",
			},
			Capabilities: []runtime.CapabilityID{"post_tool"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "post_tool", Platform: "post_tool"},
			},
		},
		{
			Platform: "claude",
			Event:    "PostToolUseFailure",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "PostToolUseFailure",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodePostToolUseFailure",
			EncodeFunc: "EncodePostToolUseFailure",
			Registrar: RegistrarMeta{
				MethodName:   "OnPostToolUseFailure",
				EventType:    "*PostToolUseFailureEvent",
				ResponseType: "*PostToolUseFailureResponse",
				WrapFunc:     "wrapPostToolUseFailure",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-posttoolusefailure",
				TableGroup: "claude",
				Summary:    "Claude PostToolUseFailure beta hook",
			},
			Capabilities: []runtime.CapabilityID{"post_tool_failure"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "post_tool_failure", Platform: "post_tool_failure"},
			},
		},
		{
			Platform: "claude",
			Event:    "PermissionRequest",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "PermissionRequest",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodePermissionRequest",
			EncodeFunc: "EncodePermissionRequest",
			Registrar: RegistrarMeta{
				MethodName:   "OnPermissionRequest",
				EventType:    "*PermissionRequestEvent",
				ResponseType: "*PermissionRequestResponse",
				WrapFunc:     "wrapPermissionRequest",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-permissionrequest",
				TableGroup: "claude",
				Summary:    "Claude PermissionRequest beta hook",
			},
			Capabilities: []runtime.CapabilityID{"permission_request"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "permission_request", Platform: "permission_request"},
			},
		},
		{
			Platform: "claude",
			Event:    "SubagentStart",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "SubagentStart",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeSubagentStart",
			EncodeFunc: "EncodeSubagentStart",
			Registrar: RegistrarMeta{
				MethodName:   "OnSubagentStart",
				EventType:    "*SubagentStartEvent",
				ResponseType: "*SubagentStartResponse",
				WrapFunc:     "wrapSubagentStart",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-subagentstart",
				TableGroup: "claude",
				Summary:    "Claude SubagentStart beta hook",
			},
			Capabilities: []runtime.CapabilityID{"subagent_start"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "subagent_start", Platform: "subagent_start"},
			},
		},
		{
			Platform: "claude",
			Event:    "SubagentStop",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "SubagentStop",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeSubagentStop",
			EncodeFunc: "EncodeSubagentStop",
			Registrar: RegistrarMeta{
				MethodName:   "OnSubagentStop",
				EventType:    "*SubagentStopEvent",
				ResponseType: "*SubagentStopResponse",
				WrapFunc:     "wrapSubagentStop",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-subagentstop",
				TableGroup: "claude",
				Summary:    "Claude SubagentStop beta hook",
			},
			Capabilities: []runtime.CapabilityID{"subagent_stop"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "subagent_stop", Platform: "subagent_stop"},
			},
		},
		{
			Platform: "claude",
			Event:    "PreCompact",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "PreCompact",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodePreCompact",
			EncodeFunc: "EncodePreCompact",
			Registrar: RegistrarMeta{
				MethodName:   "OnPreCompact",
				EventType:    "*PreCompactEvent",
				ResponseType: "*PreCompactResponse",
				WrapFunc:     "wrapPreCompact",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-precompact",
				TableGroup: "claude",
				Summary:    "Claude PreCompact beta hook",
			},
			Capabilities: []runtime.CapabilityID{"pre_compact"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "pre_compact", Platform: "pre_compact"},
			},
		},
		{
			Platform: "claude",
			Event:    "Setup",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "Setup",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeSetup",
			EncodeFunc: "EncodeSetup",
			Registrar: RegistrarMeta{
				MethodName:   "OnSetup",
				EventType:    "*SetupEvent",
				ResponseType: "*SetupResponse",
				WrapFunc:     "wrapSetup",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-setup",
				TableGroup: "claude",
				Summary:    "Claude Setup beta hook",
			},
			Capabilities: []runtime.CapabilityID{"setup"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "setup", Platform: "setup"},
			},
		},
		{
			Platform: "claude",
			Event:    "TeammateIdle",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "TeammateIdle",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeTeammateIdle",
			EncodeFunc: "EncodeTeammateIdle",
			Registrar: RegistrarMeta{
				MethodName:   "OnTeammateIdle",
				EventType:    "*TeammateIdleEvent",
				ResponseType: "*TeammateIdleResponse",
				WrapFunc:     "wrapTeammateIdle",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-teammateidle",
				TableGroup: "claude",
				Summary:    "Claude TeammateIdle beta hook",
			},
			Capabilities: []runtime.CapabilityID{"teammate_idle"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "teammate_idle", Platform: "teammate_idle"},
			},
		},
		{
			Platform: "claude",
			Event:    "TaskCompleted",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "TaskCompleted",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeTaskCompleted",
			EncodeFunc: "EncodeTaskCompleted",
			Registrar: RegistrarMeta{
				MethodName:   "OnTaskCompleted",
				EventType:    "*TaskCompletedEvent",
				ResponseType: "*TaskCompletedResponse",
				WrapFunc:     "wrapTaskCompleted",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-taskcompleted",
				TableGroup: "claude",
				Summary:    "Claude TaskCompleted beta hook",
			},
			Capabilities: []runtime.CapabilityID{"task_completed"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "task_completed", Platform: "task_completed"},
			},
		},
		{
			Platform: "claude",
			Event:    "ConfigChange",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "ConfigChange",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeConfigChange",
			EncodeFunc: "EncodeConfigChange",
			Registrar: RegistrarMeta{
				MethodName:   "OnConfigChange",
				EventType:    "*ConfigChangeEvent",
				ResponseType: "*ConfigChangeResponse",
				WrapFunc:     "wrapConfigChange",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-configchange",
				TableGroup: "claude",
				Summary:    "Claude ConfigChange beta hook",
			},
			Capabilities: []runtime.CapabilityID{"config_change"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "config_change", Platform: "config_change"},
			},
		},
		{
			Platform: "claude",
			Event:    "WorktreeCreate",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "WorktreeCreate",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeWorktreeCreate",
			EncodeFunc: "EncodeWorktreeCreate",
			Registrar: RegistrarMeta{
				MethodName:   "OnWorktreeCreate",
				EventType:    "*WorktreeCreateEvent",
				ResponseType: "*WorktreeCreateResponse",
				WrapFunc:     "wrapWorktreeCreate",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-worktreecreate",
				TableGroup: "claude",
				Summary:    "Claude WorktreeCreate beta hook",
			},
			Capabilities: []runtime.CapabilityID{"worktree_create"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "worktree_create", Platform: "worktree_create"},
			},
		},
		{
			Platform: "claude",
			Event:    "WorktreeRemove",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommandCaseFold,
				Name: "WorktreeRemove",
			},
			Carrier: runtime.CarrierStdinJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: false,
			},
			DecodeFunc: "DecodeWorktreeRemove",
			EncodeFunc: "EncodeWorktreeRemove",
			Registrar: RegistrarMeta{
				MethodName:   "OnWorktreeRemove",
				EventType:    "*WorktreeRemoveEvent",
				ResponseType: "*WorktreeRemoveResponse",
				WrapFunc:     "wrapWorktreeRemove",
			},
			Docs: DocsMeta{
				SnippetKey: "claude-worktreeremove",
				TableGroup: "claude",
				Summary:    "Claude WorktreeRemove beta hook",
			},
			Capabilities: []runtime.CapabilityID{"worktree_remove"},
			CapabilityMappings: []CapabilityMapping{
				{Unified: "worktree_remove", Platform: "worktree_remove"},
			},
		},
	}
}
