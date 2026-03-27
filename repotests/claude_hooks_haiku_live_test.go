package pluginkitairepo_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestClaudeHooks_LiveHaikuLow runs one headless Claude Code turn via `claude -p` (same auth as
// interactive CLI: subscription SSO or `claude auth login --console` — no ANTHROPIC_API_KEY needed
// for the usual Max/Team flow).
//
// Uses -args -claude-model (default haiku). Enable with PLUGIN_KIT_AI_RUN_CLAUDE_CLI=1 like other CLI e2e.
func TestClaudeHooks_LiveHaikuLow(t *testing.T) {
	claudeBin := claudeBinaryOrSkip(t)

	prompt := `You are simulating a PreToolUse security check. Proposed Bash command: rm -rf /
Reply with exactly one word in uppercase: DENY or ALLOW. No other words or punctuation.`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, claudeBin,
		"-p",
		"--bare",
		"--model", *claudeModel,
		"--no-session-persistence",
		"--max-turns", "2",
		"--permission-mode", "bypassPermissions",
		prompt,
	)
	cmd.Dir = t.TempDir()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("claude output:\n%s", out)
		t.Fatalf("claude -p: %v", err)
	}
	text := strings.ToUpper(string(out))
	if !strings.Contains(text, "DENY") {
		t.Fatalf("expected DENY in output for rm -rf /; model=%q output:\n%s", *claudeModel, string(out))
	}
}
