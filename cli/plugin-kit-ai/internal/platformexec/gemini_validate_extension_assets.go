package platformexec

import "github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"

func validateGeminiExtensionAssetContracts(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, extension importedGeminiExtension) ([]Diagnostic, error) {
	var diagnostics []Diagnostic

	settingsDiagnostics, err := validateGeminiExtensionSettingsContract(root, state, extension)
	if err != nil {
		return nil, err
	}
	diagnostics = append(diagnostics, settingsDiagnostics...)

	themeDiagnostics, err := validateGeminiExtensionThemesContract(root, state, extension)
	if err != nil {
		return nil, err
	}
	diagnostics = append(diagnostics, themeDiagnostics...)

	if len(extension.MCPServers) > 0 {
		diagnostics = append(diagnostics, validateGeminiMCPServers("gemini-extension.json", extension.MCPServers)...)
	}

	mcpDiagnostics, err := validateGeminiExtensionMCPContract(graph, extension)
	if err != nil {
		return nil, err
	}
	diagnostics = append(diagnostics, mcpDiagnostics...)
	return diagnostics, nil
}

func validateGeminiExtensionSettingsContract(root string, state pluginmodel.TargetState, extension importedGeminiExtension) ([]Diagnostic, error) {
	settings, err := loadGeminiSettings(root, state.ComponentPaths("settings"))
	if err != nil {
		return nil, err
	}
	if len(settings) > 0 {
		if jsonDocumentsEqual(settings, extension.Settings) {
			return nil, nil
		}
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json settings do not match authored targets/gemini/settings/**",
		}}, nil
	}
	if len(extension.Settings) == 0 {
		return nil, nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeGeneratedContractInvalid,
		Path:     "gemini-extension.json",
		Target:   "gemini",
		Message:  "Gemini extension manifest gemini-extension.json may not define settings when targets/gemini/settings/** is absent",
	}}, nil
}

func validateGeminiExtensionThemesContract(root string, state pluginmodel.TargetState, extension importedGeminiExtension) ([]Diagnostic, error) {
	themes, err := loadGeminiThemes(root, state.ComponentPaths("themes"))
	if err != nil {
		return nil, err
	}
	if len(themes) > 0 {
		if jsonDocumentsEqual(themes, extension.Themes) {
			return nil, nil
		}
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json themes do not match authored targets/gemini/themes/**",
		}}, nil
	}
	if len(extension.Themes) == 0 {
		return nil, nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeGeneratedContractInvalid,
		Path:     "gemini-extension.json",
		Target:   "gemini",
		Message:  "Gemini extension manifest gemini-extension.json may not define themes when targets/gemini/themes/** is absent",
	}}, nil
}

func validateGeminiExtensionMCPContract(graph pluginmodel.PackageGraph, extension importedGeminiExtension) ([]Diagnostic, error) {
	if graph.Portable.MCP != nil {
		projected, err := renderPortableMCPForTarget(graph.Portable.MCP, "gemini")
		if err != nil {
			return nil, err
		}
		if jsonDocumentsEqual(projected, extension.MCPServers) {
			return nil, nil
		}
		return []Diagnostic{{
			Severity: SeverityFailure,
			Code:     CodeGeneratedContractInvalid,
			Path:     "gemini-extension.json",
			Target:   "gemini",
			Message:  "Gemini extension manifest gemini-extension.json mcpServers do not match authored portable MCP projection",
		}}, nil
	}
	if len(extension.MCPServers) == 0 {
		return nil, nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeGeneratedContractInvalid,
		Path:     "gemini-extension.json",
		Target:   "gemini",
		Message:  "Gemini extension manifest gemini-extension.json may not define mcpServers when portable MCP is absent",
	}}, nil
}
