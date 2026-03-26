package render

import (
	"github.com/hookplex/hookplex/cli/internal/skills/adapters/filesystem"
	"github.com/hookplex/hookplex/cli/internal/skills/domain"
)

type CodexRenderer struct{}

func (CodexRenderer) Target() string { return "codex" }

func (CodexRenderer) Render(name string, doc domain.SkillDocument) ([]domain.Artifact, error) {
	skillBody, err := filesystem.RenderTemplate("render.codex.skill.md.tmpl", filesystem.TemplateData{
		SkillName:    name,
		Description:  doc.Spec.Description,
		CommandLine:  filesystem.CommandLine(doc.Spec),
		AllowedTools: doc.Spec.AllowedTools,
		Body:         doc.Body,
	})
	if err != nil {
		return nil, err
	}
	agentsBody, err := filesystem.RenderTemplate("render.codex.agents.md.tmpl", filesystem.TemplateData{
		SkillName:    name,
		Description:  doc.Spec.Description,
		CommandLine:  filesystem.CommandLine(doc.Spec),
		AllowedTools: doc.Spec.AllowedTools,
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
