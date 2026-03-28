package pluginmanifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRender_RendersVersionIntoEveryNativeManifest(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex", "go", "demo plugin", true)
	manifest.Version = "1.2.3"
	manifest.Targets = []string{"claude", "codex", "gemini"}
	mustSavePackage(t, root, manifest, "go")
	result, err := Render(root, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 5 {
		t.Fatalf("artifacts = %d, want 5", len(result.Artifacts))
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join("hooks", "hooks.json"),
		filepath.Join(".codex", "config.toml"),
		filepath.Join(".codex-plugin", "plugin.json"),
		"gemini-extension.json",
	} {
		full := filepath.Join(root, rel)
		if _, err := os.Stat(full); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
		body, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if rel == filepath.Join("hooks", "hooks.json") || rel == filepath.Join(".codex", "config.toml") {
			continue
		}
		if !strings.Contains(string(body), `"version": "1.2.3"`) {
			t.Fatalf("%s missing rendered version:\n%s", rel, body)
		}
	}
}

func TestRender_ClaudeDefaultHooksStayStableSubset(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", false)
	mustSavePackage(t, root, manifest, "go")
	result, err := Render(root, "claude")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, want := range []string{`"Stop"`, `"PreToolUse"`, `"UserPromptSubmit"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("default Claude hooks missing %s:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{`"SessionStart"`, `"WorktreeRemove"`} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("default Claude hooks unexpectedly contain %s:\n%s", unwanted, got)
		}
	}
}

func TestImport_CurrentNativeCodexShellProject(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "notify = [\"./bin/demo\", \"notify\"]\nmodel = \"gpt-5.4-mini\"\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)

	_, _, err := Import(root, "codex", false)
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := LoadLauncher(root)
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Runtime != "shell" {
		t.Fatalf("runtime = %q, want shell", launcher.Runtime)
	}
	if launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", launcher.Entrypoint)
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "codex", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "model_hint: gpt-5.4-mini") {
		t.Fatalf("package metadata = %q", string(body))
	}
}

func TestRender_CodexMergesManifestAndConfigExtra(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex", "go", "demo plugin", false)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "package.yaml"), "model_hint: gpt-5.4-mini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "manifest.extra.json"), `{"homepage":"https://example.com/demo","interface":{"defaultPrompt":"Run the demo"},"apps":["./.app.json"]}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "config.extra.toml"), "approval_policy = \"never\"\n[ui]\nverbose = true\n")

	result, err := Render(root, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}

	pluginBody, err := os.ReadFile(filepath.Join(root, ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plugin map[string]any
	if err := json.Unmarshal(pluginBody, &plugin); err != nil {
		t.Fatal(err)
	}
	if plugin["name"] != "demo" || plugin["version"] != "0.1.0" || plugin["description"] != "demo plugin" {
		t.Fatalf("plugin manifest = %+v", plugin)
	}
	if plugin["homepage"] != "https://example.com/demo" {
		t.Fatalf("plugin manifest missing homepage: %+v", plugin)
	}
	if _, ok := plugin["interface"].(map[string]any); !ok {
		t.Fatalf("plugin manifest missing interface object: %+v", plugin)
	}

	configBody, err := os.ReadFile(filepath.Join(root, ".codex", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(configBody)
	for _, want := range []string{
		`model = "gpt-5.4-mini"`,
		`notify = ["./bin/demo", "notify"]`,
		`approval_policy = "never"`,
		`[ui]`,
		`verbose = true`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf(".codex/config.toml missing %q:\n%s", want, got)
		}
	}
}

func TestRender_CodexRejectsManagedOverridesInExtraDocs(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex", "go", "demo plugin", false)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "manifest.extra.json"), `{"name":"override"}`)
	if _, err := Render(root, "codex"); err == nil || !strings.Contains(err.Error(), `codex manifest.extra.json may not override canonical field "name"`) {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "manifest.extra.json"), `{}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "config.extra.toml"), "model = \"gpt-4.1\"\n")
	if _, err := Render(root, "codex"); err == nil || !strings.Contains(err.Error(), `codex config.extra.toml may not override canonical field "model"`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestImport_CurrentNativeCodexPreservesExtraDocs(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\", \"extra\"]\napproval_policy = \"never\"\n[ui]\nverbose = true\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo","homepage":"https://example.com/demo","interface":{"defaultPrompt":"Run the demo"}}`)

	_, warnings, err := Import(root, "codex", false)
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := LoadLauncher(root)
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", launcher.Entrypoint)
	}
	if len(warnings) == 0 {
		t.Fatal("expected fidelity warnings")
	}

	manifestExtra, err := os.ReadFile(filepath.Join(root, "targets", "codex", "manifest.extra.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifestExtra), `"homepage": "https://example.com/demo"`) {
		t.Fatalf("manifest.extra.json = %s", manifestExtra)
	}
	configExtra, err := os.ReadFile(filepath.Join(root, "targets", "codex", "config.extra.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configExtra), `approval_policy =`) || !strings.Contains(string(configExtra), `never`) || !strings.Contains(string(configExtra), `[ui]`) {
		t.Fatalf("config.extra.toml = %s", configExtra)
	}
}

func TestImport_ClaudeHooksJSONParsingHandlesNonFirstCommand(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".claude-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)
	mustWritePluginFile(t, root, filepath.Join("hooks", "hooks.json"), `{
  "hooks": {
    "Stop": [{
      "hooks": [
        {"type": "prompt", "command": "ignored"},
        {"type": "command", "command": "./bin/demo Stop"}
      ]
    }]
  }
}`)

	_, _, err := Import(root, "claude", false)
	if err != nil {
		t.Fatal(err)
	}
	launcher, err := LoadLauncher(root)
	if err != nil {
		t.Fatal(err)
	}
	if launcher.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", launcher.Entrypoint)
	}
}

func TestImport_RefusesOverwriteBeforeWritingImportedLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, FileName, `format: plugin-kit-ai/package
name: "existing"
version: "0.1.0"
description: "existing"
targets: ["codex"]
`)
	mustWritePluginFile(t, root, LauncherFileName, "runtime: go\nentrypoint: ./bin/existing\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)

	_, _, err := Import(root, "codex", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite existing file plugin.yaml") {
		t.Fatalf("error = %v", err)
	}
	for _, rel := range []string{
		filepath.Join("targets", "codex", "package.yaml"),
		filepath.Join("mcp", "servers.json"),
	} {
		if _, statErr := os.Stat(filepath.Join(root, rel)); !os.IsNotExist(statErr) {
			t.Fatalf("expected %s to stay absent, err=%v", rel, statErr)
		}
	}
}

func TestImport_CurrentNativeGeminiLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "gemini-extension.json", `{
	  "name":"demo",
	  "version":"0.2.0",
	  "description":"gemini demo",
	  "contextFileName":"TEAM.md",
	  "excludeTools":["run_shell_command(rm -rf)"],
	  "plan":{"directory":".gemini/plans","retentionDays":7},
	  "settings":[{"name":"api-token","description":"token","envVar":"API_TOKEN","sensitive":true}],
	  "themes":[{"name":"release-dawn","background":"#fff9f2"}],
	  "mcpServers":{"demo":{"command":"demo","args":["serve"]}},
	  "galleryTopic":"gemini-cli-extension"
	}`)
	mustWritePluginFile(t, root, "TEAM.md", "# Team\n")
	mustWritePluginFile(t, root, filepath.Join("commands", "release", "deploy.toml"), "description = \"deploy\"\n")
	mustWritePluginFile(t, root, filepath.Join("policies", "review.toml"), "name = \"review\"\n")
	mustWritePluginFile(t, root, filepath.Join("hooks", "hooks.json"), "{}\n")

	manifest, _, err := Import(root, "gemini", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Targets) != 1 || manifest.Targets[0] != "gemini" {
		t.Fatalf("targets = %+v", manifest.Targets)
	}
	if manifest.Version != "0.2.0" {
		t.Fatalf("version = %q", manifest.Version)
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "gemini", "package.yaml")); err != nil {
		t.Fatalf("stat gemini package metadata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "mcp", "servers.json")); err != nil {
		t.Fatalf("stat mcp servers: %v", err)
	}
	for _, rel := range []string{
		filepath.Join("targets", "gemini", "settings", "api-token.yaml"),
		filepath.Join("targets", "gemini", "themes", "release-dawn.yaml"),
		filepath.Join("targets", "gemini", "manifest.extra.json"),
		filepath.Join("targets", "gemini", "commands", "release", "deploy.toml"),
		filepath.Join("targets", "gemini", "policies", "review.toml"),
		filepath.Join("targets", "gemini", "hooks", "hooks.json"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
	body, err := os.ReadFile(filepath.Join(root, "targets", "gemini", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"context_file_name: TEAM.md", "exclude_tools:", "plan_directory: .gemini/plans"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("package metadata missing %q:\n%s", want, body)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "targets", "gemini", "contexts", "TEAM.md")); err != nil {
		t.Fatalf("stat imported custom primary context: %v", err)
	}
	extra, err := os.ReadFile(filepath.Join(root, "targets", "gemini", "manifest.extra.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(extra), `"galleryTopic": "gemini-cli-extension"`) || !strings.Contains(string(extra), `"retentionDays": 7`) {
		t.Fatalf("manifest extra = %s", extra)
	}
}

func TestRender_GeminiRejectsManifestExtraCanonicalOverride(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "go", "gemini demo", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "manifest.extra.json"), `{"plan":{"directory":".gemini/other"}}`)
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), `gemini manifest.extra.json may not override canonical field "plan.directory"`) {
		t.Fatalf("Render error = %v", err)
	}
}

func TestRender_GeminiManifestParity(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "go", "gemini demo", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginFile(t, root, filepath.Join("mcp", "servers.json"), `{"demo":{"command":"node","args":["${extensionPath}/server.mjs"]}}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "package.yaml"), "context_file_name: GEMINI.md\nexclude_tools:\n  - run_shell_command(rm -rf)\nplan_directory: .gemini/plans\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "api-token.yaml"), "name: api-token\ndescription: token\nenv_var: API_TOKEN\nsensitive: true\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "themes", "release-dawn.yaml"), "name: release-dawn\nbackground: \"#fff9f2\"\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "manifest.extra.json"), `{"galleryTopic":"gemini-cli-extension","plan":{"retentionDays":7}}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "commands", "deploy.toml"), "description = \"deploy\"\n")
	result, err := Render(root, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteArtifacts(root, result.Artifacts); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "gemini-extension.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rendered map[string]any
	if err := json.Unmarshal(body, &rendered); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"mcpServers", "excludeTools", "plan", "settings", "themes", "contextFileName", "galleryTopic"} {
		if _, ok := rendered[key]; !ok {
			t.Fatalf("rendered manifest missing %q: %s", key, body)
		}
	}
	plan := rendered["plan"].(map[string]any)
	if plan["directory"] != ".gemini/plans" || plan["retentionDays"] != float64(7) {
		t.Fatalf("plan = %#v", plan)
	}
	if _, err := os.Stat(filepath.Join(root, "commands", "deploy.toml")); err != nil {
		t.Fatalf("stat generated command: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "GEMINI.md")); err != nil {
		t.Fatalf("stat generated primary context: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "settings", "api-token.yaml")); !os.IsNotExist(err) {
		t.Fatalf("settings should be rendered into manifest, err=%v", err)
	}
}

func TestRender_GeminiRejectsMalformedStructuredSettingsAndThemes(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo-gemini", "gemini", "go", "gemini demo", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "broken.yaml"), "name: broken\n")

	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), "Gemini settings require") {
		t.Fatalf("Render error = %v", err)
	}

	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "settings", "broken.yaml"), "name: fixed\ndescription: desc\nenv_var: FIXED\nsensitive: false\n")
	mustWritePluginFile(t, root, filepath.Join("targets", "gemini", "themes", "broken.yaml"), "background: \"#fff\"\n")
	if _, err := Render(root, "gemini"); err == nil || !strings.Contains(err.Error(), "Gemini themes require name") {
		t.Fatalf("Render error = %v", err)
	}
}

func TestImport_RejectsLegacyInternalProjectManifest(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\n")
	_, _, err := Import(root, "", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), ".plugin-kit-ai/project.toml is not supported") {
		t.Fatalf("error = %q", err)
	}
}

func TestAnalyze_RejectsLegacySchemaVersion(t *testing.T) {
	body := []byte(`
schema_version: 1
name: "demo"
version: "0.1.0"
description: "demo"
runtime: "go"
entrypoint: "./bin/demo"
targets:
  enabled: ["codex"]
`)
	_, _, err := Analyze(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported plugin.yaml format: schema_version-based manifests are not supported") {
		t.Fatalf("error = %q", err)
	}
}

func TestAnalyze_RejectsInvalidGeminiExtensionName(t *testing.T) {
	body := []byte(`
format: plugin-kit-ai/package
name: "Demo_Extension"
version: "0.1.0"
description: "demo"
targets: ["gemini"]
`)
	_, _, err := Analyze(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid Gemini extension name") {
		t.Fatalf("error = %q", err)
	}
}

func TestAnalyze_WarnsOnUnknownFields(t *testing.T) {
	body := []byte(`
format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["claude"]
nonsense: true
`)
	_, warnings, err := Analyze(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v", warnings)
	}
	var foundUnknown bool
	for _, warning := range warnings {
		if warning.Path == "nonsense" {
			foundUnknown = warning.Kind == WarningUnknownField
		}
	}
	if !foundUnknown {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestAnalyze_RejectsLegacyComponentsInventory(t *testing.T) {
	body := []byte(`
format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["claude"]
components:
  hooks: []
`)
	_, _, err := Analyze(body)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unsupported plugin.yaml format: flat components inventory is not supported") {
		t.Fatalf("error = %q", err)
	}
}

func TestInspect_ReturnsTargetCoverage(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "claude", "go", "demo plugin", true)
	manifest.Targets = []string{"claude", "gemini"}
	mustSavePackage(t, root, manifest, "go")
	if err := os.MkdirAll(filepath.Join(root, "targets", "claude", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWritePluginFile(t, root, filepath.Join("targets", "claude", "hooks", "hooks.json"), "{}\n")
	inspection, _, err := Inspect(root, "all")
	if err == nil {
		if len(inspection.Targets) != 2 {
			t.Fatalf("targets = %+v", inspection.Targets)
		}
		return
	}
	t.Fatal(err)
}

func TestInspect_IncludesTargetLifecycleMetadata(t *testing.T) {
	root := t.TempDir()
	manifest := Default("gemini-inspect", "gemini", "go", "gemini inspect", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("contexts", "GEMINI.md"), "# Gemini\n")

	inspection, _, err := Inspect(root, "gemini")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Targets) != 1 {
		t.Fatalf("targets = %+v", inspection.Targets)
	}
	target := inspection.Targets[0]
	if target.TargetNoun != "extension" || target.InstallModel != "copy install" || target.DevModel != "link" || target.ActivationModel != "restart required" {
		t.Fatalf("target lifecycle metadata = %+v", target)
	}
	if target.NativeRoot != "~/.gemini/extensions/<name>" {
		t.Fatalf("native root = %q", target.NativeRoot)
	}
}

func TestInspect_CodexIncludesExtraDocKinds(t *testing.T) {
	root := t.TempDir()
	manifest := Default("codex-inspect", "codex", "go", "codex inspect", true)
	mustSavePackage(t, root, manifest, "go")
	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "manifest.extra.json"), `{"homepage":"https://example.com"}`)
	mustWritePluginFile(t, root, filepath.Join("targets", "codex", "config.extra.toml"), "approval_policy = \"never\"\n")

	inspection, _, err := Inspect(root, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.Targets) != 1 {
		t.Fatalf("targets = %+v", inspection.Targets)
	}
	kinds := inspection.Targets[0].TargetNativeKinds
	for _, want := range []string{"manifest_extra", "config_extra"} {
		if !slices.Contains(kinds, want) {
			t.Fatalf("target_native_kinds = %v, want %q", kinds, want)
		}
	}
}

func TestNormalize_RewritesManifestIntoPackageStandardShape(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, FileName, `format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
targets: ["codex"]
extra_field: true
`)
	mustWritePluginFile(t, root, LauncherFileName, "runtime: go\nentrypoint: ./bin/demo\n")
	warnings, err := Normalize(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %+v", warnings)
	}
	body, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, unwanted := range []string{"extra_field"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("normalized manifest still contains %q:\n%s", unwanted, text)
		}
	}
}

func TestImport_WarnsOnIgnoredAssets(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")
	mustWritePluginFile(t, root, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\n")
	mustWritePluginFile(t, root, ".mcp.json", "{}\n")
	mustWritePluginFile(t, root, filepath.Join("agents", "reviewer.md"), "reviewer\n")

	_, warnings, err := Import(root, "codex", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) < 2 {
		t.Fatalf("warnings = %+v", warnings)
	}
	var foundMCP, foundAgents bool
	for _, warning := range warnings {
		if warning.Path == ".mcp.json" {
			foundMCP = true
		}
		if warning.Path == "agents" {
			foundAgents = true
		}
	}
	if !foundMCP || !foundAgents {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func mustWritePluginFile(t *testing.T, root, rel, body string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustSavePackage(t *testing.T, root string, manifest Manifest, runtime string) {
	t.Helper()
	if err := Save(root, manifest, false); err != nil {
		t.Fatal(err)
	}
	if err := SaveLauncher(root, DefaultLauncher(manifest.Name, runtime), false); err != nil {
		t.Fatal(err)
	}
}
