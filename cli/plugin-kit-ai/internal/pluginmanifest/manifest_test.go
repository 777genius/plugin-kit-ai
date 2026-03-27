package pluginmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/scaffold"
)

func TestRender_RendersVersionIntoEveryNativeManifest(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex", "go", "demo plugin", true)
	manifest.Version = "1.2.3"
	manifest.Targets.Enabled = []string{"claude", "codex", "gemini"}
	if err := Save(root, manifest, false); err != nil {
		t.Fatal(err)
	}
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

func TestImport_LegacyCodexShellProject(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"codex\"\nruntime = \"shell\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/demo\"\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")

	manifest, _, err := Import(root, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Runtime != "shell" {
		t.Fatalf("runtime = %q, want shell", manifest.Runtime)
	}
	if manifest.Entrypoint != "./bin/demo" {
		t.Fatalf("entrypoint = %q", manifest.Entrypoint)
	}
	if manifest.Targets.Codex.Model != "gpt-5.4-mini" {
		t.Fatalf("model = %q", manifest.Targets.Codex.Model)
	}
}

func TestAnalyze_WarnsOnUnknownAndDeprecatedFields(t *testing.T) {
	body := []byte(`
schema_version: 1
name: "demo"
version: "0.1.0"
description: "demo"
runtime: "go"
entrypoint: "./bin/demo"
targets:
  enabled: ["claude"]
  claude: {}
  nonsense: true
components:
  skills: []
  commands: []
  hooks: []
`)
	_, warnings, err := Analyze(body)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) < 3 {
		t.Fatalf("warnings = %+v", warnings)
	}
	var foundDeprecated, foundUnknownTarget, foundDeprecatedHooks bool
	for _, warning := range warnings {
		switch warning.Path {
		case "targets.claude":
			foundDeprecated = warning.Kind == WarningDeprecatedField
		case "targets.nonsense":
			foundUnknownTarget = warning.Kind == WarningUnknownField
		case "components.hooks":
			foundDeprecatedHooks = warning.Kind == WarningDeprecatedField
		}
	}
	if !foundDeprecated || !foundUnknownTarget || !foundDeprecatedHooks {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func TestRenderTemplateArtifact_ReturnsErrorInsteadOfPanicking(t *testing.T) {
	_, err := renderTemplateArtifact("broken.json", "missing-template.tmpl", scaffoldDataForTests())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "render broken.json from missing-template.tmpl") {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalize_RewritesManifestIntoSupportedV1Shape(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, FileName, `schema_version: 1
name: "demo"
version: "0.1.0"
description: "demo"
runtime: "go"
entrypoint: "./bin/demo"
targets:
  enabled:
    - "codex"
  codex:
    model: "gpt-5-codex"
  claude: {}
components:
  skills: []
  commands: []
  hooks: []
extra_field: true
`)
	warnings, err := Normalize(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) < 3 {
		t.Fatalf("warnings = %+v", warnings)
	}
	body, err := os.ReadFile(filepath.Join(root, FileName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, unwanted := range []string{"extra_field", "claude: {}", "hooks:"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("normalized manifest still contains %q:\n%s", unwanted, text)
		}
	}
}

func TestImport_WarnsOnIgnoredAssets(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\nplatform = \"codex\"\nruntime = \"shell\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/demo\"\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")
	mustWritePluginFile(t, root, ".mcp.json", "{}\n")
	mustWritePluginFile(t, root, filepath.Join("agents", "worker.md"), "agent\n")

	_, warnings, err := Import(root, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) < 2 {
		t.Fatalf("warnings = %+v", warnings)
	}
	var foundMCP, foundAgents bool
	for _, warning := range warnings {
		switch warning.Path {
		case ".mcp.json":
			foundMCP = true
		case "agents":
			foundAgents = true
		}
	}
	if !foundMCP || !foundAgents {
		t.Fatalf("warnings = %+v", warnings)
	}
}

func scaffoldDataForTests() scaffold.Data {
	return scaffold.Data{
		ProjectName: "demo",
		ModulePath:  "example.com/demo",
		Description: "demo plugin",
		Version:     "0.1.0",
		Platform:    "codex",
		Runtime:     "go",
		Entrypoint:  "./bin/demo",
		CodexModel:  "gpt-5-codex",
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
