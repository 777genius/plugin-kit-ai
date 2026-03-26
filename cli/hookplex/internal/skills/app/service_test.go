package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hookplex/hookplex/cli/internal/skills/adapters/filesystem"
)

func TestServiceInitGoCommand(t *testing.T) {
	t.Parallel()
	svc := Service{}
	root := t.TempDir()
	out, err := svc.Init(InitOptions{
		Name:      "lint-repo",
		Template:  filesystem.TemplateGoCommand,
		OutputDir: root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, filepath.Join("skills", "lint-repo")) {
		t.Fatalf("out = %q", out)
	}
	for _, rel := range []string{
		filepath.Join("skills", "lint-repo", "SKILL.md"),
		filepath.Join("cmd", "lint-repo", "main.go"),
		filepath.Join("cmd", "lint-repo", "main_test.go"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestServiceValidateAndRender(t *testing.T) {
	t.Parallel()
	svc := Service{}
	root := t.TempDir()
	if _, err := svc.Init(InitOptions{
		Name:      "lint-repo",
		Template:  filesystem.TemplateGoCommand,
		OutputDir: root,
	}); err != nil {
		t.Fatal(err)
	}
	report, err := svc.Validate(ValidateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("failures = %+v", report.Failures)
	}
	artifacts, err := svc.Render(RenderOptions{Root: root, Target: "all"})
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) == 0 {
		t.Fatal("expected artifacts")
	}
	if err := svc.WriteArtifacts(root, artifacts); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		filepath.Join("generated", "skills", "claude", "lint-repo", "SKILL.md"),
		filepath.Join("generated", "skills", "codex", "lint-repo", "SKILL.md"),
		filepath.Join("generated", "skills", "codex", "lint-repo", "AGENTS.md"),
		filepath.Join("commands", "lint-repo.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestServiceRenderDocsOnlySkipsCommandDoc(t *testing.T) {
	t.Parallel()
	svc := Service{}
	root := t.TempDir()
	if _, err := svc.Init(InitOptions{
		Name:      "playbook",
		Template:  filesystem.TemplateDocsOnly,
		OutputDir: root,
	}); err != nil {
		t.Fatal(err)
	}
	artifacts, err := svc.Render(RenderOptions{Root: root, Target: "all"})
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range artifacts {
		if artifact.RelPath == filepath.Join("commands", "playbook.md") {
			t.Fatalf("unexpected command doc for docs-only skill: %s", artifact.RelPath)
		}
	}
}

func TestServiceValidateReportsMissingSection(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills", "bad"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `---
name: bad
description: bad skill
execution_mode: docs_only
supported_agents:
  - claude
---

# bad

## What it does

something
`
	if err := os.WriteFile(filepath.Join(root, "skills", "bad", "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := Service{}
	report, err := svc.Validate(ValidateOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Failures) == 0 {
		t.Fatal("expected failures")
	}
}
