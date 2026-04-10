package platformexec

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (geminiAdapter) ManagedPaths(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]string, error) {
	meta, err := loadGeminiRenderMeta(root, state)
	if err != nil {
		return nil, err
	}
	seen := initialGeminiManagedPathSet(graph, state)
	selected, ok, err := selectGeminiPrimaryContext(graph, state, meta)
	if err != nil || !ok {
		return sortedKeys(seen), err
	}
	addGeminiManagedContextPaths(seen, state, selected)
	return sortedGeminiManagedPaths(seen), nil
}

func initialGeminiManagedPathSet(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) map[string]struct{} {
	seen := map[string]struct{}{}
	if geminiUsesGeneratedHooks(graph, state) {
		seen[filepath.ToSlash(filepath.Join("hooks", "hooks.json"))] = struct{}{}
	}
	return seen
}

func addGeminiManagedContextPaths(seen map[string]struct{}, state pluginmodel.TargetState, selected geminiContextSelection) {
	seen[selected.ArtifactName] = struct{}{}
	for _, rel := range state.ComponentPaths("contexts") {
		if rel == selected.SourcePath {
			continue
		}
		seen[geminiExtraContextArtifactPath(rel)] = struct{}{}
	}
}

func sortedGeminiManagedPaths(seen map[string]struct{}) []string {
	var out []string
	for path := range seen {
		out = append(out, path)
	}
	slices.Sort(out)
	return out
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
