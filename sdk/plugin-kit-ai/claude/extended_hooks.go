package claude

import (
	"encoding/json"

	internalclaude "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/claude"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

type SessionStartEvent = internalclaude.SessionStartInput
type SessionEndEvent = internalclaude.SessionEndInput
type NotificationEvent = internalclaude.NotificationInput
type PostToolUseEvent = internalclaude.PostToolUseInput
type PostToolUseFailureEvent = internalclaude.PostToolUseFailureInput
type PermissionRequestEvent = internalclaude.PermissionRequestInput
type SubagentStartEvent = internalclaude.SubagentStartInput
type SubagentStopEvent = internalclaude.SubagentStopInput
type PreCompactEvent = internalclaude.PreCompactInput
type SetupEvent = internalclaude.SetupInput
type TeammateIdleEvent = internalclaude.TeammateIdleInput
type TaskCompletedEvent = internalclaude.TaskCompletedInput
type ConfigChangeEvent = internalclaude.ConfigChangeInput
type WorktreeCreateEvent = internalclaude.WorktreeCreateInput
type WorktreeRemoveEvent = internalclaude.WorktreeRemoveInput

type PermissionBehavior = internalclaude.PermissionBehavior
type PermissionUpdate = internalclaude.PermissionUpdate
type PermissionRuleValue = internalclaude.PermissionRuleValue

const (
	PermissionAllow PermissionBehavior = internalclaude.PermissionAllow
	PermissionDeny  PermissionBehavior = internalclaude.PermissionDeny
)

type CommonResponse struct {
	Continue       *bool
	SuppressOutput bool
	StopReason     string
	Decision       string
	Reason         string
	SystemMessage  string
}

type ContextResponse struct {
	CommonResponse
	AdditionalContext string
}

type PostToolUseResponse struct {
	CommonResponse
	AdditionalContext    string
	UpdatedMCPToolOutput json.RawMessage
}

type PermissionDecision struct {
	Behavior           PermissionBehavior
	UpdatedInput       json.RawMessage
	UpdatedPermissions []PermissionUpdate
	Message            string
	Interrupt          bool
}

type PermissionRequestResponse struct {
	CommonResponse
	Permission *PermissionDecision
}

type SessionStartResponse = ContextResponse
type NotificationResponse = ContextResponse
type PostToolUseFailureResponse = ContextResponse
type SessionEndResponse = CommonResponse
type SubagentStartResponse = ContextResponse
type SubagentStopResponse = CommonResponse
type PreCompactResponse = CommonResponse
type SetupResponse = ContextResponse
type TeammateIdleResponse = CommonResponse
type TaskCompletedResponse = CommonResponse
type ConfigChangeResponse = CommonResponse
type WorktreeCreateResponse = CommonResponse
type WorktreeRemoveResponse = CommonResponse

func PermissionApprove() *PermissionRequestResponse {
	return &PermissionRequestResponse{
		Permission: &PermissionDecision{Behavior: PermissionAllow},
	}
}

func PermissionApproveWithUpdates(input json.RawMessage, updates []PermissionUpdate) *PermissionRequestResponse {
	return &PermissionRequestResponse{
		Permission: &PermissionDecision{
			Behavior:           PermissionAllow,
			UpdatedInput:       input,
			UpdatedPermissions: updates,
		},
	}
}

func PermissionBlock(message string, interrupt bool) *PermissionRequestResponse {
	return &PermissionRequestResponse{
		Permission: &PermissionDecision{
			Behavior:  PermissionDeny,
			Message:   message,
			Interrupt: interrupt,
		},
	}
}

func commonOutcomeFromResponse(r *CommonResponse) internalclaude.CommonOutcome {
	if r == nil {
		return internalclaude.CommonOutcome{}
	}
	return internalclaude.CommonOutcome{
		Continue:       r.Continue,
		SuppressOutput: r.SuppressOutput,
		StopReason:     r.StopReason,
		Decision:       r.Decision,
		Reason:         r.Reason,
		SystemMessage:  r.SystemMessage,
	}
}

func contextOutcomeFromResponse(r *ContextResponse) internalclaude.ContextOutcome {
	if r == nil {
		return internalclaude.ContextOutcome{}
	}
	return internalclaude.ContextOutcome{
		CommonOutcome:     commonOutcomeFromResponse(&r.CommonResponse),
		AdditionalContext: r.AdditionalContext,
	}
}

func postToolUseOutcomeFromResponse(r *PostToolUseResponse) internalclaude.PostToolUseOutcome {
	if r == nil {
		return internalclaude.PostToolUseOutcome{}
	}
	return internalclaude.PostToolUseOutcome{
		CommonOutcome:        commonOutcomeFromResponse(&r.CommonResponse),
		AdditionalContext:    r.AdditionalContext,
		UpdatedMCPToolOutput: r.UpdatedMCPToolOutput,
	}
}

func permissionOutcomeFromResponse(r *PermissionRequestResponse) internalclaude.PermissionRequestOutcome {
	if r == nil {
		return internalclaude.PermissionRequestOutcome{}
	}
	out := internalclaude.PermissionRequestOutcome{
		CommonOutcome: commonOutcomeFromResponse(&r.CommonResponse),
	}
	if r.Permission != nil {
		out.Permission = &internalclaude.PermissionDecision{
			Behavior:           internalclaude.PermissionBehavior(r.Permission.Behavior),
			UpdatedInput:       r.Permission.UpdatedInput,
			UpdatedPermissions: r.Permission.UpdatedPermissions,
			Message:            r.Permission.Message,
			Interrupt:          r.Permission.Interrupt,
		}
	}
	return out
}

func wrapSessionStart(fn func(*SessionStartEvent) *SessionStartResponse) runtime.TypedHandler {
	return wrapClaudeHandler("SessionStart", fn, func(r *SessionStartResponse) any {
		return contextOutcomeFromResponse(r)
	})
}

func wrapSessionEnd(fn func(*SessionEndEvent) *SessionEndResponse) runtime.TypedHandler {
	return wrapClaudeHandler("SessionEnd", fn, func(r *SessionEndResponse) any {
		return commonOutcomeFromResponse(r)
	})
}

func wrapNotification(fn func(*NotificationEvent) *NotificationResponse) runtime.TypedHandler {
	return wrapClaudeHandler("Notification", fn, func(r *NotificationResponse) any {
		return contextOutcomeFromResponse(r)
	})
}

func wrapPostToolUse(fn func(*PostToolUseEvent) *PostToolUseResponse) runtime.TypedHandler {
	return wrapClaudeHandler("PostToolUse", fn, func(r *PostToolUseResponse) any {
		return postToolUseOutcomeFromResponse(r)
	})
}

func wrapPostToolUseFailure(fn func(*PostToolUseFailureEvent) *PostToolUseFailureResponse) runtime.TypedHandler {
	return wrapClaudeHandler("PostToolUseFailure", fn, func(r *PostToolUseFailureResponse) any {
		return contextOutcomeFromResponse(r)
	})
}

func wrapPermissionRequest(fn func(*PermissionRequestEvent) *PermissionRequestResponse) runtime.TypedHandler {
	return wrapClaudeHandler("PermissionRequest", fn, func(r *PermissionRequestResponse) any {
		return permissionOutcomeFromResponse(r)
	})
}

func wrapSubagentStart(fn func(*SubagentStartEvent) *SubagentStartResponse) runtime.TypedHandler {
	return wrapClaudeHandler("SubagentStart", fn, func(r *SubagentStartResponse) any {
		return contextOutcomeFromResponse(r)
	})
}

func wrapSubagentStop(fn func(*SubagentStopEvent) *SubagentStopResponse) runtime.TypedHandler {
	return wrapClaudeHandler("SubagentStop", fn, func(r *SubagentStopResponse) any {
		return commonOutcomeFromResponse(r)
	})
}

func wrapPreCompact(fn func(*PreCompactEvent) *PreCompactResponse) runtime.TypedHandler {
	return wrapClaudeHandler("PreCompact", fn, func(r *PreCompactResponse) any {
		return commonOutcomeFromResponse(r)
	})
}

func wrapSetup(fn func(*SetupEvent) *SetupResponse) runtime.TypedHandler {
	return wrapClaudeHandler("Setup", fn, func(r *SetupResponse) any {
		return contextOutcomeFromResponse(r)
	})
}

func wrapTeammateIdle(fn func(*TeammateIdleEvent) *TeammateIdleResponse) runtime.TypedHandler {
	return wrapClaudeHandler("TeammateIdle", fn, func(r *TeammateIdleResponse) any {
		return commonOutcomeFromResponse(r)
	})
}

func wrapTaskCompleted(fn func(*TaskCompletedEvent) *TaskCompletedResponse) runtime.TypedHandler {
	return wrapClaudeHandler("TaskCompleted", fn, func(r *TaskCompletedResponse) any {
		return commonOutcomeFromResponse(r)
	})
}

func wrapConfigChange(fn func(*ConfigChangeEvent) *ConfigChangeResponse) runtime.TypedHandler {
	return wrapClaudeHandler("ConfigChange", fn, func(r *ConfigChangeResponse) any {
		return commonOutcomeFromResponse(r)
	})
}

func wrapWorktreeCreate(fn func(*WorktreeCreateEvent) *WorktreeCreateResponse) runtime.TypedHandler {
	return wrapClaudeHandler("WorktreeCreate", fn, func(r *WorktreeCreateResponse) any {
		return commonOutcomeFromResponse(r)
	})
}

func wrapWorktreeRemove(fn func(*WorktreeRemoveEvent) *WorktreeRemoveResponse) runtime.TypedHandler {
	return wrapClaudeHandler("WorktreeRemove", fn, func(r *WorktreeRemoveResponse) any {
		return commonOutcomeFromResponse(r)
	})
}
