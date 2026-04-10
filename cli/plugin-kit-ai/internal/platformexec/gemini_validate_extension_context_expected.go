package platformexec

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func resolveGeminiExpectedContext(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) (geminiContextSelection, bool, error) {
	return selectGeminiPrimaryContext(graph, state, meta)
}

func validateGeminiExpectedContext(root string, expected geminiContextSelection, extension importedGeminiExtension) []Diagnostic {
	var diagnostics []Diagnostic
	diagnostics = append(diagnostics, validateGeminiContextFileNameProjection(expected, extension)...)
	diagnostics = append(diagnostics, validateGeminiContextFileReadable(root, expected)...)
	return diagnostics
}

func validateGeminiContextFileNameProjection(expected geminiContextSelection, extension importedGeminiExtension) []Diagnostic {
	if strings.TrimSpace(extension.Meta.ContextFileName) == expected.ArtifactName {
		return nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeGeneratedContractInvalid,
		Path:     "gemini-extension.json",
		Target:   "gemini",
		Message:  fmt.Sprintf("Gemini extension manifest gemini-extension.json sets contextFileName %q; expected %q from authored context selection", strings.TrimSpace(extension.Meta.ContextFileName), expected.ArtifactName),
	}}
}

func validateGeminiContextFileReadable(root string, expected geminiContextSelection) []Diagnostic {
	if fileExists(filepath.Join(root, expected.ArtifactName)) {
		return nil
	}
	return []Diagnostic{{
		Severity: SeverityFailure,
		Code:     CodeGeneratedContractInvalid,
		Path:     expected.ArtifactName,
		Target:   "gemini",
		Message:  fmt.Sprintf("Gemini primary context file %s is not readable", expected.ArtifactName),
	}}
}
