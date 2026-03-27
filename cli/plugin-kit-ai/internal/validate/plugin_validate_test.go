package validate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmanifest"
)

func TestValidate_PluginProject_CodexGo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.22\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join("cmd", "x", "main.go"), "package main\nfunc main() {}\n")
	if err := pluginmanifest.Save(dir, pluginmanifest.Default("x", "codex", "go", "plugin", false), false); err != nil {
		t.Fatal(err)
	}
	rendered, err := pluginmanifest.Render(dir, "all")
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginmanifest.WriteArtifacts(dir, rendered.Artifacts); err != nil {
		t.Fatal(err)
	}

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_PluginProjectDetectsDrift(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.22\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join("cmd", "x", "main.go"), "package main\nfunc main() {}\n")
	if err := pluginmanifest.Save(dir, pluginmanifest.Default("x", "codex", "go", "plugin", false), false); err != nil {
		t.Fatal(err)
	}
	rendered, err := pluginmanifest.Render(dir, "all")
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginmanifest.WriteArtifacts(dir, rendered.Artifacts); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".codex", "config.toml"), []byte("broken\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, failure := range report.Failures {
		if failure.Kind == FailureGeneratedContractInvalid {
			found = true
		}
	}
	if !found {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestValidate_PluginProjectWarnsOnUnknownAndDeprecatedFields(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mustWriteValidateFile(t, dir, "go.mod", "module example.com/x\n\ngo 1.22\n")
	mustWriteValidateFile(t, dir, "README.md", "# x\n")
	mustWriteValidateFile(t, dir, "AGENTS.md", "repo instructions\n")
	mustWriteValidateFile(t, dir, filepath.Join("cmd", "x", "main.go"), "package main\nfunc main() {}\n")
	mustWriteValidateFile(t, dir, pluginmanifest.FileName, `schema_version: 1
name: "x"
version: "0.1.0"
description: "plugin"
runtime: "go"
entrypoint: "./bin/x"
targets:
  enabled:
    - "codex"
  codex:
    model: "gpt-5-codex"
  claude: {}
  extra: true
components:
  skills: []
  commands: []
  hooks: []
`)
	rendered, err := pluginmanifest.Render(dir, "all")
	if err != nil {
		t.Fatal(err)
	}
	if err := pluginmanifest.WriteArtifacts(dir, rendered.Artifacts); err != nil {
		t.Fatal(err)
	}

	report, err := Validate(dir, "codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("failures = %+v", report.Failures)
	}
	if len(report.Warnings) < 3 {
		t.Fatalf("warnings = %+v", report.Warnings)
	}
	if err := Run(dir, "codex"); err != nil {
		t.Fatalf("warnings-only validate should succeed, got %v", err)
	}
}
