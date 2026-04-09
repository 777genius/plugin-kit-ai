package claude

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

type BaseInput struct {
	SessionID      string `json:"session_id"`
	TranscriptPath string `json:"transcript_path,omitempty"`
	CWD            string `json:"cwd,omitempty"`
	PermissionMode string `json:"permission_mode,omitempty"`
	HookEventName  string `json:"hook_event_name"`
}

type SessionStartInput struct {
	BaseInput
	Source string `json:"source,omitempty"`
}

type SessionEndInput struct {
	BaseInput
	Reason string `json:"reason,omitempty"`
}

type NotificationInput struct {
	BaseInput
	Message          string `json:"message,omitempty"`
	NotificationType string `json:"notification_type,omitempty"`
}

type PostToolUseInput struct {
	BaseInput
	ToolName     string          `json:"tool_name"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	ToolInput    json.RawMessage `json:"tool_input,omitempty"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
}

type PostToolUseFailureInput struct {
	BaseInput
	ToolName  string          `json:"tool_name"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	ToolInput json.RawMessage `json:"tool_input,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type PermissionRequestInput struct {
	BaseInput
	ToolName       string          `json:"tool_name,omitempty"`
	ToolUseID      string          `json:"tool_use_id,omitempty"`
	ToolInput      json.RawMessage `json:"tool_input,omitempty"`
	RequestedPath  string          `json:"requested_path,omitempty"`
	RequestedScope string          `json:"requested_scope,omitempty"`
}

type SubagentStartInput struct {
	BaseInput
	AgentID         string `json:"agent_id,omitempty"`
	AgentType       string `json:"agent_type,omitempty"`
	Prompt          string `json:"prompt,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`
}

type SubagentStopInput struct {
	BaseInput
	AgentID         string `json:"agent_id,omitempty"`
	AgentType       string `json:"agent_type,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	ParentToolUseID string `json:"parent_tool_use_id,omitempty"`
}

type PreCompactInput struct {
	BaseInput
	Trigger string `json:"trigger,omitempty"`
}

type SetupInput struct {
	BaseInput
	SetupHookActive bool `json:"setup_hook_active,omitempty"`
}

type TeammateIdleInput struct {
	BaseInput
	TeamName     string `json:"team_name,omitempty"`
	TeammateName string `json:"teammate_name,omitempty"`
}

type TaskCompletedInput struct {
	BaseInput
	TaskID   string `json:"task_id,omitempty"`
	TaskName string `json:"task_name,omitempty"`
}

type ConfigChangeInput struct {
	BaseInput
	ChangedKeys []string `json:"changed_keys,omitempty"`
}

type WorktreeCreateInput struct {
	BaseInput
	WorktreePath string `json:"worktree_path,omitempty"`
}

type WorktreeRemoveInput struct {
	BaseInput
	WorktreePath string `json:"worktree_path,omitempty"`
}

type PermissionBehavior string

const (
	PermissionAllow PermissionBehavior = "allow"
	PermissionDeny  PermissionBehavior = "deny"
)

type PermissionRuleValue struct {
	Type   string   `json:"type,omitempty"`
	Action string   `json:"action,omitempty"`
	Paths  []string `json:"paths,omitempty"`
}

type PermissionUpdate struct {
	Type        string                `json:"type,omitempty"`
	Behavior    PermissionBehavior    `json:"behavior,omitempty"`
	Destination string                `json:"destination,omitempty"`
	Mode        string                `json:"mode,omitempty"`
	Directories []string              `json:"directories,omitempty"`
	Rules       []PermissionRuleValue `json:"rules,omitempty"`
}

type CommonOutcome struct {
	Continue       *bool
	SuppressOutput bool
	StopReason     string
	Decision       string
	Reason         string
	SystemMessage  string
}

type ContextOutcome struct {
	CommonOutcome
	AdditionalContext string
}

type PostToolUseOutcome struct {
	CommonOutcome
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

type PermissionRequestOutcome struct {
	CommonOutcome
	Permission *PermissionDecision
}

type syncOutputDTO struct {
	Continue           *bool  `json:"continue,omitempty"`
	SuppressOutput     bool   `json:"suppressOutput,omitempty"`
	StopReason         string `json:"stopReason,omitempty"`
	Decision           string `json:"decision,omitempty"`
	Reason             string `json:"reason,omitempty"`
	SystemMessage      string `json:"systemMessage,omitempty"`
	HookSpecificOutput any    `json:"hookSpecificOutput,omitempty"`
}

type contextHookSpecificDTO struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

type postToolUseHookSpecificDTO struct {
	HookEventName        string          `json:"hookEventName"`
	AdditionalContext    string          `json:"additionalContext,omitempty"`
	UpdatedMCPToolOutput json.RawMessage `json:"updatedMcpToolOutput,omitempty"`
}

type permissionDecisionDTO struct {
	Behavior           string             `json:"behavior"`
	UpdatedInput       json.RawMessage    `json:"updatedInput,omitempty"`
	UpdatedPermissions []PermissionUpdate `json:"updatedPermissions,omitempty"`
	Message            string             `json:"message,omitempty"`
	Interrupt          bool               `json:"interrupt,omitempty"`
}

type permissionHookSpecificDTO struct {
	HookEventName string                `json:"hookEventName"`
	Decision      permissionDecisionDTO `json:"decision"`
}

func decodeJSONInput[T any](env runtime.Envelope, label string) (*T, error) {
	return runtime.DecodeJSONPayload[T](env.Stdin, label+" input")
}

func encodeSync(label string, out CommonOutcome, hookSpecific any) runtime.Result {
	if hookSpecific == nil &&
		out.Continue == nil &&
		!out.SuppressOutput &&
		strings.TrimSpace(out.StopReason) == "" &&
		strings.TrimSpace(out.Decision) == "" &&
		strings.TrimSpace(out.Reason) == "" &&
		strings.TrimSpace(out.SystemMessage) == "" {
		return runtime.Result{ExitCode: 0, Stdout: []byte("{}")}
	}
	switch out.Decision {
	case "", "approve", "block":
	default:
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("encode %s response: unknown decision %q\n", label, out.Decision)}
	}
	b, err := json.Marshal(syncOutputDTO{
		Continue:           out.Continue,
		SuppressOutput:     out.SuppressOutput,
		StopReason:         out.StopReason,
		Decision:           out.Decision,
		Reason:             out.Reason,
		SystemMessage:      out.SystemMessage,
		HookSpecificOutput: hookSpecific,
	})
	if err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("encode %s response: %v\n", label, err)}
	}
	return runtime.Result{ExitCode: 0, Stdout: b}
}

func EncodeCommon(v any, label string) runtime.Result {
	out, ok := v.(CommonOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("encode %s response: internal outcome type mismatch\n", label)}
	}
	return encodeSync(label, out, nil)
}

func EncodeContext(v any, label, hookEventName string) runtime.Result {
	out, ok := v.(ContextOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("encode %s response: internal outcome type mismatch\n", label)}
	}
	var specific any
	if strings.TrimSpace(out.AdditionalContext) != "" {
		specific = contextHookSpecificDTO{
			HookEventName:     hookEventName,
			AdditionalContext: out.AdditionalContext,
		}
	}
	return encodeSync(label, out.CommonOutcome, specific)
}

func EncodePostToolUseOutcome(out PostToolUseOutcome) runtime.Result {
	var specific any
	if strings.TrimSpace(out.AdditionalContext) != "" || len(out.UpdatedMCPToolOutput) > 0 {
		specific = postToolUseHookSpecificDTO{
			HookEventName:        "PostToolUse",
			AdditionalContext:    out.AdditionalContext,
			UpdatedMCPToolOutput: out.UpdatedMCPToolOutput,
		}
	}
	return encodeSync("posttooluse", out.CommonOutcome, specific)
}

func EncodePostToolUse(v any) runtime.Result {
	out, ok := v.(PostToolUseOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode posttooluse response: internal outcome type mismatch\n"}
	}
	return EncodePostToolUseOutcome(out)
}

func EncodePermissionRequestOutcome(out PermissionRequestOutcome) runtime.Result {
	var specific any
	if out.Permission != nil {
		behavior := strings.ToLower(strings.TrimSpace(string(out.Permission.Behavior)))
		switch behavior {
		case string(PermissionAllow), string(PermissionDeny):
		default:
			return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("encode permissionrequest response: unknown behavior %q\n", out.Permission.Behavior)}
		}
		specific = permissionHookSpecificDTO{
			HookEventName: "PermissionRequest",
			Decision: permissionDecisionDTO{
				Behavior:           behavior,
				UpdatedInput:       out.Permission.UpdatedInput,
				UpdatedPermissions: out.Permission.UpdatedPermissions,
				Message:            out.Permission.Message,
				Interrupt:          out.Permission.Interrupt,
			},
		}
	}
	return encodeSync("permissionrequest", out.CommonOutcome, specific)
}

func EncodePermissionRequest(v any) runtime.Result {
	out, ok := v.(PermissionRequestOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode permissionrequest response: internal outcome type mismatch\n"}
	}
	return EncodePermissionRequestOutcome(out)
}

func DecodeSessionStart(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[SessionStartInput](env, "sessionstart")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeSessionEnd(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[SessionEndInput](env, "sessionend")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeNotification(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[NotificationInput](env, "notification")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodePostToolUse(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[PostToolUseInput](env, "posttooluse")
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(dto.ToolName) == "" {
		return nil, "", fmt.Errorf("decode posttooluse input: tool_name required")
	}
	return dto, dto.HookEventName, nil
}

func DecodePostToolUseFailure(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[PostToolUseFailureInput](env, "posttoolusefailure")
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(dto.ToolName) == "" {
		return nil, "", fmt.Errorf("decode posttoolusefailure input: tool_name required")
	}
	return dto, dto.HookEventName, nil
}

func DecodePermissionRequest(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[PermissionRequestInput](env, "permissionrequest")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeSubagentStart(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[SubagentStartInput](env, "subagentstart")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeSubagentStop(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[SubagentStopInput](env, "subagentstop")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodePreCompact(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[PreCompactInput](env, "precompact")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeSetup(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[SetupInput](env, "setup")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeTeammateIdle(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[TeammateIdleInput](env, "teammateidle")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeTaskCompleted(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[TaskCompletedInput](env, "taskcompleted")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeConfigChange(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[ConfigChangeInput](env, "configchange")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeWorktreeCreate(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[WorktreeCreateInput](env, "worktreecreate")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func DecodeWorktreeRemove(env runtime.Envelope) (any, string, error) {
	dto, err := decodeJSONInput[WorktreeRemoveInput](env, "worktreeremove")
	if err != nil {
		return nil, "", err
	}
	return dto, dto.HookEventName, nil
}

func EncodeSessionStart(v any) runtime.Result {
	return EncodeContext(v, "sessionstart", "SessionStart")
}
func EncodeNotification(v any) runtime.Result {
	return EncodeContext(v, "notification", "Notification")
}
func EncodePostToolUseFailure(v any) runtime.Result {
	return EncodeContext(v, "posttoolusefailure", "PostToolUseFailure")
}
func EncodeSessionEnd(v any) runtime.Result { return EncodeCommon(v, "sessionend") }
func EncodeSubagentStart(v any) runtime.Result {
	return EncodeContext(v, "subagentstart", "SubagentStart")
}
func EncodeSubagentStop(v any) runtime.Result   { return EncodeCommon(v, "subagentstop") }
func EncodePreCompact(v any) runtime.Result     { return EncodeCommon(v, "precompact") }
func EncodeSetup(v any) runtime.Result          { return EncodeContext(v, "setup", "Setup") }
func EncodeTeammateIdle(v any) runtime.Result   { return EncodeCommon(v, "teammateidle") }
func EncodeTaskCompleted(v any) runtime.Result  { return EncodeCommon(v, "taskcompleted") }
func EncodeConfigChange(v any) runtime.Result   { return EncodeCommon(v, "configchange") }
func EncodeWorktreeCreate(v any) runtime.Result { return EncodeCommon(v, "worktreecreate") }
func EncodeWorktreeRemove(v any) runtime.Result { return EncodeCommon(v, "worktreeremove") }
