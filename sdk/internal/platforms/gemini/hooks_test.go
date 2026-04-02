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
