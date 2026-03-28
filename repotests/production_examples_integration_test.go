package pluginkitairepo_test

import (
	"bytes"
	"encoding/json"
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
		buildGo  bool
		smoke    func(t *testing.T, binary string)
	}{
		{
			name:     "claude",
			dir:      filepath.Join(root, "examples", "plugins", "claude-basic-prod"),
			platform: "claude",
			binary:   "claude-basic-prod",
			buildGo:  true,
			smoke:    smokeClaudeStableSubset,
		},
		{
			name:     "codex",
			dir:      filepath.Join(root, "examples", "plugins", "codex-basic-prod"),
			platform: "codex-runtime",
			binary:   "codex-basic-prod",
			buildGo:  true,
			smoke:    smokeCodexNotify,
		},
		{
			name:     "codex-package",
			dir:      filepath.Join(root, "examples", "plugins", "codex-package-prod"),
			platform: "codex-package",
		},
		{
			name:     "gemini",
			dir:      filepath.Join(root, "examples", "plugins", "gemini-extension-package"),
			platform: "gemini",
		},
		{
			name:     "opencode",
			dir:      filepath.Join(root, "examples", "plugins", "opencode-basic"),
			platform: "opencode",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runCmd(t, root, exec.Command(pluginKitAIBin, "render", tc.dir, "--check"))
			runCmd(t, root, exec.Command(pluginKitAIBin, "validate", tc.dir, "--platform", tc.platform, "--strict"))
			if tc.platform == "codex-runtime" {
				assertCodexConfig(t, tc.dir, "gpt-5.4-mini", "./bin/codex-basic-prod")
			}
			if tc.platform == "codex-package" {
				assertCodexPackageManifest(t, tc.dir, "codex-package-prod")
			}
			if tc.platform == "gemini" {
				return
			}
			if tc.platform == "opencode" {
				assertOpenCodeConfig(t, tc.dir, "@acme/opencode-demo-plugin")
				return
			}
			if !tc.buildGo {
				return
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

func assertOpenCodeConfig(t *testing.T, root, wantPlugin string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Schema string         `json:"$schema"`
		Plugin []string       `json:"plugin"`
		MCP    map[string]any `json:"mcp"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("parse opencode config: %v\n%s", err, body)
	}
	if doc.Schema != "https://opencode.ai/config.json" {
		t.Fatalf("opencode.json schema = %q", doc.Schema)
	}
	if len(doc.Plugin) != 1 || doc.Plugin[0] != wantPlugin {
		t.Fatalf("opencode.json plugin = %v want [%q]", doc.Plugin, wantPlugin)
	}
	if len(doc.MCP) == 0 {
		t.Fatalf("opencode.json missing mcp config: %s", body)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "skills", "opencode-basic", "SKILL.md")); err != nil {
		t.Fatalf("stat mirrored opencode skill: %v", err)
	}
	for _, rel := range []string{
		filepath.Join(".opencode", "commands", "ship.md"),
		filepath.Join(".opencode", "agents", "reviewer.md"),
		filepath.Join(".opencode", "themes", "midnight.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func assertCodexPackageManifest(t *testing.T, root, wantName string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		t.Fatalf("parse codex package manifest: %v\n%s", err, body)
	}
	if manifest.Name != wantName {
		t.Fatalf(".codex-plugin/plugin.json name = %q want %q", manifest.Name, wantName)
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
