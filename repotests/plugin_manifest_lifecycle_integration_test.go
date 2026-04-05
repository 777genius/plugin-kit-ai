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

	manifestPath := filepath.Join(plugRoot, "src", "plugin.yaml")
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
	if err := os.WriteFile(filepath.Join(plugRoot, ".codex-plugin", "plugin.json"), []byte(`{"name":"demo","version":"0.1.0","description":"demo","mcpServers":"./.mcp.json"}`), 0o644); err != nil {
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
	if !strings.Contains(text, "Warning: portable MCP will be preserved under src/mcp/servers.yaml") {
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

	authoredHooks, err := os.ReadFile(filepath.Join(plugRoot, "src", "targets", "claude", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(authoredHooks), "./bin/demo PreToolUse") {
		t.Fatalf("imported Claude hooks missing expected entrypoint:\n%s", authoredHooks)
	}
	for _, rel := range []string{
		filepath.Join("src", "targets", "claude", "settings.json"),
		filepath.Join("src", "targets", "claude", "lsp.json"),
		filepath.Join("src", "targets", "claude", "user-config.json"),
		filepath.Join("src", "targets", "claude", "agents", "reviewer.md"),
	} {
		if _, err := os.Stat(filepath.Join(plugRoot, rel)); err != nil {
			t.Fatalf("missing imported Claude artifact %s: %v", rel, err)
		}
	}

	generateCmd := exec.Command(pluginKitAIBin, "generate", plugRoot)
	generateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := generateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate after Claude import: %v\n%s", err, out)
	}

	generateCmd = exec.Command(pluginKitAIBin, "generate", plugRoot, "--check")
	generateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := generateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate --check after Claude import: %v\n%s", err, out)
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
		t.Fatalf("generated Claude plugin missing userConfig:\n%s", renderedPlugin)
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
	if err := os.WriteFile(filepath.Join(plugRoot, ".codex-plugin", "plugin.json"), []byte(`{"name":"demo","version":"0.1.0","description":"demo","author":{"name":"Example Maintainer"},"homepage":"https://example.com/demo","repository":"https://github.com/example/demo","license":"MIT","keywords":["codex","demo"],"interface":{"defaultPrompt":["Run the demo"]},"apps":"./.app.json","x-extra":{"enabled":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".app.json"), []byte(`{"name":"demo-app"}`), 0o644); err != nil {
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

	packageBody, err := os.ReadFile(filepath.Join(plugRoot, "src", "targets", "codex-runtime", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(packageBody), "model_hint: gpt-5.4-mini") {
		t.Fatalf("imported codex package metadata = %q, want gpt-5.4-mini model_hint", string(packageBody))
	}
	configExtraBody, err := os.ReadFile(filepath.Join(plugRoot, "src", "targets", "codex-runtime", "config.extra.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configExtraBody), `approval_policy =`) || !strings.Contains(string(configExtraBody), `never`) || !strings.Contains(string(configExtraBody), `[ui]`) {
		t.Fatalf("config extra = %q", string(configExtraBody))
	}

	generateCmd := exec.Command(pluginKitAIBin, "generate", plugRoot)
	generateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := generateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate after Codex import: %v\n%s", err, out)
	}

	assertCodexConfig(t, plugRoot, "gpt-5.4-mini", "./bin/demo")
	renderedConfigBody, err := os.ReadFile(filepath.Join(plugRoot, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(renderedConfigBody), `approval_policy =`) || !strings.Contains(string(renderedConfigBody), `never`) || !strings.Contains(string(renderedConfigBody), `[ui]`) {
		t.Fatalf("generated codex config = %q", string(renderedConfigBody))
	}

	renderCheckCmd := exec.Command(pluginKitAIBin, "generate", plugRoot, "--check")
	renderCheckCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCheckCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate --check after Codex import: %v\n%s", err, out)
	}

	validateCmd := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "codex-runtime", "--strict")
	validateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate codex-runtime after Codex import: %v\n%s", err, out)
	}
}

func TestPluginKitAICodexPackageLifecycleRoundTripCoversFullSurface(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	authoredRoot := t.TempDir()

	initCmd := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", "codex-package", "--extras", "-o", authoredRoot)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai init codex-package: %v\n%s", err, out)
	}

	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "codex-package", "package.yaml"), `author:
  name: Example Maintainer
  email: maintainer@example.com
homepage: https://example.com/genplug
repository: https://github.com/example/genplug
license: MIT
keywords:
  - codex
  - package
  - example
`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "codex-package", "interface.json"), `{"displayName":"Genplug","defaultPrompt":["Help with Genplug.","Prefer package lane guidance."]}`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "codex-package", "app.json"), `{"name":"genplug-app","entry":"web/index.html"}`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "mcp", "servers.yaml"), `api_version: v1

servers:
  docs:
    type: remote
    remote:
      protocol: streamable_http
      url: "https://example.com/mcp"
    targets:
      - "codex-package"
`)

	generateCmd := exec.Command(pluginKitAIBin, "generate", authoredRoot)
	generateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := generateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate codex-package lifecycle: %v\n%s", err, out)
	}

	renderCheckCmd := exec.Command(pluginKitAIBin, "generate", authoredRoot, "--check")
	renderCheckCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCheckCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate --check codex-package lifecycle: %v\n%s", err, out)
	}

	validateCmd := exec.Command(pluginKitAIBin, "validate", authoredRoot, "--platform", "codex-package", "--strict")
	validateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate codex-package lifecycle: %v\n%s", err, out)
	}

	assertCodexPackageManifest(t, authoredRoot, "genplug")
	manifestBody, err := os.ReadFile(filepath.Join(authoredRoot, ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestBody), `"mcpServers": "./.mcp.json"`) {
		t.Fatalf("generated codex package manifest missing shared MCP ref:\n%s", manifestBody)
	}
	if _, err := os.Stat(filepath.Join(authoredRoot, ".app.json")); err != nil {
		t.Fatalf("stat .app.json: %v", err)
	}
	if _, err := os.Stat(filepath.Join(authoredRoot, ".mcp.json")); err != nil {
		t.Fatalf("stat .mcp.json: %v", err)
	}

	importRoot := t.TempDir()
	copyTree(t, filepath.Join(authoredRoot, ".codex-plugin"), filepath.Join(importRoot, ".codex-plugin"))
	copyTree(t, filepath.Join(authoredRoot, "src", "skills"), filepath.Join(importRoot, "skills"))
	mustCopyPluginLifecycleFile(t, filepath.Join(authoredRoot, ".app.json"), filepath.Join(importRoot, ".app.json"))
	mustCopyPluginLifecycleFile(t, filepath.Join(authoredRoot, ".mcp.json"), filepath.Join(importRoot, ".mcp.json"))

	importCmd := exec.Command(pluginKitAIBin, "import", importRoot, "--from", "codex-package")
	if out, err := importCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai import codex-package lifecycle: %v\n%s", err, out)
	}

	for _, rel := range []string{
		filepath.Join("src", "targets", "codex-package", "package.yaml"),
		filepath.Join("src", "targets", "codex-package", "interface.json"),
		filepath.Join("src", "targets", "codex-package", "app.json"),
		filepath.Join("src", "mcp", "servers.yaml"),
		filepath.Join("src", "skills", "genplug", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(importRoot, rel)); err != nil {
			t.Fatalf("missing imported codex-package artifact %s: %v", rel, err)
		}
	}

	importedInterfaceBody, err := os.ReadFile(filepath.Join(importRoot, "src", "targets", "codex-package", "interface.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(importedInterfaceBody), `"Prefer package lane guidance."`) {
		t.Fatalf("imported codex-package interface = %q", string(importedInterfaceBody))
	}

	generateCmd = exec.Command(pluginKitAIBin, "generate", importRoot)
	generateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := generateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate imported codex-package lifecycle: %v\n%s", err, out)
	}

	renderCheckCmd = exec.Command(pluginKitAIBin, "generate", importRoot, "--check")
	renderCheckCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCheckCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate --check imported codex-package lifecycle: %v\n%s", err, out)
	}

	validateCmd = exec.Command(pluginKitAIBin, "validate", importRoot, "--platform", "codex-package", "--strict")
	validateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate imported codex-package lifecycle: %v\n%s", err, out)
	}
}

func TestPluginKitAIGeminiLifecycleRoundTripCoversFullSurface(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	authoredRoot := filepath.Join(t.TempDir(), "genplug")

	initCmd := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", "gemini", "--runtime", "go", "--extras", "-o", authoredRoot)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai init gemini: %v\n%s", err, out)
	}

	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "package.yaml"), `context_file_name: GEMINI.md
exclude_tools:
  - run_shell_command(rm -rf)
migrated_to: https://github.com/example/genplug-gemini-v2
plan_directory: .gemini/plans
`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "settings", "release-profile.yaml"), `name: release-profile
description: Release profile
env_var: RELEASE_PROFILE
sensitive: false
`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "themes", "release-dawn.yaml"), `name: release-dawn
background:
  primary: "#fff9f2"
text:
  primary: "#2e1f14"
`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "hooks", "hooks.json"), `{
  "hooks": {
    "SessionStart": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiSessionStart"}]}],
    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiSessionEnd"}]}],
    "BeforeModel": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiBeforeModel"}]}],
    "AfterModel": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiAfterModel"}]}],
    "BeforeToolSelection": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiBeforeToolSelection"}]}],
    "BeforeAgent": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiBeforeAgent"}]}],
    "AfterAgent": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiAfterAgent"}]}],
    "BeforeTool": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiBeforeTool"}]}],
    "AfterTool": [{"matcher":"*","hooks":[{"type":"command","command":"${extensionPath}${/}bin${/}genplug GeminiAfterTool"}]}]
  }
}`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "contexts", "RELEASE.md"), "# Release\n")
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "policies", "release-review.toml"), "review = \"required\"\n")
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "commands", "deploy.toml"), "description = \"Deploy release\"\nprompt = \"Ship the release\"\n")
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "targets", "gemini", "manifest.extra.json"), `{"x_galleryTopic":"gemini-cli-extension","plan":{"retentionDays":7}}`)
	mustWritePluginLifecycleFile(t, authoredRoot, filepath.Join("src", "mcp", "servers.yaml"), `api_version: v1

servers:
  docs:
    type: remote
    remote:
      protocol: streamable_http
      url: "https://example.com/mcp"
    targets:
      - "gemini"
`)

	generateCmd := exec.Command(pluginKitAIBin, "generate", authoredRoot)
	generateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := generateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate gemini lifecycle: %v\n%s", err, out)
	}

	renderCheckCmd := exec.Command(pluginKitAIBin, "generate", authoredRoot, "--check")
	renderCheckCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCheckCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate --check gemini lifecycle: %v\n%s", err, out)
	}

	validateCmd := exec.Command(pluginKitAIBin, "validate", authoredRoot, "--platform", "gemini", "--strict")
	validateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate gemini lifecycle: %v\n%s", err, out)
	}

	manifestBody, err := os.ReadFile(filepath.Join(authoredRoot, "gemini-extension.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"contextFileName": "GEMINI.md"`,
		`"migratedTo": "https://github.com/example/genplug-gemini-v2"`,
		`"x_galleryTopic": "gemini-cli-extension"`,
		`"retentionDays": 7`,
		`"release-profile"`,
		`"release-dawn"`,
		`"mcpServers"`,
	} {
		if !strings.Contains(string(manifestBody), want) {
			t.Fatalf("generated gemini manifest missing %q:\n%s", want, manifestBody)
		}
	}
	if _, err := os.Stat(filepath.Join(authoredRoot, "hooks", "hooks.json")); err != nil {
		t.Fatalf("stat generated gemini hooks: %v", err)
	}
	if _, err := os.Stat(filepath.Join(authoredRoot, "GEMINI.md")); err != nil {
		t.Fatalf("stat generated gemini primary context: %v", err)
	}
	if _, err := os.Stat(filepath.Join(authoredRoot, "contexts", "RELEASE.md")); err != nil {
		t.Fatalf("stat generated gemini extra context: %v", err)
	}

	importRoot := filepath.Join(t.TempDir(), "genplug")
	mustCopyPluginLifecycleFile(t, filepath.Join(authoredRoot, "gemini-extension.json"), filepath.Join(importRoot, "gemini-extension.json"))
	copyTree(t, filepath.Join(authoredRoot, "hooks"), filepath.Join(importRoot, "hooks"))
	copyTree(t, filepath.Join(authoredRoot, "commands"), filepath.Join(importRoot, "commands"))
	copyTree(t, filepath.Join(authoredRoot, "policies"), filepath.Join(importRoot, "policies"))
	copyTree(t, filepath.Join(authoredRoot, "contexts"), filepath.Join(importRoot, "contexts"))
	copyTree(t, filepath.Join(authoredRoot, "cmd"), filepath.Join(importRoot, "cmd"))
	mustCopyPluginLifecycleFile(t, filepath.Join(authoredRoot, "GEMINI.md"), filepath.Join(importRoot, "GEMINI.md"))
	mustCopyPluginLifecycleFile(t, filepath.Join(authoredRoot, "go.mod"), filepath.Join(importRoot, "go.mod"))
	if _, err := os.Stat(filepath.Join(authoredRoot, "go.sum")); err == nil {
		mustCopyPluginLifecycleFile(t, filepath.Join(authoredRoot, "go.sum"), filepath.Join(importRoot, "go.sum"))
	}

	importCmd := exec.Command(pluginKitAIBin, "import", importRoot, "--from", "gemini")
	if out, err := importCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai import gemini lifecycle: %v\n%s", err, out)
	}

	for _, rel := range []string{
		filepath.Join("src", "targets", "gemini", "package.yaml"),
		filepath.Join("src", "targets", "gemini", "settings", "release-profile.yaml"),
		filepath.Join("src", "targets", "gemini", "themes", "release-dawn.yaml"),
		filepath.Join("src", "targets", "gemini", "hooks", "hooks.json"),
		filepath.Join("src", "targets", "gemini", "contexts", "GEMINI.md"),
		filepath.Join("src", "targets", "gemini", "contexts", "RELEASE.md"),
		filepath.Join("src", "targets", "gemini", "commands", "deploy.toml"),
		filepath.Join("src", "targets", "gemini", "policies", "release-review.toml"),
		filepath.Join("src", "targets", "gemini", "manifest.extra.json"),
		filepath.Join("src", "mcp", "servers.yaml"),
		filepath.Join("src", "launcher.yaml"),
	} {
		if _, err := os.Stat(filepath.Join(importRoot, rel)); err != nil {
			t.Fatalf("missing imported gemini artifact %s: %v", rel, err)
		}
	}

	importedPackageBody, err := os.ReadFile(filepath.Join(importRoot, "src", "targets", "gemini", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"context_file_name: GEMINI.md",
		"migrated_to: https://github.com/example/genplug-gemini-v2",
		"plan_directory: .gemini/plans",
		"- run_shell_command(rm -rf)",
	} {
		if !strings.Contains(string(importedPackageBody), want) {
			t.Fatalf("imported gemini package metadata missing %q:\n%s", want, importedPackageBody)
		}
	}
	importedExtraBody, err := os.ReadFile(filepath.Join(importRoot, "src", "targets", "gemini", "manifest.extra.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(importedExtraBody), `"x_galleryTopic"`) || !strings.Contains(string(importedExtraBody), `"retentionDays": 7`) {
		t.Fatalf("imported gemini manifest extra = %q", string(importedExtraBody))
	}

	generateCmd = exec.Command(pluginKitAIBin, "generate", importRoot)
	generateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := generateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate imported gemini lifecycle: %v\n%s", err, out)
	}

	renderCheckCmd = exec.Command(pluginKitAIBin, "generate", importRoot, "--check")
	renderCheckCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := renderCheckCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai generate --check imported gemini lifecycle: %v\n%s", err, out)
	}

	validateCmd = exec.Command(pluginKitAIBin, "validate", importRoot, "--platform", "gemini", "--strict")
	validateCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai validate imported gemini lifecycle: %v\n%s", err, out)
	}
}

func mustWritePluginLifecycleFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustCopyPluginLifecycleFile(t *testing.T, src, dst string) {
	t.Helper()
	body, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPluginKitAIHelpDoesNotExposeRemovedCommand(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	cmd := exec.Command(pluginKitAIBin, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai --help: %v\n%s", err, out)
	}
	text := string(out)
	if strings.Contains(text, " migrate ") || strings.Contains(text, "\nmigrate") {
		t.Fatalf("unexpected removed command in help:\n%s", text)
	}
}

func TestPluginKitAIImportRejectsRemovedCodexAlias(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	cmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", "codex")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected removed codex alias to fail:\n%s", out)
	}
	if !strings.Contains(string(out), `unsupported import source "codex"`) {
		t.Fatalf("unexpected import failure:\n%s", out)
	}
}

func TestPluginKitAIImportRejectsRemovedCodexNativeAlias(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()
	removedAlias := "codex" + "-native"

	cmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", removedAlias)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected removed Codex native alias to fail:\n%s", out)
	}
	if !strings.Contains(string(out), `unsupported import source "`+removedAlias+`"`) {
		t.Fatalf("unexpected import failure:\n%s", out)
	}
}
