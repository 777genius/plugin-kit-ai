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
}
