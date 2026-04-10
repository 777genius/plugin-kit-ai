package platformexec

import (
	"path/filepath"
	"testing"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func TestPortableSkillChildPath_NormalizesAuthoredRoots(t *testing.T) {
	t.Parallel()

	for _, rel := range []string{
		filepath.Join(pluginmodel.SourceDirName, "skills", "demo", "SKILL.md"),
		filepath.Join(pluginmodel.LegacySourceDirName, "skills", "demo", "SKILL.md"),
	} {
		child, err := portableSkillChildPath(rel)
		if err != nil {
			t.Fatalf("portableSkillChildPath(%q) error = %v", rel, err)
		}
		if want := filepath.Join("demo", "SKILL.md"); child != want {
			t.Fatalf("portableSkillChildPath(%q) = %q, want %q", rel, child, want)
		}
	}
}

func TestPortableSkillChildPath_RejectsNonSkillPaths(t *testing.T) {
	t.Parallel()

	if _, err := portableSkillChildPath(filepath.Join(pluginmodel.SourceDirName, "targets", "claude", "commands", "sync.md")); err == nil {
		t.Fatal("expected error")
	}
}
