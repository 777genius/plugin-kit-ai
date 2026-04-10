package platformexec

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func (geminiAdapter) Generate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
	entrypoint := ""
	if graph.Launcher != nil {
		entrypoint = strings.TrimSpace(graph.Launcher.Entrypoint)
		if entrypoint == "" {
			return nil, fmt.Errorf("invalid %s: entrypoint required", pluginmodel.LauncherFileName)
		}
	}
	meta, _, err := readYAMLDoc[geminiPackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	if err := validateGeminiRenderReady(root, graph, state, meta); err != nil {
		return nil, err
	}
	manifest := map[string]any{
		"name":        graph.Manifest.Name,
		"version":     graph.Manifest.Version,
		"description": graph.Manifest.Description,
	}
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return nil, err
		}
		manifest["mcpServers"] = projected
	}
	var artifacts []pluginmodel.Artifact
	if len(meta.ExcludeTools) > 0 {
		manifest["excludeTools"] = append([]string(nil), normalizeGeminiExcludeTools(meta.ExcludeTools)...)
	}
	if strings.TrimSpace(meta.MigratedTo) != "" {
		manifest["migratedTo"] = meta.MigratedTo
	}
	if strings.TrimSpace(meta.PlanDirectory) != "" {
		manifest["plan"] = map[string]any{"directory": meta.PlanDirectory}
	}
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
	if contextName, contextArtifact, extraContexts, ok, err := geminiContextArtifacts(root, graph, state, meta); err != nil {
		return nil, err
	} else if ok {
		manifest["contextFileName"] = contextName
		artifacts = append(artifacts, contextArtifact)
		artifacts = append(artifacts, extraContexts...)
	}
	if extra, err := loadNativeExtraDoc(root, state, "manifest_extra", pluginmodel.NativeDocFormatJSON); err != nil {
		return nil, err
	} else if err := pluginmodel.MergeNativeExtraObject(manifest, extra, "gemini manifest.extra.json", geminiManifestManagedPaths()); err != nil {
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
	if hookPaths := state.ComponentPaths("hooks"); len(hookPaths) > 0 {
		copied, err := copyArtifacts(root, authoredComponentDir(state, "hooks", filepath.Join("targets", "gemini", "hooks")), "hooks")
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, copied...)
	} else if geminiUsesGeneratedHooks(graph, state) {
		artifacts = append(artifacts, pluginmodel.Artifact{
			RelPath: filepath.Join("hooks", "hooks.json"),
			Content: defaultGeminiHooks(entrypoint),
		})
	}
	copied, err := copyArtifactDirs(root,
		artifactDir{src: authoredComponentDir(state, "commands", filepath.Join("targets", "gemini", "commands")), dst: "commands"},
		artifactDir{src: authoredComponentDir(state, "policies", filepath.Join("targets", "gemini", "policies")), dst: "policies"},
	)
	if err != nil {
		return nil, err
	}
	return append(artifacts, copied...), nil
}

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

func validateGeminiRenderReady(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) error {
	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, validateGeminiExcludeTools(state.DocPath("package_metadata"), meta.ExcludeTools)...)
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return err
		}
		diagnostics = append(diagnostics, validateGeminiMCPServers(graph.Portable.MCP.Path, projected)...)
	}
	diagnostics = append(diagnostics, validateGeminiContext(graph, state, meta)...)
	diagnostics = append(diagnostics, validateGeminiSettings(root, state.ComponentPaths("settings"))...)
	diagnostics = append(diagnostics, validateGeminiThemes(root, state.ComponentPaths("themes"))...)
	diagnostics = append(diagnostics, validateGeminiPolicies(root, state.ComponentPaths("policies"))...)
	diagnostics = append(diagnostics, validateGeminiCommands(root, state.ComponentPaths("commands"))...)
	diagnostics = append(diagnostics, validateGeminiHookFiles(root, state.ComponentPaths("hooks"))...)
	if graph.Launcher != nil {
		diagnostics = append(diagnostics, validateGeminiHookEntrypointConsistency(root, state.ComponentPaths("hooks"), strings.TrimSpace(graph.Launcher.Entrypoint))...)
	}
	if failures := collectDiagnosticMessages(diagnostics, SeverityFailure); len(failures) > 0 {
		return fmt.Errorf(failures[0])
	}
	return nil
}

func collectDiagnosticMessages(diagnostics []Diagnostic, severity DiagnosticSeverity) []string {
	var messages []string
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == severity {
			messages = append(messages, diagnostic.Message)
		}
	}
	return messages
}
