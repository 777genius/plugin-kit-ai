package platformexec

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

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
