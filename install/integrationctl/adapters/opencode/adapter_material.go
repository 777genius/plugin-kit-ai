package opencode

import (
	"context"
	"path/filepath"

	"github.com/777genius/plugin-kit-ai/install/integrationctl/adapters/portablemcp"
	"github.com/777genius/plugin-kit-ai/install/integrationctl/domain"
)

func (a Adapter) loadSourceMaterial(ctx context.Context, sourceRoot, scope string, workspaceRoot string) (sourceMaterial, error) {
	material := sourceMaterial{
		WholeFields: map[string]any{
			"$schema": "https://opencode.ai/config.json",
		},
	}
	plugins, err := readPlugins(filepath.Join(sourceRoot, "src", "targets", "opencode", "package.yaml"))
	if err != nil {
		return sourceMaterial{}, err
	}
	material.Plugins = plugins

	loader := portablemcp.Loader{FS: a.fs()}
	if loaded, err := loader.LoadForTarget(ctx, sourceRoot, domain.TargetOpenCode); err == nil {
		projected := renderOpenCodeMCP(loaded, sourceRoot)
		if len(projected) > 0 {
			material.MCP = projected
		}
	} else if !isMissingPortableMCP(err) {
		return sourceMaterial{}, err
	}

	return a.completeSourceMaterial(sourceRoot, scope, workspaceRoot, material)
}

func (a Adapter) completeSourceMaterial(sourceRoot, scope string, workspaceRoot string, material sourceMaterial) (sourceMaterial, error) {
	if err := material.loadFirstClassDocs(sourceRoot); err != nil {
		return sourceMaterial{}, err
	}
	extra, err := readConfigExtra(filepath.Join(sourceRoot, "src", "targets", "opencode", "config.extra.json"))
	if err != nil {
		return sourceMaterial{}, err
	}
	if err := material.mergeExtra(extra); err != nil {
		return sourceMaterial{}, err
	}
	copyFiles, err := collectCopyFiles(sourceRoot, a.assetsRoot(scope, workspaceRoot))
	if err != nil {
		return sourceMaterial{}, err
	}
	material.CopyFiles = copyFiles
	return material, nil
}

func (m sourceMaterial) mutationForUpdate(target domain.TargetInstallation) configMutation {
	currentKeys := ownedConfigKeys(target)
	currentPlugins := ownedPluginRefs(target)
	currentMCP := ownedMCPAliases(target)
	return configMutation{
		WholeSet:      m.WholeFields,
		WholeRemove:   subtractStrings(currentKeys, sortedManagedKeys(m.WholeFields)),
		PluginsSet:    m.Plugins,
		PluginsRemove: subtractStrings(currentPlugins, pluginRefNames(m.Plugins)),
		MCPSet:        m.MCP,
		MCPRemove:     subtractStrings(currentMCP, sortedMapKeys(m.MCP)),
	}
}
