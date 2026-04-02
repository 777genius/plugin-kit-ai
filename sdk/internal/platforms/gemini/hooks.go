package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

type syncOutputDTO struct {
	SystemMessage      string `json:"systemMessage,omitempty"`
	SuppressOutput     bool   `json:"suppressOutput,omitempty"`
	Continue           *bool  `json:"continue,omitempty"`
	StopReason         string `json:"stopReason,omitempty"`
	Decision           string `json:"decision,omitempty"`
	Reason             string `json:"reason,omitempty"`
	HookSpecificOutput any    `json:"hookSpecificOutput,omitempty"`
}

type contextHookSpecificDTO struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext,omitempty"`
}

type toolHookSpecificDTO struct {
	HookEventName string          `json:"hookEventName"`
	ToolInput     json.RawMessage `json:"tool_input,omitempty"`
}

type tailToolCallRequestDTO struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type afterToolHookSpecificDTO struct {
	HookEventName       string                  `json:"hookEventName"`
	AdditionalContext   string                  `json:"additionalContext,omitempty"`
	TailToolCallRequest *tailToolCallRequestDTO `json:"tailToolCallRequest,omitempty"`
}

func DecodeSessionStart(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[SessionStartInput](env, "session start", "SessionStart")
}

func DecodeSessionEnd(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[SessionEndInput](env, "session end", "SessionEnd")
}

func DecodeBeforeTool(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[BeforeToolInput](env, "before tool", "BeforeTool")
}

func DecodeAfterTool(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[AfterToolInput](env, "after tool", "AfterTool")
}

func EncodeSessionStart(v any) runtime.Result {
	out, ok := v.(SessionStartOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini SessionStart response: internal outcome type mismatch\n"}
	}
	out.CommonOutcome = sanitizeLifecycleOutcome(out.CommonOutcome)
	var hookSpecific any
	if strings.TrimSpace(out.AdditionalContext) != "" {
		hookSpecific = contextHookSpecificDTO{
			HookEventName:     "SessionStart",
			AdditionalContext: out.AdditionalContext,
		}
	}
	return encodeSync("Gemini SessionStart", out.CommonOutcome, hookSpecific)
}

func EncodeSessionEnd(v any) runtime.Result {
	out, ok := v.(SessionEndOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini SessionEnd response: internal outcome type mismatch\n"}
	}
	out.CommonOutcome = sanitizeLifecycleOutcome(out.CommonOutcome)
	return encodeSync("Gemini SessionEnd", out.CommonOutcome, nil)
}

func EncodeBeforeTool(v any) runtime.Result {
	out, ok := v.(BeforeToolOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini BeforeTool response: internal outcome type mismatch\n"}
	}
	if err := validateToolInputObject(out.ToolInput); err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("Gemini BeforeTool: %v\n", err)}
	}
	var hookSpecific any
	if len(out.ToolInput) > 0 {
		hookSpecific = toolHookSpecificDTO{
			HookEventName: "BeforeTool",
			ToolInput:     out.ToolInput,
		}
	}
	return encodeSync("Gemini BeforeTool", out.CommonOutcome, hookSpecific)
}

func EncodeAfterTool(v any) runtime.Result {
	out, ok := v.(AfterToolOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini AfterTool response: internal outcome type mismatch\n"}
	}
	if err := validateTailToolCallRequest(out.TailToolCallRequest); err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("Gemini AfterTool: %v\n", err)}
	}
	var hookSpecific any
	if strings.TrimSpace(out.AdditionalContext) != "" || out.TailToolCallRequest != nil {
		dto := afterToolHookSpecificDTO{
			HookEventName:     "AfterTool",
			AdditionalContext: out.AdditionalContext,
		}
		if out.TailToolCallRequest != nil {
			dto.TailToolCallRequest = &tailToolCallRequestDTO{
				Name: out.TailToolCallRequest.Name,
				Args: out.TailToolCallRequest.Args,
			}
		}
		hookSpecific = dto
	}
	return encodeSync("Gemini AfterTool", out.CommonOutcome, hookSpecific)
}

func decodeJSONInput[T any](env runtime.Envelope, label, eventName string) (any, string, error) {
	var dto T
	if err := json.Unmarshal(env.Stdin, &dto); err != nil {
		return nil, "", fmt.Errorf("decode Gemini %s input: %w", label, err)
	}
	return &dto, eventName, nil
}

func encodeSync(label string, out CommonOutcome, hookSpecific any) runtime.Result {
	if err := validateDecision(out.Decision); err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("%s: %v\n", label, err)}
	}
	if hookSpecific == nil &&
		out.Continue == nil &&
		!out.SuppressOutput &&
		strings.TrimSpace(out.StopReason) == "" &&
		strings.TrimSpace(out.Decision) == "" &&
		strings.TrimSpace(out.Reason) == "" &&
		strings.TrimSpace(out.SystemMessage) == "" {
		return runtime.Result{ExitCode: 0, Stdout: []byte("{}")}
	}
	body, err := json.Marshal(syncOutputDTO{
		SystemMessage:      out.SystemMessage,
		SuppressOutput:     out.SuppressOutput,
		Continue:           out.Continue,
		StopReason:         out.StopReason,
		Decision:           out.Decision,
		Reason:             out.Reason,
		HookSpecificOutput: hookSpecific,
	})
	if err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("%s: %v\n", label, err)}
	}
	return runtime.Result{ExitCode: 0, Stdout: body}
}

func sanitizeLifecycleOutcome(out CommonOutcome) CommonOutcome {
	out.Continue = nil
	out.StopReason = ""
	out.Decision = ""
	out.Reason = ""
	return out
}

func looksLikeJSONObject(body []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(body)), "{")
}

func validateToolInputObject(body json.RawMessage) error {
	if len(body) == 0 {
		return nil
	}
	if !looksLikeJSONObject(body) {
		return fmt.Errorf("hookSpecificOutput.tool_input must be a JSON object")
	}
	return nil
}

func validateTailToolCallRequest(req *TailToolCallRequest) error {
	if req == nil {
		return nil
	}
	if strings.TrimSpace(req.Name) == "" {
		return fmt.Errorf("hookSpecificOutput.tailToolCallRequest.name is required")
	}
	if !looksLikeJSONObject(req.Args) {
		return fmt.Errorf("hookSpecificOutput.tailToolCallRequest.args must be a JSON object")
	}
	return nil
}

func validateDecision(decision string) error {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "", "allow", "deny", "block":
		return nil
	default:
		return fmt.Errorf("unknown decision %q", decision)
	}
}
