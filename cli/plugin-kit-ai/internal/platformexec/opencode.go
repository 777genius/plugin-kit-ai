package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	skillfs "github.com/777genius/plugin-kit-ai/cli/internal/skills/adapters/filesystem"
	skillsapp "github.com/777genius/plugin-kit-ai/cli/internal/skills/app"
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
	state.AddComponent("local_plugin_code", discoverFiles(root, filepath.Join("targets", "opencode", "plugins"), nil)...)
	return nil
}

func (opencodeAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	state := opencodeImportedState{
		artifacts: map[string]pluginmodel.Artifact{},
	}

	if seed.IncludeUserScope {
		home, err := os.UserHomeDir()
		if err != nil {
			return ImportResult{}, fmt.Errorf("resolve user home for OpenCode import: %w", err)
		}
		for _, reject := range []struct {
			full    string
			display string
		}{
			{full: filepath.Join(home, ".agents", "skills"), display: filepath.ToSlash(filepath.Join("~", ".agents", "skills"))},
			{full: filepath.Join(home, ".claude", "skills"), display: filepath.ToSlash(filepath.Join("~", ".claude", "skills"))},
		} {
			if err := rejectOpenCodeCompatSkillRoot(reject.full, reject.display); err != nil {
				return ImportResult{}, err
			}
		}
		globalRoot := filepath.Join(home, ".config", "opencode")
		if err := importOpenCodeScope(&state, opencodeScopeConfig{
			root:              globalRoot,
			displayConfigRoot: filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
			workspaceRoot:     globalRoot,
			workspaceDisplay:  filepath.ToSlash(filepath.Join("~", ".config", "opencode")),
		}); err != nil {
			return ImportResult{}, err
		}
	}

	for _, reject := range []struct {
		full    string
		display string
	}{
		{full: filepath.Join(root, ".agents", "skills"), display: filepath.ToSlash(filepath.Join(".agents", "skills"))},
		{full: filepath.Join(root, ".claude", "skills"), display: filepath.ToSlash(filepath.Join(".claude", "skills"))},
	} {
		if err := rejectOpenCodeCompatSkillRoot(reject.full, reject.display); err != nil {
			return ImportResult{}, err
		}
	}
	if err := importOpenCodeScope(&state, opencodeScopeConfig{
		root:              root,
		displayConfigRoot: "",
		workspaceRoot:     filepath.Join(root, ".opencode"),
		workspaceDisplay:  ".opencode",
	}); err != nil {
		return ImportResult{}, err
	}

	if seed.Explicit {
		if envDir, ok, err := resolveOpenCodeEnvConfigDir(); err != nil {
			return ImportResult{}, err
		} else if ok {
			if err := importOpenCodeScope(&state, opencodeScopeConfig{
				root:              envDir,
				displayConfigRoot: filepath.ToSlash(filepath.Join("$OPENCODE_CONFIG_DIR")),
				workspaceRoot:     envDir,
				workspaceDisplay:  filepath.ToSlash(filepath.Join("$OPENCODE_CONFIG_DIR")),
			}); err != nil {
				return ImportResult{}, err
			}
		}
		if envFile, ok, err := resolveOpenCodeEnvConfigFile(); err != nil {
			return ImportResult{}, err
		} else if ok {
			importedConfig, configPath, _, _, err := readImportedOpenCodeConfigFromFile(envFile, filepath.ToSlash("$OPENCODE_CONFIG"))
			if err != nil {
				return ImportResult{}, err
			}
			commandArtifacts, remainingCommands, commandWarnings, err := importedOpenCodeInlineCommandArtifacts(importedConfig.Commands, configPath)
			if err != nil {
				return ImportResult{}, err
			}
			agentArtifacts, remainingAgents, agentWarnings, err := importedOpenCodeInlineAgentArtifacts(importedConfig.Agents, configPath)
			if err != nil {
				return ImportResult{}, err
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
	}
	if !state.hasInput {
		return ImportResult{}, fmt.Errorf("OpenCode import requires opencode.json, opencode.jsonc, supported workspace directories, OPENCODE_CONFIG, OPENCODE_CONFIG_DIR, or --include-user-scope with OpenCode native sources")
	}

	artifacts := []pluginmodel.Artifact{{
		RelPath: filepath.Join("targets", "opencode", "package.yaml"),
		Content: mustYAML(opencodePackageMeta{Plugins: append([]string(nil), state.plugins...)}),
	}}
	if len(state.mcp) > 0 {
		artifact, err := importedPortableMCPArtifact("opencode", state.mcp)
		if err != nil {
			return ImportResult{}, err
		}
		artifacts = append(artifacts, artifact)
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
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "opencode")
		if err != nil {
			return nil, err
		}
		doc["mcp"] = projected
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
		artifactDir{src: filepath.Join("targets", "opencode", "tools"), dst: filepath.Join(".opencode", "tools")},
		artifactDir{src: filepath.Join("targets", "opencode", "plugins"), dst: filepath.Join(".opencode", "plugins")},
	)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, copied...)
	packageArtifacts, err := copySingleArtifactIfExists(root, filepath.Join("targets", "opencode", "package.json"), filepath.Join(".opencode", "package.json"))
	if err != nil {
		return nil, err
	}
	return append(artifacts, packageArtifacts...), nil
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
		if envFile, envOK, envErr := resolveOpenCodeEnvConfigFile(); envErr != nil {
			return nil, envErr
		} else if envOK {
			configPath = filepath.ToSlash(envFile)
			ok = true
		} else if envDir, dirOK, dirErr := resolveOpenCodeEnvConfigDir(); dirErr != nil {
			return nil, dirErr
		} else if dirOK {
			configPath, warnings, ok, err = resolveOpenCodeConfigPathInDir(envDir, filepath.ToSlash("$OPENCODE_CONFIG_DIR"))
			if err != nil {
				return nil, err
			}
			if ok {
				configPath = filepath.ToSlash(configPath)
			}
		}
	}
	if !ok {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "opencode.json",
			Target:   "opencode",
			Message:  "OpenCode config opencode.json, opencode.jsonc, OPENCODE_CONFIG, or OPENCODE_CONFIG_DIR is required",
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
	configReadPath := filepath.Join(root, configPath)
	if filepath.IsAbs(configPath) || strings.HasPrefix(configPath, "$OPENCODE_CONFIG_DIR/") {
		configReadPath = configPath
		if strings.HasPrefix(configPath, "$OPENCODE_CONFIG_DIR/") {
			if envDir, ok, err := resolveOpenCodeEnvConfigDir(); err != nil {
				return nil, err
			} else if ok {
				configReadPath = filepath.Join(envDir, filepath.Base(configPath))
			}
		}
	}
	body, err := os.ReadFile(configReadPath)
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
	packageDoc, packageDiagnostics := validateOpenCodePluginPackageJSON(root, state.DocPath("local_plugin_dependencies"))
	diagnostics = append(diagnostics, packageDiagnostics...)
	diagnostics = append(diagnostics, validateOpenCodeToolFiles(root, state.ComponentPaths("tools"), packageDoc)...)
	diagnostics = append(diagnostics, validateOpenCodePluginFiles(root, state.ComponentPaths("local_plugin_code"), packageDoc)...)
	return diagnostics, nil
}

type opencodeScopeConfig struct {
	root              string
	displayConfigRoot string
	workspaceRoot     string
	workspaceDisplay  string
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

	toolArtifacts, toolWarnings, err := importOpenCodeToolArtifacts(cfg.workspaceRoot, cfg.workspaceDisplay)
	if err != nil {
		return err
	}
	state.addArtifacts(toolArtifacts...)
	state.warnings = append(state.warnings, toolWarnings...)
	if len(toolArtifacts) > 0 {
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

	skillArtifacts, _, err := importDirectoryArtifactsWithWarnings([]opencodeImportSource{{
		dir:     filepath.Join(cfg.workspaceRoot, "skills"),
		display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "skills")),
	}}, "skills", func(rel string) bool {
		return strings.HasSuffix(rel, "SKILL.md")
	})
	if err != nil {
		return err
	}
	state.addArtifacts(skillArtifacts...)
	if len(skillArtifacts) > 0 {
		state.hasInput = true
	}

	pluginsDir := filepath.Join(cfg.workspaceRoot, "plugins")
	pluginArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     pluginsDir,
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "plugins")),
		},
		filepath.Join("targets", "opencode", "plugins"),
		nil,
	)
	if err != nil {
		return err
	}
	state.addArtifacts(pluginArtifacts...)
	if len(pluginArtifacts) > 0 {
		state.hasInput = true
	}

	packageJSON := filepath.Join(cfg.workspaceRoot, "package.json")
	if body, err := os.ReadFile(packageJSON); err == nil {
		state.addArtifacts(pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join("targets", "opencode", "package.json")),
			Content: body,
		})
		state.hasInput = true
	} else if !os.IsNotExist(err) {
		return err
	}

	return nil
}

func rejectOpenCodeCompatSkillRoot(full, display string) error {
	if _, err := os.Stat(full); err == nil {
		return fmt.Errorf("unsupported OpenCode native skill path %s: use skills/**", display)
	} else if err != nil && !os.IsNotExist(err) {
		return err
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
		if s.extra == nil {
			s.extra = map[string]any{}
		}
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

func importOpenCodeToolArtifacts(workspaceRoot, workspaceDisplay string) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	legacyDir := filepath.Join(workspaceRoot, "tool")
	if _, err := os.Stat(legacyDir); err == nil {
		return nil, nil, fmt.Errorf("unsupported OpenCode native path %s: use %s", filepath.ToSlash(filepath.Join(workspaceDisplay, "tool")), filepath.ToSlash(filepath.Join(workspaceDisplay, "tools")))
	} else if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	sources := []opencodeImportSource{
		{
			dir:     filepath.Join(workspaceRoot, "tools"),
			display: filepath.ToSlash(filepath.Join(workspaceDisplay, "tools")),
		},
	}
	return importDirectoryArtifactsRejectingSymlinks(sources, filepath.Join("targets", "opencode", "tools"), nil)
}

func importDirectoryArtifactsRejectingSymlinks(sources []opencodeImportSource, dstRoot string, keep func(string) bool) ([]pluginmodel.Artifact, []pluginmodel.Warning, error) {
	artifacts := map[string]pluginmodel.Artifact{}
	for _, source := range sources {
		full := source.dir
		if _, err := os.Stat(full); err != nil {
			continue
		}
		err := filepath.WalkDir(full, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("OpenCode native import does not support symlinks under %s", source.display)
			}
			if d.IsDir() {
				return nil
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
			dst := filepath.ToSlash(filepath.Join(dstRoot, rel))
			artifacts[dst] = pluginmodel.Artifact{RelPath: dst, Content: body}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	out := make([]pluginmodel.Artifact, 0, len(artifacts))
	for _, rel := range sortedArtifactKeys(artifacts) {
		out = append(out, artifacts[rel])
	}
	return out, nil, nil
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

func validateOpenCodeToolFiles(root string, rels []string, packageDoc map[string]any) []Diagnostic {
	if len(rels) == 0 {
		return nil
	}
	var (
		diagnostics      []Diagnostic
		hasDefinition    bool
		usesPluginHelper bool
		seenCaseFolded   = map[string]string{}
	)
	for _, rel := range rels {
		clean := filepath.ToSlash(filepath.Clean(rel))
		if clean != rel || strings.Contains(clean, "..") {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s must stay within targets/opencode/tools without path traversal", rel),
			})
			continue
		}
		lower := strings.ToLower(clean)
		if prior, ok := seenCaseFolded[lower]; ok && prior != clean {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool files %s and %s collide on case-insensitive filesystems", prior, rel),
			})
		} else {
			seenCaseFolded[lower] = clean
		}
		fullPath := filepath.Join(root, rel)
		info, err := os.Lstat(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s must not be a symlink", rel),
			})
			continue
		}
		if info.IsDir() {
			continue
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode tool file %s is not readable: %v", rel, err),
			})
			continue
		}
		if isOpenCodePluginEntryFile(rel) {
			hasDefinition = true
		}
		if strings.Contains(string(body), `@opencode-ai/plugin`) {
			usesPluginHelper = true
		}
	}
	if !hasDefinition {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("targets", "opencode", "tools")),
			Target:   "opencode",
			Message:  "OpenCode standalone tools require at least one JS/TS tool definition file under targets/opencode/tools",
		})
	}
	if usesPluginHelper && !openCodePackageDeclaresDependency(packageDoc, "@opencode-ai/plugin") {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("targets", "opencode", "package.json")),
			Target:   "opencode",
			Message:  `OpenCode standalone tool files that import "@opencode-ai/plugin" must declare that dependency in targets/opencode/package.json`,
		})
	}
	return diagnostics
}

func validateOpenCodePluginPackageJSON(root string, rel string) (map[string]any, []Diagnostic) {
	if strings.TrimSpace(rel) == "" {
		return nil, nil
	}
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return nil, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  fmt.Sprintf("OpenCode plugin dependency metadata %s is not readable: %v", rel, err),
		}}
	}
	doc, err := decodeJSONObject(body, "OpenCode plugin dependency metadata "+rel)
	if err != nil {
		return nil, []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     rel,
			Target:   "opencode",
			Message:  err.Error(),
		}}
	}
	return doc, nil
}

func validateOpenCodePluginFiles(root string, rels []string, packageDoc map[string]any) []Diagnostic {
	if len(rels) == 0 {
		return nil
	}
	var (
		diagnostics      []Diagnostic
		hasEntry         bool
		usesPluginHelper bool
	)
	for _, rel := range rels {
		fullPath := filepath.Join(root, rel)
		info, err := os.Stat(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode local plugin file %s is not readable: %v", rel, err),
			})
			continue
		}
		if info.IsDir() {
			continue
		}
		if isOpenCodePluginEntryFile(rel) {
			hasEntry = true
		}
		body, err := os.ReadFile(fullPath)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  fmt.Sprintf("OpenCode local plugin file %s is not readable: %v", rel, err),
			})
			continue
		}
		src := string(body)
		if strings.Contains(src, `export default`) && strings.Contains(src, `setup(`) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "opencode",
				Message:  "OpenCode local plugin file uses the old scaffold shape `export default { setup() { ... } }`; use official named async plugin exports instead",
			})
		}
		if strings.Contains(src, `@opencode-ai/plugin`) {
			usesPluginHelper = true
		}
	}
	if !hasEntry {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("targets", "opencode", "plugins")),
			Target:   "opencode",
			Message:  "OpenCode local plugin code requires at least one JS/TS plugin entry file under targets/opencode/plugins (for example .js, .mjs, .cjs, .ts, .mts, or .cts)",
		})
	}
	if usesPluginHelper && !openCodePackageDeclaresDependency(packageDoc, "@opencode-ai/plugin") {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     filepath.ToSlash(filepath.Join("targets", "opencode", "package.json")),
			Target:   "opencode",
			Message:  `OpenCode plugin files that import "@opencode-ai/plugin" must declare that dependency in targets/opencode/package.json`,
		})
	}
	return diagnostics
}

func openCodePackageDeclaresDependency(doc map[string]any, name string) bool {
	if len(doc) == 0 {
		return false
	}
	for _, field := range []string{"dependencies", "devDependencies", "peerDependencies"} {
		raw, ok := doc[field]
		if !ok {
			continue
		}
		deps, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if value, ok := deps[name]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return true
			}
		}
	}
	return false
}

func isOpenCodePluginEntryFile(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".js", ".mjs", ".cjs", ".ts", ".mts", ".cts":
		return true
	default:
		return false
	}
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
