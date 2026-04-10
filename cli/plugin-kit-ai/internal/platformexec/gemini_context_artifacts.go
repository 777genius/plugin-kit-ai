package platformexec

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func geminiContextArtifacts(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) (string, pluginmodel.Artifact, []pluginmodel.Artifact, bool, error) {
	selected, ok, err := selectGeminiPrimaryContext(graph, state, meta)
	if err != nil {
		return "", pluginmodel.Artifact{}, nil, false, err
	}
	if !ok {
		return "", pluginmodel.Artifact{}, nil, false, nil
	}
	body, err := os.ReadFile(filepath.Join(root, selected.SourcePath))
	if err != nil {
		return "", pluginmodel.Artifact{}, nil, false, err
	}
	primary := pluginmodel.Artifact{RelPath: selected.ArtifactName, Content: body}
	var extra []pluginmodel.Artifact
	for _, rel := range state.ComponentPaths("contexts") {
		if rel == selected.SourcePath {
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			return "", pluginmodel.Artifact{}, nil, false, err
		}
		extra = append(extra, pluginmodel.Artifact{
			RelPath: geminiExtraContextArtifactPath(rel),
			Content: body,
		})
	}
	slices.SortFunc(extra, func(a, b pluginmodel.Artifact) int { return strings.Compare(a.RelPath, b.RelPath) })
	return selected.ArtifactName, primary, extra, true, nil
}
