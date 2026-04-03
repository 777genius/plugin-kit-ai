package gemini

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestSessionStartHelpers(t *testing.T) {
	t.Parallel()

	if got := SessionStartContinue(); got == nil {
		t.Fatal("SessionStartContinue() = nil")
	} else if got.AdditionalContext != "" {
		t.Fatalf("SessionStartContinue().AdditionalContext = %q", got.AdditionalContext)
	}

	if got := SessionStartAddContext("repo memory"); got == nil {
		t.Fatal("SessionStartAddContext() = nil")
	} else if got.AdditionalContext != "repo memory" {
		t.Fatalf("SessionStartAddContext().AdditionalContext = %q", got.AdditionalContext)
	}
}

func TestSessionEndContinue(t *testing.T) {
	t.Parallel()

	if got := SessionEndContinue(); got == nil {
		t.Fatal("SessionEndContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" {
		t.Fatalf("SessionEndContinue() = %#v", got)
	}
}

func TestNotificationContinue(t *testing.T) {
	t.Parallel()

	if got := NotificationContinue(); got == nil {
		t.Fatal("NotificationContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" || got.SystemMessage != "" {
		t.Fatalf("NotificationContinue() = %#v", got)
	}
}

func TestPreCompressContinue(t *testing.T) {
	t.Parallel()

	if got := PreCompressContinue(); got == nil {
		t.Fatal("PreCompressContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" || got.SystemMessage != "" {
		t.Fatalf("PreCompressContinue() = %#v", got)
	}
}

func TestBeforeModelHelpers(t *testing.T) {
	t.Parallel()

	if got := BeforeModelContinue(); got == nil {
		t.Fatal("BeforeModelContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" || len(got.LLMRequest) != 0 || len(got.LLMResponse) != 0 {
		t.Fatalf("BeforeModelContinue() = %#v", got)
	}

	if got := BeforeModelDeny("blocked"); got == nil {
		t.Fatal("BeforeModelDeny() = nil")
	} else if got.Decision != "deny" || got.Reason != "blocked" {
		t.Fatalf("BeforeModelDeny() = %#v", got)
	}

	req := json.RawMessage(`{"model":"gemini-2.5-pro"}`)
	if got := BeforeModelOverrideRequest(req); got == nil {
		t.Fatal("BeforeModelOverrideRequest() = nil")
	} else if !bytes.Equal(got.LLMRequest, req) {
		t.Fatalf("BeforeModelOverrideRequest().LLMRequest = %s", string(got.LLMRequest))
	}

	resp := json.RawMessage(`{"candidates":[{"content":{"role":"model"}}]}`)
	if got := BeforeModelSyntheticResponse(resp); got == nil {
		t.Fatal("BeforeModelSyntheticResponse() = nil")
	} else if !bytes.Equal(got.LLMResponse, resp) {
		t.Fatalf("BeforeModelSyntheticResponse().LLMResponse = %s", string(got.LLMResponse))
	}

	gotReq, err := BeforeModelOverrideRequestValue(map[string]any{"model": "gemini-2.5-pro"})
	if err != nil {
		t.Fatalf("BeforeModelOverrideRequestValue() error = %v", err)
	}
	if gotReq == nil || string(gotReq.LLMRequest) != `{"model":"gemini-2.5-pro"}` {
		t.Fatalf("BeforeModelOverrideRequestValue() = %#v", gotReq)
	}

	gotResp, err := BeforeModelSyntheticResponseValue(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model"}}}})
	if err != nil {
		t.Fatalf("BeforeModelSyntheticResponseValue() error = %v", err)
	}
	if gotResp == nil || string(gotResp.LLMResponse) == "" {
		t.Fatalf("BeforeModelSyntheticResponseValue() = %#v", gotResp)
	}
}

func TestBeforeModelValueHelpersRejectNonObject(t *testing.T) {
	t.Parallel()

	if _, err := BeforeModelOverrideRequestValue([]string{"bad"}); err == nil {
		t.Fatal("BeforeModelOverrideRequestValue() error = nil, want error")
	}
	if _, err := BeforeModelSyntheticResponseValue([]string{"bad"}); err == nil {
		t.Fatal("BeforeModelSyntheticResponseValue() error = nil, want error")
	}
}

func TestAfterModelHelpers(t *testing.T) {
	t.Parallel()

	if got := AfterModelContinue(); got == nil {
		t.Fatal("AfterModelContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" || len(got.LLMResponse) != 0 {
		t.Fatalf("AfterModelContinue() = %#v", got)
	}

	if got := AfterModelDeny("retry"); got == nil {
		t.Fatal("AfterModelDeny() = nil")
	} else if got.Decision != "deny" || got.Reason != "retry" {
		t.Fatalf("AfterModelDeny() = %#v", got)
	}

	resp := json.RawMessage(`{"candidates":[{"content":{"role":"model"}}]}`)
	if got := AfterModelReplaceResponse(resp); got == nil {
		t.Fatal("AfterModelReplaceResponse() = nil")
	} else if !bytes.Equal(got.LLMResponse, resp) {
		t.Fatalf("AfterModelReplaceResponse().LLMResponse = %s", string(got.LLMResponse))
	}

	got, err := AfterModelReplaceResponseValue(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model"}}}})
	if err != nil {
		t.Fatalf("AfterModelReplaceResponseValue() error = %v", err)
	}
	if got == nil || string(got.LLMResponse) == "" {
		t.Fatalf("AfterModelReplaceResponseValue() = %#v", got)
	}
}

func TestAfterModelReplaceResponseValueRejectsNonObject(t *testing.T) {
	t.Parallel()

	if _, err := AfterModelReplaceResponseValue([]string{"bad"}); err == nil {
		t.Fatal("AfterModelReplaceResponseValue() error = nil, want error")
	}
}

func TestBeforeAgentHelpers(t *testing.T) {
	t.Parallel()

	if got := BeforeAgentContinue(); got == nil {
		t.Fatal("BeforeAgentContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" || got.AdditionalContext != "" {
		t.Fatalf("BeforeAgentContinue() = %#v", got)
	}

	if got := BeforeAgentAddContext("repo memory"); got == nil {
		t.Fatal("BeforeAgentAddContext() = nil")
	} else if got.AdditionalContext != "repo memory" {
		t.Fatalf("BeforeAgentAddContext() = %#v", got)
	}

	if got := BeforeAgentAllow(); got == nil {
		t.Fatal("BeforeAgentAllow() = nil")
	} else if got.Decision != "allow" || got.Reason != "" {
		t.Fatalf("BeforeAgentAllow() = %#v", got)
	}

	if got := BeforeAgentDeny("blocked"); got == nil {
		t.Fatal("BeforeAgentDeny() = nil")
	} else if got.Decision != "deny" || got.Reason != "blocked" {
		t.Fatalf("BeforeAgentDeny() = %#v", got)
	}
}

func TestAfterAgentHelpers(t *testing.T) {
	t.Parallel()

	if got := AfterAgentContinue(); got == nil {
		t.Fatal("AfterAgentContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" || got.ClearContext {
		t.Fatalf("AfterAgentContinue() = %#v", got)
	}

	if got := AfterAgentAllow(); got == nil {
		t.Fatal("AfterAgentAllow() = nil")
	} else if got.Decision != "allow" || got.Reason != "" {
		t.Fatalf("AfterAgentAllow() = %#v", got)
	}

	if got := AfterAgentDeny("retry"); got == nil {
		t.Fatal("AfterAgentDeny() = nil")
	} else if got.Decision != "deny" || got.Reason != "retry" {
		t.Fatalf("AfterAgentDeny() = %#v", got)
	}

	if got := AfterAgentClearContext(); got == nil {
		t.Fatal("AfterAgentClearContext() = nil")
	} else if !got.ClearContext {
		t.Fatalf("AfterAgentClearContext() = %#v", got)
	}
}

func TestBeforeToolHelpers(t *testing.T) {
	t.Parallel()

	if got := BeforeToolContinue(); got == nil {
		t.Fatal("BeforeToolContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" || len(got.ToolInput) != 0 {
		t.Fatalf("BeforeToolContinue() = %#v", got)
	}

	if got := BeforeToolAllow(); got == nil {
		t.Fatal("BeforeToolAllow() = nil")
	} else if got.Decision != "allow" || got.Reason != "" {
		t.Fatalf("BeforeToolAllow() = %#v", got)
	}

	if got := BeforeToolDeny("blocked"); got == nil {
		t.Fatal("BeforeToolDeny() = nil")
	} else if got.Decision != "deny" || got.Reason != "blocked" {
		t.Fatalf("BeforeToolDeny() = %#v", got)
	}

	input := json.RawMessage(`{"content":"hello"}`)
	if got := BeforeToolRewriteInput(input); got == nil {
		t.Fatal("BeforeToolRewriteInput() = nil")
	} else if !bytes.Equal(got.ToolInput, input) {
		t.Fatalf("BeforeToolRewriteInput().ToolInput = %s", string(got.ToolInput))
	}

	got, err := BeforeToolRewriteInputValue(map[string]any{"content": "rewritten"})
	if err != nil {
		t.Fatalf("BeforeToolRewriteInputValue() error = %v", err)
	}
	if got == nil {
		t.Fatal("BeforeToolRewriteInputValue() = nil")
	}
	if string(got.ToolInput) != `{"content":"rewritten"}` {
		t.Fatalf("BeforeToolRewriteInputValue().ToolInput = %s", string(got.ToolInput))
	}
}

func TestBeforeToolRewriteInputValueRejectsNonObject(t *testing.T) {
	t.Parallel()

	if _, err := BeforeToolRewriteInputValue([]string{"not", "an", "object"}); err == nil {
		t.Fatal("BeforeToolRewriteInputValue() error = nil, want error")
	}
}

func TestAfterToolHelpers(t *testing.T) {
	t.Parallel()

	if got := AfterToolContinue(); got == nil {
		t.Fatal("AfterToolContinue() = nil")
	} else if got.Decision != "" || got.Reason != "" {
		t.Fatalf("AfterToolContinue() = %#v", got)
	}

	if got := AfterToolAllow(); got == nil {
		t.Fatal("AfterToolAllow() = nil")
	} else if got.Decision != "allow" || got.Reason != "" {
		t.Fatalf("AfterToolAllow() = %#v", got)
	}

	if got := AfterToolDeny("blocked"); got == nil {
		t.Fatal("AfterToolDeny() = nil")
	} else if got.Decision != "deny" || got.Reason != "blocked" {
		t.Fatalf("AfterToolDeny() = %#v", got)
	}

	if got := AfterToolAddContext("redacted"); got == nil {
		t.Fatal("AfterToolAddContext() = nil")
	} else if got.AdditionalContext != "redacted" {
		t.Fatalf("AfterToolAddContext() = %#v", got)
	}

	got, err := AfterToolTailCallValue("read_file", map[string]any{"path": "README.md"})
	if err != nil {
		t.Fatalf("AfterToolTailCallValue() error = %v", err)
	}
	if got == nil || got.TailToolCallRequest == nil {
		t.Fatal("AfterToolTailCallValue() missing tail call")
	}
	if got.TailToolCallRequest.Name != "read_file" {
		t.Fatalf("AfterToolTailCallValue().Name = %q", got.TailToolCallRequest.Name)
	}
	if string(got.TailToolCallRequest.Args) != `{"path":"README.md"}` {
		t.Fatalf("AfterToolTailCallValue().Args = %s", string(got.TailToolCallRequest.Args))
	}
}

func TestAfterToolTailCallValueRejectsNonObject(t *testing.T) {
	t.Parallel()

	if _, err := AfterToolTailCallValue("read_file", []string{"README.md"}); err == nil {
		t.Fatal("AfterToolTailCallValue() error = nil, want error")
	}
}
