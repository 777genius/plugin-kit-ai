package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

type opencodeImportedState struct {
	plugins         []opencodePluginRef
	pluginsProvided bool
	mcp             map[string]any
	defaultAgent    string
	defaultAgentSet bool
	instructions    []string
	instructionsSet bool
	permission      any
	permissionSet   bool
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

type opencodeScopeConfig struct {
	root              string
	displayConfigRoot string
	workspaceRoot     string
	workspaceDisplay  string
}

func importOpenCodePackage(root string, seed ImportSeed) (ImportResult, error) {
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
	if !state.hasInput {
		return ImportResult{}, fmt.Errorf("OpenCode import requires opencode.json, opencode.jsonc, supported workspace directories, or --include-user-scope with OpenCode native sources")
	}

	artifacts := []pluginmodel.Artifact{{
		RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "package.yaml"),
		Content: mustYAML(opencodePackageMeta{Plugins: append([]opencodePluginRef(nil), state.plugins...)}),
	}}
	if len(state.mcp) > 0 {
		artifact, err := importedPortableMCPArtifact("opencode", state.mcp)
		if err != nil {
			return ImportResult{}, err
		}
		artifacts = append(artifacts, artifact)
	}
	if state.defaultAgentSet {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "default_agent.txt"),
			Content: append([]byte(state.defaultAgent), '\n'),
		})
	}
	if state.instructionsSet {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "instructions.yaml"),
			Content: mustYAML(state.instructions),
		})
	}
	if state.permissionSet {
		body, err := marshalJSON(state.permission)
		if err != nil {
			return ImportResult{}, err
		}
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "permission.json"),
			Content: body,
		})
	}
	if len(state.extra) > 0 {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "config.extra.json"),
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

	pluginArtifacts, err := importDirectoryArtifacts(
		opencodeImportSource{
			dir:     filepath.Join(cfg.workspaceRoot, "plugins"),
			display: filepath.ToSlash(filepath.Join(cfg.workspaceDisplay, "plugins")),
		},
		filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "plugins"),
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
			RelPath: filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "package.json")),
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
		s.plugins = append([]opencodePluginRef(nil), config.Plugins...)
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
	if config.DefaultAgentSet {
		s.defaultAgent = config.DefaultAgent
		s.defaultAgentSet = true
	}
	if config.InstructionsSet {
		s.instructions = append([]string(nil), config.Instructions...)
		s.instructionsSet = true
	}
	if config.PermissionSet {
		s.permission = config.Permission
		s.permissionSet = true
	}
}
