package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/pluginmodel"
	skillfs "github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/adapters/filesystem"
	skillsapp "github.com/plugin-kit-ai/plugin-kit-ai/cli/internal/skills/app"
	"github.com/tailscale/hujson"
)

type opencodeAdapter struct{}

type opencodeImportedState struct {
	plugins         []string
	pluginsProvided bool
	mcp             map[string]any
	extra           map[string]any
	artifacts       map[string]pluginmodel.Artifact
	warnings        []pluginmodel.Warning
	hasInput        bool
}

type opencodeImportSource struct {
	dir       string
	display   string
	warnOnUse bool
	warnPath  string
	warnMsg   string
}

func (opencodeAdapter) ID() string { return "opencode" }

func (opencodeAdapter) DetectNative(root string) bool {
	_, _, ok, err := resolveOpenCodeConfigPath(root)
	return err == nil && ok
}

func (opencodeAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	if rel := strings.TrimSpace(state.DocPath("package_metadata")); rel != "" {
		meta, ok, err := readYAMLDoc[opencodePackageMeta](root, rel)
		if err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
		if ok {
			for i, plugin := range meta.Plugins {
				if strings.TrimSpace(plugin) == "" {
					return fmt.Errorf("%s plugin entry %d must be a non-empty string", rel, i)
				}
			}
		}
	}
	return nil
}

func (opencodeAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	state := opencodeImportedState{
		extra:     map[string]any{},
		artifacts: map[string]pluginmodel.Artifact{},
	}

	if seed.IncludeUserScope {
		home, err := os.UserHomeDir()
		if err != nil {
			return ImportResult{}, fmt.Errorf("resolve user home for OpenCode import: %w", err)
		}
		globalRoot := filepath.Join(home, ".config", "opencode")
		if err := importOpenCodeScope(&state, opencodeScopeConfig{
			root:              globalRoot,
			displayConfigRoot: filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
			workspaceRoot:     globalRoot,
			workspaceDisplay:  filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
			compatSkillRoots: []opencodeImportSource{
				{
					dir:       filepath.Join(home, ".agents", "skills"),
					display:   filepath.ToSlash(filepath.Join("~", ".agents", "skills")),
					warnOnUse: true,
					warnPath:  filepath.ToSlash(filepath.Join("~", ".agents", "skills")),
					warnMsg:   "normalized OpenCode-compatible skills from ~/.agents/skills into canonical portable skills/** during import",
				},
				{
					dir:       filepath.Join(home, ".claude", "skills"),
					display:   filepath.ToSlash(filepath.Join("~", ".claude", "skills")),
					warnOnUse: true,
					warnPath:  filepath.ToSlash(filepath.Join("~", ".claude", "skills")),
					warnMsg:   "normalized OpenCode-compatible skills from ~/.claude/skills into canonical portable skills/** during import",
				},
			},
			allowCompatSkills: true,
		}); err != nil {
			return ImportResult{}, err
		}
	}

	if err := importOpenCodeScope(&state, opencodeScopeConfig{
		root:              root,
		displayConfigRoot: "",
		workspaceRoot:     filepath.Join(root, ".opencode"),
		workspaceDisplay:  ".opencode",
		compatSkillRoots: []opencodeImportSource{
			{
				dir:       filepath.Join(root, ".agents", "skills"),
				display:   filepath.ToSlash(filepath.Join(".agents", "skills")),
				warnOnUse: true,
				warnPath:  filepath.ToSlash(filepath.Join(".agents", "skills")),
				warnMsg:   "normalized OpenCode-compatible skills from .agents/skills into canonical portable skills/** during import",
			},
			{
				dir:       filepath.Join(root, ".claude", "skills"),
				display:   filepath.ToSlash(filepath.Join(".claude", "skills")),
				warnOnUse: true,
				warnPath:  filepath.ToSlash(filepath.Join(".claude", "skills")),
				warnMsg:   "normalized OpenCode-compatible skills from .claude/skills into canonical portable skills/** during import",
			},
		},
		allowCompatSkills: true,
	}); err != nil {
		return ImportResult{}, err
	}

	if !state.hasInput {
		return ImportResult{}, fmt.Errorf("OpenCode import requires opencode.json, opencode.jsonc, supported workspace directories, or --include-user-scope with OpenCode native sources")
	}

	artifacts := []pluginmodel.Artifact{{
		RelPath: filepath.Join("targets", "opencode", "package.yaml"),
		Content: mustYAML(opencodePackageMeta{Plugins: append([]string(nil), state.plugins...)}),
	}}
	if len(state.mcp) > 0 {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("mcp", "servers.json"),
			Content: mustJSON(state.mcp),
		})
	}
	if len(state.extra) > 0 {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("targets", "opencode", "config.extra.json"),
			Content: mustJSON(state.extra),
		})
	}
	for _, rel := range sortedArtifactKeys(state.artifacts) {
		artifacts = append(artifacts, state.artifacts[rel])
	}
	return ImportResult{
		Manifest:  seed.Manifest,
		Launcher:  nil,
		Artifacts: compactArtifacts(artifacts),
		Warnings:  state.warnings,
	}, nil
}

func (opencodeAdapter) Render(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	meta, _, err := readYAMLDoc[opencodePackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	for i, plugin := range meta.Plugins {
		if strings.TrimSpace(plugin) == "" {
			return nil, fmt.Errorf("%s plugin entry %d must be a non-empty string", state.DocPath("package_metadata"), i)
		}
		meta.Plugins[i] = strings.TrimSpace(plugin)
	}
	extra, err := loadNativeExtraDoc(root, state, "config_extra", pluginmodel.NativeDocFormatJSON)
	if err != nil {
		return nil, err
	}
	managedPaths := []string{"$schema", "plugin", "mcp"}
	if err := pluginmodel.ValidateNativeExtraDocConflicts(extra, "opencode config.extra.json", managedPaths); err != nil {
		return nil, err
	}
	doc := map[string]any{
		"$schema": "https://opencode.ai/config.json",
	}
	if len(meta.Plugins) > 0 {
		doc["plugin"] = append([]string(nil), meta.Plugins...)
	}
	if graph.Portable.MCP != nil {
		doc["mcp"] = graph.Portable.MCP.Servers
	}
	if err := pluginmodel.MergeNativeExtraObject(doc, extra, "opencode config.extra.json", managedPaths); err != nil {
		return nil, err
	}
	body, err := marshalJSON(doc)
	if err != nil {
		return nil, err
	}
	artifacts := []pluginmodel.Artifact{{
		RelPath: "opencode.json",
		Content: body,
	}}
	skillArtifacts, err := renderPortableSkills(root, graph.Portable.Paths("skills"), ".opencode/skills")
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, skillArtifacts...)
	copied, err := copyArtifactDirs(root,
		artifactDir{src: filepath.Join("targets", "opencode", "commands"), dst: filepath.Join(".opencode", "commands")},
		artifactDir{src: filepath.Join("targets", "opencode", "agents"), dst: filepath.Join(".opencode", "agents")},
		artifactDir{src: filepath.Join("targets", "opencode", "themes"), dst: filepath.Join(".opencode", "themes")},
	)
	if err != nil {
		return nil, err
	}
	return append(artifacts, copied...), nil
}

func (opencodeAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	return nil, nil
}

func (opencodeAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
	meta, _, err := readYAMLDoc[opencodePackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	for i, plugin := range meta.Plugins {
		if strings.TrimSpace(plugin) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     state.DocPath("package_metadata"),
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode package metadata plugin entry %d must be a non-empty string", i),
			})
		}
	}
	configPath, warnings, ok, err := resolveOpenCodeConfigPath(root)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "opencode.json",
			Target:   "opencode",
			Message:  "OpenCode config opencode.json or opencode.jsonc is required",
		}}, nil
	}
	for _, warning := range warnings {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Code:     CodeManifestInvalid,
			Path:     warning.Path,
			Target:   "opencode",
			Message:  warning.Message,
		})
	}
	body, err := os.ReadFile(filepath.Join(root, configPath))
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s is not readable: %v", configPath, err),
		}}, nil
	}
	body, err = hujson.Standardize(body)
	if err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s is invalid JSON/JSONC: %v", configPath, err),
		}}, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s is invalid JSON/JSONC: %v", configPath, err),
		}}, nil
	}
	if schema, _ := doc["$schema"].(string); strings.TrimSpace(schema) != "https://opencode.ai/config.json" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     configPath,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode config %s must declare $schema %q", configPath, "https://opencode.ai/config.json"),
		})
	}
	if raw, ok := doc["plugin"]; ok {
		values, ok := raw.([]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     configPath,
				Target:   "opencode",
				Message:  `OpenCode config field "plugin" must be an array of strings`,
			})
		} else {
			for i, value := range values {
				text, ok := value.(string)
				if !ok || strings.TrimSpace(text) == "" {
					diagnostics = append(diagnostics, Diagnostic{
						Severity: SeverityFailure,
						Code:     CodeManifestInvalid,
						Path:     configPath,
						Target:   "opencode",
						Message:  fmt.Sprintf(`OpenCode config field "plugin" must contain non-empty strings (invalid entry at index %d)`, i),
					})
				}
			}
		}
	}
	if raw, ok := doc["mcp"]; ok {
		if _, ok := raw.(map[string]any); !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     configPath,
				Target:   "opencode",
				Message:  `OpenCode config field "mcp" must be a JSON object`,
			})
		}
	}
	if len(graph.Portable.Paths("skills")) > 0 {
		report, err := (skillsapp.Service{Repo: skillfs.Repository{}}).Validate(skillsapp.ValidateOptions{Root: root})
		if err != nil {
			return nil, err
		}
		for _, failure := range report.Failures {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     filepath.ToSlash(failure.Path),
				Target:   "opencode",
				Message:  "OpenCode mirrored skill is incompatible with the shared SKILL.md contract: " + failure.Message,
			})
		}
	}
	diagnostics = append(diagnostics, validateOpenCodeCommandFiles(root, state.ComponentPaths("commands"))...)
	diagnostics = append(diagnostics, validateOpenCodeAgentFiles(root, state.ComponentPaths("agents"))...)
	diagnostics = append(diagnostics, validateOpenCodeThemeFiles(root, state.ComponentPaths("themes"))...)
	return diagnostics, nil
}

type opencodeScopeConfig struct {
	root              string
	displayConfigRoot string
	workspaceRoot     string
	workspaceDisplay  string
	compatSkillRoots  []opencodeImportSource
	allowCompatSkills bool
}

func importOpenCodeScope(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	importedConfig, configDisplayPath, warnings, ok, err := readImportedOpenCodeConfigFromDir(cfg.root, cfg.displayConfigRoot)
	if err != nil {
		return err
	}
	state.warnings = append(state.warnings, warnings...)
	if ok {
		commandArtifacts, remainingCommands, commandWarnings, err := importedOpenCodeInlineCommandArtifacts(importedConfig.Commands, configDisplayPath)
		if err != nil {
			return err
		}
		agentArtifacts, remainingAgents, agentWarnings, err := importedOpenCodeInlineAgentArtifacts(importedConfig.Agents, configDisplayPath)
		if err != nil {
			return err
		}
		state.warnings = append(state.warnings, commandWarnings...)
		state.warnings = append(state.warnings, agentWarnings...)
		state.addArtifacts(commandArtifacts...)
		state.addArtifacts(agentArtifacts...)
		if len(remainingCommands) > 0 {
			if importedConfig.Extra == nil {
				importedConfig.Extra = map[string]any{}
			}
			importedConfig.Extra["command"] = remainingCommands
		}
		if len(remainingAgents) > 0 {
			if importedConfig.Extra == nil {
				importedConfig.Extra = map[string]any{}
			}
			importedConfig.Extra["agent"] = remainingAgents
		}
		state.mergeConfig(importedConfig)
		state.hasInput = true
	}

	themeArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     filepath.Join(cfg.workspaceRoot, "themes"),
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "themes")),
		},
		filepath.Join("targets", "opencode", "themes"),
		func(rel string) bool { return filepath.Ext(rel) == ".json" },
	)
	if err != nil {
		return err
	}
	state.addArtifacts(themeArtifacts...)
	if len(themeArtifacts) > 0 {
		state.hasInput = true
	}

	commandArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     filepath.Join(cfg.workspaceRoot, "commands"),
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "commands")),
		},
		filepath.Join("targets", "opencode", "commands"),
		func(rel string) bool { return filepath.Ext(rel) == ".md" },
	)
	if err != nil {
		return err
	}
	state.addArtifacts(commandArtifacts...)
	if len(commandArtifacts) > 0 {
		state.hasInput = true
	}

	agentArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     filepath.Join(cfg.workspaceRoot, "agents"),
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "agents")),
		},
		filepath.Join("targets", "opencode", "agents"),
		func(rel string) bool { return filepath.Ext(rel) == ".md" },
	)
	if err != nil {
		return err
	}
	state.addArtifacts(agentArtifacts...)
	if len(agentArtifacts) > 0 {
		state.hasInput = true
	}

	var skillSources []opencodeImportSource
	if cfg.allowCompatSkills {
		skillSources = append(skillSources, cfg.compatSkillRoots...)
	}
	skillSources = append(skillSources, opencodeImportSource{
		dir:     filepath.Join(cfg.workspaceRoot, "skills"),
		display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "skills")),
	})
	skillArtifacts, skillWarnings, err := importDirectoryArtifactsWithWarnings(skillSources, "skills", func(rel string) bool {
		return strings.HasSuffix(rel, "SKILL.md")
	})
	if err != nil {
		return err
	}
	state.addArtifacts(skillArtifacts...)
	state.warnings = append(state.warnings, skillWarnings...)
	if len(skillArtifacts) > 0 {
		state.hasInput = true
	}

	pluginsDir := filepath.Join(cfg.workspaceRoot, "plugins")
	if fileExists(pluginsDir) {
		state.warnings = append(state.warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "plugins")),
			Message: fmt.Sprintf("ignored unsupported OpenCode local plugin code under %s; local JS/TS plugin code is part of the later OpenCode code-plugin wave", filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "plugins"))),
		})
	}
	packageJSON := filepath.Join(cfg.workspaceRoot, "package.json")
	if fileExists(packageJSON) {
		state.warnings = append(state.warnings, pluginmodel.Warning{
			Kind:    pluginmodel.WarningFidelity,
			Path:    filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "package.json")),
			Message: fmt.Sprintf("ignored unsupported OpenCode local plugin dependency metadata under %s", filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "package.json"))),
		})
	}

	return nil
}

func (s *opencodeImportedState) addArtifacts(artifacts ...pluginmodel.Artifact) {
	for _, artifact := range artifacts {
		s.artifacts[artifact.RelPath] = artifact
	}
}

func (s *opencodeImportedState) mergeConfig(config importedOpenCodeConfig) {
	if config.PluginsProvided {
		s.pluginsProvided = true
		s.plugins = append([]string(nil), config.Plugins...)
	}
	if config.MCPProvided {
		if s.mcp == nil {
			s.mcp = map[string]any{}
		}
		mergeOpenCodeObject(s.mcp, config.MCP)
	}
	if len(config.Extra) > 0 {
		mergeOpenCodeObject(s.extra, config.Extra)
	}
}

func readImportedOpenCodeConfigFromDir(root string, displayBase string) (importedOpenCodeConfig, string, []pluginmodel.Warning, bool, error) {
	path, warnings, ok, err := resolveOpenCodeConfigPathInDir(root, displayBase)
	if err != nil || !ok {
		return importedOpenCodeConfig{}, "", warnings, ok, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return importedOpenCodeConfig{}, "", warnings, false, err
	}
	data, err := decodeImportedOpenCodeConfig(body)
	if err != nil {
		return importedOpenCodeConfig{}, "", warnings, false, err
	}
	displayPath := filepath.Base(path)
	if strings.TrimSpace(displayBase) != "" {
		displayPath = filepath.ToSlash(filepath.Join(displayBase, filepath.Base(path)))
	}
	return data, displayPath, warnings, true, nil
}

func importedOpenCodeInlineCommandArtifacts(raw map[string]any, configPath string) ([]pluginmodel.Artifact, map[string]any, []pluginmodel.Warning, error) {
	return importedOpenCodeInlineMarkdownArtifacts("command", raw, configPath, "commands", normalizeInlineOpenCodeCommand)
}

func importedOpenCodeInlineAgentArtifacts(raw map[string]any, configPath string) ([]pluginmodel.Artifact, map[string]any, []pluginmodel.Warning, error) {
	return importedOpenCodeInlineMarkdownArtifacts("agent", raw, configPath, "agents", normalizeInlineOpenCodeAgent)
}

type openCodeInlineNormalizer func(name string, spec map[string]any) (map[string]any, string, bool)

func importedOpenCodeInlineMarkdownArtifacts(field string, raw map[string]any, configPath string, dstKind string, normalize openCodeInlineNormalizer) ([]pluginmodel.Artifact, map[string]any, []pluginmodel.Warning, error) {
	if len(raw) == 0 {
		return nil, nil, nil, nil
	}
	var (
		artifacts []pluginmodel.Artifact
		warnings  []pluginmodel.Warning
		remaining = map[string]any{}
	)
	for name, value := range raw {
		spec, ok := value.(map[string]any)
		if !ok {
			remaining[name] = value
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    configPath,
				Message: fmt.Sprintf("preserved OpenCode inline %s %q in targets/opencode/config.extra.json because it is not representable as targets/opencode/%s/*.md", field, name, dstKind),
			})
			continue
		}
		frontmatter, body, ok := normalize(name, spec)
		if !ok {
			remaining[name] = value
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    configPath,
				Message: fmt.Sprintf("preserved OpenCode inline %s %q in targets/opencode/config.extra.json because it is not representable as targets/opencode/%s/*.md", field, name, dstKind),
			})
			continue
		}
		relPath, ok := canonicalOpenCodeNamedMarkdownPath(dstKind, name)
		if !ok {
			remaining[name] = value
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    configPath,
				Message: fmt.Sprintf("preserved OpenCode inline %s %q in targets/opencode/config.extra.json because its name cannot be normalized into a canonical markdown file path", field, name),
			})
			continue
		}
		content, err := marshalOpenCodeMarkdown(frontmatter, body)
		if err != nil {
			return nil, nil, nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{RelPath: relPath, Content: content})
	}
	return compactArtifacts(artifacts), remaining, warnings, nil
}

func normalizeInlineOpenCodeCommand(name string, spec map[string]any) (map[string]any, string, bool) {
	template, ok := spec["template"].(string)
	if !ok || strings.TrimSpace(template) == "" {
		return nil, "", false
	}
	for key := range spec {
		switch key {
		case "template", "description", "agent", "subtask", "model":
		default:
			return nil, "", false
		}
	}
	frontmatter := map[string]any{}
	if description, ok := spec["description"]; ok {
		text, ok := description.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["description"] = strings.TrimSpace(text)
	}
	if agent, ok := spec["agent"]; ok {
		text, ok := agent.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["agent"] = strings.TrimSpace(text)
	}
	if subtask, ok := spec["subtask"]; ok {
		flag, ok := subtask.(bool)
		if !ok {
			return nil, "", false
		}
		frontmatter["subtask"] = flag
	}
	if model, ok := spec["model"]; ok {
		text, ok := model.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["model"] = strings.TrimSpace(text)
	}
	return frontmatter, strings.TrimSpace(template), true
}

func normalizeInlineOpenCodeAgent(name string, spec map[string]any) (map[string]any, string, bool) {
	description, ok := spec["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		return nil, "", false
	}
	for key := range spec {
		switch key {
		case "description", "mode", "model", "temperature", "tools", "permission", "disable", "steps", "prompt":
		default:
			return nil, "", false
		}
	}
	frontmatter := map[string]any{
		"description": strings.TrimSpace(description),
	}
	if mode, ok := spec["mode"]; ok {
		text, ok := mode.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["mode"] = strings.TrimSpace(text)
	}
	if model, ok := spec["model"]; ok {
		text, ok := model.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, "", false
		}
		frontmatter["model"] = strings.TrimSpace(text)
	}
	if temperature, ok := spec["temperature"]; ok {
		switch value := temperature.(type) {
		case float64:
			frontmatter["temperature"] = value
		default:
			return nil, "", false
		}
	}
	if tools, ok := spec["tools"]; ok {
		frontmatter["tools"] = tools
	}
	if permission, ok := spec["permission"]; ok {
		frontmatter["permission"] = permission
	}
	if disable, ok := spec["disable"]; ok {
		flag, ok := disable.(bool)
		if !ok {
			return nil, "", false
		}
		frontmatter["disable"] = flag
	}
	if steps, ok := spec["steps"]; ok {
		value, ok := steps.(float64)
		if !ok || value != float64(int(value)) {
			return nil, "", false
		}
		frontmatter["steps"] = int(value)
	}
	body := ""
	if prompt, ok := spec["prompt"]; ok {
		text, ok := prompt.(string)
		if !ok {
			return nil, "", false
		}
		if strings.Contains(text, "{file:") {
			return nil, "", false
		}
		body = strings.TrimSpace(text)
	}
	return frontmatter, body, true
}

func importDirectoryArtifacts(source opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, error) {
	artifacts, _, err := importDirectoryArtifactsWithWarnings([]opencodeImportSource{source}, dstRoot, keep)
	return artifacts, err
}

func importDirectoryArtifactsWithWarnings(sources []opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	artifacts := map[string]pluginmodel.Artifact{}
	var warnings []pluginmodel.Warning
	for _, source := range sources {
		full := source.dir
		if _, err := os.Stat(full); err != nil {
			continue
		}
		var used bool
		err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(full, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if keep != nil && !keep(rel) {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			artifacts[filepath.ToSlash(filepath.Join(dstRoot, rel))] = pluginmodel.Artifact{
				RelPath: filepath.ToSlash(filepath.Join(dstRoot, rel)),
				Content: body,
			}
			used = true
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		if source.warnOnUse && used {
			warnings = append(warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    source.warnPath,
				Message: source.warnMsg,
			})
		}
	}
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, rel := range sortedArtifactKeys(artifacts) {
		out = append(out, artifacts[rel])
	}
	return out, warnings, nil
}

func mergeOpenCodeObject(dst, src map[string]any) {
	if len(src) == 0 {
		return
	}
	for key, value := range src {
		existing, hasExisting := dst[key].(map[string]any)
		incoming, incomingIsMap := value.(map[string]any)
		if hasExisting && incomingIsMap {
			mergeOpenCodeObject(existing, incoming)
			dst[key] = existing
			continue
		}
		dst[key] = value
	}
}

func marshalOpenCodeMarkdown(frontmatter map[string]any, body string) ([]byte, error) {
	fm := strings.TrimSpace(string(mustYAML(frontmatter)))
	text := "---\n" + fm + "\n---\n"
	if strings.TrimSpace(body) != "" {
		text += "\n" + strings.TrimSpace(body) + "\n"
	}
	return []byte(text), nil
}

func canonicalOpenCodeNamedMarkdownPath(kind, name string) (string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) || strings.Contains(name, "..") {
		return "", false
	}
	return filepath.ToSlash(filepath.Join("targets", "opencode", kind, name+".md")), true
}

func sortedArtifactKeys(artifacts map[string]pluginmodel.Artifact) []string {
	out := make([]string, 0, len(artifacts))
	for rel := range artifacts {
		out = append(out, rel)
	}
	slices.Sort(out)
	return out
}

func validateOpenCodeCommandFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".md" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s must use the .md extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s is not readable: %v", rel, err),
			})
			continue
		}
		frontmatter, markdown, err := parseMarkdownFrontmatterDocument(body, "OpenCode command file "+rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  err.Error(),
			})
			continue
		}
		if strings.TrimSpace(markdown) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode command file %s must define a markdown command template body", rel),
			})
		}
		if description, ok := frontmatter["description"]; ok {
			text, ok := description.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode command file %s frontmatter field %q must be a non-empty string", rel, "description"),
				})
			}
		}
	}
	return diagnostics
}

func validateOpenCodeAgentFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".md" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s must use the .md extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s is not readable: %v", rel, err),
			})
			continue
		}
		frontmatter, _, err := parseMarkdownFrontmatterDocument(body, "OpenCode agent file "+rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  err.Error(),
			})
			continue
		}
		description, ok := frontmatter["description"].(string)
		if !ok || strings.TrimSpace(description) == "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode agent file %s must declare a non-empty frontmatter description", rel),
			})
		}
		if mode, ok := frontmatter["mode"]; ok {
			text, ok := mode.(string)
			if !ok || strings.TrimSpace(text) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     rel,
					Target:   "opencode",
					Message:  fmt.Sprintf("OpenCode agent file %s frontmatter field %q must be a non-empty string", rel, "mode"),
				})
			}
		}
	}
	return diagnostics
}

func validateOpenCodeThemeFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".json" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s must use the .json extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s is not readable: %v", rel, err),
			})
			continue
		}
		doc, err := decodeJSONObject(body, "OpenCode theme file "+rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  err.Error(),
			})
			continue
		}
		if _, ok := doc["theme"]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode theme file %s must define a top-level theme object", rel),
			})
		}
	}
	return diagnostics
}

func renderPortableSkills(root string, paths []string, outputRoot string) ([]pluginmodel.Artifact, error) {
	var artifacts []pluginmodel.Artifact
	for _, rel := range paths {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return nil, err
		}
		child, err := filepath.Rel(filepath.FromSlash("skills"), filepath.FromSlash(rel))
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(outputRoot, child)),
			Content: body,
		})
	}
	return compactArtifacts(artifacts), nil
}
