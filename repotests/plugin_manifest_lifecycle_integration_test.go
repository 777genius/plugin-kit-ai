package pluginkitairepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginKitAIValidateStrictFailsOnWarningsThenNormalizeFixesThem(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	initCmd := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", "codex-runtime", "-o", plugRoot)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
	}
	bootstrapGeneratedGoPlugin(t, plugRoot)

	manifestPath := filepath.Join(plugRoot, "plugin.yaml")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("extra_field: true\n")...)
	if err := os.WriteFile(manifestPath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	validateStrict := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "codex-runtime", "--strict")
	validateStrict.Env = append(os.Environ(), "GOWORK=off")
	out, err := validateStrict.CombinedOutput()
	if err == nil {
		t.Fatalf("validate --strict should fail on warnings:\n%s", out)
	}
	if !strings.Contains(string(out), "validation warnings treated as errors") {
		t.Fatalf("unexpected strict output:\n%s", out)
	}

	normalizeCmd := exec.Command(pluginKitAIBin, "normalize", plugRoot)
	if out, err := normalizeCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai normalize: %v\n%s", err, out)
	}

	validateStrict = exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "codex-runtime", "--strict")
	validateStrict.Env = append(os.Environ(), "GOWORK=off")
	out, err = validateStrict.CombinedOutput()
	if err != nil {
		t.Fatalf("validate --strict after normalize: %v\n%s", err, out)
	}
}

func TestPluginKitAIImportPrintsWarningsForIgnoredAssets(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(plugRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, ".codex-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".codex", "config.toml"), []byte("notify = [\"./bin/demo\", \"notify\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".codex-plugin", "plugin.json"), []byte(`{"name":"demo","version":"0.1.0","description":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "scripts", "main.sh"), []byte("#!/usr/bin/env bash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".mcp.json"), []byte("{\"demo\":{\"command\":\"node\",\"args\":[\"server.mjs\"]}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	importCmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", "codex-package")
	out, err := importCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai import: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "Warning: portable MCP will be preserved under mcp/servers.yaml") {
		t.Fatalf("missing .mcp.json warning:\n%s", text)
	}
	if !strings.Contains(text, "Warning: ignored unsupported import asset: agents") {
		t.Fatalf("missing agents warning:\n%s", text)
	}
}

func TestPluginKitAIImportClaudeNativeLayoutRoundTrip(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(plugRoot, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, "cmd", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".claude-plugin", "plugin.json"), []byte(`{"name":"demo","version":"0.1.0","description":"demo","userConfig":{"api_token":{"description":"token","secret":true}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "hooks", "hooks.json"), []byte(`{
  "hooks": {
    "Stop": [{"hooks": [{"type": "command", "command": "./bin/demo Stop"}]}],
    "PreToolUse": [{"hooks": [{"type": "command", "command": "./bin/demo PreToolUse"}]}],
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "./bin/demo UserPromptSubmit"}]}]
  }
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "settings.json"), []byte(`{"agent":"reviewer"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".lsp.json"), []byte(`{"servers":{"demo":{"command":["demo-lsp"]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "agents", "reviewer.md"), []byte("---\nname: reviewer\ndescription: review\n---\nReview.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "go.mod"), []byte("module example.com/demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "cmd", "demo", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	importCmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", "claude")
	out, err := importCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai import claude: %v\n%s", err, out)
	}

	authoredHooks, err := os.ReadFile(filepath.Join(plugRoot, "targets", "claude", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(authoredHooks), "./bin/demo PreToolUse") {
		t.Fatalf("imported Claude hooks missing expected entrypoint:\n%s", authoredHooks)
	}
	for _, rel := range []string{
		filepath.Join("targets", "claude", "settings.json"),
		filepath.Join("targets", "claude", "lsp.json"),
		filepath.Join("targets", "claude", "user-config.json"),
		filepath.Join("targets", "claude", "agents", "reviewer.md"),
	} {
		if _, err := os.Stat(filepath.Join(plugRoot, rel)); err != nil {
			t.Fatalf("missing imported Claude artifact %s: %v", rel, err)
		}
	}

	renderCmd := exec.Command(pluginKitAIBin, "render", plugRoot)
	renderCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai render after Claude import: %v\n%s", err, out)
	}

	renderCmd = exec.Command(pluginKitAIBin, "render", plugRoot, "--check")
	renderCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai render --check after Claude import: %v\n%s", err, out)
	}

	validateCmd := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "claude", "--strict")
	validateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate after Claude import: %v\n%s", err, out)
	}

	renderedPlugin, err := os.ReadFile(filepath.Join(plugRoot, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(renderedPlugin), `"userConfig"`) {
		t.Fatalf("rendered Claude plugin missing userConfig:\n%s", renderedPlugin)
	}
}

func TestPluginKitAIImportCodexNativeLayoutRoundTripPreservesCheapModelHint(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(plugRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, ".codex-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, "cmd", "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".codex", "config.toml"), []byte("model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\", \"extra\"]\napproval_policy = \"never\"\n[ui]\nverbose = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".codex-plugin", "plugin.json"), []byte(`{"name":"demo","version":"0.1.0","description":"demo","homepage":"https://example.com/demo","interface":{"defaultPrompt":"Run the demo"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "go.mod"), []byte("module example.com/demo\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, "cmd", "demo", "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	importCmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", "codex-runtime")
	out, err := importCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai import codex: %v\n%s", err, out)
	}

	packageBody, err := os.ReadFile(filepath.Join(plugRoot, "targets", "codex-runtime", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageBody), "model_hint: gpt-5.4-mini") {
		t.Fatalf("imported codex package metadata = %q, want gpt-5.4-mini model_hint", string(packageBody))
	}
	configExtraBody, err := os.ReadFile(filepath.Join(plugRoot, "targets", "codex-runtime", "config.extra.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configExtraBody), `approval_policy =`) || !strings.Contains(string(configExtraBody), `never`) || !strings.Contains(string(configExtraBody), `[ui]`) {
		t.Fatalf("config extra = %q", string(configExtraBody))
	}

	renderCmd := exec.Command(pluginKitAIBin, "render", plugRoot)
	renderCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai render after Codex import: %v\n%s", err, out)
	}

	assertCodexConfig(t, plugRoot, "gpt-5.4-mini", "./bin/demo")
	renderedConfigBody, err := os.ReadFile(filepath.Join(plugRoot, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(renderedConfigBody), `approval_policy =`) || !strings.Contains(string(renderedConfigBody), `never`) || !strings.Contains(string(renderedConfigBody), `[ui]`) {
		t.Fatalf("rendered codex config = %q", string(renderedConfigBody))
	}

	renderCheckCmd := exec.Command(pluginKitAIBin, "render", plugRoot, "--check")
	renderCheckCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCheckCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai render --check after Codex import: %v\n%s", err, out)
	}

	validateCmd := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "codex-runtime", "--strict")
	validateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate codex-runtime after Codex import: %v\n%s", err, out)
	}
}

func TestPluginKitAIHelpDoesNotExposeMigrateCommand(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	cmd := exec.Command(pluginKitAIBin, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai --help: %v\n%s", err, out)
	}
	text := string(out)
	if strings.Contains(text, " migrate ") || strings.Contains(text, "\nmigrate") {
		t.Fatalf("unexpected migrate command in help:\n%s", text)
	}
}

func TestPluginKitAIImportRejectsLegacyCodexAlias(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	cmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", "codex")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected legacy codex alias to fail:\n%s", out)
	}
	if !strings.Contains(string(out), `unsupported import source "codex"`) {
		t.Fatalf("unexpected import failure:\n%s", out)
	}
}

func TestPluginKitAIImportRejectsLegacyCodexNativeAlias(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	cmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", "codex-native")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected legacy codex-native alias to fail:\n%s", out)
	}
	if !strings.Contains(string(out), `unsupported import source "codex-native"`) {
		t.Fatalf("unexpected import failure:\n%s", out)
	}
}
