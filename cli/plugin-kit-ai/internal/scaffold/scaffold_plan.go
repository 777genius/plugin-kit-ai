package scaffold

import (
	"fmt"
	"strings"
)

// Paths lists relative paths created by Write (for tests and docs).
func Paths(platform, name string, extras bool) []string {
	if platform == "gemini" || platform == "codex-package" || platform == "opencode" || platform == "cursor" || platform == "cursor-workspace" {
		return PathsForRuntime(platform, "", name, extras)
	}
	return PathsForRuntime(platform, RuntimeGo, name, extras)
}

func PathsForRuntime(platform, runtime, name string, extras bool) []string {
	return pathsForRuntime(platform, runtime, name, extras, false, false)
}

func PathsForRuntimeSharedPackage(platform, runtime, name string, extras bool) []string {
	return pathsForRuntime(platform, runtime, name, extras, false, true)
}

func PathsForRuntimeTypeScript(platform, name string, extras bool) []string {
	return pathsForRuntime(platform, RuntimeNode, name, extras, true, false)
}

func PathsForRuntimeTypeScriptSharedPackage(platform, name string, extras bool) []string {
	return pathsForRuntime(platform, RuntimeNode, name, extras, true, true)
}

func pathsForRuntime(platform, runtime, name string, extras bool, typescript bool, sharedRuntimePackage bool) []string {
	def, ok := LookupPlatform(platform)
	if !ok {
		return nil
	}
	runtime = normalizeRuntime(runtime)
	return planPaths(expandTemplateFiles(planFilesFor(def.Name, runtime, extras, typescript, sharedRuntimePackage), Data{
		ProjectName:          name,
		Platform:             def.Name,
		Runtime:              runtime,
		TypeScript:           typescript,
		SharedRuntimePackage: sharedRuntimePackage,
		ExecutionMode:        defaultExecutionMode(runtime),
		Entrypoint:           "./bin/" + name,
		WithExtras:           extras,
	}))
}

func BuildPlan(d Data) (ProjectPlan, error) {
	if err := ValidateProjectName(d.ProjectName); err != nil {
		return ProjectPlan{}, err
	}
	if strings.TrimSpace(d.ModulePath) == "" {
		d.ModulePath = DefaultModulePath(d.ProjectName)
	}
	if strings.TrimSpace(d.Description) == "" {
		d.Description = "plugin-kit-ai plugin"
	}
	if strings.TrimSpace(d.Version) == "" {
		d.Version = "0.1.0"
	}
	if strings.TrimSpace(d.GoSDKVersion) == "" {
		d.GoSDKVersion = DefaultGoSDKVersion
	}
	if IsPackageOnlyJobTemplate(d.JobTemplate) {
		return buildJobTemplatePlan(d)
	}
	p, ok := LookupPlatform(d.Platform)
	if !ok {
		return ProjectPlan{}, fmt.Errorf("unknown platform %q", d.Platform)
	}
	d.Platform = p.Name
	if d.Platform == "codex-package" || d.Platform == "opencode" || d.Platform == "cursor" || d.Platform == "cursor-workspace" {
		if d.TypeScript {
			return ProjectPlan{}, fmt.Errorf("--typescript is not supported with --platform %s", d.Platform)
		}
		if strings.TrimSpace(d.Runtime) != "" {
			return ProjectPlan{}, fmt.Errorf("--runtime is not supported with --platform %s", d.Platform)
		}
		d.Runtime = ""
		d.Entrypoint = ""
		d.ExecutionMode = ""
	} else {
		if d.Platform == "gemini" {
			d.Runtime = strings.ToLower(strings.TrimSpace(d.Runtime))
			if d.Runtime != "" && d.Runtime != RuntimeGo {
				return ProjectPlan{}, fmt.Errorf("--runtime is not supported with --platform %s", d.Platform)
			}
			if d.TypeScript {
				return ProjectPlan{}, fmt.Errorf("--typescript is not supported with --platform %s", d.Platform)
			}
			if d.Runtime == "" {
				d.Entrypoint = ""
				d.ExecutionMode = ""
			} else {
				if strings.TrimSpace(d.Entrypoint) == "" {
					d.Entrypoint = "./bin/" + d.ProjectName
				}
				if strings.TrimSpace(d.ExecutionMode) == "" {
					d.ExecutionMode = defaultExecutionMode(d.Runtime)
				}
			}
		} else {
			d.Runtime = normalizeRuntime(d.Runtime)
			if _, ok := LookupRuntime(d.Runtime); !ok {
				return ProjectPlan{}, fmt.Errorf("unknown runtime %q", d.Runtime)
			}
			if d.TypeScript && d.Runtime != RuntimeNode {
				return ProjectPlan{}, fmt.Errorf("--typescript requires --runtime node")
			}
			if strings.TrimSpace(d.Entrypoint) == "" {
				d.Entrypoint = "./bin/" + d.ProjectName
			}
			if strings.TrimSpace(d.ExecutionMode) == "" {
				d.ExecutionMode = defaultExecutionMode(d.Runtime)
			}
			if d.SharedRuntimePackage && d.Runtime != RuntimePython && d.Runtime != RuntimeNode {
				return ProjectPlan{}, fmt.Errorf("--runtime-package requires --runtime python or --runtime node")
			}
			if d.SharedRuntimePackage {
				d.RuntimePackageVersion = normalizePackageVersion(d.RuntimePackageVersion)
				if d.RuntimePackageVersion == "" {
					return ProjectPlan{}, fmt.Errorf("--runtime-package requires a pinned runtime package version")
				}
			}
		}
	}
	if d.WithExtras {
		if !d.HasSkills {
			d.HasSkills = true
		}
		if !d.HasCommands {
			d.HasCommands = true
		}
	}
	if d.Platform == "codex-runtime" && strings.TrimSpace(d.CodexModel) == "" {
		d.CodexModel = DefaultCodexModel
	}
	return ProjectPlan{
		Platform: p.Name,
		Data:     d,
		Files:    expandTemplateFiles(planFilesFor(p.Name, d.Runtime, d.WithExtras, d.TypeScript, d.SharedRuntimePackage), d),
	}, nil
}

func buildJobTemplatePlan(d Data) (ProjectPlan, error) {
	templateName := NormalizeTemplate(d.JobTemplate)
	targets := d.EffectiveTargets()
	if len(targets) == 0 {
		targets = DefaultJobTemplateTargets(templateName)
	}
	if len(targets) == 0 {
		return ProjectPlan{}, fmt.Errorf("template %q requires at least one target", templateName)
	}
	for i, target := range targets {
		p, ok := LookupPlatform(target)
		if !ok {
			return ProjectPlan{}, fmt.Errorf("unknown platform %q", target)
		}
		if p.Name == "codex-runtime" {
			return ProjectPlan{}, fmt.Errorf("template %q does not support --platform %s", templateName, p.Name)
		}
		targets[i] = p.Name
	}
	d.Targets = targets
	if len(targets) == 1 {
		d.Platform = targets[0]
	} else {
		d.Platform = ""
	}
	if d.WithExtras {
		d.HasSkills = true
	}
	return ProjectPlan{
		Platform: strings.Join(targets, ","),
		Data:     d,
		Files:    expandTemplateFiles(jobTemplateFilesFor(templateName, d.WithExtras), d),
	}, nil
}

func jobTemplateFilesFor(templateName string, extras bool) []TemplateFile {
	files := []TemplateFile{
		{Path: "src/plugin.yaml", Template: "plugin.yaml.tmpl", Extra: false},
		{Path: "src/mcp/servers.yaml", Template: "job.online-service.mcp.servers.yaml.tmpl", Extra: false},
		{Path: "src/README.md", Template: "job.online-service.README.md.tmpl", Extra: false},
		{Path: "CLAUDE.md", Template: "ROOT.CLAUDE.md.tmpl", Extra: false},
		{Path: "AGENTS.md", Template: "ROOT.AGENTS.md.tmpl", Extra: false},
	}
	switch templateName {
	case InitTemplateLocalTool:
		files[1].Template = "job.local-tool.mcp.servers.yaml.tmpl"
		files[2].Template = "job.local-tool.README.md.tmpl"
	}
	if extras {
		files = append(files, TemplateFile{Path: "src/skills/{{.ProjectName}}/SKILL.md", Template: "SKILL.md.tmpl", Extra: true})
	}
	return files
}

func planFilesFor(platform, runtime string, extras, typescript, sharedRuntimePackage bool) []TemplateFile {
	files := append([]TemplateFile(nil), filesFor(platform, runtime, extras, typescript, sharedRuntimePackage)...)
	for _, file := range runtimeTestScaffoldFiles(platform) {
		files = appendUniqueTemplateFile(files, file)
	}
	return files
}

func appendUniqueTemplateFile(files []TemplateFile, candidate TemplateFile) []TemplateFile {
	for _, file := range files {
		if file.Path == candidate.Path {
			return files
		}
	}
	return append(files, candidate)
}

func planPaths(tasks []PlannedFile) []string {
	out := make([]string, 0, len(tasks))
	for _, task := range tasks {
		out = append(out, task.RelPath)
	}
	return out
}

func expandTemplateFiles(files []TemplateFile, d Data) []PlannedFile {
	out := make([]PlannedFile, 0, len(files))
	for _, file := range files {
		if file.Extra && !d.WithExtras {
			continue
		}
		out = append(out, PlannedFile{
			RelPath:  expandPathTemplate(file.Path, d),
			Template: file.Template,
		})
	}
	return out
}

func expandPathTemplate(path string, d Data) string {
	path = strings.ReplaceAll(path, "{{.ProjectName}}", d.ProjectName)
	path = strings.ReplaceAll(path, "{{.Platform}}", d.Platform)
	return path
}
