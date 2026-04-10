package platformexec

import (
	"fmt"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

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
	extensionDiagnostics, err := validateGeminiExtensionContract(root, graph, state, meta, extension)
	if err != nil {
		return nil, err
	}
	diagnostics = append(diagnostics, extensionDiagnostics...)
	return diagnostics, nil
}
