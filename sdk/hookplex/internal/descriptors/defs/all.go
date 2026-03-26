package defs

import "github.com/hookplex/hookplex/sdk/internal/runtime"

func Profiles() []PlatformProfile {
	return []PlatformProfile{
		{
			Platform:        "claude",
			Status:          runtime.StatusRuntimeSupported,
			PublicPackage:   "claude",
			InternalPackage: "claude",
			InternalImport:  "github.com/hookplex/hookplex/sdk/internal/platforms/claude",
			TransportModes:  []runtime.TransportMode{runtime.ProcessMode},
			LiveTestProfile: "claude_cli",
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					".claude-plugin/plugin.json",
					"hooks/hooks.json",
				},
				OptionalFiles: []string{
					"Makefile",
					".goreleaser.yml",
					"skills/{{.ProjectName}}/SKILL.md",
					"commands/{{.ProjectName}}.md",
				},
				ForbiddenFiles: []string{
					"AGENTS.md",
					".codex/config.toml",
				},
				TemplateFiles: []TemplateFile{
					{Path: "go.mod", Template: "go.mod.tmpl"},
					{Path: "cmd/{{.ProjectName}}/main.go", Template: "main.go.tmpl"},
					{Path: ".claude-plugin/plugin.json", Template: "plugin.json.tmpl"},
					{Path: "hooks/hooks.json", Template: "hooks.json.tmpl"},
					{Path: "README.md", Template: "README.md.tmpl"},
					{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
					{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
					{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
					{Path: "commands/{{.ProjectName}}.md", Template: "command.md.tmpl", Extra: true},
				},
			},
			Validate: ValidateMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					".claude-plugin/plugin.json",
					"hooks/hooks.json",
				},
				ForbiddenFiles: []string{
					"AGENTS.md",
					".codex/config.toml",
				},
				BuildTargets: []string{"./..."},
			},
		},
		{
			Platform:        "codex",
			Status:          runtime.StatusRuntimeSupported,
			PublicPackage:   "codex",
			InternalPackage: "codex",
			InternalImport:  "github.com/hookplex/hookplex/sdk/internal/platforms/codex",
			TransportModes:  []runtime.TransportMode{runtime.ProcessMode},
			LiveTestProfile: "codex_notify",
			Scaffold: ScaffoldMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					"AGENTS.md",
					".codex/config.toml",
				},
				OptionalFiles: []string{
					"Makefile",
					".goreleaser.yml",
					"skills/{{.ProjectName}}/SKILL.md",
					"commands/{{.ProjectName}}.md",
				},
				ForbiddenFiles: []string{
					".claude-plugin/plugin.json",
					"hooks/hooks.json",
				},
				TemplateFiles: []TemplateFile{
					{Path: "go.mod", Template: "codex.go.mod.tmpl"},
					{Path: "cmd/{{.ProjectName}}/main.go", Template: "codex.main.go.tmpl"},
					{Path: "AGENTS.md", Template: "codex.AGENTS.md.tmpl"},
					{Path: ".codex/config.toml", Template: "codex.config.toml.tmpl"},
					{Path: "README.md", Template: "codex.README.md.tmpl"},
					{Path: "Makefile", Template: "Makefile.tmpl", Extra: true},
					{Path: ".goreleaser.yml", Template: "goreleaser.yml.tmpl", Extra: true},
					{Path: "skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true},
					{Path: "commands/{{.ProjectName}}.md", Template: "command.md.tmpl", Extra: true},
				},
			},
			Validate: ValidateMeta{
				RequiredFiles: []string{
					"go.mod",
					"README.md",
					"AGENTS.md",
					".codex/config.toml",
				},
				ForbiddenFiles: []string{
					".claude-plugin/plugin.json",
					"hooks/hooks.json",
				},
				BuildTargets: []string{"./..."},
			},
		},
	}
}

func Events() []EventDescriptor {
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
				Maturity: runtime.MaturityBeta,
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
				Maturity: runtime.MaturityBeta,
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
				Maturity: runtime.MaturityBeta,
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
				Summary:    "Claude SessionStart hook",
			},
			Capabilities: []runtime.CapabilityID{"session_start"},
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
				Summary:    "Claude SessionEnd hook",
			},
			Capabilities: []runtime.CapabilityID{"session_end"},
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
				Summary:    "Claude Notification hook",
			},
			Capabilities: []runtime.CapabilityID{"notification"},
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
				Summary:    "Claude PostToolUse hook",
			},
			Capabilities: []runtime.CapabilityID{"posttooluse"},
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
				Summary:    "Claude PostToolUseFailure hook",
			},
			Capabilities: []runtime.CapabilityID{"posttooluse_failure"},
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
				Summary:    "Claude PermissionRequest hook",
			},
			Capabilities: []runtime.CapabilityID{"permission_request"},
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
				Summary:    "Claude SubagentStart hook",
			},
			Capabilities: []runtime.CapabilityID{"subagent_start"},
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
				Summary:    "Claude SubagentStop hook",
			},
			Capabilities: []runtime.CapabilityID{"subagent_stop"},
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
				Summary:    "Claude PreCompact hook",
			},
			Capabilities: []runtime.CapabilityID{"pre_compact"},
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
				Summary:    "Claude Setup hook",
			},
			Capabilities: []runtime.CapabilityID{"setup"},
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
				Summary:    "Claude TeammateIdle hook",
			},
			Capabilities: []runtime.CapabilityID{"teammate_idle"},
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
				Summary:    "Claude TaskCompleted hook",
			},
			Capabilities: []runtime.CapabilityID{"task_completed"},
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
				Summary:    "Claude ConfigChange hook",
			},
			Capabilities: []runtime.CapabilityID{"config_change"},
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
				Summary:    "Claude WorktreeCreate hook",
			},
			Capabilities: []runtime.CapabilityID{"worktree_create"},
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
				Summary:    "Claude WorktreeRemove hook",
			},
			Capabilities: []runtime.CapabilityID{"worktree_remove"},
		},
		{
			Platform: "codex",
			Event:    "Notify",
			Invocation: InvocationBinding{
				Kind: runtime.InvocationArgvCommand,
				Name: "notify",
			},
			Carrier: runtime.CarrierArgvJSON,
			Contract: ContractMeta{
				Maturity: runtime.MaturityBeta,
				V1Target: true,
			},
			DecodeFunc: "DecodeNotify",
			EncodeFunc: "EncodeNotify",
			Registrar: RegistrarMeta{
				MethodName:   "OnNotify",
				EventType:    "*NotifyEvent",
				ResponseType: "*Response",
				WrapFunc:     "wrapNotify",
			},
			Docs: DocsMeta{
				SnippetKey: "codex-notify",
				TableGroup: "codex",
				Summary:    "Codex notify hook",
			},
			Capabilities: []runtime.CapabilityID{"notify"},
		},
	}
}
