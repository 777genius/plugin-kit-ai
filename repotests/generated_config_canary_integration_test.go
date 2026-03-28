package pluginkitairepo_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGeneratedConfigCanaries_ClaudeStableHookSubsetAndCommandShape(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := initGeneratedCanaryProject(t, pluginKitAIBin, "claude")

	runPluginKitAICommand(t, pluginKitAIBin, "render", plugRoot, "--check")

	body, err := os.ReadFile(filepath.Join(plugRoot, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hooksFile struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string `json:"type"`
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &hooksFile); err != nil {
		t.Fatalf("parse hooks/hooks.json: %v\n%s", err, body)
	}

	gotNames := sortedKeys(hooksFile.Hooks)
	wantNames := []string{"PreToolUse", "Stop", "UserPromptSubmit"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("hook names = %v, want %v", gotNames, wantNames)
	}
	for _, hookName := range wantNames {
		entries := hooksFile.Hooks[hookName]
		if len(entries) != 1 {
			t.Fatalf("%s entries = %d, want 1", hookName, len(entries))
		}
		if len(entries[0].Hooks) != 1 {
			t.Fatalf("%s hook commands = %d, want 1", hookName, len(entries[0].Hooks))
		}
		command := entries[0].Hooks[0]
		if command.Type != "command" {
			t.Fatalf("%s type = %q, want command", hookName, command.Type)
		}
		wantCommand := "./bin/genplug " + hookName
		if command.Command != wantCommand {
			t.Fatalf("%s command = %q, want %q", hookName, command.Command, wantCommand)
		}
	}

	report := inspectGeneratedProject(t, pluginKitAIBin, plugRoot, "claude")
	target := requireInspectTarget(t, report, "claude")
	mustHaveManagedArtifacts(t, target.ManagedArtifacts, ".claude-plugin/plugin.json", "hooks/hooks.json")
	mustExist(t, filepath.Join(plugRoot, ".claude-plugin", "plugin.json"))
	mustExist(t, filepath.Join(plugRoot, "hooks", "hooks.json"))
}

func TestGeneratedConfigCanaries_CodexNotifyInvocationShape(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := initGeneratedCanaryProject(t, pluginKitAIBin, "codex")

	runPluginKitAICommand(t, pluginKitAIBin, "render", plugRoot, "--check")

	body, err := os.ReadFile(filepath.Join(plugRoot, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	lines := nonCommentLines(string(body))
	if len(lines) != 2 {
		t.Fatalf("config lines = %v, want exactly model + notify", lines)
	}
	if lines[0] != `model = "gpt-5.4-mini"` {
		t.Fatalf("first config line = %q, want gpt-5.4-mini", lines[0])
	}
	if lines[1] != `notify = ["./bin/genplug", "notify"]` {
		t.Fatalf("notify line = %q, want exact argv shape", lines[1])
	}
	packageBody, err := os.ReadFile(filepath.Join(plugRoot, "targets", "codex", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageBody), `model_hint: "gpt-5.4-mini"`) {
		t.Fatalf("targets/codex/package.yaml = %q, want gpt-5.4-mini model_hint", string(packageBody))
	}

	report := inspectGeneratedProject(t, pluginKitAIBin, plugRoot, "codex")
	target := requireInspectTarget(t, report, "codex")
	mustHaveManagedArtifacts(t, target.ManagedArtifacts, ".codex-plugin/plugin.json", ".codex/config.toml")
	mustExist(t, filepath.Join(plugRoot, ".codex-plugin", "plugin.json"))
	mustExist(t, filepath.Join(plugRoot, ".codex", "config.toml"))
}

func TestGeneratedConfigCanaries_RenderCheckDetectsRuntimeArtifactDrift(t *testing.T) {
	cases := []struct {
		platform  string
		driftFile string
		driftBody string
	}{
		{
			platform:  "claude",
			driftFile: filepath.Join("hooks", "hooks.json"),
			driftBody: `{"hooks":{"Stop":[]}}`,
		},
		{
			platform:  "codex",
			driftFile: filepath.Join(".codex", "config.toml"),
			driftBody: "notify = [\"./bin/genplug\"]\n",
		},
	}

	pluginKitAIBin := buildPluginKitAI(t)
	for _, tc := range cases {
		tc := tc
		t.Run(tc.platform, func(t *testing.T) {
			plugRoot := initGeneratedCanaryProject(t, pluginKitAIBin, tc.platform)
			writeRuntimeFile(t, plugRoot, tc.driftFile, tc.driftBody)

			cmd := exec.Command(pluginKitAIBin, "render", plugRoot, "--check")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("render --check unexpectedly succeeded:\n%s", out)
			}
			if !strings.Contains(string(out), filepath.ToSlash(tc.driftFile)) {
				t.Fatalf("render --check output = %q, want drift path %q", string(out), filepath.ToSlash(tc.driftFile))
			}
		})
	}
}

func TestGeneratedConfigCanaries_ClaudeAuthoredHookEntrypointDriftIsCaughtByValidate(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := initGeneratedCanaryProject(t, pluginKitAIBin, "claude")

	writeRuntimeFile(t, plugRoot, filepath.Join("targets", "claude", "hooks", "hooks.json"), `{
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "./bin/old-genplug Stop"}]}],
    "PreToolUse": [{"hooks": [{"type": "command", "command": "./bin/old-genplug PreToolUse"}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "./bin/old-genplug UserPromptSubmit"}]}]
  }
}
`)

	runPluginKitAICommand(t, pluginKitAIBin, "render", plugRoot)
	runPluginKitAICommand(t, pluginKitAIBin, "render", plugRoot, "--check")

	cmd := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "claude", "--strict")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("validate unexpectedly succeeded:\n%s", out)
	}
	if !strings.Contains(string(out), "entrypoint mismatch") {
		t.Fatalf("validate output = %q, want entrypoint mismatch", string(out))
	}
	if !strings.Contains(string(out), "./bin/old-genplug") || !strings.Contains(string(out), "./bin/genplug") {
		t.Fatalf("validate output = %q, want old and expected entrypoint details", string(out))
	}
}

type inspectReport struct {
	Targets []inspectTarget `json:"targets"`
}

type inspectTarget struct {
	Target           string   `json:"target"`
	ManagedArtifacts []string `json:"managed_artifacts"`
}

func initGeneratedCanaryProject(t *testing.T, pluginKitAIBin, platform string) string {
	t.Helper()
	plugRoot := runtimeProjectRoot(t)
	runPluginKitAICommand(t, pluginKitAIBin, "init", "genplug", "--platform", platform, "--runtime", "go", "-o", plugRoot)
	return plugRoot
}

func inspectGeneratedProject(t *testing.T, pluginKitAIBin, root, target string) inspectReport {
	t.Helper()
	cmd := exec.Command(pluginKitAIBin, "inspect", root, "--target", target, "--format", "json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai inspect: %v\n%s", err, out)
	}
	var report inspectReport
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse inspect json: %v\n%s", err, out)
	}
	return report
}

func requireInspectTarget(t *testing.T, report inspectReport, wantTarget string) inspectTarget {
	t.Helper()
	for _, target := range report.Targets {
		if target.Target == wantTarget {
			return target
		}
	}
	t.Fatalf("missing inspect target %q in %+v", wantTarget, report.Targets)
	return inspectTarget{}
}

func mustHaveManagedArtifacts(t *testing.T, got []string, want ...string) {
	t.Helper()
	for _, item := range want {
		if !slices.Contains(got, item) {
			t.Fatalf("managed artifacts = %v, want %q", got, item)
		}
	}
}

func mustExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func runPluginKitAICommand(t *testing.T, pluginKitAIBin string, args ...string) string {
	t.Helper()
	cmd := exec.Command(pluginKitAIBin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v\n%s", cmd.String(), err, out)
	}
	return string(out)
}

func nonCommentLines(body string) []string {
	lines := strings.Split(body, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func sortedKeys[M ~map[string]V, V any](m M) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	slices.Sort(out)
	return out
}
