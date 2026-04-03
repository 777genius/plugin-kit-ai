package pluginkitai

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/sdk/claude"
	"github.com/777genius/plugin-kit-ai/sdk/codex"
	"github.com/777genius/plugin-kit-ai/sdk/gemini"
)

type testIO struct {
	in  []byte
	out bytes.Buffer
	err bytes.Buffer
}

func (t *testIO) ReadStdin(ctx context.Context) ([]byte, error) {
	return t.in, ctx.Err()
}

func (t *testIO) WriteStdout(b []byte) error {
	_, err := t.out.Write(b)
	return err
}

func (t *testIO) WriteStderr(s string) error {
	_, err := t.err.WriteString(s)
	return err
}

type testEnv map[string]string

func (e testEnv) LookupEnv(k string) (string, bool) {
	v, ok := e[k]
	return v, ok
}

func TestApp_ClaudeStop(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Stop"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "Stop"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response {
		return claude.Allow()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_CodexNotify(t *testing.T) {
	iox := &testIO{}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "notify", `{"client":"codex-tui","ignored":true}`},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Codex().OnNotify(func(e *codex.NotifyEvent) *codex.Response {
		if e.Client != "codex-tui" {
			t.Fatalf("client = %q", e.Client)
		}
		if string(e.RawJSON()) == "" {
			t.Fatal("raw json missing")
		}
		return codex.Continue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if iox.out.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", iox.out.String())
	}
}

func TestApp_GeminiSessionStart(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionStart","source":"startup"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiSessionStart"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnSessionStart(func(e *gemini.SessionStartEvent) *gemini.SessionStartResponse {
		if e.Source != "startup" {
			t.Fatalf("source = %q", e.Source)
		}
		return &gemini.SessionStartResponse{AdditionalContext: "repo memory"}
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"SessionStart"`) || !strings.Contains(got, `"additionalContext":"repo memory"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiSessionStartContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionStart","source":"startup"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiSessionStart"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnSessionStart(func(*gemini.SessionStartEvent) *gemini.SessionStartResponse {
		return gemini.SessionStartContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiSessionStartAddContextEncodesHookSpecificOutput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionStart","source":"startup"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiSessionStart"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnSessionStart(func(*gemini.SessionStartEvent) *gemini.SessionStartResponse {
		return gemini.SessionStartAddContext("repo memory")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"SessionStart"`) || !strings.Contains(got, `"additionalContext":"repo memory"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiSessionStartMessageEncodesSystemMessage(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionStart","source":"startup"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiSessionStart"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnSessionStart(func(*gemini.SessionStartEvent) *gemini.SessionStartResponse {
		return gemini.SessionStartMessage("hello")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"systemMessage":"hello"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiSessionStartIgnoresFlowControlFields(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionStart","source":"startup"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiSessionStart"},
		IO:   iox,
		Env:  testEnv{},
	})
	continueFalse := false
	app.Gemini().OnSessionStart(func(*gemini.SessionStartEvent) *gemini.SessionStartResponse {
		return &gemini.SessionStartResponse{
			CommonResponse: gemini.CommonResponse{
				SystemMessage: "hello",
				Continue:      &continueFalse,
				Decision:      "deny",
				Reason:        "ignored",
				StopReason:    "ignored",
			},
			AdditionalContext: "repo memory",
		}
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"systemMessage":"hello"`) || !strings.Contains(got, `"additionalContext":"repo memory"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestApp_GeminiSessionEndContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionEnd","reason":"prompt_input_exit"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiSessionEnd"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnSessionEnd(func(*gemini.SessionEndEvent) *gemini.SessionEndResponse {
		return gemini.SessionEndContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiSessionEndIgnoresFlowControlFields(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"SessionEnd","reason":"exit"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiSessionEnd"},
		IO:   iox,
		Env:  testEnv{},
	})
	continueFalse := false
	app.Gemini().OnSessionEnd(func(*gemini.SessionEndEvent) *gemini.SessionEndResponse {
		return &gemini.SessionEndResponse{
			SystemMessage: "bye",
			Continue:      &continueFalse,
			Decision:      "deny",
			Reason:        "ignored",
			StopReason:    "ignored",
		}
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"systemMessage":"bye"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestApp_GeminiNotificationMessageEncodesSystemMessage(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Notification","notification_type":"ToolPermission","message":"approve?","details":{"tool_name":"read_file"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiNotification"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnNotification(func(*gemini.NotificationEvent) *gemini.NotificationResponse {
		return gemini.NotificationMessage("heads up")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"systemMessage":"heads up"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestApp_GeminiPreCompressMessageEncodesSystemMessage(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"PreCompress","trigger":"auto"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiPreCompress"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnPreCompress(func(*gemini.PreCompressEvent) *gemini.PreCompressResponse {
		return gemini.PreCompressMessage("compressing")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"systemMessage":"compressing"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestApp_GeminiNotificationContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Notification","notification_type":"ToolPermission","message":"approve?","details":{"tool_name":"read_file"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiNotification"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnNotification(func(e *gemini.NotificationEvent) *gemini.NotificationResponse {
		if e.NotificationType != "ToolPermission" {
			t.Fatalf("notification type = %q", e.NotificationType)
		}
		return gemini.NotificationContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiNotificationIgnoresFlowControlFields(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Notification","notification_type":"ToolPermission","message":"approve?","details":{"tool_name":"read_file"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiNotification"},
		IO:   iox,
		Env:  testEnv{},
	})
	continueFalse := false
	app.Gemini().OnNotification(func(*gemini.NotificationEvent) *gemini.NotificationResponse {
		return &gemini.NotificationResponse{
			SystemMessage: "heads up",
			Continue:      &continueFalse,
			Decision:      "deny",
			Reason:        "ignored",
			StopReason:    "ignored",
		}
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"systemMessage":"heads up"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestApp_GeminiPreCompressContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"PreCompress","trigger":"auto"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiPreCompress"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnPreCompress(func(e *gemini.PreCompressEvent) *gemini.PreCompressResponse {
		if e.Trigger != "auto" {
			t.Fatalf("trigger = %q", e.Trigger)
		}
		return gemini.PreCompressContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiPreCompressIgnoresFlowControlFields(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"PreCompress","trigger":"manual"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiPreCompress"},
		IO:   iox,
		Env:  testEnv{},
	})
	continueFalse := false
	app.Gemini().OnPreCompress(func(*gemini.PreCompressEvent) *gemini.PreCompressResponse {
		return &gemini.PreCompressResponse{
			SystemMessage: "compressing",
			Continue:      &continueFalse,
			Decision:      "deny",
			Reason:        "ignored",
			StopReason:    "ignored",
		}
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"systemMessage":"compressing"`) {
		t.Fatalf("stdout = %q", got)
	}
	for _, unwanted := range []string{`"continue":`, `"decision":`, `"reason":`, `"stopReason":`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("stdout unexpectedly contains %q: %s", unwanted, got)
		}
	}
}

func TestApp_GeminiBeforeModelContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeModel","llm_request":{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeModel"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeModel(func(e *gemini.BeforeModelEvent) *gemini.BeforeModelResponse {
		if string(e.LLMRequest) == "" {
			t.Fatal("llm_request missing")
		}
		return gemini.BeforeModelContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiBeforeModelOverrideRequest(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeModel","llm_request":{"model":"gemini-2.5-pro"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeModel"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeModel(func(*gemini.BeforeModelEvent) *gemini.BeforeModelResponse {
		resp, err := gemini.BeforeModelOverrideRequestValue(map[string]any{"model": "gemini-2.5-flash"})
		if err != nil {
			t.Fatalf("BeforeModelOverrideRequestValue() error = %v", err)
		}
		return resp
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"BeforeModel"`) || !strings.Contains(got, `"llm_request":{"model":"gemini-2.5-flash"}`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeModelSyntheticResponse(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeModel","llm_request":{"model":"gemini-2.5-pro"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeModel"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeModel(func(*gemini.BeforeModelEvent) *gemini.BeforeModelResponse {
		resp, err := gemini.BeforeModelSyntheticResponseValue(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model"}}}})
		if err != nil {
			t.Fatalf("BeforeModelSyntheticResponseValue() error = %v", err)
		}
		return resp
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"BeforeModel"`) || !strings.Contains(got, `"llm_response":`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiAfterModelContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterModel","llm_request":{"model":"gemini-2.5-pro"},"llm_response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterModel"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterModel(func(e *gemini.AfterModelEvent) *gemini.AfterModelResponse {
		if string(e.LLMResponse) == "" {
			t.Fatal("llm_response missing")
		}
		return gemini.AfterModelContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiAfterModelReplaceResponse(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterModel","llm_request":{"model":"gemini-2.5-pro"},"llm_response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterModel"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterModel(func(*gemini.AfterModelEvent) *gemini.AfterModelResponse {
		resp, err := gemini.AfterModelReplaceResponseValue(map[string]any{"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": "rewritten"}}}}}})
		if err != nil {
			t.Fatalf("AfterModelReplaceResponseValue() error = %v", err)
		}
		return resp
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"AfterModel"`) || !strings.Contains(got, `"rewritten"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeToolSelectionContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeToolSelection","llm_request":{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"hi"}]}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeToolSelection"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeToolSelection(func(e *gemini.BeforeToolSelectionEvent) *gemini.BeforeToolSelectionResponse {
		if string(e.LLMRequest) == "" {
			t.Fatal("llm_request missing")
		}
		return gemini.BeforeToolSelectionContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiBeforeToolSelectionConfig(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeToolSelection","llm_request":{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"read the repo"}]}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeToolSelection"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeToolSelection(func(*gemini.BeforeToolSelectionEvent) *gemini.BeforeToolSelectionResponse {
		return gemini.BeforeToolSelectionConfig(gemini.ToolModeAny, "read_file", "list_directory", "read_file")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"hookEventName":"BeforeToolSelection"`) || !strings.Contains(got, `"mode":"ANY"`) {
		t.Fatalf("stdout = %q", got)
	}
	if !strings.Contains(got, `"allowedFunctionNames":["read_file","list_directory"]`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeToolSelectionAllowOnly(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeToolSelection","llm_request":{"model":"gemini-2.5-pro","messages":[{"role":"user","content":"read the repo"}]}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeToolSelection"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeToolSelection(func(*gemini.BeforeToolSelectionEvent) *gemini.BeforeToolSelectionResponse {
		return gemini.BeforeToolSelectionAllowOnly("read_file", "list_directory", "read_file")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"hookEventName":"BeforeToolSelection"`) {
		t.Fatalf("stdout = %q", got)
	}
	if strings.Contains(got, `"mode":`) {
		t.Fatalf("stdout unexpectedly contains mode: %s", got)
	}
	if !strings.Contains(got, `"allowedFunctionNames":["read_file","list_directory"]`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiAfterModelStopEncodesContinueFalse(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterModel","llm_request":{"model":"gemini-2.5-pro"},"llm_response":{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterModel"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterModel(func(*gemini.AfterModelEvent) *gemini.AfterModelResponse {
		return gemini.AfterModelStop("halt")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"continue":false`) || !strings.Contains(got, `"stopReason":"halt"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeAgentStopEncodesContinueFalse(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeAgent","prompt":"hello"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeAgent"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeAgent(func(*gemini.BeforeAgentEvent) *gemini.BeforeAgentResponse {
		return gemini.BeforeAgentStop("pause")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"continue":false`) || !strings.Contains(got, `"stopReason":"pause"`) || !strings.Contains(got, `"reason":"pause"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeToolStopEncodesContinueFalse(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeTool","tool_name":"read_file","tool_input":{"path":"README.md"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeTool(func(*gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		return gemini.BeforeToolStop("halt")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	got := iox.out.String()
	if !strings.Contains(got, `"continue":false`) || !strings.Contains(got, `"stopReason":"halt"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeAgentContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeAgent","prompt":"hello"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeAgent"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeAgent(func(e *gemini.BeforeAgentEvent) *gemini.BeforeAgentResponse {
		if e.Prompt != "hello" {
			t.Fatalf("prompt = %q", e.Prompt)
		}
		return gemini.BeforeAgentContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiBeforeAgentAddContextEncodesHookSpecificOutput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeAgent","prompt":"hello"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeAgent"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeAgent(func(*gemini.BeforeAgentEvent) *gemini.BeforeAgentResponse {
		return gemini.BeforeAgentAddContext("repo memory")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"BeforeAgent"`) || !strings.Contains(got, `"additionalContext":"repo memory"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiAfterAgentContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterAgent","prompt":"hello","prompt_response":"ok","stop_hook_active":false}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterAgent"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterAgent(func(e *gemini.AfterAgentEvent) *gemini.AfterAgentResponse {
		if e.Prompt != "hello" || e.PromptResponse != "ok" {
			t.Fatalf("event = %#v", e)
		}
		return gemini.AfterAgentContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiAfterAgentClearContextEncodesHookSpecificOutput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterAgent","prompt":"hello","prompt_response":"ok","stop_hook_active":true}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterAgent"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterAgent(func(*gemini.AfterAgentEvent) *gemini.AfterAgentResponse {
		return gemini.AfterAgentClearContext()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"AfterAgent"`) || !strings.Contains(got, `"clearContext":true`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeTool(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"content":"hello"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeTool(func(e *gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		if e.ToolName != "write_file" {
			t.Fatalf("tool = %q", e.ToolName)
		}
		return &gemini.BeforeToolResponse{
			CommonResponse: gemini.CommonResponse{Decision: "deny", Reason: "blocked"},
		}
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"decision":"deny"`) || !strings.Contains(got, `"reason":"blocked"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeToolContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"content":"hello"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeTool(func(*gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		return gemini.BeforeToolContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiBeforeToolAllowIsExplicit(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"content":"hello"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeTool(func(*gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		return gemini.BeforeToolAllow()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"decision":"allow"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeToolRewriteInputEncodesHookSpecificOutput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"content":"hello"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeTool(func(*gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		resp, err := gemini.BeforeToolRewriteInputValue(map[string]any{"content": "rewritten"})
		if err != nil {
			t.Fatalf("BeforeToolRewriteInputValue() error = %v", err)
		}
		return resp
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"BeforeTool"`) || !strings.Contains(got, `"tool_input":{"content":"rewritten"}`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiBeforeToolRejectsNonObjectRewriteInput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"content":"hello"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeTool(func(*gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		return gemini.BeforeToolRewriteInput([]byte(`["bad"]`))
	})
	if c := app.Run(); c != 1 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.err.String(); !strings.Contains(got, "hookSpecificOutput.tool_input must be a JSON object") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestApp_GeminiBeforeToolRejectsMalformedRewriteInput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"BeforeTool","tool_name":"write_file","tool_input":{"content":"hello"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiBeforeTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnBeforeTool(func(*gemini.BeforeToolEvent) *gemini.BeforeToolResponse {
		return gemini.BeforeToolRewriteInput([]byte(`{"bad":`))
	})
	if c := app.Run(); c != 1 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.err.String(); !strings.Contains(got, "hookSpecificOutput.tool_input must be valid JSON object") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestApp_GeminiAfterToolContinueIsMinimal(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterTool","tool_name":"write_file","tool_input":{"content":"hello"},"tool_response":{"llmContent":"ok","returnDisplay":"ok"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterTool(func(e *gemini.AfterToolEvent) *gemini.AfterToolResponse {
		if e.ToolName != "write_file" {
			t.Fatalf("tool = %q", e.ToolName)
		}
		if string(e.ToolResponse) == "" {
			t.Fatal("tool_response missing")
		}
		return gemini.AfterToolContinue()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q, want {}", got)
	}
}

func TestApp_GeminiAfterToolAllowIsExplicit(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterTool","tool_name":"write_file","tool_input":{"content":"hello"},"tool_response":{"llmContent":"ok","returnDisplay":"ok"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterTool(func(e *gemini.AfterToolEvent) *gemini.AfterToolResponse {
		if string(e.ToolResponse) == "" {
			t.Fatal("tool_response missing")
		}
		return gemini.AfterToolAllow()
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"decision":"allow"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiAfterToolAddContextEncodesHookSpecificOutput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterTool","tool_name":"write_file","tool_input":{"content":"hello"},"tool_response":{"llmContent":"ok","returnDisplay":"ok"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterTool(func(*gemini.AfterToolEvent) *gemini.AfterToolResponse {
		return gemini.AfterToolAddContext("redacted details")
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"AfterTool"`) || !strings.Contains(got, `"additionalContext":"redacted details"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiAfterToolTailCallEncodesHookSpecificOutput(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterTool","tool_name":"write_file","tool_input":{"content":"hello"},"tool_response":{"llmContent":"ok","returnDisplay":"ok"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterTool(func(*gemini.AfterToolEvent) *gemini.AfterToolResponse {
		resp, err := gemini.AfterToolTailCallValue("read_file", map[string]any{"path": "README.md"})
		if err != nil {
			t.Fatalf("AfterToolTailCallValue() error = %v", err)
		}
		return resp
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"tailToolCallRequest":{"name":"read_file","args":{"path":"README.md"}}`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_GeminiAfterToolRejectsInvalidTailCall(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterTool","tool_name":"write_file","tool_input":{"content":"hello"},"tool_response":{"llmContent":"ok","returnDisplay":"ok"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterTool(func(*gemini.AfterToolEvent) *gemini.AfterToolResponse {
		return gemini.AfterToolTailCall("", []byte(`["bad"]`))
	})
	if c := app.Run(); c != 1 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.err.String(); !strings.Contains(got, "hookSpecificOutput.tailToolCallRequest.name is required") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestApp_GeminiAfterToolRejectsMalformedTailCallArgs(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"AfterTool","tool_name":"write_file","tool_input":{"content":"hello"},"tool_response":{"llmContent":"ok","returnDisplay":"ok"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "GeminiAfterTool"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Gemini().OnAfterTool(func(*gemini.AfterToolEvent) *gemini.AfterToolResponse {
		return gemini.AfterToolTailCall("read_file", []byte(`{"path":`))
	})
	if c := app.Run(); c != 1 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.err.String(); !strings.Contains(got, "hookSpecificOutput.tailToolCallRequest.args must be valid JSON object") {
		t.Fatalf("stderr = %q", got)
	}
}

type customClaudeEvent struct {
	HookEventName string `json:"hook_event_name"`
	Message       string `json:"message"`
}

type customCodexEvent struct {
	Client string `json:"client"`
	Task   string `json:"task"`
}

func TestApp_ClaudeNotification(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Notification","message":"done","notification_type":"info"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "Notification"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Claude().OnNotification(func(e *claude.NotificationEvent) *claude.NotificationResponse {
		if e.Message != "done" {
			t.Fatalf("message = %q", e.Message)
		}
		return &claude.NotificationResponse{AdditionalContext: "acked"}
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"Notification"`) || !strings.Contains(got, `"additionalContext":"acked"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_ClaudePermissionRequest(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"rm -rf /tmp/demo"}}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "PermissionRequest"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Claude().OnPermissionRequest(func(e *claude.PermissionRequestEvent) *claude.PermissionRequestResponse {
		if e.ToolName != "Bash" {
			t.Fatalf("tool = %q", e.ToolName)
		}
		return claude.PermissionBlock("needs approval", true)
	})
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"PermissionRequest"`) || !strings.Contains(got, `"behavior":"deny"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_ClaudeCustomContextHook(t *testing.T) {
	iox := &testIO{in: []byte(`{"hook_event_name":"TeamHeartbeat","message":"ping"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "TeamHeartbeat"},
		IO:   iox,
		Env:  testEnv{},
	})
	if err := claude.RegisterCustomContextJSON(app.Claude(), "TeamHeartbeat", func(e *customClaudeEvent) *claude.ContextResponse {
		if e.Message != "ping" {
			t.Fatalf("message = %q", e.Message)
		}
		return &claude.ContextResponse{AdditionalContext: "pong"}
	}); err != nil {
		t.Fatalf("register custom hook: %v", err)
	}
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); !strings.Contains(got, `"hookEventName":"TeamHeartbeat"`) || !strings.Contains(got, `"additionalContext":"pong"`) {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_ClaudeCustomHookRejectsBuiltinName(t *testing.T) {
	app := New(Config{Name: "t", Args: []string{"plugin-kit-ai", "Stop"}, IO: &testIO{}, Env: testEnv{}})
	err := claude.RegisterCustomContextJSON(app.Claude(), "Stop", func(*customClaudeEvent) *claude.ContextResponse {
		return nil
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if got := err.Error(); !strings.Contains(got, "conflicts with built-in descriptor") {
		t.Fatalf("err = %q", got)
	}
}

func TestApp_CodexCustomJSONHook(t *testing.T) {
	iox := &testIO{}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "task_event", `{"client":"codex-tui","task":"lint"}`},
		IO:   iox,
		Env:  testEnv{},
	})
	if err := codex.RegisterCustomJSON(app.Codex(), "task_event", func(e *customCodexEvent) *codex.Response {
		if e.Client != "codex-tui" || e.Task != "lint" {
			t.Fatalf("event = %+v", *e)
		}
		return codex.Continue()
	}); err != nil {
		t.Fatalf("register custom codex hook: %v", err)
	}
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if iox.out.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", iox.out.String())
	}
}

func TestApp_CodexCustomHookRejectsBuiltinName(t *testing.T) {
	app := New(Config{Name: "t", Args: []string{"plugin-kit-ai", "notify", `{"client":"codex-tui"}`}, IO: &testIO{}, Env: testEnv{}})
	err := codex.RegisterCustomJSON(app.Codex(), "notify", func(*customCodexEvent) *codex.Response {
		return codex.Continue()
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if got := err.Error(); !strings.Contains(got, "conflicts with built-in invocation") && !strings.Contains(got, "conflicts with built-in descriptor") {
		t.Fatalf("err = %q", got)
	}
}

func TestApp_UnknownInvocation(t *testing.T) {
	iox := &testIO{}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "bogus"},
		IO:   iox,
		Env:  testEnv{},
	})
	if c := app.Run(); c != 1 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.err.String(); got != "unknown invocation \"bogus\"\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestApp_CodexNotifyMissingPayload(t *testing.T) {
	iox := &testIO{}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "notify"},
		IO:   iox,
		Env:  testEnv{},
	})
	if c := app.Run(); c != 1 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.err.String(); got != "decode codex notify input: missing JSON payload argument\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestApp_CodexNotifyMalformedPayload(t *testing.T) {
	iox := &testIO{}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "notify", "{"},
		IO:   iox,
		Env:  testEnv{},
	})
	if c := app.Run(); c != 1 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.err.String(); !strings.Contains(got, "decode codex notify input:") {
		t.Fatalf("stderr = %q", got)
	}
}

func TestApp_RegisterAfterRunPanics(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Stop"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "Stop"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response { return claude.Allow() })
	_ = app.Run()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response { return claude.Allow() })
}

func TestApp_LastRegistrationWins(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Stop"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "Stop"},
		IO:   iox,
		Env:  testEnv{},
	})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response { return claude.Block("first") })
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response { return claude.Allow() })
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d stderr=%q", c, iox.err.String())
	}
	if got := iox.out.String(); got != "{}" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestApp_MiddlewareRuns(t *testing.T) {
	iox := &testIO{in: []byte(`{"session_id":"s","cwd":"/","hook_event_name":"Stop"}`)}
	app := New(Config{
		Name: "t",
		Args: []string{"plugin-kit-ai", "Stop"},
		IO:   iox,
		Env:  testEnv{},
	})
	var ran bool
	app.Use(func(next Next) Next {
		return func(ctx InvocationContext) Handled {
			ran = true
			return next(ctx)
		}
	})
	app.Claude().OnStop(func(*claude.StopEvent) *claude.Response { return claude.Allow() })
	if c := app.Run(); c != 0 {
		t.Fatalf("exit %d", c)
	}
	if !ran {
		t.Fatal("middleware did not run")
	}
}
