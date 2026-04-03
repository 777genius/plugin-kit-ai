package platformexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
	"github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type geminiAdapter struct{}

func (geminiAdapter) ID() string { return "gemini" }

func (geminiAdapter) DetectNative(root string) bool {
	return fileExists(filepath.Join(root, "gemini-extension.json"))
}

func (geminiAdapter) RefineDiscovery(root string, state *pluginmodel.TargetState) error {
	if rel := state.DocPath("package_metadata"); strings.TrimSpace(rel) != "" {
		if _, ok, err := readYAMLDoc[geminiPackageMeta](root, rel); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		} else if !ok {
			return nil
		}
	}
	for _, rel := range state.ComponentPaths("hooks") {
		expected := filepath.ToSlash(filepath.Join("targets", "gemini", "hooks", "hooks.json"))
		if rel != expected {
			return fmt.Errorf("unsupported Gemini hooks layout: use only %s", expected)
		}
	}
	for _, rel := range append(append([]string{}, state.ComponentPaths("settings")...), state.ComponentPaths("themes")...) {
		if !geminiYAMLFileRe.MatchString(rel) {
			kind := "theme"
			if strings.Contains(rel, "/settings/") {
				kind = "setting"
			}
			return fmt.Errorf("unsupported Gemini %s file %s: use .yaml or .yml", kind, rel)
		}
	}
	return nil
}

func (geminiAdapter) Import(root string, seed ImportSeed) (ImportResult, error) {
	result := ImportResult{
		Manifest: seed.Manifest,
		Launcher: seed.Launcher,
	}
	hooksBody, hookBodyErr := os.ReadFile(filepath.Join(root, "hooks", "hooks.json"))
	copied, err := copySingleArtifactIfExists(root, filepath.Join("hooks", "hooks.json"), filepath.Join("targets", "gemini", "hooks", "hooks.json"))
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, copied...)
	if hookBodyErr == nil {
		if _, err := parseGeminiHooks(hooksBody); err != nil {
			return ImportResult{}, fmt.Errorf("parse %s: %w", filepath.ToSlash(filepath.Join("hooks", "hooks.json")), err)
		}
		if entrypoint, ok := inferGeminiEntrypoint(hooksBody); ok {
			if result.Launcher == nil {
				result.Launcher = &pluginmodel.Launcher{
					Runtime:    "go",
					Entrypoint: entrypoint,
				}
			} else {
				result.Launcher.Entrypoint = entrypoint
			}
		}
	}
	copied, err = copyArtifactDirs(root,
		artifactDir{src: "commands", dst: filepath.Join("targets", "gemini", "commands")},
		artifactDir{src: "policies", dst: filepath.Join("targets", "gemini", "policies")},
		artifactDir{src: "contexts", dst: filepath.Join("targets", "gemini", "contexts")},
	)
	if err != nil {
		return ImportResult{}, err
	}
	result.Artifacts = append(result.Artifacts, copied...)

	data, ok, err := readImportedGeminiExtension(root)
	if err != nil {
		return ImportResult{}, err
	}
	if ok {
		if strings.TrimSpace(data.Name) != "" {
			result.Manifest.Name = data.Name
		}
		if strings.TrimSpace(data.Version) != "" {
			result.Manifest.Version = data.Version
		}
		if strings.TrimSpace(data.Description) != "" {
			result.Manifest.Description = data.Description
		}
		if len(data.MCPServers) > 0 {
			artifact, err := importedPortableMCPArtifact("gemini", data.MCPServers)
			if err != nil {
				return ImportResult{}, err
			}
			result.Artifacts = append(result.Artifacts, artifact)
		}
		if body := importedGeminiPackageYAML(data.Meta); len(body) > 0 {
			result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{RelPath: filepath.Join("targets", "gemini", "package.yaml"), Content: body})
		}
		result.Artifacts = append(result.Artifacts, importedGeminiSettingsArtifacts(data.Settings)...)
		result.Artifacts = append(result.Artifacts, importedGeminiThemeArtifacts(data.Themes)...)
		if len(data.Extra) > 0 {
			result.Artifacts = append(result.Artifacts, pluginmodel.Artifact{RelPath: filepath.Join("targets", "gemini", "manifest.extra.json"), Content: mustJSON(data.Extra)})
			result.Warnings = append(result.Warnings, pluginmodel.Warning{
				Kind:    pluginmodel.WarningFidelity,
				Path:    filepath.ToSlash(filepath.Join("targets", "gemini", "manifest.extra.json")),
				Message: "preserved additional Gemini manifest fields under targets/gemini/manifest.extra.json",
			})
		}
		if contextName := importedGeminiPrimaryContextName(root, data.Meta); contextName != "" {
			contextArtifacts, err := copySingleArtifactIfExists(root, contextName, filepath.Join("targets", "gemini", "contexts", filepath.Base(contextName)))
			if err != nil {
				return ImportResult{}, err
			}
			result.Artifacts = append(result.Artifacts, contextArtifacts...)
		}
	}
	result.Artifacts = compactArtifacts(result.Artifacts)
	return result, nil
}

func (geminiAdapter) Render(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]pluginmodel.Artifact, error) {
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
	if hookPaths := state.ComponentPaths("hooks"); len(hookPaths) > 0 {
		copied, err := copyArtifacts(root, filepath.Join("targets", "gemini", "hooks"), "hooks")
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
		artifactDir{src: filepath.Join("targets", "gemini", "commands"), dst: "commands"},
		artifactDir{src: filepath.Join("targets", "gemini", "policies"), dst: "policies"},
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

func (geminiAdapter) Validate(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState) ([]Diagnostic, error) {
	meta, _, err := readYAMLDoc[geminiPackageMeta](root, state.DocPath("package_metadata"))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", state.DocPath("package_metadata"), err)
	}
	hookPaths := state.ComponentPaths("hooks")
	var diagnostics []Diagnostic
	if base := geminiExtensionDirBase(root); base != graph.Manifest.Name {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityWarning,
			Code:     CodeGeminiDirNameMismatch,
			Path:     root,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension directory basename %q does not match extension name %q", base, graph.Manifest.Name),
		})
	}
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return nil, err
		}
		diagnostics = append(diagnostics, validateGeminiMCPServers(graph.Portable.MCP.Path, projected)...)
	}
	diagnostics = append(diagnostics, validateGeminiExcludeTools(state.DocPath("package_metadata"), meta.ExcludeTools)...)
	diagnostics = append(diagnostics, validateGeminiContext(graph, state, meta)...)
	diagnostics = append(diagnostics, validateGeminiSettings(root, state.ComponentPaths("settings"))...)
	diagnostics = append(diagnostics, validateGeminiThemes(root, state.ComponentPaths("themes"))...)
	diagnostics = append(diagnostics, validateGeminiPolicies(root, state.ComponentPaths("policies"))...)
	diagnostics = append(diagnostics, validateGeminiCommands(root, state.ComponentPaths("commands"))...)
	diagnostics = append(diagnostics, validateGeminiHookFiles(root, hookPaths)...)
	if graph.Launcher != nil {
		diagnostics = append(diagnostics, validateGeminiHookEntrypointConsistency(root, hookPaths, strings.TrimSpace(graph.Launcher.Entrypoint))...)
	}
	diagnostics = append(diagnostics, validateGeminiGeneratedHooks(root, graph, hookPaths)...)
	extension, ok, err := readImportedGeminiExtension(root)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json is invalid: %v", err),
		})
		return diagnostics, nil
	}
	if !ok {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json is not readable",
		})
		return diagnostics, nil
	}
	if strings.TrimSpace(extension.Name) != strings.TrimSpace(graph.Manifest.Name) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets name %q; expected %q from plugin.yaml", strings.TrimSpace(extension.Name), strings.TrimSpace(graph.Manifest.Name)),
		})
	}
	if strings.TrimSpace(extension.Version) != strings.TrimSpace(graph.Manifest.Version) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets version %q; expected %q from plugin.yaml", strings.TrimSpace(extension.Version), strings.TrimSpace(graph.Manifest.Version)),
		})
	}
	if strings.TrimSpace(extension.Description) != strings.TrimSpace(graph.Manifest.Description) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets description %q; expected %q from plugin.yaml", strings.TrimSpace(extension.Description), strings.TrimSpace(graph.Manifest.Description)),
		})
	}
	if !geminiPackageMetaEqual(meta, extension.Meta) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json package metadata does not match targets/gemini/package.yaml",
		})
	}
	if settings, err := loadGeminiSettings(root, state.ComponentPaths("settings")); err != nil {
		return nil, err
	} else if len(settings) > 0 {
		if !jsonDocumentsEqual(settings, extension.Settings) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  "Gemini extension manifest gemini-extension.json settings do not match authored targets/gemini/settings/**",
			})
		}
	} else if len(extension.Settings) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json may not define settings when targets/gemini/settings/** is absent",
		})
	}
	if themes, err := loadGeminiThemes(root, state.ComponentPaths("themes")); err != nil {
		return nil, err
	} else if len(themes) > 0 {
		if !jsonDocumentsEqual(themes, extension.Themes) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  "Gemini extension manifest gemini-extension.json themes do not match authored targets/gemini/themes/**",
			})
		}
	} else if len(extension.Themes) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json may not define themes when targets/gemini/themes/** is absent",
		})
	}
	if len(extension.MCPServers) > 0 {
		diagnostics = append(diagnostics, validateGeminiMCPServers("gemini-extension.json", extension.MCPServers)...)
	}
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return nil, err
		}
		if !jsonDocumentsEqual(projected, extension.MCPServers) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  "Gemini extension manifest gemini-extension.json mcpServers do not match authored portable MCP projection",
			})
		}
	} else if len(extension.MCPServers) > 0 {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json may not define mcpServers when portable MCP is absent",
		})
	}
	if expected, ok, err := selectGeminiPrimaryContext(graph, state, meta); err != nil {
		return nil, err
	} else if ok {
		if strings.TrimSpace(extension.Meta.ContextFileName) != expected.ArtifactName {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     "gemini-extension.json",
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets contextFileName %q; expected %q from authored context selection", strings.TrimSpace(extension.Meta.ContextFileName), expected.ArtifactName),
			})
		}
		if !fileExists(filepath.Join(root, expected.ArtifactName)) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     expected.ArtifactName,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini primary context file %s is not readable", expected.ArtifactName),
			})
		}
	} else if strings.TrimSpace(extension.Meta.ContextFileName) != "" {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets contextFileName %q without an authored primary context", strings.TrimSpace(extension.Meta.ContextFileName)),
		})
	}
	return diagnostics, nil
}

func validateGeminiGeneratedHooks(root string, graph pluginmodel.PackageGraph, authoredHookPaths []string) []Diagnostic {
	const generatedHooksPath = "hooks/hooks.json"
	var diagnostics []Diagnostic
	body, err := os.ReadFile(filepath.Join(root, generatedHooksPath))
	if len(authoredHookPaths) > 0 {
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json is not readable",
			})
			return diagnostics
		}
		renderedHooks, err := parseGeminiHooks(body)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini generated hooks file %s is invalid: %v", generatedHooksPath, err),
			})
			return diagnostics
		}
		authoredBody, readErr := os.ReadFile(filepath.Join(root, authoredHookPaths[0]))
		if readErr != nil {
			return diagnostics
		}
		authoredHooks, parseErr := parseGeminiHooks(authoredBody)
		if parseErr != nil {
			return diagnostics
		}
		if !jsonDocumentsEqual(authoredHooks, renderedHooks) {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json does not match authored targets/gemini/hooks/hooks.json",
			})
		}
		return diagnostics
	}
	if !geminiUsesGeneratedHooks(graph, pluginmodel.TargetState{Target: "gemini"}) {
		if err == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeGeneratedContractInvalid,
				Path:     generatedHooksPath,
				Target:   "gemini",
				Message:  "Gemini generated hooks/hooks.json may not exist when no authored hooks or generated launcher hooks are expected",
			})
		}
		return diagnostics
	}
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  "Gemini generated hooks/hooks.json is not readable",
		})
		return diagnostics
	}
	renderedHooks, err := parseGeminiHooks(body)
	if err != nil {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  fmt.Sprintf("Gemini generated hooks file %s is invalid: %v", generatedHooksPath, err),
		})
		return diagnostics
	}
	expectedHooks, err := parseGeminiHooks(defaultGeminiHooks(strings.TrimSpace(graph.Launcher.Entrypoint)))
	if err != nil {
		return diagnostics
	}
	if !jsonDocumentsEqual(expectedHooks, renderedHooks) {
		diagnostics = append(diagnostics, Diagnostic{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     generatedHooksPath,
			Target:   "gemini",
			Message:  "Gemini generated hooks/hooks.json does not match the managed launcher-derived hooks projection",
		})
	}
	return diagnostics
}

func geminiPackageMetaEqual(left, right geminiPackageMeta) bool {
	return strings.TrimSpace(left.ContextFileName) == strings.TrimSpace(right.ContextFileName) &&
		slices.Equal(normalizeGeminiExcludeTools(left.ExcludeTools), normalizeGeminiExcludeTools(right.ExcludeTools)) &&
		strings.TrimSpace(left.MigratedTo) == strings.TrimSpace(right.MigratedTo) &&
		strings.TrimSpace(left.PlanDirectory) == strings.TrimSpace(right.PlanDirectory)
}

func geminiExtensionDirBase(root string) string {
	abs, err := filepath.Abs(root)
	if err == nil {
		return filepath.Base(filepath.Clean(abs))
	}
	return filepath.Base(filepath.Clean(root))
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

func importedGeminiPackageYAML(meta geminiPackageMeta) []byte {
	if len(meta.ExcludeTools) == 0 &&
		strings.TrimSpace(meta.ContextFileName) == "" &&
		strings.TrimSpace(meta.PlanDirectory) == "" {
		return nil
	}
	return mustYAML(meta)
}

func importedGeminiPrimaryContextName(root string, meta geminiPackageMeta) string {
	if strings.TrimSpace(meta.ContextFileName) != "" {
		return filepath.Base(strings.TrimSpace(meta.ContextFileName))
	}
	if fileExists(filepath.Join(root, "GEMINI.md")) {
		return "GEMINI.md"
	}
	return ""
}

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

func selectGeminiPrimaryContext(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) (geminiContextSelection, bool, error) {
	candidates := geminiContextCandidates(graph, state)
	selected := strings.TrimSpace(meta.ContextFileName)
	if selected != "" {
		matches := candidatesByArtifactName(candidates, selected)
		switch len(matches) {
		case 0:
			return geminiContextSelection{}, false, fmt.Errorf("gemini context_file_name %q does not resolve to a Gemini-native context source", selected)
		case 1:
			return matches[0], true, nil
		default:
			return geminiContextSelection{}, false, fmt.Errorf("gemini context_file_name %q is ambiguous across multiple context sources", selected)
		}
	}
	fallback := candidatesByArtifactName(candidates, "GEMINI.md")
	switch len(fallback) {
	case 1:
		return fallback[0], true, nil
	case 0:
		if len(candidates) == 0 {
			return geminiContextSelection{}, false, nil
		}
		if len(candidates) == 1 {
			return candidates[0], true, nil
		}
		return geminiContextSelection{}, false, fmt.Errorf("gemini primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly")
	default:
		return geminiContextSelection{}, false, fmt.Errorf("gemini primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly")
	}
}

func geminiContextCandidates(graph pluginmodel.PackageGraph, state pluginmodel.TargetState) []geminiContextSelection {
	var out []geminiContextSelection
	seen := map[string]struct{}{}
	for _, rel := range state.ComponentPaths("contexts") {
		artifactName := filepath.Base(rel)
		if artifactName == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		out = append(out, geminiContextSelection{ArtifactName: artifactName, SourcePath: rel})
	}
	slices.SortFunc(out, func(a, b geminiContextSelection) int {
		if cmp := strings.Compare(a.ArtifactName, b.ArtifactName); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.SourcePath, b.SourcePath)
	})
	return out
}

func candidatesByArtifactName(candidates []geminiContextSelection, name string) []geminiContextSelection {
	var out []geminiContextSelection
	for _, candidate := range candidates {
		if candidate.ArtifactName == name {
			out = append(out, candidate)
		}
	}
	return out
}

func validateGeminiContext(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) []Diagnostic {
	selected := strings.TrimSpace(meta.ContextFileName)
	candidates := geminiContextMatches(graph, state, "")
	if selected != "" {
		matches := geminiContextMatches(graph, state, selected)
		switch len(matches) {
		case 0:
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     state.DocPath("package_metadata"),
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini context_file_name %q does not resolve to a Gemini-native context source", selected),
			}}
		case 1:
			return nil
		default:
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     state.DocPath("package_metadata"),
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini context_file_name %q is ambiguous across multiple context sources", selected),
			}}
		}
	}
	geminiMD := geminiContextMatches(graph, state, "GEMINI.md")
	if len(geminiMD) > 1 {
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeManifestInvalid,
			Path:     "contexts",
			Target:   "gemini",
			Message:  "Gemini primary context selection is ambiguous for GEMINI.md; keep one root context or set context_file_name explicitly",
		}}
	}
	if len(geminiMD) == 1 || len(candidates) <= 1 {
		return nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeManifestInvalid,
		Path:     "contexts",
		Target:   "gemini",
		Message:  "Gemini primary context selection is ambiguous; set targets/gemini/package.yaml context_file_name explicitly",
	}}
}

func normalizeGeminiExcludeTools(values []string) []string {
	var out []string
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func validateGeminiExcludeTools(path string, values []string) []Diagnostic {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return []Diagnostic{{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  "Gemini exclude_tools entries must be non-empty strings naming built-in tools",
			}}
		}
	}
	return nil
}

func validateGeminiMCPServers(path string, servers map[string]any) []Diagnostic {
	var diagnostics []Diagnostic
	for serverName, raw := range servers {
		server, ok := raw.(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q must be a JSON object", serverName),
			})
			continue
		}
		if _, blocked := server["trust"]; blocked {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may not set trust", serverName),
			})
		}
		command, hasCommand := geminiOptionalString(server["command"])
		url, hasURL := geminiOptionalString(server["url"])
		httpURL, hasHTTPURL := geminiOptionalString(server["httpUrl"])
		transportCount := 0
		if hasCommand {
			transportCount++
		}
		if hasURL {
			transportCount++
		}
		if hasHTTPURL {
			transportCount++
		}
		if transportCount != 1 {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q must define exactly one transport via command, url, or httpUrl", serverName),
			})
		}
		if hasArgs := server["args"] != nil; hasArgs && !hasCommand {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may only use args with command-based stdio transport", serverName),
			})
		}
		if hasEnv := server["env"] != nil; hasEnv && !hasCommand {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may only use env with command-based stdio transport", serverName),
			})
		}
		if hasCwd := server["cwd"] != nil; hasCwd && !hasCommand {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q may only use cwd with command-based stdio transport", serverName),
			})
		}
		if value, ok := server["args"]; ok {
			items, valid := geminiStringSlice(value)
			if !valid {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q args must be an array of strings", serverName),
				})
			} else if len(items) == 0 {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q args may not be empty when provided", serverName),
				})
			}
		}
		if value, ok := server["env"]; ok {
			if _, valid := geminiStringMap(value); !valid {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q env must be an object of string values", serverName),
				})
			}
		}
		if value, ok := server["cwd"]; ok {
			if cwd, ok := value.(string); !ok || strings.TrimSpace(cwd) == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityFailure,
					Code:     CodeManifestInvalid,
					Path:     path,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension MCP server %q cwd must be a non-empty string", serverName),
				})
			}
		}
		if hasCommand && strings.Contains(command, " ") && server["args"] == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityWarning,
				Code:     CodeGeminiMCPCommandStyle,
				Path:     path,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini extension MCP server %q uses a space-delimited command string; prefer command plus args", serverName),
			})
		}
		_ = url
		_ = httpURL
	}
	return diagnostics
}

func geminiOptionalString(value any) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	return text, true
}

func geminiStringSlice(value any) ([]string, bool) {
	raw, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, false
		}
		out = append(out, strings.TrimSpace(text))
	}
	return out, true
}

func geminiStringMap(value any) (map[string]string, bool) {
	raw, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	out := make(map[string]string, len(raw))
	for key, item := range raw {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		out[key] = text
	}
	return out, true
}

func geminiContextMatches(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, name string) []string {
	var matches []string
	seen := map[string]struct{}{}
	for _, rel := range state.ComponentPaths("contexts") {
		rel = filepath.ToSlash(rel)
		if name == "" || filepath.Base(rel) == name {
			if _, ok := seen[rel]; ok {
				continue
			}
			seen[rel] = struct{}{}
			matches = append(matches, rel)
		}
	}
	slices.Sort(matches)
	return matches
}

func validateGeminiSettings(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	seenNames := map[string]string{}
	seenEnvVars := map[string]string{}
	for _, rel := range rels {
		body, raw, err := readGeminiYAMLMap(root, rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		var setting geminiSetting
		if err := yaml.Unmarshal(body, &setting); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		if message := validateGeminiSettingMap(rel, raw, setting); message != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s: %s", rel, message),
			})
			continue
		}
		nameKey := strings.ToLower(strings.TrimSpace(setting.Name))
		if prev, ok := seenNames[nameKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates setting name %q already declared in %s", rel, setting.Name, prev),
			})
		} else {
			seenNames[nameKey] = rel
		}
		envKey := strings.ToLower(strings.TrimSpace(setting.EnvVar))
		if prev, ok := seenEnvVars[envKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini setting file %s duplicates env_var %q already declared in %s", rel, setting.EnvVar, prev),
			})
		} else {
			seenEnvVars[envKey] = rel
		}
	}
	return diagnostics
}

func validateGeminiThemes(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	seenNames := map[string]string{}
	for _, rel := range rels {
		_, raw, err := readGeminiYAMLMap(root, rel)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s is invalid YAML: %v", rel, err),
			})
			continue
		}
		name, _ := raw["name"].(string)
		if message := validateGeminiThemeMap(rel, raw); message != "" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s: %s", rel, message),
			})
			continue
		}
		name = strings.TrimSpace(name)
		nameKey := strings.ToLower(name)
		if prev, ok := seenNames[nameKey]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini theme file %s duplicates theme name %q already declared in %s", rel, name, prev),
			})
			continue
		}
		seenNames[nameKey] = rel
	}
	return diagnostics
}

func validateGeminiPolicies(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini policy file %s is not readable: %v", rel, err),
			})
			continue
		}
		text := string(body)
		for _, key := range []string{"allow", "yolo"} {
			if strings.Contains(text, key+" =") {
				diagnostics = append(diagnostics, Diagnostic{
					Severity: SeverityWarning,
					Code:     CodeGeminiPolicyIgnored,
					Path:     rel,
					Target:   "gemini",
					Message:  fmt.Sprintf("Gemini extension policies ignore %q at extension tier", key),
				})
			}
		}
	}
	return diagnostics
}

func validateGeminiCommands(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		if filepath.Ext(rel) != ".toml" {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s must use the .toml extension", rel),
			})
			continue
		}
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s is not readable: %v", rel, err),
			})
			continue
		}
		var discard map[string]any
		if err := toml.Unmarshal(body, &discard); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini command file %s is invalid TOML: %v", rel, err),
			})
		}
	}
	return diagnostics
}

func validateGeminiHookFiles(root string, rels []string) []Diagnostic {
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini JSON asset %s is not readable: %v", rel, err),
			})
			continue
		}
		var discard map[string]any
		if err := json.Unmarshal(body, &discard); err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s is invalid JSON: %v", rel, err),
			})
			continue
		}
		hooks, ok := discard["hooks"].(map[string]any)
		if !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s must define a top-level hooks object", rel),
			})
			continue
		}
		if hooks == nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s must define a top-level hooks object", rel),
			})
		}
	}
	return diagnostics
}

func validateGeminiHookEntrypointConsistency(root string, rels []string, entrypoint string) []Diagnostic {
	if strings.TrimSpace(entrypoint) == "" {
		return nil
	}
	var diagnostics []Diagnostic
	for _, rel := range rels {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		mismatches, err := validateGeminiHookEntrypoints(body, entrypoint)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeManifestInvalid,
				Path:     rel,
				Target:   "gemini",
				Message:  fmt.Sprintf("Gemini hooks file %s is invalid JSON: %v", rel, err),
			})
			continue
		}
		for _, msg := range mismatches {
			diagnostics = append(diagnostics, Diagnostic{
				Severity: SeverityFailure,
				Code:     CodeEntrypointMismatch,
				Path:     rel,
				Target:   "gemini",
				Message:  msg,
			})
		}
	}
	return diagnostics
}
