package render

import (
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/adapters/filesystem"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/domain"
)

type ClaudeRenderer struct{}

func (ClaudeRenderer) Target() string { return "claude" }

func (ClaudeRenderer) Render(name string, doc domain.SkillDocument) ([]domain.Artifact, error) {
	body, err := filesystem.RenderTemplate("render.claude.md.tmpl", filesystem.TemplateData{
		SkillName:            name,
		Description:          doc.Spec.Description,
		CommandLine:          filesystem.CommandLine(doc.Spec),
		Runtime:              string(doc.Spec.Runtime),
		AllowedTools:         doc.Spec.AllowedTools,
		CompatibilitySummary: compatibilitySummary(doc.Spec.Compatibility),
		Body:                 doc.Body,
	})
	if err != nil {
		return nil, err
	}
	return []domain.Artifact{{
		RelPath: "generated/skills/claude/" + name + "/SKILL.md",
		Content: body,
	}}, nil
}
