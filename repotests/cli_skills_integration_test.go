package hookplexrepo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHookplexSkillsInitValidateRender(t *testing.T) {
	bin := buildHookplex(t)
	root := t.TempDir()

	initCmd := exec.Command(bin, "skills", "init", "lint-repo", "--output", root, "--template", "go-command")
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("hookplex skills init: %v\n%s", err, out)
	}

	validateCmd := exec.Command(bin, "skills", "validate", root)
	if out, err := validateCmd.CombinedOutput(); err != nil {
		t.Fatalf("hookplex skills validate: %v\n%s", err, out)
	}

	renderCmd := exec.Command(bin, "skills", "render", root, "--target", "all")
	if out, err := renderCmd.CombinedOutput(); err != nil {
		t.Fatalf("hookplex skills render: %v\n%s", err, out)
	}

	for _, rel := range []string{
		filepath.Join("skills", "lint-repo", "SKILL.md"),
		filepath.Join("cmd", "lint-repo", "main.go"),
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
