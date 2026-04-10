package platformexec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (geminiAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	meta, _, err := readYAMLDoc[geminiPackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	seen := map[string]struct{}{}
	if geminiUsesGeneratedHooks(graph, state) {
		seen[filepath.ToSlash(filepath.Join("hooks", "hooks.json"))] = struct{}{}
	}
	selected, ok, err := selectGeminiPrimaryContext(graph, state, meta)
	if err != nil || !ok {
		return sortedKeys(seen), err
	}
	seen[selected.ArtifactName] = struct{}{}
	for _, rel := range state.ComponentPaths("contexts") {
		if rel == selected.SourcePath {
			continue
		}
		seen[geminiExtraContextArtifactPath(rel)] = struct{}{}
	}
	var out []string
	for path := range seen {
		out = append(out, path)
	}
	slices.Sort(out)
	return out, nil
}

func geminiUsesGeneratedHooks(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) bool {
	if graph.Launcher == nil || strings.TrimSpace(graph.Launcher.Entrypoint) == "" {
		return false
	}
	if len(state.ComponentPaths("hooks")) > 0 {
		return false
	}
	return slices.Equal(graph.Manifest.Targets, []string{"gemini"})
}

func geminiManifestManagedPaths() []string {
	return []string{
		"name",
		"version",
		"description",
		"mcpServers",
		"contextFileName",
		"excludeTools",
		"settings",
		"themes",
		"plan.directory",
	}
}
