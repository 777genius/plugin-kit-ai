package hookplex

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/hookplex/hookplex/sdk/claude"
	"github.com/hookplex/hookplex/sdk/codex"
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
		Args: []string{"hookplex", "Stop"},
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
		Args: []string{"hookplex", "notify", `{"client":"codex-tui","ignored":true}`},
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
		Args: []string{"hookplex", "Notification"},
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
		Args: []string{"hookplex", "PermissionRequest"},
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
		Args: []string{"hookplex", "TeamHeartbeat"},
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
	app := New(Config{Name: "t", Args: []string{"hookplex", "Stop"}, IO: &testIO{}, Env: testEnv{}})
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
		Args: []string{"hookplex", "task_event", `{"client":"codex-tui","task":"lint"}`},
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
	app := New(Config{Name: "t", Args: []string{"hookplex", "notify", `{"client":"codex-tui"}`}, IO: &testIO{}, Env: testEnv{}})
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
		Args: []string{"hookplex", "bogus"},
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
		Args: []string{"hookplex", "notify"},
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
		Args: []string{"hookplex", "notify", "{"},
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
		Args: []string{"hookplex", "Stop"},
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
		Args: []string{"hookplex", "Stop"},
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
		Args: []string{"hookplex", "Stop"},
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
