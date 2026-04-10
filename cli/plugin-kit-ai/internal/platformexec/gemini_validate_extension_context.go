package platformexec

import (
	"github.com/777genius/plugin-kit-ai/cli/internal/pluginmodel"
)

func validateGeminiExtensionContextContract(root string, graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta, extension importedGeminiExtension) ([]Diagnostic, error) {
	expected, ok, err := validateGeminiExtensionContextSelection(graph, state, meta)
	if err != nil {
		return nil, err
	}
	return validateGeminiExtensionContextDiagnostics(root, expected, ok, extension), nil
}

func validateGeminiExtensionContextSelection(graph pluginmodel.PackageGraph, state pluginmodel.TargetState, meta geminiPackageMeta) (geminiContextSelection, bool, error) {
	return resolveGeminiExpectedContext(graph, state, meta)
}

func validateGeminiExtensionContextDiagnostics(root string, expected geminiContextSelection, ok bool, extension importedGeminiExtension) []Diagnostic {
	if ok {
		return validateGeminiExpectedContextContract(root, expected, extension)
	}
	return validateGeminiUnexpectedContextContract(extension)
}

func validateGeminiExpectedContextContract(root string, expected geminiContextSelection, extension importedGeminiExtension) []Diagnostic {
	return validateGeminiExpectedContext(root, expected, extension)
}

func validateGeminiUnexpectedContextContract(extension importedGeminiExtension) []Diagnostic {
	return validateGeminiUnexpectedContext(extension)
}
