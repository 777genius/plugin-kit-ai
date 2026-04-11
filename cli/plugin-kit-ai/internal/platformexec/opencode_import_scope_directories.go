package platformexec

import (
	"os"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func importOpenCodeThemeArtifacts(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	return importOpenCodeWorkspaceDirectory(state, openCodeThemeDirectoryImport(cfg))
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
	return importOpenCodeWorkspaceDirectory(state, openCodeCommandDirectoryImport(cfg))
}

func importOpenCodeAgentDirectory(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	return importOpenCodeWorkspaceDirectory(state, openCodeAgentDirectoryImport(cfg))
}

func importOpenCodeSkillDirectory(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	return importOpenCodeWorkspaceDirectory(state, openCodeSkillDirectoryImport(cfg))
}

func importOpenCodePluginDirectory(state *opencodeImportedState, cfg opencodeScopeConfig) error {
	return importOpenCodeWorkspaceDirectory(state, openCodePluginDirectoryImport(cfg))
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
