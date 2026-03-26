package frontmatter

import (
	"fmt"
	"strings"

	"github.com/hookplex/hookplex/cli/internal/skills/domain"
	"gopkg.in/yaml.v3"
)

func Parse(body []byte) (domain.SkillDocument, error) {
	src := string(body)
	if !strings.HasPrefix(src, "---\n") {
		return domain.SkillDocument{}, fmt.Errorf("SKILL.md must start with YAML frontmatter")
	}
	rest := strings.TrimPrefix(src, "---\n")
	idx := strings.Index(rest, "\n---\n")
	if idx < 0 {
		return domain.SkillDocument{}, fmt.Errorf("SKILL.md frontmatter terminator not found")
	}
	var spec domain.SkillSpec
	if err := yaml.Unmarshal([]byte(rest[:idx]), &spec); err != nil {
		return domain.SkillDocument{}, fmt.Errorf("parse SKILL.md frontmatter: %w", err)
	}
	markdown := strings.TrimSpace(rest[idx+len("\n---\n"):])
	return domain.SkillDocument{
		Spec: spec,
		Body: markdown,
	}, nil
}
