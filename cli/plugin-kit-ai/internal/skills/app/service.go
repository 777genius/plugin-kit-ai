package app

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/adapters/filesystem"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/adapters/render"
	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/domain"
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

type RenderResult struct {
	Artifacts  []domain.Artifact
	StalePaths []string
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
		desc = "plugin-kit-ai skill"
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
		report.Failures = append(report.Failures, validateDoc(opts.Root, name, doc)...)
	}
	return report, nil
}

func (s Service) Render(opts RenderOptions) (RenderResult, error) {
	names, err := s.Repo.Discover(opts.Root)
	if err != nil {
		return RenderResult{}, err
	}
	var renderers []renderer
	selectedTargets := make(map[string]struct{})
	switch strings.ToLower(strings.TrimSpace(opts.Target)) {
	case "", "all":
		renderers = []renderer{render.ClaudeRenderer{}, render.CodexRenderer{}}
		selectedTargets["claude"] = struct{}{}
		selectedTargets["codex"] = struct{}{}
	case "claude":
		renderers = []renderer{render.ClaudeRenderer{}}
		selectedTargets["claude"] = struct{}{}
	case "codex":
		renderers = []renderer{render.CodexRenderer{}}
		selectedTargets["codex"] = struct{}{}
	default:
		return RenderResult{}, fmt.Errorf("unknown render target %q", opts.Target)
	}
	docs := make(map[string]domain.SkillDocument, len(names))
	var failures []ValidationFailure
	for _, name := range names {
		doc, err := s.Repo.LoadSkill(opts.Root, name)
		if err != nil {
			failures = append(failures, ValidationFailure{Path: filepath.Join("skills", name, "SKILL.md"), Message: err.Error()})
			continue
		}
		docs[name] = doc
		failures = append(failures, validateDoc(opts.Root, name, doc)...)
	}
	if len(failures) > 0 {
		return RenderResult{}, formatValidationError("cannot render invalid skills", failures)
	}
	var out []domain.Artifact
	managed := make(map[string]struct{})
	existingManaged, err := s.Repo.ListManagedArtifacts(opts.Root, selectedTargets)
	if err != nil {
		return RenderResult{}, err
	}
	for _, path := range existingManaged {
		managed[path] = struct{}{}
	}
	for _, name := range names {
		doc := docs[name]
		supportedRenderers := renderersForSkill(doc.Spec, renderers)
		if len(supportedRenderers) == 0 {
			for path := range managedPathsForSkill(name, doc.Spec, selectedTargets) {
				managed[path] = struct{}{}
			}
			continue
		}
		for _, r := range supportedRenderers {
			artifacts, err := r.Render(name, doc)
			if err != nil {
				return RenderResult{}, err
			}
			out = append(out, artifacts...)
		}
		if doc.Spec.ExecutionMode == domain.ExecutionCommand {
			cmdBody, err := filesystem.RenderTemplate("command.md.tmpl", filesystem.TemplateData{
				SkillName:            name,
				Description:          doc.Spec.Description,
				CommandLine:          filesystem.CommandLine(doc.Spec),
				Runtime:              string(doc.Spec.Runtime),
				AllowedTools:         doc.Spec.AllowedTools,
				CompatibilitySummary: compatibilitySummary(doc.Spec.Compatibility),
				ExecutionNotes:       executionNotes(doc.Spec),
			})
			if err != nil {
				return RenderResult{}, err
			}
			out = append(out, domain.Artifact{
				RelPath: filepath.Join("commands", name+".md"),
				Content: cmdBody,
			})
		}
		for path := range managedPathsForSkill(name, doc.Spec, selectedTargets) {
			managed[path] = struct{}{}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	keep := make(map[string]struct{}, len(out))
	for _, artifact := range out {
		keep[artifact.RelPath] = struct{}{}
	}
	var stale []string
	for path := range managed {
		if _, ok := keep[path]; !ok {
			stale = append(stale, path)
		}
	}
	sort.Strings(stale)
	return RenderResult{Artifacts: out, StalePaths: stale}, nil
}

func (s Service) WriteArtifacts(root string, artifacts []domain.Artifact) error {
	return s.Repo.WriteArtifacts(root, artifacts)
}

func (s Service) RemoveArtifacts(root string, relPaths []string) error {
	return s.Repo.RemoveArtifacts(root, relPaths)
}

type renderer interface {
	Render(name string, doc domain.SkillDocument) ([]domain.Artifact, error)
	Target() string
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

func validateDoc(root, name string, doc domain.SkillDocument) []ValidationFailure {
	var failures []ValidationFailure
	skillPath := filepath.Join("skills", name, "SKILL.md")
	if strings.TrimSpace(doc.Spec.Name) == "" {
		failures = append(failures, ValidationFailure{Path: skillPath, Message: "missing frontmatter field: name"})
	} else if strings.TrimSpace(doc.Spec.Name) != name {
		failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("frontmatter name %q must match skill directory %q", doc.Spec.Name, name)})
	} else if err := validateName(strings.TrimSpace(doc.Spec.Name)); err != nil {
		failures = append(failures, ValidationFailure{Path: skillPath, Message: err.Error()})
	}
	if strings.TrimSpace(doc.Spec.Description) == "" {
		failures = append(failures, ValidationFailure{Path: skillPath, Message: "missing frontmatter field: description"})
	}
	switch doc.Spec.ExecutionMode {
	case domain.ExecutionDocsOnly, domain.ExecutionCommand:
	default:
		failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("invalid execution_mode %q (expected %q or %q)", doc.Spec.ExecutionMode, domain.ExecutionDocsOnly, domain.ExecutionCommand)})
	}
	if len(doc.Spec.SupportedAgents) == 0 {
		failures = append(failures, ValidationFailure{Path: skillPath, Message: "missing frontmatter field: supported_agents"})
	}
	seenTools := make(map[string]struct{}, len(doc.Spec.AllowedTools))
	for _, tool := range doc.Spec.AllowedTools {
		trimmed := strings.TrimSpace(tool)
		if trimmed == "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "allowed_tools cannot contain empty values"})
			continue
		}
		if _, ok := seenTools[trimmed]; ok {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("allowed_tools contains duplicate value %q", trimmed)})
			continue
		}
		seenTools[trimmed] = struct{}{}
	}
	for _, input := range doc.Spec.Inputs {
		if strings.TrimSpace(input) == "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "inputs cannot contain empty values"})
		}
	}
	for _, output := range doc.Spec.Outputs {
		if strings.TrimSpace(output) == "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "outputs cannot contain empty values"})
		}
	}
	for _, require := range doc.Spec.Compatibility.Requires {
		if strings.TrimSpace(require) == "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "compatibility.requires cannot contain empty values"})
		}
	}
	for _, osName := range doc.Spec.Compatibility.SupportedOS {
		if strings.TrimSpace(osName) == "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "compatibility.supported_os cannot contain empty values"})
		}
	}
	for _, note := range doc.Spec.Compatibility.Notes {
		if strings.TrimSpace(note) == "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "compatibility.notes cannot contain empty values"})
		}
	}
	agentHintKeys := make([]string, 0, len(doc.Spec.AgentHints))
	for key := range doc.Spec.AgentHints {
		agentHintKeys = append(agentHintKeys, key)
	}
	sort.Strings(agentHintKeys)
	for _, key := range agentHintKeys {
		hint := doc.Spec.AgentHints[key]
		switch domain.Agent(key) {
		case domain.AgentClaude, domain.AgentCodex:
		default:
			failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("unsupported agent_hints key %q", key)})
			continue
		}
		if !containsAgent(doc.Spec.SupportedAgents, domain.Agent(key)) {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("agent_hints.%s requires %q in supported_agents", key, key)})
		}
		for _, note := range hint.Notes {
			if strings.TrimSpace(note) == "" {
				failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("agent_hints.%s.notes cannot contain empty values", key)})
			}
		}
	}
	seenAgents := make(map[domain.Agent]struct{}, len(doc.Spec.SupportedAgents))
	for _, agent := range doc.Spec.SupportedAgents {
		if _, ok := seenAgents[agent]; ok {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("supported_agents contains duplicate value %q", agent)})
			continue
		}
		seenAgents[agent] = struct{}{}
		switch agent {
		case domain.AgentClaude, domain.AgentCodex:
		default:
			failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("unsupported agent %q (supported: %q, %q)", agent, domain.AgentClaude, domain.AgentCodex)})
		}
	}
	if doc.Spec.ExecutionMode == domain.ExecutionCommand {
		if strings.TrimSpace(doc.Spec.Command) == "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=command requires command"})
		}
		if wd := strings.TrimSpace(doc.Spec.WorkingDir); wd != "" {
			clean := filepath.Clean(wd)
			if filepath.IsAbs(wd) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
				failures = append(failures, ValidationFailure{Path: skillPath, Message: "working_dir must stay within the skill root"})
			} else {
				full := filepath.Join(root, "skills", name, clean)
				info, err := os.Stat(full)
				if err != nil {
					if os.IsNotExist(err) {
						failures = append(failures, ValidationFailure{Path: skillPath, Message: "working_dir must reference an existing directory under the skill root"})
					} else {
						failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("working_dir could not be checked: %v", err)})
					}
				} else if !info.IsDir() {
					failures = append(failures, ValidationFailure{Path: skillPath, Message: "working_dir must reference an existing directory under the skill root"})
				}
			}
		}
		if timeout := strings.TrimSpace(doc.Spec.Timeout); timeout != "" {
			if _, err := time.ParseDuration(timeout); err != nil {
				failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("timeout must be a valid duration: %v", err)})
			}
		}
		switch doc.Spec.Runtime {
		case domain.RuntimeGo, domain.RuntimeShell, domain.RuntimePython, domain.RuntimeNode, domain.RuntimeDeno, domain.RuntimeExternal, domain.RuntimeGeneric:
		default:
			failures = append(failures, ValidationFailure{Path: skillPath, Message: fmt.Sprintf("execution_mode=command requires valid runtime (got %q)", doc.Spec.Runtime)})
		}
	} else {
		if strings.TrimSpace(doc.Spec.Command) != "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define command"})
		}
		if len(doc.Spec.Args) > 0 {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define args"})
		}
		if strings.TrimSpace(string(doc.Spec.Runtime)) != "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define runtime"})
		}
		if strings.TrimSpace(doc.Spec.WorkingDir) != "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define working_dir"})
		}
		if strings.TrimSpace(doc.Spec.Timeout) != "" {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define timeout"})
		}
		if doc.Spec.SafeToRetry != nil {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define safe_to_retry"})
		}
		if doc.Spec.WritesFiles != nil {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define writes_files"})
		}
		if doc.Spec.ProducesJSON != nil {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "execution_mode=docs_only must not define produces_json"})
		}
	}
	requiredSections := []string{"## What it does", "## When to use", "## How to run", "## Constraints"}
	for _, section := range requiredSections {
		if !strings.Contains(doc.Body, section) {
			failures = append(failures, ValidationFailure{Path: skillPath, Message: "missing section: " + strings.TrimPrefix(section, "## ")})
		}
	}
	return failures
}

func compatibilitySummary(spec domain.CompatibilitySpec) []string {
	var out []string
	if len(spec.Requires) > 0 {
		out = append(out, "Requires: "+strings.Join(spec.Requires, ", "))
	}
	if len(spec.SupportedOS) > 0 {
		out = append(out, "Supported OS: "+strings.Join(spec.SupportedOS, ", "))
	}
	if spec.RepoRequired {
		out = append(out, "Requires a repository checkout")
	}
	if spec.NetworkRequired {
		out = append(out, "May require network access")
	}
	out = append(out, spec.Notes...)
	return out
}

func executionNotes(spec domain.SkillSpec) []string {
	var out []string
	if spec.SafeToRetry != nil {
		out = append(out, "Safe to retry: "+yesNo(*spec.SafeToRetry))
	}
	if spec.WritesFiles != nil {
		out = append(out, "Writes files: "+yesNo(*spec.WritesFiles))
	}
	if spec.ProducesJSON != nil {
		out = append(out, "Produces JSON: "+yesNo(*spec.ProducesJSON))
	}
	return out
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func renderersForSkill(spec domain.SkillSpec, candidates []renderer) []renderer {
	allowed := make(map[string]struct{}, len(spec.SupportedAgents))
	for _, agent := range spec.SupportedAgents {
		allowed[string(agent)] = struct{}{}
	}
	out := make([]renderer, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := allowed[candidate.Target()]; ok {
			out = append(out, candidate)
		}
	}
	return out
}

func containsAgent(agents []domain.Agent, want domain.Agent) bool {
	for _, agent := range agents {
		if agent == want {
			return true
		}
	}
	return false
}

func managedPathsForSkill(name string, spec domain.SkillSpec, selectedTargets map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{})
	if _, ok := selectedTargets["claude"]; ok {
		out[filepath.Join("generated", "skills", "claude", name, "SKILL.md")] = struct{}{}
	}
	if _, ok := selectedTargets["codex"]; ok {
		out[filepath.Join("generated", "skills", "codex", name, "SKILL.md")] = struct{}{}
		out[filepath.Join("generated", "skills", "codex", name, "AGENTS.md")] = struct{}{}
	}
	out[filepath.Join("commands", name+".md")] = struct{}{}
	return out
}

func formatValidationError(prefix string, failures []ValidationFailure) error {
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString(":\n")
	for _, failure := range failures {
		b.WriteString("- ")
		b.WriteString(failure.Path)
		b.WriteString(": ")
		b.WriteString(failure.Message)
		b.WriteString("\n")
	}
	return fmt.Errorf(strings.TrimRight(b.String(), "\n"))
}
