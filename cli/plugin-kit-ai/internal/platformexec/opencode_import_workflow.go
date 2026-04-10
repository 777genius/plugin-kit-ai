package platformexec

import (
	"fmt"
	"os"
	"path/filepath"

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
