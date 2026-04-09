package platformexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func TestCursorDetectNativeIgnoresStandaloneRootAgents(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("# Shared agents\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if (cursorAdapter{}).DetectNative(root) {
		t.Fatal("DetectNative unexpectedly matched standalone root AGENTS.md")
	}
}

func TestCursorImportIncludeUserScopeMergesGlobalMCP(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".cursor", "mcp.json"), []byte(`{"global":{"command":"node"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	imported, err := (cursorAdapter{}).Import(root, ImportSeed{
		Manifest:         pluginmodel.Manifest{Name: "demo", Version: "0.1.0", Description: "demo", Targets: []string{"cursor"}},
		IncludeUserScope: true,
	})
	if err != nil {
		t.Fatalf("Import error = %v", err)
	}
	if len(imported.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", imported.Artifacts)
	}
	if !strings.Contains(string(imported.Artifacts[0].Content), "global:") {
		t.Fatalf("portable MCP import missing global server:\n%s", imported.Artifacts[0].Content)
	}
}

func TestCursorValidateRejectsNonMdcRules(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "targets", "cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "targets", "cursor", "rules", "project.md"), []byte("# bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := (cursorAdapter{}).Validate(root, pluginmodel.PackageGraph{
		Portable: pluginmodel.NewPortableComponents(),
	}, pluginmodel.TargetState{
		Target: "cursor",
		Components: map[string][]string{
			"rules": {filepath.ToSlash(filepath.Join("targets", "cursor", "rules", "project.md"))},
		},
		Docs: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(diagnostics) == 0 {
		t.Fatal("expected validation diagnostics")
	}
	var found bool
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, ".mdc") {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestCursorValidateRejectsTraversalOrSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rulesDir := filepath.Join(root, "targets", "cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rulesDir, "real.mdc"), []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.mdc", filepath.Join(rulesDir, "linked.mdc")); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := (cursorAdapter{}).Validate(root, pluginmodel.PackageGraph{
		Portable: pluginmodel.NewPortableComponents(),
	}, pluginmodel.TargetState{
		Target: "cursor",
		Components: map[string][]string{
			"rules": {
				"targets/cursor/rules/../escape.mdc",
				filepath.ToSlash(filepath.Join("targets", "cursor", "rules", "linked.mdc")),
			},
		},
		Docs: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var foundTraversal, foundSymlink bool
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic.Message, "path traversal") {
			foundTraversal = true
		}
		if strings.Contains(diagnostic.Message, "must not be a symlink") {
			foundSymlink = true
		}
	}
	if !foundTraversal || !foundSymlink {
		t.Fatalf("diagnostics = %+v", diagnostics)
	}
}

func TestCursorMCPPreservesInterpolationStrings(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	parsed, err := pluginmodel.ParsePortableMCP("mcp/servers.yaml", []byte(`api_version: v1

servers:
  demo:
    type: stdio
    stdio:
      command: env
      args:
        - "API_KEY=${env:API_KEY}"
        - "ROOT=${workspaceFolder}"
        - "TOKEN=${input:token}"
`))
	if err != nil {
		t.Fatal(err)
	}
	graph := pluginmodel.PackageGraph{
		Portable: pluginmodel.PortableComponents{
			Items: map[string][]string{},
			MCP:   &pluginmodel.PortableMCP{Path: "mcp/servers.yaml", Servers: parsed.Servers, File: parsed.File},
		},
	}
	artifacts, err := (cursorAdapter{}).Generate(root, graph, pluginmodel.NewTargetState("cursor"))
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 2 {
		t.Fatalf("artifacts = %v", artifacts)
	}
	var got string
	for _, artifact := range artifacts {
		if artifact.RelPath == filepath.ToSlash(filepath.Join(".cursor", "mcp.json")) {
			got = string(artifact.Content)
			break
		}
	}
	if got == "" {
		t.Fatalf("artifacts missing .cursor/mcp.json: %+v", artifacts)
	}
	for _, want := range []string{"${env:API_KEY}", "${workspaceFolder}", "${input:token}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated mcp missing %q:\n%s", want, got)
		}
	}
}
