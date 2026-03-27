package render

import (
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/adapters/filesystem"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/domain"
)

type CodexRenderer struct{}

func (CodexRenderer) Target() string { return "codex" }

func (CodexRenderer) Render(name string, doc domain.SkillDocument) ([]domain.Artifact, error) {
	skillBody, err := filesystem.RenderTemplate("render.codex.skill.md.tmpl", filesystem.TemplateData{
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
	agentsBody, err := filesystem.RenderTemplate("render.codex.agents.md.tmpl", filesystem.TemplateData{
		SkillName:            name,
		Description:          doc.Spec.Description,
		CommandLine:          filesystem.CommandLine(doc.Spec),
		Runtime:              string(doc.Spec.Runtime),
		AllowedTools:         doc.Spec.AllowedTools,
		CompatibilitySummary: compatibilitySummary(doc.Spec.Compatibility),
	})
	if err != nil {
		return nil, err
	}
	return []domain.Artifact{
		{
			RelPath: "generated/skills/codex/" + name + "/SKILL.md",
			Content: skillBody,
		},
		{
			RelPath: "generated/skills/codex/" + name + "/AGENTS.md",
			Content: agentsBody,
		},
	}, nil
}
