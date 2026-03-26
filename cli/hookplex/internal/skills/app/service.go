package app

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hookplex/hookplex/cli/internal/skills/adapters/filesystem"
	"github.com/hookplex/hookplex/cli/internal/skills/adapters/render"
	"github.com/hookplex/hookplex/cli/internal/skills/domain"
)

type InitOptions struct {
	Name        string
	Description string
	Template    filesystem.InitTemplate
	OutputDir   string
	Command     string
	Force       bool
}

type ValidateOptions struct {
	Root string
}

type RenderOptions struct {
	Root   string
	Target string
}

type ValidationFailure struct {
	Path    string
	Message string
}

type ValidationReport struct {
	Skills   []string
	Failures []ValidationFailure
}

type Service struct {
	Repo filesystem.Repository
}

func (s Service) Init(opts InitOptions) (string, error) {
	name := strings.TrimSpace(opts.Name)
	if err := validateName(name); err != nil {
		return "", err
	}
	root := strings.TrimSpace(opts.OutputDir)
	if root == "" {
		root = "."
	}
	desc := strings.TrimSpace(opts.Description)
	if desc == "" {
		desc = "hookplex skill"
	}
	command := strings.TrimSpace(opts.Command)
	if command == "" {
		command = "replace-me"
	}
	if err := s.Repo.InitSkill(root, filesystem.TemplateData{
		SkillName:   name,
		Description: desc,
		Command:     command,
		CommandLine: command,
	}, opts.Template, opts.Force); err != nil {
		return "", err
	}
	return filepath.Join(root, "skills", name), nil
}

func (s Service) Validate(opts ValidateOptions) (ValidationReport, error) {
	names, err := s.Repo.Discover(opts.Root)
	if err != nil {
		return ValidationReport{}, err
	}
	report := ValidationReport{Skills: names}
	for _, name := range names {
		doc, err := s.Repo.LoadSkill(opts.Root, name)
		if err != nil {
			report.Failures = append(report.Failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: err.Error()})
			continue
		}
		report.Failures = append(report.Failures, validateDoc(name, doc)...)
	}
	return report, nil
}

func (s Service) Render(opts RenderOptions) ([]domain.Artifact, error) {
	names, err := s.Repo.Discover(opts.Root)
	if err != nil {
		return nil, err
	}
	var renderers []renderer
	switch strings.ToLower(strings.TrimSpace(opts.Target)) {
	case "", "all":
		renderers = []renderer{render.ClaudeRenderer{}, render.CodexRenderer{}}
	case "claude":
		renderers = []renderer{render.ClaudeRenderer{}}
	case "codex":
		renderers = []renderer{render.CodexRenderer{}}
	default:
		return nil, fmt.Errorf("unknown render target %q", opts.Target)
	}
	var out []domain.Artifact
	for _, name := range names {
		doc, err := s.Repo.LoadSkill(opts.Root, name)
		if err != nil {
			return nil, err
		}
		for _, r := range renderers {
			artifacts, err := r.Render(name, doc)
			if err != nil {
				return nil, err
			}
			out = append(out, artifacts...)
		}
		if doc.Spec.ExecutionMode == domain.ExecutionCommand {
			cmdBody, err := filesystem.RenderTemplate("command.md.tmpl", filesystem.TemplateData{
				SkillName:    name,
				Description:  doc.Spec.Description,
				CommandLine:  filesystem.CommandLine(doc.Spec),
				AllowedTools: doc.Spec.AllowedTools,
			})
			if err != nil {
				return nil, err
			}
			out = append(out, domain.Artifact{
				RelPath: filepath.Join("commands", name+".md"),
				Content: cmdBody,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}

func (s Service) WriteArtifacts(root string, artifacts []domain.Artifact) error {
	return s.Repo.WriteArtifacts(root, artifacts)
}

type renderer interface {
	Render(name string, doc domain.SkillDocument) ([]domain.Artifact, error)
}

func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("skill name is empty")
	}
	for _, r := range name {
		if !(r == '-' || r == '_' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return fmt.Errorf("invalid skill name %q", name)
		}
	}
	return nil
}

func validateDoc(name string, doc domain.SkillDocument) []ValidationFailure {
	var failures []ValidationFailure
	if strings.TrimSpace(doc.Spec.Name) == "" {
		failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "missing frontmatter field: name"})
	}
	if strings.TrimSpace(doc.Spec.Description) == "" {
		failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "missing frontmatter field: description"})
	}
	switch doc.Spec.ExecutionMode {
	case domain.ExecutionDocsOnly, domain.ExecutionCommand:
	default:
		failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "invalid execution_mode"})
	}
	if len(doc.Spec.SupportedAgents) == 0 {
		failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "missing frontmatter field: supported_agents"})
	}
	for _, tool := range doc.Spec.AllowedTools {
		if strings.TrimSpace(tool) == "" {
			failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "allowed_tools cannot contain empty values"})
		}
	}
	for _, agent := range doc.Spec.SupportedAgents {
		switch agent {
		case domain.AgentClaude, domain.AgentCodex:
		default:
			failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: fmt.Sprintf("unsupported agent %q", agent)})
		}
	}
	if doc.Spec.ExecutionMode == domain.ExecutionCommand {
		if strings.TrimSpace(doc.Spec.Command) == "" {
			failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "execution_mode=command requires command"})
		}
		switch doc.Spec.Runtime {
		case domain.RuntimeGo, domain.RuntimeShell, domain.RuntimePython, domain.RuntimeNode, domain.RuntimeDeno, domain.RuntimeExternal, domain.RuntimeGeneric:
		default:
			failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "execution_mode=command requires valid runtime"})
		}
	}
	requiredSections := []string{"## What it does", "## When to use", "## How to run", "## Constraints"}
	for _, section := range requiredSections {
		if !strings.Contains(doc.Body, section) {
			failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: "missing section: " + strings.TrimPrefix(section, "## ")})
		}
	}
	return failures
}
