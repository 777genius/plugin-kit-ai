package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

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

func EncodeBeforeToolSelection(v any) runtime.Result {
	out, ok := v.(BeforeToolSelectionOutcome)
	if !ok {
		return runtime.Result{ExitCode: 1, Stderr: "encode Gemini BeforeToolSelection response: internal outcome type mismatch\n"}
	}
	if err := validateToolConfig(out.ToolConfig); err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("Gemini BeforeToolSelection: %v\n", err)}
	}
	if out.ToolConfig == nil && !out.SuppressOutput {
		return runtime.Result{ExitCode: 0, Stdout: []byte("{}")}
	}
	dto := syncOutputDTO{SuppressOutput: out.SuppressOutput}
	if out.ToolConfig != nil {
		dto.HookSpecificOutput = beforeToolSelectionHookSpecificDTO{
			HookEventName: "BeforeToolSelection",
			ToolConfig: &toolConfigDTO{
				Mode:                 normalizeToolConfigMode(out.ToolConfig.Mode),
				AllowedFunctionNames: normalizeFunctionNames(out.ToolConfig.AllowedFunctionNames),
			},
		}
	}
	body, err := json.Marshal(dto)
	if err != nil {
		return runtime.Result{ExitCode: 1, Stderr: fmt.Sprintf("Gemini BeforeToolSelection: %v\n", err)}
	}
	return runtime.Result{ExitCode: 0, Stdout: body}
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
