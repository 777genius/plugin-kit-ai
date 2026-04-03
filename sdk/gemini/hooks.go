package gemini

import (
	"encoding/json"
	"fmt"
	"strings"

	internalgemini "github.com/777genius/plugin-kit-ai/sdk/internal/platforms/gemini"
	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

// SessionStartEvent is the Gemini SessionStart hook input.
type SessionStartEvent = internalgemini.SessionStartInput

// SessionEndEvent is the Gemini SessionEnd hook input.
type SessionEndEvent = internalgemini.SessionEndInput

// NotificationEvent is the Gemini Notification hook input.
type NotificationEvent = internalgemini.NotificationInput

// PreCompressEvent is the Gemini PreCompress hook input.
type PreCompressEvent = internalgemini.PreCompressInput

// BeforeModelEvent is the Gemini BeforeModel hook input.
type BeforeModelEvent = internalgemini.BeforeModelInput

// AfterModelEvent is the Gemini AfterModel hook input.
type AfterModelEvent = internalgemini.AfterModelInput

// BeforeAgentEvent is the Gemini BeforeAgent hook input.
type BeforeAgentEvent = internalgemini.BeforeAgentInput

// AfterAgentEvent is the Gemini AfterAgent hook input.
type AfterAgentEvent = internalgemini.AfterAgentInput

// BeforeToolEvent is the Gemini BeforeTool hook input.
type BeforeToolEvent = internalgemini.BeforeToolInput

// AfterToolEvent is the Gemini AfterTool hook input.
type AfterToolEvent = internalgemini.AfterToolInput

// CommonResponse contains fields shared by Gemini's synchronous hook envelope.
type CommonResponse struct {
	Continue       *bool
	SuppressOutput bool
	StopReason     string
	Decision       string
	Reason         string
	SystemMessage  string
}

// SessionStartResponse is the SessionStart response type.
type SessionStartResponse struct {
	CommonResponse
	AdditionalContext string
}

// SessionEndResponse is the SessionEnd response type.
type SessionEndResponse = CommonResponse

// NotificationResponse is the Notification response type.
type NotificationResponse = CommonResponse

// PreCompressResponse is the PreCompress response type.
type PreCompressResponse = CommonResponse

// BeforeModelResponse is the BeforeModel response type.
type BeforeModelResponse struct {
	CommonResponse
	LLMRequest  json.RawMessage
	LLMResponse json.RawMessage
}

// AfterModelResponse is the AfterModel response type.
type AfterModelResponse struct {
	CommonResponse
	LLMResponse json.RawMessage
}

// BeforeAgentResponse is the BeforeAgent response type.
type BeforeAgentResponse struct {
	CommonResponse
	AdditionalContext string
}

// AfterAgentResponse is the AfterAgent response type.
type AfterAgentResponse struct {
	CommonResponse
	ClearContext bool
}

// BeforeToolResponse is the BeforeTool response type.
type BeforeToolResponse struct {
	CommonResponse
	ToolInput json.RawMessage
}

// TailToolCallRequest requests an immediate follow-up tool execution from an
// AfterTool hook.
type TailToolCallRequest struct {
	Name string
	Args json.RawMessage
}

// AfterToolResponse is the AfterTool response type.
type AfterToolResponse struct {
	CommonResponse
	AdditionalContext   string
	TailToolCallRequest *TailToolCallRequest
}

// SessionStartContinue returns an explicit no-op SessionStart response.
func SessionStartContinue() *SessionStartResponse {
	return &SessionStartResponse{}
}

// SessionStartAddContext appends additional context during SessionStart.
func SessionStartAddContext(context string) *SessionStartResponse {
	return &SessionStartResponse{AdditionalContext: context}
}

// SessionEndContinue returns an explicit no-op SessionEnd response.
func SessionEndContinue() *SessionEndResponse {
	return &SessionEndResponse{}
}

// NotificationContinue returns an explicit no-op Notification response.
func NotificationContinue() *NotificationResponse {
	return &NotificationResponse{}
}

// PreCompressContinue returns an explicit no-op PreCompress response.
func PreCompressContinue() *PreCompressResponse {
	return &PreCompressResponse{}
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

// BeforeModelOverrideRequestValue marshals a replacement llm_request object for
// Gemini BeforeModel hooks.
func BeforeModelOverrideRequestValue(v any) (*BeforeModelResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal Gemini llm_request override: %w", err)
	}
	if !looksLikeJSONObject(body) {
		return nil, fmt.Errorf("marshal Gemini llm_request override: expected JSON object")
	}
	return BeforeModelOverrideRequest(body), nil
}

// BeforeModelSyntheticResponseValue marshals a synthetic llm_response object
// for Gemini BeforeModel hooks.
func BeforeModelSyntheticResponseValue(v any) (*BeforeModelResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal Gemini llm_response override: %w", err)
	}
	if !looksLikeJSONObject(body) {
		return nil, fmt.Errorf("marshal Gemini llm_response override: expected JSON object")
	}
	return BeforeModelSyntheticResponse(body), nil
}

// AfterModelContinue returns an explicit no-op AfterModel response.
func AfterModelContinue() *AfterModelResponse {
	return &AfterModelResponse{}
}

// AfterModelDeny blocks the model result with a deny decision.
func AfterModelDeny(reason string) *AfterModelResponse {
	return &AfterModelResponse{CommonResponse: CommonResponse{Decision: "deny", Reason: reason}}
}

// AfterModelReplaceResponse continues with a rewritten llm_response payload.
func AfterModelReplaceResponse(response json.RawMessage) *AfterModelResponse {
	return &AfterModelResponse{LLMResponse: response}
}

// AfterModelReplaceResponseValue marshals a replacement llm_response object for
// Gemini AfterModel hooks.
func AfterModelReplaceResponseValue(v any) (*AfterModelResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal Gemini llm_response replacement: %w", err)
	}
	if !looksLikeJSONObject(body) {
		return nil, fmt.Errorf("marshal Gemini llm_response replacement: expected JSON object")
	}
	return AfterModelReplaceResponse(body), nil
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

// BeforeToolRewriteInput continues with a rewritten tool_input payload.
func BeforeToolRewriteInput(input json.RawMessage) *BeforeToolResponse {
	return &BeforeToolResponse{ToolInput: input}
}

// BeforeToolRewriteInputValue marshals a replacement tool_input object for
// Gemini BeforeTool hooks. Gemini expects hookSpecificOutput.tool_input to be a
// JSON object, so non-object values return an error.
func BeforeToolRewriteInputValue(v any) (*BeforeToolResponse, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal Gemini tool_input rewrite: %w", err)
	}
	if !looksLikeJSONObject(body) {
		return nil, fmt.Errorf("marshal Gemini tool_input rewrite: expected JSON object")
	}
	return BeforeToolRewriteInput(body), nil
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

// AfterToolTailCall requests an immediate follow-up tool invocation.
func AfterToolTailCall(name string, args json.RawMessage) *AfterToolResponse {
	return &AfterToolResponse{
		TailToolCallRequest: &TailToolCallRequest{
			Name: name,
			Args: args,
		},
	}
}

// AfterToolTailCallValue marshals a typed follow-up tool request. Gemini
// expects tailToolCallRequest.args to be a JSON object, so non-object values
// return an error.
func AfterToolTailCallValue(name string, args any) (*AfterToolResponse, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, fmt.Errorf("marshal Gemini tail tool call args: %w", err)
	}
	if !looksLikeJSONObject(body) {
		return nil, fmt.Errorf("marshal Gemini tail tool call args: expected JSON object")
	}
	return AfterToolTailCall(name, body), nil
}

// Deprecated: prefer BeforeToolAllow, BeforeToolContinue, AfterToolAllow, or
// AfterToolContinue. Gemini handlers return typed response structs, and these
// CommonResponse helpers are kept only for backward compatibility.
//
// AllowTool returns an explicit allow decision for BeforeTool or AfterTool.
func AllowTool() *CommonResponse {
	return &CommonResponse{Decision: "allow"}
}

// Deprecated: prefer BeforeToolDeny or AfterToolDeny. Gemini handlers return
// typed response structs, and this CommonResponse helper is kept only for
// backward compatibility.
//
// DenyTool returns a deny decision with a reason for BeforeTool or AfterTool.
func DenyTool(reason string) *CommonResponse {
	return &CommonResponse{Decision: "deny", Reason: reason}
}

func looksLikeJSONObject(body []byte) bool {
	return strings.HasPrefix(strings.TrimSpace(string(body)), "{")
}

func commonOutcomeFromResponse(r *CommonResponse) internalgemini.CommonOutcome {
	if r == nil {
		return internalgemini.CommonOutcome{}
	}
	return internalgemini.CommonOutcome{
		Continue:       r.Continue,
		SuppressOutput: r.SuppressOutput,
		StopReason:     r.StopReason,
		Decision:       r.Decision,
		Reason:         r.Reason,
		SystemMessage:  r.SystemMessage,
	}
}

func lifecycleOutcomeFromResponse(r *CommonResponse) internalgemini.CommonOutcome {
	out := commonOutcomeFromResponse(r)
	out.Continue = nil
	out.StopReason = ""
	out.Decision = ""
	out.Reason = ""
	return out
}

func sessionStartOutcomeFromResponse(r *SessionStartResponse) internalgemini.SessionStartOutcome {
	if r == nil {
		return internalgemini.SessionStartOutcome{}
	}
	return internalgemini.SessionStartOutcome{
		CommonOutcome:     lifecycleOutcomeFromResponse(&r.CommonResponse),
		AdditionalContext: r.AdditionalContext,
	}
}

func beforeToolOutcomeFromResponse(r *BeforeToolResponse) internalgemini.BeforeToolOutcome {
	if r == nil {
		return internalgemini.BeforeToolOutcome{}
	}
	return internalgemini.BeforeToolOutcome{
		CommonOutcome: commonOutcomeFromResponse(&r.CommonResponse),
		ToolInput:     r.ToolInput,
	}
}

func sessionEndOutcomeFromResponse(r *SessionEndResponse) internalgemini.SessionEndOutcome {
	return internalgemini.SessionEndOutcome{CommonOutcome: lifecycleOutcomeFromResponse(r)}
}

func notificationOutcomeFromResponse(r *NotificationResponse) internalgemini.NotificationOutcome {
	return internalgemini.NotificationOutcome{CommonOutcome: lifecycleOutcomeFromResponse(r)}
}

func preCompressOutcomeFromResponse(r *PreCompressResponse) internalgemini.PreCompressOutcome {
	return internalgemini.PreCompressOutcome{CommonOutcome: lifecycleOutcomeFromResponse(r)}
}

func beforeModelOutcomeFromResponse(r *BeforeModelResponse) internalgemini.BeforeModelOutcome {
	if r == nil {
		return internalgemini.BeforeModelOutcome{}
	}
	return internalgemini.BeforeModelOutcome{
		CommonOutcome: commonOutcomeFromResponse(&r.CommonResponse),
		LLMRequest:    r.LLMRequest,
		LLMResponse:   r.LLMResponse,
	}
}

func afterModelOutcomeFromResponse(r *AfterModelResponse) internalgemini.AfterModelOutcome {
	if r == nil {
		return internalgemini.AfterModelOutcome{}
	}
	return internalgemini.AfterModelOutcome{
		CommonOutcome: commonOutcomeFromResponse(&r.CommonResponse),
		LLMResponse:   r.LLMResponse,
	}
}

func beforeAgentOutcomeFromResponse(r *BeforeAgentResponse) internalgemini.BeforeAgentOutcome {
	if r == nil {
		return internalgemini.BeforeAgentOutcome{}
	}
	return internalgemini.BeforeAgentOutcome{
		CommonOutcome:     commonOutcomeFromResponse(&r.CommonResponse),
		AdditionalContext: r.AdditionalContext,
	}
}

func afterAgentOutcomeFromResponse(r *AfterAgentResponse) internalgemini.AfterAgentOutcome {
	if r == nil {
		return internalgemini.AfterAgentOutcome{}
	}
	return internalgemini.AfterAgentOutcome{
		CommonOutcome: commonOutcomeFromResponse(&r.CommonResponse),
		ClearContext:  r.ClearContext,
	}
}

func afterToolOutcomeFromResponse(r *AfterToolResponse) internalgemini.AfterToolOutcome {
	if r == nil {
		return internalgemini.AfterToolOutcome{}
	}
	out := internalgemini.AfterToolOutcome{
		CommonOutcome:     commonOutcomeFromResponse(&r.CommonResponse),
		AdditionalContext: r.AdditionalContext,
	}
	if r.TailToolCallRequest != nil {
		out.TailToolCallRequest = &internalgemini.TailToolCallRequest{
			Name: r.TailToolCallRequest.Name,
			Args: r.TailToolCallRequest.Args,
		}
	}
	return out
}

func wrapSessionStart(fn func(*SessionStartEvent) *SessionStartResponse) runtime.TypedHandler {
	return wrapGeminiHandler("SessionStart", fn, func(r *SessionStartResponse) any {
		return sessionStartOutcomeFromResponse(r)
	})
}

func wrapSessionEnd(fn func(*SessionEndEvent) *SessionEndResponse) runtime.TypedHandler {
	return wrapGeminiHandler("SessionEnd", fn, func(r *SessionEndResponse) any {
		return sessionEndOutcomeFromResponse(r)
	})
}

func wrapNotification(fn func(*NotificationEvent) *NotificationResponse) runtime.TypedHandler {
	return wrapGeminiHandler("Notification", fn, func(r *NotificationResponse) any {
		return notificationOutcomeFromResponse(r)
	})
}

func wrapPreCompress(fn func(*PreCompressEvent) *PreCompressResponse) runtime.TypedHandler {
	return wrapGeminiHandler("PreCompress", fn, func(r *PreCompressResponse) any {
		return preCompressOutcomeFromResponse(r)
	})
}

func wrapBeforeModel(fn func(*BeforeModelEvent) *BeforeModelResponse) runtime.TypedHandler {
	return wrapGeminiHandler("BeforeModel", fn, func(r *BeforeModelResponse) any {
		return beforeModelOutcomeFromResponse(r)
	})
}

func wrapAfterModel(fn func(*AfterModelEvent) *AfterModelResponse) runtime.TypedHandler {
	return wrapGeminiHandler("AfterModel", fn, func(r *AfterModelResponse) any {
		return afterModelOutcomeFromResponse(r)
	})
}

func wrapBeforeAgent(fn func(*BeforeAgentEvent) *BeforeAgentResponse) runtime.TypedHandler {
	return wrapGeminiHandler("BeforeAgent", fn, func(r *BeforeAgentResponse) any {
		return beforeAgentOutcomeFromResponse(r)
	})
}

func wrapAfterAgent(fn func(*AfterAgentEvent) *AfterAgentResponse) runtime.TypedHandler {
	return wrapGeminiHandler("AfterAgent", fn, func(r *AfterAgentResponse) any {
		return afterAgentOutcomeFromResponse(r)
	})
}

func wrapBeforeTool(fn func(*BeforeToolEvent) *BeforeToolResponse) runtime.TypedHandler {
	return wrapGeminiHandler("BeforeTool", fn, func(r *BeforeToolResponse) any {
		return beforeToolOutcomeFromResponse(r)
	})
}

func wrapAfterTool(fn func(*AfterToolEvent) *AfterToolResponse) runtime.TypedHandler {
	return wrapGeminiHandler("AfterTool", fn, func(r *AfterToolResponse) any {
		return afterToolOutcomeFromResponse(r)
	})
}

func internalgeminiTypeMismatch(name string) error {
	return runtime.InternalHookTypeMismatch(name)
}
