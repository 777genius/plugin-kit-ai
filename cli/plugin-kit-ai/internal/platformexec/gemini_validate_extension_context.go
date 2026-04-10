package platformexec

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiExtensionContextContract(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta, extension importedGeminiExtension) ([]Diagnostic, error) {
	expected, ok, err := selectGeminiPrimaryContext(graph, state, meta)
	if err != nil {
		return nil, err
	}
	if ok {
		return validateExpectedGeminiPrimaryContext(root, expected, extension), nil
	}
	if strings.TrimSpace(extension.Meta.ContextFileName) == "" {
		return nil, nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeGeneratedContractInvalid,
		Path:     "gemini-extension.json",
		Target:   "gemini",
		Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets contextFileName %q without an authored primary context", strings.TrimSpace(extension.Meta.ContextFileName)),
	}}, nil
}

func validateExpectedGeminiPrimaryContext(root string, expected geminiContextSelection, extension importedGeminiExtension) []Diagnostic {
	var diagnostics []Diagnostic
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
	return diagnostics
}
