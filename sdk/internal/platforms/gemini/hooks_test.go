package gemini

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/sdk/internal/runtime"
)

func TestDecodeSessionStart(t *testing.T) {
	v, name, err := DecodeSessionStart(runtime.Envelope{
		Stdin: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionStart","source":"startup"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "SessionStart" {
		t.Fatalf("name = %q", name)
	}
	ev, ok := v.(*SessionStartInput)
	if !ok {
		t.Fatalf("type = %T", v)
	}
	if ev.Source != "startup" {
		t.Fatalf("source = %q", ev.Source)
	}
}

func TestDecodeBeforeToolMalformedJSON(t *testing.T) {
	_, _, err := DecodeBeforeTool(runtime.Envelope{Stdin: []byte("{")})
	if err == nil || !strings.Contains(err.Error(), "decode Gemini before tool input") {
		t.Fatalf("err = %v", err)
	}
}

func TestDecodeAfterTool(t *testing.T) {
	v, name, err := DecodeAfterTool(runtime.Envelope{
		Stdin: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterTool","tool_name":"read_file","tool_input":{"path":"README.md"},"tool_response":{"llmContent":"ok"}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if name != "AfterTool" {
		t.Fatalf("name = %q", name)
	}
	ev, ok := v.(*AfterToolInput)
	if !ok {
		t.Fatalf("type = %T", v)
	}
	if ev.ToolName != "read_file" {
		t.Fatalf("tool = %q", ev.ToolName)
	}
	if string(ev.ToolResponse) == "" {
		t.Fatal("tool_response missing")
	}
}

func TestEncodeBeforeToolOutcome(t *testing.T) {
	res := EncodeBeforeTool(BeforeToolOutcome{
		CommonOutcome: CommonOutcome{
			Decision: "deny",
			Reason:   "blocked",
		},
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if got := string(res.Stdout); !strings.Contains(got, `"decision":"deny"`) || !strings.Contains(got, `"reason":"blocked"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestEncodeBeforeToolOutcomeRejectsNonObjectToolInput(t *testing.T) {
	res := EncodeBeforeTool(BeforeToolOutcome{
		ToolInput: json.RawMessage(`["bad"]`),
	})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "hookSpecificOutput.tool_input must be a JSON object") {
		t.Fatalf("stderr = %q", res.Stderr)
	}
}

func TestEncodeSessionStartOutcome(t *testing.T) {
	res := EncodeSessionStart(SessionStartOutcome{
		AdditionalContext: "repo memory",
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if got := string(res.Stdout); !strings.Contains(got, `"hookEventName":"SessionStart"`) || !strings.Contains(got, `"additionalContext":"repo memory"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestEncodeSessionStartOutcomeEmptyIsMinimal(t *testing.T) {
	res := EncodeSessionStart(SessionStartOutcome{})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if got := string(res.Stdout); got != "{}" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestEncodeSessionStartOutcomeIgnoresFlowControlFields(t *testing.T) {
	continueFalse := false
	res := EncodeSessionStart(SessionStartOutcome{
		CommonOutcome: CommonOutcome{
			SystemMessage: "hello",
			Continue:      &continueFalse,
			Decision:      "deny",
			Reason:        "ignored",
			StopReason:    "ignored",
		},
		AdditionalContext: "repo memory",
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	got := string(res.Stdout)
	if !strings.Contains(got, `"systemMessage":"hello"`) || !strings.Contains(got, `"additionalContext":"repo memory"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestEncodeSessionEndOutcomeIgnoresFlowControlFields(t *testing.T) {
	continueFalse := false
	res := EncodeSessionEnd(SessionEndOutcome{
		CommonOutcome: CommonOutcome{
			SystemMessage: "bye",
			Continue:      &continueFalse,
			Decision:      "deny",
			Reason:        "ignored",
			StopReason:    "ignored",
		},
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	got := string(res.Stdout)
	if !strings.Contains(got, `"systemMessage":"bye"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestEncodeAfterToolOutcomeAdditionalContext(t *testing.T) {
	res := EncodeAfterTool(AfterToolOutcome{AdditionalContext: "redacted"})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if got := string(res.Stdout); !strings.Contains(got, `"hookEventName":"AfterTool"`) || !strings.Contains(got, `"additionalContext":"redacted"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestEncodeAfterToolOutcomeTailToolCall(t *testing.T) {
	res := EncodeAfterTool(AfterToolOutcome{
		TailToolCallRequest: &TailToolCallRequest{
			Name: "read_file",
			Args: json.RawMessage(`{"path":"README.md"}`),
		},
	})
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if got := string(res.Stdout); !strings.Contains(got, `"tailToolCallRequest":{"name":"read_file","args":{"path":"README.md"}}`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestEncodeAfterToolOutcomeRejectsTailToolCallWithoutName(t *testing.T) {
	res := EncodeAfterTool(AfterToolOutcome{
		TailToolCallRequest: &TailToolCallRequest{
			Args: json.RawMessage(`{"path":"README.md"}`),
		},
	})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "hookSpecificOutput.tailToolCallRequest.name is required") {
		t.Fatalf("stderr = %q", res.Stderr)
	}
}

func TestEncodeAfterToolOutcomeRejectsNonObjectTailToolCallArgs(t *testing.T) {
	res := EncodeAfterTool(AfterToolOutcome{
		TailToolCallRequest: &TailToolCallRequest{
			Name: "read_file",
			Args: json.RawMessage(`["bad"]`),
		},
	})
	if res.ExitCode != 1 {
		t.Fatalf("exit = %d stderr=%q", res.ExitCode, res.Stderr)
	}
	if !strings.Contains(res.Stderr, "hookSpecificOutput.tailToolCallRequest.args must be a JSON object") {
		t.Fatalf("stderr = %q", res.Stderr)
	}
}
