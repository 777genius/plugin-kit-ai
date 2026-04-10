package platformexec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func importOpenCodeScope(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	importedConfig, configDisplayPath, warnings, ok, err := readImportedOpenCodeConfigFromDir(cfg.root, cfg.displayConfigRoot)
	if err != nil {
		return err
	}
	state.warnings = append(state.warnings, warnings...)
	if ok {
		if err := importOpenCodeConfigArtifacts(state, importedConfig, configDisplayPath); err != nil {
			return err
		}
		state.hasInput = true
	}
	if err := importOpenCodeWorkspaceArtifacts(state, cfg); err != nil {
		return err
	}
	return nil
}

func importOpenCodeConfigArtifacts(state *opencodeImportedState, importedConfig importedOpenCodeConfig, configDisplayPath string) error {
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
	return nil
}

func importOpenCodeWorkspaceArtifacts(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	if err := importOpenCodeThemeArtifacts(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeToolArtifactsIntoState(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeCommandDirectory(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeAgentDirectory(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodeSkillDirectory(state, cfg); err != nil {
		return err
	}
	if err := importOpenCodePluginDirectory(state, cfg); err != nil {
		return err
	}
	return importOpenCodePackageJSON(state, cfg)
}

func importOpenCodeThemeArtifacts(state *opencodeImportedState, cfg opencodeScopeConfig) error {
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
	return nil
}

func importOpenCodeToolArtifactsIntoState(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	toolArtifacts, toolWarnings, err := importOpenCodeToolArtifacts(cfg.workspaceRoot, cfg.workspaceDisplay)
	if err != nil {
		return err
	}
	state.addArtifacts(toolArtifacts...)
	state.warnings = append(state.warnings, toolWarnings...)
	if len(toolArtifacts) > 0 {
		state.hasInput = true
	}
	return nil
}

func importOpenCodeCommandDirectory(state *opencodeImportedState, cfg opencodeScopeConfig) error {
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
	return nil
}

func importOpenCodeAgentDirectory(state *opencodeImportedState, cfg opencodeScopeConfig) error {
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
	return nil
}

func importOpenCodeSkillDirectory(state *opencodeImportedState, cfg opencodeScopeConfig) error {
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
	return nil
}

func importOpenCodePluginDirectory(state *opencodeImportedState, cfg opencodeScopeConfig) error {
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
	return nil
}

func importOpenCodePackageJSON(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	packageJSON := filepath.Join(cfg.workspaceRoot, "package.json")
	if body, err := os.ReadFile(packageJSON); err == nil {
		state.addArtifacts(pluginmodel.Artifact{
			RelPath: filepath.ToSlash(filepath.Join(pluginmodel.SourceDirName, "targets", "opencode", "package.json")),
			Content: body,
		})
		state.hasInput = true
		return nil
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
