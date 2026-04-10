package platformexec

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (geminiAdapter) Generate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	entrypoint, err := geminiRenderEntrypoint(graph)
	if err != nil {
		return nil, err
	}
	meta, _, err := readYAMLDoc[geminiPackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	if err := validateGeminiRenderReady(root, graph, state, meta); err != nil {
		return nil, err
	}
	manifest, artifacts, err := renderGeminiManifest(root, graph, state, meta)
	if err != nil {
		return nil, err
	}
	manifestJSON, err := marshalJSON(manifest)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, pluginmodel.Artifact{RelPath: "gemini-extension.json", Content: manifestJSON})
	return appendGeminiGeneratedArtifacts(root, graph, state, entrypoint, artifacts)
}

func geminiRenderEntrypoint(graph pluginmodel.PackageGraph) (string, error) {
	if graph.Launcher == nil {
		return "", nil
	}
	entrypoint := strings.TrimSpace(graph.Launcher.Entrypoint)
	if entrypoint == "" {
		return "", fmt.Errorf("invalid %s: entrypoint required", pluginmodel.LauncherFileName)
	}
	return entrypoint, nil
}
