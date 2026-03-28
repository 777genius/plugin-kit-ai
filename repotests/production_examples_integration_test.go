package pluginkitairepo_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProductionExamples_RenderValidateBuildAndSmoke(t *testing.T) {
	root := RepoRoot(t)
	pluginKitAIBin := buildPluginKitAI(t)

	cases := []struct {
		name     string
		dir      string
		platform string
		binary   string
		smoke    func(t *testing.T, binary string)
	}{
		{
			name:     "claude",
			dir:      filepath.Join(root, "examples", "plugins", "claude-basic-prod"),
			platform: "claude",
			binary:   "claude-basic-prod",
			smoke:    smokeClaudeStableSubset,
		},
		{
			name:     "codex",
			dir:      filepath.Join(root, "examples", "plugins", "codex-basic-prod"),
			platform: "codex",
			binary:   "codex-basic-prod",
			smoke:    smokeCodexNotify,
		},
		{
			name:     "gemini",
			dir:      filepath.Join(root, "examples", "plugins", "gemini-extension-package"),
			platform: "gemini",
			binary:   "gemini-extension-package",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runCmd(t, root, exec.Command(pluginKitAIBin, "render", tc.dir, "--check"))
			runCmd(t, root, exec.Command(pluginKitAIBin, "validate", tc.dir, "--platform", tc.platform, "--strict"))
			if tc.platform == "codex" {
				assertCodexConfig(t, tc.dir, "gpt-5.4-mini", "./bin/codex-basic-prod")
			}

			testCmd := exec.Command("go", "test", "./...")
			testCmd.Dir = tc.dir
			testCmd.Env = append(os.Environ(), "GOWORK=off")
			runCmd(t, root, testCmd)

			binDir := filepath.Join(tc.dir, "bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				t.Fatal(err)
			}
			binName := tc.binary
			if runtime.GOOS == "windows" {
				binName += ".exe"
			}
			binPath := filepath.Join(binDir, binName)
			buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/"+tc.binary)
			buildCmd.Dir = tc.dir
			buildCmd.Env = append(os.Environ(), "GOWORK=off")
			runCmd(t, root, buildCmd)

			if tc.smoke != nil {
				tc.smoke(t, binPath)
			}
		})
	}
}

func smokeClaudeStableSubset(t *testing.T, binary string) {
	t.Helper()
	cases := []struct {
		name    string
		payload string
	}{
		{name: "Stop", payload: `{"session_id":"s","cwd":"/tmp","hook_event_name":"Stop"}`},
		{name: "PreToolUse", payload: `{"session_id":"e2e-session","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"toolu_e2e","tool_input":{"command":"echo ok"}}`},
		{name: "UserPromptSubmit", payload: `{"session_id":"e2e-session","transcript_path":"/tmp/t.jsonl","cwd":"/tmp","permission_mode":"default","hook_event_name":"UserPromptSubmit","prompt":"hello e2e"}`},
	}
	for _, tc := range cases {
		cmd := exec.Command(binary, tc.name)
		cmd.Stdin = bytes.NewBufferString(tc.payload)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("claude smoke %s: %v\n%s", tc.name, err, out)
		}
		if string(bytes.TrimSpace(out)) != "{}" {
			t.Fatalf("claude smoke %s stdout = %q", tc.name, out)
		}
	}
}

func smokeCodexNotify(t *testing.T, binary string) {
	t.Helper()
	cmd := exec.Command(binary, "notify", `{"client":"codex-tui"}`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("codex smoke: %v\n%s", err, out)
	}
	if len(bytes.TrimSpace(out)) != 0 {
		t.Fatalf("codex smoke stdout = %q", out)
	}
}

func runCmd(t *testing.T, root string, cmd *exec.Cmd) {
	t.Helper()
	if cmd.Dir == "" {
		cmd.Dir = root
	}
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s: %v\n%s", cmd.String(), err, out)
	}
}
