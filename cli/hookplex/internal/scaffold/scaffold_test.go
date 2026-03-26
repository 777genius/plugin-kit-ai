package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateProjectName(t *testing.T) {
	t.Parallel()
	ok := []string{"a", "ab", "my-plugin", "My_Plugin2"}
	for _, s := range ok {
		if err := ValidateProjectName(s); err != nil {
			t.Errorf("ValidateProjectName(%q) = %v, want nil", s, err)
		}
	}
	bad := []string{"", " ", "9bad", "bad@", "a" + strings.Repeat("b", 64)} // 65 chars > max 64
	for _, s := range bad {
		if err := ValidateProjectName(s); err == nil {
			t.Errorf("ValidateProjectName(%q) = nil, want error", s)
		}
	}
}

func TestLookupPlatform(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"claude", "codex"} {
		if _, ok := LookupPlatform(name); !ok {
			t.Fatalf("LookupPlatform(%q) = missing", name)
		}
	}
	if _, ok := LookupPlatform("gemini"); ok {
		t.Fatal("unexpected platform")
	}
}

func TestPaths_Codex(t *testing.T) {
	t.Parallel()
	got := Paths("codex", "my-plugin", true)
	for _, want := range []string{
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
		"AGENTS.md",
		filepath.Join(".codex", "config.toml"),
		"README.md",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("commands", "my-plugin.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestPaths_ClaudeCurrentState(t *testing.T) {
	t.Parallel()
	got := Paths("claude", "my-plugin", true)
	for _, want := range []string{
		"go.mod",
		filepath.Join("cmd", "my-plugin", "main.go"),
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join("hooks", "hooks.json"),
		"README.md",
		"Makefile",
		".goreleaser.yml",
		filepath.Join("skills", "my-plugin", "SKILL.md"),
		filepath.Join("commands", "my-plugin.md"),
	} {
		if !contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
}

func TestWrite_Codex(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "hookplex plugin",
		Platform:    "codex",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"AGENTS.md",
		filepath.Join(".codex", "config.toml"),
		filepath.Join("cmd", "my-plugin", "main.go"),
		filepath.Join("skills", "my-plugin", "SKILL.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("stat %s: %v", rel, err)
		}
	}
}

func TestWrite_ClaudeIncludesOfficialHookRoutes(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	err := Write(root, Data{
		ProjectName: "my-plugin",
		ModulePath:  DefaultModulePath("my-plugin"),
		Description: "hookplex plugin",
		Platform:    "claude",
		WithExtras:  true,
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	body, err := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	for _, hook := range []string{
		"SessionStart",
		"SessionEnd",
		"Notification",
		"PostToolUse",
		"PostToolUseFailure",
		"PermissionRequest",
		"SubagentStart",
		"SubagentStop",
		"PreCompact",
		"Setup",
		"Stop",
		"PreToolUse",
		"TeammateIdle",
		"TaskCompleted",
		"UserPromptSubmit",
		"ConfigChange",
		"WorktreeCreate",
		"WorktreeRemove",
	} {
		if !strings.Contains(got, `"`+hook+`"`) {
			t.Fatalf("hooks.json missing %q:\n%s", hook, got)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, item := range haystack {
		if item == needle {
			return true
		}
	}
	return false
}
