package gemini

import "encoding/json"

// SessionStartContinue returns an explicit no-op SessionStart response.
func SessionStartContinue() *SessionStartResponse {
	return &SessionStartResponse{}
}

// SessionStartAddContext appends additional context during SessionStart.
func SessionStartAddContext(context string) *SessionStartResponse {
	return &SessionStartResponse{AdditionalContext: context}
}

// SessionStartMessage emits a systemMessage during SessionStart.
func SessionStartMessage(message string) *SessionStartResponse {
	return &SessionStartResponse{CommonResponse: CommonResponse{SystemMessage: message}}
}

// SessionEndContinue returns an explicit no-op SessionEnd response.
func SessionEndContinue() *SessionEndResponse {
	return &SessionEndResponse{}
}

// SessionEndMessage emits a systemMessage during SessionEnd.
func SessionEndMessage(message string) *SessionEndResponse {
	return &SessionEndResponse{SystemMessage: message}
}

// BeforeModelContinue returns an explicit no-op BeforeModel response.
func BeforeModelContinue() *BeforeModelResponse {
	return &BeforeModelResponse{}
}

// BeforeModelDeny blocks the LLM request with a deny decision.
func BeforeModelDeny(reason string) *BeforeModelResponse {
	return &BeforeModelResponse{CommonResponse: CommonResponse{Decision: "deny", Reason: reason}}
}

// BeforeModelOverrideRequest continues with a rewritten llm_request payload.
func BeforeModelOverrideRequest(request json.RawMessage) *BeforeModelResponse {
	return &BeforeModelResponse{LLMRequest: request}
}

// BeforeModelSyntheticResponse short-circuits the LLM request with a synthetic
// llm_response payload.
func BeforeModelSyntheticResponse(response json.RawMessage) *BeforeModelResponse {
	return &BeforeModelResponse{LLMResponse: response}
}

// AfterModelContinue returns an explicit no-op AfterModel response.
func AfterModelContinue() *AfterModelResponse {
	return &AfterModelResponse{}
}

// AfterModelDeny blocks the model result with a deny decision.
func AfterModelDeny(reason string) *AfterModelResponse {
	return &AfterModelResponse{CommonResponse: CommonResponse{Decision: "deny", Reason: reason}}
}

// AfterModelStop stops the entire Gemini agent loop immediately.
func AfterModelStop(reason string) *AfterModelResponse {
	continueFalse := false
	return &AfterModelResponse{CommonResponse: CommonResponse{Continue: &continueFalse, StopReason: reason}}
}

// AfterModelReplaceResponse continues with a rewritten llm_response payload.
func AfterModelReplaceResponse(response json.RawMessage) *AfterModelResponse {
	return &AfterModelResponse{LLMResponse: response}
}

// BeforeToolSelectionContinue returns an explicit no-op BeforeToolSelection response.
func BeforeToolSelectionContinue() *BeforeToolSelectionResponse {
	return &BeforeToolSelectionResponse{}
}

// BeforeToolSelectionQuiet suppresses Gemini's internal hook metadata for the
// current tool-selection step without changing toolConfig.
func BeforeToolSelectionQuiet() *BeforeToolSelectionResponse {
	return &BeforeToolSelectionResponse{SuppressOutput: true}
}

// BeforeToolSelectionConfig applies a tool selection mode. Gemini currently
// accepts allowedFunctionNames only together with ANY mode.
func BeforeToolSelectionConfig(mode ToolMode, allowedFunctionNames ...string) *BeforeToolSelectionResponse {
	return &BeforeToolSelectionResponse{
		Mode:                 mode,
		AllowedFunctionNames: append([]string(nil), allowedFunctionNames...),
	}
}

// BeforeToolSelectionAllowOnly restricts Gemini tool selection to the provided
// allowlist by using ANY mode, which is the vendor-accepted shape for
// allowedFunctionNames.
func BeforeToolSelectionAllowOnly(allowedFunctionNames ...string) *BeforeToolSelectionResponse {
	return BeforeToolSelectionConfig(ToolModeAny, allowedFunctionNames...)
}

// BeforeToolSelectionForceAny requires Gemini to pick at least one tool and
// optionally narrows the candidate set with an allowlist.
func BeforeToolSelectionForceAny(allowedFunctionNames ...string) *BeforeToolSelectionResponse {
	return BeforeToolSelectionConfig(ToolModeAny, allowedFunctionNames...)
}

// BeforeToolSelectionForceAuto explicitly restores AUTO tool mode. Gemini does
// not currently accept allowedFunctionNames outside ANY mode, so any optional
// allowlist arguments are ignored.
func BeforeToolSelectionForceAuto(allowedFunctionNames ...string) *BeforeToolSelectionResponse {
	return BeforeToolSelectionConfig(ToolModeAuto)
}

// BeforeToolSelectionDisableAll disables all tools for the current decision step.
func BeforeToolSelectionDisableAll() *BeforeToolSelectionResponse {
	return BeforeToolSelectionConfig(ToolModeNone)
}

// BeforeAgentContinue returns an explicit no-op BeforeAgent response.
func BeforeAgentContinue() *BeforeAgentResponse {
	return &BeforeAgentResponse{}
}

// BeforeAgentAddContext appends additional context to the current turn prompt.
func BeforeAgentAddContext(context string) *BeforeAgentResponse {
	return &BeforeAgentResponse{AdditionalContext: context}
}

// BeforeAgentAllow returns an explicit allow decision for BeforeAgent.
func BeforeAgentAllow() *BeforeAgentResponse {
	return &BeforeAgentResponse{CommonResponse: CommonResponse{Decision: "allow"}}
}

// BeforeAgentDeny blocks the turn and discards the user's prompt from history.
func BeforeAgentDeny(reason string) *BeforeAgentResponse {
	return &BeforeAgentResponse{CommonResponse: CommonResponse{Decision: "deny", Reason: reason}}
}

// BeforeAgentStop aborts the current turn but keeps the user's prompt in
// history, matching Gemini's continue=false semantics.
func BeforeAgentStop(reason string) *BeforeAgentResponse {
	continueFalse := false
	return &BeforeAgentResponse{CommonResponse: CommonResponse{Continue: &continueFalse, StopReason: reason, Reason: reason}}
}

// AfterAgentContinue returns an explicit no-op AfterAgent response.
func AfterAgentContinue() *AfterAgentResponse {
	return &AfterAgentResponse{}
}

// AfterAgentAllow returns an explicit allow decision for AfterAgent.
func AfterAgentAllow() *AfterAgentResponse {
	return &AfterAgentResponse{CommonResponse: CommonResponse{Decision: "allow"}}
}

// AfterAgentDeny rejects the response and requests a retry.
func AfterAgentDeny(reason string) *AfterAgentResponse {
	return &AfterAgentResponse{CommonResponse: CommonResponse{Decision: "deny", Reason: reason}}
}

// AfterAgentStop stops the session without triggering a retry.
func AfterAgentStop(reason string) *AfterAgentResponse {
	continueFalse := false
	return &AfterAgentResponse{CommonResponse: CommonResponse{Continue: &continueFalse, StopReason: reason}}
}

// AfterAgentClearContext clears LLM conversation memory while preserving the
// UI display.
func AfterAgentClearContext() *AfterAgentResponse {
	return &AfterAgentResponse{ClearContext: true}
}

// BeforeToolContinue returns an explicit no-op BeforeTool response.
func BeforeToolContinue() *BeforeToolResponse {
	return &BeforeToolResponse{}
}

// BeforeToolAllow returns an explicit allow decision for BeforeTool.
func BeforeToolAllow() *BeforeToolResponse {
	return &BeforeToolResponse{CommonResponse: CommonResponse{Decision: "allow"}}
}

// BeforeToolDeny blocks the tool invocation with a deny decision.
func BeforeToolDeny(reason string) *BeforeToolResponse {
	return &BeforeToolResponse{CommonResponse: CommonResponse{Decision: "deny", Reason: reason}}
}

// BeforeToolStop stops the entire Gemini agent loop before the tool executes.
func BeforeToolStop(reason string) *BeforeToolResponse {
	continueFalse := false
	return &BeforeToolResponse{CommonResponse: CommonResponse{Continue: &continueFalse, StopReason: reason}}
}

// BeforeToolRewriteInput continues with a rewritten tool_input payload.
func BeforeToolRewriteInput(input json.RawMessage) *BeforeToolResponse {
	return &BeforeToolResponse{ToolInput: input}
}

// AfterToolContinue returns an explicit no-op AfterTool response.
func AfterToolContinue() *AfterToolResponse {
	return &AfterToolResponse{}
}

// AfterToolAddContext appends additional text to the tool result sent back to
// the agent.
func AfterToolAddContext(context string) *AfterToolResponse {
	return &AfterToolResponse{AdditionalContext: context}
}

// AfterToolAllow returns an explicit allow decision for AfterTool.
func AfterToolAllow() *AfterToolResponse {
	return &AfterToolResponse{CommonResponse: CommonResponse{Decision: "allow"}}
}

// AfterToolDeny blocks the follow-up path with a deny decision.
func AfterToolDeny(reason string) *AfterToolResponse {
	return &AfterToolResponse{CommonResponse: CommonResponse{Decision: "deny", Reason: reason}}
}

// AfterToolStop stops the entire Gemini agent loop after tool execution.
func AfterToolStop(reason string) *AfterToolResponse {
	continueFalse := false
	return &AfterToolResponse{CommonResponse: CommonResponse{Continue: &continueFalse, StopReason: reason}}
}

// AfterToolTailCall requests an immediate follow-up tool invocation.
func AfterToolTailCall(name string, args json.RawMessage) *AfterToolResponse {
	return &AfterToolResponse{
		TailToolCallRequest: &TailToolCallRequest{
			Name: name,
			Args: args,
		},
	}
}
