package claude

import (
	"encoding/json"

	internalclaude "github.com/plugin-kit-ai/plugin-kit-ai/sdk/internal/platforms/claude"
	"github.com/plugin-kit-ai/plugin-kit-ai/sdk/internal/runtime"
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
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*SessionStartEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude SessionStart")}
		}
		return runtime.Handled{Value: contextOutcomeFromResponse(fn(ev))}
	}
}

func wrapSessionEnd(fn func(*SessionEndEvent) *SessionEndResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*SessionEndEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude SessionEnd")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}

func wrapNotification(fn func(*NotificationEvent) *NotificationResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*NotificationEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude Notification")}
		}
		return runtime.Handled{Value: contextOutcomeFromResponse(fn(ev))}
	}
}

func wrapPostToolUse(fn func(*PostToolUseEvent) *PostToolUseResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*PostToolUseEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude PostToolUse")}
		}
		return runtime.Handled{Value: postToolUseOutcomeFromResponse(fn(ev))}
	}
}

func wrapPostToolUseFailure(fn func(*PostToolUseFailureEvent) *PostToolUseFailureResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*PostToolUseFailureEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude PostToolUseFailure")}
		}
		return runtime.Handled{Value: contextOutcomeFromResponse(fn(ev))}
	}
}

func wrapPermissionRequest(fn func(*PermissionRequestEvent) *PermissionRequestResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*PermissionRequestEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude PermissionRequest")}
		}
		return runtime.Handled{Value: permissionOutcomeFromResponse(fn(ev))}
	}
}

func wrapSubagentStart(fn func(*SubagentStartEvent) *SubagentStartResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*SubagentStartEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude SubagentStart")}
		}
		return runtime.Handled{Value: contextOutcomeFromResponse(fn(ev))}
	}
}

func wrapSubagentStop(fn func(*SubagentStopEvent) *SubagentStopResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*SubagentStopEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude SubagentStop")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}

func wrapPreCompact(fn func(*PreCompactEvent) *PreCompactResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*PreCompactEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude PreCompact")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}

func wrapSetup(fn func(*SetupEvent) *SetupResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*SetupEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude Setup")}
		}
		return runtime.Handled{Value: contextOutcomeFromResponse(fn(ev))}
	}
}

func wrapTeammateIdle(fn func(*TeammateIdleEvent) *TeammateIdleResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*TeammateIdleEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude TeammateIdle")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}

func wrapTaskCompleted(fn func(*TaskCompletedEvent) *TaskCompletedResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*TaskCompletedEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude TaskCompleted")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}

func wrapConfigChange(fn func(*ConfigChangeEvent) *ConfigChangeResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*ConfigChangeEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude ConfigChange")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}

func wrapWorktreeCreate(fn func(*WorktreeCreateEvent) *WorktreeCreateResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*WorktreeCreateEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude WorktreeCreate")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}

func wrapWorktreeRemove(fn func(*WorktreeRemoveEvent) *WorktreeRemoveResponse) runtime.TypedHandler {
	return func(_ runtime.InvocationContext, v any) runtime.Handled {
		ev, ok := v.(*WorktreeRemoveEvent)
		if !ok {
			return runtime.Handled{Err: internalclaudeTypeMismatch("claude WorktreeRemove")}
		}
		return runtime.Handled{Value: commonOutcomeFromResponse(fn(ev))}
	}
}
