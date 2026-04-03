package runtime

import "testing"

func TestCanonicalInvocationNameGemini(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"GeminiSessionStart":        "SessionStart",
		"GeminiSessionEnd":          "SessionEnd",
		"GeminiBeforeModel":         "BeforeModel",
		"GeminiAfterModel":          "AfterModel",
		"GeminiBeforeToolSelection": "BeforeToolSelection",
		"GeminiBeforeAgent":         "BeforeAgent",
		"GeminiAfterAgent":          "AfterAgent",
		"GeminiBeforeTool":          "BeforeTool",
		"GeminiAfterTool":           "AfterTool",
	}
	for raw, want := range cases {
		if got := CanonicalInvocationName("gemini", raw); got != want {
			t.Fatalf("%s => %s, want %s", raw, got, want)
		}
	}
}

func TestAttachMismatchWarningUsesCanonicalInvocationName(t *testing.T) {
	t.Parallel()
	res := attachMismatchWarning(Result{ExitCode: 0, Stdout: []byte("{}")}, "gemini", "GeminiSessionStart", "SessionStart")
	if res.Stderr != "" {
		t.Fatalf("stderr = %q", res.Stderr)
	}
}
