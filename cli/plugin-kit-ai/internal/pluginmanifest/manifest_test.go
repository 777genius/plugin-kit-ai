package pluginmanifest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRender_RendersVersionIntoEveryNativeManifest(t *testing.T) {
	root := t.TempDir()
	manifest := Default("demo", "codex", "go", "demo plugin", true)
	manifest.Version = "1.2.3"
	manifest.Targets = []string{"claude", "codex", "gemini"}
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

func TestImport_CurrentNativeCodexShellProject(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join("scripts", "main.sh"), "#!/usr/bin/env bash\n")
	mustWritePluginFile(t, root, filepath.Join(".codex", "config.toml"), "model = \"gpt-5.4-mini\"\nnotify = [\"./bin/demo\", \"notify\"]\n")
	mustWritePluginFile(t, root, filepath.Join(".codex-plugin", "plugin.json"), `{"name":"demo","version":"0.1.0","description":"demo"}`)

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
	body, err := os.ReadFile(filepath.Join(root, "targets", "codex", "package.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "model_hint: gpt-5.4-mini") {
		t.Fatalf("package metadata = %q", string(body))
	}
}

func TestImport_CurrentNativeGeminiLayout(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, "gemini-extension.json", `{"name":"demo","version":"0.2.0","description":"gemini demo","contextFileName":"GEMINI.md","mcpServers":{"demo":{"command":"demo"}}}`)
	mustWritePluginFile(t, root, "GEMINI.md", "# Gemini\n")

	manifest, _, err := Import(root, "gemini")
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
}

func TestImport_RejectsLegacyInternalProjectManifest(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, filepath.Join(".plugin-kit-ai", "project.toml"), "schema_version = 1\n")
	_, _, err := Import(root, "")
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

func TestAnalyze_WarnsOnUnknownFields(t *testing.T) {
	body := []byte(`
format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
runtime: "go"
entrypoint: "./bin/demo"
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
runtime: "go"
entrypoint: "./bin/demo"
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
	if err := Save(root, manifest, false); err != nil {
		t.Fatal(err)
	}
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

func TestNormalize_RewritesManifestIntoPackageStandardShape(t *testing.T) {
	root := t.TempDir()
	mustWritePluginFile(t, root, FileName, `format: plugin-kit-ai/package
name: "demo"
version: "0.1.0"
description: "demo"
runtime: "go"
entrypoint: "./bin/demo"
targets: ["codex"]
extra_field: true
`)
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

	_, warnings, err := Import(root, "codex")
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
