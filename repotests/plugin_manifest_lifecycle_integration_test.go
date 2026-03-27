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

	initCmd := exec.Command(pluginKitAIBin, "init", "genplug", "--platform", "codex", "-o", plugRoot)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("plugin-kit-ai init: %v\n%s", err, out)
	}

	manifestPath := filepath.Join(plugRoot, "plugin.yaml")
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, []byte("extra_field: true\n")...)
	if err := os.WriteFile(manifestPath, body, 0o644); err != nil {
		t.Fatal(err)
	}

	validateStrict := exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "codex", "--strict")
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

	validateStrict = exec.Command(pluginKitAIBin, "validate", plugRoot, "--platform", "codex", "--strict")
	validateStrict.Env = append(os.Environ(), "GOWORK=off")
	out, err = validateStrict.CombinedOutput()
	if err != nil {
		t.Fatalf("validate --strict after normalize: %v\n%s", err, out)
	}
}

func TestPluginKitAIImportPrintsWarningsForIgnoredAssets(t *testing.T) {
	pluginKitAIBin := buildPluginKitAI(t)
	plugRoot := t.TempDir()

	if err := os.MkdirAll(filepath.Join(plugRoot, ".plugin-kit-ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, ".codex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plugRoot, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".plugin-kit-ai", "project.toml"), []byte("schema_version = 1\nplatform = \"codex\"\nruntime = \"shell\"\nexecution_mode = \"launcher\"\nentrypoint = \"./bin/demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".codex", "config.toml"), []byte("notify = [\"./bin/demo\", \"notify\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plugRoot, ".mcp.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	importCmd := exec.Command(pluginKitAIBin, "import", plugRoot, "--from", "codex")
	out, err := importCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plugin-kit-ai import: %v\n%s", err, out)
	}
	text := string(out)
	if !strings.Contains(text, "Warning: ignored unsupported import asset: .mcp.json") {
		t.Fatalf("missing .mcp.json warning:\n%s", text)
	}
	if !strings.Contains(text, "Warning: ignored unsupported import asset: agents") {
		t.Fatalf("missing agents warning:\n%s", text)
	}
}
