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

type afterAgentHookSpecificDTO struct {
	HookEventName string `json:"hookEventName"`
	ClearContext  bool   `json:"clearContext,omitempty"`
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

type modelHookSpecificDTO struct {
	HookEventName string          `json:"hookEventName"`
	LLMRequest    json.RawMessage `json:"llm_request,omitempty"`
	LLMResponse   json.RawMessage `json:"llm_response,omitempty"`
}

func DecodeSessionStart(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[SessionStartInput](env, "session start", "SessionStart")
}

func DecodeSessionEnd(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[SessionEndInput](env, "session end", "SessionEnd")
}

func DecodeNotification(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[NotificationInput](env, "notification", "Notification")
}

func DecodePreCompress(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[PreCompressInput](env, "pre-compress", "PreCompress")
}

func DecodeBeforeModel(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[BeforeModelInput](env, "before model", "BeforeModel")
}

func DecodeAfterModel(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[AfterModelInput](env, "after model", "AfterModel")
}

func DecodeBeforeAgent(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[BeforeAgentInput](env, "before agent", "BeforeAgent")
}

func DecodeAfterAgent(env runtime.Envelope) (any, string, error) {
	return decodeJSONInput[AfterAgentInput](env, "after agent", "AfterAgent")
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

func EncodeNotification(v any) runtime.Result {
	out, ok := v.(NotificationOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini Notification response: internal outcome type mismatch\n"}
	}
	out.CommonOutcome = sanitizeAdvisoryOutcome(out.CommonOutcome)
	return encodeSync("Gemini Notification", out.CommonOutcome, nil)
}

func EncodePreCompress(v any) runtime.Result {
	out, ok := v.(PreCompressOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini PreCompress response: internal outcome type mismatch\n"}
	}
	out.CommonOutcome = sanitizeAdvisoryOutcome(out.CommonOutcome)
	return encodeSync("Gemini PreCompress", out.CommonOutcome, nil)
}

func EncodeBeforeModel(v any) runtime.Result {
	out, ok := v.(BeforeModelOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini BeforeModel response: internal outcome type mismatch\n"}
	}
	if err := validateModelResponse(out.LLMResponse); err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("Gemini BeforeModel: %v\n", err)}
	}
	if err := validateModelRequest(out.LLMRequest); err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("Gemini BeforeModel: %v\n", err)}
	}
	var hookSpecific any
	if len(out.LLMRequest) > 0 || len(out.LLMResponse) > 0 {
		hookSpecific = modelHookSpecificDTO{
			HookEventName: "BeforeModel",
			LLMRequest:    out.LLMRequest,
			LLMResponse:   out.LLMResponse,
		}
	}
	return encodeSync("Gemini BeforeModel", out.CommonOutcome, hookSpecific)
}

func EncodeAfterModel(v any) runtime.Result {
	out, ok := v.(AfterModelOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini AfterModel response: internal outcome type mismatch\n"}
	}
	if err := validateModelResponse(out.LLMResponse); err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("Gemini AfterModel: %v\n", err)}
	}
	var hookSpecific any
	if len(out.LLMResponse) > 0 {
		hookSpecific = modelHookSpecificDTO{
			HookEventName: "AfterModel",
			LLMResponse:   out.LLMResponse,
		}
	}
	return encodeSync("Gemini AfterModel", out.CommonOutcome, hookSpecific)
}

func EncodeBeforeAgent(v any) runtime.Result {
	out, ok := v.(BeforeAgentOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini BeforeAgent response: internal outcome type mismatch\n"}
	}
	var hookSpecific any
	if strings.TrimSpace(out.AdditionalContext) != "" {
		hookSpecific = contextHookSpecificDTO{
			HookEventName:     "BeforeAgent",
			AdditionalContext: out.AdditionalContext,
		}
	}
	return encodeSync("Gemini BeforeAgent", out.CommonOutcome, hookSpecific)
}

func EncodeAfterAgent(v any) runtime.Result {
	out, ok := v.(AfterAgentOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini AfterAgent response: internal outcome type mismatch\n"}
	}
	var hookSpecific any
	if out.ClearContext {
		hookSpecific = afterAgentHookSpecificDTO{
			HookEventName: "AfterAgent",
			ClearContext:  true,
		}
	}
	return encodeSync("Gemini AfterAgent", out.CommonOutcome, hookSpecific)
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
	return sanitizeAdvisoryOutcome(out)
}

func sanitizeAdvisoryOutcome(out CommonOutcome) CommonOutcome {
	out.Continue = nil
	out.StopReason = ""
	out.Decision = ""
	out.Reason = ""
	return out
}

func validateToolInputObject(body json.RawMessage) error {
	if len(body) == 0 {
		return nil
	}
	if err := validateJSONObjectBytes(body, "hookSpecificOutput.tool_input"); err != nil {
		return err
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
	if err := validateJSONObjectBytes(req.Args, "hookSpecificOutput.tailToolCallRequest.args"); err != nil {
		return err
	}
	return nil
}

func validateModelRequest(body json.RawMessage) error {
	if len(body) == 0 {
		return nil
	}
	return validateJSONObjectBytes(body, "hookSpecificOutput.llm_request")
}

func validateModelResponse(body json.RawMessage) error {
	if len(body) == 0 {
		return nil
	}
	return validateJSONObjectBytes(body, "hookSpecificOutput.llm_response")
}

func validateJSONObjectBytes(body []byte, field string) error {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return fmt.Errorf("%s must be a JSON object", field)
	}
	if !strings.HasPrefix(trimmed, "{") {
		return fmt.Errorf("%s must be a JSON object", field)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return fmt.Errorf("%s must be valid JSON object: %w", field, err)
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
