package platformexec

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiExtensionContract(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta, extension importedGeminiExtension) ([]Diagnostic, error) {
	var diagnostics []Diagnostic
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
