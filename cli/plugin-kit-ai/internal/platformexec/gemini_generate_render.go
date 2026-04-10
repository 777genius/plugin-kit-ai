package platformexec

import (
	"fmt"
	"path/filepath"
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
	skillArtifacts, err := renderPortableSkills(root, graph.Portable.Paths("skills"), "skills")
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, skillArtifacts...)
	hookArtifacts, err := renderGeminiHookArtifacts(root, graph, state, entrypoint)
	if err != nil {
		return nil, err
	}
	artifacts = append(artifacts, hookArtifacts...)
	copied, err := copyArtifactDirs(root,
		artifactDir{src: authoredComponentDir(state, "commands", filepath.Join("targets", "gemini", "commands")), dst: "commands"},
		artifactDir{src: authoredComponentDir(state, "policies", filepath.Join("targets", "gemini", "policies")), dst: "policies"},
	)
	if err != nil {
		return nil, err
	}
	return append(artifacts, copied...), nil
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

func renderGeminiManifest(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) (map[string]any, []pluginmodel.Artifact, error) {
	manifest := map[string]any{
		"name":        graph.Manifest.Name,
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	if err := mergeGeminiManifestPortableMCP(manifest, graph); err != nil {
		return nil, nil, err
	}
	manifest, err := mergeGeminiManifestMeta(manifest, meta)
	if err != nil {
		return nil, nil, err
	}
	manifest, err = mergeGeminiManifestAssets(root, state, manifest)
	if err != nil {
		return nil, nil, err
	}
	contextArtifacts, err := mergeGeminiManifestContexts(root, graph, state, meta, manifest)
	if err != nil {
		return nil, nil, err
	}
	if extra, err := loadNativeExtraDoc(root, state, "manifest_extra", pluginmodel.NativeDocFormatJSON); err != nil {
		return nil, nil, err
	} else if err := pluginmodel.MergeNativeExtraObject(manifest, extra, "gemini manifest.extra.json", geminiManifestManagedPaths()); err != nil {
		return nil, nil, err
	}
	return manifest, contextArtifacts, nil
}

func mergeGeminiManifestPortableMCP(manifest map[string]any, graph pluginmodel.PackageGraph) error {
	if graph.Portable.MCP == nil {
		return nil
	}
	projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
	if err != nil {
		return err
	}
	manifest["mcpServers"] = projected
	return nil
}

func mergeGeminiManifestMeta(manifest map[string]any, meta geminiPackageMeta) (map[string]any, error) {
	if len(meta.ExcludeTools) > 0 {
		manifest["excludeTools"] = append([]string(nil), normalizeGeminiExcludeTools(meta.ExcludeTools)...)
	}
	if strings.TrimSpace(meta.MigratedTo) != "" {
		manifest["migratedTo"] = meta.MigratedTo
	}
	if strings.TrimSpace(meta.PlanDirectory) != "" {
		manifest["plan"] = map[string]any{"directory": meta.PlanDirectory}
	}
	return manifest, nil
}

func mergeGeminiManifestAssets(root string, state pluginmodel.TargetState, manifest map[string]any) (map[string]any, error) {
	settings, err := loadGeminiSettings(root, state.ComponentPaths("settings"))
	if err != nil {
		return nil, err
	}
	if len(settings) > 0 {
		manifest["settings"] = settings
	}
	themes, err := loadGeminiThemes(root, state.ComponentPaths("themes"))
	if err != nil {
		return nil, err
	}
	if len(themes) > 0 {
		manifest["themes"] = themes
	}
	return manifest, nil
}

func mergeGeminiManifestContexts(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta, manifest map[string]any) ([]pluginmodel.Artifact, error) {
	if contextName, contextArtifact, extraContexts, ok, err := geminiContextArtifacts(root, graph, state, meta); err != nil {
		return nil, err
	} else if ok {
		manifest["contextFileName"] = contextName
		return append([]pluginmodel.Artifact{contextArtifact}, extraContexts...), nil
	}
	return nil, nil
}

func renderGeminiHookArtifacts(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, entrypoint string) ([]pluginmodel.Artifact, error) {
	if hookPaths := state.ComponentPaths("hooks"); len(hookPaths) > 0 {
		return copyArtifacts(root, authoredComponentDir(state, "hooks", filepath.Join("targets", "gemini", "hooks")), "hooks")
	}
	if geminiUsesGeneratedHooks(graph, state) {
		return []pluginmodel.Artifact{{
			RelPath: filepath.Join("hooks", "hooks.json"),
			Content: defaultGeminiHooks(entrypoint),
		}}, nil
	}
	return nil, nil
}
